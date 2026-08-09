package app

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
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

	img, err := h.pool.Random(orientation)
	if err != nil {
		logStructured("error", map[string]any{"type": "pool_error", "err": err.Error(), "orientation": orientation})
		if errors.Is(err, ErrPoolEmpty) {
			http.Error(w, "Service Unavailable: URL pool is empty", http.StatusServiceUnavailable)
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
		http.Redirect(w, r, img.URL, http.StatusFound)
		logStructured("redirect", map[string]any{"ip": ip, "ua": ua, "orientation": orientation, "url": img.URL})
	}
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
	stats, err := h.pool.Load(h.cfg.URLs.Portrait, h.cfg.URLs.Landscape, h.cfg.Server.CacheDir)
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
