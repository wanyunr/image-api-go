package app

import (
	"bufio"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrPoolEmpty = errors.New("URL pool is empty")

// ErrLocalImageMissing 表示本地图源里的文件在扫描后已不存在（被删除或移动）。
var ErrLocalImageMissing = errors.New("local image file is missing")

const (
	metadataFetchConcurrency = 1
	metadataFetchDelay       = 200 * time.Millisecond
	metadataWorkerInterval   = 30 * time.Second
	metadataMaxAttempts      = 3
)

type pendingItem struct {
	url         string
	orientation string
	attempts    int
}

type metaUpdate struct {
	info        ImageInfo
	orientation string
}

type PoolSnapshot struct {
	Portrait   []ImageInfo
	Landscape  []ImageInfo
	Retry      []pendingItem
	Dead       []DeadItem
	LocalFiles map[string]string
}

// URLPool 通过 atomic.Pointer 提供无锁读取。
// 热路径（Random/Count）使用 atomic.Load，低开销。
// 变更操作（Load/retryPending/ClearDead）通过 mu 串行化，保证原子替换。
type URLPool struct {
	current      atomic.Pointer[PoolSnapshot]
	mu           sync.Mutex
	cacheDir     string
	metadataMode string
	rng          *rand.Rand
	rngMu        sync.Mutex
}

type LoadStats struct {
	CreatedFiles   bool
	Portrait       int
	Landscape      int
	LocalPortrait  int
	LocalLandscape int
	Pending        int
	Dead           int
}

func NewURLPool() *URLPool {
	p := &URLPool{rng: newRandGenerator(), metadataMode: "prefetch"}
	p.current.Store(&PoolSnapshot{})
	return p
}

func (p *URLPool) SetMetadataMode(mode string) {
	if mode == "" {
		mode = "prefetch"
	}
	p.metadataMode = mode
}

func newRandGenerator() *rand.Rand {
	var seed int64
	buf := make([]byte, 8)
	if _, err := cryptoRand.Read(buf); err == nil {
		seed = int64(binary.LittleEndian.Uint64(buf))
	} else {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
}

// Load 完整构建新 Snapshot 后原子替换。
// 先读文件 → 扫描本地目录 → 构建 pool → 构建 metadata → 一切就绪 → 原子 swap。
func (p *URLPool) Load(portraitFile, landscapeFile, cacheDir, source string, local LocalConfig, basePath string) (LoadStats, error) {
	var portraitURLs, landscapeURLs []string
	var createdPortrait, createdLandscape bool
	var err error

	useLocal := source != SourceURL
	if !useLocal {
		portraitURLs, createdPortrait, err = loadOrCreate(portraitFile, "# portrait URL list\n# Add one image URL per line\n", "竖屏")
		if err != nil {
			return LoadStats{}, err
		}
		landscapeURLs, createdLandscape, err = loadOrCreate(landscapeFile, "# landscape URL list\n# Add one image URL per line\n", "横屏")
		if err != nil {
			return LoadStats{}, err
		}

		portraitURLs = dedupe(portraitURLs)
		landscapeURLs = dedupe(landscapeURLs)
	}

	var localPortrait, localLandscape []ImageInfo
	var localFiles map[string]string
	if useLocal {
		localPortrait, localLandscape, localFiles, err = scanLocalImages(local, basePath)
		if err != nil {
			return LoadStats{}, err
		}
	}

	p.cacheDir = cacheDir

	stats := LoadStats{
		CreatedFiles:   createdPortrait || createdLandscape,
		LocalPortrait:  len(localPortrait),
		LocalLandscape: len(localLandscape),
	}

	if p.metadataMode == "on_request" {
		snapshot := &PoolSnapshot{
			Portrait:   append(makeInfos(portraitURLs), localPortrait...),
			Landscape:  append(makeInfos(landscapeURLs), localLandscape...),
			LocalFiles: localFiles,
		}
		p.mu.Lock()
		p.current.Store(snapshot)
		p.mu.Unlock()
		stats.Portrait = len(snapshot.Portrait)
		stats.Landscape = len(snapshot.Landscape)
		return stats, nil
	}

	cache, _ := LoadCache(cacheDir)
	if cache == nil {
		cache = &Cache{Version: cacheVersion}
	}
	cacheP := make(map[string]ImageInfo, len(cache.Portrait))
	for _, info := range cache.Portrait {
		cacheP[info.URL] = info
	}
	cacheL := make(map[string]ImageInfo, len(cache.Landscape))
	for _, info := range cache.Landscape {
		cacheL[info.URL] = info
	}

	portrait, retryP := buildWithCache(portraitURLs, cacheP, "portrait")
	landscape, retryL := buildWithCache(landscapeURLs, cacheL, "landscape")

	portrait = append(portrait, localPortrait...)
	landscape = append(landscape, localLandscape...)

	urlSet := make(map[string]bool, len(portraitURLs)+len(landscapeURLs))
	for _, u := range portraitURLs {
		urlSet[u] = true
	}
	for _, u := range landscapeURLs {
		urlSet[u] = true
	}
	var dead []DeadItem
	for _, d := range cache.Dead {
		if urlSet[d.URL] {
			dead = append(dead, d)
		}
	}

	// 所有数据就绪，一次原子替换
	snapshot := &PoolSnapshot{
		Portrait:   portrait,
		Landscape:  landscape,
		Retry:      append(retryP, retryL...),
		Dead:       dead,
		LocalFiles: localFiles,
	}
	p.mu.Lock()
	p.current.Store(snapshot)
	p.mu.Unlock()

	if err := p.saveCache(); err != nil {
		return LoadStats{}, err
	}

	stats.Portrait = len(portrait)
	stats.Landscape = len(landscape)
	stats.Pending = len(retryP) + len(retryL)
	stats.Dead = len(dead)
	return stats, nil
}

func (p *URLPool) RetryPending() {
	p.retryPending()
}

func (p *URLPool) MetadataWorker(done <-chan struct{}) {
	ticker := time.NewTicker(metadataWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			p.retryPending()
		}
	}
}

func (p *URLPool) retryPending() {
	p.mu.Lock()
	snapshot := p.current.Load()
	items := snapshot.Retry
	// 清空当前 retry，避免重入重复抓取
	p.current.Store(&PoolSnapshot{
		Portrait:   snapshot.Portrait,
		Landscape:  snapshot.Landscape,
		Dead:       snapshot.Dead,
		LocalFiles: snapshot.LocalFiles,
	})
	p.mu.Unlock()

	if len(items) == 0 {
		return
	}

	sem := make(chan struct{}, metadataFetchConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var added []metaUpdate
	var still []pendingItem
	var newlyDead []DeadItem

	for _, it := range items {
		wg.Add(1)
		go func(it pendingItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			time.Sleep(metadataFetchDelay)

			info := FetchImageInfo(it.url)
			mu.Lock()
			defer mu.Unlock()
			if info.Width > 0 && info.Height > 0 {
				added = append(added, metaUpdate{info: info, orientation: it.orientation})
				return
			}
			it.attempts++
			if it.attempts >= metadataMaxAttempts {
				newlyDead = append(newlyDead, DeadItem{
					URL: it.url, Orientation: it.orientation, Attempts: it.attempts,
				})
			} else {
				still = append(still, it)
			}
		}(it)
	}
	wg.Wait()

	// 构建新 snapshot（含 Reload 期间可能新增的 retry 项）
	p.mu.Lock()
	snapshot = p.current.Load()
	newPortrait := applyUpdates(snapshot.Portrait, added, "portrait")
	newLandscape := applyUpdates(snapshot.Landscape, added, "landscape")
	newDead := make([]DeadItem, 0, len(snapshot.Dead)+len(newlyDead))
	newDead = append(newDead, snapshot.Dead...)
	newDead = append(newDead, newlyDead...)
	newRetry := make([]pendingItem, 0, len(snapshot.Retry)+len(still))
	newRetry = append(newRetry, snapshot.Retry...)
	newRetry = append(newRetry, still...)
	p.current.Store(&PoolSnapshot{
		Portrait:   newPortrait,
		Landscape:  newLandscape,
		Retry:      newRetry,
		Dead:       newDead,
		LocalFiles: snapshot.LocalFiles,
	})
	p.mu.Unlock()

	if len(added) > 0 || len(newlyDead) > 0 {
		logStructured("metadata_retry", map[string]any{"added": len(added), "dead": len(newlyDead), "remaining": len(still)})
		_ = p.saveCache()
	}
}

func applyUpdates(pool []ImageInfo, updates []metaUpdate, orientation string) []ImageInfo {
	updateMap := make(map[string]ImageInfo)
	for _, u := range updates {
		if u.orientation == orientation {
			updateMap[u.info.URL] = u.info
		}
	}
	if len(updateMap) == 0 {
		return pool
	}
	newPool := make([]ImageInfo, len(pool))
	copy(newPool, pool)
	for i := range newPool {
		if info, ok := updateMap[newPool[i].URL]; ok {
			newPool[i] = info
		}
	}
	return newPool
}

func (p *URLPool) Random(orientation string) (ImageInfo, error) {
	snapshot := p.current.Load()

	var pool []ImageInfo
	switch orientation {
	case "portrait":
		pool = snapshot.Portrait
	case "landscape":
		pool = snapshot.Landscape
	default:
		return ImageInfo{}, fmt.Errorf("无效的 orientation: %s", orientation)
	}

	if len(pool) == 0 {
		return ImageInfo{}, fmt.Errorf("%w: %s", ErrPoolEmpty, orientation)
	}

	p.rngMu.Lock()
	idx := p.rng.Intn(len(pool))
	p.rngMu.Unlock()
	return pool[idx], nil
}

// RandomExcluding 在不含 exclude 的候选中随机取一张（不放回抽样）。
// 用于本地文件失效后的重选：既保证循环有限次结束，又保证只要还有可用
// 候选就一定能抽到。候选耗尽时返回 ErrPoolEmpty。
func (p *URLPool) RandomExcluding(orientation string, exclude map[string]struct{}) (ImageInfo, error) {
	snapshot := p.current.Load()

	var pool []ImageInfo
	switch orientation {
	case "portrait":
		pool = snapshot.Portrait
	case "landscape":
		pool = snapshot.Landscape
	default:
		return ImageInfo{}, fmt.Errorf("无效的 orientation: %s", orientation)
	}

	candidates := make([]ImageInfo, 0, len(pool))
	for _, img := range pool {
		if img.LocalPath != "" {
			if _, skip := exclude[img.LocalPath]; skip {
				continue
			}
		}
		candidates = append(candidates, img)
	}
	if len(candidates) == 0 {
		return ImageInfo{}, fmt.Errorf("%w: %s", ErrPoolEmpty, orientation)
	}

	p.rngMu.Lock()
	idx := p.rng.Intn(len(candidates))
	p.rngMu.Unlock()
	return candidates[idx], nil
}

func (p *URLPool) Count() (portrait, landscape int) {
	s := p.current.Load()
	return len(s.Portrait), len(s.Landscape)
}

func (p *URLPool) Pending() int {
	return len(p.current.Load().Retry)
}

func (p *URLPool) Dead() int {
	return len(p.current.Load().Dead)
}

// LocalFile 返回本地静态路由对应的绝对路径。仅返回已扫描到的文件。
func (p *URLPool) LocalFile(routePath string) (string, bool) {
	path, ok := p.current.Load().LocalFiles[routePath]
	return path, ok
}

func (p *URLPool) ClearDead() {
	p.mu.Lock()
	s := p.current.Load()
	p.current.Store(&PoolSnapshot{
		Portrait:   s.Portrait,
		Landscape:  s.Landscape,
		Retry:      s.Retry,
		LocalFiles: s.LocalFiles,
	})
	p.mu.Unlock()
	_ = p.saveCache()
}

func (p *URLPool) saveCache() error {
	if p.metadataMode != "prefetch" || p.cacheDir == "" {
		return nil
	}
	s := p.current.Load()
	cache := &Cache{
		Portrait:  remoteInfos(s.Portrait),
		Landscape: remoteInfos(s.Landscape),
		Dead:      append([]DeadItem(nil), s.Dead...),
	}
	return SaveCache(p.cacheDir, cache)
}

func buildWithCache(urls []string, cacheMap map[string]ImageInfo, orientation string) ([]ImageInfo, []pendingItem) {
	pool := make([]ImageInfo, 0, len(urls))
	retry := make([]pendingItem, 0, len(urls))
	for _, u := range urls {
		if info, ok := cacheMap[u]; ok {
			pool = append(pool, info)
		} else {
			pool = append(pool, ImageInfo{URL: u})
		}
		retry = append(retry, pendingItem{url: u, orientation: orientation})
	}
	return pool, retry
}

func makeInfos(urls []string) []ImageInfo {
	infos := make([]ImageInfo, len(urls))
	for i, u := range urls {
		infos[i] = ImageInfo{URL: u}
	}
	return infos
}

func loadOrCreate(path, defaultContent, name string) ([]string, bool, error) {
	urls, err := loadURLFile(path)
	if err == nil {
		return urls, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("加载%s URL 列表失败: %w", name, err)
	}
	if err := createDefaultURLFile(path, defaultContent); err != nil {
		return nil, false, fmt.Errorf("创建默认%s URL 列表失败: %w", name, err)
	}
	urls, err = loadURLFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("加载%s URL 列表失败: %w", name, err)
	}
	return urls, true, nil
}

func createDefaultURLFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func dedupe(urls []string) []string {
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

func loadURLFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var urls []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !isValidURL(line) {
			continue
		}
		urls = append(urls, line)
	}
	return urls, scanner.Err()
}

func isValidURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// remoteInfos 过滤掉本地图片，缓存只持久化远程 URL。
func remoteInfos(infos []ImageInfo) []ImageInfo {
	out := make([]ImageInfo, 0, len(infos))
	for _, info := range infos {
		if info.LocalPath == "" {
			out = append(out, info)
		}
	}
	return out
}

func scanLocalImages(local LocalConfig, basePath string) ([]ImageInfo, []ImageInfo, map[string]string, error) {
	files := make(map[string]string)
	// seen 在横竖屏之间共享：同一文件即使同时落在两类目录里，也只归入先扫描到的方向。
	seen := make(map[string]struct{})
	portrait, err := scanLocalDirs(local.PortraitDirs, local, basePath, files, seen)
	if err != nil {
		return nil, nil, nil, err
	}
	landscape, err := scanLocalDirs(local.LandscapeDirs, local, basePath, files, seen)
	if err != nil {
		return nil, nil, nil, err
	}
	return portrait, landscape, files, nil
}

func scanLocalDirs(dirs []string, local LocalConfig, basePath string, files map[string]string, seen map[string]struct{}) ([]ImageInfo, error) {
	var out []ImageInfo
	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("解析本地图片目录失败 %s: %w", dir, err)
		}
		stat, err := os.Stat(absDir)
		if err != nil {
			return nil, fmt.Errorf("访问本地图片目录失败 %s: %w", dir, err)
		}
		if !stat.IsDir() {
			return nil, fmt.Errorf("本地图片路径不是目录: %s", dir)
		}

		err = filepath.WalkDir(absDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			// 跳过隐藏文件与隐藏目录（如 .DS_Store、.thumbnails/），不对外暴露。
			// 配置的根目录本身即使以点开头也照常扫描。
			if p != absDir && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !local.allowsExtension(p) {
				return nil
			}

			absPath, err := filepath.Abs(p)
			if err != nil {
				return err
			}
			if _, ok := seen[absPath]; ok {
				return nil
			}
			seen[absPath] = struct{}{}

			publicURL, routePath := localRouteURLs(basePath, absPath)
			info := FetchLocalImageInfo(absPath, publicURL)
			if info.Width <= 0 || info.Height <= 0 {
				return nil
			}
			if routePath != "" {
				files[routePath] = absPath
			}
			out = append(out, info)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("扫描本地图片目录失败 %s: %w", dir, err)
		}
	}
	return out, nil
}

func (l LocalConfig) allowsExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, allowed := range l.Extensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

// localRouteURLs 生成对外可访问地址，以及用于查找文件的内部路由。
func localRouteURLs(basePath, absPath string) (publicURL, routePath string) {
	sum := sha256.Sum256([]byte(absPath))
	id := hex.EncodeToString(sum[:10])
	name := filepath.Base(absPath)
	routePath = "/local/" + id + "/" + name

	prefix := strings.TrimRight(basePath, "/")
	if prefix == "/" {
		prefix = ""
	}
	return prefix + "/local/" + id + "/" + url.PathEscape(name), routePath
}
