package app

import (
	"fmt"
	"os"

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

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 必须在 1-65535 之间，当前值: %d", c.Server.Port)
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
