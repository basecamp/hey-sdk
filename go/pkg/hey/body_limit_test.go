package hey

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testBodyLimit caps JSON and HTML bodies at a kilobyte, which the tests overrun by a few.
const testBodyLimit int64 = 1024

// newCappedTestClient builds a client against handler with the cap at testBodyLimit and
// the SDK's retries left on, so a refused body is shown not to be retried. The count is how
// many requests the server saw.
func newCappedTestClient(t *testing.T, handler http.HandlerFunc, opts ...ClientOption) (*Client, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	opts = append([]ClientOption{
		WithMaxResponseBodyBytes(testBodyLimit),
		WithMaxRetries(3),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Millisecond),
	}, opts...)
	return NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, opts...), &hits
}

// cappedTransportClient is the transport on its own, for what the Client's own error
// handling would otherwise hide: an error body's length, a blob's Content-Type.
func cappedTransportClient() *http.Client {
	return &http.Client{Transport: &bodyLimitTransport{inner: http.DefaultTransport, limit: testBodyLimit}}
}

func getAccepting(t *testing.T, client *http.Client, url, accept string) (status int, body []byte, err error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", accept)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

// serveOversizedJSON answers with 4 KiB of JSON, four times the test limit, declaring the
// length or streaming it.
func serveOversizedJSON(declare bool) http.HandlerFunc {
	const size = 4096
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if declare {
			w.Header().Set("Content-Length", strconv.Itoa(size))
		}
		flusher, _ := w.(http.Flusher)
		body := `{"content":"` + strings.Repeat("x", size-14) + `"}`
		for len(body) > 0 {
			chunk := min(len(body), 64)
			_, _ = io.WriteString(w, body[:chunk])
			body = body[chunk:]
			if flusher != nil && !declare {
				flusher.Flush()
			}
		}
	}
}

func TestBodyLimitPassesABodyWithinTheLimit(t *testing.T) {
	client, hits := newCappedTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"content":"short"}`)
	})

	resp, err := client.Get(context.Background(), "/messages/1.json")
	if err != nil || string(resp.Data) != `{"id":1,"content":"short"}` {
		t.Fatalf("data = %q, err = %v", resp.Data, err)
	}
	if hits.Load() != 1 {
		t.Errorf("server saw %d requests, want 1", hits.Load())
	}
}

// A body past the limit ends in ErrResponseTooLarge before a parser has it all, whether the
// server declared its length or streamed it — and the request is not retried: the failure is
// at the read, not the round trip, so the retry loop never sees a network error.
func TestBodyLimitStopsAnOversizedBody(t *testing.T) {
	for name, declare := range map[string]bool{"declared": true, "streamed": false} {
		t.Run(name, func(t *testing.T) {
			client, hits := newCappedTestClient(t, serveOversizedJSON(declare))

			_, err := client.Get(context.Background(), "/messages/1.json")
			if !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("err = %v, want ErrResponseTooLarge", err)
			}
			if hits.Load() != 1 {
				t.Errorf("server saw %d requests, want 1: a refused body must not be retried", hits.Load())
			}
		})
	}
}

// The generated parsers are what the cap exists for: they io.ReadAll a body before any
// service method sees it. Their retry loop does not retry the refusal either.
func TestBodyLimitStopsAnOversizedBodyBeforeTheGeneratedParser(t *testing.T) {
	client, hits := newCappedTestClient(t, serveOversizedJSON(false))

	_, err := client.Messages().Get(context.Background(), 1)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge through the generated client", err)
	}
	if hits.Load() != 1 {
		t.Errorf("server saw %d requests, want 1", hits.Load())
	}
}

// A body of exactly the limit is read whole; one byte more is refused.
func TestBodyLimitAcceptsABodyExactlyAtTheLimit(t *testing.T) {
	for _, size := range []int{int(testBodyLimit), int(testBodyLimit) + 1} {
		client, _ := newCappedTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			flusher, _ := w.(http.Flusher)
			_, _ = io.WriteString(w, strings.Repeat("x", size))
			if flusher != nil {
				flusher.Flush()
			}
		})
		resp, err := client.Get(context.Background(), "/messages/1.json")
		switch int64(size) {
		case testBodyLimit:
			if err != nil || int64(len(resp.Data)) != testBodyLimit {
				t.Errorf("exactly at the limit: err = %v, want the whole body", err)
			}
		default:
			if !errors.Is(err, ErrResponseTooLarge) {
				t.Errorf("one past the limit: err = %v, want ErrResponseTooLarge", err)
			}
		}
	}
}

// The limit counts decompressed bytes: a small gzip that inflates past it is refused.
func TestBodyLimitCountsDecompressedBytes(t *testing.T) {
	client, _ := newCappedTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, `{"content":"`+strings.Repeat("a", 8192)+`"}`)
		_ = zw.Close()
	})

	_, err := client.Get(context.Background(), "/messages/1.json")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge for an inflated body", err)
	}
}

// serveErrorJSON answers status with a JSON error body of the given size.
func serveErrorJSON(status, size int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-7f3a")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":"`+strings.Repeat("e", size-12)+`"}`)
	}
}

// An error body past the limit is refused like a success body, but the refusal is the
// status's own *Error with ErrResponseTooLarge as its cause. The generated parsers decode a
// modeled error payload before a service wrapper reaches CheckResponse, so a body merely cut
// off at the cap would reach the caller as a JSON syntax error with no status on it.
func TestBodyLimitRefusesAnErrorBodyWithItsStatus(t *testing.T) {
	client, hits := newCappedTestClient(t, serveErrorJSON(http.StatusUnauthorized, 4096))

	_, err := client.Messages().Get(context.Background(), 1)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want a *Error through the generated client", err, err)
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized || apiErr.Code != CodeAuth || apiErr.RequestID != "req-7f3a" {
		t.Errorf("err = %+v, want the 401's code, status and request id", apiErr)
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("err = %v, want ErrResponseTooLarge as the cause", err)
	}
	if hits.Load() != 1 {
		t.Errorf("server saw %d requests, want 1", hits.Load())
	}

	// A 5xx is the same refusal, retryable as the status is; read through the transport
	// alone, since both clients' retry loops would wait out their backoff on a 500 first.
	server := httptest.NewServer(serveErrorJSON(http.StatusInternalServerError, 4096))
	t.Cleanup(server.Close)
	status, _, err := getAccepting(t, cappedTransportClient(), server.URL, "application/json")
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusInternalServerError || !apiErr.Retryable || !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("status %d, err = %v, want the 500's *Error wrapping ErrResponseTooLarge", status, err)
	}
}

// An error body within the limit reads whole and is handled as it always was: the generated
// parser decodes it and CheckResponse reports the status, with no refusal in sight.
func TestBodyLimitPassesAnErrorBodyWithinTheLimit(t *testing.T) {
	client, _ := newCappedTestClient(t, serveErrorJSON(http.StatusNotFound, 64))

	_, err := client.Messages().Get(context.Background(), 1)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound || apiErr.Code != CodeNotFound {
		t.Fatalf("err = %v, want the 404", err)
	}
	if errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("err = %v, want no refusal for a body within the limit", err)
	}

	// The hand-written path reads what it can of an error body and reports the status
	// either way, within the limit or past it.
	for _, size := range []int{64, 4096} {
		client, _ := newCappedTestClient(t, serveErrorJSON(http.StatusUnprocessableEntity, size))
		_, err := client.Get(context.Background(), "/messages/1.json")
		if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusUnprocessableEntity {
			t.Errorf("%d-byte 422 body: err = %v, want the 422 to stand", size, err)
		}
	}
}

// The largest cap an int64 holds must not overflow the one extra byte a read asks for.
func TestBodyLimitAcceptsTheLargestCap(t *testing.T) {
	client, _ := newCappedTestClient(t, serveOversizedJSON(false), WithMaxResponseBodyBytes(math.MaxInt64))

	resp, err := client.Get(context.Background(), "/messages/1.json")
	if err != nil || len(resp.Data) != 4096 {
		t.Fatalf("err = %v, %d bytes; want the whole body under a cap of math.MaxInt64", err, len(resp.Data))
	}
}

// What is capped is decided by the request. A blob asked for with */* and an export asked
// for with text/csv read whole whatever they turn out to be; a JSON or HTML answer is capped
// whatever the server labels it.
func TestBodyLimitDecidesByTheRequest(t *testing.T) {
	serveLabelled := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", r.URL.Query().Get("type"))
		_, _ = io.WriteString(w, strings.Repeat("%", 4096))
	}

	client, _ := newCappedTestClient(t, serveLabelled)
	ctx := context.Background()
	if resp, err := client.GetBlob(ctx, "/blobs/report.txt?type=text/plain"); err != nil || len(resp.Data) != 4096 {
		t.Errorf("GetBlob: err = %v, want the whole blob", err)
	}
	if resp, err := client.GetCSV(ctx, "/contacts/export?type=text/csv"); err != nil || len(resp.Data) != 4096 {
		t.Errorf("GetCSV: err = %v, want the whole export", err)
	}
	if _, err := client.GetHTML(ctx, "/topics/1?type=text/html"); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("GetHTML: err = %v, want ErrResponseTooLarge", err)
	}
	if _, err := client.Get(ctx, "/messages/1.json?type=image/png"); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("Get labelled as an image: err = %v, want ErrResponseTooLarge", err)
	}

	server := httptest.NewServer(http.HandlerFunc(serveLabelled))
	t.Cleanup(server.Close)
	for _, test := range []struct {
		accept, contentType string
		capped              bool
	}{
		{"*/*", "application/pdf", false},
		{"*/*", "text/plain", false},
		{"*/*", "application/json", false},
		{"text/csv", "text/csv", false},
		{"application/json", "application/json", true},
		{"application/json", "image/png", true},
		{"application/json", "application/octet-stream", true},
		{"application/vnd.api+json", "application/json", true},
		{"text/html", "text/html", true},
		{"text/html, application/json;q=0.9", "text/html", true},
		{"", "application/json", true},
	} {
		_, body, err := getAccepting(t, cappedTransportClient(), server.URL+"?type="+test.contentType, test.accept)
		if capped := errors.Is(err, ErrResponseTooLarge); capped != test.capped {
			t.Errorf("Accept %q, Content-Type %q: capped = %v (err %v, %d bytes), want %v", test.accept, test.contentType, capped, err, len(body), test.capped)
		}
	}
}

// 0 and a negative value both install the default cap: there is no opting out.
func TestBodyLimitOptionDefaults(t *testing.T) {
	for _, configured := range []int64{0, -1} {
		client, _ := newCappedTestClient(t, serveOversizedJSON(false), WithMaxResponseBodyBytes(configured))
		inner := client.httpClient.Transport.(*loggingTransport).inner
		capped, ok := inner.(*bodyLimitTransport)
		if !ok || capped.limit != DefaultMaxResponseBodyBytes {
			t.Errorf("MaxResponseBodyBytes %d: transport %T, want the default cap of %d", configured, inner, DefaultMaxResponseBodyBytes)
		}
	}
}

// What singleRequest buffers is bounded by the request's kind: a parsed answer at the
// configured cap the transport already holds it to, so no second bound refuses it below
// that; anything else at the 50 MiB constant.
func TestBufferBoundFollowsTheRequest(t *testing.T) {
	client, _ := newCappedTestClient(t, serveOversizedJSON(false))
	for accept, want := range map[string]int64{
		"application/json":         testBodyLimit,
		"application/vnd.api+json": testBodyLimit,
		"text/html":                testBodyLimit,
		"":                         testBodyLimit,
		"*/*":                      MaxResponseBodyBytes,
		"text/csv":                 MaxResponseBodyBytes,
	} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://app.hey.com/messages/1.json", nil)
		if err != nil {
			t.Fatal(err)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if got := client.bufferBound(req); got != want {
			t.Errorf("Accept %q: bound %d, want %d", accept, got, want)
		}
	}
}

// A cached body is held to the cap too. The transport never saw it — a client with a
// higher cap, or an older SDK with none, wrote it — so a 304 that would hand it back is
// refused the same way the live answer would have been.
func TestBodyLimitRefusesACachedBodyPastTheCap(t *testing.T) {
	const etag = `"v1"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", etag)
		_, _ = io.WriteString(w, `{"content":"`+strings.Repeat("x", 4096)+`"}`)
	}))
	t.Cleanup(server.Close)

	cache := NewCache(t.TempDir())
	newClient := func(limit int64) *Client {
		return NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
			WithMaxRetries(0), WithCache(cache), WithMaxResponseBodyBytes(limit))
	}
	if resp, err := newClient(1<<20).Get(context.Background(), "/messages/1.json"); err != nil || resp.FromCache {
		t.Fatalf("seeding the cache: err = %v, from cache %v", err, resp != nil && resp.FromCache)
	}

	_, err := newClient(testBodyLimit).Get(context.Background(), "/messages/1.json")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge for a cached body past the cap", err)
	}
}
