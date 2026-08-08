package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
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

// extractHost 从 URL 中提取 host（去掉端口与路径，正确处理 IPv6 与用户信息）
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
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

// tokenQueryRe 匹配 query 串中的 token 参数（大小写不敏感，含 & 连接符场景），用于日志脱敏
var tokenQueryRe = regexp.MustCompile(`(?i)(^|&)token=[^&]*`)

// logStructured 输出 JSON 格式的结构化日志。
func logStructured(event string, fields map[string]any) {
	m := make(map[string]any, len(fields)+2)
	m["event"] = event
	m["ts"] = time.Now().UTC().Format(time.RFC3339)
	for k, v := range fields {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	log.Println(string(b))
}

// sanitizeQuery 对 query 串做脱敏：将 token 参数值替换为 ***，避免敏感信息进入日志。
// 非法参数（如未编码的 % 序列）原样保留，不做二次解析，防止误改。
func sanitizeQuery(raw string) string {
	if raw == "" || !strings.Contains(strings.ToLower(raw), "token") {
		return raw
	}
	return tokenQueryRe.ReplaceAllString(raw, "${1}token=***")
}
