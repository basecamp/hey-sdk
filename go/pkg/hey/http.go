package hey

import (
	"context"
	"net/http"
	"time"
)

// Default values for HTTP client configuration.
const (
	DefaultMaxRetries = 3
	DefaultBaseDelay  = 1 * time.Second
	DefaultMaxJitter  = 100 * time.Millisecond
	DefaultTimeout    = 30 * time.Second
	DefaultMaxPages   = 10000

	// DefaultMaxResponseBodyBytes is the most a JSON or HTML response may deliver, in
	// decompressed bytes: 16 MiB, which is a message with a very large HTML body several
	// times over, and small enough that a server answering one page with gigabytes is
	// refused long before it exhausts memory.
	DefaultMaxResponseBodyBytes int64 = 16 << 20
)

// HTTPOptions configures the HTTP client behavior.
type HTTPOptions struct {
	// Timeout is the request timeout (default: 30s).
	Timeout time.Duration

	// MaxRetries is the maximum retry attempts for GET requests (default: 3).
	MaxRetries int

	// BaseDelay is the initial backoff delay (default: 1s).
	BaseDelay time.Duration

	// MaxJitter is the maximum random jitter to add to delays (default: 100ms).
	MaxJitter time.Duration

	// MaxPages is the maximum pages to fetch in GetAll (default: 10000).
	MaxPages int

	// Transport is the HTTP transport to use. If nil, a default transport
	// with sensible connection pooling is created.
	Transport http.RoundTripper

	// MaxResponseBodyBytes is the most a JSON or HTML response body may deliver, in
	// decompressed bytes, before its read fails with an error wrapping ErrResponseTooLarge —
	// success and error responses alike; a refused error body still carries its status. 0
	// or a negative value means DefaultMaxResponseBodyBytes: the cap is always installed.
	// The transport applies it; a client built with WithHTTPClient keeps it only on Get and
	// GetHTML, which bound their own buffering at the same number, not on the service
	// methods.
	//
	// Blob (*/*) and CSV answers are not capped here: GetBlob and GetCSV buffer under the
	// 50 MiB MaxResponseBodyBytes constant instead, and only DownloadBlob streams without
	// a bound.
	MaxResponseBodyBytes int64
}

// DefaultHTTPOptions returns HTTPOptions with sensible defaults.
func DefaultHTTPOptions() HTTPOptions {
	return HTTPOptions{
		Timeout:              DefaultTimeout,
		MaxRetries:           DefaultMaxRetries,
		BaseDelay:            DefaultBaseDelay,
		MaxJitter:            DefaultMaxJitter,
		MaxPages:             DefaultMaxPages,
		MaxResponseBodyBytes: DefaultMaxResponseBodyBytes,
	}
}

// responseBodyLimit is the cap NewClient installs: the configured MaxResponseBodyBytes, or
// the default when that is 0 or negative. There is no opting out — a consumer that wants
// more raises the cap.
func (o HTTPOptions) responseBodyLimit() int64 {
	if o.MaxResponseBodyBytes <= 0 {
		return DefaultMaxResponseBodyBytes
	}
	return o.MaxResponseBodyBytes
}

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpOpts.Timeout = d
	}
}

// WithMaxRetries sets the maximum number of retry attempts for GET requests.
func WithMaxRetries(n int) ClientOption {
	return func(c *Client) {
		c.httpOpts.MaxRetries = n
	}
}

// WithBaseDelay sets the initial backoff delay.
func WithBaseDelay(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpOpts.BaseDelay = d
	}
}

// WithMaxJitter sets the maximum random jitter to add to delays.
func WithMaxJitter(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpOpts.MaxJitter = d
	}
}

// WithMaxPages sets the maximum pages to fetch in GetAll.
func WithMaxPages(n int) ClientOption {
	return func(c *Client) {
		c.httpOpts.MaxPages = n
	}
}

// WithTransport sets a custom HTTP transport. The SDK still wraps it with its own
// response body cap, logging and hooks; WithHTTPClient is the option that replaces all of
// that.
func WithTransport(t http.RoundTripper) ClientOption {
	return func(c *Client) {
		c.httpOpts.Transport = t
	}
}

// WithMaxResponseBodyBytes sets the most a JSON or HTML response body may deliver, in
// decompressed bytes, before its read fails with an error wrapping ErrResponseTooLarge. 0
// or a negative value restores DefaultMaxResponseBodyBytes; the cap cannot be removed.
func WithMaxResponseBodyBytes(n int64) ClientOption {
	return func(c *Client) {
		c.httpOpts.MaxResponseBodyBytes = n
	}
}

// retryableError wraps an error with retry metadata.
type retryableError struct {
	err        error
	retryAfter time.Duration
}

func (r *retryableError) Error() string {
	return r.err.Error()
}

func (r *retryableError) Unwrap() error {
	return r.err
}

// newDefaultTransport creates an HTTP transport with sensible defaults.
func newDefaultTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = 90 * time.Second
	return t
}

// attemptKey is the context key for tracking request attempt number.
type attemptKey struct{}

// contextWithAttempt adds the request attempt number to the context.
func contextWithAttempt(ctx context.Context, attempt int) context.Context {
	return context.WithValue(ctx, attemptKey{}, attempt)
}

// attemptFromContext extracts the attempt number from context (defaults to 1).
func attemptFromContext(ctx context.Context) int {
	if v := ctx.Value(attemptKey{}); v != nil {
		if attempt, ok := v.(int); ok {
			return attempt
		}
	}
	return 1
}

// loggingTransport wraps an http.RoundTripper to log requests and responses,
// and calls observability hooks for all HTTP requests.
type loggingTransport struct {
	inner  http.RoundTripper
	client *Client
}

// RoundTrip implements http.RoundTripper with logging and hooks.
func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	info := RequestInfo{
		Method:  req.Method,
		URL:     req.URL.String(),
		Attempt: attemptFromContext(req.Context()),
	}
	hookCtx := t.client.hooks.OnRequestStart(req.Context(), info)
	startTime := time.Now()

	req = req.WithContext(hookCtx)

	var result RequestResult
	defer func() {
		result.Duration = time.Since(startTime)
		t.client.hooks.OnRequestEnd(hookCtx, info, result)
	}()

	if t.client.logger != nil {
		t.client.logger.Debug("http request", "method", req.Method)
	}

	resp, err := t.inner.RoundTrip(req)

	if err != nil {
		result.Error = err
	} else {
		result.StatusCode = resp.StatusCode
		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			result.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
		}
		if t.client.logger != nil {
			t.client.logger.Debug("http response",
				"status", resp.StatusCode)
		}
	}

	return resp, err
}
