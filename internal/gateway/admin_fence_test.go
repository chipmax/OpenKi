package gateway

import (
	"net/http"
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

func mutatingAdminRequest(t *testing.T) *http.Request {
	t.Helper()
	r := httptest.NewRequest("POST", "http://127.0.0.1:8787/admin/api/settings", nil)
	r.Host = "127.0.0.1:8787"
	r.RemoteAddr = "127.0.0.1:4567"
	r.Header.Set("Content-Type", "application/json")
	return r
}

// The old substring check accepted http://127.0.0.1.evil.com and
// http://127.0.0.1@evil.com because both contain "://127.0.0.1". A cross-site
// page could drive every mutating admin route through that hole.
func TestAdminFenceRejectsLookalikeOrigins(t *testing.T) {
	for _, origin := range []string{
		"http://127.0.0.1.evil.com",
		"http://127.0.0.1@evil.com",
		"http://localhost.evil.com",
		"http://127.0.0.1:9999",
		"null",
	} {
		r := mutatingAdminRequest(t)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-OpenKi-Admin", "1")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		if adminAllowed(w, r, true) {
			t.Errorf("Origin %q was accepted as same-origin", origin)
		}
		if w.Code != 403 {
			t.Errorf("Origin %q status = %d, want 403", origin, w.Code)
		}
	}
}

func TestAdminFenceAcceptsSameOriginWithCSRFHeader(t *testing.T) {
	r := mutatingAdminRequest(t)
	r.Header.Set("Origin", "http://127.0.0.1:8787")
	r.Header.Set("Referer", "http://127.0.0.1:8787/admin/settings")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("X-OpenKi-Admin", "1")
	w := httptest.NewRecorder()
	if !adminAllowed(w, r, true) {
		t.Fatalf("same-origin admin mutation rejected: %d %s", w.Code, w.Body.String())
	}
}

func TestAdminFenceRejectsBrowserMutationWithoutCSRFHeader(t *testing.T) {
	r := mutatingAdminRequest(t)
	r.Header.Set("Origin", "http://127.0.0.1:8787")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	if adminAllowed(w, r, true) {
		t.Fatal("browser mutation without X-OpenKi-Admin was accepted")
	}
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAdminFenceRejectsCrossSiteFetchMetadata(t *testing.T) {
	r := mutatingAdminRequest(t)
	r.Header.Set("Origin", "http://127.0.0.1:8787")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("X-OpenKi-Admin", "1")
	w := httptest.NewRecorder()
	if adminAllowed(w, r, true) {
		t.Fatal("Sec-Fetch-Site: cross-site was accepted")
	}
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAdminFenceKeepsNonBrowserAdminClientsWorking(t *testing.T) {
	r := mutatingAdminRequest(t)
	w := httptest.NewRecorder()
	if !adminAllowed(w, r, true) {
		t.Fatalf("headerless non-browser admin request rejected: %d %s", w.Code, w.Body.String())
	}
}
