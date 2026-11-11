package lane

import (
	"context"
	"sync"
	"time"
)

// exitBudget is a token-rate governor keyed by egress identity. The upstream
// meters quota per egress IP, not per model: one burst through a single exit
// can exhaust the free window that every model on that IP shares. Spacing the
// work out is cheaper than discovering that with a 429.
//
// The accounting is deliberately approximate. Prompt tokens are estimated from
// character counts and max_tokens is reserved up front, so the governor can
// only over-charge, never under-charge. A setting of 0 disables it entirely.
type exitBudget struct {
	mu      sync.Mutex
	perMin  int
	buckets map[string]*exitBucket
	now     func() time.Time
}

type exitBucket struct {
	tokens float64
	last   time.Time
}

func newExitBudget(perMin int) *exitBudget {
	return &exitBudget{perMin: perMin, buckets: map[string]*exitBucket{}, now: time.Now}
}

// setRate replaces the rate. Buckets keep their balance, clamped to the new
// burst cap, so a live settings change neither drains nor refunds past usage.
func (b *exitBudget) setRate(perMin int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.perMin = perMin
	b.mu.Unlock()
}

// SetExitTokenRate installs the per-exit token-rate governor (tokens per
// minute; <= 0 disables it). The governor is keyed by the rotator's stable
// egress identity, so subscription nodes share a bucket by IP rather than by
// node id — the upstream meters the IP.
func (l *Lane) SetExitTokenRate(perMin int) {
	if perMin <= 0 {
		l.exitBudget.Store(nil)
		return
	}
	if b := l.exitBudget.Load(); b != nil {
		b.setRate(perMin)
		return
	}
	l.exitBudget.CompareAndSwap(nil, newExitBudget(perMin))
}

// Wait blocks until `tokens` may be spent through `exit`. Exit keys are the
// stable egress identities used by the rotator (an IP for subscription nodes);
// the empty key is the direct transport and gets its own lane.
func (b *exitBudget) Wait(ctx context.Context, exit string, tokens int) error {
	if b == nil {
		return nil
	}
	if exit == "" {
		exit = "direct"
	}
	if tokens < 1 {
		tokens = 1
	}
	for {
		b.mu.Lock()
		if b.perMin <= 0 {
			b.mu.Unlock()
			return nil
		}
		ratePerSec := float64(b.perMin) / 60
		capacity := float64(b.perMin)
		if need := float64(tokens); need > capacity {
			// One request larger than a minute's budget must still make
			// progress; it takes the whole bucket and the next request waits
			// for the refill.
			capacity = need
		}
		bucket := b.buckets[exit]
		if bucket == nil {
			bucket = &exitBucket{tokens: capacity, last: b.now()}
			b.buckets[exit] = bucket
		}
		if bucket.tokens > capacity {
			// A live rate reduction lowers the burst cap; do not let an old
			// balance spend above the new ceiling.
			bucket.tokens = capacity
		}
		now := b.now()
		if elapsed := now.Sub(bucket.last).Seconds(); elapsed > 0 {
			bucket.tokens += elapsed * ratePerSec
			if bucket.tokens > capacity {
				bucket.tokens = capacity
			}
			bucket.last = now
		}
		if bucket.tokens >= float64(tokens) {
			bucket.tokens -= float64(tokens)
			b.mu.Unlock()
			return nil
		}
		missing := float64(tokens) - bucket.tokens
		delay := time.Duration(missing / ratePerSec * float64(time.Second))
		b.mu.Unlock()

		if delay < time.Millisecond {
			delay = time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// estimateRequestTokens approximates the prompt and reserves the requested
// output ceiling. Four characters per token is the usual English/code ratio;
// CJK text is denser, which is why the estimate also adds a per-message
// overhead and never trims the max_tokens reservation.
func estimateRequestTokens(messages []Message, tools []ToolDef, maxTokens int) int {
	chars := 0
	for _, m := range messages {
		chars += 8
		for _, p := range m.Parts {
			switch v := p.(type) {
			case TextPart:
				chars += len(v.Text)
			case ReasoningPart:
				chars += len(v.Text)
			case ToolCallPart:
				chars += len(v.Name) + len(v.Arguments)
			case ToolResultPart:
				chars += len(v.Text)
			case ImagePart:
				chars += len(v.DataURL) / 8
			case AudioPart:
				chars += len(v.Data) / 8
			case FilePart:
				chars += len(v.Data) / 8
			}
		}
	}
	for _, t := range tools {
		chars += len(t.Name) + len(t.Description) + len(t.Parameters)
	}
	tokens := chars / 4
	if tokens < 1 {
		tokens = 1
	}
	if maxTokens < 0 {
		maxTokens = 0
	}
	return tokens + maxTokens + 16
}
