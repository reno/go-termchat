package chat

import (
	"sync"
	"time"
)

type Rate struct {
	Burst  int
	Refill time.Duration
	Idle   time.Duration
}

func DefaultRate() Rate {
	return Rate{
		Burst:  10,
		Refill: 2 * time.Second,
		Idle:   5 * time.Minute,
	}
}

type bucket struct {
	tokens   int
	last     time.Time
	lastSeen time.Time
}

func (b *bucket) refill(rate Rate, now time.Time) {
	if gained := int(now.Sub(b.last) / rate.Refill); gained > 0 {
		b.tokens = min(b.tokens+gained, rate.Burst)
		b.last = b.last.Add(time.Duration(gained) * rate.Refill)
	}
	if b.tokens >= rate.Burst {
		b.last = now
	}
}

type RateLimiter struct {
	mu      sync.Mutex
	rate    Rate
	buckets map[string]*bucket
}

func NewRateLimiter(rate Rate) *RateLimiter {
	return &RateLimiter{rate: rate, buckets: make(map[string]*bucket)}
}

func (l *RateLimiter) Allow(key string, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, known := l.buckets[key]
	if !known {
		b = &bucket{tokens: l.rate.Burst, last: now}
		l.buckets[key] = b
	}

	b.refill(l.rate, now)
	b.lastSeen = now

	if b.tokens <= 0 {
		wait := l.rate.Refill - now.Sub(b.last)
		if wait <= 0 {
			wait = l.rate.Refill
		}
		return &Error{Reason: ReasonRateLimited, RetryAfter: wait}
	}

	b.tokens--
	return nil
}

func (l *RateLimiter) Forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.buckets, key)
}

func (l *RateLimiter) Sweep(now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	removed := 0
	for key, b := range l.buckets {
		b.refill(l.rate, now)
		if b.tokens >= l.rate.Burst && now.Sub(b.lastSeen) > l.rate.Idle {
			delete(l.buckets, key)
			removed++
		}
	}
	return removed
}
