package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// scanLocalImages 扫描配置的本地图片目录，返回竖屏/横屏图片列表与静态路由表。
func scanLocalImages(local LocalConfig, basePath string) ([]ImageInfo, []ImageInfo, map[string]string, error) {
	files := make(map[string]string)
	// seen 在横竖屏之间共享：同一文件即使同时落在两类目录里，也只归入先扫描到的方向。
	seen := make(map[string]struct{})
	portrait, err := scanLocalDirs(local.PortraitDirs, local, basePath, files, seen)
	if err != nil {
		return nil, nil, nil, err
	}
	landscape, err := scanLocalDirs(local.LandscapeDirs, local, basePath, files, seen)
	if err != nil {
		return nil, nil, nil, err
	}
	return portrait, landscape, files, nil
}

func scanLocalDirs(dirs []string, local LocalConfig, basePath string, files map[string]string, seen map[string]struct{}) ([]ImageInfo, error) {
	var out []ImageInfo
	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("解析本地图片目录失败 %s: %w", dir, err)
		}
		stat, err := os.Stat(absDir)
		if err != nil {
			return nil, fmt.Errorf("访问本地图片目录失败 %s: %w", dir, err)
		}
		if !stat.IsDir() {
			return nil, fmt.Errorf("本地图片路径不是目录: %s", dir)
		}

		err = filepath.WalkDir(absDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			// 跳过隐藏文件与隐藏目录（如 .DS_Store、.thumbnails/），不对外暴露。
			// 配置的根目录本身即使以点开头也照常扫描。
			if p != absDir && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			// 符号链接一律跳过：WalkDir 不会进入链接目录，但链接文件会被
			// 当作普通文件读取，进而把配置目录之外的内容纳入白名单。
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !local.allowsExtension(p) {
				return nil
			}

			absPath, err := filepath.Abs(p)
			if err != nil {
				return err
			}
			if _, ok := seen[absPath]; ok {
				return nil
			}
			seen[absPath] = struct{}{}

			publicURL, routePath := localRouteURLs(basePath, absPath)
			info := FetchLocalImageInfo(absPath, publicURL)
			if info.Width <= 0 || info.Height <= 0 {
				return nil
			}
			if routePath != "" {
				files[routePath] = absPath
			}
			out = append(out, info)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("扫描本地图片目录失败 %s: %w", dir, err)
		}
	}
	return out, nil
}

func (l LocalConfig) allowsExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, allowed := range l.Extensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

// localRouteURLs 生成对外可访问地址，以及用于查找文件的内部路由。
func localRouteURLs(basePath, absPath string) (publicURL, routePath string) {
	sum := sha256.Sum256([]byte(absPath))
	id := hex.EncodeToString(sum[:10])
	name := filepath.Base(absPath)
	routePath = "/local/" + id + "/" + name

	prefix := strings.TrimRight(basePath, "/")
	if prefix == "/" {
		prefix = ""
	}
	return prefix + "/local/" + id + "/" + url.PathEscape(name), routePath
}
