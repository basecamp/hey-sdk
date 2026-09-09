package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTopicsService_GetEntriesPage(t *testing.T) {
	var gotPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/topics/42/entries.json" {
			t.Errorf("expected the entry index, got %q", r.URL.Path)
		}
		gotPage = r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/topics/42/entries.json?page=eyJwYWdlIjozfQ>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id":13,"kind":"message","summary":"Quarterly planning"}]`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Topics().GetEntriesPage(context.Background(), 42, "eyJwYWdlIjoyfQ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPage != "eyJwYWdlIjoyfQ" {
		t.Errorf("expected the cursor to be passed through, got %q", gotPage)
	}
	if len(page.Entries) != 1 || page.Entries[0].Id != 13 {
		t.Fatalf("expected entry 13, got %+v", page.Entries)
	}
	if page.NextPage != "eyJwYWdlIjozfQ" {
		t.Errorf("expected the cursor for the next page, got %q", page.NextPage)
	}
}

// The last page carries no Link header, which is how a caller walking a thread is told it
// has read every entry.
func TestTopicsService_GetEntriesPageOnLastPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("page") {
			t.Errorf("expected no page on the first read, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":13,"kind":"message"}]`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Topics().GetEntriesPage(context.Background(), 42, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("expected one entry, got %+v", page.Entries)
	}
	if page.NextPage != "" {
		t.Errorf("expected no cursor past the last page, got %q", page.NextPage)
	}
}

func TestTopicsService_GetEntriesPageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	if _, err := client.Topics().GetEntriesPage(context.Background(), 42, ""); err == nil {
		t.Fatal("expected an error for a topic that is not there")
	}
}
