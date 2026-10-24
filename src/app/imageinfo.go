package app

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

// ImageInfo 图片元数据
type ImageInfo struct {
	URL       string `json:"url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Format    string `json:"format"`
	LocalPath string `json:"-"`
}

var imageClient = &http.Client{Timeout: 5 * time.Second}

// FetchImageInfo 探测图片尺寸和格式。URL 中的 # 会自动转义为 %23。
func FetchImageInfo(imgURL string) ImageInfo {
	info := ImageInfo{URL: imgURL}
	fetchURL := strings.ReplaceAll(imgURL, "#", "%23")

	resp, err := imageClient.Get(fetchURL)
	if err != nil {
		log.Printf("[探测失败] %s: %v", imgURL, err)
		return info
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[探测失败] %s: HTTP %d", imgURL, resp.StatusCode)
		return info
	}

	cfg, format, err := image.DecodeConfig(resp.Body)
	if err != nil {
		log.Printf("[解析失败] %s: %v", imgURL, err)
		return info
	}

	info.Width = cfg.Width
	info.Height = cfg.Height
	info.Format = format
	return info
}

// FetchLocalImageInfo 直接从磁盘读取本地图片的尺寸和格式。
func FetchLocalImageInfo(path, publicURL string) ImageInfo {
	info := ImageInfo{URL: publicURL, LocalPath: path}
	f, err := openLocalFile(path)
	if err != nil {
		log.Printf("[本地探测失败] %s: %v", path, err)
		return info
	}
	defer f.Close()

	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		log.Printf("[本地解析失败] %s: %v", path, err)
		return info
	}

	info.Width = cfg.Width
	info.Height = cfg.Height
	info.Format = format
	return info
}
