package lane

import (
	"testing"
	"time"
)

func TestThrottleRampStartsAtNinetySecondsAndCaps(t *testing.T) {
	b := NewThrottleBook()
	now := time.Unix(1_700_000_000, 0)
	b.now = func() time.Time { return now }

	want := []time.Duration{90 * time.Second, 180 * time.Second, 360 * time.Second, 720 * time.Second, 900 * time.Second}
	for i, w := range want {
		b.MarkThrottled("m", 0)
		got := time.UnixMilli(b.Note("m").CooldownUntil).Sub(now)
		if got != w {
			t.Fatalf("refusal %d parked for %s, want %s", i+1, got, w)
		}
		now = now.Add(time.Second)
	}
}

func TestThrottleIgnoresNextMidnightRetryAfter(t *testing.T) {
	b := NewThrottleBook()
	now := time.Unix(1_700_000_000, 0)
	b.now = func() time.Time { return now }
	b.MarkThrottled("m", 24*3600)
	if got := time.UnixMilli(b.Note("m").CooldownUntil).Sub(now); got != 90*time.Second {
		t.Fatalf("a 24h hint parked the model for %s, want the 90s ramp start", got)
	}
}

func TestThrottleHonorsSaneFirstRetryAfter(t *testing.T) {
	b := NewThrottleBook()
	now := time.Unix(1_700_000_000, 0)
	b.now = func() time.Time { return now }
	b.MarkThrottled("m", 5)
	if got := time.UnixMilli(b.Note("m").CooldownUntil).Sub(now); got != 5*time.Second {
		t.Fatalf("first 5s hint parked for %s, want 5s", got)
	}
}

func TestRetireConfirmedCapsAtFifteenMinutes(t *testing.T) {
	b := NewThrottleBook()
	now := time.Unix(1_700_000_000, 0)
	b.now = func() time.Time { return now }
	b.RetireConfirmed("m", now.Add(2*time.Hour))
	n := b.Note("m")
	if !n.Confirmed {
		t.Fatal("RetireConfirmed did not mark the model confirmed")
	}
	if got := time.UnixMilli(n.CooldownUntil).Sub(now); got != time.Duration(throttleConfirmedPark)*time.Millisecond {
		t.Fatalf("confirmed park = %s, want %s", got, time.Duration(throttleConfirmedPark)*time.Millisecond)
	}
	now = now.Add(time.Duration(throttleConfirmedPark)*time.Millisecond + time.Second)
	if b.Throttled("m") {
		t.Fatal("confirmed park never expired")
	}
}

func TestQuotaRetireNeedsThreeSpacedRefusals(t *testing.T) {
	l := NewLane()
	now := time.Now()
	set := func(offsets ...time.Duration) {
		l.quotaFails["m"] = nil
		for _, off := range offsets {
			l.quotaFails["m"] = append(l.quotaFails["m"], now.Add(-off).UnixMilli())
		}
	}
	set(1*time.Minute, 2*time.Minute)
	if l.quotaRetireReady("m") {
		t.Fatal("two refusals were enough to retire")
	}
	set(1*time.Minute, 5*time.Minute, 9*time.Minute)
	if l.quotaRetireReady("m") {
		t.Fatal("three refusals inside the spacing window were enough to retire")
	}
	set(1*time.Minute, 15*time.Minute, 30*time.Minute)
	if !l.quotaRetireReady("m") {
		t.Fatal("three spaced refusals were not enough to retire")
	}
}

func TestProbeConfirmationParksOnlyWithSpacedLiveFailures(t *testing.T) {
	l := NewLane()
	fixed := time.Unix(1_700_000_000, 0)
	l.throttle.now = func() time.Time { return fixed }
	now := fixed
	l.quotaFails["m"] = []int64{
		now.Add(-30 * time.Minute).UnixMilli(),
		now.Add(-15 * time.Minute).UnixMilli(),
		now.Add(-1 * time.Minute).UnixMilli(),
	}
	l.confirmThrottleFromProbe("m", StateThrottled)
	n := l.throttle.Note("m")
	if !n.Confirmed {
		t.Fatal("spaced live failures plus a throttled probe did not confirm the park")
	}
	if got := time.UnixMilli(n.CooldownUntil).Sub(now); got != time.Duration(throttleConfirmedPark)*time.Millisecond {
		t.Fatalf("confirmed park = %s, want %s", got, time.Duration(throttleConfirmedPark)*time.Millisecond)
	}

	// A single live 429 plus the same probe must stay on the ordinary ramp.
	l2 := NewLane()
	l2.throttle.now = func() time.Time { return fixed }
	l2.quotaFails["m"] = []int64{now.UnixMilli()}
	l2.confirmThrottleFromProbe("m", StateThrottled)
	if l2.throttle.Note("m").Confirmed {
		t.Fatal("one live refusal was enough to confirm exhaustion")
	}
}
