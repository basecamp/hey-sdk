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

// The generated retry loop reports its resends the way doRequestURL does: OnRetry before
// each one, told the attempt that failed and the one about to be made, and the transport
// told which attempt each send is.
func TestGeneratedRetriesFireOnRetry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) < 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)

	hooks := &retryRecordingHooks{}
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithHooks(hooks), WithMaxRetries(2), WithBaseDelay(time.Millisecond))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the third attempt to succeed: %v", err)
	}
	retries := hooks.retryCalls()
	if len(retries) != 2 {
		t.Fatalf("expected OnRetry before each of the two resends, got %d calls", len(retries))
	}
	for i, retry := range retries {
		failed := i + 1
		if retry.info.Attempt != failed || retry.attempt != failed+1 {
			t.Errorf("retry %d: told attempt %d failed and %d is next, want %d and %d", i, retry.info.Attempt, retry.attempt, failed, failed+1)
		}
		if retry.info.Method != http.MethodGet {
			t.Errorf("retry %d: method = %q, want GET", i, retry.info.Method)
		}
		var sdkErr *Error
		if !errors.As(retry.err, &sdkErr) || sdkErr.HTTPStatus != http.StatusServiceUnavailable || !sdkErr.Retryable {
			t.Errorf("retry %d: expected the 503 as a retryable SDK error, got %v", i, retry.err)
		}
	}
	if got := hooks.startAttempts(); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("expected the transport to see attempts 1, 2, 3, got %v", got)
	}
	if starts := hooks.startURLs(); retries[0].info.URL != starts[0] || !strings.Contains(retries[0].info.URL, "42") {
		t.Errorf("expected OnRetry to carry the URL the transport sends, %q, got %q", starts[0], retries[0].info.URL)
	}
}

// A resend after a credential refresh is a retry too, reported as attempt 2 with the
// authentication error that prompted it...
func TestGeneratedRefreshResendFiresOnRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(server.Close)

	hooks := &retryRecordingHooks{}
	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithHooks(hooks), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().CreateGroup(context.Background(), 7, []int64{1, 2}); err != nil {
		t.Fatalf("expected the resend after a refresh to succeed: %v", err)
	}
	retries := hooks.retryCalls()
	if len(retries) != 1 {
		t.Fatalf("expected OnRetry once, before the resend, got %d calls", len(retries))
	}
	retry := retries[0]
	if retry.info.Attempt != 1 || retry.attempt != 2 || retry.info.Method != http.MethodPost {
		t.Errorf("expected attempt 1 of the POST to be reported failed and 2 next, got %+v next %d", retry.info, retry.attempt)
	}
	var sdkErr *Error
	if !errors.As(retry.err, &sdkErr) || sdkErr.Code != CodeAuth || !sdkErr.Retryable {
		t.Errorf("expected the 401 as a retryable authentication error, as doRequestURL reports its refresh, got %v", retry.err)
	}
	if got := hooks.startAttempts(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("expected the transport to see attempts 1, 2, got %v", got)
	}
}

// A failure in the transport is reported as the SDK's network error, as doRequestURL
// reports it, so a hook can classify it and see it marked retryable.
func TestGeneratedTransportFailuresFireOnRetryAsNetworkErrors(t *testing.T) {
	transport := &scriptedTransport{turns: []func() (*http.Response, error){
		func() (*http.Response, error) { return nil, errors.New("connection reset") },
		func() (*http.Response, error) { return scriptedResponse(http.StatusOK, `[]`), nil },
	}}
	hooks := &retryRecordingHooks{}
	root := NewClient(&Config{BaseURL: "https://app.hey.test"}, &StaticTokenProvider{Token: "token"},
		WithTransport(transport), WithHooks(hooks), WithMaxRetries(1), WithBaseDelay(time.Millisecond))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the second attempt to succeed: %v", err)
	}
	retries := hooks.retryCalls()
	if len(retries) != 1 {
		t.Fatalf("expected OnRetry once, before the resend, got %d calls", len(retries))
	}
	retry := retries[0]
	if retry.info.Attempt != 1 || retry.attempt != 2 || retry.info.Method != http.MethodGet {
		t.Errorf("expected attempt 1 of the GET to be reported failed and 2 next, got %+v next %d", retry.info, retry.attempt)
	}
	var sdkErr *Error
	if !errors.As(retry.err, &sdkErr) || sdkErr.Code != CodeNetwork || !sdkErr.Retryable {
		t.Errorf("expected the transport failure as a retryable network error, got %v", retry.err)
	}
	if !strings.Contains(retry.err.Error(), "connection reset") {
		t.Errorf("expected the network error to carry the transport's cause, got %v", retry.err)
	}
}

// A transport is free to leave Response.Request unset, as a test or adapter transport
// that builds its own responses does; the refresh resend reports the request it sent
// regardless.
func TestGeneratedRefreshResendFiresOnRetryWhenTheTransportDropsTheRequest(t *testing.T) {
	transport := &scriptedTransport{turns: []func() (*http.Response, error){
		func() (*http.Response, error) { return scriptedResponse(http.StatusUnauthorized, `unauthorized`), nil },
		func() (*http.Response, error) { return scriptedResponse(http.StatusOK, `[]`), nil },
	}}
	hooks := &retryRecordingHooks{}
	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: "https://app.hey.test"}, nil,
		WithTransport(transport), WithAuthStrategy(auth), WithHooks(hooks), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the resend after a refresh to succeed: %v", err)
	}
	retries := hooks.retryCalls()
	if len(retries) != 1 {
		t.Fatalf("expected OnRetry once, before the resend, got %d calls", len(retries))
	}
	retry := retries[0]
	if retry.info.Method != http.MethodGet || !strings.Contains(retry.info.URL, "42") || retry.attempt != 2 {
		t.Errorf("expected the GET the transport was sent, scoped to the account, as attempt 2, got %+v next %d", retry.info, retry.attempt)
	}
}

// A hook that cancels the context on the refresh resend stops it before the transport
// is asked, as it does an ordinary retry.
func TestGeneratedRefreshResendStopsWhenTheHookCancels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hooks := &retryRecordingHooks{onRetry: func() { cancel() }}
	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithHooks(hooks), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	_, err := client.Boxes().List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation to be surfaced, got %v", err)
	}
	if got := hooks.startAttempts(); len(got) != 1 || got[0] != 1 {
		t.Errorf("expected the transport to see only the first send, got %v", got)
	}
	if retries := hooks.retryCalls(); len(retries) != 1 {
		t.Errorf("expected OnRetry once, for the resend that was then cancelled, got %d calls", len(retries))
	}
}

// scriptedTransport answers each round trip from a script, the way a test or adapter
// transport does: responses it built itself, with Response.Request unset, or errors.
type scriptedTransport struct {
	mu    sync.Mutex
	turns []func() (*http.Response, error)
}

func (t *scriptedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.turns) == 0 {
		return nil, errors.New("scriptedTransport: no turn left")
	}
	turn := t.turns[0]
	t.turns = t.turns[1:]
	return turn()
}

func scriptedResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// ...and the attempts after it keep counting from where the operation was, not from 1.
func TestGeneratedRetryAttemptsCountAcrossTheRefresh(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("Authorization") != "Bearer fresh":
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case requests.Add(1) < 2:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
		}
	}))
	t.Cleanup(server.Close)

	hooks := &retryRecordingHooks{}
	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithHooks(hooks),
		WithMaxRetries(3), WithBaseDelay(time.Millisecond))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the operation to succeed on its third attempt: %v", err)
	}
	retries := hooks.retryCalls()
	next := make([]int, 0, len(retries))
	for _, retry := range retries {
		next = append(next, retry.attempt)
	}
	if len(next) != 2 || next[0] != 2 || next[1] != 3 {
		t.Errorf("expected OnRetry for attempts 2 and 3, got %v", next)
	}
	if got := hooks.startAttempts(); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("expected the transport to see attempts 1, 2, 3, got %v", got)
	}
}

// OnRetry is for a resend that happens: a send that succeeds, a failure past the budget
// and a 401 nothing can refresh all go unreported.
func TestGeneratedSendsWithoutResendDoNotFireOnRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "a first send that succeeds", status: http.StatusOK},
		{name: "a failure with no budget to resend on", status: http.StatusServiceUnavailable},
		{name: "a 401 nothing can refresh", status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status != http.StatusOK {
					http.Error(w, http.StatusText(tc.status), tc.status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `[]`)
			}))
			t.Cleanup(server.Close)

			hooks := &retryRecordingHooks{}
			root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithHooks(hooks), WithMaxRetries(0))
			client := scopedTestClient(root, 42)

			_, _ = client.Boxes().List(context.Background())
			if retries := hooks.retryCalls(); len(retries) != 0 {
				t.Errorf("expected no OnRetry, got %d calls", len(retries))
			}
			if got := hooks.startAttempts(); len(got) != 1 || got[0] != 1 {
				t.Errorf("expected one send, attempt 1, got %v", got)
			}
		})
	}
}

type retryCall struct {
	info    RequestInfo
	attempt int
	err     error
}

// retryRecordingHooks keeps what the hooks were told about each send and each resend.
type retryRecordingHooks struct {
	NoopHooks
	mu      sync.Mutex
	starts  []RequestInfo
	retries []retryCall
	onRetry func()
}

func (h *retryRecordingHooks) OnRequestStart(ctx context.Context, info RequestInfo) context.Context {
	h.mu.Lock()
	h.starts = append(h.starts, info)
	h.mu.Unlock()
	return ctx
}

func (h *retryRecordingHooks) OnRetry(_ context.Context, info RequestInfo, attempt int, err error) {
	h.mu.Lock()
	h.retries = append(h.retries, retryCall{info: info, attempt: attempt, err: err})
	h.mu.Unlock()
	if h.onRetry != nil {
		h.onRetry()
	}
}

func (h *retryRecordingHooks) retryCalls() []retryCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]retryCall(nil), h.retries...)
}

func (h *retryRecordingHooks) startAttempts() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	attempts := make([]int, 0, len(h.starts))
	for _, start := range h.starts {
		attempts = append(attempts, start.Attempt)
	}
	return attempts
}

func (h *retryRecordingHooks) startURLs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	urls := make([]string, 0, len(h.starts))
	for _, start := range h.starts {
		urls = append(urls, start.URL)
	}
	return urls
}
