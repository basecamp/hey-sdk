package hey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// renderings is every text a caller or a logger can get out of err: its Error, its
// %+v, and the Error of every link of its cause chain.
func renderings(err error) []string {
	out := []string{err.Error(), fmt.Sprintf("%+v", err)}
	for cause := errors.Unwrap(err); cause != nil; cause = errors.Unwrap(cause) {
		out = append(out, cause.Error(), fmt.Sprintf("%+v", cause))
	}
	return out
}

type requestRecordingHooks struct {
	NoopHooks
	infos   []RequestInfo
	results []RequestResult
	fresh   bool // hand back a context of the hook's own, not derived from the request's
}

func (h *requestRecordingHooks) OnRequestStart(ctx context.Context, info RequestInfo) context.Context {
	h.infos = append(h.infos, info)
	if h.fresh {
		return context.Background()
	}
	return ctx
}

func (h *requestRecordingHooks) OnRequestEnd(_ context.Context, _ RequestInfo, result RequestResult) {
	h.results = append(h.results, result)
}

// TestAttachmentsUploadTransportErrorRendersNoSignedURL forces a dial failure on the
// storage hop of an attachment upload — the one request the SDK issues to a URL whose
// query is the credential — and checks the signature reaches neither the error's
// renderings nor the request hooks. net/http's *url.Error carries the whole URL, and
// ErrNetwork used to copy it into the hint verbatim.
func TestAttachmentsUploadTransportErrorRendersNoSignedURL(t *testing.T) {
	const signedURL = "http://127.0.0.1:1/blob?signature=SECRETVALUE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"signed_id":"signed-123",
			"attachable_sgid":"sgid-456",
			"direct_upload":{"url":"` + signedURL + `","headers":{"Content-Type":"text/plain"}}
		}`))
	}))
	t.Cleanup(server.Close)
	hooks := &requestRecordingHooks{}
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithHooks(hooks))

	_, err := client.Attachments().Upload(context.Background(), "note.txt", "text/plain", strings.NewReader("contents"))
	if err == nil {
		t.Fatal("expected a network error dialing a closed port")
	}
	var sdkErr *Error
	if !errors.As(err, &sdkErr) || sdkErr.Code != CodeNetwork {
		t.Fatalf("expected a network *Error, got %T: %v", err, err)
	}
	for _, text := range append(renderings(err), sdkErr.Hint, sdkErr.Message) {
		if strings.Contains(text, "SECRETVALUE") {
			t.Errorf("the signed query leaked into %q", text)
		}
	}
	if !strings.Contains(sdkErr.Hint, `"http://127.0.0.1:1"`) {
		t.Errorf("hint should keep the URL's scheme, host and path, got %q", sdkErr.Hint)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr.URL != "http://127.0.0.1:1" {
		t.Errorf("the cause chain should still classify as the projected *url.Error, got %v", err)
	}

	if len(hooks.infos) != 2 || len(hooks.results) != 2 {
		t.Fatalf("expected the API request and the storage request in the hooks, got %d starts, %d ends", len(hooks.infos), len(hooks.results))
	}
	if storage := hooks.infos[1]; storage.Method != http.MethodPut || storage.URL != "http://127.0.0.1:1" {
		t.Errorf("storage request reached the hooks as %s %q, want the projected URL", storage.Method, storage.URL)
	}
	if api := hooks.infos[0]; !strings.HasPrefix(api.URL, server.URL+"/") {
		t.Errorf("API request URL should reach the hooks whole, got %q", api.URL)
	}
	for _, result := range hooks.results {
		if result.Error != nil && strings.Contains(result.Error.Error(), "SECRETVALUE") {
			t.Errorf("the signed query leaked into a hook result: %q", result.Error.Error())
		}
	}
}

// leakyStorageTransport is a custom transport that reports the storage request's
// failure the way a transport built on another http.Client would: as a *url.Error of
// its own, rendering the whole URL.
type leakyStorageTransport struct {
	inner http.RoundTripper
	plain bool // report a plain error interpolating the URL rather than a *url.Error
}

func (t *leakyStorageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPut {
		if t.plain {
			return nil, fmt.Errorf("PUT %s failed", req.URL)
		}
		return nil, &url.Error{Op: "Put", URL: req.URL.String(), Err: errors.New("connection refused")}
	}
	return t.inner.RoundTrip(req)
}

// TestAttachmentsUploadCustomTransportErrorRendersNoSignedURL is the same check with a
// custom transport whose own error carries the signed URL: the hook result and the
// SDK error are projected all the same.
func TestAttachmentsUploadCustomTransportErrorRendersNoSignedURL(t *testing.T) {
	for name, plain := range map[string]bool{"url.Error": false, "plain error": true} {
		t.Run(name, func(t *testing.T) { testAttachmentsUploadCustomTransportError(t, plain) })
	}
}

func testAttachmentsUploadCustomTransportError(t *testing.T, plain bool) {
	const signedURL = "http://127.0.0.1:1/blob?signature=SECRETVALUE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"signed_id":"signed-123",
			"attachable_sgid":"sgid-456",
			"direct_upload":{"url":"` + signedURL + `","headers":{"Content-Type":"text/plain"}}
		}`))
	}))
	t.Cleanup(server.Close)
	hooks := &requestRecordingHooks{}
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithHooks(hooks), WithTransport(&leakyStorageTransport{inner: http.DefaultTransport, plain: plain}))

	_, err := client.Attachments().Upload(context.Background(), "note.txt", "text/plain", strings.NewReader("contents"))
	if err == nil {
		t.Fatal("expected a network error")
	}
	var sdkErr *Error
	if !errors.As(err, &sdkErr) || sdkErr.Code != CodeNetwork {
		t.Fatalf("expected a network *Error, got %T: %v", err, err)
	}
	for _, text := range append(renderings(err), sdkErr.Hint) {
		if strings.Contains(text, "SECRETVALUE") {
			t.Errorf("the signed query leaked into %q", text)
		}
	}
	if len(hooks.results) != 2 || hooks.results[1].Error == nil {
		t.Fatalf("expected the storage request's failure in the hooks, got %+v", hooks.results)
	}
	for _, text := range renderings(hooks.results[1].Error) {
		if strings.Contains(text, "SECRETVALUE") {
			t.Errorf("the signed query leaked into a hook result: %q", text)
		}
	}
	if got := hooks.results[1].Error.Error(); got != "transport failure" {
		t.Errorf("hook result should be the failure's classification alone, got %q", got)
	}
}

// TestBlobDownloadRedirectReachesHooksProjected downloads a blob HEY answers with a
// redirect to a signed storage URL and checks the hooks see the storage hop projected:
// net/http follows the redirect on the same context, and a successful download has no
// error for ErrNetwork to project.
func TestBlobDownloadRedirectReachesHooksProjected(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "download")
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/signed-download?signature=SECRETVALUE", http.StatusFound)
	}))
	t.Cleanup(source.Close)
	hooks := &requestRecordingHooks{}
	client := NewClient(&Config{BaseURL: source.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithHooks(hooks))

	for name, download := range map[string]func() error{
		"GetBlob": func() error {
			_, err := client.GetBlob(context.Background(), "/blob")
			return err
		},
		"DownloadBlob": func() error {
			_, _, err := client.DownloadBlob(context.Background(), "/blob", io.Discard)
			return err
		},
		"GetBlob under a hook returning its own context": func() error {
			hooks.fresh = true
			defer func() { hooks.fresh = false }()
			_, err := client.GetBlob(context.Background(), "/blob")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			hooks.infos = nil
			if err := download(); err != nil {
				t.Fatal(err)
			}
			if len(hooks.infos) != 2 {
				t.Fatalf("expected the HEY hop and the storage hop in the hooks, got %+v", hooks.infos)
			}
			if got := hooks.infos[1].URL; got != target.URL {
				t.Errorf("storage hop reached the hooks as %q, want the projected URL", got)
			}
			for _, info := range hooks.infos {
				if strings.Contains(info.URL, "SECRETVALUE") {
					t.Errorf("the signed query leaked into a hook argument: %q", info.URL)
				}
			}
		})
	}
}

// TestBlobDownloadRetryHookSeesProjectedURL retries a blob download whose own URL
// carries a query and checks OnRetry sees it as the request hooks do.
func TestBlobDownloadRetryHookSeesProjectedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	hooks := &retryRecordingHooks{}
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(1), WithBaseDelay(time.Millisecond), WithMaxJitter(time.Millisecond), WithHooks(hooks))

	if _, err := client.GetBlob(context.Background(), "/blob?signature=SECRETVALUE"); err == nil {
		t.Fatal("expected the 503 to surface")
	}
	if len(hooks.retries) != 1 {
		t.Fatalf("expected one retry, got %+v", hooks.retries)
	}
	if got := hooks.retries[0].info.URL; got != server.URL {
		t.Errorf("retry hook saw %q, want the projected URL", got)
	}
	for _, start := range hooks.starts {
		if strings.Contains(start.URL, "SECRETVALUE") {
			t.Errorf("the signed query leaked into a request hook argument: %q", start.URL)
		}
	}
}

func TestRedactTransportError(t *testing.T) {
	signed := &url.Error{Op: "Get", URL: "https://user:pw@storage.example.com/blob/1?sig=SECRETVALUE#frag", Err: context.Canceled}

	t.Run("projects the URL of a bare transport error and keeps its classification", func(t *testing.T) {
		got := redactTransportError(signed, "")
		want := `Get "https://storage.example.com": context canceled`
		if got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		if !errors.Is(got, context.Canceled) {
			t.Error("the cause chain should still reach the context sentinel")
		}
	})

	t.Run("drops a wrapper around the projection, whatever its text carried", func(t *testing.T) {
		for name, wrapped := range map[string]error{
			"prefix":           fmt.Errorf("fetching the blob: %w", signed),
			"interpolated URL": fmt.Errorf("request %s failed: %w", signed.URL, signed),
			"opaque":           &opaqueWrapperError{cause: signed},
		} {
			got := redactTransportError(wrapped, "")
			if want := `Get "https://storage.example.com": context canceled`; got.Error() != want {
				t.Errorf("%s: got %q, want %q", name, got.Error(), want)
			}
			var urlErr *url.Error
			if !errors.As(got, &urlErr) || urlErr.URL != "https://storage.example.com" {
				t.Errorf("%s: should unwrap to the projected *url.Error, got %v", name, got)
			}
			if !errors.Is(got, context.Canceled) {
				t.Errorf("%s: the cause chain should still reach the context sentinel", name)
			}
		}
	})

	t.Run("keeps only the classification beneath a projected URL", func(t *testing.T) {
		opaque := &url.Error{Op: "Put", URL: "https://storage.example.com/blob/1?sig=SECRETVALUE", Err: timeoutError{}}
		got := redactTransportError(opaque, "")
		if want := `Put "https://storage.example.com": transport timeout`; got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		for _, text := range renderings(got) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
		var netErr net.Error
		if !errors.As(got, &netErr) || !netErr.Timeout() {
			t.Errorf("the timeout classification should survive, got %v", got)
		}
		plain := redactTransportError(&url.Error{Op: "Put", URL: "https://storage.example.com/blob/1?sig=SECRETVALUE", Err: errors.New("connection refused")}, "")
		if want := `Put "https://storage.example.com": transport failure`; plain.Error() != want {
			t.Errorf("got %q, want %q", plain.Error(), want)
		}
		if !errors.As(plain, &netErr) || netErr.Timeout() {
			t.Errorf("a refused connection should not classify as a timeout, got %v", plain)
		}
	})

	t.Run("projects every transport error in a nested chain", func(t *testing.T) {
		outer := &url.Error{Op: "Get", URL: "https://proxy.example.com/relay", Err: signed}
		got := redactTransportError(outer, "")
		want := `Get "https://proxy.example.com": context canceled`
		if got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		for _, text := range renderings(got) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
		if got = redactTransportError(fmt.Errorf("relaying to %s: %w", outer.URL, outer), ""); got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
	})

	t.Run("projects every member of a joined error and keeps the rest", func(t *testing.T) {
		other := &url.Error{Op: "Get", URL: "https://other.example.com/x?token=SECRETVALUE", Err: errors.New("reset")}
		got := redactTransportError(errors.Join(signed, context.DeadlineExceeded, other), "")
		want := "Get \"https://storage.example.com\": context canceled\ncontext deadline exceeded\nGet \"https://other.example.com\": transport failure"
		if got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		if !errors.Is(got, context.Canceled) || !errors.Is(got, context.DeadlineExceeded) {
			t.Error("every member should still be reachable through the chain")
		}
		for _, text := range renderings(got) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
	})

	t.Run("builds a projected transport error from fixed parts alone", func(t *testing.T) {
		const signedURL = "https://storage.example.com/blob/1?sig=SECRETVALUE"
		cancelledTimeout := &url.Error{Op: "fetch " + signedURL, URL: signedURL, Err: cancelledTimeoutError{}}
		got := redactTransportError(cancelledTimeout, "")
		if want := `Request "https://storage.example.com": context canceled`; got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		for _, text := range renderings(got) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
		var netErr net.Error
		if !errors.Is(got, context.Canceled) || !errors.As(got, &netErr) || !netErr.Timeout() {
			t.Errorf("the cancellation and the timeout classification should both survive, got %v", got)
		}

		joined := redactTransportError(errors.Join(signed, fmt.Errorf("request %s failed", signedURL), context.DeadlineExceeded), "")
		if want := "Get \"https://storage.example.com\": context canceled\ncontext deadline exceeded"; joined.Error() != want {
			t.Errorf("got %q, want %q", joined.Error(), want)
		}
		if !errors.Is(joined, context.Canceled) || !errors.Is(joined, context.DeadlineExceeded) {
			t.Error("the sentinel siblings should still be reachable through the chain")
		}
	})

	t.Run("keeps a dropped sibling's classification and an exposed transport error", func(t *testing.T) {
		joined := redactTransportError(errors.Join(timeoutError{}, signed), "")
		var netErr net.Error
		if !errors.As(joined, &netErr) || !netErr.Timeout() {
			t.Errorf("a dropped net.Error sibling should leave its classification, got %v", joined)
		}
		for _, text := range renderings(joined) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
		exposed := redactTransportError(&asOnlyError{target: signed}, "")
		if want := `Get "https://storage.example.com": context canceled`; exposed.Error() != want {
			t.Errorf("got %q, want %q", exposed.Error(), want)
		}
	})

	t.Run("keeps the cause beneath a URL on the API origin", func(t *testing.T) {
		api := &url.Error{Op: "Get", URL: "https://api.example.com/boxes?page=2", Err: errors.New("connection refused")}
		got := redactTransportError(api, "https://api.example.com")
		if want := `Get "https://api.example.com/boxes": connection refused`; got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		off := redactTransportError(api, "https://other.example.com")
		if want := `Get "https://api.example.com": transport failure`; off.Error() != want {
			t.Errorf("got %q, want %q", off.Error(), want)
		}
		withUser := &url.Error{Op: "Get", URL: "https://user:SECRETVALUE@api.example.com/x", Err: errors.New("request https://user:SECRETVALUE@api.example.com/x failed")}
		got = redactTransportError(withUser, "https://api.example.com")
		if want := `Get "https://api.example.com": transport failure`; got.Error() != want {
			t.Errorf("userinfo on the API origin is not trusted: got %q, want %q", got.Error(), want)
		}
		ancestor := &url.Error{Op: "relay of https://storage.example.com/blob?sig=SECRETVALUE", URL: "https://api.example.com/relay", Err: signed}
		got = redactTransportError(ancestor, "https://api.example.com")
		if want := `Request "https://api.example.com/relay": Get "https://storage.example.com": context canceled`; got.Error() != want {
			t.Errorf("an ancestor keeps only the Op token: got %q, want %q", got.Error(), want)
		}
		both := &url.Error{Op: "Get", URL: "https://storage.example.com/blob?sig=SECRETVALUE", Err: errors.Join(context.Canceled, context.DeadlineExceeded)}
		got = redactTransportError(both, "")
		if !errors.Is(got, context.Canceled) || !errors.Is(got, context.DeadlineExceeded) {
			t.Errorf("both sentinels beneath a projected URL should survive, got %v", got)
		}
	})

	t.Run("returns an error with nothing to drop unchanged", func(t *testing.T) {
		plain := &url.Error{Op: "Get", URL: "https://api.example.com/x", Err: context.Canceled}
		if got := redactTransportError(plain, "https://api.example.com"); got != plain { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
		other := errors.New("not a transport error")
		if got := redactTransportError(other, ""); got != other { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
		nested := &url.Error{Op: "Get", URL: "https://api.example.com/relay", Err: plain}
		if got := redactTransportError(nested, "https://api.example.com"); got != nested { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
		wrapped := fmt.Errorf("fetching: %w", plain)
		if got := redactTransportError(wrapped, "https://api.example.com"); got != wrapped { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
	})

	t.Run("projects an unparsable URL to a fixed token", func(t *testing.T) {
		bad := &url.Error{Op: "Get", URL: "http://[::1/x?sig=SECRETVALUE", Err: context.Canceled}
		got := redactTransportError(bad, "")
		if strings.Contains(got.Error(), "SECRETVALUE") || !strings.Contains(got.Error(), `"unparsable"`) {
			t.Errorf("got %q", got.Error())
		}
	})
}

func TestAsErrorRendersNoSignedQuery(t *testing.T) {
	signed := &url.Error{Op: "Put", URL: "https://storage.example.com/blob?sig=SECRETVALUE", Err: errors.New("connection reset")}
	e := AsError(signed)
	for _, text := range append(renderings(e), e.Message) {
		if strings.Contains(text, "SECRETVALUE") {
			t.Errorf("the signed query leaked into %q", text)
		}
	}
}

// timeoutError is a custom transport's failure: it classifies as a timeout and, being
// text this package did not build, renders the request URL on its own.
type timeoutError struct{}

func (timeoutError) Error() string {
	return "request https://storage.example.com/blob/1?sig=SECRETVALUE failed: i/o timeout"
}
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// cancelledTimeoutError is a custom transport's failure that wraps a cancellation and
// classifies as a timeout at once.
type cancelledTimeoutError struct{}

func (cancelledTimeoutError) Error() string   { return "cancelled: " + context.Canceled.Error() }
func (cancelledTimeoutError) Unwrap() error   { return context.Canceled }
func (cancelledTimeoutError) Timeout() bool   { return true }
func (cancelledTimeoutError) Temporary() bool { return false }

// asOnlyError exposes a *url.Error through As alone, with no Unwrap, and renders the
// signed URL on its own.
type asOnlyError struct{ target *url.Error }

func (e *asOnlyError) Error() string { return "request " + e.target.URL + " failed" }

func (e *asOnlyError) As(target any) bool {
	if p, ok := target.(**url.Error); ok {
		*p = e.target
		return true
	}
	return false
}

// opaqueWrapperError wraps an error without rendering it.
type opaqueWrapperError struct{ cause error }

func (w *opaqueWrapperError) Error() string { return "request failed" }
func (w *opaqueWrapperError) Unwrap() error { return w.cause }

// TestAttachmentsUploadInsecureTargetRendersNoSignedURL hands Upload a direct-upload
// URL RequireSecureEndpoint rejects — plain http off localhost — and checks the
// rejection names the target's origin, not the signed URL.
func TestAttachmentsUploadInsecureTargetRendersNoSignedURL(t *testing.T) {
	const signedURL = "http://storage.example/blob?signature=SECRETVALUE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"signed_id":"signed-123",
			"attachable_sgid":"sgid-456",
			"direct_upload":{"url":"` + signedURL + `","headers":{"Content-Type":"text/plain"}}
		}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	_, err := client.Attachments().Upload(context.Background(), "note.txt", "text/plain", strings.NewReader("contents"))
	if err == nil {
		t.Fatal("expected the insecure upload target to be rejected")
	}
	for _, text := range renderings(err) {
		if strings.Contains(text, "SECRETVALUE") {
			t.Errorf("the signed query leaked into %q", text)
		}
	}
	if !strings.Contains(err.Error(), "http://storage.example") {
		t.Errorf("the rejection should name the target's origin, got %q", err)
	}
}

// callerURLRequests is every request method that takes a caller's absolute URL, each
// sending one to the given server.
func callerURLRequests(client *Client, target string) map[string]func() error {
	return map[string]func() error{
		"Get": func() error {
			_, err := client.Get(context.Background(), target)
			return err
		},
		"GetAll": func() error {
			_, err := client.GetAll(context.Background(), target)
			return err
		},
		"PostForm": func() error {
			_, err := client.PostForm(context.Background(), target, url.Values{"name": {"value"}})
			return err
		},
		"PostMultipart": func() error {
			_, err := client.PostMultipart(context.Background(), target, "multipart/form-data; boundary=b", []byte("--b--"))
			return err
		},
	}
}

// TestCallerAbsoluteURLReachesHooksProjected hands each method that takes a caller's
// absolute URL a signed one — a disk-service token in the path, on HEY's own origin
// — and checks the hooks see its origin alone, and a 404 names no more.
func TestCallerAbsoluteURLReachesHooksProjected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Location", "/done")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)
	hooks := &requestRecordingHooks{}
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithHooks(hooks))

	for name, request := range callerURLRequests(client, server.URL+"/rails/active_storage/disk/SECRETVALUE/file.txt") {
		t.Run(name, func(t *testing.T) {
			hooks.infos = nil
			if err := request(); err != nil {
				t.Fatal(err)
			}
			if len(hooks.infos) != 1 {
				t.Fatalf("expected one request in the hooks, got %+v", hooks.infos)
			}
			if got := hooks.infos[0].URL; got != server.URL {
				t.Errorf("the request reached the hooks as %q, want the projected URL", got)
			}
		})
	}

	t.Run("not found", func(t *testing.T) {
		_, err := client.Get(context.Background(), server.URL+"/rails/active_storage/disk/SECRETVALUE/missing")
		var sdkErr *Error
		if !errors.As(err, &sdkErr) || sdkErr.Code != CodeNotFound {
			t.Fatalf("expected a not-found *Error, got %T: %v", err, err)
		}
		for _, text := range append(renderings(err), sdkErr.Message) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed path leaked into %q", text)
			}
		}
		if !strings.Contains(sdkErr.Message, server.URL) {
			t.Errorf("the not-found error should name the origin, got %q", sdkErr.Message)
		}
	})
}

// TestCallerAbsoluteURLTransportErrorRendersNoSignedURL dials a closed port through
// each method that takes a caller's absolute URL — a signed one on the API origin
// itself, as the disk service serves — and checks the network error keeps the origin
// and nothing beneath it.
func TestCallerAbsoluteURLTransportErrorRendersNoSignedURL(t *testing.T) {
	const origin = "https://127.0.0.1:1"
	client := NewClient(&Config{BaseURL: origin}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	for name, request := range callerURLRequests(client, origin+"/rails/active_storage/disk/SECRETVALUE/file.txt") {
		t.Run(name, func(t *testing.T) {
			err := request()
			var sdkErr *Error
			if !errors.As(err, &sdkErr) || sdkErr.Code != CodeNetwork {
				t.Fatalf("expected a network *Error, got %T: %v", err, err)
			}
			for _, text := range append(renderings(err), sdkErr.Hint) {
				if strings.Contains(text, "SECRETVALUE") {
					t.Errorf("the signed query leaked into %q", text)
				}
			}
			if !strings.Contains(sdkErr.Hint, `"`+origin+`"`) {
				t.Errorf("hint should keep the origin, got %q", sdkErr.Hint)
			}
		})
	}
}

// TestCallerInsecureURLRendersNoSignedURL hands Get a signed http URL on another host,
// which buildURL rejects, and checks the rejection names the origin alone.
func TestCallerInsecureURLRendersNoSignedURL(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"})

	_, err := client.Get(context.Background(), "http://storage.example/blob?signature=SECRETVALUE")
	if err == nil {
		t.Fatal("expected the http URL on another host to be rejected")
	}
	if strings.Contains(err.Error(), "SECRETVALUE") {
		t.Errorf("the signed query leaked into %q", err)
	}
	if !strings.Contains(err.Error(), "http://storage.example") {
		t.Errorf("the rejection should name the origin, got %q", err)
	}
}
