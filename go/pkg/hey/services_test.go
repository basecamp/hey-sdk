package hey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// newServiceTestClient creates a Client pointing at a test server that
// routes based on URL path and returns appropriate JSON responses.
func newServiceTestClient(t *testing.T, routes map[string]string, methods ...string) *Client { //nolint:unparam // methods intentionally variadic for non-GET service tests
	t.Helper()
	wantMethod := "GET"
	if len(methods) > 0 {
		wantMethod = methods[0]
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != wantMethod {
			t.Errorf("expected %s, got %s", wantMethod, r.Method)
		}
		path := r.URL.Path
		for pattern, body := range routes {
			if pathMatch(pattern, path) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found: ` + path + `"}`))
	}))
	t.Cleanup(server.Close)

	cfg := &Config{BaseURL: server.URL}
	return NewClient(cfg, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
		WithBaseDelay(1*time.Millisecond),
		WithMaxJitter(1*time.Millisecond),
	)
}

func pathMatch(pattern, path string) bool {
	// Simple matching: pattern segments containing %s match any single segment
	pp := strings.Split(pattern, "/")
	sp := strings.Split(path, "/")
	if len(pp) != len(sp) {
		return false
	}
	for i, seg := range pp {
		if strings.Contains(seg, "%s") {
			continue
		}
		if seg != sp[i] {
			return false
		}
	}
	return true
}

// identityJSON is used by mutation tests that need DefaultSenderID to resolve.
const identityJSON = `{"email_address":"user@hey.com","id":1,"senders":[{"id":42,"default":true}],"primary_contact":{"id":42}}`

// newMutationTestClientWithValidation creates a test client that validates request bodies.
func newMutationTestClientWithValidation(t *testing.T, wantMethod, wantPath string, validateBody func(t *testing.T, body map[string]any), responseJSON string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/identity.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(identityJSON))
			return
		}
		if r.Method != wantMethod {
			t.Errorf("expected %s, got %s", wantMethod, r.Method)
		}
		if !pathMatch(wantPath, path) {
			t.Errorf("expected path matching %s, got %s", wantPath, path)
		}
		if validateBody != nil {
			data, _ := io.ReadAll(r.Body)
			var body map[string]any
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			validateBody(t, body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(responseJSON))
	}))
	t.Cleanup(server.Close)

	cfg := &Config{BaseURL: server.URL}
	return NewClient(cfg, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
		WithBaseDelay(1*time.Millisecond),
		WithMaxJitter(1*time.Millisecond),
	)
}

// newFormTestClient creates a Client pointing at a test server that validates
// form-encoded requests and returns redirect responses.
func newFormTestClient(t *testing.T, wantMethod, wantPath string, validateForm func(t *testing.T, values url.Values), redirectLocation string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/identity.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(identityJSON))
			return
		}
		if r.Method != wantMethod {
			t.Errorf("expected %s, got %s", wantMethod, r.Method)
		}
		if !pathMatch(wantPath, path) {
			t.Errorf("expected path matching %s, got %s", wantPath, path)
		}
		if validateForm != nil {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}
			validateForm(t, r.PostForm)
		}
		if redirectLocation != "" {
			w.Header().Set("Location", redirectLocation)
			w.WriteHeader(302)
		} else {
			w.WriteHeader(200)
		}
	}))
	t.Cleanup(server.Close)

	cfg := &Config{BaseURL: server.URL}
	return NewClient(cfg, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
		WithBaseDelay(1*time.Millisecond),
		WithMaxJitter(1*time.Millisecond),
	)
}

// --- Identity ---

func TestIdentityService_GetIdentity(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/identity.json": `{"email_address":"user@example.com","id":1}`,
	})

	result, err := client.Identity().GetIdentity(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestIdentityService_GetNavigation(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/my/navigation.json": `{"accounts":[],"identity":{}}`,
	})

	result, err := client.Identity().GetNavigation(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestIdentityService_GetIdentity_Error(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{})
	_, err := client.Identity().GetIdentity(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Boxes ---

func TestBoxesService_List(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/boxes.json": `[{"id":1,"kind":"imbox","name":"Imbox"}]`,
	})

	result, err := client.Boxes().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/boxes/%s": `{"id":1,"kind":"imbox","name":"Imbox","postings":[]}`,
	})

	result, err := client.Boxes().Get(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_GetImbox(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/imbox.json": `{"id":1,"kind":"imbox","name":"Imbox","postings":[]}`,
	})

	result, err := client.Boxes().GetImbox(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_GetFeedbox(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/feedbox.json": `{"id":2,"kind":"feedbox","name":"The Feed","postings":[]}`,
	})

	result, err := client.Boxes().GetFeedbox(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_GetTrailbox(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/paper_trail.json": `{"id":3,"kind":"trailbox","name":"Paper Trail","postings":[]}`,
	})

	result, err := client.Boxes().GetTrailbox(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_GetAsidebox(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/set_aside.json": `{"id":4,"kind":"asidebox","name":"Set Aside","postings":[]}`,
	})

	result, err := client.Boxes().GetAsidebox(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_GetLaterbox(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/reply_later.json": `{"id":5,"kind":"laterbox","name":"Reply Later","postings":[]}`,
	})

	result, err := client.Boxes().GetLaterbox(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_GetBubblebox(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/bubble_up.json": `{"id":6,"kind":"bubblebox","name":"Bubbled Up","postings":[]}`,
	})

	result, err := client.Boxes().GetBubblebox(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBoxesService_List_Error(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{})
	// All paths will 404 since we provide no routes, verifying error propagation
	_, err := client.Boxes().List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Topics ---

func TestTopicsService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/%s": `{"id":42,"subject":"Hello"}`,
	})

	result, err := client.Topics().Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTopicsService_GetEntries(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/%s/entries": `[{"id":1}]`,
	})

	result, err := client.Topics().GetEntries(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTopicsService_GetSent(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/sent.json": `{"title":"Sent","topics":[{"id":1}]}`,
	})

	result, err := client.Topics().GetSent(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTopicsService_GetSpam(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/spam.json": `{"title":"Spam","topics":[{"id":1}]}`,
	})

	result, err := client.Topics().GetSpam(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTopicsService_GetTrash(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/trash.json": `{"title":"Trash","topics":[{"id":1}]}`,
	})

	result, err := client.Topics().GetTrash(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTopicsService_GetEverything(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/everything.json": `{"title":"Everything","topics":[{"id":1}]}`,
	})

	result, err := client.Topics().GetEverything(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- Messages ---

func TestMessagesService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/messages/%s": `{"id":1,"subject":"Test"}`,
	})

	result, err := client.Messages().Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMessagesService_Create(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/messages.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if _, ok := body["acting_sender_id"]; !ok {
				t.Error("missing acting_sender_id")
			}
			msg, ok := body["message"].(map[string]any)
			if !ok {
				t.Fatal("missing message wrapper")
			}
			if msg["subject"] != "Test" {
				t.Errorf("expected subject 'Test', got %v", msg["subject"])
			}
			if msg["content"] != "Hello" {
				t.Errorf("expected content 'Hello', got %v", msg["content"])
			}
			entry, ok := body["entry"].(map[string]any)
			if !ok {
				t.Fatal("missing entry wrapper")
			}
			addressed, ok := entry["addressed"].(map[string]any)
			if !ok {
				t.Fatal("missing addressed in entry")
			}
			directly, ok := addressed["directly"].(string)
			if !ok || directly != "test@example.com" {
				t.Errorf("expected directly 'test@example.com', got %v", addressed["directly"])
			}
			copied, ok := addressed["copied"].(string)
			if !ok || copied != "cc@example.com" {
				t.Errorf("expected copied 'cc@example.com', got %v", addressed["copied"])
			}
			if _, ok := addressed["blindcopied"]; ok {
				t.Error("expected no blindcopied key for empty bcc")
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Messages().Create(context.Background(), "Test", "Hello", []string{"test@example.com"}, []string{"cc@example.com"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMessagesService_CreateTopicMessage(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/topics/%s/entries.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if _, ok := body["acting_sender_id"]; !ok {
				t.Error("missing acting_sender_id")
			}
			msg, ok := body["message"].(map[string]any)
			if !ok {
				t.Fatal("missing message wrapper")
			}
			if msg["content"] != "Reply text" {
				t.Errorf("expected content 'Reply text', got %v", msg["content"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Messages().CreateTopicMessage(context.Background(), 42, "Reply text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Entries ---

func TestEntriesService_ListDrafts(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/entries/drafts.json": `[{"id":1}]`,
	})

	result, err := client.Entries().ListDrafts(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestEntriesService_CreateReply(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if _, ok := body["acting_sender_id"]; !ok {
				t.Error("missing acting_sender_id")
			}
			msg, ok := body["message"].(map[string]any)
			if !ok {
				t.Fatal("missing message wrapper")
			}
			if msg["content"] != "My reply" {
				t.Errorf("expected content 'My reply', got %v", msg["content"])
			}
			entry, ok := body["entry"].(map[string]any)
			if !ok {
				t.Fatal("missing entry wrapper")
			}
			addressed, ok := entry["addressed"].(map[string]any)
			if !ok {
				t.Fatal("missing addressed in entry")
			}
			if addressed["directly"] != "one@example.com,two@example.org" {
				t.Errorf("expected comma-separated directly recipients, got %v", addressed["directly"])
			}
			if addressed["copied"] != "copy@example.com" {
				t.Errorf("expected copied recipient, got %v", addressed["copied"])
			}
			if _, ok := addressed["blindcopied"]; ok {
				t.Error("expected no blindcopied key for nil bcc")
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, "My reply",
		[]string{"one@example.com", "two@example.org"}, []string{"copy@example.com"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReplyDraft(t *testing.T) {
	const richHTML = `<div>New reply</div><ol><li>First item</li><li>Second item</li></ol><blockquote>Quoted thread</blockquote><div>Signature</div>`
	const location = "/entries/drafts/987?source=save#composer"
	autoQuoting := true
	metadata, ok := generated.GetOperationMetadata("CreateReplyDraft")
	if !ok || !metadata.HasSensitiveParams {
		t.Fatal("CreateReplyDraft metadata must mark its content and CSRF fields as sensitive")
	}

	for _, tc := range []struct {
		name                 string
		status               int
		token                string
		actingSenderID       int64
		wantActingSenderID   string
		wantIdentityRequests int
	}{
		{
			name:               "created response uses composer-selected sender",
			status:             http.StatusCreated,
			token:              "csrf-test-token",
			actingSenderID:     84,
			wantActingSenderID: "84",
		},
		{
			name:               "redirect response with bearer auth and composer-selected sender",
			status:             http.StatusFound,
			actingSenderID:     84,
			wantActingSenderID: "84",
		},
		{
			name:                 "see other response falls back to default sender",
			status:               http.StatusSeeOther,
			token:                "csrf-test-token",
			wantActingSenderID:   "42",
			wantIdentityRequests: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draftRequests := 0
			identityRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity.json" {
					identityRequests++
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(identityJSON))
					return
				}

				draftRequests++
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/entries/10/replies" {
					t.Errorf("expected draft reply path, got %s", r.URL.Path)
				}
				if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
					t.Errorf("expected form content type, got %q", contentType)
				}
				if accept := r.Header.Get("Accept"); accept != "*/*" {
					t.Errorf("expected Accept */*, got %q", accept)
				}
				if csrf := r.Header.Get("X-CSRF-Token"); csrf != tc.token {
					t.Errorf("expected CSRF header %q, got %q", tc.token, csrf)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}
				if got := r.PostForm.Get("acting_sender_id"); got != tc.wantActingSenderID {
					t.Errorf("expected acting sender %s, got %q", tc.wantActingSenderID, got)
				}
				if got := r.PostForm.Get("entry[status]"); got != "drafted" {
					t.Errorf("expected drafted status, got %q", got)
				}
				if got := r.PostForm.Get("message[subject]"); got != "Re: Project update" {
					t.Errorf("expected preserved subject, got %q", got)
				}
				if got := r.PostForm.Get("message[content]"); got != richHTML {
					t.Errorf("rich HTML changed:\nwant: %s\n got: %s", richHTML, got)
				}
				if got := r.PostForm.Get("message[auto_quoting]"); got != "true" {
					t.Errorf("expected auto_quoting=true, got %q", got)
				}
				if got := strings.Join(r.PostForm["entry[addressed][directly][]"], ","); got != "one@example.com,two@example.org" {
					t.Errorf("unexpected To recipients: %q", got)
				}
				if got := strings.Join(r.PostForm["entry[addressed][copied][]"], ","); got != "copy@example.com" {
					t.Errorf("unexpected CC recipients: %q", got)
				}
				if got := strings.Join(r.PostForm["entry[addressed][blindcopied][]"], ","); got != "blind@example.org" {
					t.Errorf("unexpected BCC recipients: %q", got)
				}
				if tc.token == "" {
					if _, ok := r.PostForm["authenticity_token"]; ok {
						t.Error("expected omitted authenticity_token for bearer-only request")
					}
				} else if got := r.PostForm.Get("authenticity_token"); got != tc.token {
					t.Errorf("expected authenticity_token %q, got %q", tc.token, got)
				}

				w.Header().Set("Location", location)
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)

			client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
			result, err := client.Entries().CreateReplyDraft(context.Background(), 10, CreateReplyDraftParams{
				ActingSenderID:    tc.actingSenderID,
				Content:           richHTML,
				Subject:           "Re: Project update",
				AutoQuoting:       &autoQuoting,
				To:                []string{"one@example.com", "two@example.org"},
				CC:                []string{"copy@example.com"},
				BCC:               []string{"blind@example.org"},
				AuthenticityToken: tc.token,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ID != 987 {
				t.Errorf("expected draft ID 987, got %d", result.ID)
			}
			if result.Location != location {
				t.Errorf("expected raw Location %q, got %q", location, result.Location)
			}
			if result.EditURL != "/entries/drafts/987/edit" {
				t.Errorf("expected edit URL without query/fragment, got %q", result.EditURL)
			}
			if draftRequests != 1 {
				t.Errorf("expected one draft request and no followed redirect, got %d", draftRequests)
			}
			if identityRequests != tc.wantIdentityRequests {
				t.Errorf("expected %d identity requests, got %d", tc.wantIdentityRequests, identityRequests)
			}
		})
	}
}

func TestEntriesService_UpdateDraft(t *testing.T) {
	const richHTML = `<div>Updated reply</div><ol><li>Keep the list</li></ol><blockquote>Quoted thread</blockquote><div>Signature</div>`
	metadata, ok := generated.GetOperationMetadata("UpdateDraft")
	if !ok {
		t.Fatal("UpdateDraft metadata missing")
	}
	if metadata.Idempotent {
		t.Fatal("UpdateDraft must remain single-attempt")
	}
	if !metadata.HasSensitiveParams {
		t.Fatal("UpdateDraft metadata must mark its content and CSRF fields as sensitive")
	}

	for _, tc := range []struct {
		name     string
		status   int
		location string
		wantErr  bool
	}{
		{name: "ok", status: http.StatusOK},
		{name: "found redirect", status: http.StatusFound, location: "/entries/drafts"},
		{name: "see other redirect", status: http.StatusSeeOther, location: "/entries/drafts"},
		{name: "server failure is not retried", status: http.StatusServiceUnavailable, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPatch {
					t.Errorf("expected PATCH, got %s", r.Method)
				}
				if r.URL.Path != "/messages/987" {
					t.Errorf("expected draft message path, got %s", r.URL.Path)
				}
				if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
					t.Errorf("expected form content type, got %q", contentType)
				}
				if accept := r.Header.Get("Accept"); accept != "*/*" {
					t.Errorf("expected Accept */*, got %q", accept)
				}
				if requestedWith := r.Header.Get("X-Requested-With"); requestedWith != "XMLHttpRequest" {
					t.Errorf("expected XMLHttpRequest header, got %q", requestedWith)
				}
				if csrf := r.Header.Get("X-CSRF-Token"); csrf != "csrf-update-token" {
					t.Errorf("expected CSRF header, got %q", csrf)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}
				if got := r.PostForm.Get("acting_sender_id"); got != "84" {
					t.Errorf("expected acting sender 84, got %q", got)
				}
				if got := r.PostForm.Get("entry[status]"); got != "drafted" {
					t.Errorf("expected drafted status, got %q", got)
				}
				if got := r.PostForm.Get("message[subject]"); got != "Re: Project update" {
					t.Errorf("expected preserved subject, got %q", got)
				}
				if got := r.PostForm.Get("message[content]"); got != richHTML {
					t.Errorf("rich HTML changed:\nwant: %s\n got: %s", richHTML, got)
				}
				if got := r.PostForm.Get("authenticity_token"); got != "csrf-update-token" {
					t.Errorf("expected form authenticity token, got %q", got)
				}
				if _, present := r.PostForm["_method"]; present {
					t.Error("canonical PATCH request unexpectedly included a method override")
				}
				if got := strings.Join(r.PostForm["entry[addressed][directly][]"], ","); got != "one@example.com,two@example.org" {
					t.Errorf("unexpected To recipients: %q", got)
				}
				if got := strings.Join(r.PostForm["entry[addressed][copied][]"], ","); got != "copy@example.com" {
					t.Errorf("unexpected CC recipients: %q", got)
				}
				if got := strings.Join(r.PostForm["entry[addressed][blindcopied][]"], ","); got != "blind@example.org" {
					t.Errorf("unexpected BCC recipients: %q", got)
				}

				if tc.location != "" {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)

			client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(2))
			result, err := client.Entries().UpdateDraft(context.Background(), 987, UpdateDraftParams{
				ActingSenderID:    84,
				Content:           richHTML,
				Subject:           "Re: Project update",
				To:                []string{"one@example.com", "two@example.org"},
				CC:                []string{"copy@example.com"},
				BCC:               []string{"blind@example.org"},
				AuthenticityToken: "csrf-update-token",
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.ID != 987 || result.StatusCode != tc.status || result.Location != tc.location || result.EditURL != "/messages/987/edit" {
					t.Errorf("unexpected result: %+v", result)
				}
			}
			if requests != 1 {
				t.Errorf("expected exactly one request with no redirect follow or retry, got %d", requests)
			}
		})
	}
}

func TestEntriesService_UpdateDraftValidatesInput(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://app.hey.com"}, &StaticTokenProvider{Token: "test-token"})
	valid := UpdateDraftParams{ActingSenderID: 42, AuthenticityToken: "token"}

	if _, err := client.Entries().UpdateDraft(context.Background(), 0, valid); err == nil {
		t.Fatal("expected non-positive message ID error")
	}
	if _, err := client.Entries().UpdateDraft(context.Background(), 1, UpdateDraftParams{AuthenticityToken: "token"}); err == nil {
		t.Fatal("expected non-positive acting sender ID error")
	}
	if _, err := client.Entries().UpdateDraft(context.Background(), 1, UpdateDraftParams{ActingSenderID: 42}); err == nil {
		t.Fatal("expected missing authenticity token error")
	}
	if _, err := client.Entries().UpdateDraft(context.Background(), 1, UpdateDraftParams{ActingSenderID: 42, AuthenticityToken: "   "}); err == nil {
		t.Fatal("expected blank authenticity token error")
	}
}

func TestEntriesService_DeleteDraft(t *testing.T) {
	metadata, ok := generated.GetOperationMetadata("DeleteDraft")
	if !ok {
		t.Fatal("DeleteDraft metadata missing")
	}
	if metadata.Idempotent {
		t.Fatal("DeleteDraft browser form must remain single-attempt")
	}
	if !metadata.HasSensitiveParams {
		t.Fatal("DeleteDraft metadata must mark its CSRF token as sensitive")
	}

	for _, tc := range []struct {
		name     string
		status   int
		location string
		wantErr  bool
	}{
		{name: "ok", status: http.StatusOK},
		{name: "found redirect", status: http.StatusFound, location: "/topics/123"},
		{name: "see other redirect", status: http.StatusSeeOther, location: "/topics/123"},
		{name: "server failure is not retried", status: http.StatusServiceUnavailable, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPost {
					t.Errorf("expected browser-compatible POST, got %s", r.Method)
				}
				if r.URL.Path != "/messages/987" {
					t.Errorf("expected draft message path, got %s", r.URL.Path)
				}
				if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
					t.Errorf("expected form content type, got %q", contentType)
				}
				if accept := r.Header.Get("Accept"); accept != "*/*" {
					t.Errorf("expected Accept */*, got %q", accept)
				}
				if csrf := r.Header.Get("X-CSRF-Token"); csrf != "csrf-delete-token" {
					t.Errorf("expected CSRF header, got %q", csrf)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}
				if got := r.PostForm.Get("_method"); got != "delete" {
					t.Errorf("expected Rails delete override, got %q", got)
				}
				if got := r.PostForm.Get("status"); got != "drafted" {
					t.Errorf("expected drafted status, got %q", got)
				}
				if _, present := r.PostForm["authenticity_token"]; present {
					t.Error("CSRF token must be sent only in the header")
				}

				if tc.location != "" {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)

			client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(2))
			result, err := client.Entries().DeleteDraft(context.Background(), 987, DeleteDraftParams{
				AuthenticityToken: "csrf-delete-token",
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.StatusCode != tc.status || result.Location != tc.location {
					t.Errorf("unexpected result: %+v", result)
				}
			}
			if requests != 1 {
				t.Errorf("expected exactly one request with no redirect follow or retry, got %d", requests)
			}
		})
	}
}

func TestEntriesService_DeleteDraftValidatesInput(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://app.hey.com"}, &StaticTokenProvider{Token: "test-token"})

	if _, err := client.Entries().DeleteDraft(context.Background(), 0, DeleteDraftParams{AuthenticityToken: "token"}); err == nil {
		t.Fatal("expected non-positive message ID error")
	}
	if _, err := client.Entries().DeleteDraft(context.Background(), 1, DeleteDraftParams{}); err == nil {
		t.Fatal("expected missing authenticity token error")
	}
	if _, err := client.Entries().DeleteDraft(context.Background(), 1, DeleteDraftParams{AuthenticityToken: "   "}); err == nil {
		t.Fatal("expected blank authenticity token error")
	}
}

func TestReplyDraftFromLocation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		location string
		wantID   int64
		wantEdit string
		wantErr  bool
	}{
		{name: "relative", location: "/entries/drafts/42", wantID: 42, wantEdit: "/entries/drafts/42/edit"},
		{name: "relative edit", location: "/entries/drafts/43/edit", wantID: 43, wantEdit: "/entries/drafts/43/edit"},
		{name: "relative empty query", location: "/entries/drafts/44?", wantID: 44, wantEdit: "/entries/drafts/44/edit"},
		{name: "relative edit empty query", location: "/entries/drafts/45/edit?", wantID: 45, wantEdit: "/entries/drafts/45/edit"},
		{name: "encoded edit separator", location: "/entries/drafts/46%2Fedit?source=save", wantID: 46, wantEdit: "/entries/drafts/46/edit"},
		{name: "absolute", location: "https://app.hey.com/entries/drafts/99/?source=save#composer", wantID: 99, wantEdit: "https://app.hey.com/entries/drafts/99/edit"},
		{name: "absolute edit", location: "https://app.hey.com/entries/drafts/100/edit/?source=save#composer", wantID: 100, wantEdit: "https://app.hey.com/entries/drafts/100/edit"},
		{name: "live message", location: "/messages/101", wantID: 101, wantEdit: "/messages/101/edit"},
		{name: "live message edit", location: "/messages/102/edit", wantID: 102, wantEdit: "/messages/102/edit"},
		{name: "live message absolute", location: "https://app.hey.com/messages/103?source=save#composer", wantID: 103, wantEdit: "https://app.hey.com/messages/103/edit"},
		{name: "live message encoded edit separator", location: "/messages/104%2Fedit?source=save", wantID: 104, wantEdit: "/messages/104/edit"},
		{name: "missing", wantErr: true},
		{name: "malformed", location: "/entries/drafts/%zz", wantErr: true},
		{name: "nonnumeric", location: "/entries/drafts/not-a-number", wantErr: true},
		{name: "nonnumeric edit", location: "/entries/drafts/not-a-number/edit", wantErr: true},
		{name: "live message nonnumeric", location: "/messages/not-a-number", wantErr: true},
		{name: "live message nonnumeric edit", location: "/messages/not-a-number/edit", wantErr: true},
		{name: "edit without ID", location: "/edit", wantErr: true},
		{name: "unrelated numeric route", location: "/topics/42", wantErr: true},
		{name: "unexpected suffix", location: "/entries/drafts/42/preview", wantErr: true},
		{name: "repeated edit suffix", location: "/entries/drafts/42/edit/edit", wantErr: true},
		{name: "live message unexpected suffix", location: "/messages/42/preview", wantErr: true},
		{name: "live message repeated edit suffix", location: "/messages/42/edit/edit", wantErr: true},
		{name: "zero", location: "/entries/drafts/0", wantErr: true},
		{name: "negative", location: "/entries/drafts/-1", wantErr: true},
		{name: "live message zero", location: "/messages/0", wantErr: true},
		{name: "live message negative", location: "/messages/-1", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := replyDraftFromLocation(tc.location)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ID != tc.wantID || result.EditURL != tc.wantEdit || result.Location != tc.location {
				t.Errorf("unexpected result: %+v", result)
			}
		})
	}
}

// --- Contacts ---

func TestContactsService_List(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/contacts.json": `[{"id":1,"name":"Alice"}]`,
	})

	result, err := client.Contacts().List(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestContactsService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/contacts/%s": `{"id":1,"name":"Alice"}`,
	})

	result, err := client.Contacts().Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- Calendars ---

func TestCalendarsService_List(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/calendars.json": `{"calendars":[{"calendar":{"id":1,"name":"My Calendar"}}]}`,
	})

	result, err := client.Calendars().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestCalendarsService_GetRecordings(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/calendars/%s/recordings": `{"events":[{"id":1}]}`,
	})

	result, err := client.Calendars().GetRecordings(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- CalendarTodos ---

func TestCalendarTodosService_Create(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/calendar/todos.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			todo, ok := body["calendar_todo"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_todo wrapper")
			}
			if todo["title"] != "Do something" {
				t.Errorf("expected title 'Do something', got %v", todo["title"])
			}
			if _, ok := todo["starts_at"]; !ok {
				t.Error("missing starts_at")
			}
		},
		`{"id":1,"type":"CalendarTodo"}`,
	)

	result, err := client.CalendarTodos().Create(context.Background(), "Do something", "2026-03-13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestCalendarTodosService_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1,"type":"CalendarTodo"}`))
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	result, err := client.CalendarTodos().Complete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestCalendarTodosService_Uncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1,"type":"CalendarTodo"}`))
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	result, err := client.CalendarTodos().Uncomplete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestCalendarTodosService_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	err := client.CalendarTodos().Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Habits ---

func TestHabitsService_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1,"type":"Habit"}`))
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	result, err := client.Habits().Complete(context.Background(), "2026-03-09", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHabitsService_Uncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1,"type":"Habit"}`))
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	result, err := client.Habits().Uncomplete(context.Background(), "2026-03-09", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- TimeTracks ---

func TestTimeTracksService_GetOngoing(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/calendar/ongoing_time_track.json": `{"id":1,"type":"TimeTrack"}`,
	})

	result, err := client.TimeTracks().GetOngoing(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTimeTracksService_GetOngoing_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	result, err := client.TimeTracks().GetOngoing(context.Background())
	if err != nil {
		t.Fatalf("expected no error for 404 (ADR-004), got %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for no ongoing time track")
	}
}

func TestTimeTracksService_Start(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/calendar/ongoing_time_track.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if _, ok := body["calendar_time_track"]; !ok {
				t.Fatal("missing calendar_time_track wrapper")
			}
		},
		`{"id":1,"type":"TimeTrack"}`,
	)

	body := generated.StartTimeTrackJSONRequestBody{}
	result, err := client.TimeTracks().Start(context.Background(), body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTimeTracksService_Update(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PUT", "/calendar/time_tracks/%s.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if _, ok := body["calendar_time_track"]; !ok {
				t.Fatal("missing calendar_time_track wrapper")
			}
		},
		`{"id":1,"type":"TimeTrack"}`,
	)

	body := generated.UpdateTimeTrackJSONRequestBody{}
	result, err := client.TimeTracks().Update(context.Background(), 1, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestTimeTracksService_Stop(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PUT", "/calendar/time_tracks/%s.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			tt, ok := body["calendar_time_track"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_time_track wrapper")
			}
			if _, ok := tt["ends_at"]; !ok {
				t.Error("missing ends_at in stop body")
			}
		},
		`{"id":1,"type":"TimeTrack"}`,
	)

	err := client.TimeTracks().Stop(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Journal ---

func TestJournalService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/calendar/days/%s/journal_entry": `{"id":1,"type":"JournalEntry"}`,
	})

	result, err := client.Journal().Get(context.Background(), "2026-03-09")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestJournalService_Update(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PATCH", "/calendar/days/%s/journal_entry",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			entry, ok := body["calendar_journal_entry"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_journal_entry wrapper")
			}
			if entry["content"] != "Today was great" {
				t.Errorf("expected content 'Today was great', got %v", entry["content"])
			}
		},
		`{"id":1,"type":"JournalEntry"}`,
	)

	err := client.Journal().Update(context.Background(), "2026-03-09", "Today was great")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Search ---

func TestSearchService_Search(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/search.json": `{"topics":[{"id":1}]}`,
	})

	params := &generated.SearchParams{Q: "test query"}
	result, err := client.Search().Search(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- CalendarEvents ---

func TestCalendarEventsService_Create(t *testing.T) {
	client := newFormTestClient(t, "POST", "/calendar/events",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("calendar_event[calendar_id]") != "1" {
				t.Errorf("expected calendar_id 1, got %s", values.Get("calendar_event[calendar_id]"))
			}
			if values.Get("calendar_event[summary]") != "Meeting" {
				t.Errorf("expected summary 'Meeting', got %s", values.Get("calendar_event[summary]"))
			}
			if values.Get("calendar_event[starts_at]") != "2026-04-06" {
				t.Errorf("expected starts_at '2026-04-06', got %s", values.Get("calendar_event[starts_at]"))
			}
			if values.Get("calendar_event[all_day]") != "0" {
				t.Error("expected all_day to be 0")
			}
			if values.Get("calendar_event[starts_at_time]") != "10:00:00" {
				t.Errorf("expected starts_at_time '10:00:00', got %s", values.Get("calendar_event[starts_at_time]"))
			}
			if values.Get("calendar_event[ends_at_time]") != "11:00:00" {
				t.Errorf("expected ends_at_time '11:00:00', got %s", values.Get("calendar_event[ends_at_time]"))
			}
			if values.Get("calendar_event[starts_at_time_zone_name]") != "America/New_York" {
				t.Errorf("expected timezone 'America/New_York', got %s", values.Get("calendar_event[starts_at_time_zone_name]"))
			}
		},
		"/calendar/events/99",
	)

	id, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
		CalendarID: 1,
		Title:      "Meeting",
		StartsAt:   "2026-04-06",
		StartTime:  "10:00",
		EndTime:    "11:00",
		TimeZone:   "America/New_York",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 99 {
		t.Errorf("expected ID 99, got %d", id)
	}
}

func TestCalendarEventsService_Create_AllDay(t *testing.T) {
	client := newFormTestClient(t, "POST", "/calendar/events",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("calendar_event[all_day]") != "1" {
				t.Error("expected all_day to be 1")
			}
			if values.Get("calendar_event[starts_at_time]") != "" {
				t.Error("expected no starts_at_time for all-day event")
			}
			reminders := values["all_day_reminder_durations[]"]
			if len(reminders) != 1 || reminders[0] != "86400" {
				t.Errorf("expected all_day_reminder_durations [86400], got %v", reminders)
			}
		},
		"/calendar/events/100",
	)

	id, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
		CalendarID: 1,
		Title:      "Holiday",
		StartsAt:   "2026-04-06",
		AllDay:     true,
		Reminders:  []time.Duration{24 * time.Hour},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 100 {
		t.Errorf("expected ID 100, got %d", id)
	}
}

func TestCalendarEventsService_Update(t *testing.T) {
	newTitle := "Updated Meeting"
	client := newFormTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("calendar_event[summary]") != "Updated Meeting" {
				t.Errorf("expected summary 'Updated Meeting', got %s", values.Get("calendar_event[summary]"))
			}
		},
		"/calendar/events/99",
	)

	err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Title: &newTitle,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCalendarEventsService_Delete(t *testing.T) {
	client := newFormTestClient(t, "DELETE", "/calendar/events/%s",
		nil,
		"/calendar",
	)

	err := client.CalendarEvents().Delete(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Designations ---

func TestDesignationsService_Create(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/boxes/%s/designations.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			contactID, ok := body["contact_id"].(float64)
			if !ok || int64(contactID) != 42 {
				t.Errorf("expected contact_id 42, got %v", body["contact_id"])
			}
		},
		``,
	)

	err := client.Designations().Create(context.Background(), 5, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDesignationsService_Destroy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !pathMatch("/boxes/%s/designations/%s.json", r.URL.Path) {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	err := client.Designations().Destroy(context.Background(), 5, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Extenzions ---

func TestExtenzionsService_List(t *testing.T) {
	htmlContent := `<html><body>
		<div>
			<div>
				<span>sales@example.com</span>
				<a href="/accounts/1/domains/extenzions/10/edit">Edit</a>
				<span>alice@example.com</span>
			</div>
			<div>
				<span>support@example.com</span>
				<a href="/accounts/1/domains/extenzions/20/edit">Edit</a>
				<span>bob@example.com</span>
			</div>
		</div>
	</body></html>`

	client := newServiceTestClient(t, map[string]string{
		"/accounts/%s/domains/extenzions": htmlContent,
	})

	result, err := client.Extenzions().List(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 extenzions, got %d", len(result))
	}
	if result[0].ID != 10 {
		t.Errorf("expected first extenzion ID 10, got %d", result[0].ID)
	}
	if result[1].ID != 20 {
		t.Errorf("expected second extenzion ID 20, got %d", result[1].ID)
	}
}

func TestExtenzionsService_Create(t *testing.T) {
	client := newFormTestClient(t, "POST", "/accounts/%s/domains/extenzions",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("extenzion[name]") != "sales" {
				t.Errorf("expected name 'sales', got %s", values.Get("extenzion[name]"))
			}
			members := values["extenzion[members][]"]
			if len(members) != 1 || members[0] != "alice@example.com" {
				t.Errorf("expected members [alice@example.com], got %v", members)
			}
		},
		"/accounts/1/domains/extenzions/10",
	)

	id, err := client.Extenzions().Create(context.Background(), 1, CreateExtenzionParams{
		Name:    "sales",
		Members: []string{"alice@example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 10 {
		t.Errorf("expected ID 10, got %d", id)
	}
}

func TestExtenzionsService_Update(t *testing.T) {
	client := newFormTestClient(t, "PATCH", "/accounts/%s/domains/extenzions/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("extenzion[name]") != "support" {
				t.Errorf("expected name 'support', got %s", values.Get("extenzion[name]"))
			}
		},
		"/accounts/1/domains/extenzions/10",
	)

	err := client.Extenzions().Update(context.Background(), 1, 10, UpdateExtenzionParams{
		Name: "support",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtenzionsService_Delete(t *testing.T) {
	client := newFormTestClient(t, "DELETE", "/accounts/%s/domains/extenzions/%s",
		nil,
		"/accounts/1/domains/extenzions",
	)

	err := client.Extenzions().Delete(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- FormResponse ---

func TestFormResponse_ExtractID(t *testing.T) {
	tests := []struct {
		name     string
		location string
		wantID   int64
		wantErr  bool
	}{
		{"simple path", "/calendar/events/42", 42, false},
		{"full URL", "https://app.hey.com/calendar/events/99", 99, false},
		{"trailing slash", "/calendar/events/7/", 7, false},
		{"no ID", "/calendar", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &FormResponse{Location: tt.location, StatusCode: 302}
			id, err := resp.ExtractID()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("expected %d, got %d", tt.wantID, id)
			}
		})
	}
}
