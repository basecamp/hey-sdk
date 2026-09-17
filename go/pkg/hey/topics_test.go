package hey

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestTopicsService_UpdateRenamesViaJSONPatch(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotCT     string
		gotAccept string
		gotBody   string
		requests  int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		if r.URL.Path == "/topics/42" || r.URL.Path == "/topics/42.html" {
			t.Fatal("topic rename redirect was followed")
		}
		w.Header().Set("Location", "/topics/42")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	if err := client.Topics().Update(context.Background(), 42, "Amex charge"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/topics/42.json" {
		t.Errorf("path = %q, want /topics/42.json", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotAccept != "*/*" {
		t.Errorf("accept = %q, want */* (doBodyRequest)", gotAccept)
	}
	if gotBody != `{"name":"Amex charge"}` {
		t.Errorf("body = %q", gotBody)
	}
}

func TestTopicsService_UpdateDoesNotSendSubjectField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "subject") {
			t.Fatalf("body must not use subject: %s", body)
		}
		w.Header().Set("Location", "/topics/7")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))
	if err := client.Topics().Update(context.Background(), 7, "Bancolombia alert"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTopicsService_UpdateSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))
	if err := client.Topics().Update(context.Background(), 42, "x"); err == nil {
		t.Fatal("expected a 403 to surface")
	}
}
