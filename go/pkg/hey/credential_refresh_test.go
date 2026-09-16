package hey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// requestEndHooks runs the test's own function as each request ends.
type requestEndHooks struct {
	NoopHooks
	onEnd func(info RequestInfo, result RequestResult)
}

func (h *requestEndHooks) OnRequestEnd(_ context.Context, info RequestInfo, result RequestResult) {
	h.onEnd(info, result)
}

// steeredAuth is a strategy whose refresh the test steers: it signs with whatever token
// is current, counts every refresh, and runs the test's own function for the refresh
// itself, which may block on a gate or fail.
type steeredAuth struct {
	token     atomic.Value
	signings  atomic.Int64
	refreshes atomic.Int64
	refresh   func(ctx context.Context) error
}

func newSteeredAuth(refresh func(ctx context.Context) error) *steeredAuth {
	auth := &steeredAuth{refresh: refresh}
	auth.token.Store("stale")
	return auth
}

func (a *steeredAuth) Authenticate(_ context.Context, req *http.Request) error {
	a.signings.Add(1)
	req.Header.Set("Authorization", "Bearer "+a.token.Load().(string))
	return nil
}

func (a *steeredAuth) Refresh(ctx context.Context) error {
	a.refreshes.Add(1)
	return a.refresh(ctx)
}

// renew is Refresh as the coordinator runs it.
func (a *steeredAuth) renew(ctx context.Context) bool {
	return a.Refresh(ctx) == nil
}

// tokenRecorder keeps the Authorization header of every request a server saw.
type tokenRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *tokenRecorder) record(req *http.Request) string {
	token := req.Header.Get("Authorization")
	r.mu.Lock()
	r.seen = append(r.seen, token)
	r.mu.Unlock()
	return token
}

func (r *tokenRecorder) count(token string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, seen := range r.seen {
		if seen == token {
			n++
		}
	}
	return n
}

func (r *tokenRecorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

// await waits for ch to be closed, within a bound generous enough for the CI, and fails
// the test rather than hanging it when that never happens: a handler or a refresh
// waiting on the test's own choreography carries on with the test already failed, so
// the request it would have held forever is answered and the test ends with the message.
func await(t *testing.T, ch <-chan struct{}, what string) bool {
	t.Helper()
	select {
	case <-ch:
		return true
	case <-time.After(10 * time.Second):
		t.Errorf("timed out waiting for %s", what)
		return false
	}
}

// waitFor is await on the test's own goroutine, where a timeout can end the test at once.
func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	if !await(t, ch, what) {
		t.FailNow()
	}
}

// Two requests signed with the same stale token, both answered 401 only once both are
// out, earn one refresh between them, and both are resent with the token it produced.
func TestRequestsSignedWithTheSameStaleTokenShareOneRefresh(t *testing.T) {
	recorder := &tokenRecorder{}
	var staleOut atomic.Int32
	bothOut := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			if staleOut.Add(1) == 2 {
				close(bothOut)
			}
			await(t, bothOut, "both stale requests to reach the server")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(func(context.Context) error { return nil })
	auth.refresh = func(context.Context) error {
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.Get(context.Background(), "/whatever.json")
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("expected the resend after the shared refresh to succeed: %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh for the two requests, got %d", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 2 || fresh != 2 {
		t.Errorf("expected two stale sends then two fresh ones, got %v", recorder.seen)
	}
}

// A refresh that fails is the answer of every request signed with the credentials it
// could not renew: a 401 that arrives after it has ended takes that answer without a
// refresh of its own, and neither request is resent.
func TestAFailedRefreshIsSharedByARequestWhose401ArrivesAfterIt(t *testing.T) {
	recorder := &tokenRecorder{}
	var staleOut atomic.Int32
	bothOut := make(chan struct{})
	firstDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		// Neither 401 goes out until both requests are at the server, and so signed
		// under the credentials the refresh fails to renew; the second is then answered
		// only once the first request has been failed, so it is read against a refresh
		// that has already run.
		arrival := staleOut.Add(1)
		if arrival == 2 {
			close(bothOut)
		}
		await(t, bothOut, "both stale requests to reach the server")
		if arrival == 2 {
			await(t, firstDone, "the first request to finish")
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(func(context.Context) error { return errors.New("issuer down") })
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.Get(context.Background(), "/whatever.json")
			errs <- err
		}()
	}
	first := <-errs
	close(firstDone)
	second := <-errs
	for _, err := range []error{first, second} {
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
			t.Errorf("expected an authentication error, got %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected the failed refresh to be run once and shared, got %d", refreshes)
	}
	if recorder.total() != 2 {
		t.Errorf("expected no resend after a failed refresh, got %v", recorder.seen)
	}
}

// A 401 that arrives while the refresh is still running waits for it and takes its
// answer, a failure included, rather than running a refresh of its own.
func TestARequestWhose401ArrivesDuringARefreshWaitsForItsAnswer(t *testing.T) {
	recorder := &tokenRecorder{}
	secondArrived := make(chan struct{})
	secondOut := make(chan struct{})
	entered := make(chan struct{})
	gate := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		// Both are signed before either 401 goes out; the second's is held until the
		// refresh the first's earns is running.
		switch r.URL.Path {
		case "/first.json":
			await(t, secondArrived, "the second request to reach the server")
		case "/second.json":
			close(secondArrived)
			await(t, entered, "the refresh to start")
			defer close(secondOut)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(func(context.Context) error {
		close(entered)
		await(t, gate, "the test to release the refresh")
		return errors.New("issuer down")
	})
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	errs := make(chan error, 2)
	for _, path := range []string{"/first.json", "/second.json"} {
		go func() {
			_, err := client.Get(context.Background(), path)
			errs <- err
		}()
	}
	waitFor(t, secondOut, "the second request to be answered 401")
	select {
	case err := <-errs:
		t.Fatalf("expected both requests to wait on the refresh in flight, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(gate)

	for range 2 {
		var apiErr *Error
		if err := <-errs; !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
			t.Errorf("expected an authentication error, got %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if recorder.total() != 2 {
		t.Errorf("expected no resend after a failed refresh, got %v", recorder.seen)
	}
}

// The second request here reads the coordinator directly, where the wait on the refresh
// in flight can be seen rather than inferred: its answer is not given until the refresh
// ends, and it is the refresh's own.
func TestCoordinatorHandsAWaiterTheAnswerOfTheRefreshInFlight(t *testing.T) {
	entered := make(chan struct{})
	gate := make(chan struct{})
	auth := newSteeredAuth(func(context.Context) error {
		close(entered)
		await(t, gate, "the test to release the refresh")
		return errors.New("issuer down")
	})
	var refresh credentialRefresh
	signedUnder := refresh.generation()

	answers := make(chan bool, 2)
	go func() { answers <- refresh.answer(context.Background(), signedUnder, auth.renew, time.Minute) }()
	waitFor(t, entered, "the refresh to start")
	go func() { answers <- refresh.answer(context.Background(), signedUnder, auth.renew, time.Minute) }()
	select {
	case answer := <-answers:
		t.Fatalf("expected no answer before the refresh ended, got %v", answer)
	case <-time.After(100 * time.Millisecond):
	}
	close(gate)
	for range 2 {
		if <-answers {
			t.Error("expected the failed refresh's answer")
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if after := refresh.generation(); after.runs != 1 || after.refreshes != 0 {
		t.Errorf("expected one run and no renewal, got %+v", after)
	}
}

// A request signed after a failed refresh asks again: its 401 is news, since the
// credentials it went out with were never the subject of a refresh.
func TestARequestSignedAfterAFailedRefreshRefreshesAgain(t *testing.T) {
	recorder := &tokenRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	var issuerDown atomic.Bool
	issuerDown.Store(true)
	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		if issuerDown.Load() {
			return errors.New("issuer down")
		}
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	var apiErr *Error
	if _, err := client.Get(context.Background(), "/whatever.json"); !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
		t.Fatalf("expected the failed refresh to surface as an authentication error, got %v", err)
	}
	issuerDown.Store(false)
	if _, err := client.Get(context.Background(), "/whatever.json"); err != nil {
		t.Fatalf("expected the request signed after the failure to be refreshed and resent: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 2 {
		t.Errorf("expected the later request to run a refresh of its own, got %d refreshes", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 2 || fresh != 1 {
		t.Errorf("expected two stale sends and one fresh one, got %v", recorder.seen)
	}
}

// A request signed with the stale token whose 401 arrives after another request's
// refresh has already renewed the credentials is resent with the new ones without a
// refresh of its own: someone else has answered it.
func TestA401OnCredentialsAnotherRequestAlreadyRefreshedIsResentWithoutARefresh(t *testing.T) {
	recorder := &tokenRecorder{}
	var staleOut atomic.Int32
	bothOut := make(chan struct{})
	firstDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			// Neither 401 goes out until both requests are at the server, and so signed
			// with the stale token; the second is then answered only once the first
			// request has been resent and answered, so it is read against a refresh
			// that has renewed the credentials.
			arrival := staleOut.Add(1)
			if arrival == 2 {
				close(bothOut)
			}
			await(t, bothOut, "both stale requests to reach the server")
			if arrival == 2 {
				await(t, firstDone, "the first request to finish")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.Get(context.Background(), "/whatever.json")
			errs <- err
		}()
	}
	if err := <-errs; err != nil {
		t.Errorf("expected the first request to be refreshed and resent: %v", err)
	}
	close(firstDone)
	if err := <-errs; err != nil {
		t.Errorf("expected the second request to be resent with the renewed credentials: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 2 || fresh != 2 {
		t.Errorf("expected two stale sends then two fresh ones, got %v", recorder.seen)
	}
}

// A request signed with the credentials a refresh produced, whose 401 says those were
// rejected too, is refreshed again: a 401 on renewed credentials is news.
func TestA401OnCredentialsARefreshProducedIsRefreshedAgain(t *testing.T) {
	recorder := &tokenRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresher" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		if auth.token.Load().(string) == "stale" {
			auth.token.Store("fresh")
		} else {
			auth.token.Store("fresher")
		}
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	// The first request's one resend, signed with "fresh", is refused too and surfaced.
	var apiErr *Error
	if _, err := client.Get(context.Background(), "/whatever.json"); !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
		t.Fatalf("expected the 401 on the resend to be surfaced, got %v", err)
	}
	if _, err := client.Get(context.Background(), "/whatever.json"); err != nil {
		t.Fatalf("expected the request signed with the renewed credentials to be refreshed and resent: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 2 {
		t.Errorf("expected a refresh per set of credentials, got %d", refreshes)
	}
	want := []string{"Bearer stale", "Bearer fresh", "Bearer fresh", "Bearer fresher"}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.seen) != len(want) {
		t.Fatalf("requests = %v, want %v", recorder.seen, want)
	}
	for i := range want {
		if recorder.seen[i] != want[i] {
			t.Errorf("requests = %v, want %v", recorder.seen, want)
			break
		}
	}
}

// The generated client's 401 path goes through the same coordinator: two generated
// operations signed with the same stale token earn one refresh between them.
func TestGeneratedOperationsSignedWithTheSameStaleTokenShareOneRefresh(t *testing.T) {
	recorder := &tokenRecorder{}
	var staleOut atomic.Int32
	bothOut := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			if staleOut.Add(1) == 2 {
				close(bothOut)
			}
			await(t, bothOut, "both stale requests to reach the server")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.Boxes().List(context.Background())
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("expected the resend after the shared refresh to succeed: %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh for the two operations, got %d", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 2 || fresh != 2 {
		t.Errorf("expected two stale sends then two fresh ones, got %v", recorder.seen)
	}
}

// The form path shares the refresh too, with a generated operation signed under the same
// credentials, since the coordinator is the client's rather than the path's.
func TestFormAndGeneratedRequestsShareOneRefresh(t *testing.T) {
	recorder := &tokenRecorder{}
	var staleOut atomic.Int32
	bothOut := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			if staleOut.Add(1) == 2 {
				close(bothOut)
			}
			await(t, bothOut, "both stale requests to reach the server")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/things" {
			http.Redirect(w, r, "/things/42", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))

	errs := make(chan error, 2)
	go func() {
		_, err := client.PostForm(context.Background(), "/things", url.Values{"name": {"x"}})
		errs <- err
	}()
	go func() {
		_, err := client.Boxes().List(context.Background())
		errs <- err
	}()
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("expected the resend after the shared refresh to succeed: %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh for the two requests, got %d", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 2 || fresh != 2 {
		t.Errorf("expected two stale sends then two fresh ones, got %v", recorder.seen)
	}
}

// A client derived for one account shares the root's coordinator, so a request from each
// signed with the same stale token earns one refresh between them.
func TestAnAccountScopedClientSharesTheRootClientsRefresh(t *testing.T) {
	recorder := &tokenRecorder{}
	var staleOut atomic.Int32
	bothOut := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			if staleOut.Add(1) == 2 {
				close(bothOut)
			}
			await(t, bothOut, "both stale requests to reach the server")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		auth.token.Store("fresh")
		return nil
	}
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))
	scoped := scopedTestClient(root, 42)

	errs := make(chan error, 2)
	for _, client := range []*Client{root, scoped} {
		go func() {
			_, err := client.Get(context.Background(), "/whatever.json")
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("expected the resend after the shared refresh to succeed: %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh across the root and the scoped client, got %d", refreshes)
	}
}

// A request whose context ends while it waits on a refresh another request started is
// answered for itself, and leaves the refresh to run to its end for the others: a
// refresh half done is a rotated token nobody holds.
func TestACancelledWaiterDoesNotAbandonTheRefreshItWaitedOn(t *testing.T) {
	recorder := &tokenRecorder{}
	secondArrived := make(chan struct{})
	secondOut := make(chan struct{})
	entered := make(chan struct{})
	gate := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			// Both are signed before either 401 goes out; the second's is held until
			// the refresh the first's earns is running.
			switch r.URL.Path {
			case "/first.json":
				await(t, secondArrived, "the second request to reach the server")
			case "/second.json":
				close(secondArrived)
				await(t, entered, "the refresh to start")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	// The second's 401 is in the client's hands once the request hook has seen it end.
	hooks := &requestEndHooks{onEnd: func(info RequestInfo, result RequestResult) {
		if strings.HasSuffix(info.URL, "/second.json") && result.StatusCode == http.StatusUnauthorized {
			close(secondOut)
		}
	}}

	var sawCancel atomic.Bool
	auth := newSteeredAuth(nil)
	auth.refresh = func(ctx context.Context) error {
		close(entered)
		await(t, gate, "the test to release the refresh")
		sawCancel.Store(ctx.Err() != nil)
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1), WithHooks(hooks))

	first := make(chan error, 1)
	go func() {
		_, err := client.Get(context.Background(), "/first.json")
		first <- err
	}()
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() {
		_, err := client.Get(ctx, "/second.json")
		second <- err
	}()
	waitFor(t, secondOut, "the second request to be answered 401")
	cancel()
	var apiErr *Error
	if err := <-second; !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
		t.Fatalf("expected the cancelled waiter to be answered with the 401 it drew, got %v", err)
	}
	close(gate)
	if err := <-first; err != nil {
		t.Fatalf("expected the request that started the refresh to be resent with its answer: %v", err)
	}
	if sawCancel.Load() {
		t.Error("expected the refresh to run on a context the waiter's cancellation does not reach")
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
}

// The refresh runs on a context the request that started it cannot cancel, bound instead
// by the client's request timeout.
func TestTheRefreshOutlivesTheRequestThatStartedIt(t *testing.T) {
	entered := make(chan struct{})
	gate := make(chan struct{})
	var deadlineSet, cancelled atomic.Bool
	auth := newSteeredAuth(func(ctx context.Context) error {
		_, hasDeadline := ctx.Deadline()
		deadlineSet.Store(hasDeadline)
		close(entered)
		await(t, gate, "the test to release the refresh")
		cancelled.Store(ctx.Err() != nil)
		return nil
	})
	var refresh credentialRefresh
	signedUnder := refresh.generation()

	ctx, cancel := context.WithCancel(context.Background())
	answer := make(chan bool, 1)
	go func() { answer <- refresh.answer(ctx, signedUnder, auth.renew, time.Minute) }()
	waitFor(t, entered, "the refresh to start")
	cancel()
	if <-answer {
		t.Error("expected the cancelled request to be answered false for itself")
	}
	close(gate)
	waitFor(t, waitForRun(&refresh), "the refresh to end")
	if cancelled.Load() {
		t.Error("expected the refresh to run on a context the request's cancellation does not reach")
	}
	if !deadlineSet.Load() {
		t.Error("expected the refresh to be bound by the client's timeout")
	}
	if after := refresh.generation(); after.refreshes != 1 || after.runs != 1 {
		t.Errorf("expected the refresh to run to its end and be counted, got %+v", after)
	}
}

// waitForRun is closed once the coordinator has no refresh in flight.
func waitForRun(refresh *credentialRefresh) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		for {
			refresh.mu.Lock()
			inFlight := refresh.inFlight
			refresh.mu.Unlock()
			if inFlight == nil {
				close(done)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	return done
}

// No request is signed while a refresh runs: a strategy that changes hands under the
// refresh is never read half way.
func TestNoRequestIsSignedWhileARefreshRuns(t *testing.T) {
	recorder := &tokenRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	entered := make(chan struct{})
	gate := make(chan struct{})
	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		close(entered)
		await(t, gate, "the test to release the refresh")
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	first := make(chan error, 1)
	go func() {
		_, err := client.Get(context.Background(), "/whatever.json")
		first <- err
	}()
	waitFor(t, entered, "the first request's refresh to start")

	// A request made while the refresh runs is signed only once it has ended, and so
	// goes out with the renewed credentials and never sees a 401.
	second := make(chan error, 1)
	go func() {
		_, err := client.Get(context.Background(), "/whatever.json")
		second <- err
	}()
	select {
	case err := <-second:
		t.Fatalf("expected the second request to wait for the refresh before being signed, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if recorder.total() != 1 {
		t.Fatalf("expected no request to go out while the refresh runs, got %v", recorder.seen)
	}
	close(gate)
	for _, errs := range []chan error{first, second} {
		if err := <-errs; err != nil {
			t.Errorf("expected both requests to succeed: %v", err)
		}
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 1 || fresh != 2 {
		t.Errorf("expected one stale send then two fresh ones, got %v", recorder.seen)
	}
}

// A request whose context has ended, or ends, while a refresh runs is not made to wait
// the refresh out: it is refused before it is signed, with its context's error, and it
// neither abandons nor duplicates the refresh, which the requests that need it get to
// the end of.
func TestARequestWhoseContextEndsDuringARefreshIsRefusedWithoutBeingSigned(t *testing.T) {
	recorder := &tokenRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) != "Bearer fresh" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	entered := make(chan struct{})
	gate := make(chan struct{})
	auth := newSteeredAuth(nil)
	auth.refresh = func(context.Context) error {
		close(entered)
		await(t, gate, "the test to release the refresh")
		auth.token.Store("fresh")
		return nil
	}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	first := make(chan error, 1)
	go func() {
		_, err := client.Get(context.Background(), "/whatever.json")
		first <- err
	}()
	waitFor(t, entered, "the first request's refresh to start")
	signings, sent := auth.signings.Load(), recorder.total()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expiring, expire := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer expire()
	for _, request := range []struct {
		ctx  context.Context
		want error
	}{{cancelled, context.Canceled}, {expiring, context.DeadlineExceeded}} {
		started := time.Now()
		_, err := client.Get(request.ctx, "/whatever.json")
		if !errors.Is(err, request.want) {
			t.Errorf("expected the request to be refused with %v, got %v", request.want, err)
		}
		if waited := time.Since(started); waited > 5*time.Second {
			t.Errorf("expected the request to return promptly, took %v", waited)
		}
	}
	if auth.signings.Load() != signings {
		t.Errorf("expected neither request to be signed, got %d signings", auth.signings.Load()-signings)
	}
	if recorder.total() != sent {
		t.Errorf("expected neither request to be sent, got %v", recorder.seen)
	}
	select {
	case err := <-first:
		t.Fatalf("expected the refresh to still be running, but the first request ended with %v", err)
	default:
	}

	close(gate)
	if err := <-first; err != nil {
		t.Fatalf("expected the request that started the refresh to be resent with its answer: %v", err)
	}
	if _, err := client.Get(context.Background(), "/whatever.json"); err != nil {
		t.Fatalf("expected a request after the refresh to go out signed with its answer: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	if stale, fresh := recorder.count("Bearer stale"), recorder.count("Bearer fresh"); stale != 1 || fresh != 2 {
		t.Errorf("expected one stale send then two fresh ones, got %v", recorder.seen)
	}
}

// The refresh is bound by the timeout of the client every send goes through: a client
// supplied with WithHTTPClient sets its own, and one supplied without any leaves the
// SDK's configured timeout as the bound.
func TestTheRefreshIsBoundByTheSuppliedClientsTimeout(t *testing.T) {
	for _, tc := range []struct {
		name     string
		supplied *http.Client
		want     time.Duration
	}{
		{name: "a supplied client's own timeout", supplied: &http.Client{Timeout: 3 * time.Second}, want: 3 * time.Second},
		{name: "the configured timeout when the supplied client has none", supplied: &http.Client{}, want: 7 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fresh" {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			t.Cleanup(server.Close)

			var remaining atomic.Int64
			auth := newSteeredAuth(nil)
			auth.refresh = func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Error("expected the refresh to carry a deadline")
				}
				remaining.Store(int64(time.Until(deadline)))
				auth.token.Store("fresh")
				return nil
			}
			client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithHTTPClient(tc.supplied), WithTimeout(7*time.Second))

			if _, err := client.Get(context.Background(), "/whatever.json"); err != nil {
				t.Fatalf("expected the resend after the refresh to succeed: %v", err)
			}
			if got := time.Duration(remaining.Load()); got <= tc.want-time.Second || got > tc.want {
				t.Errorf("expected the refresh to be bound by %v, got %v left", tc.want, got)
			}
		})
	}
}

// rotatingProvider is a token provider that, like AuthManager with an expiring token,
// renews the token as it hands it out: every AccessToken call after the first answers
// a new token, and counts itself. Refresh renews it too, and counts itself apart.
type rotatingProvider struct {
	mu        sync.Mutex
	tokens    []string
	handed    int
	refreshes atomic.Int64
}

func (p *rotatingProvider) AccessToken(context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handed++
	return p.tokens[min(p.handed, len(p.tokens))-1], nil
}

// current is the token the provider hands out now, without counting a hand-out.
func (p *rotatingProvider) current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokens[min(max(p.handed, 1), len(p.tokens))-1]
}

func (p *rotatingProvider) Refresh(context.Context) error {
	p.refreshes.Add(1)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens = append(p.tokens, "refreshed")
	p.handed = len(p.tokens)
	return nil
}

// A token the provider replaced on its own account while handing out the next one is a
// renewal the coordinator counts: a 401 on the old token, arriving after a request was
// signed with the new one, is resent with the new one and refreshes nothing.
func TestA401OnATokenTheProviderAlreadyRotatedIsResentWithoutARefresh(t *testing.T) {
	recorder := &tokenRecorder{}
	provider := &rotatingProvider{tokens: []string{"t0", "t1", "t1"}}
	firstArrived := make(chan struct{})
	secondArrived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch recorder.record(r) {
		case "Bearer t0":
			// The 401 goes out only once the second request, signed with the token
			// that replaced this one, has arrived.
			close(firstArrived)
			await(t, secondArrived, "the second request to reach the server")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "Bearer t1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.Error(w, "unexpected token", http.StatusTeapot)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, provider, WithMaxRetries(1))

	first := make(chan error, 1)
	go func() {
		_, err := client.Get(context.Background(), "/first.json")
		first <- err
	}()
	waitFor(t, firstArrived, "the first request to reach the server")
	if _, err := client.Get(context.Background(), "/second.json"); err != nil {
		t.Fatalf("expected the request signed with the rotated token to succeed: %v", err)
	}
	close(secondArrived)
	if err := <-first; err != nil {
		t.Fatalf("expected the first request to be resent with the rotated token: %v", err)
	}
	if refreshes := provider.refreshes.Load(); refreshes != 0 {
		t.Errorf("expected no refresh of a token the provider had already replaced, got %d", refreshes)
	}
	want := []string{"Bearer t0", "Bearer t1", "Bearer t1"}
	recorder.mu.Lock()
	seen := append([]string(nil), recorder.seen...)
	recorder.mu.Unlock()
	if len(seen) != len(want) || seen[0] != want[0] || seen[1] != want[1] || seen[2] != want[2] {
		t.Errorf("requests = %v, want %v", seen, want)
	}
	if handed := provider.handed; handed != 3 {
		t.Errorf("expected the provider to be asked once per signing and never probed, got %d", handed)
	}
}

// A refresh asks the provider what it would sign with before renewing: a token the
// provider has replaced since the request was signed is the renewal, and the refresher
// is not asked to spend the new token's refresh token again; a token it would still
// sign with is renewed, once, and a 401 on the renewed token refreshes once more.
func TestARefreshAsksTheProviderBeforeRenewingATokenItMayHaveReplaced(t *testing.T) {
	recorder := &tokenRecorder{}
	provider := &rotatingProvider{tokens: []string{"t0", "t1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch recorder.record(r) {
		case "Bearer t0", "Bearer t1":
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, provider, WithMaxRetries(1))

	// Signed with t0; the provider hands out t1 when the refresh asks, so t0 was
	// already replaced and no Refresh is made. The resend, signed with t1, is refused
	// too and surfaced: one refresh per request.
	var apiErr *Error
	if _, err := client.Get(context.Background(), "/whatever.json"); !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
		t.Fatalf("expected the resend's 401 to be surfaced, got %v", err)
	}
	if refreshes := provider.refreshes.Load(); refreshes != 0 {
		t.Errorf("expected no refresh of a token the provider had already replaced, got %d", refreshes)
	}
	// Signed with t1, which the provider would still sign with: refreshed, once, and
	// resent with what the refresh produced.
	if _, err := client.Get(context.Background(), "/whatever.json"); err != nil {
		t.Fatalf("expected the request to be refreshed and resent: %v", err)
	}
	if refreshes := provider.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh of the token the provider would still sign with, got %d", refreshes)
	}
	if got := recorder.count("Bearer refreshed"); got != 1 {
		t.Errorf("expected one send with the refreshed token, got %v", recorder.seen)
	}
	if provider.current() != "refreshed" {
		t.Errorf("expected the provider to hand out the refreshed token now, got %q", provider.current())
	}
}

// A refresh that cannot start because a signing under way has not ended within the
// bound — a strategy hung in Authenticate — is given up rather than left to wedge every
// request after it: the waiters are answered not renewed, nothing is counted, and once
// the signing ends the next 401 asks again.
func TestARefreshGivesUpWhenASigningDoesNotEndWithinTheBound(t *testing.T) {
	var refresh credentialRefresh
	release := make(chan struct{})
	signing := make(chan struct{})
	signed := make(chan error, 1)
	go func() {
		_, err := refresh.sign(context.Background(), func() (string, error) {
			close(signing)
			await(t, release, "the test to release the signing")
			return "", nil
		})
		signed <- err
	}()
	waitFor(t, signing, "the signing to start")

	auth := newSteeredAuth(func(context.Context) error { return nil })
	started := time.Now()
	if refresh.answer(context.Background(), refresh.generation(), auth.renew, 100*time.Millisecond) {
		t.Error("expected the refresh that could not start to answer not renewed")
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Errorf("expected the refresh to be given up at the bound, took %v", waited)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 0 {
		t.Errorf("expected the refresher not to be asked while a signing was under way, got %d", refreshes)
	}
	if after := refresh.generation(); after != (generation{}) {
		t.Errorf("expected a refresh that never ran to count for nothing, got %+v", after)
	}
	waitFor(t, waitForRun(&refresh), "the coordinator to have nothing in flight")

	close(release)
	if err := <-signed; err != nil {
		t.Fatalf("expected the signing to end normally once released: %v", err)
	}
	if !refresh.answer(context.Background(), refresh.generation(), auth.renew, time.Minute) {
		t.Error("expected the refresh after the signing ended to run and renew")
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh once the signing had ended, got %d", refreshes)
	}
}
