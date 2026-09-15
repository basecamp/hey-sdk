package hey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// A redirect answers for a URL other than the one asked for, so the cache entry of the one
// asked for neither goes out as the redirect target's validator nor takes the target's
// body: after a hop from /a to /b, /a is still revalidated with a's ETag and a 304 still
// answers a's body.
func TestRedirectNeitherCarriesNorTakesTheCacheEntryOfTheURLAskedFor(t *testing.T) {
	var mu sync.Mutex
	var aRequests int
	var bConditional = "unset"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/a.json":
			aRequests++
			switch aRequests {
			case 1:
				w.Header().Set("ETag", `"x"`)
				_, _ = io.WriteString(w, `{"which":"a"}`)
			case 2:
				http.Redirect(w, r, "/b.json", http.StatusFound)
			default:
				if r.Header.Get("If-None-Match") == `"x"` {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				t.Errorf("third GET /a validated with %q, want a's own ETag", r.Header.Get("If-None-Match"))
				w.Header().Set("ETag", `"x"`)
				_, _ = io.WriteString(w, `{"which":"a"}`)
			}
		case "/b.json":
			bConditional = r.Header.Get("If-None-Match")
			w.Header().Set("ETag", `"y"`)
			_, _ = io.WriteString(w, `{"which":"b"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))

	get := func() *Response {
		t.Helper()
		resp, err := client.Get(context.Background(), "/a.json")
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if resp := get(); string(resp.Data) != `{"which":"a"}` {
		t.Fatalf("first answer = %s, want a's body", resp.Data)
	}
	if resp := get(); string(resp.Data) != `{"which":"b"}` || resp.FromCache {
		t.Fatalf("answer through the redirect = %s (from cache %v), want b's body from the server", resp.Data, resp.FromCache)
	}
	if resp := get(); string(resp.Data) != `{"which":"a"}` || !resp.FromCache {
		t.Fatalf("answer after the redirect = %s (from cache %v), want a's body from the cache", resp.Data, resp.FromCache)
	}

	mu.Lock()
	defer mu.Unlock()
	if bConditional != "" {
		t.Errorf("b was asked to validate %q, want no validator: the entry was a's", bConditional)
	}
	if aRequests != 3 {
		t.Errorf("requests to /a = %d, want 3", aRequests)
	}
}

// The generated reads go through the same policy: a redirected ListBoxes neither
// validates with the entry of the URL asked for nor replaces it.
func TestGeneratedOperationsKeepTheCacheEntryOfTheURLAskedForAcrossARedirect(t *testing.T) {
	var mu sync.Mutex
	var boxRequests int
	var conditionals []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/boxes.json":
			boxRequests++
			conditionals = append(conditionals, r.Header.Get("If-None-Match"))
			if boxRequests == 2 {
				http.Redirect(w, r, "/elsewhere.json", http.StatusFound)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = io.WriteString(w, `[{"id":7,"name":"Imbox"}]`)
		case "/elsewhere.json":
			conditionals = append(conditionals, r.Header.Get("If-None-Match"))
			w.Header().Set("ETag", `"other"`)
			_, _ = io.WriteString(w, `[{"id":8,"name":"The Feed"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	for i, want := range []string{"Imbox", "The Feed", "Imbox"} {
		boxes, err := client.Boxes().List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if boxes == nil || len(*boxes) != 1 || (*boxes)[0].Name != want {
			t.Fatalf("call %d: boxes = %v, want %s", i+1, boxes, want)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	// The first read is unconditional, the redirected one validates with v1 and its target
	// with nothing, and the last validates with v1 still: the target's ETag never took over.
	if want := []string{"", `"v1"`, "", `"v1"`}; fmt.Sprint(conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want %v", conditionals, want)
	}
}

// A hop to another origin goes out without HEY's credentials, so a 401 from there rejected
// none of them: nothing is refreshed and nothing is sent again.
func TestUnauthorizedFromAHopThatCarriedNoCredentialsRefreshesNothing(t *testing.T) {
	var requests atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Errorf("the cross-origin hop carried %q", authorization)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, target.URL+"/export.json", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	client := NewClient(&Config{BaseURL: source.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	_, err := client.Get(context.Background(), "/export.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth || apiErr.Retryable {
		t.Fatalf("expected the authentication failure, got %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 0 {
		t.Errorf("expected no refresh, got %d: HEY's credentials were not the ones rejected", refreshes)
	}
	if requests.Load() != 2 {
		t.Errorf("expected the request and its one hop, got %d requests", requests.Load())
	}
}

// A 401 at the end of a chain that stayed on HEY did reject its credentials, and is still
// answered by a refresh and one more send of the whole chain.
func TestUnauthorizedOnASameOriginRedirectChainIsRetriedAfterARefresh(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		switch r.URL.Path {
		case "/start.json":
			http.Redirect(w, r, "/final.json", http.StatusFound)
		case "/final.json":
			if r.Header.Get("Authorization") != "Bearer fresh" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	if _, err := client.Get(context.Background(), "/start.json"); err != nil {
		t.Fatalf("expected the resend after a refresh to succeed: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"/start.json Bearer stale", "/final.json Bearer stale", "/start.json Bearer fresh", "/final.json Bearer fresh"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("requests = %v, want %v", seen, want)
	}
}

// signingAuth is a strategy that carries its credential under a header of its own rather
// than Authorization, and can renew it.
type signingAuth struct {
	signature atomic.Value
	refreshes atomic.Int64
}

func (a *signingAuth) Authenticate(_ context.Context, req *http.Request) error {
	req.Header.Set("X-Signature", a.signature.Load().(string))
	return nil
}

func (a *signingAuth) Refresh(context.Context) error {
	a.refreshes.Add(1)
	a.signature.Store("renewed")
	return nil
}

// Whatever header the strategy set is the credential, so a hop off the origin goes out
// without it too, and a 401 from there refreshes nothing.
func TestCrossOriginHopCarriesNoneOfTheHeadersTheStrategySet(t *testing.T) {
	var requests atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if signature := r.Header.Get("X-Signature"); signature != "" {
			t.Errorf("the cross-origin hop carried the signature %q", signature)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Signature") != "signed" {
			t.Errorf("HEY's own request lost its signature: %q", r.Header.Get("X-Signature"))
		}
		http.Redirect(w, r, target.URL+"/export.json", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	auth := &signingAuth{}
	auth.signature.Store("signed")
	client := NewClient(&Config{BaseURL: source.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(1))

	_, err := client.Get(context.Background(), "/export.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
		t.Fatalf("expected the authentication failure, got %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 0 {
		t.Errorf("expected no refresh, got %d: the signature never reached the target", refreshes)
	}
	if requests.Load() != 2 {
		t.Errorf("expected the request and its one hop, got %d requests", requests.Load())
	}
}

// A hop that stays on the origin keeps every header the strategy set, and a hop back to
// the origin after one that left it gets none of them back.
func TestSameOriginHopKeepsTheHeadersTheStrategySetUntilTheChainLeaves(t *testing.T) {
	var mu sync.Mutex
	signatures := map[string]string{}
	var elsewhere *httptest.Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		signatures[r.URL.Path] = r.Header.Get("X-Signature")
		mu.Unlock()
		switch r.URL.Path {
		case "/start.json":
			http.Redirect(w, r, "/next.json", http.StatusFound)
		case "/next.json":
			http.Redirect(w, r, elsewhere.URL+"/away.json", http.StatusFound)
		case "/back.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	elsewhere = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		signatures["away"] = r.Header.Get("X-Signature")
		mu.Unlock()
		http.Redirect(w, r, server.URL+"/back.json", http.StatusFound)
	}))
	t.Cleanup(elsewhere.Close)

	auth := &signingAuth{}
	auth.signature.Store("signed")
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))

	if _, err := client.Get(context.Background(), "/start.json"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := map[string]string{"/start.json": "signed", "/next.json": "signed", "away": "", "/back.json": ""}
	if fmt.Sprint(signatures) != fmt.Sprint(want) {
		t.Errorf("signatures by hop = %v, want %v", signatures, want)
	}
}

// A client supplied with WithHTTPClient runs the same redirect bookkeeping: the hop
// carries no validator, and the answer neither comes from nor goes into the entry of the
// URL asked for.
func TestSuppliedHTTPClientKeepsTheCacheEntryOfTheURLAskedForAcrossARedirect(t *testing.T) {
	var mu sync.Mutex
	var aRequests int
	bConditional := "unset"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/a.json":
			aRequests++
			switch aRequests {
			case 1:
				w.Header().Set("ETag", `"x"`)
				_, _ = io.WriteString(w, `{"which":"a"}`)
			case 2:
				http.Redirect(w, r, "/b.json", http.StatusFound)
			default:
				if r.Header.Get("If-None-Match") == `"x"` {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				t.Errorf("third GET /a validated with %q, want a's own ETag", r.Header.Get("If-None-Match"))
				w.Header().Set("ETag", `"x"`)
				_, _ = io.WriteString(w, `{"which":"a"}`)
			}
		case "/b.json":
			bConditional = r.Header.Get("If-None-Match")
			w.Header().Set("ETag", `"y"`)
			_, _ = io.WriteString(w, `{"which":"b"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	supplied := &http.Client{}
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithHTTPClient(supplied), WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	if supplied.CheckRedirect != nil {
		t.Fatal("the caller's own client was changed")
	}

	for i, want := range []string{`{"which":"a"}`, `{"which":"b"}`, `{"which":"a"}`} {
		resp, err := client.Get(context.Background(), "/a.json")
		if err != nil {
			t.Fatal(err)
		}
		if string(resp.Data) != want {
			t.Fatalf("answer %d = %s, want %s", i+1, resp.Data, want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if bConditional != "" {
		t.Errorf("b was asked to validate %q, want no validator: the entry was a's", bConditional)
	}
}

// The policy a supplied client came with decides each hop before the SDK does anything
// with it: a hop the policy declines is not followed, and the answer is the redirect.
func TestSuppliedHTTPClientKeepsItsOwnRedirectPolicy(t *testing.T) {
	var hops atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, "/elsewhere.json", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	var seen []string
	supplied := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		seen = append(seen, req.URL.Path)
		return http.ErrUseLastResponse
	}}
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithHTTPClient(supplied), WithMaxRetries(0))

	_, err := client.Get(context.Background(), "/start.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusFound {
		t.Fatalf("expected the redirect the caller's policy declined to follow, got %v", err)
	}
	if hops.Load() != 1 || len(seen) != 1 || seen[0] != "/elsewhere.json" {
		t.Errorf("hops = %d, policy consulted for %v; want one send and the policy asked once", hops.Load(), seen)
	}
}

// A generated operation whose chain left the origin gets the same answer to a 401 from
// there: no refresh and no resend, with or without the cache in the way.
func TestGeneratedOperationsRefreshNothingWhenACrossOriginHopAnswers401(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%v", cached), func(t *testing.T) {
			var requests atomic.Int64
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if authorization := r.Header.Get("Authorization"); authorization != "" {
					t.Errorf("the cross-origin hop carried %q", authorization)
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			}))
			t.Cleanup(target.Close)
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Redirect(w, r, target.URL+"/boxes.json", http.StatusFound)
			}))
			t.Cleanup(source.Close)

			auth := &refreshingAuth{refreshed: "fresh"}
			auth.token.Store("stale")
			opts := []ClientOption{WithAuthStrategy(auth), WithMaxRetries(1)}
			if cached {
				opts = append(opts, WithCache(NewCache(t.TempDir())))
			}
			root := NewClient(&Config{BaseURL: source.URL}, nil, opts...)
			client := scopedTestClient(root, 42)

			_, err := client.Boxes().List(context.Background())
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth {
				t.Fatalf("expected the authentication failure, got %v", err)
			}
			if refreshes := auth.refreshes.Load(); refreshes != 0 {
				t.Errorf("expected no refresh, got %d: HEY's credentials were not the ones rejected", refreshes)
			}
			if requests.Load() != 2 {
				t.Errorf("expected the request and its one hop, got %d requests", requests.Load())
			}
		})
	}
}

// A generated operation whose chain stayed on HEY still has its 401 answered by a refresh
// and one more send.
func TestGeneratedOperationsRetryOnceAfterRefreshOnASameOriginRedirectChain(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		switch r.URL.Path {
		case "/boxes.json":
			http.Redirect(w, r, "/all-boxes.json", http.StatusFound)
		case "/all-boxes.json":
			if r.Header.Get("Authorization") != "Bearer fresh" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	auth := &refreshingAuth{refreshed: "fresh"}
	auth.token.Store("stale")
	root := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(auth), WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the resend after a refresh to succeed: %v", err)
	}
	if refreshes := auth.refreshes.Load(); refreshes != 1 {
		t.Errorf("expected one refresh, got %d", refreshes)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"/boxes.json Bearer stale", "/all-boxes.json Bearer stale", "/boxes.json Bearer fresh", "/all-boxes.json Bearer fresh"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("requests = %v, want %v", seen, want)
	}
}

// The caller's policy decides whether a hop is taken, and the SDK's cleanup is the last
// thing to touch the hop before it goes out: a policy that copies every header from the
// first request, as many do, restores nothing a cross-origin target must not see.
func TestSuppliedRedirectPolicyCannotRestoreWhatTheSDKStrips(t *testing.T) {
	var targetHeaders http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"which":"b"}`)
	}))
	t.Cleanup(target.Close)

	var aRequests atomic.Int64
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if aRequests.Add(1) == 1 {
			w.Header().Set("ETag", `"x"`)
			_, _ = io.WriteString(w, `{"which":"a"}`)
			return
		}
		http.Redirect(w, r, target.URL+"/b.json", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	var policyCalls atomic.Int64
	supplied := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		policyCalls.Add(1)
		for name, values := range via[0].Header {
			req.Header[name] = values
		}
		return nil
	}}
	auth := &signingAuth{}
	auth.signature.Store("signed")
	client := NewClient(&Config{BaseURL: source.URL}, nil, WithAuthStrategy(auth),
		WithHTTPClient(supplied), WithMaxRetries(0), WithCache(NewCache(t.TempDir())))

	for _, want := range []string{`{"which":"a"}`, `{"which":"b"}`} {
		resp, err := client.Get(context.Background(), "/a.json")
		if err != nil {
			t.Fatal(err)
		}
		if string(resp.Data) != want {
			t.Fatalf("answer = %s, want %s", resp.Data, want)
		}
	}
	if policyCalls.Load() != 1 {
		t.Errorf("the caller's policy was consulted %d times, want once", policyCalls.Load())
	}
	for _, name := range []string{"If-None-Match", "X-Signature", "Authorization"} {
		if got := targetHeaders.Get(name); got != "" {
			t.Errorf("the cross-origin target received %s: %q", name, got)
		}
	}
}
