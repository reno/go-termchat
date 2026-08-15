package chat_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/chat"
)

var start = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

func testRate() chat.Rate {
	return chat.Rate{Burst: 3, Refill: time.Second, Idle: time.Minute}
}

func TestLimiterAllowsTheBurstThenRefuses(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())

	for i := range 3 {
		if err := limiter.Allow("s1", start); err != nil {
			t.Fatalf("call %d within the burst was refused: %v", i, err)
		}
	}
	if err := limiter.Allow("s1", start); err == nil {
		t.Fatal("the call past the burst was allowed")
	} else if chat.ReasonOf(err) != chat.ReasonRateLimited {
		t.Errorf("reason = %s, want rate limited", chat.ReasonOf(err))
	}
}

func TestLimiterReportsAPositiveRetryAfter(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())
	for range 3 {
		if err := limiter.Allow("s1", start); err != nil {
			t.Fatalf("burst refused: %v", err)
		}
	}

	err := limiter.Allow("s1", start)
	var domain *chat.Error
	if !asDomainError(err, &domain) {
		t.Fatalf("not a domain error: %v", err)
	}
	if domain.RetryAfter <= 0 || domain.RetryAfter > time.Second {
		t.Errorf("RetryAfter = %s, want a positive delay no larger than the refill", domain.RetryAfter)
	}
}

func TestLimiterRefillsOneTokenPerInterval(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())
	for range 3 {
		if err := limiter.Allow("s1", start); err != nil {
			t.Fatalf("burst refused: %v", err)
		}
	}

	if err := limiter.Allow("s1", start.Add(999*time.Millisecond)); err == nil {
		t.Error("a token appeared before the refill interval elapsed")
	}
	if err := limiter.Allow("s1", start.Add(time.Second)); err != nil {
		t.Errorf("no token after one refill interval: %v", err)
	}
	if err := limiter.Allow("s1", start.Add(time.Second)); err == nil {
		t.Error("one interval yielded more than one token")
	}
}

func TestLimiterRefillIsCappedAtTheBurst(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())
	if err := limiter.Allow("s1", start); err != nil {
		t.Fatalf("first call refused: %v", err)
	}

	idle := start.Add(time.Hour)
	for i := range 3 {
		if err := limiter.Allow("s1", idle); err != nil {
			t.Fatalf("call %d after a long idle was refused: %v", i, err)
		}
	}
	if err := limiter.Allow("s1", idle); err == nil {
		t.Error("a long idle banked more than the burst")
	}
}

func TestLimiterKeepsSessionsApart(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())
	for range 3 {
		if err := limiter.Allow("s1", start); err != nil {
			t.Fatalf("burst refused: %v", err)
		}
	}
	if err := limiter.Allow("s1", start); err == nil {
		t.Fatal("s1 was not limited")
	}
	if err := limiter.Allow("s2", start); err != nil {
		t.Errorf("s2 was caught by s1's limit: %v", err)
	}
}

func TestForgetResetsASession(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())
	for range 3 {
		if err := limiter.Allow("s1", start); err != nil {
			t.Fatalf("burst refused: %v", err)
		}
	}

	limiter.Forget("s1")

	if err := limiter.Allow("s1", start); err != nil {
		t.Errorf("Forget did not reset the bucket: %v", err)
	}
}

func TestSweepReclaimsBucketsWhoseRemovalChangesNothing(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(testRate())

	if err := limiter.Allow("light", start); err != nil {
		t.Fatalf("light session refused: %v", err)
	}
	for range 3 {
		if err := limiter.Allow("heavy", start); err != nil {
			t.Fatalf("heavy session refused: %v", err)
		}
	}

	if removed := limiter.Sweep(start.Add(time.Second)); removed != 0 {
		t.Errorf("swept %d buckets while both were recently active", removed)
	}

	if removed := limiter.Sweep(start.Add(2 * time.Minute)); removed != 2 {
		t.Errorf("swept %d buckets, want both: each has refilled to its burst, so forgetting "+
			"them grants nothing a fresh bucket would not", removed)
	}
}

func TestSweepDoesNotForgiveAStillLimitedSession(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(chat.Rate{Burst: 3, Refill: time.Hour, Idle: time.Minute})
	for range 3 {
		if err := limiter.Allow("heavy", start); err != nil {
			t.Fatalf("burst refused: %v", err)
		}
	}

	if removed := limiter.Sweep(start.Add(2 * time.Minute)); removed != 0 {
		t.Errorf("swept %d buckets that still owe tokens, handing back a full burst", removed)
	}
	if err := limiter.Allow("heavy", start.Add(2*time.Minute)); err == nil {
		t.Error("the sweep reset a session that was still rate limited")
	}
}

func TestLimiterIsSafeUnderConcurrency(t *testing.T) {
	t.Parallel()

	limiter := chat.NewRateLimiter(chat.Rate{Burst: 100, Refill: time.Hour, Idle: time.Hour})

	const workers, each = 10, 20

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for range each {
				if err := limiter.Allow("shared", start); err == nil {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if allowed != 100 {
		t.Errorf("allowed %d calls, want exactly the burst of 100", allowed)
	}
}

func asDomainError(err error, target **chat.Error) bool {
	return errors.As(err, target)
}
