package hey

import (
	"context"
	"time"
)

// Hooks provides observability callbacks for SDK operations.
type Hooks interface {
	OnOperationStart(ctx context.Context, op OperationInfo) context.Context
	OnOperationEnd(ctx context.Context, op OperationInfo, err error, duration time.Duration)
	OnRequestStart(ctx context.Context, info RequestInfo) context.Context
	OnRequestEnd(ctx context.Context, info RequestInfo, result RequestResult)
	OnRetry(ctx context.Context, info RequestInfo, attempt int, err error)
}

// GatingHooks extends Hooks with request gating capability.
type GatingHooks interface {
	Hooks
	OnOperationGate(ctx context.Context, op OperationInfo) (context.Context, error)
}

// RequestInfo contains information about an HTTP request.
type RequestInfo struct {
	Method  string
	URL     string
	Attempt int
}

// OperationInfo describes a semantic SDK operation.
type OperationInfo struct {
	Service      string
	Operation    string
	ResourceType string
	IsMutation   bool
	ResourceID   int64
}

// RequestResult contains the result of an HTTP request.
type RequestResult struct {
	StatusCode int
	Duration   time.Duration
	Error      error
	FromCache  bool
	Retryable  bool
	RetryAfter int
}

// NoopHooks is a no-op implementation of Hooks.
type NoopHooks struct{}

var _ Hooks = NoopHooks{}

func (NoopHooks) OnOperationStart(ctx context.Context, _ OperationInfo) context.Context { return ctx }
func (NoopHooks) OnOperationEnd(context.Context, OperationInfo, error, time.Duration)   {}
func (NoopHooks) OnRequestStart(ctx context.Context, _ RequestInfo) context.Context     { return ctx }
func (NoopHooks) OnRequestEnd(context.Context, RequestInfo, RequestResult)              {}
func (NoopHooks) OnRetry(context.Context, RequestInfo, int, error)                      {}

// ChainHooks combines multiple Hooks implementations.
type ChainHooks struct {
	hooks []Hooks
}

// NewChainHooks creates a ChainHooks from the given hooks.
func NewChainHooks(hooks ...Hooks) Hooks {
	filtered := make([]Hooks, 0, len(hooks))
	for _, h := range hooks {
		if h != nil {
			if _, isNoop := h.(NoopHooks); !isNoop {
				filtered = append(filtered, h)
			}
		}
	}
	if len(filtered) == 0 {
		return NoopHooks{}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &ChainHooks{hooks: filtered}
}

func (c *ChainHooks) OnOperationStart(ctx context.Context, op OperationInfo) context.Context {
	for _, h := range c.hooks {
		ctx = h.OnOperationStart(ctx, op)
	}
	return ctx
}

func (c *ChainHooks) OnOperationEnd(ctx context.Context, op OperationInfo, err error, duration time.Duration) {
	for i := len(c.hooks) - 1; i >= 0; i-- {
		c.hooks[i].OnOperationEnd(ctx, op, err, duration)
	}
}

func (c *ChainHooks) OnRequestStart(ctx context.Context, info RequestInfo) context.Context {
	for _, h := range c.hooks {
		ctx = h.OnRequestStart(ctx, info)
	}
	return ctx
}

func (c *ChainHooks) OnRequestEnd(ctx context.Context, info RequestInfo, result RequestResult) {
	for i := len(c.hooks) - 1; i >= 0; i-- {
		c.hooks[i].OnRequestEnd(ctx, info, result)
	}
}

func (c *ChainHooks) OnRetry(ctx context.Context, info RequestInfo, attempt int, err error) {
	for _, h := range c.hooks {
		h.OnRetry(ctx, info, attempt, err)
	}
}

func (c *ChainHooks) OnOperationGate(ctx context.Context, op OperationInfo) (context.Context, error) {
	for _, h := range c.hooks {
		if gater, ok := h.(GatingHooks); ok {
			return gater.OnOperationGate(ctx, op)
		}
	}
	return ctx, nil
}

// WithHooks sets the observability hooks for the client.
func WithHooks(hooks Hooks) ClientOption {
	return func(c *Client) {
		if hooks == nil {
			c.hooks = NoopHooks{}
		} else {
			c.hooks = hooks
		}
	}
}
