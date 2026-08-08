package app

import (
	"bufio"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrPoolEmpty = errors.New("URL pool is empty")

const (
	metadataFetchConcurrency = 8
	metadataWorkerInterval   = 30 * time.Second
	metadataMaxAttempts      = 3
)

type pendingItem struct {
	url         string
	orientation string
	attempts    int
}

type URLPool struct {
	mu           sync.RWMutex
	portrait     []ImageInfo
	landscape    []ImageInfo
	retry        []pendingItem
	dead         []DeadItem
	cacheDir     string
	metadataMode string
	rng          *rand.Rand
}

type LoadStats struct {
	CreatedFiles bool
	Portrait     int
	Landscape    int
	Pending      int
	Dead         int
}

func NewURLPool() *URLPool {
	return &URLPool{rng: newRandGenerator(), metadataMode: "prefetch"}
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

// Load 加载 URL 列表。缓存命中带 metadata 入池，全部进 retry 后台补抓。
func (p *URLPool) Load(portraitFile, landscapeFile, cacheDir string) (LoadStats, error) {
	portraitURLs, createdPortrait, err := loadOrCreate(portraitFile, "# portrait URL list\n# Add one image URL per line\n", "竖屏")
	if err != nil {
		return LoadStats{}, err
	}
	landscapeURLs, createdLandscape, err := loadOrCreate(landscapeFile, "# landscape URL list\n# Add one image URL per line\n", "横屏")
	if err != nil {
		return LoadStats{}, err
	}

	portraitURLs = dedupe(portraitURLs)
	landscapeURLs = dedupe(landscapeURLs)

	p.cacheDir = cacheDir
	if p.metadataMode == "on_request" {
		return p.loadOnRequest(portraitURLs, landscapeURLs, createdPortrait || createdLandscape)
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

	portrait, retryP, deadP := p.buildPoolWithCache(portraitURLs, cacheP, "portrait")
	landscape, retryL, deadL := p.buildPoolWithCache(landscapeURLs, cacheL, "landscape")

	p.mu.Lock()
	p.portrait = portrait
	p.landscape = landscape
	p.retry = append(retryP, retryL...)
	p.dead = append(deadP, deadL...)
	p.mu.Unlock()

	if err := p.saveCache(); err != nil {
		return LoadStats{}, err
	}

	return LoadStats{
		CreatedFiles: createdPortrait || createdLandscape,
		Portrait:     len(portrait),
		Landscape:    len(landscape),
		Pending:      len(retryP) + len(retryL),
		Dead:         len(deadP) + len(deadL),
	}, nil
}

func (p *URLPool) RetryPending() {
	p.retryPending()
}

func (p *URLPool) buildPoolWithCache(urls []string, cacheMap map[string]ImageInfo, orientation string) ([]ImageInfo, []pendingItem, []DeadItem) {
	pool := make([]ImageInfo, 0, len(urls))
	var retry []pendingItem
	for _, u := range urls {
		if info, ok := cacheMap[u]; ok {
			pool = append(pool, info)
		} else {
			pool = append(pool, ImageInfo{URL: u})
		}
		retry = append(retry, pendingItem{url: u, orientation: orientation})
	}
	return pool, retry, nil
}

func (p *URLPool) loadOnRequest(portraitURLs, landscapeURLs []string, createdFiles bool) (LoadStats, error) {
	portrait := make([]ImageInfo, len(portraitURLs))
	for i, u := range portraitURLs {
		portrait[i] = ImageInfo{URL: u}
	}
	landscape := make([]ImageInfo, len(landscapeURLs))
	for i, u := range landscapeURLs {
		landscape[i] = ImageInfo{URL: u}
	}

	p.mu.Lock()
	p.portrait = portrait
	p.landscape = landscape
	p.retry = nil
	p.dead = nil
	p.mu.Unlock()

	return LoadStats{
		CreatedFiles: createdFiles,
		Portrait:     len(portrait),
		Landscape:    len(landscape),
	}, nil
}

// MetadataWorker 后台周期补抓 metadata 缺失的 URL。
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
	items := p.retry
	p.retry = nil
	p.mu.Unlock()
	if len(items) == 0 {
		return
	}

	sem := make(chan struct{}, metadataFetchConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	type readyItem struct {
		info        ImageInfo
		orientation string
	}
	var added []readyItem
	var still []pendingItem
	var newlyDead []DeadItem

	for _, it := range items {
		wg.Add(1)
		go func(it pendingItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info := FetchImageInfo(it.url)
			mu.Lock()
			defer mu.Unlock()
			if info.Width > 0 && info.Height > 0 {
				added = append(added, readyItem{info: info, orientation: it.orientation})
				return
			}
			it.attempts++
			if it.attempts >= metadataMaxAttempts {
				newlyDead = append(newlyDead, DeadItem{
					URL:         it.url,
					Orientation: it.orientation,
					Attempts:    it.attempts,
				})
			} else {
				still = append(still, it)
			}
		}(it)
	}
	wg.Wait()

	p.mu.Lock()
	for _, a := range added {
		p.updatePoolEntry(a.info, a.orientation)
	}
	p.retry = append(p.retry, still...)
	p.dead = append(p.dead, newlyDead...)
	p.mu.Unlock()

	if len(added) > 0 || len(newlyDead) > 0 {
		logStructured("metadata_retry", map[string]any{"added": len(added), "dead": len(newlyDead), "remaining": len(still)})
		_ = p.saveCache()
	}
}

func (p *URLPool) updatePoolEntry(info ImageInfo, orientation string) {
	var pool *[]ImageInfo
	if orientation == "portrait" {
		pool = &p.portrait
	} else {
		pool = &p.landscape
	}
	for i := range *pool {
		if (*pool)[i].URL == info.URL {
			(*pool)[i] = info
			return
		}
	}
}

func (p *URLPool) Random(orientation string) (ImageInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var pool []ImageInfo
	switch orientation {
	case "portrait":
		pool = p.portrait
	case "landscape":
		pool = p.landscape
	default:
		return ImageInfo{}, fmt.Errorf("无效的 orientation: %s", orientation)
	}

	if len(pool) == 0 {
		return ImageInfo{}, fmt.Errorf("%w: %s", ErrPoolEmpty, orientation)
	}

	if p.rng == nil {
		p.rng = newRandGenerator()
	}
	return pool[p.rng.Intn(len(pool))], nil
}

func (p *URLPool) Count() (portrait, landscape int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.portrait), len(p.landscape)
}

func (p *URLPool) Pending() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.retry)
}

func (p *URLPool) Dead() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.dead)
}

func (p *URLPool) ClearDead() {
	p.mu.Lock()
	p.dead = nil
	p.mu.Unlock()
	_ = p.saveCache()
}

func (p *URLPool) saveCache() error {
	if p.metadataMode != "prefetch" || p.cacheDir == "" {
		return nil
	}
	p.mu.RLock()
	cache := &Cache{
		Portrait:  append([]ImageInfo(nil), p.portrait...),
		Landscape: append([]ImageInfo(nil), p.landscape...),
		Dead:      append([]DeadItem(nil), p.dead...),
	}
	p.mu.RUnlock()
	return SaveCache(p.cacheDir, cache)
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
