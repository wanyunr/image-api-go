package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBasePathHandlerSupportsExactBasePath(t *testing.T) {
	target := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("expected root path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	handler := newBasePathHandler("/api", target)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestNewBasePathHandlerSupportsPrefixedPath(t *testing.T) {
	target := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("expected stripped path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	handler := newBasePathHandler("/api", target)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}
