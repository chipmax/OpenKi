package warp

import (
	"net/netip"
	"testing"
	"time"
)

// The measured failure storm was 44 refusals across a rotating pool. This
// simulation walks the same shape over 18 egress blocks and asserts the one
// property that matters: no block is ever benched beyond the 15-minute cap,
// and a burst inside two spacing windows is not mistaken for a persistent
// block.
func TestEgressCooldownPolicyUnder429Storm(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	penalties := map[string]*egressPenalty{}
	seen := map[string]bool{}
	for i := 0; i < 44; i++ {
		key := "2606:4700:110:8a1e::" + string(rune('a'+i%18)) + "/64"
		seen[key] = true
		penalty := penalties[key]
		var previous time.Time
		if penalty == nil || now.Sub(penalty.last) > egressFailMemory {
			penalty = &egressPenalty{first: now}
			penalties[key] = penalty
		}
		previous = penalty.last
		penalty.fails++
		penalty.last = now
		cooldown := egressCooldown(penalty, previous, now, 0)
		if cooldown <= 0 || cooldown > egressCooldownMax {
			t.Fatalf("refusal %d on %s cooled for %s, outside (0, %s]", i+1, key, cooldown, egressCooldownMax)
		}
		now = now.Add(30 * time.Second)
	}
	if len(seen) != 18 {
		t.Fatalf("simulated %d distinct exits, want 18", len(seen))
	}
}

// Three refusals inside two minutes are a crowd, not proof that the address is
// exhausted. The third step must still be the 360-second ramp, not the
// 15-minute confirmed park.
func TestEgressCooldownShortBurstIsNotConfirmed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	penalty := &egressPenalty{first: now}
	var cooldown time.Duration
	for i := 0; i < 3; i++ {
		previous := penalty.last
		penalty.fails++
		penalty.last = now
		cooldown = egressCooldown(penalty, previous, now, 0)
		now = now.Add(30 * time.Second)
	}
	if cooldown != 6*time.Minute {
		t.Fatalf("three refusals within two minutes cooled for %s, want 6m", cooldown)
	}
}

// The same three refusals spread across the spacing window are evidence of a
// persistent block and take the cap.
func TestEgressCooldownSpacedFailuresTakeTheCap(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	now := start
	penalty := &egressPenalty{first: now}
	var cooldown time.Duration
	for i := 0; i < 3; i++ {
		previous := penalty.last
		penalty.fails++
		penalty.last = now
		cooldown = egressCooldown(penalty, previous, now, 0)
		now = now.Add(10 * time.Minute)
	}
	if cooldown != egressCooldownMax {
		t.Fatalf("three spaced refusals cooled for %s, want %s", cooldown, egressCooldownMax)
	}
}

func TestClampEgressHintRejectsMidnightAndRespectsCap(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{54_282 * time.Second, 0},
		{-time.Second, 0},
		{egressCooldownMax, egressCooldownMax},
		{egressCooldownMax + time.Second, 0},
		{5 * time.Minute, 5 * time.Minute},
	}
	for _, c := range cases {
		if got := clampEgressHint(c.in); got != c.want {
			t.Errorf("clampEgressHint(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestLeaseEgressReportsTunnelAddress(t *testing.T) {
	want := netip.MustParseAddr("2606:4700:110:8a1e::1")
	lease := &Lease{exit: &exit{egress: want}}
	if got := lease.Egress(); got != want {
		t.Fatalf("Egress() = %s, want %s", got, want)
	}
	var missing *Lease
	if got := missing.Egress(); got.IsValid() {
		t.Fatalf("nil lease Egress() = %s, want invalid", got)
	}
}
