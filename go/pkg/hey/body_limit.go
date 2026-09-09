package hey

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// The generated parsers read every response body with io.ReadAll before a service method
// can apply a budget, so a server answering one entries page or one message with gigabytes
// was a memory exhaustion for every consumer of the SDK. The bound lives in the transport
// NewClient builds, where it covers every request the root client, an account-scoped client
// and the generated client make: a RoundTripper that caps what a text-bearing response can
// deliver, success and error alike, before a parser sees it.
//
// A body past the limit is refused — its first read past the cap fails with an error that
// wraps ErrResponseTooLarge — since a parser that buffers it would buffer it whole. An error
// body is refused the same way, but the refusal is the *Error CheckResponse would have built
// for the status, with the refusal as its Cause: the generated parsers decode a modeled error
// payload before a service wrapper reaches CheckResponse, so a body merely cut off at the
// cap would surface as a JSON syntax error and the status would be lost with it. Both happen
// at read time rather than at the round trip: a round-trip error is one the retry loops, the
// SDK's and the generated client's, treat as a network failure and try again, and the body
// would be too large again.
//
// Which responses are capped is decided by the request, not by what the server says it
// answered with: the SDK asks for application/json where a generated parser or doRequest
// will buffer the answer and for text/html where GetHTML will, and asks for */* for a blob,
// which it streams to a destination of any size (DownloadBlob) or buffers under its own
// MaxResponseBodyBytes (GetBlob), and for text/csv for an export. A server that labels a
// JSON answer as a PNG is still capped; an attachment that happens to be a text file is
// still streamed.

// bodyLimitTransport wraps an http.RoundTripper so that JSON and HTML responses cannot
// deliver more than limit decompressed bytes.
type bodyLimitTransport struct {
	inner http.RoundTripper
	limit int64
}

// RoundTrip implements http.RoundTripper.
func (t *bodyLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || !isParsedRequest(req) {
		return resp, err
	}
	remaining := t.limit
	if resp.ContentLength > t.limit {
		// Declared past the limit: refused on its first read, the same failure a streamed
		// body produces once it passes the cap, so neither retry loop sees a round-trip error.
		remaining = -1
	}
	body := &cappedBody{
		ReadCloser: resp.Body,
		remaining:  remaining,
		declared:   resp.ContentLength,
		limit:      t.limit,
		request:    req,
	}
	if resp.StatusCode >= 400 {
		// The status is what matters about an error; the refusal carries it.
		body.status, _ = CheckResponse(resp).(*Error)
	}
	resp.Body = body
	return resp, nil
}

// isParsedRequest reports a request whose answer the SDK buffers and parses, by what the
// request asked for. Anything it did not ask for as JSON or HTML — a blob's */*, an export's
// text/csv — it handles by streaming or under its own bound.
func isParsedRequest(req *http.Request) bool {
	accept := req.Header.Get("Accept")
	if accept == "" {
		return true
	}
	for part := range strings.SplitSeq(accept, ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") || mediaType == "text/html" {
			return true
		}
	}
	return false
}

// cappedBody is a body that delivers up to the limit and fails on the first byte past it.
type cappedBody struct {
	io.ReadCloser
	remaining int64
	declared  int64
	limit     int64
	request   *http.Request
	// status is the *Error CheckResponse built for an error response, which the refusal
	// then carries; nil for a success.
	status *Error
}

// Read delivers up to the limit and fails on the first byte past it. A body that is exactly
// the limit is read whole: with nothing remaining, the next read still asks the wrapped body
// for one byte, and gets its EOF rather than a refusal. A body declared past the limit
// starts with a negative remainder and fails on its first read.
func (b *cappedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining < 0 {
		return 0, b.refuse(fmt.Errorf("%s %s: Content-Length %d: %w of %d bytes",
			b.request.Method, b.request.URL.Path, b.declared, ErrResponseTooLarge, b.limit))
	}
	// Ask for one byte past what remains, so a body exactly at the limit reads to its EOF
	// and one byte longer is caught. Compare before adding: a limit of math.MaxInt64 has
	// no room for the extra byte.
	if b.remaining < int64(len(p)) {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	if int64(n) > b.remaining {
		n = int(b.remaining)
		b.remaining = 0
		return n, b.refuse(fmt.Errorf("%s %s: %w of %d bytes", b.request.Method, b.request.URL.Path, ErrResponseTooLarge, b.limit))
	}
	b.remaining -= int64(n)
	return n, err
}

// refuse is the error a read past the limit ends with: the refusal itself for a success,
// and for an error response the status's own *Error with the refusal as its Cause, so
// errors.As finds the status and errors.Is finds ErrResponseTooLarge.
func (b *cappedBody) refuse(cause error) error {
	if b.status == nil {
		return cause
	}
	refused := *b.status
	refused.Cause = cause
	if refused.Hint == "" {
		refused.Hint = cause.Error()
	}
	return &refused
}
