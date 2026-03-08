package hey

import (
	"context"
	"sync"
	"time"
)

// RateLimitConfig configures client-side rate limiting.
type RateLimitConfig struct {
	RequestsPerSecond float64
	BurstSize         int
	RespectRetryAfter bool
	Now               func() time.Time
}

// DefaultRateLimitConfig returns production-ready defaults.
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerSecond: 50,
		BurstSize:         10,
		RespectRetryAfter: true,
	}
}

type rateLimiter struct {
	config *RateLimitConfig

	mu             sync.Mutex
	tokens         float64
	lastRefillTime time.Time

	retryAfterUntil time.Time
}

func newRateLimiter(config *RateLimitConfig) *rateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 50
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 10
	}

	now := time.Now()
	if config.Now != nil {
		now = config.Now()
	}

	return &rateLimiter{
		config:         config,
		tokens:         float64(config.BurstSize),
		lastRefillTime: now,
	}
}

func (r *rateLimiter) now() time.Time {
	if r.config.Now != nil {
		return r.config.Now()
	}
	return time.Now()
}

func (r *rateLimiter) refill() {
	now := r.now()
	elapsed := now.Sub(r.lastRefillTime)
	r.lastRefillTime = now

	tokensToAdd := elapsed.Seconds() * r.config.RequestsPerSecond
	r.tokens += tokensToAdd

	if r.tokens > float64(r.config.BurstSize) {
		r.tokens = float64(r.config.BurstSize)
	}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.RespectRetryAfter && !r.retryAfterUntil.IsZero() {
		if r.now().Before(r.retryAfterUntil) {
			return false
		}
		r.retryAfterUntil = time.Time{}
	}

	r.refill()

	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

func (r *rateLimiter) Wait(ctx context.Context) error {
	for {
		r.mu.Lock()

		if r.config.RespectRetryAfter && !r.retryAfterUntil.IsZero() {
			waitUntil := r.retryAfterUntil
			if r.now().Before(waitUntil) {
				r.mu.Unlock()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(waitUntil.Sub(r.now())):
					continue
				}
			}
			r.retryAfterUntil = time.Time{}
		}

		r.refill()

		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}

		tokensNeeded := 1 - r.tokens
		waitDuration := time.Duration(tokensNeeded/r.config.RequestsPerSecond*float64(time.Second)) + time.Millisecond

		r.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}
}

func (r *rateLimiter) Reserve() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.RespectRetryAfter && !r.retryAfterUntil.IsZero() {
		if r.now().Before(r.retryAfterUntil) {
			return -1
		}
		r.retryAfterUntil = time.Time{}
	}

	r.refill()

	if r.tokens >= 1 {
		r.tokens--
		return 0
	}

	tokensNeeded := 1 - r.tokens
	waitDuration := time.Duration(tokensNeeded / r.config.RequestsPerSecond * float64(time.Second))

	if waitDuration > time.Second {
		return -1
	}

	r.tokens--
	return waitDuration
}

func (r *rateLimiter) SetRetryAfter(until time.Time) {
	if !r.config.RespectRetryAfter {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if until.After(r.retryAfterUntil) {
		r.retryAfterUntil = until
	}
}

func (r *rateLimiter) SetRetryAfterDuration(d time.Duration) {
	r.SetRetryAfter(r.now().Add(d))
}

func (r *rateLimiter) Tokens() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refill()
	return r.tokens
}

func (r *rateLimiter) RetryAfterRemaining() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.retryAfterUntil.IsZero() {
		return 0
	}

	remaining := r.retryAfterUntil.Sub(r.now())
	if remaining < 0 {
		return 0
	}
	return remaining
}
