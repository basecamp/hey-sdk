package hey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, "My reply")
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
