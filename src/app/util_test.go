package app

import (
	"net/http/httptest"
	"testing"
	"time"
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

func TestDetectDeviceType(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)", "mobile"},
		{"Mozilla/5.0 (Linux; Android 14; Pixel)", "mobile"},
		{"Mozilla/5.0 (iPad; CPU OS 17_0)", "tablet"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "desktop"},
		{"curl/8.0", "desktop"},
	}
	for _, c := range cases {
		if got := DetectDeviceType(c.ua); got != c.want {
			t.Errorf("DetectDeviceType(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}

func TestFilenameFromURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://cdn.example.com/img/001.jpg", "001.jpg"},
		{"https://cdn.example.com/img/001.jpg?v=2&x=1", "001.jpg"},
		{"https://cdn.example.com/a%20b.png", "a b.png"},
		{"https://cdn.example.com/", ""},
	}
	for _, c := range cases {
		if got := filenameFromURL(c.in); got != c.want {
			t.Errorf("filenameFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m 30s"},
		{2*time.Hour + 3*time.Minute, "2h 3m 0s"},
		{25 * time.Hour, "1d 1h 0m 0s"},
	}
	for _, c := range cases {
		if got := formatDuration(c.in); got != c.want {
			t.Errorf("formatDuration = %q, want %q", got, c.want)
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
