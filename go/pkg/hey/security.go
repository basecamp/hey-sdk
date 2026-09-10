package hey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
)

// Response body size limits. JSON and HTML answers are capped in the transport, at the
// client's configured limit (HTTPOptions.MaxResponseBodyBytes, WithMaxResponseBodyBytes;
// DefaultMaxResponseBodyBytes, 16 MiB, by default), before any of these apply.
const (
	// MaxResponseBodyBytes is the most the client buffers of a successful body the
	// transport cap leaves alone — a blob read with GetBlob, an export read with GetCSV, a
	// mutation answered to a */* request, a form or multipart answer — 50 MiB. Only
	// DownloadBlob, which streams to the caller's writer, reads without a bound. A body
	// past it fails with an error wrapping ErrResponseTooLarge.
	//
	// This constant shares its name with the HTTPOptions.MaxResponseBodyBytes field and is
	// not the same limit: the field is the configurable cap on JSON and HTML, this is the
	// fixed bound on everything else. The name predates the field and is kept for
	// compatibility.
	MaxResponseBodyBytes int64 = 50 * 1024 * 1024
	// MaxErrorBodyBytes is the maximum size for error response bodies (1 MB).
	MaxErrorBodyBytes int64 = 1 * 1024 * 1024
	// MaxErrorMessageBytes is the maximum length for error messages included in errors (500 bytes).
	MaxErrorMessageBytes = 500
)

// limitedReadAll reads up to maxBytes from r, and fails with an error wrapping
// ErrResponseTooLarge on a body longer than that.
func limitedReadAll(r io.Reader, maxBytes int64) ([]byte, error) {
	// One byte past the bound is what tells a body over it from one exactly at it. A bound
	// of math.MaxInt64 has no room for that byte, and nothing can exceed it anyway.
	past := maxBytes
	if past < math.MaxInt64 {
		past++
	}
	data, err := io.ReadAll(io.LimitReader(r, past))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w of %d bytes", ErrResponseTooLarge, maxBytes)
	}
	return data, nil
}

// truncateString truncates s to maxLen bytes, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// requireHTTPS validates that the given URL uses the https:// scheme.
func requireHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("URL must use HTTPS: %s", rawURL)
	}
	return nil
}

// isSameOrigin checks whether two absolute URLs share the same scheme and host.
func isSameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	if ua.Scheme == "" || ub.Scheme == "" {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) &&
		strings.EqualFold(normalizeHost(ua), normalizeHost(ub))
}

// resolveURL resolves a possibly-relative URL against a base URL.
func resolveURL(base, target string) string {
	bu, err := url.Parse(base)
	if err != nil {
		return target
	}
	tu, err := url.Parse(target)
	if err != nil {
		return target
	}
	return bu.ResolveReference(tu).String()
}

// normalizeHost returns the host with default ports stripped.
func normalizeHost(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		return host
	}
	if (strings.EqualFold(u.Scheme, "https") && port == "443") ||
		(strings.EqualFold(u.Scheme, "http") && port == "80") {
		return host
	}
	return host + ":" + port
}

// isLocalhost checks if a URL points to localhost (for test environments).
func isLocalhost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()

	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	if strings.HasSuffix(host, ".localhost") {
		return true
	}

	return false
}

// RequireSecureEndpoint validates that an endpoint URL is secure.
func RequireSecureEndpoint(rawURL string) error {
	if isLocalhost(rawURL) {
		return nil
	}
	return requireHTTPS(rawURL)
}

// sensitiveHeaders is the list of headers that should be redacted for logging.
var sensitiveHeaders = []string{
	"Authorization",
	"Cookie",
	"Set-Cookie",
	"X-CSRF-Token",
}

// RedactHeaders returns a copy of the headers with sensitive values replaced by "[REDACTED]".
func RedactHeaders(headers http.Header) http.Header {
	result := headers.Clone()
	for _, key := range sensitiveHeaders {
		if result.Get(key) != "" {
			result.Set(key, "[REDACTED]")
		}
	}
	return result
}

// redactTransportError returns err with the URL Go's *url.Error renders projected to
// its scheme, host and path. net/http reports every transport failure as a *url.Error
// carrying the whole request URL; a signed storage URL carries its credential in the
// query, and a proxy URL its password in the userinfo, so through ErrNetwork that
// rendering would become the hint, the message and every log line printing the error.
// The projection keeps what a reader needs to place the failure and drops the rest
// before any text is built, and walks the whole error tree: a transport built on
// another http.Client nests one *url.Error inside another, and errors.Join holds
// several side by side. An error with no *url.Error whose URL has anything to drop is
// returned as it is.
func redactTransportError(err error) error {
	_, redacted := projectTransportError(err)
	return redacted
}

// projectTransportError rebuilds err's tree with every *url.Error projected, reporting
// whether anything was, so a tree with nothing to drop comes back untouched at every
// level. Beneath a projected URL only the failure's classification survives: whatever
// the transport reported there is text this package did not build — a custom
// transport's own wrapper, a message interpolating the request URL — and cannot be
// shown free of the URL in any spelling, so it is replaced by the context sentinel it
// wraps or a fixed transport failure carrying its net.Error flags. Above one, a
// wrapper is dropped in favour of the projection for the same reason, and a
// multi-error is rebuilt as errors.Join of its projected members.
func projectTransportError(err error) (projected bool, result error) {
	switch e := err.(type) { //nolint:errorlint // rebuilding the tree node by node is the point
	case nil:
		return false, nil
	case *url.Error:
		if projectedURL := redactURL(e.URL); projectedURL != e.URL {
			return true, &url.Error{Op: e.Op, URL: projectedURL, Err: classifyTransportFailure(e)}
		}
		innerProjected, inner := projectTransportError(e.Err)
		if !innerProjected {
			return false, err
		}
		return true, &url.Error{Op: e.Op, URL: e.URL, Err: inner}
	case interface{ Unwrap() []error }:
		members := e.Unwrap()
		rebuilt := make([]error, 0, len(members))
		for _, member := range members {
			memberProjected, projectedMember := projectTransportError(member)
			projected = projected || memberProjected
			rebuilt = append(rebuilt, projectedMember)
		}
		if !projected {
			return false, err
		}
		return true, errors.Join(rebuilt...)
	case interface{ Unwrap() error }:
		causeProjected, projectedCause := projectTransportError(e.Unwrap())
		if !causeProjected {
			return false, err
		}
		return true, projectedCause
	}
	return false, err
}

// classifyTransportFailure is what stands beneath a projected URL in place of the
// transport's own cause: the context sentinel the failure wraps, so errors.Is still
// sees a cancellation or a deadline, or a transportFailureError carrying the
// net.Error flags the *url.Error delegated to that cause.
func classifyTransportFailure(e *url.Error) error {
	switch {
	case errors.Is(e.Err, context.Canceled):
		return context.Canceled
	case errors.Is(e.Err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	}
	return &transportFailureError{timeout: e.Timeout(), temporary: e.Temporary()}
}

// redactURL projects rawURL to its scheme, host and path, dropping userinfo, query
// and fragment. A URL that does not parse projects to the fixed token "unparsable".
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unparsable"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}).String()
}

// transportFailureError is the cause beneath a projected URL: a transport failure
// known only by its net.Error classification.
type transportFailureError struct{ timeout, temporary bool }

func (e *transportFailureError) Error() string {
	if e.timeout {
		return "transport timeout"
	}
	return "transport failure"
}

func (e *transportFailureError) Timeout() bool { return e.timeout }

func (e *transportFailureError) Temporary() bool { return e.temporary }
