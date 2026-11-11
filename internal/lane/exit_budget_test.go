package lane

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExitBudgetChargesPerExit(t *testing.T) {
	b := newExitBudget(60) // 60 tokens/min = 1 token/s
	ctx := context.Background()
	if err := b.Wait(ctx, "a", 60); err != nil {
		t.Fatal(err)
	}
	// A different exit has its own bucket and must not wait for `a`.
	start := time.Now()
	if err := b.Wait(ctx, "b", 60); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("second exit waited %s; buckets are not independent", elapsed)
	}
}

func TestExitBudgetCancellationWhileWaiting(t *testing.T) {
	b := newExitBudget(60)
	if err := b.Wait(context.Background(), "a", 60); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := b.Wait(ctx, "a", 60); err == nil {
		t.Fatal("Wait returned nil while the bucket was empty")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
}

func TestExitBudgetRateDropClampsBurst(t *testing.T) {
	b := newExitBudget(60)
	if err := b.Wait(context.Background(), "a", 1); err != nil {
		t.Fatal(err)
	}
	b.setRate(30)
	if err := b.Wait(context.Background(), "a", 30); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Wait(ctx, "a", 10); err == nil {
		t.Fatal("a lower rate still allowed the old, larger burst")
	}
}

func TestExitBudgetDisabled(t *testing.T) {
	b := newExitBudget(0)
	start := time.Now()
	if err := b.Wait(context.Background(), "a", 1_000_000); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("disabled governor waited %s", elapsed)
	}
}

func TestExitBudgetWaitCappedRefusesWhenRefillIsFar(t *testing.T) {
	b := newExitBudget(60000) // 1000 tokens/s
	if ok, _ := b.Availability("a", 60000); !ok {
		t.Fatal("fresh bucket could not hold its full minute")
	}
	if ok, _ := b.spend("a", 60000); !ok {
		t.Fatal("fresh bucket refused its full minute")
	}
	start := time.Now()
	ok, delay, err := b.WaitCapped(context.Background(), "a", 100, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("WaitCapped reported success with an empty bucket and a 20ms window")
	}
	if delay < 50*time.Millisecond {
		t.Fatalf("reported delay %s, want roughly the 100ms refill", delay)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("WaitCapped held the request %s, want the 20ms cap", elapsed)
	}
}

func TestExitBudgetWaitCappedWaitsWithinWindow(t *testing.T) {
	b := newExitBudget(60000)
	b.spend("a", 60000)
	start := time.Now()
	ok, _, err := b.WaitCapped(context.Background(), "a", 100, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("WaitCapped refused a refill that fits inside the window")
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond || elapsed > 900*time.Millisecond {
		t.Fatalf("WaitCapped took %s, want about 100ms", elapsed)
	}
	if ok, _ := b.Availability("a", 100); ok {
		t.Fatal("successful WaitCapped did not charge the bucket")
	}
}

func TestEstimateRequestTokensReservesOutput(t *testing.T) {
	msgs := []Message{{Role: RoleUser, Parts: []Part{TextPart{Text: "hi"}}}}
	small := estimateRequestTokens(msgs, nil, 10)
	large := estimateRequestTokens(msgs, nil, 1000)
	if large-small < 990 {
		t.Fatalf("max_tokens was not reserved: small=%d large=%d", small, large)
	}
	withTools := estimateRequestTokens(msgs, []ToolDef{{Name: "read", Description: strings.Repeat("d", 400), Parameters: "{}"}}, 10)
	if withTools <= small {
		t.Fatalf("tool definitions did not raise the estimate: %d <= %d", withTools, small)
	}
}

// Five 140k-token turns cannot all fit inside one 300k/minute window, but no
// turn may hold a connection for longer than exitBudgetMaxWait. The virtual
// clock makes the test instantaneous: refused turns must surface as a prompt
// 429 with the remaining delay instead of silently queueing for minutes.
func TestExitBudgetLargeBurstHonoursPerRequestWaitCap(t *testing.T) {
	b := newExitBudget(DefaultExitTokensPerMinute)
	now := time.Unix(1_700_000_000, 0)
	b.now = func() time.Time { return now }
	b.pause = func(_ context.Context, delay time.Duration) error {
		now = now.Add(delay)
		return nil
	}
	successes, refusals := 0, 0
	for i := 0; i < 5; i++ {
		started := now
		ok, wait, err := b.WaitCapped(context.Background(), "exit-a", 140_000, exitBudgetMaxWait)
		if err != nil {
			t.Fatal(err)
		}
		if elapsed := now.Sub(started); elapsed > exitBudgetMaxWait {
			t.Fatalf("turn %d waited %s, over the %s cap", i+1, elapsed, exitBudgetMaxWait)
		}
		if ok {
			successes++
		} else {
			refusals++
			if wait <= 0 {
				t.Fatalf("refused turn %d reported no retry delay", i+1)
			}
		}
	}
	if successes != 3 || refusals != 2 {
		t.Fatalf("successes/refusals = %d/%d, want 3/2 for a 5x140k burst at 300k/min", successes, refusals)
	}
	if exitBudgetMaxWait != 20*time.Second {
		t.Fatalf("exitBudgetMaxWait = %s, want 20s", exitBudgetMaxWait)
	}
}
