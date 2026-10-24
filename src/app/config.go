package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// source 取值。
const (
	SourceLocal = "local"
	SourceURL   = "url"
)

// server.metadata_mode 取值。
const (
	MetadataModePrefetch  = "prefetch"
	MetadataModeOnRequest = "on_request"
)

type Config struct {
	Source    string          `yaml:"source"`
	Server    ServerConfig    `yaml:"server"`
	URLs      URLsConfig      `yaml:"urls"`
	Local     LocalConfig     `yaml:"local"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Referer   RefererConfig   `yaml:"referer"`
}

type ServerConfig struct {
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	CacheDir         string `yaml:"cache_dir"`
	MetadataMode     string `yaml:"metadata_mode"`
	BasePath         string `yaml:"base_path"`
	RecentBufferSize int    `yaml:"recent_buffer_size,omitempty"`
}

type URLsConfig struct {
	Portrait  string `yaml:"portrait"`
	Landscape string `yaml:"landscape"`
}

type LocalConfig struct {
	PortraitDirs  []string `yaml:"portrait_dirs"`
	LandscapeDirs []string `yaml:"landscape_dirs"`
	Extensions    []string `yaml:"extensions"`
}

func (c *Config) UsesLocal() bool {
	return c.Source != SourceURL
}

func (c *Config) UsesURL() bool {
	return c.Source == SourceURL
}

// PrefetchEnabled 表示 metadata 预取模式是否开启。
// Validate 会把空 metadata_mode 规范化为 on_request，经校验的配置可安全使用。
func (c *Config) PrefetchEnabled() bool {
	return c.Server.MetadataMode == MetadataModePrefetch
}

type AuthConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Tokens     []string `yaml:"tokens"`
	AdminToken string   `yaml:"admin_token"`
}

type RateLimitConfig struct {
	Enabled   bool `yaml:"enabled"`
	PerMinute int  `yaml:"per_minute"`
	PerHour   int  `yaml:"per_hour"`
	PerDay    int  `yaml:"per_day"`
}

type RefererConfig struct {
	Enabled    bool     `yaml:"enabled"`
	AllowEmpty bool     `yaml:"allow_empty"`
	Whitelist  []string `yaml:"whitelist"`
}

func LoadConfig(path string) (*Config, bool, error) {
	data, err := os.ReadFile(path)
	created := false
	if err != nil {
		if os.IsNotExist(err) {
			if err := createDefaultConfig(path); err != nil {
				return nil, false, fmt.Errorf("生成默认配置失败: %w", err)
			}
			created = true
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, false, fmt.Errorf("读取默认配置失败: %w", err)
			}
		} else {
			return nil, false, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, false, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, false, fmt.Errorf("配置校验失败: %w", err)
	}

	return &cfg, created, nil
}

func DefaultConfig() *Config {
	return &Config{
		Source: SourceURL,
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         28085,
			CacheDir:     "cache",
			MetadataMode: MetadataModeOnRequest,
		},
		URLs: URLsConfig{
			Portrait:  "urls/portrait.txt",
			Landscape: "urls/landscape.txt",
		},
		Local: LocalConfig{
			Extensions:    defaultLocalExtensions(),
			PortraitDirs:  []string{"images/portrait"},
			LandscapeDirs: []string{"images/landscape"},
		},
		Auth: AuthConfig{
			Enabled: false,
			Tokens:  []string{"your_secret_token"},
		},
		RateLimit: RateLimitConfig{
			Enabled:   false,
			PerMinute: 30,
			PerHour:   120,
			PerDay:    300,
		},
		Referer: RefererConfig{
			Enabled:    false,
			AllowEmpty: true,
			Whitelist:  []string{"example.com", "*.example.com"},
		},
	}
}

func createDefaultConfig(path string) error {
	cfg := DefaultConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if cfg.UsesLocal() {
		dirs := append(append([]string{}, cfg.Local.PortraitDirs...), cfg.Local.LandscapeDirs...)
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	defaultHeader := []byte("# 默认配置文件，由程序首次启动时自动生成。\n# 请根据需要修改后重新启动。\n\n")
	return os.WriteFile(path, append(defaultHeader, data...), 0o644)
}

// Validate 先补齐默认值再逐项校验，任何一项不合法即返回错误。
func (c *Config) Validate() error {
	c.applyDefaults()
	return c.check()
}

// applyDefaults 就地补齐缺省字段，保证校验与后续使用看到的是规范化配置。
func (c *Config) applyDefaults() {
	if c.Source == "" {
		// 兼容旧配置：未声明 source 时保持 URL 列表模式，与升级前行为一致。
		c.Source = SourceURL
	}
	if c.Server.MetadataMode == "" {
		c.Server.MetadataMode = MetadataModeOnRequest
	}
	c.Local.normalizeExtensions()
}

func (c *Config) check() error {
	if c.Source != SourceLocal && c.Source != SourceURL {
		return fmt.Errorf("source 必须是 local 或 url，当前值: %q", c.Source)
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 必须在 1-65535 之间，当前值: %d", c.Server.Port)
	}
	if c.Server.MetadataMode != MetadataModePrefetch && c.Server.MetadataMode != MetadataModeOnRequest {
		return fmt.Errorf("server.metadata_mode 必须是 prefetch 或 on_request，当前值: %q", c.Server.MetadataMode)
	}
	if c.Server.MetadataMode == MetadataModePrefetch && c.Server.CacheDir == "" {
		return fmt.Errorf("prefetch 模式下 server.cache_dir 不能为空")
	}
	if c.Server.BasePath != "" && c.Server.BasePath != "/" {
		if !strings.HasPrefix(c.Server.BasePath, "/") {
			return fmt.Errorf("server.base_path 必须以 / 开头，当前值: %q", c.Server.BasePath)
		}
		if strings.HasSuffix(c.Server.BasePath, "/") {
			return fmt.Errorf("server.base_path 不应以 / 结尾，当前值: %q", c.Server.BasePath)
		}
		if strings.ContainsAny(c.Server.BasePath, "?#") {
			return fmt.Errorf("server.base_path 不能包含特殊字符，当前值: %q", c.Server.BasePath)
		}
	}
	if c.UsesURL() {
		if c.URLs.Portrait == "" {
			return fmt.Errorf("source 为 url 时，urls.portrait 不能为空")
		}
		if c.URLs.Landscape == "" {
			return fmt.Errorf("source 为 url 时，urls.landscape 不能为空")
		}
	}
	if c.Auth.Enabled && len(c.Auth.Tokens) == 0 {
		return fmt.Errorf("auth.enabled 为 true 时，auth.tokens 不能为空")
	}
	if c.RateLimit.Enabled {
		if c.RateLimit.PerMinute <= 0 && c.RateLimit.PerHour <= 0 && c.RateLimit.PerDay <= 0 {
			return fmt.Errorf("rate_limit.enabled 为 true 时，至少需要设置一个限制值 > 0")
		}
	}
	if c.Referer.Enabled && len(c.Referer.Whitelist) == 0 {
		return fmt.Errorf("referer.enabled 为 true 时，referer.whitelist 不能为空")
	}
	if err := c.Local.validate(c.Source); err != nil {
		return err
	}
	return nil
}

func defaultLocalExtensions() []string {
	return []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}
}

// normalizeExtensions 扩展名缺省时补默认列表，否则统一为小写并补齐前导点。
func (l *LocalConfig) normalizeExtensions() {
	if len(l.Extensions) == 0 {
		l.Extensions = defaultLocalExtensions()
		return
	}
	for i, ext := range l.Extensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext != "" && !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		l.Extensions[i] = ext
	}
}

func (l *LocalConfig) validate(source string) error {
	if source == SourceLocal && len(l.PortraitDirs) == 0 && len(l.LandscapeDirs) == 0 {
		return fmt.Errorf("source 为 local 时，至少需要配置一个 local.portrait_dirs 或 local.landscape_dirs")
	}
	return nil
}
