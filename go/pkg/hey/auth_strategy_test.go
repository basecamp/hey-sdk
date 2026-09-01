package hey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// refreshingAuth is a caller that keeps its credentials somewhere the SDK knows nothing
// about, which is the case a *AuthManager check could never recognise.
type refreshingAuth struct {
	token     atomic.Value
	refreshes atomic.Int64
	refreshed string
}

func (a *refreshingAuth) Authenticate(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.token.Load().(string))
	return nil
}

func (a *refreshingAuth) Refresh(context.Context) error {
	a.refreshes.Add(1)
	a.token.Store(a.refreshed)
	return nil
}

func TestUnauthorizedIsRetriedAfterTheStrategyRefreshes(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	client := NewClient(&Config{BaseURL: srv.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	if _, err := client.Get(context.Background(), "/whatever.json"); err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if len(seen) != 2 || seen[0] != "Bearer stale" || seen[1] != "Bearer fresh" {
		t.Errorf("expected the stale token then the fresh one, got %v", seen)
	}
}

// A strategy that cannot renew its credentials must not turn a 401 into a retry loop.
func TestUnauthorizedIsSurfacedWhenNothingCanRefresh(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "stale"}, WithMaxRetries(1))

	_, err := client.Get(context.Background(), "/whatever.json")
	if err == nil {
		t.Fatal("expected an authentication error")
	}
	if requests.Load() != 1 {
		t.Errorf("expected the 401 to be surfaced on the first request, got %d requests", requests.Load())
	}
}

// The form path holds its body as bytes so the one resend after a refresh carries it again.
func TestFormUnauthorizedIsRetriedAfterTheStrategyRefreshes(t *testing.T) {
	var seen []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/things/42", http.StatusSeeOther)
	}))
	t.Cleanup(srv.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	client := NewClient(&Config{BaseURL: srv.URL}, nil, WithAuthStrategy(auth))

	resp, err := client.PostForm(context.Background(), "/things", url.Values{"name": {"x"}})
	if err != nil {
		t.Fatalf("expected the retry after a refresh to succeed: %v", err)
	}
	if id, err := resp.ExtractID(); err != nil || id != 42 {
		t.Errorf("expected the redirect to be captured, got %+v (%v)", resp, err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if len(seen) != 2 || seen[0] != "Bearer stale" || seen[1] != "Bearer fresh" {
		t.Errorf("expected the stale token then the fresh one, got %v", seen)
	}
	if len(bodies) != 2 || bodies[0] != "name=x" || bodies[1] != "name=x" {
		t.Errorf("expected the body to be sent again on the retry, got %q", bodies)
	}
}

// A refresh that does not help gets no second refresh on the form path either: the 401 is
// surfaced after one resend rather than recursing for as long as refreshes keep succeeding.
func TestFormUnauthorizedIsSurfacedAfterOneRefresh(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Answer anything past the one permitted resend with a status the test can tell
		// apart from the 401, so an unbounded retry fails instead of hanging.
		if requests.Add(1) > 2 {
			http.Error(w, "too many attempts", http.StatusTeapot)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	client := NewClient(&Config{BaseURL: srv.URL}, nil, WithAuthStrategy(auth))

	_, err := client.PostForm(context.Background(), "/things", url.Values{"name": {"x"}})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
		t.Fatalf("expected an authentication error, got %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if requests.Load() != 2 {
		t.Errorf("expected the original request and one resend, got %d requests", requests.Load())
	}
}
