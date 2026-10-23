package app

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type ImageJSONResponse struct {
	URL         string `json:"url"`
	Orientation string `json:"orientation"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Format      string `json:"format"`
	Filename    string `json:"filename"`
}

type HealthResponse struct {
	Status             string   `json:"status"`
	Uptime             string   `json:"uptime"`
	RequestsSinceStart int64    `json:"requests_since_start"`
	Pool               PoolInfo `json:"pool"`
}

type PoolInfo struct {
	Portrait  int  `json:"portrait"`
	Landscape int  `json:"landscape"`
	Total     int  `json:"total"`
	Pending   *int `json:"pending,omitempty"`
	Dead      *int `json:"dead,omitempty"`
}

type ReloadResponse struct {
	Status    string `json:"status"`
	Portrait  int    `json:"portrait"`
	Landscape int    `json:"landscape"`
	Pending   *int   `json:"pending,omitempty"`
	Dead      *int   `json:"dead,omitempty"`
}

type Handler struct {
	cfg       *Config
	pool      *URLPool
	limiter   *RateLimiter
	recent    *recentBuffer
	startTime time.Time
	totalReqs atomic.Int64
}

func NewHandler(cfg *Config, pool *URLPool, limiter *RateLimiter) *Handler {
	return &Handler{
		cfg:       cfg,
		pool:      pool,
		limiter:   limiter,
		recent:    newRecentBuffer(cfg.Server.RecentBufferSize),
		startTime: time.Now(),
	}
}

func (h *Handler) ServeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ip := getClientIP(r)
	ua := r.UserAgent()
	h.totalReqs.Add(1)
	logStructured("request", map[string]any{"ip": ip, "ua": ua, "query": sanitizeQuery(r.URL.RawQuery)})

	if !h.checkReferer(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !h.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !h.limiter.Allow() {
		logStructured("rate_limit_hit", map[string]any{"ip": ip, "ua": ua, "query": sanitizeQuery(r.URL.RawQuery)})
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// 随机接口每次都应返回新图，禁止浏览器缓存和条件请求复用。
	setNoCacheHeaders(w)

	orientation := r.URL.Query().Get("orientation")
	if orientation == "" {
		device := DetectDeviceType(ua)
		if device == "mobile" {
			orientation = "portrait"
		} else {
			orientation = "landscape"
		}
	}
	if orientation != "portrait" && orientation != "landscape" {
		http.Error(w, "Bad Request: orientation must be portrait or landscape", http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format")
	if format != "" && format != "json" && format != "text" && format != "redirect" {
		http.Error(w, "Bad Request: format must be json, text or redirect", http.StatusBadRequest)
		return
	}
	// local 图源不存在"外部地址"可返回，只支持直接输出文件。
	if h.cfg.UsesLocal() && (format == "json" || format == "text") {
		logStructured("error", map[string]any{"type": "local_format_unsupported", "format": format})
		http.Error(w, "Bad Request: source=local only supports format=redirect", http.StatusBadRequest)
		return
	}

	img, err := h.pickImage(orientation)
	if err != nil {
		logStructured("error", map[string]any{"type": "pool_error", "err": err.Error(), "orientation": orientation})
		if errors.Is(err, ErrPoolEmpty) {
			http.Error(w, "Service Unavailable: URL pool is empty", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, ErrLocalImageMissing) {
			http.Error(w, "Service Unavailable: local image files are missing", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	effectiveFormat := format
	if effectiveFormat == "" {
		effectiveFormat = "redirect"
	}
	h.recent.Add(recentAccess{
		URL:         img.URL,
		Orientation: orientation,
		Format:      effectiveFormat,
		At:          time.Now(),
	})

	switch format {
	case "json":
		info := img
		if h.cfg.Server.MetadataMode == "on_request" || info.Width == 0 {
			info = FetchImageInfo(img.URL)
		}
		resp := ImageJSONResponse{
			URL:         info.URL,
			Orientation: orientation,
			Width:       info.Width,
			Height:      info.Height,
			Format:      info.Format,
			Filename:    filenameFromURL(info.URL),
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(resp)
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(img.URL))
	default:
		if img.LocalPath != "" {
			if err := serveFileNoConditional(w, r, img.LocalPath); err != nil {
				logStructured("error", map[string]any{"type": "serve_local_error", "err": err.Error(), "path": img.LocalPath})
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			logStructured("serve_local", map[string]any{"ip": ip, "ua": ua, "orientation": orientation, "path": img.LocalPath})
			return
		}
		http.Redirect(w, r, img.URL, http.StatusFound)
		logStructured("redirect", map[string]any{"ip": ip, "ua": ua, "orientation": orientation, "url": img.URL})
	}
}

// localPickAttempts 是本地文件失效时的重选次数。
const localPickAttempts = 3

// pickImage 随机取一张可用的图片。本地文件若在扫描后被删除，
// 丢弃这一张并在剩余候选中重选，最多尝试 localPickAttempts 次，
// 避免随机接口稳定返回坏图，同时保证循环次数有上限。
func (h *Handler) pickImage(orientation string) (ImageInfo, error) {
	tried := make(map[string]struct{}, localPickAttempts)
	var missing error
	for i := 0; i < localPickAttempts; i++ {
		img, err := h.pool.RandomExcluding(orientation, tried)
		if err != nil {
			// 候选用尽：若过程中遇到过缺失文件，返回更具体的错误。
			if missing != nil {
				return ImageInfo{}, fmt.Errorf("%w: %v", ErrLocalImageMissing, missing)
			}
			return ImageInfo{}, err
		}
		if img.LocalPath == "" {
			return img, nil
		}

		_, statErr := os.Stat(img.LocalPath)
		if statErr == nil {
			return img, nil
		}
		if !os.IsNotExist(statErr) {
			// 权限等其它问题交给读取阶段报错，不当作文件缺失处理。
			return img, nil
		}
		missing = statErr
		tried[img.LocalPath] = struct{}{}
		logStructured("error", map[string]any{"type": "local_file_missing", "path": img.LocalPath})
	}
	if missing != nil {
		return ImageInfo{}, fmt.Errorf("%w: %v", ErrLocalImageMissing, missing)
	}
	return ImageInfo{}, fmt.Errorf("%w: %s", ErrPoolEmpty, orientation)
}

// ServeLocal 提供本地图片的静态访问入口。
// 只允许访问启动时扫描到的文件，避免目录穿越和隐藏文件暴露。
func (h *Handler) ServeLocal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.checkReferer(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !h.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !h.limiter.Allow() {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	path, ok := h.pool.LocalFile(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// serveFileNoConditional 以零修改时间返回文件，避免 If-Modified-Since 命中 304。
func serveFileNoConditional(w http.ResponseWriter, r *http.Request, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	http.ServeContent(w, r, filepath.Base(path), time.Time{}, f)
	return nil
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	portrait, landscape := h.pool.Count()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := HealthResponse{
		Status:             "ok",
		Uptime:             formatDuration(time.Since(h.startTime)),
		RequestsSinceStart: h.totalReqs.Load(),
		Pool: PoolInfo{
			Portrait:  portrait,
			Landscape: landscape,
			Total:     portrait + landscape,
		},
	}
	if h.cfg.Server.MetadataMode == "prefetch" {
		p := h.pool.Pending()
		d := h.pool.Dead()
		resp.Pool.Pending = &p
		resp.Pool.Dead = &d
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	stats, err := h.pool.Load(h.cfg.URLs.Portrait, h.cfg.URLs.Landscape, h.cfg.Server.CacheDir, h.cfg.Source, h.cfg.Local, h.cfg.Server.BasePath)
	if err != nil {
		log.Printf("[reload failed] %v", err)
		http.Error(w, "Reload Failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if h.cfg.Server.MetadataMode == "prefetch" {
		go h.pool.RetryPending()
		logStructured("reload", map[string]any{
			"portrait": stats.Portrait, "landscape": stats.Landscape,
			"pending": stats.Pending, "dead": stats.Dead,
		})
	} else {
		logStructured("reload", map[string]any{
			"portrait": stats.Portrait, "landscape": stats.Landscape,
		})
	}
	if stats.Portrait == 0 || stats.Landscape == 0 {
		log.Printf("[警告] reload 后存在空池: portrait=%d landscape=%d", stats.Portrait, stats.Landscape)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := ReloadResponse{
		Status:    "reloaded",
		Portrait:  stats.Portrait,
		Landscape: stats.Landscape,
	}
	if h.cfg.Server.MetadataMode == "prefetch" {
		resp.Pending = &stats.Pending
		resp.Dead = &stats.Dead
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) ClearDead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	h.pool.ClearDead()
	logStructured("clear_dead", map[string]any{})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "dead links cleared"})
}

// Recent 返回最近访问的图片记录（按时间倒序）。
// 用于追溯刚访问过的图，避免喜欢的图找不到。
func (h *Handler) Recent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items := h.recent.Recent(limit)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"total": h.recent.Size(),
		"items": items,
	})
}

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
