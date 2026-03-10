package hey

import (
	"context"
	"testing"
	"time"
)

func TestNoopHooks(t *testing.T) {
	var h Hooks = NoopHooks{}

	ctx := context.Background()
	op := OperationInfo{Service: "Test", Operation: "Op"}
	info := RequestInfo{Method: "GET", URL: "https://example.com"}
	result := RequestResult{StatusCode: 200}

	// All methods should be safe to call (no panics)
	ctx = h.OnOperationStart(ctx, op)
	h.OnOperationEnd(ctx, op, nil, time.Second)
	ctx = h.OnRequestStart(ctx, info)
	h.OnRequestEnd(ctx, info, result)
	h.OnRetry(ctx, info, 2, nil)

	if ctx == nil {
		t.Fatal("expected non-nil context from NoopHooks")
	}
}

type recordingHook struct {
	starts []string
	ends   []string
}

func (h *recordingHook) OnOperationStart(ctx context.Context, op OperationInfo) context.Context {
	h.starts = append(h.starts, op.Service+"."+op.Operation)
	return ctx
}
func (h *recordingHook) OnOperationEnd(_ context.Context, op OperationInfo, _ error, _ time.Duration) {
	h.ends = append(h.ends, op.Service+"."+op.Operation)
}
func (h *recordingHook) OnRequestStart(ctx context.Context, _ RequestInfo) context.Context {
	return ctx
}
func (h *recordingHook) OnRequestEnd(context.Context, RequestInfo, RequestResult) {}
func (h *recordingHook) OnRetry(context.Context, RequestInfo, int, error)         {}

func TestChainHooks(t *testing.T) {
	h1 := &recordingHook{}
	h2 := &recordingHook{}

	chain := NewChainHooks(h1, h2)

	ctx := context.Background()
	op := OperationInfo{Service: "Svc", Operation: "Do"}

	ctx = chain.OnOperationStart(ctx, op)
	chain.OnOperationEnd(ctx, op, nil, time.Second)

	// OnOperationStart called in order
	if len(h1.starts) != 1 || h1.starts[0] != "Svc.Do" {
		t.Fatal("expected h1 start recorded")
	}
	if len(h2.starts) != 1 || h2.starts[0] != "Svc.Do" {
		t.Fatal("expected h2 start recorded")
	}

	// OnOperationEnd called in reverse order
	if len(h1.ends) != 1 {
		t.Fatal("expected h1 end recorded")
	}
	if len(h2.ends) != 1 {
		t.Fatal("expected h2 end recorded")
	}
}

func TestChainHooks_FiltersNoops(t *testing.T) {
	h1 := &recordingHook{}
	chain := NewChainHooks(h1, NoopHooks{}, nil)

	// Should only have the real hook, so returns it directly (not ChainHooks)
	if _, ok := chain.(*ChainHooks); ok {
		t.Fatal("expected single hook unwrapped, not ChainHooks")
	}
}

func TestChainHooks_AllNoops(t *testing.T) {
	chain := NewChainHooks(NoopHooks{}, nil)
	if _, ok := chain.(NoopHooks); !ok {
		t.Fatal("expected NoopHooks when all are no-ops")
	}
}

func TestNewChainHooks_Empty(t *testing.T) {
	chain := NewChainHooks()
	if _, ok := chain.(NoopHooks); !ok {
		t.Fatal("expected NoopHooks for empty chain")
	}
}

func TestWithHooks_Nil(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"}, WithHooks(nil))
	if _, ok := c.hooks.(NoopHooks); !ok {
		t.Fatal("expected NoopHooks for nil hooks option")
	}
}
