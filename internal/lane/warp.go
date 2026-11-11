package lane

// WARP egress for the lane: an optional RoundTripper wrapper that carries
// each request out through the userspace Cloudflare WARP pool in
// internal/warp (ported from opencode2api; the pool itself is adapted from
// the MIT-licensed ai-route — see internal/warp/NOTICE).
//
// Design contract:
//   - Opt-in only: useWarp decides from config (warpEnabled, or
//     proxyMode == "warp"). WarpEnabled wins over every ProxyMode.
//   - One lease per request, held until the response Body closes, so an
//     exit can never rotate mid-stream (SSE safety).
//   - 429/403 responses are reported back to the pool (with Retry-After)
//     so the burned exit cools down / gets replaced; other statuses are
//     not the exit's verdict and are not reported.
//   - Any failure to build the pool or acquire a lease degrades to the
//     base transport instead of failing the request: WARP needs outbound
//     UDP to Cloudflare, which plenty of networks block, and a gateway
//     that answers slowly beats one that answers 502. Fallbacks are
//     counted (WarpFallbacks) so the degradation is observable.

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"zen-gate/internal/warp"
)

// useWarp is the pure mode decision, split out so it can be tested without
// a network: WARP is on when the dedicated switch is set, or when the
// proxy mode string names it.
func useWarp(warpEnabled bool, proxyMode string) bool {
	return warpEnabled || proxyMode == "warp"
}

var (
	warpCfgEnabled  atomic.Bool
	warpCfgPoolSize atomic.Int64
	warpFallbackN   atomic.Int64
)

// SetWarpEnabled records the config's WARP switch and pool size. Call it
// before SetProxy (boot and settings saves both do) so the transport that
// SetProxy builds knows whether to wrap.
func SetWarpEnabled(enabled bool, poolSize int) {
	warpCfgEnabled.Store(enabled)
	warpCfgPoolSize.Store(int64(poolSize))
}

// WarpFallbacks counts requests that wanted a WARP exit but were served by
// the base transport because the pool could not be built or leased.
func WarpFallbacks() int64 { return warpFallbackN.Load() }

// --- shared pool (lazy) ---------------------------------------------------

var (
	warpPoolOnce    sync.Once
	warpPoolInst    *warp.Pool
	warpPoolErr     error
	warpPoolErrAt   time.Time
	warpPoolRetryIn = time.Minute
)

// sharedWarpPool returns the process-wide pool, building it on first use.
// A failed build is remembered for warpPoolRetryIn so a blocked network
// does not pay a registration attempt per request, then retried — the
// network may have opened up since.
func sharedWarpPool(poolSize int) (*warp.Pool, error) {
	if warpPoolInst != nil {
		return warpPoolInst, nil
	}
	if warpPoolErr != nil && time.Since(warpPoolErrAt) < warpPoolRetryIn {
		return nil, warpPoolErr
	}
	// Retry after a cooled-down failure: arm a fresh Once.
	if warpPoolErr != nil {
		warpPoolOnce = sync.Once{}
		warpPoolErr = nil
	}
	warpPoolOnce.Do(func() {
		if poolSize <= 0 {
			poolSize = 2 // opencode2api's DefaultPoolSize
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		p, err := warp.NewWithOptions(ctx, slog.Default(), warp.Options{
			Size:  poolSize,
			Stack: warp.StackIPv6,
		})
		if err != nil {
			warpPoolErr = err
			warpPoolErrAt = time.Now()
			slog.Default().Warn("lane: WARP pool unavailable, lane traffic falls back to the base transport", "err", err)
			return
		}
		warpPoolInst = p
	})
	if warpPoolInst == nil {
		return nil, warpPoolErr
	}
	return warpPoolInst, nil
}

// --- round tripper ---------------------------------------------------------

// warpRoundTripper wraps the base transport built by SetProxy.
type warpRoundTripper struct {
	base     *http.Transport
	poolSize int
	// strict makes an unavailable/empty WARP pool a transport error instead of
	// falling back to base. The equal-peer selector needs that signal so a
	// failed WARP peer moves to relay/sing-box; the legacy single-mode wrapper
	// keeps the forgiving fallback.
	strict bool
}

// newWarpPeer builds the equal-peer form of the WARP transport.
func newWarpPeer(base *http.Transport, poolSize int, strict bool) *warpRoundTripper {
	return &warpRoundTripper{base: base, poolSize: poolSize, strict: strict}
}

// warpPoolFor is a seam for tests: when set, it replaces sharedWarpPool.
var warpPoolFor = sharedWarpPool

func (w *warpRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Loopback is never WARP business: the lane client also serves
	// user-added providers on this machine (e.g. a local opencode2api),
	// and a WARP netstack cannot dial 127.0.0.1.
	if req.URL != nil && isLoopbackAddr(req.URL.Host) {
		return w.base.RoundTrip(req)
	}
	pool, err := warpPoolFor(w.poolSize)
	if err != nil {
		warpFallbackN.Add(1)
		if w.strict {
			return nil, err
		}
		return w.base.RoundTrip(req)
	}
	actx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	lease, err := pool.Acquire(actx)
	cancel()
	if err != nil {
		warpFallbackN.Add(1)
		slog.Default().Warn("lane: WARP lease failed, request falls back to the base transport", "err", err)
		if w.strict {
			return nil, err
		}
		return w.base.RoundTrip(req)
	}

	clone := w.base.Clone()
	clone.Proxy = nil // the tunnel is the egress; a proxy would double-hop
	clone.DisableKeepAlives = true
	clone.ForceAttemptHTTP2 = false
	clone.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	clone.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return lease.DialContext(ctx, lease.DialNetwork(), addr)
	}
	resp, err := clone.RoundTrip(req)
	if err != nil {
		pool.Fail(lease, "transport error")
		lease.Release()
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		head, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		pool.ReportWithRetryAfter(lease, resp.StatusCode, head, parseRetryAfter(resp.Header))
		resp.Body = &leaseBody{
			Reader:  io.MultiReader(bytes.NewReader(head), resp.Body),
			closer:  resp.Body,
			release: lease.Release,
		}
		return resp, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		pool.Success(lease)
	}
	resp.Body = &leaseBody{Reader: resp.Body, closer: resp.Body, release: lease.Release}
	return resp, nil
}

// parseRetryAfter reads a Retry-After header as seconds or an HTTP date.
func parseRetryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// leaseBody ties a response body to its WARP lease: the lease is released
// exactly once, when the body is closed — never earlier, so a streaming
// response keeps its exit for its whole lifetime.
type leaseBody struct {
	io.Reader
	closer  io.Closer
	release func()
	once    sync.Once
}

func (b *leaseBody) Close() error {
	var err error
	b.once.Do(func() {
		if b.closer != nil {
			err = b.closer.Close()
		}
		if b.release != nil {
			b.release()
		}
	})
	return err
}
