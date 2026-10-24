package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

// requestStatus 依次执行防盗链 → 鉴权 → 限流检查，返回应写入的状态码；0 表示放行。
func (h *Handler) requestStatus(r *http.Request) int {
	if !h.checkReferer(r) {
		return http.StatusForbidden
	}
	if !h.checkAuth(r) {
		return http.StatusUnauthorized
	}
	if !h.limiter.Allow() {
		return http.StatusTooManyRequests
	}
	return 0
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

	if code := h.requestStatus(r); code != 0 {
		if code == http.StatusTooManyRequests {
			logStructured("rate_limit_hit", map[string]any{"ip": ip, "ua": ua, "query": sanitizeQuery(r.URL.RawQuery)})
		}
		http.Error(w, http.StatusText(code), code)
		return
	}

	// 随机接口每次都应返回新图，禁止浏览器缓存和条件请求复用。
	setNoCacheHeaders(w)

	orientation := r.URL.Query().Get("orientation")
	if orientation == "" {
		device := DetectDeviceType(ua)
		if device == "mobile" {
			orientation = OrientationPortrait
		} else {
			orientation = OrientationLandscape
		}
	}
	if orientation != OrientationPortrait && orientation != OrientationLandscape {
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
	if format == "" {
		format = "redirect"
	}

	img, localFile, err := h.pickImage(orientation)
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
	if localFile != nil {
		defer localFile.Close()
	}

	h.recent.Add(recentAccess{
		URL:         img.URL,
		Orientation: orientation,
		Format:      format,
		At:          time.Now(),
	})

	switch format {
	case "json":
		info := img
		if h.cfg.Server.MetadataMode == MetadataModeOnRequest || info.Width == 0 {
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
		if localFile != nil {
			serveFileHandle(w, r, filepath.Base(img.LocalPath), localFile)
			logStructured("serve_local", map[string]any{"ip": ip, "ua": ua, "orientation": orientation, "path": img.LocalPath})
			return
		}
		http.Redirect(w, r, img.URL, http.StatusFound)
		logStructured("redirect", map[string]any{"ip": ip, "ua": ua, "orientation": orientation, "url": img.URL})
	}
}

// localPickAttempts 是本地文件失效时的重选次数。
const localPickAttempts = 3

// pickImage 随机取一张可用的图片。本地图源会在挑选时直接打开文件：
// 文件若在扫描后被删除，os.Open 会返回 ENOENT，丢弃这一张并在剩余候选
// 中重选（最多 localPickAttempts 次），既避免选中坏图，也没有 stat/open
// 之间的竞态窗口。远程 URL 不返回文件句柄。
func (h *Handler) pickImage(orientation string) (ImageInfo, *os.File, error) {
	tried := make(map[string]struct{}, localPickAttempts)
	var missing error
	for i := 0; i < localPickAttempts; i++ {
		img, err := h.pool.RandomExcluding(orientation, tried)
		if err != nil {
			// 候选用尽：若过程中遇到过缺失文件，返回更具体的错误。
			if missing != nil {
				return ImageInfo{}, nil, fmt.Errorf("%w: %v", ErrLocalImageMissing, missing)
			}
			return ImageInfo{}, nil, err
		}
		if img.LocalPath == "" {
			return img, nil, nil
		}

		f, openErr := openLocalFile(img.LocalPath)
		if openErr == nil {
			return img, f, nil
		}
		if errors.Is(openErr, errLocalFileChanged) {
			// 扫描后被替换成符号链接/其它文件：当作不可用，排除后重选。
			missing = openErr
			tried[img.LocalPath] = struct{}{}
			logStructured("error", map[string]any{"type": "local_file_changed", "path": img.LocalPath})
			continue
		}
		if !os.IsNotExist(openErr) {
			// 权限等其它问题交给调用方按 500 处理，不当作文件缺失重选。
			return ImageInfo{}, nil, openErr
		}
		missing = openErr
		tried[img.LocalPath] = struct{}{}
		logStructured("error", map[string]any{"type": "local_file_missing", "path": img.LocalPath})
	}
	if missing != nil {
		return ImageInfo{}, nil, fmt.Errorf("%w: %v", ErrLocalImageMissing, missing)
	}
	return ImageInfo{}, nil, fmt.Errorf("%w: %s", ErrPoolEmpty, orientation)
}

// ServeLocal 提供本地图片的静态访问入口。
// 只允许访问启动时扫描到的文件，避免目录穿越和隐藏文件暴露。
func (h *Handler) ServeLocal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if code := h.requestStatus(r); code != 0 {
		http.Error(w, http.StatusText(code), code)
		return
	}

	path, ok := h.pool.LocalFile(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	f, err := openLocalFile(path)
	if err != nil {
		// 缺失或扫描后被替换为符号链接：一律按不存在处理，避免暴露路径状态。
		if os.IsNotExist(err) || errors.Is(err, errLocalFileChanged) {
			logStructured("error", map[string]any{"type": "local_static_unavailable", "path": path, "err": err.Error()})
			http.NotFound(w, r)
			return
		}
		logStructured("error", map[string]any{"type": "local_static_error", "path": path, "err": err.Error()})
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		logStructured("error", map[string]any{"type": "local_static_stat", "path": path, "err": err.Error()})
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, filepath.Base(path), fi.ModTime(), f)
}

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// serveFileHandle 以零修改时间返回已打开的文件，避免 If-Modified-Since 命中 304。
// 文件在挑选阶段已成功打开，因此这里不存在 stat 后再被删除的竞态。
func serveFileHandle(w http.ResponseWriter, r *http.Request, name string, f *os.File) {
	http.ServeContent(w, r, name, time.Time{}, f)
}
