package app

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrPoolEmpty 表示 URL 池为空，无可用图片
var ErrPoolEmpty = errors.New("URL pool is empty")

type URLPool struct {
	mu        sync.RWMutex
	portrait  []string
	landscape []string
}

func NewURLPool() *URLPool {
	return &URLPool{}
}

func (p *URLPool) Load(portraitFile, landscapeFile string) (bool, error) {
	portrait, createdPortrait, err := loadOrCreate(portraitFile, "# portrait URL list\n# Add one image URL per line\n", "竖屏")
	if err != nil {
		return false, err
	}
	landscape, createdLandscape, err := loadOrCreate(landscapeFile, "# landscape URL list\n# Add one image URL per line\n", "横屏")
	if err != nil {
		return false, err
	}

	p.mu.Lock()
	p.portrait = portrait
	p.landscape = landscape
	p.mu.Unlock()

	return createdPortrait || createdLandscape, nil
}

// loadOrCreate 加载 URL 列表文件；文件不存在时创建默认文件后重读。
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
		return "", fmt.Errorf("%w: %s", ErrPoolEmpty, orientation)
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
