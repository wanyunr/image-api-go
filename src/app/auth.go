package app

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

func (h *Handler) checkAuth(r *http.Request) bool {
	if !h.cfg.Auth.Enabled {
		return true
	}
	token := extractToken(r)
	for _, t := range h.cfg.Auth.Tokens {
		if tokenMatches(token, t) {
			return true
		}
	}
	return false
}

// checkAdminAuth 管理端点鉴权：优先使用专用 admin_token，
// 未配置时回退到普通 tokens（仅在 auth.enabled 时生效）。
func (h *Handler) checkAdminAuth(r *http.Request) bool {
	token := extractToken(r)
	if h.cfg.Auth.AdminToken != "" {
		return tokenMatches(token, h.cfg.Auth.AdminToken)
	}
	if h.cfg.Auth.Enabled {
		for _, t := range h.cfg.Auth.Tokens {
			if tokenMatches(token, t) {
				return true
			}
		}
	}
	return false
}

func (h *Handler) AdminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.checkAdminAuth(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractToken 从 query 参数 token 或 Authorization: Bearer 头提取令牌。
func extractToken(r *http.Request) string {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
	}
	return token
}

func tokenMatches(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (h *Handler) checkReferer(r *http.Request) bool {
	if !h.cfg.Referer.Enabled {
		return true
	}
	referer := r.Header.Get("Referer")
	if referer == "" {
		return h.cfg.Referer.AllowEmpty
	}
	refererHost := extractHost(referer)
	if refererHost == "" {
		return h.cfg.Referer.AllowEmpty
	}
	for _, pattern := range h.cfg.Referer.Whitelist {
		if matchReferer(refererHost, pattern) {
			return true
		}
	}
	return false
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
