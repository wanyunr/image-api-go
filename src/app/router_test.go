package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newRouterTestHandler(t *testing.T) (*Config, *Handler) {
	t.Helper()
	cfg := DefaultConfig()
	pool := NewURLPool()
	return cfg, NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit))
}

func TestRouterHealth(t *testing.T) {
	cfg, h := newRouterTestHandler(t)
	router := NewRouter(cfg, h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("health 响应非法: %v", err)
	}
	if resp.Status != "ok" || resp.Pool.Total != 0 {
		t.Fatalf("health 响应不符: %+v", resp)
	}
}

func TestRouterServeImageMounted(t *testing.T) {
	cfg, h := newRouterTestHandler(t)
	router := NewRouter(cfg, h)

	// 非法 orientation 的 400 证明随机图片处理器已挂载在根路径。
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?orientation=square", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, want 400", rec.Code)
	}
}

func TestRouterAdminAuth(t *testing.T) {
	cfg, h := newRouterTestHandler(t)
	cfg.Auth.AdminToken = "admin-only"
	router := NewRouter(cfg, h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/recent", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无凭证状态码 = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/recent?token=admin-only", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("携带 admin_token 状态码 = %d, want 200", rec.Code)
	}
}

func TestRouterBasePath(t *testing.T) {
	cfg, h := newRouterTestHandler(t)
	cfg.Server.BasePath = "/imgapi"
	router := NewRouter(cfg, h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/imgapi/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("前缀下 health 状态码 = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("无前缀访问状态码 = %d, want 404", rec.Code)
	}

	// 管理端点同样应挂在前缀下。
	cfg.Auth.AdminToken = "admin-only"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/imgapi/admin/recent?token=admin-only", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("前缀下 admin 状态码 = %d, want 200", rec.Code)
	}
}
