package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://cdn.example.com/img/001.jpg", "cdn.example.com"},
		{"http://example.com:8080/path?x=1", "example.com"},
		{"https://[2001:db8::1]:8443/a.jpg", "2001:db8::1"},
		{"https://EXAMPLE.com/UPPER", "example.com"},
		{"not-a-url", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractHost(c.in); got != c.want {
			t.Errorf("extractHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchReferer(t *testing.T) {
	cases := []struct {
		host, pattern string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "other.com", false},
		{"www.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},
		{"badexample.com", "*.example.com", false},
		{"www.example.com", "*.Example.com", true}, // pattern 大小写不敏感；host 已由 extractHost 归一为小写
	}
	for _, c := range cases {
		if got := matchReferer(c.host, c.pattern); got != c.want {
			t.Errorf("matchReferer(%q, %q) = %v, want %v", c.host, c.pattern, got, c.want)
		}
	}
}

func TestExtractToken(t *testing.T) {
	t.Run("query 参数", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?token=abc", nil)
		if got := extractToken(req); got != "abc" {
			t.Fatalf("got %q, want abc", got)
		}
	})
	t.Run("Bearer 头", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer abc")
		if got := extractToken(req); got != "abc" {
			t.Fatalf("got %q, want abc", got)
		}
	})
	t.Run("缺失时为空", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := extractToken(req); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func newAuthTestHandler(t *testing.T, auth AuthConfig, referer RefererConfig) (*Handler, *URLPool) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Auth = auth
	cfg.Referer = referer
	pool := NewURLPool()
	return NewHandler(cfg, pool, NewRateLimiter(cfg.RateLimit)), pool
}

func TestCheckAuth(t *testing.T) {
	t.Run("未启用时恒放行", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{Enabled: false}, RefererConfig{})
		if !h.checkAuth(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("auth 关闭时应放行")
		}
	})
	t.Run("启用后无 token 拒绝", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{Enabled: true, Tokens: []string{"abc"}}, RefererConfig{})
		if h.checkAuth(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("缺少 token 应拒绝")
		}
	})
	t.Run("tokens 任一匹配即放行", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{Enabled: true, Tokens: []string{"abc", "xyz"}}, RefererConfig{})
		for _, token := range []string{"abc", "xyz"} {
			req := httptest.NewRequest(http.MethodGet, "/?token="+token, nil)
			if !h.checkAuth(req) {
				t.Fatalf("token=%s 应放行", token)
			}
		}
		req := httptest.NewRequest(http.MethodGet, "/?token=bad", nil)
		if h.checkAuth(req) {
			t.Fatal("错误 token 应拒绝")
		}
	})
}

func TestCheckAdminAuth(t *testing.T) {
	t.Run("admin_token 优先且不再回退普通 tokens", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{
			Enabled:    true,
			Tokens:     []string{"user"},
			AdminToken: "admin",
		}, RefererConfig{})
		req := httptest.NewRequest(http.MethodGet, "/?token=admin", nil)
		if !h.checkAdminAuth(req) {
			t.Fatal("admin_token 应放行")
		}
		req = httptest.NewRequest(http.MethodGet, "/?token=user", nil)
		if h.checkAdminAuth(req) {
			t.Fatal("配置了 admin_token 后普通 token 不应放行管理端点")
		}
	})
	t.Run("未配置 admin_token 时回退普通 tokens", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{Enabled: true, Tokens: []string{"user"}}, RefererConfig{})
		req := httptest.NewRequest(http.MethodGet, "/?token=user", nil)
		if !h.checkAdminAuth(req) {
			t.Fatal("普通 token 应放行")
		}
	})
	t.Run("auth 未启用且无 admin_token 时拒绝", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{Enabled: false}, RefererConfig{})
		if h.checkAdminAuth(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("无任何可用凭证时应拒绝")
		}
	})
}

func TestCheckReferer(t *testing.T) {
	cfg := RefererConfig{
		Enabled:    true,
		AllowEmpty: false,
		Whitelist:  []string{"example.com", "*.trusted.org"},
	}
	requestWithReferer := func(referer string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		return req
	}

	t.Run("未启用时恒放行", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{}, RefererConfig{Enabled: false})
		if !h.checkReferer(httptest.NewRequest(http.MethodGet, "/", nil)) {
			t.Fatal("referer 校验关闭时应放行")
		}
	})
	t.Run("白名单精确与通配", func(t *testing.T) {
		h, _ := newAuthTestHandler(t, AuthConfig{}, cfg)
		for _, referer := range []string{
			"https://example.com/page",
			"https://sub.trusted.org/a",
			"https://trusted.org/a",
		} {
			if !h.checkReferer(requestWithReferer(referer)) {
				t.Fatalf("referer=%s 应放行", referer)
			}
		}
		if h.checkReferer(requestWithReferer("https://evil.com/")) {
			t.Fatal("非白名单referer应拒绝")
		}
		if h.checkReferer(requestWithReferer("")) {
			t.Fatal("allow_empty=false 时空 Referer 应拒绝")
		}
	})
	t.Run("allow_empty 放行空 Referer", func(t *testing.T) {
		allowEmpty := cfg
		allowEmpty.AllowEmpty = true
		h, _ := newAuthTestHandler(t, AuthConfig{}, allowEmpty)
		if !h.checkReferer(requestWithReferer("")) {
			t.Fatal("allow_empty=true 时空 Referer 应放行")
		}
	})
}
