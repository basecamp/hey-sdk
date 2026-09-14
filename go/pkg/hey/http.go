package hey

import (
	"context"
	"net/http"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
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

	// MaxRetries is the most times any request is resent (default: 3). A modelled
	// operation is resent as many times as its own retry policy allows and no more; this
	// only lowers that. A GET on a path the caller wrote, which no policy covers, is
	// resent this many times.
	MaxRetries int

	// BaseDelay is the least the client waits before the first resend (default: 1s). A
	// modelled operation whose policy names a longer wait starts from that instead.
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

// WithMaxRetries caps the resends of any request. Every modelled operation carries its
// own retry policy from the API contract — how many sends it gets in all, which statuses
// earn another, and the wait before the first resend — and the client's settings only
// ever make that gentler: an operation is sent at most n+1 times, or as many times as its
// policy allows when that is fewer, and never more than its policy allows; the one resend
// after a 401 that a credential refresh answered is granted on top when those sends are
// already spent. The policy holds on the first request and on every page read after it,
// through the service methods' page parameters and FollowPagination. A GET on a path the caller wrote
// (Get, GetAll) has no policy to bring and is resent n times on the SDK's own list of
// transient failures.
func WithMaxRetries(n int) ClientOption {
	return func(c *Client) {
		c.httpOpts.MaxRetries = n
	}
}

// WithBaseDelay sets the least the client waits before the first resend. A modelled
// operation whose policy names a longer wait starts from that instead; each wait after the
// first is longer by the backoff multiplier.
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

// contextWithAttempt adds the request attempt number to the context. The generated client
// owns the key, since its retry loop marks its own sends and the transport underneath
// both clients reads the one mark.
func contextWithAttempt(ctx context.Context, attempt int) context.Context {
	return generated.ContextWithAttempt(ctx, attempt)
}

// attemptFromContext extracts the attempt number from context (defaults to 1).
func attemptFromContext(ctx context.Context) int {
	return generated.AttemptFromContext(ctx)
}

// projectedRequestKey is the context key marking a request whose URL, on some hop, is
// the credential: the attachment upload's PUT to the signed storage URL, and a blob
// download, which HEY answers with a redirect to a signed storage URL that net/http
// follows on the same context. The hooks see every hop of such a request projected to
// its origin — a storage service can sign the query or the path. An API request's URL
// carries no credential (the token is in the Authorization header), so the hooks see
// it whole.
type projectedRequestKey struct{}

// markProjectedRequest marks ctx as belonging to a request the hooks see projected.
func markProjectedRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, projectedRequestKey{}, true)
}

// isProjectedRequest reports whether ctx carries the projection marker.
func isProjectedRequest(ctx context.Context) bool {
	v, _ := ctx.Value(projectedRequestKey{}).(bool)
	return v
}

// displayURL is url as the hooks and the SDK's own error text show it: whole on an
// API request, its origin alone on a request the transport projects.
func displayURL(ctx context.Context, url string) string {
	if isProjectedRequest(ctx) {
		return projectURL(url, false)
	}
	return url
}

// loggingTransport wraps an http.RoundTripper to log requests and responses,
// and calls observability hooks for all HTTP requests.
type loggingTransport struct {
	inner  http.RoundTripper
	client *Client
}

// RoundTrip implements http.RoundTripper with logging and hooks.
func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	projected := isProjectedRequest(req.Context())
	displayURL := req.URL.String()
	if projected {
		displayURL = projectURL(displayURL, false)
	}
	info := RequestInfo{
		Method:  req.Method,
		URL:     displayURL,
		Attempt: attemptFromContext(req.Context()),
	}
	hookCtx := t.client.hooks.OnRequestStart(req.Context(), info)
	if projected {
		// A hook may hand back a context of its own; the redirect net/http derives
		// from this request must still carry the mark.
		hookCtx = markProjectedRequest(hookCtx)
	}
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
		if projected {
			// A custom transport's failure is text this package cannot vouch for —
			// a *url.Error of its own, or a message interpolating the URL — so the
			// hooks get its classification alone.
			result.Error, _ = classifyFailure(err)
		}
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
