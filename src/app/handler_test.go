package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testPNGBytes 生成指定尺寸的 PNG 字节，供 httptest 图片服务返回。
func testPNGBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "img.png")
	writeTestPNG(t, path, width, height)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newURLModeHandler 构建 url 模式处理器：图片元数据由本地 httptest 服务提供，
// json 格式的尺寸探测不会触网。
func newURLModeHandler(t *testing.T) *Handler {
	t.Helper()

	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(testPNGBytes(t, 640, 360))
	}))
	t.Cleanup(imgSrv.Close)

	root := t.TempDir()
	portraitFile := filepath.Join(root, "portrait.txt")
	if err := os.WriteFile(portraitFile, []byte(imgSrv.URL+"/p1.png\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	landscapeFile := filepath.Join(root, "landscape.txt")
	if err := os.WriteFile(landscapeFile, []byte(imgSrv.URL+"/l1.png\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig() // on_request 模式
	cfg.URLs.Portrait = portraitFile
	cfg.URLs.Landscape = landscapeFile

	pool := NewURLPool()
	if _, err := pool.Load(cfg); err != nil {
		t.Fatal(err)
	}
	return NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))
}

func TestServeImageFormats(t *testing.T) {
	h := newURLModeHandler(t)

	t.Run("redirect 默认 302", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=landscape", nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("状态码 = %d, want 302", rec.Code)
		}
		if loc := rec.Header().Get("Location"); !strings.HasSuffix(loc, "/l1.png") {
			t.Fatalf("Location = %q", loc)
		}
	})

	t.Run("text 输出原始 URL", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=landscape&format=text", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("状态码 = %d", rec.Code)
		}
		if body := rec.Body.String(); !strings.HasSuffix(body, "/l1.png") {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("json 携带探测到的尺寸", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=landscape&format=json", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("状态码 = %d", rec.Code)
		}
		var resp ImageJSONResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析 JSON 失败: %v", err)
		}
		if resp.Width != 640 || resp.Height != 360 {
			t.Fatalf("尺寸探测不符: %+v", resp)
		}
		if resp.Filename != "l1.png" || resp.Format != "png" {
			t.Fatalf("文件名/格式不符: %+v", resp)
		}
	})
}

func TestServeImageRequestErrors(t *testing.T) {
	h := newURLModeHandler(t)

	cases := []struct {
		name, target string
		wantCode     int
	}{
		{"非 GET 方法", "", http.StatusMethodNotAllowed},
		{"未知路径", "/x", http.StatusNotFound},
		{"非法 orientation", "/?orientation=square", http.StatusBadRequest},
		{"非法 format", "/?format=xml", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req *http.Request
			if c.target == "" {
				req = httptest.NewRequest(http.MethodPost, "/", nil)
			} else {
				req = httptest.NewRequest(http.MethodGet, c.target, nil)
			}
			rec := httptest.NewRecorder()
			h.ServeImage(rec, req)
			if rec.Code != c.wantCode {
				t.Fatalf("状态码 = %d, want %d", rec.Code, c.wantCode)
			}
		})
	}
}

func TestServeImageEmptyPool(t *testing.T) {
	cfg := DefaultConfig()
	pool := NewURLPool() // 空池
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	rec := httptest.NewRecorder()
	h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=portrait&format=text", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("空池状态码 = %d, want 503", rec.Code)
	}
}

func TestServeImageRejectsAuth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Enabled = true
	cfg.Auth.Tokens = []string{"secret"}
	pool := NewURLPool()
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	rec := httptest.NewRecorder()
	h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=portrait", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 状态码 = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?orientation=portrait&token=secret&format=text", nil))
	if rec.Code != http.StatusServiceUnavailable { // 鉴权通过但池为空
		t.Fatalf("携带 token 应通过鉴权，状态码 = %d, want 503", rec.Code)
	}
}

func TestServeImageRateLimited(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.PerMinute = 1
	pool := NewURLPool()
	h := NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))

	rec := httptest.NewRecorder()
	h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?format=text", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("首个请求应通过限流（空池 503），实际 %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeImage(rec, httptest.NewRequest(http.MethodGet, "/?format=text", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超额请求状态码 = %d, want 429", rec.Code)
	}
}
