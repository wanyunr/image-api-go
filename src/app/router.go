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

	return root
}
