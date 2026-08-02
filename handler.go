package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// ImageJSONResponse format=json 返回结构（字段顺序即 JSON 输出顺序）
type ImageJSONResponse struct {
	URL         string `json:"url"`
	Orientation string `json:"orientation"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Format      string `json:"format"`
	Filename    string `json:"filename"`
}

type Handler struct {
	cfg       *Config
	pool      *URLPool
	limiter   *RateLimiter
	startTime time.Time
	totalReqs atomic.Int64
}

func NewHandler(cfg *Config, pool *URLPool, limiter *RateLimiter) *Handler {
	return &Handler{
		cfg:       cfg,
		pool:      pool,
		limiter:   limiter,
		startTime: time.Now(),
	}
}

// ServeImage 主接口：返回随机图片 URL
func (h *Handler) ServeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 忽略 favicon 等无关请求
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ip := getClientIP(r)
	ua := r.UserAgent()
	h.totalReqs.Add(1)
	log.Printf("[请求] IP=%s UA=%s Query=%s", ip, ua, r.URL.RawQuery)

	// Referer 防盗链检查
	if !h.checkReferer(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Token 鉴权
	if !h.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 访问频率限制
	if !h.limiter.Allow(ip) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// 解析参数：未指定 orientation 时根据 UA 自动判断
	orientation := r.URL.Query().Get("orientation")
	if orientation == "" {
		device := DetectDeviceType(ua)
		if device == "mobile" {
			orientation = "portrait"
		} else {
			// desktop / tablet 默认横屏
			orientation = "landscape"
		}
	}
	if orientation != "portrait" && orientation != "landscape" {
		http.Error(w, "Bad Request: orientation 必须为 portrait 或 landscape", http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format")
	if format != "" && format != "json" && format != "text" && format != "redirect" {
		http.Error(w, "Bad Request: format 必须为 json、text 或 redirect", http.StatusBadRequest)
		return
	}

	// 从 URL 池随机选取
	imgURL, err := h.pool.Random(orientation)
	if err != nil {
		if strings.Contains(err.Error(), "URL 池为空") {
			http.Error(w, "Service Unavailable: URL 池为空", http.StatusServiceUnavailable)
			return
		}
		log.Printf("[错误] %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 返回结果
	switch format {
	case "json":
		// JSON 模式：请求时探测图片尺寸和格式
		info := FetchImageInfo(imgURL)
		resp := ImageJSONResponse{
			URL:         info.URL,
			Orientation: orientation,
			Width:       info.Width,
			Height:      info.Height,
			Format:      info.Format,
			Filename:    filenameFromURL(imgURL),
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(resp)
	case "text":
		// 纯文本模式：返回 URL 字符串（适合 curl/脚本）
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(imgURL))
	default:
		// 默认：302 重定向到图片，浏览器直接展示
		http.Redirect(w, r, imgURL, http.StatusFound)
	}
}

// Health 健康检查
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	portrait, landscape := h.pool.Count()
	uptime := time.Since(h.startTime)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"uptime": formatDuration(uptime),
		"requests_since_start": h.totalReqs.Load(),
		"pool": map[string]int{
			"portrait":  portrait,
			"landscape": landscape,
			"total":     portrait + landscape,
		},
	})
}

// Reload 热重载 URL 列表
func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 管理接口鉴权
	if !h.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	err := h.pool.Load(h.cfg.URLs.Portrait, h.cfg.URLs.Landscape)
	if err != nil {
		log.Printf("[重载失败] %v", err)
		http.Error(w, "Reload Failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	portrait, landscape := h.pool.Count()
	log.Printf("[重载成功] portrait=%d landscape=%d", portrait, landscape)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "reloaded",
		"portrait":  portrait,
		"landscape": landscape,
	})
}

// checkAuth 检查 Token 鉴权
func (h *Handler) checkAuth(r *http.Request) bool {
	if !h.cfg.Auth.Enabled {
		return true
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
	}

	for _, t := range h.cfg.Auth.Tokens {
		if token == t {
			return true
		}
	}
	return false
}

// checkReferer 检查 Referer 防盗链
func (h *Handler) checkReferer(r *http.Request) bool {
	if !h.cfg.Referer.Enabled {
		return true
	}

	referer := r.Header.Get("Referer")

	// Referer 为空
	if referer == "" {
		return h.cfg.Referer.AllowEmpty
	}

	// 提取 Referer 的 host
	refererHost := extractHost(referer)
	if refererHost == "" {
		return h.cfg.Referer.AllowEmpty
	}

	// 匹配白名单
	for _, pattern := range h.cfg.Referer.Whitelist {
		if matchReferer(refererHost, pattern) {
			return true
		}
	}
	return false
}
