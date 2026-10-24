package app

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// loadOrCreate 读取 URL 列表文件；文件不存在时生成带注释的默认文件再读取。
// 返回值中的 bool 表示是否触发了默认文件创建。
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

// loadURLFile 逐行读取 URL 列表：忽略空行、注释行（# 开头）与非法 URL。
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

// dedupe 按出现顺序去重。
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

func makeInfos(urls []string) []ImageInfo {
	infos := make([]ImageInfo, len(urls))
	for i, u := range urls {
		infos[i] = ImageInfo{URL: u}
	}
	return infos
}
