package app

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// getClientIP 获取客户端 IP（仅用于日志展示，与限流无关——限流为全局额度）。
// 优先取 X-Forwarded-For / X-Real-IP（存在时），否则回退 TCP 直连来源。
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// DetectDeviceType 根据 User-Agent 判断设备类型
func DetectDeviceType(ua string) string {
	ua = strings.ToLower(ua)

	switch {
	case strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet"):
		return "tablet"
	case strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone"):
		return "mobile"
	default:
		return "desktop"
	}
}

// filenameFromURL 从 URL 中提取文件名
func filenameFromURL(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "?"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		s = s[idx+1:]
	}
	if name, err := url.PathUnescape(s); err == nil {
		return name
	}
	return s
}

// formatDuration 格式化运行时间为可读字符串
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
