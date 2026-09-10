package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// policyTestServer answers each request with the next status in turn, a 200 carrying the
// given body, and counts what it was sent.
func policyTestServer(t *testing.T, body string, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(requests.Add(1))
		status := http.StatusOK
		if n <= len(statuses) {
			status = statuses[n-1]
		}
		if status != http.StatusOK {
			http.Error(w, http.StatusText(status), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

// DeleteExtenzion is modelled with two sends in all. A client allowing five resends gets
// the policy's two sends, not six: the client's count is a ceiling, never a grant.
func TestOperationPolicyBoundsTheClientRetries(t *testing.T) {
	server, requests := policyTestServer(t, `{}`, 503, 503, 503, 503, 503)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(5), WithBaseDelay(time.Millisecond))

	if err := root.Extenzions().Delete(context.Background(), 1, 2); err == nil {
		t.Fatal("expected the 503 to surface once the policy's sends were spent")
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("requests = %d, want the policy's 2", got)
	}
}

// A 502 is on the SDK's own list of transient failures, and the client's count would
// allow a resend, but ListBoxes is modelled to resend on 429 and 503 only: the 502 is
// the answer.
func TestOperationPolicyDecidesWhichStatusesAreResent(t *testing.T) {
	server, requests := policyTestServer(t, `[]`, 502, 200)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(3), WithBaseDelay(time.Millisecond))
	client := scopedTestClient(root, 42)

	if _, err := client.Boxes().List(context.Background()); err == nil {
		t.Fatal("expected the 502 to surface")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

// An operation the contract gives no policy is sent once, whatever the client allows.
func TestOperationWithoutPolicyIsSentOnce(t *testing.T) {
	server, requests := policyTestServer(t, `[]`, 503, 503, 503)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(3), WithBaseDelay(time.Millisecond))

	if _, err := root.Workflows().GetStage(context.Background(), 1, 2); err == nil {
		t.Fatal("expected the 503 to surface")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

// The first wait is the operation's own base delay when the client's is shorter, and the
// client's when that is longer; the client's MaxDelay bounds both.
func TestOperationPolicyDelayIsFlooredByTheClient(t *testing.T) {
	for _, tc := range []struct {
		name      string
		baseDelay time.Duration
		want      time.Duration
	}{
		{name: "a shorter client delay leaves the policy's second", baseDelay: time.Millisecond, want: time.Second},
		{name: "a longer client delay is the floor", baseDelay: 2 * time.Second, want: 2 * time.Second},
		{name: "a client delay above the default ceiling lifts the ceiling with it", baseDelay: 45 * time.Second, want: 45 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewClient(&Config{BaseURL: "https://example.test"}, &StaticTokenProvider{Token: "token"},
				WithBaseDelay(tc.baseDelay))
			root.initGeneratedClient()

			policy := root.gen.ClientInterface.(*generated.Client).RetryPolicy("ListBoxes")
			if policy.BaseDelay != tc.want {
				t.Errorf("BaseDelay = %v, want %v", policy.BaseDelay, tc.want)
			}
			if policy.MaxAttempts != 3 {
				t.Errorf("MaxAttempts = %d, want the policy's 3", policy.MaxAttempts)
			}
		})
	}
}

// On the wire, ListBoxes waits its modelled second before the resend even when the client
// asked for a millisecond.
func TestOperationPolicyDelayHoldsOnTheWire(t *testing.T) {
	server, requests := policyTestServer(t, `[]`, 503, 200)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(3), WithBaseDelay(time.Millisecond))
	client := scopedTestClient(root, 42)

	start := time.Now()
	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatalf("expected the resend to succeed: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Errorf("resend came after %v, want the policy's second", elapsed)
	}
}

// A walk on from a generated operation's first page reads every later page under the same
// policy: a 502 on page two is the answer, and page two's 503s are resent up to the
// policy's three sends and no further, whatever the client's count allows.
func TestFollowPaginationReadsLaterPagesUnderTheOperationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		pageTwo      []int
		pageRequests int32
	}{
		{name: "a status outside the policy is the answer", pageTwo: []int{502, 200}, pageRequests: 1},
		{name: "the policy's sends bound the resends", pageTwo: []int{503, 503, 503, 200}, pageRequests: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pageTwoRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") != "2" {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Link", `</boxes.json?page=2>; rel="next"`)
					_, _ = fmt.Fprint(w, `[{"id": 1}]`)
					return
				}
				n := int(pageTwoRequests.Add(1))
				status := http.StatusOK
				if n <= len(tc.pageTwo) {
					status = tc.pageTwo[n-1]
				}
				if status != http.StatusOK {
					http.Error(w, http.StatusText(status), status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `[{"id": 2}]`)
			}))
			t.Cleanup(server.Close)

			root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
				WithMaxRetries(5), WithBaseDelay(time.Millisecond))
			first, err := root.genClient().ListBoxesWithResponse(context.Background())
			if err != nil {
				t.Fatalf("first page: %v", err)
			}

			if _, err := root.FollowPagination(context.Background(), first.HTTPResponse, 1, 0); err == nil {
				t.Fatal("expected page two's failure to surface")
			}
			if got := pageTwoRequests.Load(); got != tc.pageRequests {
				t.Errorf("page two requests = %d, want %d", got, tc.pageRequests)
			}
		})
	}
}

// A walk on from a response the generated client did not answer has no operation to bring
// a policy, and runs on the client's own settings as before.
func TestFollowPaginationWithoutAnOperationRunsOnTheClientSettings(t *testing.T) {
	var pageTwoRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			w.Header().Set("Link", `</items.json?page=2>; rel="next"`)
			_, _ = fmt.Fprint(w, `[{"id": 1}]`)
			return
		}
		if pageTwoRequests.Add(1) <= 2 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		_, _ = fmt.Fprint(w, `[{"id": 2}]`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"},
		WithMaxRetries(3), WithBaseDelay(time.Millisecond))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/items.json", nil)
	if err != nil {
		t.Fatalf("first page request: %v", err)
	}
	first, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	defer func() { _ = first.Body.Close() }()

	items, err := root.FollowPagination(context.Background(), first, 1, 0)
	if err != nil {
		t.Fatalf("expected the 502s to be resent on the client's settings: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("items = %d, want 1", len(items))
	}
	if got := pageTwoRequests.Load(); got != 3 {
		t.Errorf("page two requests = %d, want 3", got)
	}
}
