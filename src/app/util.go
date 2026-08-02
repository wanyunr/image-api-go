package app

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// getClientIP 获取客户端真实 IP
func getClientIP(r *http.Request) string {
	// 优先从 X-Forwarded-For 获取
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// 直接取 RemoteAddr
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

// extractHost 从 URL 中提取 host
func extractHost(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "://"); idx != -1 {
		s = s[idx+3:]
	}
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.LastIndex(s, ":"); idx != -1 {
		s = s[:idx]
	}
	return strings.ToLower(s)
}

// matchReferer 匹配 Referer host 与白名单模式
func matchReferer(host, pattern string) bool {
	pattern = strings.ToLower(pattern)

	// 通配符匹配 *.example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix) || host == pattern[2:]
	}

	return host == pattern
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
