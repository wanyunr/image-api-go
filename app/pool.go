package app

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"sync"
)

type URLPool struct {
	mu        sync.RWMutex
	portrait  []string
	landscape []string
}

func NewURLPool() *URLPool {
	return &URLPool{}
}

func (p *URLPool) Load(portraitFile, landscapeFile string) error {
	portrait, err := loadURLFile(portraitFile)
	if err != nil {
		return fmt.Errorf("加载竖屏 URL 列表失败: %w", err)
	}

	landscape, err := loadURLFile(landscapeFile)
	if err != nil {
		return fmt.Errorf("加载横屏 URL 列表失败: %w", err)
	}

	p.mu.Lock()
	p.portrait = portrait
	p.landscape = landscape
	p.mu.Unlock()

	return nil
}

func (p *URLPool) Random(orientation string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var pool []string
	switch orientation {
	case "portrait":
		pool = p.portrait
	case "landscape":
		pool = p.landscape
	default:
		return "", fmt.Errorf("无效的 orientation: %s", orientation)
	}

	if len(pool) == 0 {
		return "", fmt.Errorf("URL 池为空: %s", orientation)
	}

	return pool[rand.Intn(len(pool))], nil
}

func (p *URLPool) Count() (portrait, landscape int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.portrait), len(p.landscape)
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

		// 忽略空行
		if line == "" {
			continue
		}
		// 忽略 # 开头的注释
		if strings.HasPrefix(line, "#") {
			continue
		}
		// URL 格式检查
		if !isValidURL(line) {
			continue
		}
		urls = append(urls, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return urls, nil
}

func isValidURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
