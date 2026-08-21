package hey

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// The apps sync this endpoint for the Screener badge, so the count must not drag the queue
// along with it.
func TestClearancesService_PendingCountAsksForTheCountAlone(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/clearances.json" {
			t.Errorf("expected GET /clearances.json, got %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pending_clearances_count":3,"signed_stream_name":"abc"}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	count, err := client.Clearances().PendingCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 pending, got %d", count)
	}
	if gotQuery != "" {
		t.Errorf("expected no query, got %q", gotQuery)
	}
}

// The Screener's stream name comes with the count, so a client can follow it without a
// second read — and without the queue either.
func TestClearancesService_SummaryCarriesTheStreamToFollow(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/clearances.json" {
			t.Errorf("expected GET /clearances.json, got %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pending_clearances_count":3,"signed_stream_name":"eyJpZGVudGl0eSI6MX0--sig"}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	summary, err := client.Clearances().Summary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.PendingClearancesCount != 3 {
		t.Errorf("expected 3 pending, got %d", summary.PendingClearancesCount)
	}
	if summary.SignedStreamName != "eyJpZGVudGl0eSI6MX0--sig" {
		t.Errorf("stream name = %q", summary.SignedStreamName)
	}
	if len(summary.Clearances) != 0 {
		t.Errorf("expected no queue, got %d clearances", len(summary.Clearances))
	}
	if gotQuery != "" {
		t.Errorf("expected no query, got %q", gotQuery)
	}
}

func TestClearancesService_Pending(t *testing.T) {
	var gotInclude, gotPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/clearances.json" {
			t.Errorf("expected GET /clearances.json, got %s %s", r.Method, r.URL.Path)
		}
		gotInclude = r.URL.Query().Get("include_clearances")
		gotPage = r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pending_clearances_count":1,"clearances":[
			{"id":91,"status":"pending","petitioner":{"id":51,"name":"Hollis Heimboch","email_address":"hollis@example.com"},
			 "most_recent_entry":{"id":71,"subject":"New numbers!","topic_id":81,"summary":"The latest sales numbers"}}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	summary, err := client.Clearances().Pending(context.Background(), "eyJwYWdlIjoyfQ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotInclude != "true" {
		t.Errorf("expected include_clearances=true, got %q", gotInclude)
	}
	if gotPage != "eyJwYWdlIjoyfQ" {
		t.Errorf("expected the page token to be passed through, got %q", gotPage)
	}
	if len(summary.Clearances) != 1 {
		t.Fatalf("expected one clearance, got %+v", summary.Clearances)
	}

	clearance := summary.Clearances[0]
	if clearance.Petitioner.EmailAddress != "hollis@example.com" {
		t.Errorf("expected the petitioner, got %+v", clearance.Petitioner)
	}
	if clearance.MostRecentEntry.Subject != "New numbers!" {
		t.Errorf("expected the subject, got %q", clearance.MostRecentEntry.Subject)
	}
	if clearance.MostRecentEntry.TopicId != 81 {
		t.Errorf("expected the topic to reply to, got %d", clearance.MostRecentEntry.TopicId)
	}
}

func TestClearancesService_PendingPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if page := r.URL.Query().Get("page"); page != "eyJwYWdlIjoyfQ" {
			t.Errorf("expected the page token to be passed through, got %q", page)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/clearances.json?page=eyJwYWdlIjozfQ>; rel="next"`)
		_, _ = w.Write([]byte(`{"pending_clearances_count":24,"clearances":[
			{"id":91,"status":"pending","petitioner":{"id":51,"name":"Hollis Heimboch"}}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Clearances().PendingPage(context.Background(), "eyJwYWdlIjoyfQ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Clearances) != 1 || page.Clearances[0].Id != 91 {
		t.Errorf("expected clearance 91, got %+v", page.Clearances)
	}
	if page.PendingCount != 24 {
		t.Errorf("expected 24 waiting, got %d", page.PendingCount)
	}
	if page.NextPage != "eyJwYWdlIjozfQ" {
		t.Errorf("expected the cursor for the next page, got %q", page.NextPage)
	}
}

// The queue's last page carries no Link header, which is how a caller walking it is told
// it has reached the end.
func TestClearancesService_PendingPageOnLastPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pending_clearances_count":1,"clearances":[{"id":91,"status":"pending"}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Clearances().PendingPage(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.NextPage != "" {
		t.Errorf("next page = %q, want none", page.NextPage)
	}
}

func TestClearancesService_ScreenedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/my/clearances.json" {
			t.Errorf("expected GET /my/clearances.json, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/my/clearances.json?page=eyJwYWdlIjoyfQ>; rel="next"`)
		_, _ = w.Write([]byte(`{"clearances":[{"id":91,"status":"approved","petitioner":{"id":51,"name":"Glenn"}}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.Clearances().ScreenedPage(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Clearances) != 1 || page.Clearances[0].Petitioner.Name != "Glenn" {
		t.Errorf("expected Glenn's decision, got %+v", page.Clearances)
	}
	if page.NextPage != "eyJwYWdlIjoyfQ" {
		t.Errorf("expected the cursor for the next page, got %q", page.NextPage)
	}
}

func TestClearancesService_Screen(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/clearances/91.json" {
			t.Errorf("expected PATCH /clearances/91.json, got %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":91,"status":"approved"}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	clearance, err := client.Clearances().Screen(context.Background(), 91, ClearanceApproved,
		ScreenOptions{DesignationBoxID: 7, MarkTopicsAsSeen: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clearance.Status != "approved" {
		t.Errorf("expected the updated clearance, got %+v", clearance)
	}
	if gotBody["status"] != "approved" {
		t.Errorf("expected the status at the top level, got %+v", gotBody)
	}
	if gotBody["designation_box_id"] != float64(7) {
		t.Errorf("expected the designation box, got %+v", gotBody["designation_box_id"])
	}
	if gotBody["mark_topics_as_seen"] != true {
		t.Errorf("expected mark_topics_as_seen, got %+v", gotBody["mark_topics_as_seen"])
	}
	// Options left alone stay off the wire entirely, since HEY reads them for truthiness.
	if _, sent := gotBody["spam"]; sent {
		t.Errorf("expected no spam key, got %+v", gotBody)
	}
}

func TestClearancesService_ScreenExplicitRetryOverrideReplaysCompleteBody(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requests.Add(1)
		var body generated.UpdateClearanceJSONRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("request %d body: %v", request, err)
		}
		if body.Status != ClearanceApproved || body.DesignationBoxId != 215744 {
			t.Errorf("request %d body = %+v", request, body)
		}
		if request < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"status":"approved"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(2),
		WithBaseDelay(time.Nanosecond),
	)
	if _, err := client.Clearances().Screen(context.Background(), 123, ClearanceApproved,
		ScreenOptions{DesignationBoxID: 215744}); err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
}

func TestGeneratedUpdateClearanceWithBodyRetryReplaysCompleteBody(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requests.Add(1)
		var body generated.UpdateClearanceJSONRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("request %d body: %v", request, err)
		}
		if body.Status != ClearanceDenied {
			t.Errorf("request %d status = %q", request, body.Status)
		}
		if request < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"status":"denied"}`))
	}))
	t.Cleanup(server.Close)

	client, err := generated.NewClient(
		server.URL,
		generated.WithMaxRetries(2),
		generated.WithBaseDelay(time.Nanosecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.UpdateClearanceWithBody(
		context.Background(),
		123,
		"application/json",
		strings.NewReader(`{"status":"denied"}`),
	)
	if err != nil {
		t.Fatalf("UpdateClearanceWithBody: %v", err)
	}
	defer response.Body.Close()
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
}

func TestClearancesService_ScreenUsesOperationRetryPolicy(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		wantRequests int64
	}{
		{name: "declared status", status: http.StatusServiceUnavailable, wantRequests: 2},
		{name: "undeclared status", status: http.StatusInternalServerError, wantRequests: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)

			client := NewClient(
				&Config{BaseURL: server.URL},
				&StaticTokenProvider{Token: "test-token"},
				WithBaseDelay(time.Nanosecond),
			)
			if _, err := client.Clearances().Screen(context.Background(), 123, ClearanceDenied, ScreenOptions{}); err == nil {
				t.Fatal("Screen returned nil, want API error")
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tt.wantRequests)
			}
		})
	}
}

// The Screener only takes a decision. Anything else is a caller bug, and HEY answers 403,
// so it is worth catching before the round trip.
func TestClearancesService_ScreenRejectsOtherStatuses(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://example.invalid"}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	for _, status := range []string{"pending", "unexamined", "punting", ""} {
		_, err := client.Clearances().Screen(context.Background(), 91, status, ScreenOptions{})

		var heyErr *Error
		if !errors.As(err, &heyErr) || heyErr.Code != CodeValidation {
			t.Errorf("expected a validation error for %q, got %v", status, err)
		}
	}
}

func TestClearancesService_ScreenMany(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/clearances/bulk.json" {
			t.Errorf("expected PATCH /clearances/bulk.json, got %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clearances":[{"id":91,"status":"denied"},{"id":92,"status":"denied"}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	clearances, err := client.Clearances().ScreenMany(context.Background(), []int64{91, 92}, ClearanceDenied, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["ids"] != "91,92" {
		t.Errorf("expected a comma separated list of ids, got %+v", gotBody["ids"])
	}
	if gotBody["spam"] != true {
		t.Errorf("expected spam, got %+v", gotBody["spam"])
	}
	if len(clearances) != 2 || clearances[1].Id != 92 {
		t.Errorf("expected both clearances back, got %+v", clearances)
	}
}

func TestClearancesService_ScreenManyRequiresClearances(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://example.invalid"}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	_, err := client.Clearances().ScreenMany(context.Background(), nil, ClearanceDenied, false)

	var heyErr *Error
	if !errors.As(err, &heyErr) || heyErr.Code != CodeUsage {
		t.Fatalf("expected a usage error, got %v", err)
	}
}

// HEY answers 404 when none of the ids belong to the caller, rather than reporting an
// empty success.
func TestClearancesService_ScreenManySurfacesUnknownIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	_, err := client.Clearances().ScreenMany(context.Background(), []int64{404}, ClearanceDenied, false)

	var heyErr *Error
	if !errors.As(err, &heyErr) || heyErr.Code != CodeNotFound {
		t.Fatalf("expected a not found error, got %v", err)
	}
}

func TestClearancesService_Punt(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost || r.URL.Path != "/clearances/punt.json" {
			t.Errorf("expected POST /clearances/punt.json, got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	if err := client.Clearances().Punt(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected the punt to be sent")
	}
}

func TestClearancesService_Screened(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/my/clearances.json" {
			t.Errorf("expected GET /my/clearances.json, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clearances":[
			{"id":91,"status":"approved","petitioner":{"id":51,"name":"Glenn"}},
			{"id":92,"status":"denied","petitioner":{"id":52,"name":"Spammer"}}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	clearances, err := client.Clearances().Screened(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clearances) != 2 {
		t.Fatalf("expected two clearances, got %+v", clearances)
	}
	if clearances[0].Status != "approved" || clearances[1].Status != "denied" {
		t.Errorf("expected both decisions, got %+v", clearances)
	}
	if clearances[0].Petitioner.Name != "Glenn" {
		t.Errorf("expected the petitioner, got %+v", clearances[0].Petitioner)
	}
}

func TestClearancesService_Rescreen(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/my/clearances/91.json" {
			t.Errorf("expected PATCH /my/clearances/91.json, got %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":91,"status":"denied"}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	clearance, err := client.Clearances().Rescreen(context.Background(), 91, ClearanceDenied)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// HEY takes the status flat here, not nested under a clearance key like its own form.
	if gotBody["status"] != "denied" {
		t.Errorf("expected the status at the top level, got %+v", gotBody)
	}
	if clearance.Status != "denied" {
		t.Errorf("expected the updated clearance, got %+v", clearance)
	}
}
