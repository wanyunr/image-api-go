package app

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

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

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	portrait, landscape := h.pool.Count()
	w.Header().Set("Cache-Control", "no-store")
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
	if h.cfg.PrefetchEnabled() {
		p := h.pool.Pending()
		d := h.pool.Dead()
		resp.Pool.Pending = &p
		resp.Pool.Dead = &d
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	// 支持浏览器直接访问 URL 刷新（GET，token 走查询参数），也支持脚本用 POST；
	// 其它方法拒绝。
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// GET 刷新必须直达服务端：禁止浏览器/代理缓存 reload 响应。
	w.Header().Set("Cache-Control", "no-store")

	stats, err := h.pool.Load(h.cfg)
	if err != nil {
		log.Printf("[reload failed] %v", err)
		http.Error(w, "Reload Failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if h.cfg.PrefetchEnabled() {
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
	if h.cfg.PrefetchEnabled() {
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
	w.Header().Set("Cache-Control", "no-store")
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
	w.Header().Set("Cache-Control", "no-store")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items := h.recent.Recent(limit)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"total": h.recent.Size(),
		"items": items,
	})
}
