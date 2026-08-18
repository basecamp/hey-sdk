package hey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

const boxesJSON = `[{"id":24088,"kind":"imbox","name":"Imbox"},{"id":24089,"kind":"feedbox","name":"The Feed"},{"id":24090,"kind":"asidebox","name":"Set Aside"},{"id":24091,"kind":"laterbox","name":"Reply Later"},{"id":24092,"kind":"trailbox","name":"Paper Trail"}]`

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   map[string]any
}

// newPostingsTestClient serves /boxes.json and records every other request.
func newPostingsTestClient(t *testing.T, status int) (*Client, *[]recordedRequest, *int32) {
	t.Helper()
	var reqs []recordedRequest
	var boxCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boxes.json" {
			atomic.AddInt32(&boxCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(boxesJSON))
			return
		}
		rec := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &rec.Body)
		}
		reqs = append(reqs, rec)
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	cfg := &Config{BaseURL: server.URL}
	c := NewClient(cfg, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithBaseDelay(time.Millisecond), WithMaxJitter(time.Millisecond))
	return c, &reqs, &boxCalls
}

func idsOf(body map[string]any) []float64 {
	raw, _ := body["posting_ids"].([]any)
	out := make([]float64, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(float64))
	}
	return out
}

func TestPostingsService_BulkEndpoints(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		call     func(c *Client) error
		method   string
		path     string
		wantIDs  []float64
		wantBox  float64
		wantQry  string
		boxCalls int32
	}{
		{"MarkSeen", func(c *Client) error { return c.Postings().MarkSeen(ctx, []int64{1, 2}) }, "POST", "/postings/seen.json", []float64{1, 2}, 0, "", 0},
		{"MarkUnseen", func(c *Client) error { return c.Postings().MarkUnseen(ctx, []int64{3}) }, "POST", "/postings/unseen.json", []float64{3}, 0, "", 0},
		{"Move", func(c *Client) error { return c.Postings().Move(ctx, 999, 1, 2) }, "POST", "/postings/moves.json", []float64{1, 2}, 999, "", 0},
		{"MoveToFeed", func(c *Client) error { return c.Postings().MoveToFeed(ctx, 5) }, "POST", "/postings/moves.json", []float64{5}, 24089, "", 1},
		{"MoveToSetAside", func(c *Client) error { return c.Postings().MoveToSetAside(ctx, 5) }, "POST", "/postings/moves.json", []float64{5}, 24090, "", 1},
		{"MoveToReplyLater", func(c *Client) error { return c.Postings().MoveToReplyLater(ctx, 5) }, "POST", "/postings/moves.json", []float64{5}, 24091, "", 1},
		{"MoveToPaperTrail", func(c *Client) error { return c.Postings().MoveToPaperTrail(ctx, 5) }, "POST", "/postings/moves.json", []float64{5}, 24092, "", 1},
		{"MoveToImbox", func(c *Client) error { return c.Postings().MoveToImbox(ctx, 5, 6) }, "POST", "/postings/moves.json", []float64{5, 6}, 24088, "", 1},
		{"MoveToTrash", func(c *Client) error { return c.Postings().MoveToTrash(ctx, 7) }, "POST", "/postings/trash.json", []float64{7}, 0, "", 0},
		{"Mute", func(c *Client) error { return c.Postings().Mute(ctx, 8, 9) }, "POST", "/postings/mutings.json", []float64{8, 9}, 0, "", 0},
		{"Unmute", func(c *Client) error { return c.Postings().Unmute(ctx, 8, 9) }, "DELETE", "/postings/mutings.json", nil, 0, "posting_ids=8%2C9", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, reqs, boxCalls := newPostingsTestClient(t, 204)
			if err := tc.call(c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(*reqs) != 1 {
				t.Fatalf("expected 1 request, got %d", len(*reqs))
			}
			r := (*reqs)[0]
			if r.Method != tc.method || r.Path != tc.path {
				t.Errorf("got %s %s, want %s %s", r.Method, r.Path, tc.method, tc.path)
			}
			if tc.wantQry != "" && r.Query != tc.wantQry {
				t.Errorf("query = %q, want %q", r.Query, tc.wantQry)
			}
			if tc.wantIDs != nil {
				got := idsOf(r.Body)
				if len(got) != len(tc.wantIDs) {
					t.Fatalf("posting_ids = %v, want %v", got, tc.wantIDs)
				}
				for i := range got {
					if got[i] != tc.wantIDs[i] {
						t.Errorf("posting_ids = %v, want %v", got, tc.wantIDs)
					}
				}
			}
			if tc.wantBox != 0 && r.Body["box_id"] != tc.wantBox {
				t.Errorf("box_id = %v, want %v", r.Body["box_id"], tc.wantBox)
			}
			if atomic.LoadInt32(boxCalls) != tc.boxCalls {
				t.Errorf("ListBoxes calls = %d, want %d", *boxCalls, tc.boxCalls)
			}
		})
	}
}

func TestPostingsService_BoxKindResolvedOnce(t *testing.T) {
	ctx := context.Background()
	c, reqs, boxCalls := newPostingsTestClient(t, 204)
	for i := 0; i < 3; i++ {
		if err := c.Postings().MoveToFeed(ctx, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Postings().MoveToPaperTrail(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(boxCalls); got != 1 {
		t.Errorf("ListBoxes called %d times, want 1 (cached)", got)
	}
	if len(*reqs) != 4 {
		t.Errorf("expected 4 move requests, got %d", len(*reqs))
	}
}

func TestPostingsService_UnknownBoxKind(t *testing.T) {
	c, reqs, _ := newPostingsTestClient(t, 204)
	err := c.Postings().MoveToBox(context.Background(), "nope", 1)
	if err == nil {
		t.Fatal("expected error for unknown box kind")
	}
	if len(*reqs) != 0 {
		t.Errorf("no move request should be sent, got %d", len(*reqs))
	}
}

func TestPostingsService_RequiresIDs(t *testing.T) {
	c, reqs, _ := newPostingsTestClient(t, 204)
	if err := c.Postings().MoveToTrash(context.Background()); err == nil {
		t.Fatal("expected error for empty IDs")
	}
	if err := c.Postings().MarkSeen(context.Background(), nil); err == nil {
		t.Fatal("expected error for empty IDs")
	}
	if len(*reqs) != 0 {
		t.Errorf("no request should be sent, got %d", len(*reqs))
	}
}

func TestPostingsService_TrashRemoveAccess(t *testing.T) {
	ctx := context.Background()

	mine, reqs, _ := newPostingsTestClient(t, 204)
	if err := mine.Postings().MoveToTrash(ctx, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := (*reqs)[0].Body["remove_access"]; ok {
		t.Error("remove_access should be left out so the server keeps its default")
	}

	everyone, sharedReqs, _ := newPostingsTestClient(t, 204)
	if err := everyone.Postings().TrashForEveryone(ctx, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := (*sharedReqs)[0].Body["remove_access"]; got != "false" {
		t.Errorf(`remove_access = %v, want "false"`, got)
	}
}

func TestPostingsService_ServerErrorSurfaced(t *testing.T) {
	c, _, _ := newPostingsTestClient(t, 500)
	if err := c.Postings().Mute(context.Background(), 1); err == nil {
		t.Fatal("expected error on 500")
	}
}

// opRecorder captures the operation names announced to hooks.
type opRecorder struct {
	NoopHooks
	ops []string
}

func (r *opRecorder) OnOperationStart(ctx context.Context, op OperationInfo) context.Context {
	r.ops = append(r.ops, op.Operation)
	return ctx
}

func TestTimeTracksService_StopAnnouncesItself(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"type":"TimeTrack"}`))
	}))
	t.Cleanup(srv.Close)
	rec := &opRecorder{}
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0), WithHooks(rec))
	if err := c.TimeTracks().Stop(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(rec.ops) != 1 || rec.ops[0] != "StopTimeTrack" {
		t.Errorf("hooks saw %v, want exactly [StopTimeTrack] (not UpdateTimeTrack, and not both)", rec.ops)
	}
}

func TestCheckResponse_ForbiddenMutationIsScopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	err := c.Postings().Mute(context.Background(), 1) // POST through the generated client
	if e := AsError(err); e == nil || e.Code != CodeForbidden || e.Hint == "" {
		t.Errorf("mutation 403 should carry the scope hint, got %#v", err)
	}
	_, err = c.Boxes().List(context.Background()) // GET
	if e := AsError(err); e == nil || e.Code != CodeForbidden || e.Hint != "" {
		t.Errorf("read 403 should be a plain forbidden, got %#v", err)
	}
}

func TestTimeTracksService_StartConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Ongoing time track already in progress"}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))
	_, err := c.TimeTracks().Start(context.Background())
	e := AsError(err)
	if e == nil || e.Code != CodeConflict || e.HTTPStatus != 409 || e.Message != "Ongoing time track already in progress" {
		t.Fatalf("expected a conflict error carrying the server message, got %#v", err)
	}
}

// --- Behaviour fixes from the #64 review ---

func TestContactsService_UpdateMergesUnsetFields(t *testing.T) {
	var sent generated.ContactRequestContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/contacts/7.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":7,"name":"Jane Dawson","email_address":"jane@x.com","aliases":[{"id":8,"email_address":"jd@x.com"},{"id":9,"email_address":"jane.d@x.com"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/contacts/7.json":
			sent = generated.ContactRequestContent{}
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Errorf("decoding the request body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":7,"name":"Jane Dawson","email_address":"new@x.com"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	// Only the email changes: name and both aliases must be carried over.
	contact, err := c.Contacts().Update(context.Background(), 7, ContactParams{EmailAddress: "new@x.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contact == nil || contact.Id != 7 {
		t.Errorf("expected the updated contact, got %+v", contact)
	}
	if sent.Contact.Name != "Jane Dawson" {
		t.Errorf("name should be preserved, got %q", sent.Contact.Name)
	}
	if sent.Contact.EmailAddress != "new@x.com" {
		t.Errorf("email should be updated, got %q", sent.Contact.EmailAddress)
	}
	if got := sent.Contact.AliasEmailAddresses; len(got) != 2 || got[0] != "jd@x.com" {
		t.Errorf("aliases should be preserved, got %v", got)
	}

	// An explicit empty alias list clears them.
	if _, err := c.Contacts().Update(context.Background(), 7, ContactParams{AliasEmailAddresses: []string{}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := sent.Contact.AliasEmailAddresses; len(got) != 0 {
		t.Errorf("explicit empty aliases should clear them, got %v", got)
	}
	if sent.Contact.Name != "Jane Dawson" {
		t.Errorf("name should still be preserved, got %q", sent.Contact.Name)
	}
}

func TestPublicationsService_CreateIsOneOperation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/topics/5/publication":
			w.Header().Set("Location", "http://"+r.Host+"/topics/5")
			w.WriteHeader(http.StatusFound)
		case r.Method == http.MethodGet && r.URL.Path == "/topics/5/publication.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"published":true,"url":"https://public.hey.com/p/abc123"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	rec := &opRecorder{}
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0), WithHooks(rec))

	pub, err := c.Publications().Create(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pub.Published || pub.Url != "https://public.hey.com/p/abc123" {
		t.Errorf("expected the public link back, got %#v", pub)
	}
	if len(rec.ops) != 1 || rec.ops[0] != "CreateTopicPublication" {
		t.Errorf("hooks saw %v, want exactly [CreateTopicPublication] — Get must not be a nested operation", rec.ops)
	}
}

func TestTopicsService_TrashSharedTopicNeedsConfirmation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/topics/9/status/trashed.json":
			// A shared topic without confirm_destroy: HEY redirects to the confirmation page.
			w.Header().Set("Location", "http://"+r.Host+"/topics/9/removal/new")
			w.WriteHeader(http.StatusFound)
		case r.URL.Path == "/topics/9/removal/new":
			w.WriteHeader(http.StatusNotAcceptable) // HTML page asked for as JSON
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	err := c.Topics().Trash(context.Background(), 9, false)
	e := AsError(err)
	if e == nil || e.Code != CodeUsage || e.Hint == "" {
		t.Fatalf("expected a usage error telling the caller to confirm, got %#v", err)
	}
}
