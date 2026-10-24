package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeTestPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func setupLocalDirs(t *testing.T) (portraitDir, landscapeDir string) {
	t.Helper()
	root := t.TempDir()
	portraitDir = filepath.Join(root, "portrait")
	landscapeDir = filepath.Join(root, "landscape")

	writeTestPNG(t, filepath.Join(portraitDir, "p1.png"), 100, 200)
	writeTestPNG(t, filepath.Join(portraitDir, "nested", "p2.png"), 100, 200)
	writeTestPNG(t, filepath.Join(landscapeDir, "l1.png"), 300, 100)

	if err := os.WriteFile(filepath.Join(portraitDir, "note.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	return portraitDir, landscapeDir
}

func TestLocalConfigValidate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local.PortraitDirs = []string{"images/portrait"}
	cfg.Local.LandscapeDirs = nil
	cfg.Local.Extensions = []string{"JPG", ".png"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("有效配置校验失败: %v", err)
	}
	if got := cfg.Local.Extensions; got[0] != ".jpg" || got[1] != ".png" {
		t.Fatalf("扩展名未归一化: %v", got)
	}

	cfg = DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local.PortraitDirs = nil
	cfg.Local.LandscapeDirs = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("local 模式未配置目录时应报错")
	}

	cfg = DefaultConfig()
	cfg.Source = SourceURL
	cfg.Local.PortraitDirs = nil
	cfg.Local.LandscapeDirs = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("url 模式不应要求本地目录: %v", err)
	}

	cfg = DefaultConfig()
	cfg.Source = "mixed"
	if err := cfg.Validate(); err == nil {
		t.Fatal("非法 source 应报错")
	}
}

func TestPoolLoadLocalServe(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	local := LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}

	stats, err := pool.Load("", "", "", SourceLocal, local, "/api")
	if err != nil {
		t.Fatalf("加载本地图片失败: %v", err)
	}
	if stats.Portrait != 2 || stats.Landscape != 1 {
		t.Fatalf("数量不符: %+v", stats)
	}
	if stats.LocalPortrait != 2 || stats.LocalLandscape != 1 {
		t.Fatalf("本地数量不符: %+v", stats)
	}

	info, err := pool.Random("portrait")
	if err != nil {
		t.Fatal(err)
	}
	if info.LocalPath == "" {
		t.Fatal("本地图片缺少 LocalPath")
	}
	if !strings.HasPrefix(info.URL, "/api/local/") {
		t.Fatalf("URL 前缀错误: %s", info.URL)
	}
	if info.Width != 100 || info.Height != 200 || info.Format != "png" {
		t.Fatalf("元数据错误: %+v", info)
	}
	if _, ok := pool.LocalFile(strings.TrimPrefix(info.URL, "/api")); !ok {
		t.Fatalf("静态路由未注册: %s", info.URL)
	}
}

func TestPoolLoadURLSource(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)
	root := t.TempDir()
	portraitFile := filepath.Join(root, "portrait.txt")
	if err := os.WriteFile(portraitFile, []byte("https://cdn.example.com/p.jpg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	landscapeFile := filepath.Join(root, "landscape.txt")
	if err := os.WriteFile(landscapeFile, []byte("https://cdn.example.com/l.jpg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	local := LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}

	stats, err := pool.Load(portraitFile, landscapeFile, "", SourceURL, local, "")
	if err != nil {
		t.Fatalf("URL 模式加载失败: %v", err)
	}
	if stats.Portrait != 1 || stats.Landscape != 1 {
		t.Fatalf("URL 数量不符: %+v", stats)
	}
	if stats.LocalPortrait != 0 || stats.LocalLandscape != 0 {
		t.Fatalf("URL 模式不应扫描本地图片: %+v", stats)
	}
	if len(pool.current.Load().LocalFiles) != 0 {
		t.Fatal("URL 模式不应注册本地静态路由")
	}
}

func TestPrefetchCacheExcludesLocal(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)
	cacheDir := t.TempDir()

	pool := NewURLPool()
	pool.SetMetadataMode("prefetch")
	local := LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}
	if _, err := pool.Load("", "", cacheDir, SourceLocal, local, ""); err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	data, err := os.ReadFile(cacheFilePath(cacheDir))
	if err != nil {
		t.Fatalf("读取缓存失败: %v", err)
	}
	if strings.Contains(string(data), "/local/") || strings.Contains(string(data), portraitDir) {
		t.Fatalf("缓存不应包含本地图片: %s", data)
	}
}

func TestHandlerServesLocalImage(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)

	cfg := DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local = LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	if _, err := pool.Load("", "", "", cfg.Source, cfg.Local, ""); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	t.Run("redirect 模式直接返回文件", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil)
		rec := httptest.NewRecorder()
		h.ServeImage(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("状态码 = %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/png") {
			t.Fatalf("Content-Type = %q", ct)
		}
		if rec.Body.Len() == 0 {
			t.Fatal("响应体为空")
		}
	})

	t.Run("条件请求不会返回 304", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil)
		req.Header.Set("If-Modified-Since", time.Now().Add(24*time.Hour).UTC().Format(http.TimeFormat))
		rec := httptest.NewRecorder()
		h.ServeImage(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("状态码 = %d, want 200", rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Fatalf("Cache-Control 未禁止缓存: %q", cc)
		}
		if rec.Body.Len() == 0 {
			t.Fatal("响应体为空")
		}
	})

	t.Run("json 模式在 local 下被拒绝", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait&format=json", nil)
		rec := httptest.NewRecorder()
		h.ServeImage(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("状态码 = %d, want 400", rec.Code)
		}
	})

	t.Run("静态路由仅返回已索引文件", func(t *testing.T) {
		info, err := pool.Random("portrait")
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodGet, info.URL, nil)
		rec := httptest.NewRecorder()
		h.ServeLocal(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("本地路由状态码 = %d, want 200", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/local/../../etc/passwd", nil)
		rec = httptest.NewRecorder()
		h.ServeLocal(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("目录穿越状态码 = %d, want 404", rec.Code)
		}
	})
}

func TestDefaultSourceIsURL(t *testing.T) {
	if got := DefaultConfig().Source; got != SourceURL {
		t.Fatalf("默认 source = %q, want %q", got, SourceURL)
	}

	// 旧配置没有 source 字段时，应保持 URL 列表模式，不能因为缺 local 目录而启动失败。
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	legacy := []byte(`server:
  host: "0.0.0.0"
  port: 28085
  cache_dir: "cache"
  metadata_mode: "on_request"
urls:
  portrait: "urls/portrait.txt"
  landscape: "urls/landscape.txt"
auth:
  enabled: false
`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, created, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("旧配置加载失败: %v", err)
	}
	if created {
		t.Fatal("配置文件已存在，不应触发默认配置生成")
	}
	if cfg.Source != SourceURL {
		t.Fatalf("旧配置 source = %q, want %q", cfg.Source, SourceURL)
	}
}

func TestScanLocalSkipsHiddenFiles(t *testing.T) {
	portraitDir, _ := setupLocalDirs(t)
	writeTestPNG(t, filepath.Join(portraitDir, ".hidden.png"), 10, 10)
	writeTestPNG(t, filepath.Join(portraitDir, ".thumbs", "inner.png"), 10, 10)

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	stats, err := pool.Load("", "", "", SourceLocal, LocalConfig{
		PortraitDirs: []string{portraitDir},
		Extensions:   defaultLocalExtensions(),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalPortrait != 2 {
		t.Fatalf("隐藏文件应被跳过，本地数量 = %d", stats.LocalPortrait)
	}
	for _, info := range pool.current.Load().Portrait {
		if strings.Contains(info.URL, ".hidden") || strings.Contains(info.URL, ".thumbs") {
			t.Fatalf("隐藏文件进了池: %s", info.URL)
		}
		if _, ok := pool.LocalFile(strings.TrimPrefix(info.URL, "")); !ok {
			t.Fatalf("路由未登记: %s", info.URL)
		}
	}
}

func TestScanLocalDedupesAcrossOrientations(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "shared.png"), 20, 10)

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	stats, err := pool.Load("", "", "", SourceLocal, LocalConfig{
		PortraitDirs:  []string{dir},
		LandscapeDirs: []string{dir},
		Extensions:    defaultLocalExtensions(),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalPortrait != 1 || stats.LocalLandscape != 0 {
		t.Fatalf("同一目录重复配置时应只归入一个方向: %+v", stats)
	}
}

func TestServeLocalRejectsSymlinkSwap(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.png")
	writeTestPNG(t, secret, 64, 64)

	cfg := DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local = LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}
	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	if _, err := pool.Load("", "", "", cfg.Source, cfg.Local, ""); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	p1 := filepath.Join(portraitDir, "p1.png")
	_, route := localRouteURLs("", p1)
	serveStatic := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeLocal(rec, httptest.NewRequest(http.MethodGet, route, nil))
		return rec
	}

	// 扫描后覆盖为普通文件：应继续可服务（不把文件身份钉死在扫描时刻）。
	writeTestPNG(t, p1, 280, 640)
	rec := serveStatic()
	if rec.Code != http.StatusOK {
		t.Fatalf("覆盖普通文件后静态路由 = %d, want 200", rec.Code)
	}
	if cfgImg, format, err := image.DecodeConfig(bytes.NewReader(rec.Body.Bytes())); err != nil ||
		format != "png" || cfgImg.Width != 280 || cfgImg.Height != 640 {
		t.Fatalf("覆盖后返回内容不符: %v %s %dx%d", err, format, cfgImg.Width, cfgImg.Height)
	}

	// 扫描后替换为指向目录外的符号链接：静态路由必须 404，不能返回链接目标。
	if err := os.Remove(p1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, p1); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
	if rec := serveStatic(); rec.Code != http.StatusNotFound {
		t.Fatalf("符号链接文件静态路由 = %d, want 404", rec.Code)
	}

	// 随机接口应跳过坏文件，返回剩余的正常图片，且绝不能返回 secret(64x64)。
	for i := 0; i < 15; i++ {
		rec := httptest.NewRecorder()
		h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("仍有可用图时应 200，实际 %d", rec.Code)
		}
		cfgImg, format, err := image.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
		if err != nil || format != "png" {
			t.Fatalf("响应不是 PNG: %v %q", err, format)
		}
		if cfgImg.Width == 64 && cfgImg.Height == 64 {
			t.Fatal("随机接口返回了符号链接指向的 secret.png")
		}
	}

	// 剩余文件也被替换：返回 503 而不是读取链接目标。
	p2 := filepath.Join(portraitDir, "nested", "p2.png")
	if err := os.Remove(p2); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, p2); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("全部被替换为符号链接时 = %d, want 503", rec.Code)
	}
}

func TestPoolConcurrentLoadNoRace(t *testing.T) {
	portraitDir, _ := setupLocalDirs(t)
	cacheDir := filepath.Join(t.TempDir(), "cache")

	pool := NewURLPool()
	pool.SetMetadataMode("prefetch")
	local := LocalConfig{
		PortraitDirs: []string{portraitDir},
		Extensions:   defaultLocalExtensions(),
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := pool.Load("", "", cacheDir, SourceLocal, local, ""); err != nil {
				t.Errorf("并发 Load 失败: %v", err)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(filepath.Join(cacheDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("并发 Load 后缓存损坏: %v", err)
	}
}

func TestScanLocalSkipsSymlinks(t *testing.T) {
	portraitDir, _ := setupLocalDirs(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "outside.png")
	writeTestPNG(t, target, 10, 10)

	if err := os.Symlink(target, filepath.Join(portraitDir, "sneaky.png")); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(portraitDir, "sneaky-dir")); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	stats, err := pool.Load("", "", "", SourceLocal, LocalConfig{
		PortraitDirs: []string{portraitDir},
		Extensions:   defaultLocalExtensions(),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalPortrait != 2 {
		t.Fatalf("符号链接应被跳过，本地数量 = %d, want 2", stats.LocalPortrait)
	}

	snapshot := pool.current.Load()
	for _, info := range snapshot.Portrait {
		if strings.Contains(info.LocalPath, "sneaky") {
			t.Fatalf("符号链接进了池: %s", info.LocalPath)
		}
	}
	if len(snapshot.LocalFiles) != 2 {
		t.Fatalf("静态路由白名单 = %d 项, want 2", len(snapshot.LocalFiles))
	}
	for route := range snapshot.LocalFiles {
		if strings.Contains(route, "sneaky") {
			t.Fatalf("符号链接被登记为静态路由: %s", route)
		}
	}
}

func TestHandlerLocalMissingFileRetriesDeterministic(t *testing.T) {
	root := t.TempDir()
	portraitDir := filepath.Join(root, "portrait")
	names := []string{"a", "b", "c", "d"}
	for _, name := range names {
		writeTestPNG(t, filepath.Join(portraitDir, name+".png"), 10, 20)
	}

	cfg := DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local = LocalConfig{
		PortraitDirs: []string{portraitDir},
		Extensions:   defaultLocalExtensions(),
	}

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	if _, err := pool.Load("", "", "", cfg.Source, cfg.Local, ""); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	// 删除两张，剩余候选仍多于重试次数：不放回抽样应保证每次都能命中可用文件。
	for _, name := range names[:2] {
		if err := os.Remove(filepath.Join(portraitDir, name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil)
		rec := httptest.NewRecorder()
		h.ServeImage(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求存在可用图却返回 %d", i+1, rec.Code)
		}
	}
}

func TestHandlerLocalMissingFile(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)

	cfg := DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local = LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	if _, err := pool.Load("", "", "", cfg.Source, cfg.Local, ""); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	// 扫描后文件被删除：应重选，仍有可用图时照常返回。
	if err := os.Remove(filepath.Join(portraitDir, "p1.png")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil)
		rec := httptest.NewRecorder()
		h.ServeImage(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("存在可用图时应返回 200，实际 %d", rec.Code)
		}
	}

	// 全部文件消失：返回 503 而不是 500，且提示检查目录。
	if err := os.Remove(filepath.Join(portraitDir, "nested", "p2.png")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil)
	rec := httptest.NewRecorder()
	h.ServeImage(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("本地文件全部缺失时状态码 = %d, want 503", rec.Code)
	}
}

func TestHandlerLocalRejectsLinkFormats(t *testing.T) {
	portraitDir, landscapeDir := setupLocalDirs(t)

	cfg := DefaultConfig()
	cfg.Source = SourceLocal
	cfg.Local = LocalConfig{
		PortraitDirs:  []string{portraitDir},
		LandscapeDirs: []string{landscapeDir},
		Extensions:    defaultLocalExtensions(),
	}

	pool := NewURLPool()
	pool.SetMetadataMode("on_request")
	if _, err := pool.Load("", "", "", cfg.Source, cfg.Local, ""); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	// local 模式没有可返回的外部地址，json/text 一律拒绝。
	for _, format := range []string{"json", "text"} {
		req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait&format="+format, nil)
		rec := httptest.NewRecorder()
		h.ServeImage(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("format=%s 应返回 400，实际 %d", format, rec.Code)
		}
		if body := rec.Body.String(); !strings.Contains(body, "only supports format=redirect") {
			t.Fatalf("format=%s 提示信息不符: %s", format, body)
		}
	}

	// redirect 模式不受影响，仍然直接返回文件。
	req := httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil)
	rec := httptest.NewRecorder()
	h.ServeImage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("redirect 模式状态码 = %d, want 200", rec.Code)
	}
}
