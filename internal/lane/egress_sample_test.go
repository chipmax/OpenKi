package lane

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type sampleTransport struct {
	body string
	err  error
}

func (s sampleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

// SampleEgressNow is the dashboard's forced sample. It must update the same
// fields the periodic watcher writes, and a failed sample must not erase the
// last known exit.
func TestSampleEgressNowRecordsAndPreservesEvidence(t *testing.T) {
	old := laneHTTP.Transport
	t.Cleanup(func() { laneHTTP.Transport = old })

	laneHTTP.Transport = sampleTransport{body: `{"ip":"203.0.113.9","country":"sg"}`}
	l := NewLane()
	eg := l.SampleEgressNow(context.Background())
	if eg.IP != "203.0.113.9" || eg.Country != "SG" {
		t.Fatalf("sample = %+v", eg)
	}
	info := l.EgressInfo()
	if info.IP != eg.IP || info.Country != eg.Country || info.CheckedAt == 0 {
		t.Fatalf("recorded info = %+v, sample = %+v", info, eg)
	}
	if info.LastAt != 0 {
		t.Fatalf("a forced sample must not claim served traffic: LastAt=%d", info.LastAt)
	}

	laneHTTP.Transport = sampleTransport{err: errors.New("offline")}
	before := l.EgressInfo()
	eg = l.SampleEgressNow(context.Background())
	if eg.IP != "" {
		t.Fatalf("failed sample = %+v, want empty", eg)
	}
	after := l.EgressInfo()
	if after.IP != before.IP || after.Country != before.Country || after.CheckedAt != before.CheckedAt {
		t.Fatalf("failed sample erased evidence: before=%+v after=%+v", before, after)
	}
}
