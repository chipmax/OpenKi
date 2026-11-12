package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The embedded hub used to accept every local caller and advertise ACAO:*.
// It must now drop a request with no key and keep the CORS header off even
// for an authorized response, so a web page cannot read the account pool.
func TestHubAuthDropsUnauthenticatedRequests(t *testing.T) {
	s, _ := newPoolServer(t)
	s.SetAuthKeys([]string{"hub-key"})

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(rec, req)
	}()
	if recovered != http.ErrAbortHandler {
		t.Fatalf("unauthenticated request recovered %v, want http.ErrAbortHandler", recovered)
	}
}

func TestHubAuthAcceptsBearerAndXAPIKeyWithoutCORS(t *testing.T) {
	s, _ := newPoolServer(t)
	s.SetAuthKeys([]string{"hub-key"})

	for _, set := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer hub-key") },
		func(r *http.Request) { r.Header.Set("x-api-key", "hub-key") },
	} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		set(req)
		rec := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("authorized /health = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("hub still advertises Access-Control-Allow-Origin: %q", got)
		}
	}
}
