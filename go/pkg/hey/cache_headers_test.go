package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// pagedServer answers a two-page collection with an ETag per page and the pagination
// headers geared_pagination sends on a 200. A conditional request naming the page's ETag
// draws a bare 304 carrying the ETag alone, as a 304 is allowed to: whatever else the
// page came with is the cache's to supply. It records every request's If-None-Match and
// counts the bodies it served.
type pagedServer struct {
	mu           sync.Mutex
	conditionals []string
	bodiesServed int
	// notModifiedHeaders are added to every 304, so a test can have one carry news.
	notModifiedHeaders http.Header
}

func (s *pagedServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		etag := `"page-` + page + `"`
		s.conditionals = append(s.conditionals, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			for name, values := range s.notModifiedHeaders {
				w.Header()[name] = values
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		s.bodiesServed++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "2")
		w.Header().Set("Set-Cookie", "session=abc")
		if page == "1" {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
		}
		// A folder is an object with its postings; any other path is a list.
		if strings.HasPrefix(r.URL.Path, "/42/folders/") || strings.HasPrefix(r.URL.Path, "/folders/") {
			_, _ = fmt.Fprintf(w, `{"id":%s,"name":"Receipts"}`, page)
			return
		}
		_, _ = fmt.Fprintf(w, `[{"id":%s}]`, page)
	}
}

func (s *pagedServer) snapshot() ([]string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.conditionals...), s.bodiesServed
}

// A hand-written walk of a paginated collection revalidates every page, and each bare 304
// is answered with the headers stored with the page: the Link the 304 left out still leads
// to page two, so the second walk reads both pages from the cache rather than stopping
// after the first.
func TestPaginationSurvivesARevalidationThatOmitsItsHeaders(t *testing.T) {
	backend := &pagedServer{}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))

	for walk := range 2 {
		items, err := client.GetAll(context.Background(), "/items.json")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatalf("walk %d: %d items, want both pages' (%s)", walk, len(items), items)
		}
	}

	conditionals, bodies := backend.snapshot()
	if want := []string{"", "", `"page-1"`, `"page-2"`}; fmt.Sprint(conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want %v", conditionals, want)
	}
	if bodies != 2 {
		t.Errorf("bodies served = %d, want each page once", bodies)
	}

	response, err := client.Get(context.Background(), "/items.json")
	if err != nil {
		t.Fatal(err)
	}
	if !response.FromCache {
		t.Fatal("the third read was not answered from the cache")
	}
	if got := response.Headers.Get("X-Total-Count"); got != "2" {
		t.Errorf("X-Total-Count = %q, want the stored 2", got)
	}
	if got := response.Headers.Get("Set-Cookie"); got != "" {
		t.Errorf("Set-Cookie = %q, want a credential kept out of the cache", got)
	}
}

// A generated paginated read takes its pagination state from the stored headers when the
// 304 carries none, and a header the 304 does carry replaces the stored one — in the
// answer and in the entry, so the next bare 304 answers with the update (RFC 9111 §4.3.4).
func TestGeneratedOperationsKeepPaginationHeadersThroughABareRevalidation(t *testing.T) {
	var bodiesServed atomic.Int32
	var mu sync.Mutex
	notModified := http.Header{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			mu.Lock()
			for name, values := range notModified {
				w.Header()[name] = values
			}
			mu.Unlock()
			w.WriteHeader(http.StatusNotModified)
			return
		}
		bodiesServed.Add(1)
		w.Header().Set("X-Total-Count", "7")
		w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`?page=abc123>; rel="next"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"name":"Receipts"}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(NewCache(t.TempDir())))
	client := scopedTestClient(root, 42)

	read := func(call string, wantTotal int) {
		t.Helper()
		page, err := client.Folders().GetPage(context.Background(), 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		if page.TotalCount != wantTotal || page.NextPage != "abc123" {
			t.Errorf("%s: TotalCount = %d, NextPage = %q, want %d and %q",
				call, page.TotalCount, page.NextPage, wantTotal, "abc123")
		}
	}

	read("first read", 7)
	read("bare 304", 7)

	mu.Lock()
	notModified.Set("X-Total-Count", "8")
	mu.Unlock()
	read("304 carrying a new count", 8)

	mu.Lock()
	notModified = http.Header{}
	mu.Unlock()
	read("bare 304 after the update", 8)

	if bodiesServed.Load() != 1 {
		t.Errorf("bodies served = %d, want every later read answered from the cache", bodiesServed.Load())
	}
}

// An entry written before the cache kept headers has a body and an ETag but nothing to
// answer a 304's missing Link with, so it is no better than a miss: the read goes out
// unconditional, and the fresh 200 replaces it with an entry that has its headers.
func TestCacheEntriesWithoutHeadersAreRefetched(t *testing.T) {
	backend := &pagedServer{}
	server := httptest.NewServer(backend.handler())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	cache := NewCache(dir)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(cache))
	scoped := scopedTestClient(client, 42)

	// Seed both paths' entries, then strip their headers as an older SDK would have left them.
	if _, err := client.Get(context.Background(), "/items.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Folders().GetPage(context.Background(), 1, nil); err != nil {
		t.Fatal(err)
	}
	headerFiles, err := filepath.Glob(filepath.Join(dir, "responses", "*.headers"))
	if err != nil || len(headerFiles) != 2 {
		t.Fatalf("header files = %v (%v), want one per entry", headerFiles, err)
	}
	for _, file := range headerFiles {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := client.Get(context.Background(), "/items.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Folders().GetPage(context.Background(), 1, nil); err != nil {
		t.Fatal(err)
	}
	conditionals, bodies := backend.snapshot()
	if want := []string{"", "", "", ""}; fmt.Sprint(conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want every read unconditional", conditionals)
	}
	if bodies != 4 {
		t.Errorf("bodies served = %d, want 4", bodies)
	}

	response, err := client.Get(context.Background(), "/items.json")
	if err != nil {
		t.Fatal(err)
	}
	if !response.FromCache || response.Headers.Get("Link") == "" {
		t.Errorf("FromCache = %v, Link = %q: want the refetched entry to answer with its headers",
			response.FromCache, response.Headers.Get("Link"))
	}
}

// The headers stored beside a body leave out credentials and the connection's own fields,
// and read back under canonical names whatever case another SDK sharing the directory
// wrote them in.
func TestCacheStoresHeadersWithoutCredentials(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	key := cache.Key("https://example.com/boxes.json", "tok")

	header := http.Header{}
	header.Set("Link", `</boxes.json?page=2>; rel="next"`)
	header.Set("X-Total-Count", "2")
	header.Set("Set-Cookie", "session=abc")
	header.Set("Connection", "keep-alive, X-Hop")
	header.Set("X-Hop", "1")
	header.Set("Keep-Alive", "timeout=5")
	header.Set("Proxy-Authenticate", `Basic realm="proxy"`)
	header.Set("Proxy-Authentication-Info", "nextnonce=abc")
	if err := cache.SetResponse(key, []byte(`[]`), `"v1"`, header); err != nil {
		t.Fatal(err)
	}
	got := cache.GetHeaders(key)
	if got.Get("Link") == "" || got.Get("X-Total-Count") != "2" {
		t.Errorf("headers = %v, want Link and X-Total-Count kept", got)
	}
	for _, name := range []string{"Set-Cookie", "Connection", "X-Hop", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authentication-Info"} {
		if got.Get(name) != "" {
			t.Errorf("%s = %q, want it left out of the cache", name, got.Get(name))
		}
	}

	// A header built by hand need not use canonical names, and is filtered all the same.
	handBuilt := http.Header{
		"x-total-count": {"2"},
		"set-cookie":    {"session=abc"},
		"authorization": {"Bearer token"},
		"connection":    {"x-hop"},
		"x-hop":         {"1"},
	}
	if err := cache.SetResponse(key, []byte(`[]`), `"v1"`, handBuilt); err != nil {
		t.Fatal(err)
	}
	if got := cache.GetHeaders(key); len(got) != 1 || got.Get("X-Total-Count") != "2" {
		t.Errorf("headers = %v, want only X-Total-Count kept from non-canonical names", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "responses", key+".headers"),
		[]byte(`{"link":["</boxes.json?page=3>; rel=\"next\""],"x-total-count":["3"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := cache.GetHeaders(key); got.Get("X-Total-Count") != "3" || got.Get("Link") == "" {
		t.Errorf("headers = %v, want lowercase names read back canonically", got)
	}

	if err := cache.Invalidate(key); err != nil {
		t.Fatal(err)
	}
	if got := cache.GetHeaders(key); got != nil {
		t.Errorf("headers after Invalidate = %v, want none", got)
	}
}

// onlyEntry is the key of the one entry a test's cache holds.
func onlyEntry(t *testing.T, dir string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "responses", "*.headers"))
	if err != nil || len(files) != 1 {
		t.Fatalf("header files = %v (%v), want exactly one entry", files, err)
	}
	return strings.TrimSuffix(filepath.Base(files[0]), ".headers")
}

// A 304 is answered from the entry the request went out conditional on, even when another
// request stores a newer version while it is in flight; and the newer version stays, since
// nothing the 304 confirms is newer than it. Both the hand-written path and the generated
// client's doer read the entry whole before sending.
func TestARevalidationAnswersTheVersionItConfirms(t *testing.T) {
	for _, reader := range []struct {
		name string
		read func(*Client) (body string, total string, err error)
	}{
		{"hand-written", func(c *Client) (string, string, error) {
			resp, err := c.Get(context.Background(), "/items.json")
			if err != nil {
				return "", "", err
			}
			return string(resp.Data), resp.Headers.Get("X-Total-Count"), nil
		}},
		{"generated", func(c *Client) (string, string, error) {
			page, err := scopedTestClient(c, 42).Folders().GetPage(context.Background(), 1, nil)
			if err != nil {
				return "", "", err
			}
			return page.Folder.Name, strconv.Itoa(page.TotalCount), nil
		}},
	} {
		t.Run(reader.name, func(t *testing.T) {
			arrived := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", `"v1"`)
				if r.Header.Get("If-None-Match") == `"v1"` {
					close(arrived)
					<-release
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Total-Count", "2")
				if strings.Contains(r.URL.Path, "/folders/") {
					_, _ = fmt.Fprint(w, `{"id":1,"name":"v1"}`)
					return
				}
				_, _ = fmt.Fprint(w, `[{"id":"v1"}]`)
			}))
			t.Cleanup(server.Close)

			dir := t.TempDir()
			cache := NewCache(dir)
			client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
				WithMaxRetries(0), WithCache(cache))
			if _, _, err := reader.read(client); err != nil {
				t.Fatal(err)
			}
			key := onlyEntry(t, dir)

			type answer struct {
				body, total string
				err         error
			}
			answered := make(chan answer, 1)
			go func() {
				body, total, err := reader.read(client)
				answered <- answer{body, total, err}
			}()
			<-arrived
			newer := `[{"id":"v2"}]`
			if strings.Contains(reader.name, "generated") {
				newer = `{"id":1,"name":"v2"}`
			}
			if err := cache.SetResponse(key, []byte(newer), `"v2"`, http.Header{"X-Total-Count": {"99"}}); err != nil {
				t.Fatal(err)
			}
			close(release)
			got := <-answered
			if got.err != nil {
				t.Fatal(got.err)
			}
			if !strings.Contains(got.body, "v1") || got.total != "2" {
				t.Errorf("answered %q with X-Total-Count %q, want v1's body and count", got.body, got.total)
			}
			if etag, body, header, _ := cache.lookup(key); etag != `"v2"` || string(body) != newer || header.Get("X-Total-Count") != "99" {
				t.Errorf("entry = %s %s %v, want the newer version left as stored", etag, body, header)
			}
		})
	}
}

// What a 304 carries reaches the caller whole, and the entry keeps what it may: a new
// validator is the one the next read goes out with, a fresh Set-Cookie is the caller's but
// never the cache's, and the body on disk is left as it was rather than written again.
func TestARevalidationKeepsItsMetadataWithoutRewritingTheBody(t *testing.T) {
	var conditionals []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		conditionals = append(conditionals, r.Header.Get("If-None-Match"))
		mu.Unlock()
		switch r.Header.Get("If-None-Match") {
		case `"v1"`:
			w.Header().Set("ETag", `"v1-gzip"`)
			w.Header().Set("Set-Cookie", "session=fresh")
			w.WriteHeader(http.StatusNotModified)
		case `"v1-gzip"`:
			w.Header().Set("ETag", `"v1-gzip"`)
			w.WriteHeader(http.StatusNotModified)
		default:
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("X-Total-Count", "2")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `[{"id":1}]`)
		}
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	cache := NewCache(dir)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(0), WithCache(cache))
	if _, err := client.Get(context.Background(), "/items.json"); err != nil {
		t.Fatal(err)
	}
	key := onlyEntry(t, dir)
	before, err := os.Stat(filepath.Join(dir, "responses", key+".body"))
	if err != nil {
		t.Fatal(err)
	}

	revalidated, err := client.Get(context.Background(), "/items.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := revalidated.Headers.Get("Set-Cookie"); got != "session=fresh" {
		t.Errorf("Set-Cookie = %q, want the 304's own handed to the caller", got)
	}
	if got := revalidated.Headers.Get("X-Total-Count"); got != "2" {
		t.Errorf("X-Total-Count = %q, want the stored 2", got)
	}
	if got := cache.GetHeaders(key).Get("Set-Cookie"); got != "" {
		t.Errorf("stored Set-Cookie = %q, want it kept out of the cache", got)
	}
	if got := cache.GetETag(key); got != `"v1-gzip"` {
		t.Errorf("stored ETag = %q, want the one the 304 carried", got)
	}
	after, err := os.Stat(filepath.Join(dir, "responses", key+".body"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("the body file was written again on a 304")
	}

	if _, err := client.Get(context.Background(), "/items.json"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"", `"v1"`, `"v1-gzip"`}; fmt.Sprint(conditionals) != fmt.Sprint(want) {
		t.Errorf("conditionals = %v, want %v", conditionals, want)
	}
}
