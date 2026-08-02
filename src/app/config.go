package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	URLs      URLsConfig      `yaml:"urls"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Referer   RefererConfig   `yaml:"referer"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// BasePath URL 路径前缀。部署在反向代理子目录（如 /api）下时，
	// 设为此前缀值即可正常响应，无需反代做路径 rewrite。
	// 默认为空（等同于 /），表示无前缀。
	// 示例：BasePath="/api" → 路由注册为 /api/、/api/health、/api/admin/reload
	BasePath string `yaml:"base_path"`
}

type URLsConfig struct {
	Portrait  string `yaml:"portrait"`
	Landscape string `yaml:"landscape"`
}

type AuthConfig struct {
	Enabled bool     `yaml:"enabled"`
	Tokens  []string `yaml:"tokens"`
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
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 28085,
		},
		URLs: URLsConfig{
			Portrait:  "urls/portrait.txt",
			Landscape: "urls/landscape.txt",
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
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	defaultHeader := []byte("# 默认配置文件，由程序首次启动时自动生成。\n# 请根据需要修改后重新启动。\n\n")
	return os.WriteFile(path, append(defaultHeader, data...), 0o644)
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 必须在 1-65535 之间，当前值: %d", c.Server.Port)
	}
	if c.Server.BasePath != "" {
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
	if c.URLs.Portrait == "" {
		return fmt.Errorf("urls.portrait 不能为空")
	}
	if c.URLs.Landscape == "" {
		return fmt.Errorf("urls.landscape 不能为空")
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
	return nil
}
