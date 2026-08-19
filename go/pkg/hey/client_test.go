package hey

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cfg := &Config{BaseURL: server.URL}
	return NewClient(cfg, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
		WithBaseDelay(1*time.Millisecond),
		WithMaxJitter(1*time.Millisecond),
	)
}

func TestNewClient_HTTPSEnforcement(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-HTTPS base URL")
		}
	}()
	cfg := &Config{BaseURL: "http://example.com"}
	NewClient(cfg, &StaticTokenProvider{Token: "test"})
}

func TestNewClient_LocalhostHTTP(t *testing.T) {
	// Should not panic for localhost HTTP
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "test"})
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_InvalidTimeout(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for zero timeout")
		}
	}()
	cfg := &Config{BaseURL: "http://localhost:3000"}
	NewClient(cfg, &StaticTokenProvider{Token: "test"}, WithTimeout(0))
}

func TestNewClient_NegativeRetries(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative retries")
		}
	}()
	cfg := &Config{BaseURL: "http://localhost:3000"}
	NewClient(cfg, &StaticTokenProvider{Token: "test"}, WithMaxRetries(-1))
}

func TestNewClient_InvalidMaxPages(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for zero maxPages")
		}
	}()
	cfg := &Config{BaseURL: "http://localhost:3000"}
	NewClient(cfg, &StaticTokenProvider{Token: "test"}, WithMaxPages(0))
}

func TestClient_Get(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("expected Bearer token in Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	})

	resp, err := client.Get(context.Background(), "/test.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClient_GetBlob(t *testing.T) {
	binary := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff, 0xfe}
	var requests atomic.Int64
	var lastIfNoneMatch atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		lastIfNoneMatch.Store(r.Header.Get("If-None-Match"))
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Errorf("Accept = %q, want %q", got, "*/*")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binary)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Millisecond),
		WithCache(NewCache(t.TempDir())),
	)

	for i := 0; i < 2; i++ {
		resp, err := client.GetBlob(context.Background(), "/rails/blobs/abc/image.png")
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, resp.StatusCode, http.StatusOK)
		}
		if !bytes.Equal(resp.Data, binary) {
			t.Fatalf("request %d body = %d bytes, want %d", i+1, len(resp.Data), len(binary))
		}
		if resp.FromCache {
			t.Errorf("request %d unexpectedly came from cache", i+1)
		}
		if value, _ := lastIfNoneMatch.Load().(string); value != "" {
			t.Errorf("request %d If-None-Match = %q, want empty", i+1, value)
		}
	}
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

func TestClient_GetBlobRejectsCrossOriginURL(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("cross-origin blob URL should be rejected before making a request")
	})

	_, err := client.GetBlob(context.Background(), "https://files.example.org/report.pdf")
	sdkErr, ok := err.(*Error)
	if !ok || sdkErr.Code != CodeUsage {
		t.Fatalf("cross-origin error = %v, want usage", err)
	}
}

func TestClient_GetBlobStripsAuthorizationOnCrossOriginRedirect(t *testing.T) {
	binary := []byte{0x00, 0xff, 0x01, 0xfe}
	var targetAuthorization atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetAuthorization.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binary)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("source Authorization = %q, want Bearer token", got)
		}
		http.Redirect(w, r, target.URL+"/download/file.bin", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	client := NewClient(
		&Config{BaseURL: source.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
	)
	resp, err := client.GetBlob(context.Background(), "/rails/active_storage/blobs/redirect/abc/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resp.Data, binary) {
		t.Fatalf("body = %v, want %v", resp.Data, binary)
	}
	if got, _ := targetAuthorization.Load().(string); got != "" {
		t.Errorf("target Authorization = %q, want empty", got)
	}
}

func TestClient_DownloadBlobStreamsCrossOriginRedirect(t *testing.T) {
	binary := []byte{0x00, 0xff, 0x01, 0xfe}
	var targetAuthorization atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetAuthorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(binary)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("source Authorization = %q, want Bearer token", got)
		}
		http.Redirect(w, r, target.URL+"/download/file.bin", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	client := NewClient(
		&Config{BaseURL: source.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
	)
	var destination bytes.Buffer
	written, headers, err := client.DownloadBlob(context.Background(), "/rails/active_storage/blobs/redirect/abc/file.bin", &destination)
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len(binary)) || !bytes.Equal(destination.Bytes(), binary) {
		t.Fatalf("download = %d bytes %v, want %v", written, destination.Bytes(), binary)
	}
	if headers.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Content-Type = %q", headers.Get("Content-Type"))
	}
	if got, _ := targetAuthorization.Load().(string); got != "" {
		t.Errorf("target Authorization = %q, want empty", got)
	}
}

type failingStreamWriter struct {
	limit   int
	written int
}

func (w *failingStreamWriter) Write(data []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, errors.New("destination failed")
	}
	if len(data) > remaining {
		w.written += remaining
		return remaining, errors.New("destination failed")
	}
	w.written += len(data)
	return len(data), nil
}

func TestClient_DownloadBlobDoesNotRetryPartialWrites(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("streamed attachment"))
	}))
	t.Cleanup(server.Close)
	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(3),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Millisecond),
	)
	destination := &failingStreamWriter{limit: 4}
	written, _, err := client.DownloadBlob(context.Background(), "/blob", destination)
	if err == nil || written != 4 {
		t.Fatalf("DownloadBlob = %d, %v", written, err)
	}
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1", requests.Load())
	}
}

func TestClient_DownloadBlobRequiresDestination(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("nil destination should be rejected before making a request")
	})
	if _, _, err := client.DownloadBlob(context.Background(), "/blob", nil); err == nil {
		t.Fatal("expected destination error")
	}
}

func TestClient_Post(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["key"] != "value" {
			t.Errorf("expected body key=value, got %v", body)
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"id":1}`))
	})

	resp, err := client.Post(context.Background(), "/create.json", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestClient_Delete(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(204)
	})

	resp, err := client.Delete(context.Background(), "/thing/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestClient_ErrorResponses(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantCode string
	}{
		{"401", 401, CodeAuth},
		{"403_GET", 403, CodeForbidden},
		{"404", 404, CodeNotFound},
		{"500", 500, CodeAPI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(`{}`))
			})
			_, err := client.Get(context.Background(), "/fail")
			if err == nil {
				t.Fatal("expected error")
			}
			apiErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("expected *Error, got %T", err)
			}
			if apiErr.Code != tc.wantCode {
				t.Fatalf("expected code %q, got %q", tc.wantCode, apiErr.Code)
			}
		})
	}
}

func TestBuildURL(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	url, err := c.buildURL("/boxes.json")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://localhost:3000/boxes.json" {
		t.Fatalf("expected full URL, got %q", url)
	}

	url, err = c.buildURL("boxes.json")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://localhost:3000/boxes.json" {
		t.Fatalf("expected leading slash added, got %q", url)
	}

	url, err = c.buildURL("https://cdn.example.com/file")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn.example.com/file" {
		t.Fatalf("expected HTTPS URL passthrough, got %q", url)
	}

	_, err = c.buildURL("http://insecure.example.com/file")
	if err == nil {
		t.Fatal("expected error for HTTP URL to different host")
	}

	// http:// with same host as base should be allowed (pagination in dev)
	url, err = c.buildURL("http://localhost:3000/imbox.json?page=abc")
	if err != nil {
		t.Fatalf("expected http:// same-host URL to be allowed, got error: %v", err)
	}
	if url != "http://localhost:3000/imbox.json?page=abc" {
		t.Fatalf("expected HTTP same-host passthrough, got %q", url)
	}
}

func TestParseNextLink(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{`<https://example.com/page2>; rel="next"`, "https://example.com/page2"},
		{`<https://example.com/page1>; rel="prev", <https://example.com/page2>; rel="next"`, "https://example.com/page2"},
		{`<https://example.com/page1>; rel="prev"`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			got := parseNextLink(tc.header)
			if got != tc.want {
				t.Fatalf("parseNextLink(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		header string
		want   int
	}{
		{"", 0},
		{"30", 30},
		{"0", 0},
		{"-5", 0},
		{"garbage", 0},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			got := parseRetryAfter(tc.header)
			if got != tc.want {
				t.Fatalf("parseRetryAfter(%q) = %d, want %d", tc.header, got, tc.want)
			}
		})
	}
}

func TestClient_ServiceAccessors(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	if c.Identity() == nil {
		t.Fatal("expected non-nil IdentityService")
	}
	if c.Boxes() == nil {
		t.Fatal("expected non-nil BoxesService")
	}
	if c.Topics() == nil {
		t.Fatal("expected non-nil TopicsService")
	}
	if c.Messages() == nil {
		t.Fatal("expected non-nil MessagesService")
	}
	if c.Attachments() == nil {
		t.Fatal("expected non-nil AttachmentsService")
	}
	if c.Entries() == nil {
		t.Fatal("expected non-nil EntriesService")
	}
	if c.Contacts() == nil {
		t.Fatal("expected non-nil ContactsService")
	}
	if c.Calendars() == nil {
		t.Fatal("expected non-nil CalendarsService")
	}
	if c.CalendarTodos() == nil {
		t.Fatal("expected non-nil CalendarTodosService")
	}
	if c.Habits() == nil {
		t.Fatal("expected non-nil HabitsService")
	}
	if c.TimeTracks() == nil {
		t.Fatal("expected non-nil TimeTracksService")
	}
	if c.Journal() == nil {
		t.Fatal("expected non-nil JournalService")
	}
	if c.Search() == nil {
		t.Fatal("expected non-nil SearchService")
	}
	if c.CalendarEvents() == nil {
		t.Fatal("expected non-nil CalendarEventsService")
	}
	if c.Designations() == nil {
		t.Fatal("expected non-nil DesignationsService")
	}
	if c.Extenzions() == nil {
		t.Fatal("expected non-nil ExtenzionsService")
	}

	// Verify idempotency (same instance returned)
	b1 := c.Boxes()
	b2 := c.Boxes()
	if b1 != b2 {
		t.Fatal("expected same BoxesService instance on repeated calls")
	}
}

func TestClient_ConfigCopy(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	cfgCopy := c.Config()
	cfgCopy.BaseURL = "http://modified" //nolint:govet // testing that mutation doesn't propagate

	if c.Config().BaseURL != "http://localhost:3000" {
		t.Fatal("expected original config to be unchanged")
	}
}

func TestClient_WithUserAgent(t *testing.T) {
	var gotUA string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})
	// Default UA
	resp, err := client.Get(context.Background(), "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if gotUA != DefaultUserAgent {
		t.Fatalf("expected default UA, got %q", gotUA)
	}
}

func TestResponse_UnmarshalData(t *testing.T) {
	resp := &Response{Data: json.RawMessage(`{"id":42}`)}
	var result struct {
		ID int `json:"id"`
	}
	if err := resp.UnmarshalData(&result); err != nil {
		t.Fatal(err)
	}
	if result.ID != 42 {
		t.Fatalf("expected 42, got %d", result.ID)
	}
}

func TestClient_RateLimitResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
	})
	client.httpOpts.MaxRetries = 0
	client.httpOpts.BaseDelay = 1 * time.Millisecond
	client.httpOpts.MaxJitter = 1 * time.Millisecond

	_, err := client.Get(context.Background(), "/rate-limited")
	if err == nil {
		t.Fatal("expected error for 429")
	}
}

func TestClient_PutRetriesOn503(t *testing.T) {
	var requestCount atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1}`))
	})
	client.httpOpts.MaxRetries = 2
	client.httpOpts.BaseDelay = 1 * time.Millisecond
	client.httpOpts.MaxJitter = 1 * time.Millisecond

	resp, err := client.Put(context.Background(), "/time_tracks/1.json", map[string]any{"stopped": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 requests (1 retry), got %d", requestCount.Load())
	}
}

func TestClient_DeleteRetriesOn503(t *testing.T) {
	var requestCount atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	})
	client.httpOpts.MaxRetries = 2
	client.httpOpts.BaseDelay = 1 * time.Millisecond
	client.httpOpts.MaxJitter = 1 * time.Millisecond

	resp, err := client.Delete(context.Background(), "/calendar/todos/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 requests (1 retry), got %d", requestCount.Load())
	}
}

func TestClient_PostDoesNotRetryOn503(t *testing.T) {
	var requestCount atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(503)
	})
	client.httpOpts.MaxRetries = 2

	_, err := client.Post(context.Background(), "/messages.json", map[string]string{"subject": "test"})
	if err == nil {
		t.Fatal("expected error for 503 POST")
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 request (no retry for POST), got %d", requestCount.Load())
	}
}

func TestClient_PatchDoesNotRetryOn503(t *testing.T) {
	var requestCount atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(503)
	})
	client.httpOpts.MaxRetries = 2

	_, err := client.Patch(context.Background(), "/things/1.json", map[string]string{"name": "test"})
	if err == nil {
		t.Fatal("expected error for 503 PATCH")
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 request (no retry for PATCH), got %d", requestCount.Load())
	}
}

func TestClient_GatewayErrors(t *testing.T) {
	for _, status := range []int{502, 503, 504} {
		t.Run("status_"+string(rune('0'+status/100))+string(rune('0'+(status/10)%10))+string(rune('0'+status%10)), func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			})
			client.httpOpts.MaxRetries = 0

			_, err := client.Get(context.Background(), "/gw")
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestWithJSONExtension(t *testing.T) {
	cases := map[string]string{
		"/boxes.json":                        "/boxes.json",
		"/stickies/12":                       "/stickies/12.json",
		"/calendar/days/2026-08-16/habits/7": "/calendar/days/2026-08-16/habits/7.json",
		"/topics/1/status/trashed.json":      "/topics/1/status/trashed.json",
		"/world/lists/a@b.com":               "/world/lists/a@b.com", // has a dot: left alone
		"/":                                  "/",
		"":                                   "",
	}
	for in, want := range cases {
		if got := withJSONExtension(in); got != want {
			t.Errorf("withJSONExtension(%q) = %q, want %q", in, got, want)
		}
	}
}
