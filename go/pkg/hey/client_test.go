package hey

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		t.Fatal("expected error for HTTP URL")
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
