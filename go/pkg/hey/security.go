package hey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
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
// requireHTTPS rejects a URL whose scheme is not https. The error names the URL's
// origin alone: HEY's direct-upload URL is signed, and the rejection is what a caller
// logs.
func requireHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		var parseErr *url.Error
		if errors.As(err, &parseErr) {
			err = parseErr.Err
		}
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("URL must use HTTPS: %s", describeOrigin(rawURL))
	}
	return nil
}

// describeOrigin is rawURL's origin for an error's text, or a fixed token for a URL
// with neither scheme nor host.
func describeOrigin(rawURL string) string {
	if origin := projectURL(rawURL, false); origin != "" {
		return origin
	}
	return "a URL with no scheme"
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

// redactTransportError returns err with every URL Go's *url.Error renders projected.
// net/http reports every transport failure as a *url.Error carrying the whole request
// URL, and a URL off the SDK's API origin can carry a credential anywhere a storage
// service puts one — a signed query, a token in the path, a password in the userinfo
// — so through ErrNetwork that rendering would become the hint, the message and every
// log line printing the error. The projection keeps what a reader needs to place the
// failure and drops the rest before any text is built, and walks the whole error
// tree: a transport built on another http.Client nests one *url.Error inside another,
// and errors.Join holds several side by side. An error with nothing to drop is
// returned as it is.
//
// apiOrigin is the SDK's own API origin, or empty. A URL on it, carrying no userinfo,
// is trusted: its path stays — the token rides in the Authorization header, and the
// query is paging and filtering — and beneath it the transport's cause is kept, and
// its text with it, as the failure's diagnostic. Every other URL is projected to its
// origin, and beneath it only the failure's classification survives.
func redactTransportError(err error, apiOrigin string) error {
	_, redacted := projectTransportError(err, apiOrigin)
	return redacted
}

// projectTransportError rebuilds err's tree with every *url.Error projected, reporting
// whether anything was, so a tree with nothing to drop comes back untouched at every
// level. A projected *url.Error off the trusted origin is built from fixed parts
// alone: its Op kept only as the method token net/http writes there, its URL as the
// origin, its cause replaced by what classifies the failure — the context sentinels
// it wrapped and its net.Error flags — because whatever a transport put there is text
// this package did not build (a custom transport's own wrapper, a message
// interpolating the request URL) and cannot be shown free of the URL in any spelling.
// Around a projected URL the same holds: an ancestor keeps only that Op token, a
// wrapper is dropped in favour of the projection, and a multi-error is rebuilt as
// errors.Join of its projected members and their sentinel siblings, the opaque
// siblings dropped.
func projectTransportError(err error, apiOrigin string) (projected bool, result error) {
	switch e := err.(type) { //nolint:errorlint // rebuilding the tree node by node is the point
	case nil:
		return false, nil
	case *url.Error:
		trusted := trustedURL(e.URL, apiOrigin)
		projectedURL := projectURL(e.URL, trusted)
		if !trusted {
			stand, _ := classifyFailure(e)
			return true, &url.Error{Op: transportOp(e.Op), URL: projectedURL, Err: stand}
		}
		innerProjected, inner := projectTransportError(e.Err, apiOrigin)
		if projectedURL == e.URL && !innerProjected {
			return false, err
		}
		return true, &url.Error{Op: transportOp(e.Op), URL: projectedURL, Err: inner}
	case interface{ Unwrap() []error }:
		members := e.Unwrap()
		kept := make([]error, 0, len(members))
		for _, member := range members {
			if memberProjected, projectedMember := projectTransportError(member, apiOrigin); memberProjected {
				projected = true
				kept = append(kept, projectedMember)
			} else if stand, classified := classifyFailure(member); classified {
				kept = append(kept, stand)
			}
		}
		if !projected {
			return false, err
		}
		return true, errors.Join(kept...)
	case interface{ Unwrap() error }:
		causeProjected, projectedCause := projectTransportError(e.Unwrap(), apiOrigin)
		if !causeProjected {
			return false, err
		}
		return true, projectedCause
	}
	// An error can expose a *url.Error through an As method without unwrapping to it;
	// its own rendering is then text this package cannot vouch for, and the transport
	// error it exposes is what is kept, projected.
	var exposed *url.Error
	if errors.As(err, &exposed) {
		_, projectedExposed := projectTransportError(exposed, apiOrigin)
		return true, projectedExposed
	}
	return false, err
}

// trustedURL reports whether rawURL is on apiOrigin and carries no userinfo: a URL
// whose path and query are the API's own, with no credential anywhere in it.
func trustedURL(rawURL, apiOrigin string) bool {
	if apiOrigin == "" || !isSameOrigin(rawURL, apiOrigin) {
		return false
	}
	u, err := url.Parse(rawURL)
	return err == nil && u.User == nil
}

// projectURL projects rawURL to its origin — scheme and host — or, for a trusted URL,
// to its scheme, host and path. A URL that does not parse projects to the fixed token
// "unparsable".
func projectURL(rawURL string, keepPath bool) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unparsable"
	}
	projected := &url.URL{Scheme: u.Scheme, Host: u.Host}
	if keepPath {
		projected.Path, projected.RawPath = u.Path, u.RawPath
	}
	return projected.String()
}

// transportOp is op as net/http writes it — the request method, letters alone — or
// the fixed "Request" when a transport put anything else there.
func transportOp(op string) string {
	for _, r := range op {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return "Request"
		}
	}
	if op == "" {
		return "Request"
	}
	return op
}

// classifyFailure is what stands in for a transport failure whose text this package
// cannot vouch for: a transportFailureError carrying the net.Error flags the failure
// reports — a *url.Error delegates them to its immediate cause, as before — and
// unwrapping to the context sentinels it wrapped, so errors.Is still sees a
// cancellation or a deadline. classified reports whether the failure had any of those
// to carry; beneath a projected URL the stand-in is kept either way.
func classifyFailure(err error) (stand *transportFailureError, classified bool) {
	stand = &transportFailureError{sentinel: contextSentinels(err)}
	var netErr net.Error
	if errors.As(err, &netErr) {
		stand.timeout, stand.temporary = netErr.Timeout(), netErr.Temporary()
		classified = true
	}
	return stand, classified || stand.sentinel != nil
}

// contextSentinels is every bare context sentinel err wraps — one, both joined, or
// nil.
func contextSentinels(err error) error {
	var sentinels []error
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, sentinel) {
			sentinels = append(sentinels, sentinel)
		}
	}
	return errors.Join(sentinels...)
}

// transportFailureError is the cause beneath a projected URL: a transport failure
// known only by its net.Error classification and the context sentinels it wrapped.
type transportFailureError struct {
	timeout, temporary bool
	sentinel           error
}

func (e *transportFailureError) Error() string {
	switch {
	case e.sentinel != nil:
		return e.sentinel.Error()
	case e.timeout:
		return "transport timeout"
	}
	return "transport failure"
}

func (e *transportFailureError) Unwrap() error { return e.sentinel }

func (e *transportFailureError) Timeout() bool { return e.timeout }

func (e *transportFailureError) Temporary() bool { return e.temporary }
