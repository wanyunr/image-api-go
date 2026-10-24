package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDefaults(t *testing.T) {
	cfg := DefaultConfig()
	// 模拟旧配置缺省字段：仅保留必要项。
	cfg.Source = ""
	cfg.Server.MetadataMode = ""
	cfg.Local.Extensions = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("缺省字段应补默认值而非报错: %v", err)
	}
	if cfg.Source != SourceURL {
		t.Fatalf("source 默认值 = %q, want url", cfg.Source)
	}
	if cfg.Server.MetadataMode != MetadataModeOnRequest {
		t.Fatalf("metadata_mode 默认值 = %q, want on_request", cfg.Server.MetadataMode)
	}
	if got := cfg.Local.Extensions; len(got) == 0 || got[0] != ".jpg" {
		t.Fatalf("extensions 未补默认列表: %v", got)
	}
	if cfg.PrefetchEnabled() {
		t.Fatal("默认应为 on_request，非 prefetch")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
		want string // 期望错误信息包含的关键字
	}{
		{"非法 source", func(c *Config) { c.Source = "mixed" }, "source"},
		{"端口越界", func(c *Config) { c.Server.Port = 70000 }, "port"},
		{"非法 metadata_mode", func(c *Config) { c.Server.MetadataMode = "sometimes" }, "metadata_mode"},
		{"prefetch 缺 cache_dir", func(c *Config) {
			c.Server.MetadataMode = MetadataModePrefetch
			c.Server.CacheDir = ""
		}, "cache_dir"},
		{"base_path 无前导斜杠", func(c *Config) { c.Server.BasePath = "api" }, "base_path"},
		{"base_path 尾部斜杠", func(c *Config) { c.Server.BasePath = "/api/" }, "base_path"},
		{"base_path 含特殊字符", func(c *Config) { c.Server.BasePath = "/a?b" }, "base_path"},
		{"url 模式缺 portrait 文件", func(c *Config) { c.URLs.Portrait = "" }, "portrait"},
		{"url 模式缺 landscape 文件", func(c *Config) { c.URLs.Landscape = "" }, "landscape"},
		{"auth 启用但无 tokens", func(c *Config) { c.Auth.Enabled = true; c.Auth.Tokens = nil }, "tokens"},
		{"限流启用但无额度", func(c *Config) {
			c.RateLimit.Enabled = true
			c.RateLimit.PerMinute, c.RateLimit.PerHour, c.RateLimit.PerDay = 0, 0, 0
		}, "rate_limit"},
		{"防盗链启用但无白名单", func(c *Config) { c.Referer.Enabled = true; c.Referer.Whitelist = nil }, "whitelist"},
		{"local 模式无目录", func(c *Config) {
			c.Source = SourceLocal
			c.Local.PortraitDirs, c.Local.LandscapeDirs = nil, nil
		}, "portrait_dirs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := DefaultConfig()
			c.mut(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("应返回校验错误")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("错误信息 %q 未包含关键字 %q", err.Error(), c.want)
			}
		})
	}
}

func TestValidateBasePathAcceptsRoot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.BasePath = "/"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("base_path=/ 应视为合法: %v", err)
	}
}

func TestLoadConfigCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, created, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("首次加载应生成默认配置: %v", err)
	}
	if !created {
		t.Fatal("应报告已创建默认配置")
	}
	if cfg.Source != SourceURL {
		t.Fatalf("默认 source = %q, want url", cfg.Source)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("默认配置文件未落盘: %v", err)
	}

	// 二次加载不再生成。
	_, created, err = LoadConfig(path)
	if err != nil || created {
		t.Fatalf("二次加载 created=%v err=%v", created, err)
	}
}

func TestLoadConfigRejectsBrokenYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server: [broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadConfig(path); err == nil {
		t.Fatal("损坏的 YAML 应报错")
	}
}
