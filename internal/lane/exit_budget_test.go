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
