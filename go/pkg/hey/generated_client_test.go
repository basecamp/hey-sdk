package hey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
