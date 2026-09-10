package hey

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
}

func (h *requestRecordingHooks) OnRequestStart(ctx context.Context, info RequestInfo) context.Context {
	h.infos = append(h.infos, info)
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
	if !strings.Contains(sdkErr.Hint, `"http://127.0.0.1:1/blob"`) {
		t.Errorf("hint should keep the URL's scheme, host and path, got %q", sdkErr.Hint)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr.URL != "http://127.0.0.1:1/blob" {
		t.Errorf("the cause chain should still classify as the projected *url.Error, got %v", err)
	}

	if len(hooks.infos) != 2 || len(hooks.results) != 2 {
		t.Fatalf("expected the API request and the storage request in the hooks, got %d starts, %d ends", len(hooks.infos), len(hooks.results))
	}
	if storage := hooks.infos[1]; storage.Method != http.MethodPut || storage.URL != "http://127.0.0.1:1/blob" {
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

func TestRedactTransportError(t *testing.T) {
	signed := &url.Error{Op: "Get", URL: "https://user:pw@storage.example.com/blob/1?sig=SECRETVALUE#frag", Err: context.Canceled}

	t.Run("projects the URL of a bare transport error and keeps its classification", func(t *testing.T) {
		got := redactTransportError(signed)
		want := `Get "https://storage.example.com/blob/1": context canceled`
		if got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		if !errors.Is(got, context.Canceled) {
			t.Error("the cause chain should still reach the context sentinel")
		}
	})

	t.Run("keeps a wrapper's text around the projection", func(t *testing.T) {
		got := redactTransportError(fmt.Errorf("fetching the blob: %w", signed))
		want := `fetching the blob: Get "https://storage.example.com/blob/1": context canceled`
		if got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		var urlErr *url.Error
		if !errors.As(got, &urlErr) || urlErr.URL != "https://storage.example.com/blob/1" {
			t.Errorf("should unwrap to the projected *url.Error, got %v", got)
		}
		if !errors.Is(got, context.Canceled) {
			t.Error("the cause chain should still reach the context sentinel")
		}
	})

	t.Run("keeps only the transport error when a wrapper hides the URL", func(t *testing.T) {
		got := redactTransportError(&opaqueWrapperError{cause: signed})
		if got.Error() != `Get "https://storage.example.com/blob/1": context canceled` {
			t.Errorf("got %q", got.Error())
		}
	})

	t.Run("projects every transport error in a nested chain", func(t *testing.T) {
		outer := &url.Error{Op: "Get", URL: "https://proxy.example.com/relay", Err: signed}
		got := redactTransportError(outer)
		want := `Get "https://proxy.example.com/relay": Get "https://storage.example.com/blob/1": context canceled`
		if got.Error() != want {
			t.Errorf("got %q, want %q", got.Error(), want)
		}
		for _, text := range renderings(got) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
		got = redactTransportError(fmt.Errorf("relaying: %w", outer))
		if got.Error() != "relaying: "+want {
			t.Errorf("got %q, want %q", got.Error(), "relaying: "+want)
		}
	})

	t.Run("keeps only the first transport error of a joined pair", func(t *testing.T) {
		other := &url.Error{Op: "Get", URL: "https://other.example.com/x?token=SECRETVALUE", Err: errors.New("reset")}
		got := redactTransportError(errors.Join(signed, other))
		if got.Error() != `Get "https://storage.example.com/blob/1": context canceled` {
			t.Errorf("got %q", got.Error())
		}
		for _, text := range renderings(got) {
			if strings.Contains(text, "SECRETVALUE") {
				t.Errorf("the signed query leaked into %q", text)
			}
		}
	})

	t.Run("returns an error with nothing to drop unchanged", func(t *testing.T) {
		plain := &url.Error{Op: "Get", URL: "https://api.example.com/x", Err: context.Canceled}
		if got := redactTransportError(plain); got != plain { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
		other := errors.New("not a transport error")
		if got := redactTransportError(other); got != other { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
		nested := &url.Error{Op: "Get", URL: "https://proxy.example.com/relay", Err: plain}
		if got := redactTransportError(nested); got != nested { //nolint:errorlint // identity is the point
			t.Errorf("got %v, want the same error", got)
		}
	})

	t.Run("projects an unparsable URL to a fixed token", func(t *testing.T) {
		bad := &url.Error{Op: "Get", URL: "http://[::1/x?sig=SECRETVALUE", Err: context.Canceled}
		got := redactTransportError(bad)
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

// opaqueWrapperError wraps an error without rendering it.
type opaqueWrapperError struct{ cause error }

func (w *opaqueWrapperError) Error() string { return "request failed" }
func (w *opaqueWrapperError) Unwrap() error { return w.cause }
