package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClearancesServiceList(t *testing.T) {
	var requests atomic.Int64
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/clearances" {
			t.Errorf("path = %s, want /clearances", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/html" {
			t.Errorf("Accept = %q, want text/html", got)
		}
		w.Header().Set("Content-Type", "text/html")
		if page := r.URL.Query().Get("page"); page != "" {
			if page != "cursor-2" {
				t.Errorf("page = %q, want cursor-2", page)
			}
			_, _ = w.Write([]byte(`<!doctype html>
<html><body>
  <article class="clearance" id="clearance_321">
    <span id="email_clearance_321">second@example.com</span>
  </article>
  <article class="clearance" id="clearance_654">
    <span id="name_clearance_654">Third Sender</span>
    <span id="email_clearance_654">third@example.org</span>
  </article>
</body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html>
<html><body>
  <span id="email_clearance_999">navigation@example.com</span>
  <article class="card clearance pending" id="clearance_123">
    <span id="name_clearance_123"> Acme &amp; Co. </span>
    <span id="email_clearance_123">sender@example.com</span>
    <span class="clearance__subject">  A useful   subject </span>
    <turbo-frame src="/clearances/entries/456"></turbo-frame>
    <input type="hidden" name="reply_to_topic_id" value="789">
    <form action="/clearances/123" method="post">
      <input type="hidden" name="designation_box_id" value="215744">
      <button data-clearances-target="feedboxButton">Feed</button>
    </form>
    <form action="/clearances/123" method="post">
      <input type="hidden" name="designation_box_id" value="215747">
      <button data-clearances-target="trailboxButton">Paper Trail</button>
    </form>
  </article>
  <article class="clearance" id="clearance_321">
    <span id="name_clearance_321">Second Sender</span>
    <span id="email_clearance_321">second@example.com</span>
    <span class="clearance__subject">Second subject</span>
  </article>
  <article class="clearance" id="not-a-clearance"></article>
  <a class="pagination-link" href="/clearances?page=cursor-2">Next</a>
</body></html>`))
	})

	got, err := client.Clearances().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(List) = %d, want 3", len(got))
	}

	wantFirst := PendingClearance{
		ID:           123,
		EntryID:      456,
		TopicID:      789,
		Name:         "Acme & Co.",
		EmailAddress: "sender@example.com",
		Subject:      "A useful subject",
		FeedBoxID:    215744,
		TrailBoxID:   215747,
	}
	if got[0] != wantFirst {
		t.Errorf("first clearance = %#v, want %#v", got[0], wantFirst)
	}
	if got[1].ID != 321 || got[1].EmailAddress != "second@example.com" {
		t.Errorf("second clearance = %#v", got[1])
	}
	if got[1].FeedBoxID != 0 || got[1].TrailBoxID != 0 {
		t.Errorf("second clearance destinations = (%d, %d), want zero values", got[1].FeedBoxID, got[1].TrailBoxID)
	}
	if got[2].ID != 654 || got[2].EmailAddress != "third@example.org" {
		t.Errorf("third clearance = %#v", got[2])
	}
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

func TestClearancesServiceListWithLimit(t *testing.T) {
	var requests atomic.Int64
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("page") != "" {
			t.Error("limit should stop before requesting another page")
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
  <article class="clearance" id="clearance_123"></article>
  <article class="clearance" id="clearance_321"></article>
  <a class="pagination-link" href="/clearances?page=cursor-2">Next</a>
</body></html>`))
	})

	got, err := client.Clearances().ListWithLimit(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListWithLimit: %v", err)
	}
	if len(got) != 1 || got[0].ID != 123 {
		t.Fatalf("ListWithLimit = %#v, want only clearance 123", got)
	}
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1", requests.Load())
	}

	if _, err := client.Clearances().ListWithLimit(context.Background(), -1); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("negative limit error = %v, want validation error", err)
	}
	if requests.Load() != 1 {
		t.Errorf("requests after invalid limit = %d, want 1", requests.Load())
	}
}

func TestClearancesServiceListReturnsEmptySlice(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body></body></html>`))
	})

	got, err := client.Clearances().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Fatal("List returned nil, want an empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(List) = %d, want 0", len(got))
	}
}

func TestClearancesServiceRejectsCrossOriginPagination(t *testing.T) {
	var requests atomic.Int64
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
  <article class="clearance" id="clearance_123"></article>
  <a class="pagination-link" href="https://files.example.org/clearances?page=cursor-2">Next</a>
</body></html>`))
	})

	_, err := client.Clearances().List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("List error = %v, want cross-origin pagination error", err)
	}
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1", requests.Load())
	}
}

func TestClearancesServiceRejectsCrossOriginInitialRedirect(t *testing.T) {
	var originRequests atomic.Int64
	var foreignRequests atomic.Int64
	var foreignAuthorization atomic.Value

	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignRequests.Add(1)
		foreignAuthorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
  <article class="clearance" id="clearance_123"></article>
  <a class="pagination-link" href="/clearances?page=cursor-2">Next</a>
</body></html>`))
	}))
	t.Cleanup(foreign.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		originRequests.Add(1)
		w.Header().Set("Location", foreign.URL+"/clearances")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	client := NewClient(
		&Config{BaseURL: origin.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
	)
	_, err := client.Clearances().List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "redirected to different origin") {
		t.Fatalf("List error = %v, want initial redirect rejection", err)
	}
	if originRequests.Load() != 1 || foreignRequests.Load() != 1 {
		t.Fatalf("requests = origin %d, foreign %d; want 1 and 1", originRequests.Load(), foreignRequests.Load())
	}
	if got, _ := foreignAuthorization.Load().(string); got != "" {
		t.Fatalf("foreign redirect received Authorization header %q", got)
	}
}

func TestClearancesServiceRejectsPaginationLoop(t *testing.T) {
	var requests atomic.Int64
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
  <article class="clearance" id="clearance_123"></article>
  <a class="pagination-link" href="/clearances">Next</a>
</body></html>`))
	})

	_, err := client.Clearances().List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "loop detected") {
		t.Fatalf("List error = %v, want pagination loop error", err)
	}
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1", requests.Load())
	}
}

func TestClearancesServiceHonorsMaxPages(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html><body>
  <article class="clearance" id="clearance_%d"></article>
  <a class="pagination-link" href="/clearances?page=cursor-%d">Next</a>
</body></html>`, page, page+1)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxPages(2),
		WithMaxRetries(0),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Millisecond),
	)

	_, err := client.Clearances().List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeded max pages (2)") {
		t.Fatalf("List error = %v, want max-pages error", err)
	}
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

func TestClearancesServiceUpdates(t *testing.T) {
	tests := []struct {
		name            string
		call            func(*ClearancesService) error
		wantStatus      string
		wantDesignation string
	}{
		{
			name:       "approve to Imbox",
			call:       func(service *ClearancesService) error { return service.Approve(context.Background(), 123, 0) },
			wantStatus: "approved",
		},
		{
			name:            "approve to a designation box",
			call:            func(service *ClearancesService) error { return service.Approve(context.Background(), 123, 215744) },
			wantStatus:      "approved",
			wantDesignation: "215744",
		},
		{
			name:       "deny",
			call:       func(service *ClearancesService) error { return service.Deny(context.Background(), 123) },
			wantStatus: "denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					t.Errorf("method = %s, want PATCH", r.Method)
				}
				if r.URL.Path != "/clearances/123" {
					t.Errorf("path = %s, want /clearances/123", r.URL.Path)
				}
				if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
					t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
				}
				if got := r.Header.Get("Accept"); got != "*/*" {
					t.Errorf("Accept = %q, want */*", got)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if got := r.PostForm.Get("status"); got != tt.wantStatus {
					t.Errorf("status = %q, want %q", got, tt.wantStatus)
				}
				if got := r.PostForm.Get("designation_box_id"); got != tt.wantDesignation {
					t.Errorf("designation_box_id = %q, want %q", got, tt.wantDesignation)
				}
				w.WriteHeader(http.StatusNoContent)
			})

			if err := tt.call(client.Clearances()); err != nil {
				t.Fatalf("update: %v", err)
			}
		})
	}
}

func TestClearancesServiceUpdateAcceptsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/clearances")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	httpClient := server.Client()
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithHTTPClient(httpClient),
		WithMaxRetries(0),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Millisecond),
	)

	if err := client.Clearances().Deny(context.Background(), 123); err != nil {
		t.Fatalf("Deny: %v", err)
	}
}

func TestClearancesServiceUpdateRetriesWithCompleteBody(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requests.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.PostForm.Get("status"); got != "approved" {
			t.Errorf("request %d status = %q, want approved", request, got)
		}
		if got := r.PostForm.Get("designation_box_id"); got != "215744" {
			t.Errorf("request %d designation_box_id = %q, want 215744", request, got)
		}
		if request == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(1),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Millisecond),
	)
	if err := client.Clearances().Approve(context.Background(), 123, 215744); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestClearancesServiceRejectsInvalidIDsBeforeRequest(t *testing.T) {
	var requests atomic.Int64
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.Clearances().Deny(context.Background(), 0); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("Deny(0) error = %v, want positive ID error", err)
	}
	if err := client.Clearances().Approve(context.Background(), 123, -1); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("Approve(-1) error = %v, want negative box ID error", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestClientClearancesReturnsCachedService(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	first := client.Clearances()
	second := client.Clearances()
	if first == nil || first != second {
		t.Fatalf("Clearances service pointers = (%p, %p), want same non-nil service", first, second)
	}
}
