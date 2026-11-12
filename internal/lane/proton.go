package lane

// Proton VPN egress peer.
//
// Proton's desktop clients do not expose a gateway API; they expose a local
// SOCKS5 listener (the official client on some platforms, or a wrapper such as
// the pVPN helper used on this machine). That listener is a single exit, so it
// is modelled as one equal egress peer: requests are routed through it, and a
// deployment that wants failover can combine it with relay/sing-box/WARP in
// egressModes.
//
// The setting is deliberately a SOCKS5 URL, not Proton credentials. OpenKi
// never talks to Proton's account API, never stores the Proton password, and
// never manages the tunnel: it only dials the listener the operator already
// started. This keeps credential ownership and VPN lifecycle outside the
// gateway while still giving the lane a real, testable egress path.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultProtonSocks5 is the conventional loopback listener of a Proton VPN
// client. The value is used when "proton" is enabled without an explicit URL.
const DefaultProtonSocks5 = "socks5h://127.0.0.1:1080"

// NormalizeProtonSocks5 validates a Proton listener URL and fills in the
// default when the operator left it empty. Only SOCKS5 endpoints are accepted:
// Proton's client exposes SOCKS5, and silently treating an HTTP proxy as
// Proton would mislabel a different egress in the dashboard and logs.
func NormalizeProtonSocks5(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultProtonSocks5, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "socks5h://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("Proton SOCKS5 URL không hợp lệ")
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h":
	default:
		return "", fmt.Errorf("Proton chỉ hỗ trợ socks5:// hoặc socks5h://, nhận %q", u.Scheme)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("Proton SOCKS5 URL không được có đường dẫn")
	}
	if _, err := parsePoolNode(raw); err != nil {
		return "", err
	}
	return raw, nil
}

// NewProtonTransport builds a transport that dials every non-loopback target
// through one Proton SOCKS5 listener. Loopback targets still go direct: a
// local provider must remain reachable even when the VPN is down, and sending
// it to Proton would make it dial its own loopback.
//
// The transport is intentionally a *http.Transport so it can be installed as
// an equal peer next to relay/WARP/sing-box. Keep-alives are left enabled:
// Proton is a stable single exit, not a per-request rotation pool.
func NewProtonTransport(raw string) (*http.Transport, error) {
	raw, err := NormalizeProtonSocks5(raw)
	if err != nil {
		return nil, err
	}
	pool, err := NewProxyPool([]string{raw})
	if err != nil {
		return nil, err
	}
	node := pool.nodes[0]
	if node.scheme != "socks5" && node.scheme != "socks5h" {
		return nil, fmt.Errorf("Proton phải là SOCKS5, nhận %q", node.scheme)
	}
	t := &http.Transport{
		ResponseHeaderTimeout: responseHeaderTimeout,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if isLoopbackAddr(addr) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			}
			dctx, cancel := context.WithTimeout(ctx, dialTimeout)
			defer cancel()
			return pool.dialNode(dctx, node, network, addr)
		},
	}
	return t, nil
}
