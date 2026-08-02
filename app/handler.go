package app

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// ImageJSONResponse is returned when format=json.
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
	log.Printf("[request] IP=%s UA=%s Query=%s", ip, ua, r.URL.RawQuery)

	if !h.checkReferer(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if !h.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !h.limiter.Allow(ip) {
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

	imgURL, err := h.pool.Random(orientation)
	if err != nil {
		if strings.Contains(err.Error(), "URL pool is empty") {
			http.Error(w, "Service Unavailable: URL pool is empty", http.StatusServiceUnavailable)
			return
		}
		log.Printf("[error] %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	switch format {
	case "json":
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
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(imgURL))
	default:
		http.Redirect(w, r, imgURL, http.StatusFound)
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	portrait, landscape := h.pool.Count()
	uptime := time.Since(h.startTime)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":               "ok",
		"uptime":               formatDuration(uptime),
		"requests_since_start": h.totalReqs.Load(),
		"pool": map[string]int{
			"portrait":  portrait,
			"landscape": landscape,
			"total":     portrait + landscape,
		},
	})
}

func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	err := h.pool.Load(h.cfg.URLs.Portrait, h.cfg.URLs.Landscape)
	if err != nil {
		log.Printf("[reload failed] %v", err)
		http.Error(w, "Reload Failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	portrait, landscape := h.pool.Count()
	log.Printf("[reload success] portrait=%d landscape=%d", portrait, landscape)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "reloaded",
		"portrait":  portrait,
		"landscape": landscape,
	})
}

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
