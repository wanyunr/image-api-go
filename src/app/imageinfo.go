package app

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"time"

	_ "golang.org/x/image/webp" // 注册 WebP 解码器（R2/CDN 上的 .webp 图片）
)

// ImageInfo 图片元数据
type ImageInfo struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
}

// imageClient 全局共享 HTTP 客户端（连接复用，避免每次 JSON 请求重建 TCP 握手）
var imageClient = &http.Client{Timeout: 10 * time.Second}

// FetchImageInfo 请求时探测图片尺寸和格式
func FetchImageInfo(imgURL string) ImageInfo {
	info := ImageInfo{URL: imgURL}

	req, err := http.NewRequest(http.MethodGet, imgURL, nil)
	if err != nil {
		log.Printf("[探测失败] %s: %v", imgURL, err)
		return info
	}
	// 只请求前 64KB，足够解析图片头部
	req.Header.Set("Range", "bytes=0-65535")

	resp, err := imageClient.Do(req)
	if err != nil {
		log.Printf("[探测失败] %s: %v", imgURL, err)
		return info
	}
	defer resp.Body.Close()

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
