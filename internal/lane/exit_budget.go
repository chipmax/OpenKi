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
	// pause is the clock seam used by the burst-cap tests. Production always
	// gets sleepCtx; nil falls back to it for zero-value budgets.
	pause func(context.Context, time.Duration) error
}

type exitBucket struct {
	tokens float64
	last   time.Time
}

func newExitBudget(perMin int) *exitBudget {
	return &exitBudget{perMin: perMin, buckets: map[string]*exitBucket{}, now: time.Now, pause: sleepCtx}
}

func (b *exitBudget) waitFor(ctx context.Context, delay time.Duration) error {
	if b.pause == nil {
		return sleepCtx(ctx, delay)
	}
	return b.pause(ctx, delay)
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

// DefaultExitTokensPerMinute is the governor's rate when config.json has no
// exitTokensPerMinute key. 300k/min is roughly one large coding turn every
// minute per exit: enough headroom for normal use, low enough that a burst
// cannot silently spend a whole window on one IP. An explicit 0 disables it.
const DefaultExitTokensPerMinute = 300_000

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
	for {
		ok, delay := b.spend(exit, tokens)
		if ok {
			return nil
		}
		if err := b.waitFor(ctx, delay); err != nil {
			return err
		}
	}
}

// WaitCapped waits at most maxWait for `tokens` to become affordable. It
// returns ok=false with the remaining delay when the bucket cannot be charged
// inside the window, so the caller can answer 429 + Retry-After instead of
// holding a request open forever. The wait is capped because an exit that is
// minutes away from refilling is no better than a refusal to the caller.
func (b *exitBudget) WaitCapped(ctx context.Context, exit string, tokens int, maxWait time.Duration) (bool, time.Duration, error) {
	if b == nil {
		return true, 0, nil
	}
	if maxWait < 0 {
		maxWait = 0
	}
	deadline := b.now().Add(maxWait)
	for {
		ok, delay := b.spend(exit, tokens)
		if ok {
			return true, 0, nil
		}
		if maxWait == 0 {
			return false, delay, nil
		}
		remaining := deadline.Sub(b.now())
		if remaining <= 0 {
			return false, delay, nil
		}
		sleep := delay
		if sleep > remaining {
			sleep = remaining
		}
		if sleep < time.Millisecond {
			sleep = time.Millisecond
		}
		if err := b.waitFor(ctx, sleep); err != nil {
			return false, delay, err
		}
	}
}

// Availability reports whether a spend would succeed right now and, if not,
// how long the refill would take. It refills the bucket but does not charge it,
// so a caller can compare exits before committing the reservation.
func (b *exitBudget) Availability(exit string, tokens int) (bool, time.Duration) {
	if b == nil {
		return true, 0
	}
	return b.try(exit, tokens, false)
}

func (b *exitBudget) spend(exit string, tokens int) (bool, time.Duration) {
	if b == nil {
		return true, 0
	}
	return b.try(exit, tokens, true)
}

func (b *exitBudget) try(exit string, tokens int, charge bool) (bool, time.Duration) {
	if exit == "" {
		exit = "direct"
	}
	if tokens < 1 {
		tokens = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.perMin <= 0 {
		return true, 0
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
		if charge {
			bucket.tokens -= float64(tokens)
		}
		return true, 0
	}
	missing := float64(tokens) - bucket.tokens
	delay := time.Duration(missing / ratePerSec * float64(time.Second))
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	return false, delay
}

func sleepCtx(ctx context.Context, delay time.Duration) error {
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
		return nil
	}
}

// exitBudgetMaxWait is how long a request may be held for its exit's token
// refill before the lane answers 429. Beyond this the client is better served
// by an explicit Retry-After than by a connection that looks hung.
const (
	exitBudgetMaxWait     = 20 * time.Second
	exitBudgetRetryCapSec = 3600
)

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
