package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The dashboard is embedded in the binary, so an upgrade changes the same URL
// without changing its path. If the response is cacheable, a browser can keep
// showing the previous build (old branding and no new network controls) even
// though the new process is already serving /health. The page must never be
// stored.
func TestDashboardDisablesBrowserCache(t *testing.T) {
	req := httptest.NewRequest("GET", "http://127.0.0.1:8787/", nil)
	req.Host = "127.0.0.1:8787"
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()

	(&Server{}).serveDashboard(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	cache := strings.ToLower(rec.Header().Get("Cache-Control"))
	if !strings.Contains(cache, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store so an upgrade cannot leave the old UI in the browser", rec.Header().Get("Cache-Control"))
	}
	if !strings.Contains(strings.ToLower(rec.Header().Get("Pragma")), "no-cache") {
		t.Errorf("Pragma = %q, want no-cache", rec.Header().Get("Pragma"))
	}
}

func TestDashboardUsesOpenKiBranding(t *testing.T) {
	html := string(dashboardHTML)
	if !strings.Contains(html, "<title>OpenKi") {
		t.Error("dashboard title is not OpenKi")
	}
	if strings.Contains(strings.ToLower(html), "zen-gate") || strings.Contains(html, `ZEN<span`) {
		t.Error("dashboard still exposes the legacy zen-gate brand")
	}
	if !strings.Contains(html, `OPEN<span class="tick">—</span>KI`) {
		t.Error("dashboard sidebar/window mark is not OpenKi")
	}
}
