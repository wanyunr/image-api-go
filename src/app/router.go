package app

import (
	"net/http"
	"strings"
)

// NewRouter 组装全部 HTTP 路由：随机图片、健康检查、本地静态访问与管理端点。
// server.base_path 非空时，所有路由整体挂载到该前缀之下。
func NewRouter(cfg *Config, h *Handler) http.Handler {
	root := http.NewServeMux()
	routes := root

	if bp := strings.TrimRight(cfg.Server.BasePath, "/"); bp != "" {
		inner := http.NewServeMux()
		root.Handle(bp+"/", http.StripPrefix(bp, inner))
		routes = inner
	}

	routes.HandleFunc("/", h.ServeImage)
	routes.HandleFunc("/health", h.Health)
	if cfg.UsesLocal() {
		routes.HandleFunc("/local/", h.ServeLocal)
	}

	admin := http.NewServeMux()
	admin.HandleFunc("/admin/reload", h.Reload)
	admin.HandleFunc("/admin/clear-dead", h.ClearDead)
	admin.HandleFunc("/admin/recent", h.Recent)
	routes.Handle("/admin/", h.AdminAuth(admin))

	return nosniffHeaders(root)
}

// nosniffHeaders 对所有响应禁用浏览器 MIME 嗅探，强制按声明的 Content-Type 渲染。
// 这是 polyglot 文件（既是合法图片、又嵌恶意脚本的内容）的核心防线：本服务
// 的图片 Content-Type 由扩展名白名单决定、恒为 image/*，nosniff 使浏览器无从
// 把它嗅探成 HTML 执行。
// 有意不加 Content-Security-Policy：导航过带 CSP 响应的页面会把策略继承给
// 后续 about:blank 文档（实测 default-src 'none' 会阻断图片加载，sandbox 会使
// 加载悬挂），伤及正常嵌入场景；nosniff 已覆盖本服务的实际攻击面。
func nosniffHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
