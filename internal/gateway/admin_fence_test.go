package gateway

import (
	"net/http/httptest"
	"testing"
)

func TestAdminFenceRejectsForwardedHostWithoutPort(t *testing.T) {
	r := httptest.NewRequest("GET", "http://gateway.example.com/admin/api/state", nil)
	r.Host = "gateway.example.com"
	r.RemoteAddr = "127.0.0.1:4567"
	w := httptest.NewRecorder()
	if adminAllowed(w, r, false) {
		t.Fatal("proxied non-loopback Host must be rejected")
	}
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAdminFenceRejectsProxyHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:8787/admin/api/state", nil)
	r.Host = "127.0.0.1:8787"
	r.RemoteAddr = "127.0.0.1:4567"
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	w := httptest.NewRecorder()
	if adminAllowed(w, r, false) {
		t.Fatal("admin behind a proxy must be rejected")
	}
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}
