package lane

import (
	"context"
	"sync"
)

// egressIdentity carries the physical egress chosen inside a RoundTripper back
// to CallRecord. ExitPlan is decided before the request is built (a rotator
// node), but WARP and relay peers are selected later, while the request is in
// flight; without this hand-off their address is visible to the transport and
// lost before the attempt is recorded.
//
// One pointer is installed per attempt. The equal-peer selector resets it
// before trying the next peer, so a WARP attempt that falls through to a relay
// cannot leave a stale address on the successful record.
type egressIdentity struct {
	mu   sync.Mutex
	peer string
	ip   string
}

type egressIdentityKey struct{}

func withEgressIdentity(ctx context.Context) (context.Context, *egressIdentity) {
	identity := &egressIdentity{}
	return context.WithValue(ctx, egressIdentityKey{}, identity), identity
}

func egressIdentityFrom(ctx context.Context) *egressIdentity {
	if identity, ok := ctx.Value(egressIdentityKey{}).(*egressIdentity); ok {
		return identity
	}
	return nil
}

// setEgressIdentity records the peer and, when the transport knows it, the
// public address that served the response.
func setEgressIdentity(ctx context.Context, peer, ip string) {
	if identity := egressIdentityFrom(ctx); identity != nil {
		identity.set(peer, ip)
	}
}

// resetEgressIdentity clears an earlier peer before another peer is tried.
func resetEgressIdentity(ctx context.Context) {
	if identity := egressIdentityFrom(ctx); identity != nil {
		identity.reset()
	}
}

func (e *egressIdentity) set(peer, ip string) {
	e.mu.Lock()
	e.peer, e.ip = peer, ip
	e.mu.Unlock()
}

func (e *egressIdentity) reset() {
	e.mu.Lock()
	e.peer, e.ip = "", ""
	e.mu.Unlock()
}

func (e *egressIdentity) get() (peer, ip string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.peer, e.ip
}
