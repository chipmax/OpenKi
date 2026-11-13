package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chipmax/openki/internal/lane"
	"github.com/chipmax/openki/internal/store"
)

// relayProbeRecord is one dashboard-triggered relay test. Results are keyed by
// canonical URL, not by row id: an unsaved editor row has no stable id, and a
// URL is what the user is actually testing.
type relayProbeRecord struct {
	URL       string `json:"url"`
	IP        string `json:"ip,omitempty"`
	Country   string `json:"country,omitempty"`
	LatencyMs int64  `json:"latencyMs"`
	CheckedAt int64  `json:"checkedAt"`
	Error     string `json:"error,omitempty"`
}

func canonicalRelayProbeURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	return strings.TrimRight(u, "/")
}

func (s *Server) setRelayProbe(rec relayProbeRecord) {
	key := canonicalRelayProbeURL(rec.URL)
	if key == "" {
		return
	}
	s.relayProbeMu.Lock()
	if s.relayProbe == nil {
		s.relayProbe = map[string]relayProbeRecord{}
	}
	s.relayProbe[key] = rec
	s.relayProbeMu.Unlock()
}

func (s *Server) relayProbeSnapshot() map[string]relayProbeRecord {
	s.relayProbeMu.Lock()
	defer s.relayProbeMu.Unlock()
	out := make(map[string]relayProbeRecord, len(s.relayProbe))
	for k, v := range s.relayProbe {
		out[k] = v
	}
	return out
}

// adminRelayTest probes one or many relay URLs. It accepts the editor's
// current rows, including unsaved ones, so a test does not require saving a
// possibly broken relay first. Tests run with bounded concurrency; each gets
// its own timeout and the live egress selector is never touched.
func (s *Server) adminRelayTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Relays []store.RelayPeer `json:"relays"`
		URL    string            `json:"url"`
		Token  string            `json:"token"`
		Kind   string            `json:"kind"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if len(in.Relays) == 0 && strings.TrimSpace(in.URL) != "" {
		in.Relays = []store.RelayPeer{{URL: in.URL, Token: in.Token, Kind: in.Kind, Enabled: true}}
	}
	if len(in.Relays) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Chưa có relay để dò"})
		return
	}
	if len(in.Relays) > 50 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Tối đa 50 relay mỗi lượt dò"})
		return
	}

	type job struct {
		index int
		peer  lane.RelayEndpoint
	}
	jobs := make([]job, 0, len(in.Relays))
	seen := map[string]bool{}
	for _, relay := range in.Relays {
		key := canonicalRelayProbeURL(relay.URL)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		jobs = append(jobs, job{
			index: len(jobs),
			peer: lane.RelayEndpoint{
				ID:    relay.ID,
				URL:   relay.URL,
				Token: relay.Token,
				Kind:  relay.Kind,
			},
		})
	}
	if len(jobs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Không có URL relay hợp lệ"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	results := make([]relayProbeRecord, len(jobs))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[j.index] = relayProbeRecord{URL: j.peer.URL, CheckedAt: time.Now().UnixMilli(), Error: ctx.Err().Error()}
				return
			}
			defer func() { <-sem }()

			pctx, pcancel := context.WithTimeout(ctx, 12*time.Second)
			defer pcancel()
			eg, latency, err := lane.ProbeRelay(pctx, j.peer)
			rec := relayProbeRecord{
				URL:       j.peer.URL,
				IP:        eg.IP,
				Country:   eg.Country,
				LatencyMs: latency,
				CheckedAt: time.Now().UnixMilli(),
			}
			if err != nil {
				rec.Error = err.Error()
			}
			results[j.index] = rec
			s.setRelayProbe(rec)
		}(j)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": results})
}
