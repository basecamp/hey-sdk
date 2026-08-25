package generated

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsFormURLEncoded(t *testing.T) {
	tests := map[string]bool{
		"application/x-www-form-urlencoded":                true,
		"application/x-www-form-urlencoded; charset=utf-8": true,
		"application/x-www-form-urlencoded-extra":          false,
		"application/json":                                 false,
	}
	for contentType, expected := range tests {
		if got := isFormURLEncoded(contentType); got != expected {
			t.Fatalf("isFormURLEncoded(%q) = %v, want %v", contentType, got, expected)
		}
	}
}

func TestNormalizeRailsArrayFormValues(t *testing.T) {
	values := url.Values{
		"entry[addressed][directly][][1]": {"two@example.org"},
		"entry[addressed][directly][][0]": {"one@example.com"},
		"message[subject]":                {"Project update"},
	}
	normalized := normalizeRailsArrayFormValues(values)
	if got := normalized["entry[addressed][directly][]"]; strings.Join(got, ",") != "one@example.com,two@example.org" {
		t.Fatalf("normalized recipients = %#v", got)
	}
	if got := normalized.Get("message[subject]"); got != "Project update" {
		t.Fatalf("subject = %q", got)
	}
}

func TestDoRequestCapturesFormRedirectWithoutMutatingHTTPClient(t *testing.T) {
	var followed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			followed.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	supplied := &http.Client{Timeout: 17 * time.Second}
	client, err := NewClient(server.URL, WithHTTPClient(supplied))
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/start", strings.NewReader("status=drafted"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	response, err := client.doRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound || followed.Load() != 0 {
		t.Fatalf("status=%d followed=%d", response.StatusCode, followed.Load())
	}
	if client.Client != supplied || supplied.Timeout != 17*time.Second {
		t.Fatal("form request mutated or replaced the supplied HTTP client")
	}
}

type generatedRecordingDoer struct {
	calls atomic.Int32
}

func (d *generatedRecordingDoer) Do(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"/messages/1"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestDoRequestPreservesCustomDoer(t *testing.T) {
	doer := &generatedRecordingDoer{}
	client, err := NewClient("https://example.com", WithHTTPClient(doer))
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com/drafts", strings.NewReader("status=drafted"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.doRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if doer.calls.Load() != 1 || client.Client != doer {
		t.Fatalf("calls=%d client preserved=%t", doer.calls.Load(), client.Client == doer)
	}
}
