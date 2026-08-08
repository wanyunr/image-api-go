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
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
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
