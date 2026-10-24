package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const cacheVersion = 1

type Cache struct {
	Version   int         `json:"version"`
	UpdatedAt time.Time   `json:"updated_at"`
	Portrait  []ImageInfo `json:"portrait"`
	Landscape []ImageInfo `json:"landscape"`
	Dead      []DeadItem  `json:"dead"`
}

type DeadItem struct {
	URL         string `json:"url"`
	Orientation string `json:"orientation"`
	Attempts    int    `json:"attempts"`
}

func cacheFilePath(cacheDir string) string {
	return filepath.Join(cacheDir, "metadata.json")
}

func LoadCache(cacheDir string) (*Cache, error) {
	path := cacheFilePath(cacheDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Cache{Version: cacheVersion}, nil
		}
		return nil, fmt.Errorf("读取缓存失败: %w", err)
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("解析缓存失败: %w", err)
	}
	if c.Version != cacheVersion {
		return &Cache{Version: cacheVersion}, nil
	}
	return &c, nil
}

func SaveCache(cacheDir string, c *Cache) error {
	path := cacheFilePath(cacheDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	c.Version = cacheVersion
	c.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化缓存失败: %w", err)
	}
	data = append(data, '\n')
	// 先写临时文件再原子替换，避免并发/崩溃留下半截缓存；缓存可能含带签名参数的 URL，权限收紧。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入缓存失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("替换缓存失败: %w", err)
	}
	return nil
}
