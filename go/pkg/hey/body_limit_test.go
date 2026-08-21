package hey

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
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

// An error body past the limit is cut off at the limit, not refused: the status is what
// matters about an error, and a read failure would hide it.
func TestBodyLimitTruncatesErrorBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"`+strings.Repeat("e", 4096)+`"}`)
	}))
	t.Cleanup(server.Close)

	status, body, err := getAccepting(t, cappedTransportClient(), server.URL, "application/json")
	if err != nil {
		t.Fatalf("err = %v, want the error body read without a refusal", err)
	}
	if status != http.StatusInternalServerError || int64(len(body)) != testBodyLimit {
		t.Fatalf("status %d, %d bytes; want the status with the body cut at the limit", status, len(body))
	}

	// Through the client, the status is what the caller gets.
	client, _ := newCappedTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":"`+strings.Repeat("e", 4096)+`"}`)
	})
	_, err = client.Get(context.Background(), "/messages/1.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusUnprocessableEntity || errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want the 422 to stand", err)
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

// 0 installs the default cap; a negative value installs none.
func TestBodyLimitOptionDefaultsAndOptsOut(t *testing.T) {
	defaulted, _ := newCappedTestClient(t, serveOversizedJSON(false), WithMaxResponseBodyBytes(0))
	capped, ok := defaulted.httpClient.Transport.(*loggingTransport).inner.(*bodyLimitTransport)
	if !ok || capped.limit != DefaultMaxResponseBodyBytes {
		t.Fatalf("MaxResponseBodyBytes 0: transport %T, want the default cap of %d", defaulted.httpClient.Transport.(*loggingTransport).inner, DefaultMaxResponseBodyBytes)
	}

	uncapped, _ := newCappedTestClient(t, serveOversizedJSON(false), WithMaxResponseBodyBytes(-1))
	if _, ok := uncapped.httpClient.Transport.(*loggingTransport).inner.(*bodyLimitTransport); ok {
		t.Fatal("MaxResponseBodyBytes -1: a cap was installed")
	}
	if resp, err := uncapped.Get(context.Background(), "/messages/1.json"); err != nil || len(resp.Data) != 4096 {
		t.Fatalf("uncapped: err = %v, want the whole body", err)
	}
}
