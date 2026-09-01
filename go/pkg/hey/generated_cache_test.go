package hey

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// boxServer answers ListBoxes with an ETag-versioned body: a conditional request
// naming the current version draws a 304, anything else the full body. It records
// the If-None-Match header of every request and counts the bodies it served.
type boxServer struct {
	mu           sync.Mutex
	etag         string
	body         string
	conditionals []string
	bodiesServed int
}

func (s *boxServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.conditionals = append(s.conditionals, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", s.etag)
		if r.Header.Get("If-None-Match") == s.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		s.bodiesServed++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, s.body)
	}
}

// A generated read revalidates with the ETag its last answer carried, and a 304 is
// answered from the cache: the server sends the body once, and every later call parses
// the stored copy.
func TestGeneratedOperationsRevalidateWithETags(t *testing.T) {
	backend := &boxServer{etag: `"v1"`, body: `[{"id":7,"name":"Imbox"}]`}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	for range 3 {
		boxes, err := client.Boxes().List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if boxes == nil || len(*boxes) != 1 || (*boxes)[0].Name != "Imbox" {
			t.Fatalf("boxes = %v, want the Imbox", boxes)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := []string{"", `"v1"`, `"v1"`}; fmt.Sprint(backend.conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want %v", backend.conditionals, want)
	}
	if backend.bodiesServed != 1 {
		t.Errorf("bodies served = %d, want 1", backend.bodiesServed)
	}
}

// A changed resource answers the conditional request with a fresh 200, and the cache
// follows: the next call revalidates against the new ETag and reads the new body.
func TestGeneratedOperationsRefreshTheCacheWhenTheResourceChanges(t *testing.T) {
	backend := &boxServer{etag: `"v1"`, body: `[{"id":7,"name":"Imbox"}]`}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatal(err)
	}

	backend.mu.Lock()
	backend.etag = `"v2"`
	backend.body = `[{"id":7,"name":"Imbox"},{"id":8,"name":"The Feed"}]`
	backend.mu.Unlock()

	for range 2 {
		boxes, err := client.Boxes().List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if boxes == nil || len(*boxes) != 2 {
			t.Fatalf("boxes = %v, want both boxes after the change", boxes)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := []string{"", `"v1"`, `"v2"`}; fmt.Sprint(backend.conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want %v", backend.conditionals, want)
	}
	if backend.bodiesServed != 2 {
		t.Errorf("bodies served = %d, want one per version", backend.bodiesServed)
	}
}

// An ETag whose body the cache no longer holds cannot answer a 304, so the request
// goes out unconditional and the fresh 200 restores the entry.
func TestGeneratedOperationsSkipConditionalRequestsWithoutACachedBody(t *testing.T) {
	backend := &boxServer{etag: `"v1"`, body: `[{"id":7,"name":"Imbox"}]`}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	cacheDir := t.TempDir()
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(cacheDir)))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(cacheDir, "responses")); err != nil {
		t.Fatal(err)
	}

	boxes, err := client.Boxes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if boxes == nil || len(*boxes) != 1 {
		t.Fatalf("boxes = %v, want the refetched Imbox", boxes)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := []string{"", ""}; fmt.Sprint(backend.conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want unconditional requests", backend.conditionals)
	}
	if backend.bodiesServed != 2 {
		t.Errorf("bodies served = %d, want a refetch", backend.bodiesServed)
	}
}

// Authentication that sets no Authorization header leaves the cache without a
// credential to key by, so generated reads never go out conditional — the same
// stance the hand-written path takes.
func TestGeneratedOperationsWithoutAuthorizationBypassTheResponseCache(t *testing.T) {
	backend := &boxServer{etag: `"v1"`, body: `[{"id":7,"name":"Imbox"}]`}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, nil,
		WithAuthStrategy(testHeaderAuth{identity: "personal"}),
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	for range 2 {
		if _, err := client.Boxes().List(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := []string{"", ""}; fmt.Sprint(backend.conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want caching bypassed", backend.conditionals)
	}
}

// Accounts share one cache but not entries: the account filter each scoped client
// stamps on its URLs keys them apart, so a 304 for one account can never answer
// with another account's boxes.
func TestGeneratedOperationsCacheEachAccountSeparately(t *testing.T) {
	var mu sync.Mutex
	conditionals := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account := r.URL.Query().Get(filteredAccountIDParameter)
		mu.Lock()
		conditionals[account] = append(conditionals[account], r.Header.Get("If-None-Match"))
		mu.Unlock()
		etag := `"account-` + account + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id":%s,"name":"Imbox"}]`, account)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	personal := scopedTestClient(root, 42)
	work := scopedTestClient(root, 43)

	for _, client := range []*Client{personal, work, personal, work} {
		boxes, err := client.Boxes().List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if boxes == nil || len(*boxes) != 1 || (*boxes)[0].Id != client.accountID {
			t.Fatalf("account %d read another account's boxes: %v", client.accountID, boxes)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for account, requests := range conditionals {
		want := []string{"", `"account-` + account + `"`}
		if fmt.Sprint(requests) != fmt.Sprint(want) {
			t.Errorf("account %s conditionals = %v, want %v", account, requests, want)
		}
	}
}

// A body the cap refuses fails inside store's read, and the failure is kept for the
// generated parser's own read to surface: the caller gets the error after exactly one
// request, because an error from Do would send the retry loop after a response it
// already has.
func TestGeneratedOperationsDoNotRetryABodyTheCacheFailedToRead(t *testing.T) {
	client, hits := newCappedTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		serveOversizedJSON(false)(w, r)
	}, WithCache(NewCache(t.TempDir())))

	_, err := client.Messages().Get(context.Background(), 1)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge through the cache-enabled generated client", err)
	}
	if hits.Load() != 1 {
		t.Errorf("server saw %d requests, want 1: a refused body must not be retried", hits.Load())
	}
}

// A custom transport carries no body cap, so store can read a body past the configured
// bound. The read guard would never serve such an entry, so it is not written: the
// answer still parses in full, and the next read goes out unconditional.
func TestGeneratedOperationsDoNotCacheABodyPastTheBound(t *testing.T) {
	backend := &boxServer{etag: `"v1"`,
		body: `[{"id":7,"name":"` + strings.Repeat("x", 2048) + `"}]`}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithHTTPClient(&http.Client{}), WithMaxResponseBodyBytes(1024),
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	for range 2 {
		boxes, err := client.Boxes().List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if boxes == nil || len(*boxes) != 1 {
			t.Fatalf("boxes = %v, want the oversized box parsed in full", boxes)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := []string{"", ""}; fmt.Sprint(backend.conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want no conditional requests for an unstored body", backend.conditionals)
	}
	if backend.bodiesServed != 2 {
		t.Errorf("bodies served = %d, want every answer in full", backend.bodiesServed)
	}
}

// HEY regenerates pagination headers on every response — geared_pagination sets
// X-Total-Count and Link in an after_action that runs on 304s too — so a revalidated
// read takes its pagination state from the 304 itself and the synthesized 200
// carries it through to the wrappers that parse those headers.
func TestGeneratedOperationsReadPaginationHeadersFromTheRevalidation(t *testing.T) {
	var bodiesServed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("X-Total-Count", "7")
		w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`?page=abc123>; rel="next"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		bodiesServed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"name":"Receipts"}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	for call := range 2 {
		page, err := client.Folders().GetPage(context.Background(), 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		if page.TotalCount != 7 || page.NextPage != "abc123" {
			t.Errorf("call %d: TotalCount = %d, NextPage = %q, want 7 and %q",
				call, page.TotalCount, page.NextPage, "abc123")
		}
	}
	if bodiesServed.Load() != 1 {
		t.Errorf("bodies served = %d, want the second read answered from the cache", bodiesServed.Load())
	}
}

// A client without a cache sends generated requests untouched: no conditional
// headers, every answer fetched in full.
func TestGeneratedOperationsWithoutACacheSendPlainRequests(t *testing.T) {
	backend := &boxServer{etag: `"v1"`, body: `[{"id":7,"name":"Imbox"}]`}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0))
	client := scopedTestClient(root, 42)

	for range 2 {
		if _, err := client.Boxes().List(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := []string{"", ""}; fmt.Sprint(backend.conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want no conditional requests", backend.conditionals)
	}
	if backend.bodiesServed != 2 {
		t.Errorf("bodies served = %d, want every answer in full", backend.bodiesServed)
	}
}
