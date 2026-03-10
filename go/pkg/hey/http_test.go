package hey

import (
	"testing"
	"time"
)

func TestDefaultHTTPOptions(t *testing.T) {
	opts := DefaultHTTPOptions()

	if opts.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", opts.Timeout)
	}
	if opts.MaxRetries != 3 {
		t.Fatalf("expected 3 retries, got %d", opts.MaxRetries)
	}
	if opts.BaseDelay != 1*time.Second {
		t.Fatalf("expected 1s base delay, got %v", opts.BaseDelay)
	}
	if opts.MaxJitter != 100*time.Millisecond {
		t.Fatalf("expected 100ms max jitter, got %v", opts.MaxJitter)
	}
	if opts.MaxPages != 10000 {
		t.Fatalf("expected 10000 max pages, got %d", opts.MaxPages)
	}
}

func TestRetryableError(t *testing.T) {
	inner := ErrRateLimit(30)
	re := &retryableError{err: inner, retryAfter: 30 * time.Second}

	if re.Error() != inner.Error() {
		t.Fatalf("expected %q, got %q", inner.Error(), re.Error())
	}
	if re.Unwrap() != inner {
		t.Fatal("expected unwrap to return inner error")
	}
}

func TestContextWithAttempt(t *testing.T) {
	ctx := contextWithAttempt(t.Context(), 3)
	got := attemptFromContext(ctx)
	if got != 3 {
		t.Fatalf("expected attempt 3, got %d", got)
	}
}

func TestAttemptFromContext_Default(t *testing.T) {
	got := attemptFromContext(t.Context())
	if got != 1 {
		t.Fatalf("expected default attempt 1, got %d", got)
	}
}

func TestClientOptions(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}

	c := NewClient(cfg, &StaticTokenProvider{Token: "t"},
		WithTimeout(10*time.Second),
		WithMaxRetries(5),
		WithBaseDelay(2*time.Second),
		WithMaxJitter(50*time.Millisecond),
		WithMaxPages(100),
	)

	if c.httpOpts.Timeout != 10*time.Second {
		t.Fatalf("expected 10s timeout, got %v", c.httpOpts.Timeout)
	}
	if c.httpOpts.MaxRetries != 5 {
		t.Fatalf("expected 5 retries, got %d", c.httpOpts.MaxRetries)
	}
	if c.httpOpts.BaseDelay != 2*time.Second {
		t.Fatalf("expected 2s base delay, got %v", c.httpOpts.BaseDelay)
	}
	if c.httpOpts.MaxJitter != 50*time.Millisecond {
		t.Fatalf("expected 50ms jitter, got %v", c.httpOpts.MaxJitter)
	}
	if c.httpOpts.MaxPages != 100 {
		t.Fatalf("expected 100 maxPages, got %d", c.httpOpts.MaxPages)
	}
}
