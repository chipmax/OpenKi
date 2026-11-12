package lane

// Equal egress peers: relay, sing-box and WARP.
//
// Historically each of these was a proxy mode (or a wrapper) that won over the
// others: WarpEnabled overrode ProxyMode, "rotate" was the only subscription
// path, and a relay did not exist. The user can now tick any combination of
// the three. They are equal peers: each request starts at the next peer in a
// round-robin, a transport failure moves to the next peer, and a 429/403 moves
// to the next peer before the lane's own quota retry sees the response. An
// empty mode list keeps the legacy single-choice behaviour untouched.
//
// The peers are deliberately different shapes:
//   - relay is a RoundTripper that rewrites the URL to a Bun relay;
//   - WARP is a RoundTripper that swaps the dialer for a Cloudflare lease;
//   - sing-box is a dialer-level Rotator (via rotateDialContext), wrapped in a
//     transport so it can sit next to the other two.
//
// Making them all RoundTrippers is what lets the selector choose one per
// request and fall back without knowing how the peer reaches the network.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// RelayEndpoint is one relay server. It is the lane's copy of
// store.RelayPeer; the store type is converted at the wiring layer so lane
// stays free of a store import.
type RelayEndpoint struct {
	ID    string
	URL   string
	Token string
}

// EgressOptions is the lane-facing egress configuration. BaseMode/BaseURL
// describe the legacy proxy used for peers that need a plain network path
// (the relay and WARP transports). Rotate/pool/warp base modes collapse to
// direct here because the peer itself supplies the egress.
type EgressOptions struct {
	Modes        []string
	Relays       []RelayEndpoint
	WarpPoolSize int
	BaseMode     string
	BaseURL      string
}

// EgressNote describes a mode that was saved but cannot dial yet. Empty means
// every requested peer is usable.
func EgressNote(note string) string { return note }

var egressExplicit atomic.Bool

// SetEgress installs the equal-peer selector. An empty Modes list restores
// the legacy transport exactly as SetProxy built it. The returned note is the
// user-facing explanation for a requested peer that is configured but not
// dialable; it never makes the save fail.
func SetEgress(o EgressOptions) (note string) {
	modes, err := normalizeEgressModes(o.Modes)
	if err != nil {
		// A bad mode is a programming/config validation error, not a reason to
		// leave the previous transport in place silently.
		clearEgress()
		setProxyTransport("direct", "")
		return err.Error()
	}
	if len(modes) == 0 {
		clearEgress()
		setProxyTransport(o.BaseMode, o.BaseURL)
		return ""
	}

	baseMode := o.BaseMode
	switch baseMode {
	case "rotate", "pool", "warp":
		baseMode = "direct"
	}
	base := newPeerBaseTransport(baseMode, o.BaseURL)
	peers := make([]egressPeer, 0, len(modes))
	var notes []string
	for _, mode := range modes {
		switch mode {
		case "relay":
			relays := usableRelays(o.Relays)
			if len(relays) == 0 {
				notes = append(notes, "relay đã bật nhưng chưa có relayPool khả dụng")
				continue
			}
			peers = append(peers, egressPeer{name: "relay", rt: &relayRoundTripper{base: base, peers: relays}})
		case "singbox":
			// The rotator may arrive after boot (the subscription sidecar is
			// pulled in a goroutine), so the peer is installed even when the
			// pool is empty right now. rotateDialContext falls back to a plain
			// dial until a node exists, exactly like the legacy rotate mode.
			peers = append(peers, egressPeer{name: "singbox", rt: newSingBoxRoundTripper()})
			if CurrentRotator() == nil {
				notes = append(notes, "sing-box đã bật nhưng chưa có node nào; dial sẽ tạm đi trực tiếp")
			}
		case "warp":
			strict := len(modes) > 1 // with alternatives, a dead pool must move on
			peers = append(peers, egressPeer{name: "warp", rt: newWarpPeer(base, o.WarpPoolSize, strict)})
		}
	}
	if len(peers) == 0 {
		clearEgress()
		setProxyTransport(baseMode, o.BaseURL)
		if len(notes) == 0 {
			notes = append(notes, "không có egress peer nào khả dụng")
		}
		return strings.Join(notes, "; ")
	}
	egressExplicit.Store(true)
	egressActive.Store(len(peers) > 1 || peers[0].name == "relay" || peers[0].name == "singbox")
	laneHTTP.Transport = &egressSelector{peers: peers, fallback: base}
	return strings.Join(notes, "; ")
}

// EgressExplicit reports whether the selector is installed. It is used by the
// dashboard/tests to distinguish the new peer layer from the legacy transport.
func EgressExplicit() bool { return egressExplicit.Load() }

// EgressActive reports whether the installed egress can change per request.
// The lane uses it to decide whether a 429 is worth re-offering; unlike
// RotationActive it also covers a relay or a multi-peer selector with no
// sing-box node.
func EgressActive() bool { return egressActive.Load() }

var egressActive atomic.Bool

func clearEgress() {
	egressExplicit.Store(false)
	egressActive.Store(false)
}

// NormalizeEgressModes validates and deduplicates the mode strings for the
// settings handler. It is exported so the gateway does not need a second
// whitelist that can drift from the lane's.
func NormalizeEgressModes(in []string) ([]string, error) {
	return normalizeEgressModes(in)
}

func normalizeEgressModes(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		m := strings.ToLower(strings.TrimSpace(raw))
		if m == "" {
			continue
		}
		switch m {
		case "relay", "singbox", "warp":
		default:
			return nil, fmt.Errorf("egress mode không xác định: %s", raw)
		}
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out, nil
}

func usableRelays(in []RelayEndpoint) []RelayEndpoint {
	out := make([]RelayEndpoint, 0, len(in))
	seen := map[string]bool{}
	for _, r := range in {
		u := strings.TrimSpace(r.URL)
		if u == "" {
			continue
		}
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		key := strings.TrimRight(u, "/")
		if seen[key] {
			continue
		}
		seen[key] = true
		r.URL = key
		r.ID = strings.TrimSpace(r.ID)
		if r.ID == "" {
			r.ID = fmt.Sprintf("relay-%d", len(out)+1)
		}
		out = append(out, r)
	}
	return out
}

// newPeerBaseTransport is the plain path for relay/WARP. It mirrors the
// tuning in SetProxy but deliberately ignores rotate/pool/warp mode strings:
// those are represented by peer entries, not by the base.
func newPeerBaseTransport(mode, rawURL string) *http.Transport {
	t := &http.Transport{
		ResponseHeaderTimeout: responseHeaderTimeout,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	switch mode {
	case "direct", "rotate", "pool", "warp":
		t.Proxy = nil
	case "custom":
		if u, err := parseProxyURL(rawURL); err == nil {
			t.Proxy = func(*http.Request) (*url.URL, error) { return u, nil }
		} else {
			t.Proxy = http.ProxyFromEnvironment
		}
	case "system":
		t.Proxy = func(*http.Request) (*url.URL, error) { return SystemProxyURL(), nil }
	default:
		t.Proxy = http.ProxyFromEnvironment
	}
	return t
}

// newSingBoxRoundTripper wraps the existing per-dial Rotator in a transport.
// Keep-alives are disabled for the same reason as SetProxy("rotate"): quota
// is counted per egress IP and a reused connection pins the process to one
// node despite the rotator's per-request choice.
func newSingBoxRoundTripper() *http.Transport {
	return &http.Transport{
		ResponseHeaderTimeout: responseHeaderTimeout,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     true,
		DialContext:           rotateDialContext,
	}
}

// --- relay ---------------------------------------------------------------

type relayRoundTripper struct {
	base  *http.Transport
	peers []RelayEndpoint
	rr    atomic.Uint64
}

func (r *relayRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || isLoopbackAddr(req.URL.Host) {
		return r.base.RoundTrip(req)
	}
	if len(r.peers) == 0 {
		return r.base.RoundTrip(req)
	}
	start := int(r.rr.Add(1)-1) % len(r.peers)
	var lastErr error
	for off := range len(r.peers) {
		peer := r.peers[(start+off)%len(r.peers)]
		clone, err := cloneRelayRequest(req, peer, off == 0)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := r.base.RoundTrip(clone)
		if err == nil {
			if resp.Header != nil {
				resp.Header.Set("X-LoiMo-Egress", "relay:"+peer.ID)
			}
			return resp, nil
		}
		lastErr = err
		if req.Context().Err() != nil {
			return nil, err
		}
		if req.Body != nil && req.GetBody == nil {
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("relay: không có đầu ra")
	}
	return nil, lastErr
}

func cloneRelayRequest(req *http.Request, peer RelayEndpoint, first bool) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body != nil {
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			clone.Body = body
		} else if !first {
			return nil, errors.New("relay: request body không thể phát lại")
		}
	}
	target, err := relayTarget(peer.URL, req.URL)
	if err != nil {
		return nil, err
	}
	clone.URL = target
	clone.Host = ""
	clone.RequestURI = ""
	clone.Header = req.Header.Clone()
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	for k := range clone.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-loimo-relay-") {
			clone.Header.Del(k)
		}
	}
	clone.Header.Set("X-LoiMo-Relay-Token", peer.Token)
	clone.Header.Set("X-LoiMo-Relay-Upstream", req.URL.Scheme+"://"+req.URL.Host)
	return clone, nil
}

func relayTarget(base string, orig *url.URL) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("relay URL không hợp lệ: %s", base)
	}
	prefix := strings.TrimRight(u.Path, "/")
	u.Path = prefix + orig.Path
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawQuery = orig.RawQuery
	u.Fragment = ""
	return u, nil
}

// --- selector ------------------------------------------------------------

type egressPeer struct {
	name string
	rt   http.RoundTripper
}

type egressSelector struct {
	peers    []egressPeer
	fallback http.RoundTripper
	rr       atomic.Uint64
}

func (s *egressSelector) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || isLoopbackAddr(req.URL.Host) || len(s.peers) == 0 {
		return s.fallback.RoundTrip(req)
	}
	start := int(s.rr.Add(1)-1) % len(s.peers)
	var lastErr error
	var lastResp *http.Response
	var lastBody []byte
	for off := range len(s.peers) {
		peer := s.peers[(start+off)%len(s.peers)]
		attempt, err := cloneEgressAttempt(req, off == 0)
		if err != nil {
			lastErr = err
			break
		}
		// A WARP attempt that falls through to the next peer must not leave
		// its lease address on the response that another peer finally served.
		resetEgressIdentity(attempt.Context())
		resp, err := peer.rt.RoundTrip(attempt)
		if err != nil {
			lastErr = fmt.Errorf("egress %s: %w", peer.name, err)
			if req.Context().Err() != nil {
				return nil, err
			}
			if req.Body != nil && req.GetBody == nil {
				break
			}
			continue
		}
		if resp.Header != nil && resp.Header.Get("X-LoiMo-Egress") == "" {
			resp.Header.Set("X-LoiMo-Egress", peer.name)
		}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusForbidden {
			return resp, nil
		}
		// The upstream rejected this egress, not necessarily the turn. Buffer
		// the small error body, close the connection and let the next equal
		// peer try. If every peer rejects, the last response is returned with
		// its body intact so the lane still applies its normal quota rules.
		if off == len(s.peers)-1 {
			return resp, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
		lastResp = resp
		lastBody = body
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		if req.Body != nil && req.GetBody == nil {
			break
		}
	}
	if lastResp != nil {
		lastResp.Body = io.NopCloser(bytes.NewReader(lastBody))
		if lastResp.Header != nil {
			lastResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(lastBody)))
		}
		return lastResp, nil
	}
	if lastErr == nil {
		lastErr = errors.New("egress: không có peer khả dụng")
	}
	return nil, lastErr
}

func cloneEgressAttempt(req *http.Request, first bool) (*http.Request, error) {
	if first {
		return req, nil
	}
	if req.Body == nil {
		return req.Clone(req.Context()), nil
	}
	if req.GetBody == nil {
		return nil, errors.New("egress: request body không thể phát lại")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.Body = body
	return clone, nil
}

// newWarpPeer is in warp.go (it needs the WARP pool seam for tests).
