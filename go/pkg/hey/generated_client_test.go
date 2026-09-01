package hey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// Generated operations run the generated client's own retry loop, so what WithMaxRetries
// configured has to reach it: a caller who lowered the cap to fail fast gets that, not the
// generated default of three.
func TestGeneratedOperationsHonorMaxRetries(t *testing.T) {
	for _, tc := range []struct {
		maxRetries int
		requests   int32
	}{
		{maxRetries: 0, requests: 1},
		{maxRetries: 2, requests: 3},
	} {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(server.Close)

		root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
			WithMaxRetries(tc.maxRetries), WithBaseDelay(time.Millisecond))
		client := scopedTestClient(root, 42)

		if _, err := client.Boxes().List(context.Background()); err == nil {
			t.Fatalf("maxRetries=%d: expected the 503 to surface", tc.maxRetries)
		}
		if got := requests.Load(); got != tc.requests {
			t.Errorf("maxRetries=%d: requests = %d, want %d", tc.maxRetries, got, tc.requests)
		}
	}
}

// A 401 mid-flight is answered by a refresh and one more send, for a generated read...
func TestGeneratedOperationsRetryOnceAfterRefresh(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if len(seen) != 2 || seen[0] != "Bearer stale" || seen[1] != "Bearer fresh" {
		t.Errorf("expected the stale token then the fresh one, got %v", seen)
	}
}

// ...and for a generated mutation, which the retry loop otherwise never resends: a 401
// means the server did nothing with the request, and the body is rebuilt for the retry.
func TestGeneratedMutationsRetryOnceAfterRefresh(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().CreateGroup(context.Background(), 7, []int64{1, 2}); err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if len(seen) != 2 || seen[0] != "Bearer stale" || seen[1] != "Bearer fresh" {
		t.Errorf("expected the stale token then the fresh one, got %v", seen)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Errorf("expected the same body on both sends, got %q", bodies)
	}
}

// A body handed over as a reader is sent whole both times, not drained by the first send.
func TestGeneratedReaderBodiesSurviveTheRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	if _, err := client.CalendarTodos().Create(context.Background(), "Renew", "2026-09-01"); err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Errorf("expected the same body on both sends, got %q", bodies)
	}
}

// A body that both seeks and closes, an *os.File say, is kept from the transport so the
// first send does not close it out from under the resend; it is closed once the last
// response is in, as the transport would have.
func TestGeneratedSeekableClosingBodiesSurviveTheRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(server.Close)

	file, err := os.CreateTemp(t.TempDir(), "body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"title":"Renew","due_on":"2026-09-01"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	client.initGeneratedClient()

	resp, err := client.gen.CreateCalendarTodoWithBody(context.Background(), "application/json", file)
	if err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Errorf("expected the same body on both sends, got %q", bodies)
	}
	if err := file.Close(); !errors.Is(err, os.ErrClosed) {
		t.Errorf("expected the body to be closed once the response was in, got %v", err)
	}
}

// A body that cannot seek is held in memory for the resend, and the first send draining
// the original reader is no loss.
func TestGeneratedNonSeekableBodiesSurviveTheRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	client.initGeneratedClient()

	body := struct{ io.Reader }{strings.NewReader(`{"title":"Renew","due_on":"2026-09-01"}`)}
	resp, err := client.gen.CreateCalendarTodoWithBody(context.Background(), "application/json", body)
	if err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	_ = resp.Body.Close()
	if len(bodies) != 2 || bodies[0] == "" || bodies[0] != bodies[1] {
		t.Errorf("expected the same body on both sends, got %q", bodies)
	}
}

// The resend after a refresh draws on the retry budget the operation has left rather
// than starting a fresh one, and always gets at least the one attempt.
func TestGeneratedRefreshResendSharesTheRetryBudget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		maxRetries int
		statuses   []int
		succeeds   bool
	}{
		{name: "the budget remaining after the 401 carries the resend's transient failures", maxRetries: 3, statuses: []int{401, 503, 503, 200}, succeeds: true},
		{name: "a budget spent before the 401 still grants the one resend", maxRetries: 1, statuses: []int{503, 401, 200}, succeeds: true},
		{name: "the resend does not get a budget of its own", maxRetries: 1, statuses: []int{401, 503, 200}, succeeds: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				n := int(requests.Add(1))
				status := http.StatusInternalServerError
				if n <= len(tc.statuses) {
					status = tc.statuses[n-1]
				}
				if status != http.StatusOK {
					http.Error(w, http.StatusText(status), status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `[]`)
			}))
			t.Cleanup(server.Close)

			auth := &refreshingAuth{refreshed: "fresh"}
			auth.token.Store("stale")
			root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth),
				WithMaxRetries(tc.maxRetries), WithBaseDelay(time.Millisecond))
			client := scopedTestClient(root, 42)

			_, err := client.Boxes().List(context.Background())
			if tc.succeeds && err != nil {
				t.Fatalf("expected the resend to succeed within the budget: %v", err)
			}
			if !tc.succeeds && err == nil {
				t.Fatal("expected the resend's failure to surface once the budget was spent")
			}
			wantRequests := int32(len(tc.statuses))
			if !tc.succeeds {
				wantRequests--
			}
			if got := requests.Load(); got != wantRequests {
				t.Errorf("requests = %d, want %d", got, wantRequests)
			}
		})
	}
}

// WithBaseDelay is honored verbatim by the generated loop, which sleeps BaseDelay before
// its first retry: a delay above the generated MaxDelay ceiling lifts the ceiling rather
// than being cut down to it.
func TestGeneratedRetriesHonorBaseDelayAboveTheDefaultCeiling(t *testing.T) {
	root := NewClient(&Config{BaseURL: "https://example.test"}, &StaticTokenProvider{Token: "token"},
		WithBaseDelay(45*time.Second))
	client := scopedTestClient(root, 42)
	client.initGeneratedClient()

	cfg := client.gen.ClientInterface.(*generated.Client).RetryConfig
	if cfg.BaseDelay != 45*time.Second {
		t.Errorf("BaseDelay = %v, want 45s", cfg.BaseDelay)
	}
	if cfg.MaxDelay < cfg.BaseDelay {
		t.Errorf("MaxDelay = %v, below the configured BaseDelay %v", cfg.MaxDelay, cfg.BaseDelay)
	}
}

// A refresh that does not help gets no second refresh: the 401 is surfaced after one retry.
func TestGeneratedOperationsSurfaceUnauthorizedAfterOneRefresh(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(3))
	client := scopedTestClient(root, 42)

	_, err := client.Boxes().List(context.Background())
	var sdkErr *Error
	if !errors.As(err, &sdkErr) || sdkErr.Code != CodeAuth {
		t.Fatalf("expected an authentication error, got %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if requests.Load() != 2 {
		t.Errorf("expected the original send and one retry, got %d requests", requests.Load())
	}
}

// Nothing to refresh with means nothing to retry with: one request, and the 401 surfaces.
func TestGeneratedOperationsSurfaceUnauthorizedWhenNothingCanRefresh(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "stale"}, WithMaxRetries(3))
	client := scopedTestClient(root, 42)

	_, err := client.Boxes().List(context.Background())
	var sdkErr *Error
	if !errors.As(err, &sdkErr) || sdkErr.Code != CodeAuth {
		t.Fatalf("expected an authentication error, got %v", err)
	}
	if requests.Load() != 1 {
		t.Errorf("expected the 401 to be surfaced on the first request, got %d requests", requests.Load())
	}
}
