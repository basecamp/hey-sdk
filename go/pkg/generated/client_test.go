package generated

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientPreservesHTTPClientIdentity(t *testing.T) {
	defaultClient, err := NewClient("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := defaultClient.Client.(*http.Client); !ok {
		t.Fatalf("default Client doer has type %T, want *http.Client", defaultClient.Client)
	}

	supplied := &http.Client{Timeout: 17 * time.Second}
	client, err := NewClient("https://example.com", WithHTTPClient(supplied))
	if err != nil {
		t.Fatal(err)
	}
	if client.Client != supplied {
		t.Fatalf("Client doer = %p, want supplied client %p", client.Client, supplied)
	}
}

func TestIsFormURLEncoded(t *testing.T) {
	tests := map[string]bool{
		"application/x-www-form-urlencoded":                true,
		"application/x-www-form-urlencoded; charset=utf-8": true,
		"application/x-www-form-urlencoded-extra":          false,
		"application/json":                                 false,
		"not a media type":                                 false,
	}
	for contentType, expected := range tests {
		t.Run(contentType, func(t *testing.T) {
			if got := isFormURLEncoded(contentType); got != expected {
				t.Fatalf("isFormURLEncoded(%q) = %v, want %v", contentType, got, expected)
			}
		})
	}
}

func TestDoRequestCapturesFormRedirectWithoutMutatingHTTPClient(t *testing.T) {
	var redirectRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirectRequests.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	var redirectChecks atomic.Int32
	supplied := &http.Client{
		Timeout: 17 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirectChecks.Add(1)
			return nil
		},
	}
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
	if response.StatusCode != http.StatusFound {
		t.Fatalf("form response status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	if got := response.Header.Get("Location"); got != "/redirected" {
		t.Fatalf("form response Location = %q, want %q", got, "/redirected")
	}
	if got := redirectRequests.Load(); got != 0 {
		t.Fatalf("form request followed redirect %d times, want 0", got)
	}
	if got := redirectChecks.Load(); got != 0 {
		t.Fatalf("form request called supplied CheckRedirect %d times, want 0", got)
	}
	if client.Client != supplied || supplied.Timeout != 17*time.Second {
		t.Fatal("form request mutated or replaced the supplied HTTP client")
	}

	directRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	directResponse, err := supplied.Do(directRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer directResponse.Body.Close()
	if directResponse.StatusCode != http.StatusOK {
		t.Fatalf("direct response status = %d, want %d", directResponse.StatusCode, http.StatusOK)
	}
	if got := redirectRequests.Load(); got != 1 {
		t.Fatalf("direct request followed redirect %d times, want 1", got)
	}
	if got := redirectChecks.Load(); got != 1 {
		t.Fatalf("direct request called supplied CheckRedirect %d times, want 1", got)
	}
}

func TestDoRequestPreservesCustomDoerIdentity(t *testing.T) {
	doer := &recordingDoer{}
	client, err := NewClient("https://example.com", WithHTTPClient(doer))
	if err != nil {
		t.Fatal(err)
	}
	if client.Client != doer {
		t.Fatal("NewClient replaced the supplied custom doer")
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
	if got := doer.calls.Load(); got != 1 {
		t.Fatalf("custom doer calls = %d, want 1", got)
	}
}

func TestPostConstructionTransportOptionsPreserveExistingTransport(t *testing.T) {
	base := &recordingTransport{}
	client, err := NewClient("https://example.com", WithHTTPClient(&http.Client{Transport: base}))
	if err != nil {
		t.Fatal(err)
	}

	if err := WithAuthTransport(&StaticTokenProvider{Token: "test-token"}, "test-agent")(client); err != nil {
		t.Fatal(err)
	}
	httpClient, ok := client.Client.(*http.Client)
	if !ok {
		t.Fatalf("authenticated Client doer has type %T, want *http.Client", client.Client)
	}
	authTransport, ok := httpClient.Transport.(*AuthTransport)
	if !ok {
		t.Fatalf("authenticated transport has type %T, want *AuthTransport", httpClient.Transport)
	}
	if authTransport.Base != base {
		t.Fatal("WithAuthTransport did not preserve the existing transport")
	}

	if err := WithCachingTransport(NewInMemoryCache())(client); err != nil {
		t.Fatal(err)
	}
	httpClient, ok = client.Client.(*http.Client)
	if !ok {
		t.Fatalf("cached Client doer has type %T, want *http.Client", client.Client)
	}
	cachingTransport, ok := httpClient.Transport.(*CachingTransport)
	if !ok {
		t.Fatalf("cached transport has type %T, want *CachingTransport", httpClient.Transport)
	}
	if cachingTransport.Base != authTransport {
		t.Fatal("WithCachingTransport did not preserve the authenticated transport")
	}
}

type recordingDoer struct {
	calls atomic.Int32
}

func (d *recordingDoer) Do(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"/drafts/1"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

type recordingTransport struct{}

func (*recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

var _ http.RoundTripper = (*recordingTransport)(nil)
var _ HttpRequestDoer = (*recordingDoer)(nil)
