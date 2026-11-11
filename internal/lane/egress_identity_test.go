package lane

import (
	"context"
	"testing"
	"time"
)

// A WARP lease is chosen inside the transport, after ExitPlan was built. The
// attempt record must still name the public address that served it, and must
// not keep a stale peer when the selector falls through to another egress.
func TestRecordStampsTransportEgressIdentity(t *testing.T) {
	oldExplicit := egressExplicit.Load()
	egressExplicit.Store(true)
	t.Cleanup(func() { egressExplicit.Store(oldExplicit) })

	l := NewLane()
	var got CallRecord
	l.OnCall = func(rec CallRecord) { got = rec }

	ctx, _ := withEgressIdentity(context.Background())
	setEgressIdentity(ctx, "warp", "2606:4700:110:8a1e::1")
	ctx = WithExitPlan(ctx, ExitPlan{Model: "glm-5.3-flash-free"})
	l.record(ctx, CallRecord{Model: "glm-5.3-flash-free", At: time.Now().UnixMilli()}, StreamResult{}, nil, 0)

	if got.EgressIP != "2606:4700:110:8a1e::1" {
		t.Fatalf("EgressIP = %q, want the WARP lease address", got.EgressIP)
	}
	if got.ExitKind != "warp" {
		t.Fatalf("ExitKind = %q, want warp", got.ExitKind)
	}

	// The equal-peer selector resets before each peer. A relay that follows a
	// failed WARP attempt must not inherit the WARP address.
	resetEgressIdentity(ctx)
	l.record(ctx, CallRecord{Model: "glm-5.3-flash-free", At: time.Now().UnixMilli()}, StreamResult{}, nil, 0)
	if got.EgressIP != "" {
		t.Fatalf("EgressIP = %q after reset, want empty", got.EgressIP)
	}
}
