package hey

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ResilienceConfig combines all resilience settings.
type ResilienceConfig struct {
	CircuitBreaker *CircuitBreakerConfig
	Bulkhead       *BulkheadConfig
	RateLimit      *RateLimitConfig
}

// DefaultResilienceConfig returns production-ready defaults.
func DefaultResilienceConfig() *ResilienceConfig {
	return &ResilienceConfig{
		CircuitBreaker: DefaultCircuitBreakerConfig(),
		Bulkhead:       DefaultBulkheadConfig(),
		RateLimit:      DefaultRateLimitConfig(),
	}
}

type resilienceHooks struct {
	inner           Hooks
	circuitBreakers *circuitBreakerRegistry
	bulkheads       *bulkheadRegistry
	rateLimiter     *rateLimiter

	releaseCounter  atomic.Uint64
	pendingReleases sync.Map
	activeReleases  sync.Map
}

var _ GatingHooks = (*resilienceHooks)(nil)

type bulkheadPendingKey struct{}
type bulkheadActiveKey struct{}

func shouldTripCircuit(err error) bool {
	if err == ErrCircuitOpen || err == ErrBulkheadFull || err == ErrRateLimited {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if e, ok := err.(*Error); ok {
		if e.Code == CodeNetwork {
			return true
		}
		if e.HTTPStatus >= 500 {
			return true
		}
		return false
	}

	return true
}

func (h *resilienceHooks) OnOperationGate(ctx context.Context, op OperationInfo) (context.Context, error) {
	scope := op.Service + "." + op.Operation

	if h.circuitBreakers != nil {
		cb := h.circuitBreakers.get(scope)
		if !cb.Allow() {
			return ctx, ErrCircuitOpen
		}
	}

	if h.bulkheads != nil {
		bh := h.bulkheads.get(scope)
		release, err := bh.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx, ctx.Err()
			}
			return ctx, ErrBulkheadFull
		}
		pendingID := h.releaseCounter.Add(1)
		h.pendingReleases.Store(pendingID, release)
		ctx = context.WithValue(ctx, bulkheadPendingKey{}, pendingID)
	}

	if h.rateLimiter != nil {
		if !h.rateLimiter.Allow() {
			if pendingID, ok := ctx.Value(bulkheadPendingKey{}).(uint64); ok {
				if release, loaded := h.pendingReleases.LoadAndDelete(pendingID); loaded {
					release.(func())()
				}
			}
			return ctx, ErrRateLimited
		}
	}

	return ctx, nil
}

func (h *resilienceHooks) OnOperationStart(ctx context.Context, op OperationInfo) context.Context {
	resultCtx := h.inner.OnOperationStart(ctx, op)

	if pendingID, ok := ctx.Value(bulkheadPendingKey{}).(uint64); ok {
		if release, loaded := h.pendingReleases.LoadAndDelete(pendingID); loaded {
			h.activeReleases.Store(pendingID, release)
			resultCtx = context.WithValue(resultCtx, bulkheadActiveKey{}, pendingID)
		}
	}

	return resultCtx
}

func (h *resilienceHooks) OnOperationEnd(ctx context.Context, op OperationInfo, err error, duration time.Duration) {
	scope := op.Service + "." + op.Operation

	if releaseID, ok := ctx.Value(bulkheadActiveKey{}).(uint64); ok {
		if release, loaded := h.activeReleases.LoadAndDelete(releaseID); loaded {
			release.(func())()
		}
	}

	if h.circuitBreakers != nil {
		cb := h.circuitBreakers.get(scope)
		if err != nil && shouldTripCircuit(err) {
			cb.RecordFailure()
		} else if err == nil {
			cb.RecordSuccess()
		}
	}

	h.inner.OnOperationEnd(ctx, op, err, duration)
}

func (h *resilienceHooks) OnRequestStart(ctx context.Context, info RequestInfo) context.Context {
	return h.inner.OnRequestStart(ctx, info)
}

func (h *resilienceHooks) OnRequestEnd(ctx context.Context, info RequestInfo, result RequestResult) {
	if h.rateLimiter != nil {
		if result.StatusCode == 429 {
			retryAfter := result.RetryAfter
			if retryAfter <= 0 {
				retryAfter = 60
			}
			h.rateLimiter.SetRetryAfterDuration(time.Duration(retryAfter) * time.Second)
		} else if result.StatusCode == 503 && result.RetryAfter > 0 {
			h.rateLimiter.SetRetryAfterDuration(time.Duration(result.RetryAfter) * time.Second)
		}
	}

	h.inner.OnRequestEnd(ctx, info, result)
}

func (h *resilienceHooks) OnRetry(ctx context.Context, info RequestInfo, attempt int, err error) {
	h.inner.OnRetry(ctx, info, attempt, err)
}

// WithResilience enables circuit breaker, bulkhead, and rate limiting.
func WithResilience(cfg *ResilienceConfig) ClientOption {
	return func(c *Client) {
		if cfg == nil {
			cfg = DefaultResilienceConfig()
		}

		rh := &resilienceHooks{
			inner: c.hooks,
		}

		if cfg.CircuitBreaker != nil {
			rh.circuitBreakers = newCircuitBreakerRegistry(cfg.CircuitBreaker)
		}
		if cfg.Bulkhead != nil {
			rh.bulkheads = newBulkheadRegistry(cfg.Bulkhead)
		}
		if cfg.RateLimit != nil {
			rh.rateLimiter = newRateLimiter(cfg.RateLimit)
		}

		c.hooks = rh
	}
}

// WithCircuitBreaker enables only the circuit breaker.
func WithCircuitBreaker(cfg *CircuitBreakerConfig) ClientOption {
	return func(c *Client) {
		if cfg == nil {
			cfg = DefaultCircuitBreakerConfig()
		}
		rh := &resilienceHooks{
			inner:           c.hooks,
			circuitBreakers: newCircuitBreakerRegistry(cfg),
		}
		c.hooks = rh
	}
}

// WithBulkhead enables only the bulkhead (concurrency limiter).
func WithBulkhead(cfg *BulkheadConfig) ClientOption {
	return func(c *Client) {
		if cfg == nil {
			cfg = DefaultBulkheadConfig()
		}
		rh := &resilienceHooks{
			inner:     c.hooks,
			bulkheads: newBulkheadRegistry(cfg),
		}
		c.hooks = rh
	}
}

// WithRateLimit enables only client-side rate limiting.
func WithRateLimit(cfg *RateLimitConfig) ClientOption {
	return func(c *Client) {
		if cfg == nil {
			cfg = DefaultRateLimitConfig()
		}
		rh := &resilienceHooks{
			inner:       c.hooks,
			rateLimiter: newRateLimiter(cfg),
		}
		c.hooks = rh
	}
}
