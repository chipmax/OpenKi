package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chipmax/openki/internal/store"
)

func TestQoderProbeSendsTheHubKey(t *testing.T) {
	SetQoderHubKey("hub-key")
	t.Cleanup(func() { SetQoderHubKey("") })

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	old := qoderBaseURL
	qoderBaseURL = srv.URL
	t.Cleanup(func() { qoderBaseURL = old })

	if code, _, err := qoderGet("/health"); err != nil || code != http.StatusOK {
		t.Fatalf("qoderGet = %d, %v", code, err)
	}
	if got != "Bearer hub-key" {
		t.Fatalf("Qoder probe Authorization = %q, want the hub key", got)
	}
}

func TestQoderProviderSyncCarriesTheHubKey(t *testing.T) {
	SetQoderHubKey("hub-key")
	t.Cleanup(func() { SetQoderHubKey("") })

	s := qoderSeedProvider(t)
	s.Store.Mutate(func(c *store.Config) {
		for i := range c.Providers {
			if c.Providers[i].ID == qoderProviderID {
				c.Providers[i].APIKey = ""
			}
		}
	})
	st := qoderStatus{Definitive: true, LoggedIn: true, ModelIDs: []string{"m"}}
	if !s.ensureQoderProvider(st) {
		t.Fatal("provider sync did not wire the Qoder provider")
	}
	for _, p := range s.Store.Config().Providers {
		if p.ID == qoderProviderID {
			if p.APIKey != "hub-key" {
				t.Fatalf("Qoder provider APIKey = %q, want the hub key", p.APIKey)
			}
			return
		}
	}
	t.Fatal("Qoder provider missing after sync")
}
