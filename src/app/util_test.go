package app

import (
	"net/http/httptest"
	"testing"
)

func TestGetClientIP_Priority(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	if got := getClientIP(req); got != "1.2.3.4" {
		t.Fatalf("XFF 应优先，got %q", got)
	}
}

func TestGetClientIP_Fallback(t *testing.T) {
	t.Run("X-Real-IP 回退", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.5:12345"
		req.Header.Set("X-Real-IP", "5.6.7.8")
		if got := getClientIP(req); got != "5.6.7.8" {
			t.Fatalf("got %q, want 5.6.7.8", got)
		}
	})

	t.Run("RemoteAddr 回退", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.5:12345"
		if got := getClientIP(req); got != "203.0.113.5" {
			t.Fatalf("got %q, want 203.0.113.5", got)
		}
	})
}

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

func TestSanitizeQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"orientation=portrait", "orientation=portrait"},
		{"token=secret123", "token=***"},
		{"format=json&token=secret123", "format=json&token=***"},
		{"token=secret123&orientation=landscape", "token=***&orientation=landscape"},
		{"a=1&TOKEN=ABC&b=2", "a=1&token=***&b=2"},
	}
	for _, c := range cases {
		if got := sanitizeQuery(c.in); got != c.want {
			t.Errorf("sanitizeQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
