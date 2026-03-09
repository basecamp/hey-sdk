package hey

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCircuitBreaker_StartsOpen(t *testing.T) {
	cb := newCircuitBreaker(DefaultCircuitBreakerConfig())
	if !cb.Allow() {
		t.Fatal("expected circuit breaker to allow requests initially (closed state)")
	}
	if cb.State() != "closed" {
		t.Fatalf("expected closed state, got %q", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cfg := &CircuitBreakerConfig{
		FailureThreshold:     3,
		SuccessThreshold:     2,
		OpenTimeout:          1 * time.Second,
		FailureRateThreshold: 50,
		SlidingWindowSize:    10,
	}
	cb := newCircuitBreaker(cfg)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != "open" {
		t.Fatalf("expected open state after %d failures, got %q", 3, cb.State())
	}
	if cb.Allow() {
		t.Fatal("expected circuit breaker to reject requests when open")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	now := time.Now()
	cfg := &CircuitBreakerConfig{
		FailureThreshold:     2,
		SuccessThreshold:     1,
		OpenTimeout:          100 * time.Millisecond,
		FailureRateThreshold: 50,
		SlidingWindowSize:    10,
		Now:                  func() time.Time { return now },
	}
	cb := newCircuitBreaker(cfg)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != "open" {
		t.Fatal("expected open")
	}

	// Advance time past the open timeout
	now = now.Add(200 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("expected circuit breaker to allow (half-open) after timeout")
	}
	if cb.State() != "half-open" {
		t.Fatalf("expected half-open, got %q", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccessInHalfOpen(t *testing.T) {
	now := time.Now()
	cfg := &CircuitBreakerConfig{
		FailureThreshold:     2,
		SuccessThreshold:     2,
		OpenTimeout:          100 * time.Millisecond,
		FailureRateThreshold: 50,
		SlidingWindowSize:    10,
		Now:                  func() time.Time { return now },
	}
	cb := newCircuitBreaker(cfg)

	cb.RecordFailure()
	cb.RecordFailure()

	now = now.Add(200 * time.Millisecond)
	cb.Allow() // transitions to half-open

	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != "closed" {
		t.Fatalf("expected closed after success threshold, got %q", cb.State())
	}
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	now := time.Now()
	cfg := &CircuitBreakerConfig{
		FailureThreshold:     2,
		SuccessThreshold:     2,
		OpenTimeout:          100 * time.Millisecond,
		FailureRateThreshold: 50,
		SlidingWindowSize:    10,
		Now:                  func() time.Time { return now },
	}
	cb := newCircuitBreaker(cfg)

	cb.RecordFailure()
	cb.RecordFailure()

	now = now.Add(200 * time.Millisecond)
	cb.Allow()

	cb.RecordFailure()
	if cb.State() != "open" {
		t.Fatalf("expected open after failure in half-open, got %q", cb.State())
	}
}

func TestCircuitBreaker_FailureRate(t *testing.T) {
	cfg := &CircuitBreakerConfig{
		FailureThreshold:     100, // high so count-based won't trigger
		SuccessThreshold:     2,
		OpenTimeout:          1 * time.Second,
		FailureRateThreshold: 50,
		SlidingWindowSize:    4,
	}
	cb := newCircuitBreaker(cfg)

	// Fill window: 2 successes + 2 failures = 50% failure rate
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordFailure() // window now filled, 50% failure rate

	// Next failure should check the rate
	cb.RecordFailure()

	if cb.State() != "open" {
		t.Fatalf("expected open due to failure rate, got %q", cb.State())
	}
}

func TestBulkhead_Acquire(t *testing.T) {
	bh := newBulkhead(&BulkheadConfig{MaxConcurrent: 2, MaxWait: 0})

	r1, err := bh.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	r2, err := bh.Acquire(context.Background())
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}

	// Third should fail (no wait)
	_, err = bh.Acquire(context.Background())
	if err != ErrBulkheadFull {
		t.Fatalf("expected ErrBulkheadFull, got %v", err)
	}

	if bh.InUse() != 2 {
		t.Fatalf("expected 2 in use, got %d", bh.InUse())
	}
	if bh.Available() != 0 {
		t.Fatalf("expected 0 available, got %d", bh.Available())
	}

	// Release one
	r1()

	if bh.InUse() != 1 {
		t.Fatalf("expected 1 in use after release, got %d", bh.InUse())
	}

	// Should succeed now
	r3, err := bh.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	r2()
	r3()
}

func TestBulkhead_TryAcquire(t *testing.T) {
	bh := newBulkhead(&BulkheadConfig{MaxConcurrent: 1, MaxWait: 0})

	release, ok := bh.TryAcquire()
	if !ok {
		t.Fatal("expected TryAcquire to succeed")
	}

	_, ok = bh.TryAcquire()
	if ok {
		t.Fatal("expected TryAcquire to fail when full")
	}

	release()
}

func TestBulkhead_AcquireWithWait(t *testing.T) {
	bh := newBulkhead(&BulkheadConfig{MaxConcurrent: 1, MaxWait: 100 * time.Millisecond})

	release, _ := bh.Acquire(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		release()
	}()

	r2, err := bh.Acquire(context.Background())
	if err != nil {
		t.Fatalf("expected acquire to succeed after wait, got %v", err)
	}
	r2()
}

func TestBulkhead_AcquireTimeout(t *testing.T) {
	bh := newBulkhead(&BulkheadConfig{MaxConcurrent: 1, MaxWait: 10 * time.Millisecond})

	release, _ := bh.Acquire(context.Background())
	defer release()

	_, err := bh.Acquire(context.Background())
	if err != ErrBulkheadFull {
		t.Fatalf("expected ErrBulkheadFull after timeout, got %v", err)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         5,
		Now:               func() time.Time { return now },
	})

	// Should allow up to burst size
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Fatalf("expected Allow on request %d", i+1)
		}
	}

	// Next should fail (no time passed)
	if rl.Allow() {
		t.Fatal("expected rate limit after burst")
	}

	// Advance time enough for 1 token
	now = now.Add(20 * time.Millisecond) // 100 rps => 1 token per 10ms, so 20ms = 2 tokens
	if !rl.Allow() {
		t.Fatal("expected Allow after time advance")
	}
}

func TestRateLimiter_RespectRetryAfter(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1000,
		BurstSize:         100,
		RespectRetryAfter: true,
		Now:               func() time.Time { return now },
	})

	rl.SetRetryAfter(now.Add(1 * time.Second))

	if rl.Allow() {
		t.Fatal("expected block during Retry-After period")
	}

	remaining := rl.RetryAfterRemaining()
	if remaining <= 0 {
		t.Fatal("expected positive remaining duration")
	}

	// Advance past retry-after
	now = now.Add(2 * time.Second)

	if !rl.Allow() {
		t.Fatal("expected Allow after Retry-After expired")
	}

	if rl.RetryAfterRemaining() != 0 {
		t.Fatal("expected 0 remaining after expiry")
	}
}

func TestRateLimiter_Tokens(t *testing.T) {
	rl := newRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         10,
	})

	tokens := rl.Tokens()
	if tokens != 10 {
		t.Fatalf("expected 10 initial tokens, got %f", tokens)
	}

	rl.Allow()
	tokens = rl.Tokens()
	if tokens >= 10 {
		t.Fatal("expected tokens to decrease after Allow")
	}
}

func TestRateLimiter_SetRetryAfterDuration(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1000,
		BurstSize:         100,
		RespectRetryAfter: true,
		Now:               func() time.Time { return now },
	})

	rl.SetRetryAfterDuration(5 * time.Second)
	if rl.Allow() {
		t.Fatal("expected block during retry-after duration")
	}

	now = now.Add(6 * time.Second)
	if !rl.Allow() {
		t.Fatal("expected allow after retry-after duration")
	}
}

func TestShouldTripCircuit(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"circuit open", ErrCircuitOpen, false},
		{"bulkhead full", ErrBulkheadFull, false},
		{"rate limited", ErrRateLimited, false},
		{"context canceled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"network error", ErrNetwork(fmt.Errorf("connection refused")), true},
		{"server 500", &Error{Code: CodeAPI, HTTPStatus: 500}, true},
		{"server 503", &Error{Code: CodeAPI, HTTPStatus: 503}, true},
		{"client 400", &Error{Code: CodeAPI, HTTPStatus: 400}, false},
		{"auth error", &Error{Code: CodeAuth, HTTPStatus: 401}, false},
		{"unknown error", context.DeadlineExceeded, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTripCircuit(tc.err)
			if got != tc.want {
				t.Fatalf("shouldTripCircuit(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDefaultResilienceConfig(t *testing.T) {
	cfg := DefaultResilienceConfig()
	if cfg.CircuitBreaker == nil {
		t.Fatal("expected non-nil CircuitBreaker config")
	}
	if cfg.Bulkhead == nil {
		t.Fatal("expected non-nil Bulkhead config")
	}
	if cfg.RateLimit == nil {
		t.Fatal("expected non-nil RateLimit config")
	}
}

func TestWithResilience(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"}, WithResilience(nil))
	if _, ok := c.hooks.(*resilienceHooks); !ok {
		t.Fatal("expected resilienceHooks after WithResilience")
	}
}

func TestWithCircuitBreaker(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"}, WithCircuitBreaker(nil))
	rh, ok := c.hooks.(*resilienceHooks)
	if !ok {
		t.Fatal("expected resilienceHooks")
	}
	if rh.circuitBreakers == nil {
		t.Fatal("expected circuit breakers initialized")
	}
}

func TestWithBulkhead(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"}, WithBulkhead(nil))
	rh, ok := c.hooks.(*resilienceHooks)
	if !ok {
		t.Fatal("expected resilienceHooks")
	}
	if rh.bulkheads == nil {
		t.Fatal("expected bulkheads initialized")
	}
}

func TestWithRateLimit(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"}, WithRateLimit(nil))
	rh, ok := c.hooks.(*resilienceHooks)
	if !ok {
		t.Fatal("expected resilienceHooks")
	}
	if rh.rateLimiter == nil {
		t.Fatal("expected rate limiter initialized")
	}
}

func TestCircuitBreakerRegistry(t *testing.T) {
	reg := newCircuitBreakerRegistry(DefaultCircuitBreakerConfig())

	cb1 := reg.get("scope1")
	cb2 := reg.get("scope2")
	cb3 := reg.get("scope1")

	if cb1 == cb2 {
		t.Fatal("expected different circuit breakers for different scopes")
	}
	if cb1 != cb3 {
		t.Fatal("expected same circuit breaker for same scope")
	}
}

func TestBulkheadRegistry(t *testing.T) {
	reg := newBulkheadRegistry(DefaultBulkheadConfig())

	bh1 := reg.get("scope1")
	bh2 := reg.get("scope2")
	bh3 := reg.get("scope1")

	if bh1 == bh2 {
		t.Fatal("expected different bulkheads for different scopes")
	}
	if bh1 != bh3 {
		t.Fatal("expected same bulkhead for same scope")
	}
}
