package hey

import (
	"context"
	"encoding/json"
	"fmt"
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

// newFormJSONTestClient creates a Client pointing at a test server that validates
// form-encoded requests and answers the written resource, the way the .json write
// endpoints do.
func newFormJSONTestClient(t *testing.T, wantMethod, wantPath string, validateForm func(t *testing.T, values url.Values), status int, responseJSON string) *Client {
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
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
		"/topics/%s/entries.json": `[{"id":1}]`,
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
			directly, ok := addressed["directly"].([]any)
			if !ok || len(directly) != 1 || directly[0] != "test@example.com" {
				t.Errorf("expected directly ['test@example.com'], got %v", addressed["directly"])
			}
			copied, ok := addressed["copied"].([]any)
			if !ok || len(copied) != 1 || copied[0] != "cc@example.com" {
				t.Errorf("expected copied ['cc@example.com'], got %v", addressed["copied"])
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
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, "My reply", []string{"test@example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_OmitsEntryWithoutRecipients(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			// An empty entry.addressed would clear the recipient list server-side
			// (Entry#enter_reply treats {} as explicit), so it must be omitted.
			if _, present := body["entry"]; present {
				t.Errorf("entry must be omitted when no recipients are given, got %v", body["entry"])
			}
		},
		`{"notice":"sent"}`,
	)

	if err := client.Entries().CreateReply(context.Background(), 10, "Reply-all", nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_RecipientsAreArrays(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			entry, _ := body["entry"].(map[string]any)
			addressed, _ := entry["addressed"].(map[string]any)
			if _, ok := addressed["directly"].([]any); !ok {
				t.Errorf("directly must be a JSON array, got %T", addressed["directly"])
			}
			if _, ok := addressed["blindcopied"].([]any); !ok {
				t.Errorf("blindcopied must be a JSON array, got %T", addressed["blindcopied"])
			}
			if _, present := addressed["copied"]; present {
				t.Errorf("empty cc must be omitted, got %v", addressed["copied"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, "hi", []string{"a@x.com"}, nil, []string{"b@x.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		"/calendars/%s/recordings.json": `{"events":[{"id":1}]}`,
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
			// Stop must not touch the start: a zero-valued starts_at would be
			// applied by the server and rewrite the track to year 0001.
			for _, k := range []string{"starts_at", "category", "notes", "title"} {
				if v, present := tt[k]; present {
					t.Errorf("stop body must only carry ends_at; got %s=%v", k, v)
				}
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
		"/calendar/days/%s/journal_entry.json": `{"id":1,"type":"JournalEntry"}`,
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
	page := `<div id="fulltext_search_results">
		<article class="search-result posting" id="posting_4471829">
			<a href="/topics/331/entries/5512">
				<h3><span id="topic_name_posting_4471829" class="posting__title">Kitchen remodel</span></h3>
				<span id="creator_posting_4471829">Jane Dawson</span>
				<span id="summary_posting_4471829">The cabinets arrive on Tuesday</span>
			</a>
		</article>
	</div>`

	var requestedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(page)) //nolint:errcheck // test server
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	matches, err := client.Search().Search(context.Background(), SearchParams{Query: "cabinets", In: "imbox", Date: "last_30_days"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(requestedURL, "q=cabinets") {
		t.Errorf("request %q should carry the query", requestedURL)
	}
	if !strings.Contains(requestedURL, url.QueryEscape("refine[in]")+"=imbox") {
		t.Errorf("request %q should carry the box refinement", requestedURL)
	}

	if len(matches) != 1 {
		t.Fatalf("expected one match, got %d", len(matches))
	}
	if matches[0].PostingID != 4471829 || matches[0].TopicID != 331 {
		t.Errorf("expected posting 4471829 on topic 331, got %+v", matches[0])
	}
	if matches[0].Title != "Kitchen remodel" || matches[0].Creator != "Jane Dawson" {
		t.Errorf("expected the title and creator, got %+v", matches[0])
	}
	if matches[0].Summary != "The cabinets arrive on Tuesday" {
		t.Errorf("expected the summary, got %q", matches[0].Summary)
	}
}

func TestSearchService_Filters(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/advanced_search_filters.json": `{"refine_in":[{"title":"Imbox","value":"imbox"}],"refine_labels":[{"title":"Receipts","value":"Receipts"}]}`,
	})

	filters, err := client.Search().Filters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters.RefineIn) != 1 || filters.RefineIn[0].Value != "imbox" {
		t.Errorf("expected the imbox refinement, got %+v", filters.RefineIn)
	}
}

// --- CalendarEvents ---

// calendarEventJSON is what the .json create and update branches answer with — the same
// recording shape GET /calendars/{id}/recordings serves.
const calendarEventJSON = `{
	"id": 99,
	"title": "Meeting",
	"type": "Calendar::Event",
	"all_day": false,
	"starts_at": "2026-04-06T14:00:00Z",
	"ends_at": "2026-04-06T15:00:00Z",
	"calendar": {"id": 1, "name": "David Heinemeier Hansson"}
}`

func TestCalendarEventsService_Create(t *testing.T) {
	client := newFormJSONTestClient(t, "POST", "/calendar/events.json",
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
		201, calendarEventJSON,
	)

	recording, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
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
	if recording.Id != 99 {
		t.Errorf("expected ID 99, got %d", recording.Id)
	}
	if recording.Title != "Meeting" {
		t.Errorf("expected the title, got %q", recording.Title)
	}
	if recording.Type != "Calendar::Event" {
		t.Errorf("expected the recording type, got %q", recording.Type)
	}
	if recording.Calendar.Id != 1 {
		t.Errorf("expected calendar 1, got %d", recording.Calendar.Id)
	}
}

// A server without the JSON create branch redirects to the event, which still names its id.
func TestCalendarEventsService_Create_OnAServerWithoutTheJSONBranch(t *testing.T) {
	client := newFormTestClient(t, "POST", "/calendar/events.json", nil, "/calendar/events/99")

	recording, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
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
	if recording.Id != 99 {
		t.Errorf("expected ID 99, got %d", recording.Id)
	}
	if recording.Title != "" {
		t.Errorf("expected no title from a redirect, got %q", recording.Title)
	}
}

func TestCalendarEventsService_Create_AllDay(t *testing.T) {
	client := newFormJSONTestClient(t, "POST", "/calendar/events.json",
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
		201, `{"id": 100, "title": "Holiday", "type": "Calendar::Event", "all_day": true}`,
	)

	recording, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
		CalendarID: 1,
		Title:      "Holiday",
		StartsAt:   "2026-04-06",
		AllDay:     true,
		Reminders:  []time.Duration{24 * time.Hour},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 100 {
		t.Errorf("expected ID 100, got %d", recording.Id)
	}
	if !recording.AllDay {
		t.Error("expected an all-day recording")
	}
}

func TestCalendarEventsService_Update(t *testing.T) {
	newTitle := "Updated Meeting"
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("calendar_event[summary]") != "Updated Meeting" {
				t.Errorf("expected summary 'Updated Meeting', got %s", values.Get("calendar_event[summary]"))
			}
		},
		200, `{"id": 99, "title": "Updated Meeting", "type": "Calendar::Event"}`,
	)

	recording, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Title: &newTitle,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 99 {
		t.Errorf("expected ID 99, got %d", recording.Id)
	}
	if recording.Title != "Updated Meeting" {
		t.Errorf("expected the new title, got %q", recording.Title)
	}
}

// A server without the JSON update branch redirects to the event, which still names its id.
func TestCalendarEventsService_Update_OnAServerWithoutTheJSONBranch(t *testing.T) {
	newTitle := "Updated Meeting"
	client := newFormTestClient(t, "PATCH", "/calendar/events/%s", nil, "/calendar/events/99")

	recording, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Title: &newTitle,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 99 {
		t.Errorf("expected ID 99, got %d", recording.Id)
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
	navigationJSON := `{
		"items": [
			{"title": "Imbox", "app_url": "https://app.hey.com/imbox"},
			{"title": "Extensions", "menu_items": [
				{"title": "All Extensions", "app_url": "https://app.hey.com/accounts/1/domains/extenzions"},
				{"title": "sales", "app_url": "https://app.hey.com/contacts/10"},
				{"title": "support", "app_url": "https://app.hey.com/contacts/20"}
			]}
		]
	}`

	client := newServiceTestClient(t, map[string]string{
		"/my/navigation.json": navigationJSON,
	})

	result, err := client.Extenzions().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 extenzions, got %d", len(result))
	}
	if result[0].ID != 10 || result[0].Name != "sales" {
		t.Errorf("expected sales with ID 10, got %s with ID %d", result[0].Name, result[0].ID)
	}
	if result[1].ID != 20 || result[1].Name != "support" {
		t.Errorf("expected support with ID 20, got %s with ID %d", result[1].Name, result[1].ID)
	}
}

func TestExtenzionsService_Create(t *testing.T) {
	client := newFormJSONTestClient(t, "POST", "/accounts/%s/domains/extenzions.json",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("extenzion[name]") != "sales" {
				t.Errorf("expected name 'sales', got %s", values.Get("extenzion[name]"))
			}
			members := values["extenzion[members][]"]
			if len(members) != 1 || members[0] != "jane.dawson@example.com" {
				t.Errorf("expected members [jane.dawson@example.com], got %v", members)
			}
		},
		201, `{"id": 55, "name": "sales", "app_url": "https://app.hey.com/contacts/10"}`,
	)

	extenzion, err := client.Extenzions().Create(context.Background(), 1, CreateExtenzionParams{
		Name:    "sales",
		Members: []string{"jane.dawson@example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The id is the contact's, out of app_url — the payload's own id belongs to the
	// Extenzion record, which no write endpoint takes.
	if extenzion.ID != 10 {
		t.Errorf("expected ID 10, got %d", extenzion.ID)
	}
	if extenzion.Name != "sales" {
		t.Errorf("expected the name, got %q", extenzion.Name)
	}
	if extenzion.AppURL != "https://app.hey.com/contacts/10" {
		t.Errorf("expected the app URL, got %q", extenzion.AppURL)
	}
}

// A server without the JSON create branch redirects to the extensions page, which names
// nothing about the new extension.
func TestExtenzionsService_Create_OnAServerWithoutTheJSONBranch(t *testing.T) {
	client := newFormTestClient(t, "POST", "/accounts/%s/domains/extenzions.json", nil, "/accounts/1/domains/extenzions")

	extenzion, err := client.Extenzions().Create(context.Background(), 1, CreateExtenzionParams{Name: "sales"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if extenzion != nil {
		t.Errorf("expected no extension back, got %+v", extenzion)
	}
}

func TestExtenzionsService_Update(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/accounts/%s/domains/extenzions/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if values.Get("extenzion[name]") != "support" {
				t.Errorf("expected name 'support', got %s", values.Get("extenzion[name]"))
			}
		},
		200, `{"id": 55, "name": "support", "app_url": "https://app.hey.com/contacts/10"}`,
	)

	extenzion, err := client.Extenzions().Update(context.Background(), 1, 10, UpdateExtenzionParams{
		Name: "support",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if extenzion.ID != 10 {
		t.Errorf("expected ID 10, got %d", extenzion.ID)
	}
	if extenzion.Name != "support" {
		t.Errorf("expected the new name, got %q", extenzion.Name)
	}
}

func TestExtenzionsService_Update_OnAServerWithoutTheJSONBranch(t *testing.T) {
	client := newFormTestClient(t, "PATCH", "/accounts/%s/domains/extenzions/%s", nil, "/accounts/1/domains/extenzions")

	extenzion, err := client.Extenzions().Update(context.Background(), 1, 10, UpdateExtenzionParams{Name: "support"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if extenzion != nil {
		t.Errorf("expected no extension back, got %+v", extenzion)
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

// --- Helpers for the 1.0 coverage pass ---

// newRequestTestClient serves a single endpoint and hands the whole request to the test, so
// query strings and bodies alike can be asserted on.
func newRequestTestClient(t *testing.T, wantMethod, wantPath string, inspect func(t *testing.T, r *http.Request), status int, responseJSON string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != wantMethod {
			t.Errorf("expected %s, got %s", wantMethod, r.Method)
		}
		if !pathMatch(wantPath, r.URL.Path) {
			t.Errorf("expected path matching %s, got %s", wantPath, r.URL.Path)
		}
		if inspect != nil {
			inspect(t, r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if responseJSON != "" {
			w.Write([]byte(responseJSON))
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

// requestBody parses a request's JSON body for assertions.
func requestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("failed to read request body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	return body
}

// --- Postings, bulk ---

func TestPostingsService_MarkSpam(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/postings/spam.json", nil, 204, "")

	if err := client.Postings().MarkSpam(context.Background(), 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostingsService_BoxGroupMembership(t *testing.T) {
	adder := newRequestTestClient(t, "POST", "/postings/box_groups.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		body := requestBody(t, r)
		if fmt.Sprint(body["box_id"]) != "5" || fmt.Sprint(body["box_group_id"]) != "9" {
			t.Errorf("expected box 5 and group 9, got %v and %v", body["box_id"], body["box_group_id"])
		}
	}, 201, "")
	if err := adder.Postings().AddToBoxGroup(context.Background(), 5, 9, 1); err != nil {
		t.Fatalf("unexpected error adding: %v", err)
	}

	remover := newRequestTestClient(t, "DELETE", "/postings/box_groups.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get("posting_ids"); got != "1" {
			t.Errorf("expected posting_ids 1, got %q", got)
		}
	}, 204, "")
	if err := remover.Postings().RemoveFromBoxGroup(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error removing: %v", err)
	}
}

func TestPostingsService_FileAndUnfile(t *testing.T) {
	filer := newRequestTestClient(t, "POST", "/postings/filings.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if body := requestBody(t, r); fmt.Sprint(body["folder_id"]) != "12" {
			t.Errorf("expected folder_id 12, got %v", body["folder_id"])
		}
	}, 201, "")
	if err := filer.Postings().File(context.Background(), 12, 1); err != nil {
		t.Fatalf("unexpected error filing: %v", err)
	}

	unfiler := newRequestTestClient(t, "DELETE", "/postings/filings.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get("folder_id"); got != "12" {
			t.Errorf("expected folder_id 12, got %q", got)
		}
	}, 204, "")
	if err := unfiler.Postings().Unfile(context.Background(), 12, 1); err != nil {
		t.Fatalf("unexpected error unfiling: %v", err)
	}
}

func TestPostingsService_UnfileEveryLabel(t *testing.T) {
	client := newRequestTestClient(t, "DELETE", "/postings/filings.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if r.URL.Query().Has("folder_id") {
			t.Error("expected folder_id to be left out when removing every label")
		}
	}, 204, "")

	if err := client.Postings().Unfile(context.Background(), 0, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostingsService_CreateFolder(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/postings/folders.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		folder, ok := requestBody(t, r)["folder"].(map[string]any)
		if !ok {
			t.Fatal("missing folder wrapper")
		}
		if folder["name"] != "Receipts" {
			t.Errorf("expected name Receipts, got %v", folder["name"])
		}
	}, 201, "")

	if err := client.Postings().CreateFolder(context.Background(), "Receipts", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostingsService_BubbleUp(t *testing.T) {
	canceller := newRequestTestClient(t, "DELETE", "/postings/bubble_up.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get("posting_ids"); got != "1,2" {
			t.Errorf("expected posting_ids 1,2, got %q", got)
		}
	}, 204, "")
	if err := canceller.Postings().CancelBubbleUp(context.Background(), 1, 2); err != nil {
		t.Fatalf("unexpected error cancelling: %v", err)
	}

	bubbler := newRequestTestClient(t, "POST", "/postings/bulk_bubble_up_now.json", nil, 204, "")
	if err := bubbler.Postings().BubbleUpNow(context.Background(), 1, 2); err != nil {
		t.Fatalf("unexpected error bubbling: %v", err)
	}
}

// --- Topics, status and moves ---

func TestTopicsService_Trash(t *testing.T) {
	client := newRequestTestClient(t, "PUT", "/topics/%s/status/trashed.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get("confirm_destroy"); got != "1" {
			t.Errorf("expected confirm_destroy 1, got %q", got)
		}
	}, 204, "")

	if err := client.Topics().Trash(context.Background(), 4471829, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTopicsService_TrashWithoutConfirmation(t *testing.T) {
	client := newRequestTestClient(t, "PUT", "/topics/%s/status/trashed.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if r.URL.Query().Has("confirm_destroy") {
			t.Error("expected confirm_destroy to be left out")
		}
	}, 204, "")

	if err := client.Topics().Trash(context.Background(), 4471829, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTopicsService_RestoreAndHam(t *testing.T) {
	restorer := newRequestTestClient(t, "PUT", "/topics/%s/status/active.json", nil, 204, "")
	if err := restorer.Topics().Restore(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error restoring: %v", err)
	}

	hammer := newRequestTestClient(t, "PUT", "/topics/%s/status/ham.json", nil, 204, "")
	if err := hammer.Topics().MarkHam(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error hamming: %v", err)
	}
}

func TestTopicsService_EmptyTrashAndSpam(t *testing.T) {
	trash := newRequestTestClient(t, "DELETE", "/topics/trash/all.json", nil, 204, "")
	if err := trash.Topics().EmptyTrash(context.Background()); err != nil {
		t.Fatalf("unexpected error emptying trash: %v", err)
	}

	spam := newRequestTestClient(t, "DELETE", "/topics/spam/all.json", nil, 204, "")
	if err := spam.Topics().EmptySpam(context.Background()); err != nil {
		t.Fatalf("unexpected error emptying spam: %v", err)
	}
}

func TestTopicsService_Move(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/topics/%s/moves.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if body := requestBody(t, r); fmt.Sprint(body["box_id"]) != "88" {
			t.Errorf("expected box_id 88, got %v", body["box_id"])
		}
	}, 201, "")

	if err := client.Topics().Move(context.Background(), 4471829, 88); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Entries ---

func TestEntriesService_MarkSpam(t *testing.T) {
	client := newRequestTestClient(t, "PUT", "/entries/%s/status/spam.json", nil, 204, "")

	if err := client.Entries().MarkSpam(context.Background(), 5512); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_NewForward(t *testing.T) {
	client := newRequestTestClient(t, "GET", "/entries/%s/forwards/new.json", nil, 200,
		`{"url":"https://app.hey.com/messages","subject":"Fwd: Quarterly planning","content":"<div>quoted</div>","is_reply":false}`)

	draft, err := client.Entries().NewForward(context.Background(), 5512)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if draft.Subject != "Fwd: Quarterly planning" {
		t.Errorf("expected the forwarded subject, got %q", draft.Subject)
	}
	if draft.Url != "https://app.hey.com/messages" {
		t.Errorf("expected the compose URL, got %q", draft.Url)
	}
}

// --- Contacts, bundles and screening ---

func TestContactsService_BundleAndUnbundle(t *testing.T) {
	bundler := newRequestTestClient(t, "POST", "/contacts/%s/bundle.json", nil, 201, "")
	if err := bundler.Contacts().Bundle(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error bundling: %v", err)
	}

	unbundler := newRequestTestClient(t, "DELETE", "/contacts/%s/bundle.json", nil, 204, "")
	if err := unbundler.Contacts().Unbundle(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error unbundling: %v", err)
	}
}

func TestContactsService_Screen(t *testing.T) {
	client := newRequestTestClient(t, "PATCH", "/contacts/%s/clearance.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if body := requestBody(t, r); body["status"] != ClearanceDenied {
			t.Errorf("expected status denied, got %v", body["status"])
		}
	}, 204, "")

	if err := client.Contacts().Screen(context.Background(), 91824, ClearanceDenied); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContactsService_ScreenRejectsUnknownStatus(t *testing.T) {
	client := newRequestTestClient(t, "PATCH", "/contacts/%s/clearance.json", nil, 204, "")

	err := client.Contacts().Screen(context.Background(), 91824, "maybe")
	if err == nil {
		t.Fatal("expected an error for an unknown clearance status")
	}
	if AsError(err).Code != CodeValidation {
		t.Errorf("expected a validation error, got %v", err)
	}
}

func TestContactsService_Clearances(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/clearances.json": `{"pending_clearances_count":3,"signed_stream_name":"abc"}`,
	})

	summary, err := client.Contacts().Clearances(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.PendingClearancesCount != 3 {
		t.Errorf("expected 3 pending clearances, got %d", summary.PendingClearancesCount)
	}
}

// --- Boxes, groups and observation ---

func TestBoxesService_ListGroups(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/boxes/%s/groups.json": `{"box_groups":[{"id":11},{"id":12}]}`,
	})

	groups, err := client.Boxes().ListGroups(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups.BoxGroups) != 2 || groups.BoxGroups[0].Id != 11 {
		t.Errorf("expected groups 11 and 12, got %+v", groups.BoxGroups)
	}
}

func TestBoxesService_CreateGroup(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/boxes/%s/groups.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if body := requestBody(t, r); fmt.Sprint(body["posting_ids"]) != "[1 2]" {
			t.Errorf("expected posting_ids [1 2], got %v", body["posting_ids"])
		}
	}, 200, `{"id":11}`)

	group, err := client.Boxes().CreateGroup(context.Background(), 5, []int64{1, 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.Id != 11 {
		t.Errorf("expected group 11, got %d", group.Id)
	}
}

func TestBoxesService_DeleteGroup(t *testing.T) {
	client := newRequestTestClient(t, "DELETE", "/boxes/%s/groups/%s.json", nil, 204, "")

	if err := client.Boxes().DeleteGroup(context.Background(), 5, 11); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBoxesService_MarkSeen(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/boxes/%s/observation.json", nil, 201, "")

	if err := client.Boxes().MarkSeen(context.Background(), 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Folders ---

func TestFoldersService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/folders/%s": `{"id":12,"name":"Receipts","postings":[{"id":1,"kind":"topic"}]}`,
	})

	folder, err := client.Folders().Get(context.Background(), 12, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder.Name != "Receipts" || len(folder.Postings) != 1 {
		t.Errorf("expected Receipts with one posting, got %+v", folder)
	}
}

// --- Collections ---

func TestCollectionsService_List(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/collections.json": `[{"id":3,"name":"Kitchen remodel"}]`,
	})

	collections, err := client.Collections().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*collections) != 1 || (*collections)[0].Name != "Kitchen remodel" {
		t.Errorf("expected one Kitchen remodel collection, got %+v", collections)
	}
}

func TestCollectionsService_Update(t *testing.T) {
	client := newRequestTestClient(t, "PATCH", "/collections/%s.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		collection, ok := requestBody(t, r)["collection"].(map[string]any)
		if !ok {
			t.Fatal("missing collection wrapper")
		}
		if collection["name"] != "Kitchen remodel 2026" {
			t.Errorf("expected the new name, got %v", collection["name"])
		}
		if _, ok := collection["summary"]; ok {
			t.Error("expected summary to be left out when unset")
		}
	}, 204, "")

	if err := client.Collections().Update(context.Background(), 3, UpdateCollectionParams{Name: "Kitchen remodel 2026"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectionsService_Create(t *testing.T) {
	client := newFormTestClient(t, "POST", "/collections", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("collection[name]") != "Kitchen remodel" {
			t.Errorf("expected the collection name, got %q", values.Get("collection[name]"))
		}
		if values.Get("account_id") != "77" {
			t.Errorf("expected account_id 77, got %q", values.Get("account_id"))
		}
	}, "/collections")

	err := client.Collections().Create(context.Background(), CreateCollectionParams{Name: "Kitchen remodel", AccountID: 77})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectionsService_AddAndRemoveTopic(t *testing.T) {
	adder := newFormTestClient(t, "POST", "/topics/%s/collecting", nil, "/topics/4471829")
	if err := adder.Collections().AddTopic(context.Background(), 4471829, 3); err != nil {
		t.Fatalf("unexpected error adding: %v", err)
	}

	remover := newFormTestClient(t, "DELETE", "/topics/%s/collecting", nil, "/topics/4471829")
	if err := remover.Collections().RemoveTopic(context.Background(), 4471829, 3); err != nil {
		t.Fatalf("unexpected error removing: %v", err)
	}
}

// --- Stickies ---

func TestStickiesService_List(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/stickies.json": `[{"id":1,"body":"Call the plumber","size":"medium"}]`,
	})

	stickies, err := client.Stickies().List(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*stickies) != 1 || (*stickies)[0].Body != "Call the plumber" {
		t.Errorf("expected one sticky, got %+v", stickies)
	}
}

func TestStickiesService_Create(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/stickies.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		sticky, ok := requestBody(t, r)["sticky"].(map[string]any)
		if !ok {
			t.Fatal("missing sticky wrapper")
		}
		if sticky["body"] != "Call the plumber" || sticky["size"] != StickyLarge {
			t.Errorf("expected the sticky body and size, got %v", sticky)
		}
	}, 200, `{"id":1,"body":"Call the plumber","size":"large"}`)

	sticky, err := client.Stickies().Create(context.Background(), "Call the plumber", StickyLarge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sticky.Id != 1 {
		t.Errorf("expected sticky 1, got %d", sticky.Id)
	}
}

func TestStickiesService_Update(t *testing.T) {
	client := newRequestTestClient(t, "PATCH", "/stickies/%s.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		sticky, ok := requestBody(t, r)["sticky"].(map[string]any)
		if !ok {
			t.Fatal("missing sticky wrapper")
		}
		if sticky["body"] != "Call the electrician" {
			t.Errorf("expected the new body, got %v", sticky["body"])
		}
		if _, ok := sticky["size"]; ok {
			t.Error("expected size to be left out when unset")
		}
	}, 200, `{"id":1,"body":"Call the electrician"}`)

	sticky, err := client.Stickies().Update(context.Background(), 1, "Call the electrician", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sticky.Body != "Call the electrician" {
		t.Errorf("expected the new body back, got %q", sticky.Body)
	}
}

func TestStickiesService_DeleteAndMove(t *testing.T) {
	deleter := newRequestTestClient(t, "DELETE", "/stickies/%s.json", nil, 204, "")
	if err := deleter.Stickies().Delete(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	mover := newRequestTestClient(t, "POST", "/stickies/moves.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		body := requestBody(t, r)
		if fmt.Sprint(body["id"]) != "1" || fmt.Sprint(body["position"]) != "2" {
			t.Errorf("expected sticky 1 at position 2, got %v", body)
		}
	}, 204, "")
	if err := mover.Stickies().Move(context.Background(), 1, 2); err != nil {
		t.Fatalf("unexpected error moving: %v", err)
	}
}

func TestStickiesService_ListClampsLimit(t *testing.T) {
	client := newRequestTestClient(t, "GET", "/stickies.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("limit = %q, want it clamped to 100", got)
		}
	}, 200, `[]`)

	if _, err := client.Stickies().List(context.Background(), 5000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStickiesService_MoveRejectsOutOfRangePosition(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/stickies/moves.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		t.Error("expected no request for an out-of-range position")
	}, 204, "")

	for _, position := range []int{-1, MaxStickyPosition + 1} {
		err := client.Stickies().Move(context.Background(), 1, position)
		if err == nil {
			t.Fatalf("expected an error for position %d", position)
		}
		if AsError(err).Code != CodeUsage {
			t.Errorf("position %d gave code %q, want %q", position, AsError(err).Code, CodeUsage)
		}
	}
}

// --- Time tracks, write ---

func TestTimeTracksService_Create(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/calendar/time_tracks.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		body := requestBody(t, r)
		if body["category_title"] != "Client work" {
			t.Errorf("expected the category title, got %v", body["category_title"])
		}
		if _, ok := body["starts_at"]; !ok {
			t.Error("missing starts_at")
		}
	}, 200, `{"id":1,"type":"Calendar::TimeTrack"}`)

	recording, err := client.TimeTracks().Create(context.Background(), generated.CreateTimeTrackJSONRequestBody{
		StartsAt:      time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC),
		EndsAt:        time.Date(2026, 4, 6, 11, 0, 0, 0, time.UTC),
		CategoryTitle: "Client work",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 1 {
		t.Errorf("expected recording 1, got %d", recording.Id)
	}
}

func TestTimeTracksService_Delete(t *testing.T) {
	client := newRequestTestClient(t, "DELETE", "/calendar/time_tracks/%s.json", nil, 204, "")

	if err := client.TimeTracks().Delete(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Designations ---

func TestDesignationsService_CreateAndDestroy(t *testing.T) {
	creator := newRequestTestClient(t, "POST", "/boxes/%s/designations.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		if body := requestBody(t, r); fmt.Sprint(body["contact_id"]) != "91824" {
			t.Errorf("expected contact_id 91824, got %v", body["contact_id"])
		}
	}, 201, "")
	if err := creator.Designations().Create(context.Background(), 5, 91824); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	destroyer := newRequestTestClient(t, "DELETE", "/boxes/%s/designations/%s.json", nil, 204, "")
	if err := destroyer.Designations().Destroy(context.Background(), 5, 44); err != nil {
		t.Fatalf("unexpected error destroying: %v", err)
	}
}

// --- Habit CRUD ---

func TestHabitsService_Create(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/calendar/habits.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		habit, ok := requestBody(t, r)["calendar_habit"].(map[string]any)
		if !ok {
			t.Fatal("missing calendar_habit wrapper")
		}
		if habit["name"] != "Morning run" {
			t.Errorf("expected the habit name, got %v", habit["name"])
		}
		if fmt.Sprint(habit["days"]) != "[1 3 5]" {
			t.Errorf("expected days [1 3 5], got %v", habit["days"])
		}
	}, 201, `{"id": 7712, "title": "Morning run", "type": "Calendar::Habit", "days": [1, 3, 5]}`)

	recording, err := client.Habits().Create(context.Background(), HabitParams{Name: "Morning run", Days: []int32{1, 3, 5}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 7712 {
		t.Errorf("expected ID 7712, got %d", recording.Id)
	}
	if recording.Title != "Morning run" {
		t.Errorf("expected the habit name, got %q", recording.Title)
	}
	if fmt.Sprint(recording.Days) != "[1 3 5]" {
		t.Errorf("expected days [1 3 5], got %v", recording.Days)
	}
}

// A server without the JSON create branch redirects to the habits page, which names nothing
// about the new habit.
func TestHabitsService_Create_OnAServerWithoutTheJSONBranch(t *testing.T) {
	client := newRequestTestClient(t, "POST", "/calendar/habits.json", nil, 204, "")

	recording, err := client.Habits().Create(context.Background(), HabitParams{Name: "Morning run", Days: []int32{1, 3, 5}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording != nil {
		t.Errorf("expected no recording back, got %+v", recording)
	}
}

func TestHabitsService_Update(t *testing.T) {
	client := newRequestTestClient(t, "PATCH", "/calendar/habits/%s.json", func(t *testing.T, r *http.Request) {
		t.Helper()
		habit, ok := requestBody(t, r)["calendar_habit"].(map[string]any)
		if !ok {
			t.Fatal("missing calendar_habit wrapper")
		}
		if habit["name"] != "Evening run" {
			t.Errorf("expected the new name, got %v", habit["name"])
		}
		if _, ok := habit["color"]; ok {
			t.Error("expected color to be left out when unset")
		}
	}, 200, `{"id": 7712, "title": "Evening run", "type": "Calendar::Habit"}`)

	recording, err := client.Habits().Update(context.Background(), 7712, HabitParams{Name: "Evening run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 7712 {
		t.Errorf("expected ID 7712, got %d", recording.Id)
	}
	if recording.Title != "Evening run" {
		t.Errorf("expected the new name, got %q", recording.Title)
	}
}

func TestHabitsService_Update_OnAServerWithoutTheJSONBranch(t *testing.T) {
	client := newRequestTestClient(t, "PATCH", "/calendar/habits/%s.json", nil, 204, "")

	recording, err := client.Habits().Update(context.Background(), 7712, HabitParams{Name: "Evening run"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording != nil {
		t.Errorf("expected no recording back, got %+v", recording)
	}
}

func TestHabitsService_DeleteStopAndResume(t *testing.T) {
	deleter := newRequestTestClient(t, "DELETE", "/calendar/habits/%s.json", nil, 204, "")
	if err := deleter.Habits().Delete(context.Background(), 7712); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	stopper := newRequestTestClient(t, "POST", "/calendar/habits/%s/stop.json", nil, 204, "")
	if err := stopper.Habits().Stop(context.Background(), 7712); err != nil {
		t.Fatalf("unexpected error stopping: %v", err)
	}

	resumer := newRequestTestClient(t, "DELETE", "/calendar/habits/%s/stop.json", nil, 204, "")
	if err := resumer.Habits().Resume(context.Background(), 7712); err != nil {
		t.Fatalf("unexpected error resuming: %v", err)
	}
}

// --- Contact CRUD and notes ---

func TestContactsService_Create(t *testing.T) {
	client := newFormTestClient(t, "POST", "/contacts", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("contact[name]") != "Jane Dawson" {
			t.Errorf("expected the contact name, got %q", values.Get("contact[name]"))
		}
		if values.Get("contact[email_address]") != "jane.dawson@example.com" {
			t.Errorf("expected the email address, got %q", values.Get("contact[email_address]"))
		}
		if aliases := values["contact[alias_email_addresses][]"]; len(aliases) != 1 {
			t.Errorf("expected one alias, got %v", aliases)
		}
	}, "/contacts/91824")

	id, err := client.Contacts().Create(context.Background(), ContactParams{
		Name:                "Jane Dawson",
		EmailAddress:        "jane.dawson@example.com",
		AliasEmailAddresses: []string{"j.dawson@example.org"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 91824 {
		t.Errorf("expected contact 91824, got %d", id)
	}
}

func TestContactsService_HideAndReveal(t *testing.T) {
	hider := newFormTestClient(t, "DELETE", "/contacts/%s", nil, "/contacts")
	if err := hider.Contacts().Hide(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error hiding: %v", err)
	}

	revealer := newFormTestClient(t, "POST", "/contacts/%s/reveal", nil, "/contacts/91824")
	if err := revealer.Contacts().Reveal(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error revealing: %v", err)
	}
}

func TestContactsService_Notes(t *testing.T) {
	setter := newFormTestClient(t, "PATCH", "/contacts/%s/note", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("contact[note]") != "Prefers a call to an email" {
			t.Errorf("expected the note, got %q", values.Get("contact[note]"))
		}
	}, "/contacts/91824/note")
	if err := setter.Contacts().SetNote(context.Background(), 91824, "Prefers a call to an email"); err != nil {
		t.Fatalf("unexpected error setting the note: %v", err)
	}

	deleter := newFormTestClient(t, "DELETE", "/contacts/%s/note", nil, "/contacts/91824/note")
	if err := deleter.Contacts().DeleteNote(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error deleting the note: %v", err)
	}
}

// --- Time track categories and exports ---

func TestTimeTracksService_Categories(t *testing.T) {
	page := `<div class="category">
		<a href="/calendar/time_tracks/categories/31/edit" class="action-group__action">Client work</a>
	</div>
	<div class="category">
		<a href="/calendar/time_tracks/categories/32/edit" class="action-group__action">Admin</a>
	</div>`

	client := newServiceTestClient(t, map[string]string{
		"/calendar/time_tracks/categories": page,
	})

	categories, err := client.TimeTracks().Categories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].ID != 31 || categories[0].Title != "Client work" {
		t.Errorf("expected Client work with id 31, got %+v", categories[0])
	}
}

func TestTimeTracksService_CategoryWrites(t *testing.T) {
	creator := newFormTestClient(t, "POST", "/calendar/time_tracks/categories", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("category[title]") != "Client work" {
			t.Errorf("expected the title, got %q", values.Get("category[title]"))
		}
	}, "/calendar/time_tracks/categories")
	if err := creator.TimeTracks().CreateCategory(context.Background(), "Client work"); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	updater := newFormTestClient(t, "PATCH", "/calendar/time_tracks/categories/%s", nil, "/calendar/time_tracks/categories/31")
	if err := updater.TimeTracks().UpdateCategory(context.Background(), 31, "Client work 2026"); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}

	deleter := newFormTestClient(t, "DELETE", "/calendar/time_tracks/categories/%s", nil, "/calendar/time_tracks/categories")
	if err := deleter.TimeTracks().DeleteCategory(context.Background(), 31); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
}

func TestTimeTracksService_Export(t *testing.T) {
	csv := "Start,End,Duration,Category,Notes\n2026-04-06 09:00,2026-04-06 11:00,2:00,Client work,Kitchen remodel call\n"

	client := newServiceTestClient(t, map[string]string{
		"/calendar/time_tracks/exports": csv,
	})

	data, err := client.TimeTracks().Export(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != csv {
		t.Errorf("expected the CSV back verbatim, got %q", string(data))
	}
}

// --- Clips ---

func TestClipsService_List(t *testing.T) {
	page := `<article id="clip_9182" class="clip-item">
		<a href="/topics/331" class="clip-item__name"><h3>Jane Dawson in Kitchen remodel</h3></a>
		<p class="clip-item__content"><span class="txt--highlight">The cabinets arrive on Tuesday</span></p>
	</article>`

	client := newServiceTestClient(t, map[string]string{"/clips": page})

	clips, err := client.Clips().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("expected one clip, got %d", len(clips))
	}
	if clips[0].ID != 9182 {
		t.Errorf("expected clip 9182, got %d", clips[0].ID)
	}
	if clips[0].Content != "The cabinets arrive on Tuesday" {
		t.Errorf("expected the clipped text, got %q", clips[0].Content)
	}
	if clips[0].Title != "Jane Dawson in Kitchen remodel" {
		t.Errorf("expected the clip title, got %q", clips[0].Title)
	}
}

func TestClipsService_CreateAndDelete(t *testing.T) {
	creator := newFormTestClient(t, "POST", "/clips", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("clip[entry_id]") != "5512" {
			t.Errorf("expected entry_id 5512, got %q", values.Get("clip[entry_id]"))
		}
		if values.Get("clip[content]") != "The cabinets arrive on Tuesday" {
			t.Errorf("expected the clipped text, got %q", values.Get("clip[content]"))
		}
	}, "/clips")
	if err := creator.Clips().Create(context.Background(), 5512, "The cabinets arrive on Tuesday"); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	deleter := newFormTestClient(t, "DELETE", "/clips/%s", nil, "/clips")
	if err := deleter.Clips().Delete(context.Background(), 9182); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
}

// --- Snippets ---

func TestSnippetsService_List(t *testing.T) {
	page := `<div class="action-group__item">
		<a href="/snippets/44/edit" class="action-group__action">Scheduling reply</a>
	</div>
	<div class="action-group__item">
		<a href="/snippets/45/edit" class="action-group__action">Invoice reminder</a>
	</div>`

	client := newServiceTestClient(t, map[string]string{"/snippets": page})

	snippets, err := client.Snippets().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 2 {
		t.Fatalf("expected 2 snippets, got %d", len(snippets))
	}
	if snippets[0].ID != 44 || snippets[0].Name != "Scheduling reply" {
		t.Errorf("expected Scheduling reply with id 44, got %+v", snippets[0])
	}
}

func TestSnippetsService_Writes(t *testing.T) {
	creator := newFormTestClient(t, "POST", "/snippets", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("snippet[name]") != "Scheduling reply" {
			t.Errorf("expected the snippet name, got %q", values.Get("snippet[name]"))
		}
	}, "/snippets")
	if err := creator.Snippets().Create(context.Background(), "Scheduling reply", "<div>Does Tuesday work?</div>"); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	updater := newFormTestClient(t, "PATCH", "/snippets/%s", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Has("snippet[content]") {
			t.Error("expected content to be left out when unset")
		}
	}, "/snippets")
	if err := updater.Snippets().Update(context.Background(), 44, "Scheduling", ""); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}

	deleter := newFormTestClient(t, "DELETE", "/snippets/%s", nil, "/snippets")
	if err := deleter.Snippets().Delete(context.Background(), 44); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
}

// --- Workflows ---

func TestWorkflowsService_List(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/autocompletable/accounts/%s/workflows": `[["8801","Hiring","Example Co"],["8802","Sales pipeline","Example Co"]]`,
	})

	workflows, err := client.Workflows().List(context.Background(), 77)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(workflows))
	}
	if workflows[0].ID != 8801 || workflows[0].Name != "Hiring" || workflows[0].AccountName != "Example Co" {
		t.Errorf("expected Hiring at Example Co with id 8801, got %+v", workflows[0])
	}
}

func TestWorkflowsService_Stages(t *testing.T) {
	page := `<section id="workflow_8801">
		<section class="workflow__stage" id="container_workflow_stage_5512">
			<h2 id="name_workflow_stage_5512"><div class="input"><span aria-hidden="true">Applied</span><span class="u-for-screen-reader">Applied stage</span></div></h2>
		</section>
		<section class="workflow__stage" id="container_workflow_stage_5513">
			<h2 id="name_workflow_stage_5513"><div class="input"><span aria-hidden="true">Interviewing</span><span class="u-for-screen-reader">Interviewing stage</span></div></h2>
		</section>
	</section>`

	client := newServiceTestClient(t, map[string]string{"/workflows/%s": page})

	stages, err := client.Workflows().Stages(context.Background(), 8801)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}
	if stages[0].ID != 5512 || stages[0].Name != "Applied" {
		t.Errorf("expected Applied with id 5512, got %+v", stages[0])
	}
	if stages[1].Name != "Interviewing" {
		t.Errorf("expected Interviewing, got %q", stages[1].Name)
	}
}

func TestWorkflowsService_Writes(t *testing.T) {
	creator := newFormTestClient(t, "POST", "/workflows", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("workflow[name]") != "Hiring" {
			t.Errorf("expected the workflow name, got %q", values.Get("workflow[name]"))
		}
		if values.Get("account_id") != "77" {
			t.Errorf("expected account_id 77, got %q", values.Get("account_id"))
		}
	}, "/workflows")
	if err := creator.Workflows().Create(context.Background(), "Hiring", 77); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	stager := newFormTestClient(t, "POST", "/workflows/%s/stages", nil, "/workflows/8801")
	if err := stager.Workflows().CreateStage(context.Background(), 8801); err != nil {
		t.Fatalf("unexpected error adding a stage: %v", err)
	}

	renamer := newFormTestClient(t, "PATCH", "/workflows/%s/stages/%s", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("workflow_stage[name]") != "Applied" {
			t.Errorf("expected the stage name, got %q", values.Get("workflow_stage[name]"))
		}
	}, "/workflows/8801/stages/5512")
	if err := renamer.Workflows().UpdateStage(context.Background(), 8801, 5512, "Applied"); err != nil {
		t.Fatalf("unexpected error renaming a stage: %v", err)
	}

	filer := newFormTestClient(t, "POST", "/topics/%s/workflows/%s/stagings", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("workflow_stage_id") != "5512" {
			t.Errorf("expected the stage id, got %q", values.Get("workflow_stage_id"))
		}
	}, "/topics/4471829")
	if err := filer.Workflows().StageTopic(context.Background(), 4471829, 8801, 5512); err != nil {
		t.Fatalf("unexpected error staging a topic: %v", err)
	}
}

// --- Publications ---

func TestPublicationsService_GetPublished(t *testing.T) {
	page := `<turbo-frame id="edit_topic_publication">
		<span class="copy-to-clipboard__text" data-copy-to-clipboard-target="copyable">https://public.hey.com/p/abc123</span>
		<input type="submit" value="Turn off the sharing link">
	</turbo-frame>`

	client := newServiceTestClient(t, map[string]string{"/topics/%s/publication/edit": page})

	publication, err := client.Publications().Get(context.Background(), 4471829)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !publication.Published {
		t.Error("expected the thread to read as published")
	}
	if publication.URL != "https://public.hey.com/p/abc123" {
		t.Errorf("expected the public link, got %q", publication.URL)
	}
}

func TestPublicationsService_GetUnpublished(t *testing.T) {
	page := `<turbo-frame id="edit_topic_publication">
		<button>Get a link to share</button>
	</turbo-frame>`

	client := newServiceTestClient(t, map[string]string{"/topics/%s/publication/edit": page})

	publication, err := client.Publications().Get(context.Background(), 4471829)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if publication.Published {
		t.Error("expected the thread to read as unpublished")
	}
}

func TestPublicationsService_Delete(t *testing.T) {
	client := newFormTestClient(t, "DELETE", "/topics/%s/publication", nil, "/topics/4471829")

	if err := client.Publications().Delete(context.Background(), 4471829); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- HEY World ---

func TestWorldService_Publish(t *testing.T) {
	client := newFormTestClient(t, "POST", "/messages", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("entry[addressed][directly]") != WorldAddress {
			t.Errorf("expected the world address, got %q", values.Get("entry[addressed][directly]"))
		}
		if values.Get("message[subject]") != "On writing less" {
			t.Errorf("expected the subject, got %q", values.Get("message[subject]"))
		}
		if values.Get("entry[status]") != "active" {
			t.Errorf("expected an active entry, got %q", values.Get("entry[status]"))
		}
	}, "/world/posts/a1b2c3d4")

	token, err := client.World().Publish(context.Background(), "On writing less", "<div>Fewer words, more meaning.</div>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "a1b2c3d4" {
		t.Errorf("expected the post token, got %q", token)
	}
}

func TestWorldService_PublishWhenTheMessageDidNotBecomeAPost(t *testing.T) {
	client := newFormTestClient(t, "POST", "/messages", nil, "/topics/4471829")

	_, err := client.World().Publish(context.Background(), "On writing less", "<div>Fewer words.</div>")
	if err == nil {
		t.Fatal("expected an error when the message did not land on a world post")
	}
}

func TestWorldService_UpdateAndDelete(t *testing.T) {
	updater := newFormTestClient(t, "PATCH", "/world/posts/%s", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("world_post[subject]") != "On writing less, revisited" {
			t.Errorf("expected the new subject, got %q", values.Get("world_post[subject]"))
		}
		if values.Has("world_post[content]") {
			t.Error("expected content to be left out when unset")
		}
	}, "/world/posts/a1b2c3d4")
	if err := updater.World().Update(context.Background(), "a1b2c3d4", "On writing less, revisited", ""); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}

	deleter := newFormTestClient(t, "DELETE", "/world/posts/%s", nil, "/world/lists/david@example.com")
	if err := deleter.World().Delete(context.Background(), "a1b2c3d4"); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
}

func TestWorldService_ExportSubscribers(t *testing.T) {
	csv := "email_address,subscribed_at\njane.dawson@example.com,2026-04-06 09:00:00 UTC\n"

	client := newServiceTestClient(t, map[string]string{"/world/lists/%s/export.csv": csv})

	data, err := client.World().ExportSubscribers(context.Background(), "david@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != csv {
		t.Errorf("expected the CSV back verbatim, got %q", string(data))
	}
}

func TestWorldService_ImportSubscribers(t *testing.T) {
	var contentType string
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Location", "/world/lists/david@example.com/imports/1")
		w.WriteHeader(302)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	err := client.World().ImportSubscribers(context.Background(), "david@example.com", "subscribers", []byte("email_address\njane.dawson@example.com\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("expected a multipart upload, got %q", contentType)
	}
	if !strings.Contains(string(body), `name="world_list_import[source]"; filename="subscribers.csv"`) {
		t.Errorf("expected the CSV part, got %q", string(body))
	}
}
