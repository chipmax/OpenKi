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

// Proton must be a first-class, visible part of the network page, not a
// checkbox buried inside the collapsed upstream-proxy fold. The card is the
// only place these controls may live: duplicate ids would bind only the first
// one and leave the visible form inert.
func TestProtonControlsLiveInVisibleNetworkCard(t *testing.T) {
	html := string(dashboardHTML)
	card := indexAfter(t, html, `id="net-egress-card"`)
	exits := indexAfter(t, html, `id="net-exits"`)
	fold := indexAfter(t, html, `id="net-proxy"`)
	if !(card < exits && exits < fold) {
		t.Fatalf("expected egress card before the exit table and the fold (card=%d exits=%d fold=%d)", card, exits, fold)
	}
	visible := html[card:exits]
	for _, id := range []string{`id="set-proton-url"`, `id="btn-proton-test"`, `id="btn-proton-save"`} {
		if !strings.Contains(visible, id) {
			t.Errorf("%s is not inside the visible network card", id)
		}
	}
	if strings.Count(html, `id="set-proton-url"`) != 1 {
		t.Errorf("set-proton-url is declared %d times; duplicate ids bind only the first control", strings.Count(html, `id="set-proton-url"`))
	}
	if !strings.Contains(visible, `value="proton"`) {
		t.Error("the visible card has no Proton egress checkbox")
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

func indexAfter(t *testing.T, s, marker string) int {
	t.Helper()
	i := strings.Index(s, marker)
	if i < 0 {
		t.Fatalf("dashboard is missing %s", marker)
	}
	return i
}
