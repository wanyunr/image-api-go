package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &Cache{
		Portrait:  []ImageInfo{{URL: "https://a.com/p.jpg", Width: 100, Height: 200, Format: "jpeg"}},
		Landscape: []ImageInfo{{URL: "https://a.com/l.jpg", Width: 300, Height: 100, Format: "png"}},
		Dead:      []DeadItem{{URL: "https://a.com/dead.jpg", Orientation: "portrait", Attempts: 3}},
	}
	if err := SaveCache(dir, want); err != nil {
		t.Fatalf("保存缓存失败: %v", err)
	}

	got, err := LoadCache(dir)
	if err != nil {
		t.Fatalf("读取缓存失败: %v", err)
	}
	if len(got.Portrait) != 1 || got.Portrait[0].URL != want.Portrait[0].URL ||
		got.Portrait[0].Width != 100 || got.Portrait[0].Height != 200 {
		t.Fatalf("portrait 往返不符: %+v", got.Portrait)
	}
	if len(got.Landscape) != 1 || got.Landscape[0].URL != want.Landscape[0].URL {
		t.Fatalf("landscape 往返不符: %+v", got.Landscape)
	}
	if len(got.Dead) != 1 || got.Dead[0].URL != want.Dead[0].URL || got.Dead[0].Attempts != 3 {
		t.Fatalf("dead 往返不符: %+v", got.Dead)
	}
	if got.UpdatedAt.IsZero() || time.Since(got.UpdatedAt) > time.Minute {
		t.Fatalf("updated_at 异常: %v", got.UpdatedAt)
	}
}

func TestLoadCacheMissingDir(t *testing.T) {
	cache, err := LoadCache(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("缓存不存在应返回空缓存而非报错: %v", err)
	}
	if cache.Version != cacheVersion || len(cache.Portrait) != 0 {
		t.Fatalf("空缓存不符: %+v", cache)
	}
}

func TestLoadCacheVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := SaveCache(dir, &Cache{Portrait: []ImageInfo{{URL: "https://a.com/p.jpg"}}}); err != nil {
		t.Fatal(err)
	}
	// 把版本号改掉，模拟旧格式缓存。
	path := cacheFilePath(dir)
	overwritten := []byte(`{"version":999,"updated_at":"2026-01-01T00:00:00Z","portrait":[{"url":"https://a.com/p.jpg"}]}`)
	if err := os.WriteFile(path, overwritten, 0o600); err != nil {
		t.Fatal(err)
	}

	cache, err := LoadCache(dir)
	if err != nil {
		t.Fatalf("版本不匹配应静默丢弃而非报错: %v", err)
	}
	if cache.Version != cacheVersion || len(cache.Portrait) != 0 {
		t.Fatalf("版本不匹配应返回空缓存: %+v", cache)
	}
}

func TestLoadCacheCorrupted(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFilePath(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCache(dir); err == nil {
		t.Fatal("损坏缓存应报错，由调用方决定是否重建")
	}
}
