package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJournalServiceListPage(t *testing.T) {
	var gotPage, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/journal_entries.json" {
			t.Errorf("expected the journal entry index, got %q", r.URL.Path)
		}
		gotPage = r.URL.Query().Get("page")
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/calendar/journal_entries.json?page=eyJwYWdlIjozfQ&q=meetings+%26+notes>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id":13,"type":"CalendarJournalEntry","content":"Quarterly planning","starts_at":"2026-08-19T00:00:00Z"}]`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Journal().ListPage(context.Background(), "eyJwYWdlIjoyfQ", "meetings & notes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPage != "eyJwYWdlIjoyfQ" {
		t.Errorf("expected the cursor to be passed through, got %q", gotPage)
	}
	if gotQuery != "meetings & notes" {
		t.Errorf("expected the search query to be passed through, got %q", gotQuery)
	}
	if len(page.Entries) != 1 || page.Entries[0].Id != 13 || page.Entries[0].Content != "Quarterly planning" {
		t.Fatalf("expected journal entry 13, got %+v", page.Entries)
	}
	if page.NextPage != "eyJwYWdlIjozfQ" {
		t.Errorf("expected the cursor for the next page, got %q", page.NextPage)
	}
}

func TestJournalServiceListPageOnLastPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("page") || r.URL.Query().Has("q") {
			t.Errorf("expected no empty query parameters on the first unfiltered read, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":13,"type":"CalendarJournalEntry","content":"Quarterly planning"}]`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Journal().ListPage(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("expected one journal entry, got %+v", page.Entries)
	}
	if page.NextPage != "" {
		t.Errorf("expected no cursor past the last page, got %q", page.NextPage)
	}
}
