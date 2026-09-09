package hey

import (
	"context"
	"encoding/json"
	"errors"
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

func TestIdentityService_UpdateFirstWeekDay(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PUT", "/calendar/identity/first_week_day.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			preference, _ := body["identity_preference"].(map[string]any)
			if preference["first_week_day"] != "monday" {
				t.Errorf("expected monday, got %v", preference["first_week_day"])
			}
		}, `{"first_week_day":1}`)

	stored, err := client.Identity().UpdateFirstWeekDay(context.Background(), time.Monday)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored != time.Monday {
		t.Errorf("stored day = %v, want Monday", stored)
	}
}

func TestIdentityService_UpdateTimeFormat(t *testing.T) {
	twentyFour := newMutationTestClientWithValidation(t, "PUT", "/identity/time_format.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if body["twenty_four_hour_time_format"] != true {
				t.Errorf("expected true, got %v", body["twenty_four_hour_time_format"])
			}
		}, `{"time_format":"twenty_four_hour"}`)

	stored, err := twentyFour.Identity().UpdateTimeFormat(context.Background(), TimeFormatTwentyFourHour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored != TimeFormatTwentyFourHour {
		t.Errorf("stored format = %v, want twenty_four_hour", stored)
	}

	twelve := newMutationTestClientWithValidation(t, "PUT", "/identity/time_format.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if body["twenty_four_hour_time_format"] != false {
				t.Errorf("expected false, got %v", body["twenty_four_hour_time_format"])
			}
		}, `{"time_format":"twelve_hour"}`)

	stored, err = twelve.Identity().UpdateTimeFormat(context.Background(), TimeFormatTwelveHour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored != TimeFormatTwelveHour {
		t.Errorf("stored format = %v, want twelve_hour", stored)
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
			if msg["subject"] != "Re: A thread" {
				t.Errorf("expected subject 'Re: A thread', got %v", msg["subject"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, 0, "Re: A thread", "My reply", []string{"test@example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_SendsTheChosenActingSender(t *testing.T) {
	// The identity a reply goes out as: HEY resolves it from the thread (a shared
	// support address, an extension) and hands it back as NewReply's sender. The
	// caller's choice must reach the wire untouched — not the account default.
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if got, _ := body["acting_sender_id"].(float64); got != 4242 {
				t.Errorf("acting_sender_id = %v, want the chosen sender 4242", body["acting_sender_id"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, 4242, "Re: A thread", "My reply", []string{"test@example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_PassesNonZeroSendersThroughUntouched(t *testing.T) {
	// Only zero means "the account default". Anything else — a stale or even
	// negative id — goes to the server as given; an invalid sender is the
	// server's to reject, not this SDK's to silently rewrite.
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if got, _ := body["acting_sender_id"].(float64); got != -7 {
				t.Errorf("acting_sender_id = %v, want -7 passed through", body["acting_sender_id"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, -7, "Re: A thread", "My reply", []string{"test@example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_ZeroActingSenderFallsBackToDefault(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			if got, _ := body["acting_sender_id"].(float64); got != 42 {
				t.Errorf("acting_sender_id = %v, want the account default 42", body["acting_sender_id"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, 0, "Re: A thread", "My reply", []string{"test@example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_EmptySubjectStaysOffTheWire(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			msg, _ := body["message"].(map[string]any)
			if _, present := msg["subject"]; present {
				t.Errorf("an empty subject must be omitted, got %v", msg["subject"])
			}
		},
		`{"notice":"sent"}`,
	)

	err := client.Entries().CreateReply(context.Background(), 10, 0, "", "My reply", []string{"test@example.com"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_CreateReply_RequiresRecipients(t *testing.T) {
	// HEY saves an unaddressed reply as a draft instead of delivering it, so the
	// SDK refuses before making a request.
	client := newMutationTestClientWithValidation(t, "POST", "/entries/%s/replies.json",
		func(t *testing.T, _ map[string]any) { t.Helper(); t.Error("no request should be sent") },
		`{"notice":"sent"}`,
	)
	err := client.Entries().CreateReply(context.Background(), 10, 0, "Re: hello", "hello", nil, nil, nil)
	if e := AsError(err); e == nil || e.Code != CodeUsage {
		t.Fatalf("expected a usage error, got %#v", err)
	}
	err = client.Messages().Create(context.Background(), "s", "b", nil, nil, nil)
	if e := AsError(err); e == nil || e.Code != CodeUsage {
		t.Fatalf("expected a usage error for a message with no recipients, got %#v", err)
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

	err := client.Entries().CreateReply(context.Background(), 10, 0, "Re: hi", "hi", []string{"a@x.com"}, nil, []string{"b@x.com"})
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

func TestContactsService_ThreadsPage(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<http://`+r.Host+`/contacts/88.json?page=b2xkZXI>; rel="next"`)
		}
		_, _ = w.Write([]byte(`{"id":88,"name":"GitHub","entries_title":"All threads with GitHub","postings":[{"id":501,"kind":"topic","name":"Deploy failed on main","app_url":"https://app.hey.com/topics/9001"}]}`))
	}))
	t.Cleanup(server.Close)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	page, err := client.Contacts().ThreadsPage(context.Background(), 88, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Contact.EntriesTitle != "All threads with GitHub" || len(page.Contact.Postings) != 1 {
		t.Errorf("page = %+v", page.Contact)
	}
	if page.NextPage != "b2xkZXI" {
		t.Errorf("NextPage = %q, want the Link header's cursor", page.NextPage)
	}

	next, err := client.Contacts().ThreadsPage(context.Background(), 88, page.NextPage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.NextPage != "" {
		t.Errorf("NextPage = %q, want none on the last page", next.NextPage)
	}
	if len(queries) != 2 || queries[0] != "" || queries[1] != "page=b2xkZXI" {
		t.Errorf("queries = %q, want the cursor passed through", queries)
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

func TestCalendarsService_GetRecordingsPage(t *testing.T) {
	var gotPage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPage = r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/calendars/1/recordings.json?page=next-cursor>; rel="next"`)
		_, _ = io.WriteString(w, `{"Calendar::Event":[{"id":1}]}`)
	}))
	t.Cleanup(server.Close)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	cursor := "current-cursor"
	page, err := client.Calendars().GetRecordingsPage(context.Background(), 1, &generated.GetCalendarRecordingsParams{Page: &cursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPage != cursor {
		t.Errorf("page = %q, want %q", gotPage, cursor)
	}
	if page.Recordings == nil || len((*page.Recordings)["Calendar::Event"]) != 1 {
		t.Errorf("recordings = %+v, want the event page", page.Recordings)
	}
	if page.NextPage != "next-cursor" {
		t.Errorf("next page = %q, want the Link header's cursor", page.NextPage)
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
		`{"id":1,"type":"Calendar::Todo"}`,
	)

	result, err := client.CalendarTodos().Create(context.Background(), "Do something", "2026-03-13")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestCalendarTodosService_Update(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PATCH", "/calendar/todos/1.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			todo, ok := body["calendar_todo"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_todo wrapper")
			}
			if todo["title"] != "Renew the passport" {
				t.Errorf("expected title 'Renew the passport', got %v", todo["title"])
			}
			// A rename says nothing about the day or the focus, so the server keeps them.
			if _, ok := todo["starts_at"]; ok {
				t.Errorf("a rename sent starts_at: %v", todo)
			}
			if _, ok := todo["focused"]; ok {
				t.Errorf("a rename sent focused: %v", todo)
			}
		},
		`{"id":1,"type":"Calendar::Todo","title":"Renew the passport"}`,
	)

	result, err := client.CalendarTodos().Update(context.Background(), 1, TodoChanges{Title: "Renew the passport"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// starts_at goes out as the bare date it was given: sent as an RFC 3339 instant at UTC
// midnight it can land on the day before once the server reads it in the user's zone.
func TestCalendarTodosService_UpdateSendsABareDate(t *testing.T) {
	focused := true
	client := newMutationTestClientWithValidation(t, "PATCH", "/calendar/todos/1.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			todo, ok := body["calendar_todo"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_todo wrapper")
			}
			if todo["starts_at"] != "2026-03-13" {
				t.Errorf("starts_at = %v, want the bare date", todo["starts_at"])
			}
			if todo["focused"] != true {
				t.Errorf("focused = %v", todo["focused"])
			}
		},
		`{"id":1,"type":"Calendar::Todo"}`,
	)

	if _, err := client.CalendarTodos().Update(context.Background(), 1, TodoChanges{
		StartsAt: "2026-03-13",
		Focused:  &focused,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCalendarTodosService_UpdateRefusesAnEmptyChange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("an update that changes nothing reached the server: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"})
	if _, err := client.CalendarTodos().Update(context.Background(), 1, TodoChanges{}); err == nil {
		t.Fatal("an update that changes nothing was accepted")
	}
}

func TestCalendarTodosService_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1,"type":"Calendar::Todo"}`))
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
		w.Write([]byte(`{"id":1,"type":"Calendar::Todo"}`))
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

func TestTimeTracksService_ListPage(t *testing.T) {
	var gotPage, gotCategory string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/calendar/time_tracks.json" {
			t.Errorf("expected GET /calendar/time_tracks.json, got %s %s", r.Method, r.URL.Path)
		}
		gotPage = r.URL.Query().Get("page")
		gotCategory = r.URL.Query().Get("category_id")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/calendar/time_tracks.json?page=eyJwYWdlIjozfQ&category_id=31>; rel="next"`)
		_, _ = w.Write([]byte(`{"time_tracks":[
			{"id":701,"type":"Calendar::TimeTrack","title":"Time Track",
			 "starts_at":"2026-08-19T09:00:00Z","ends_at":"2026-08-19T11:30:00Z",
			 "completed_at":"2026-08-19T11:30:00Z",
			 "notes":"Reviewed the migration plan","category":"Client work"}],
			"categories":[{"id":31,"title":"Client work"},{"id":32,"title":"Internal"}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	cursor, categoryID := "eyJwYWdlIjoyfQ", int64(31)
	page, err := client.TimeTracks().ListPage(context.Background(), &generated.ListTimeTracksParams{
		Page:       &cursor,
		CategoryId: &categoryID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPage != "eyJwYWdlIjoyfQ" {
		t.Errorf("expected the cursor to be passed through, got %q", gotPage)
	}
	if gotCategory != "31" {
		t.Errorf("expected the category filter to be passed through, got %q", gotCategory)
	}
	if len(page.TimeTracks) != 1 || page.TimeTracks[0].Id != 701 {
		t.Fatalf("expected time track 701, got %+v", page.TimeTracks)
	}
	// A track's category is the title as a plain string, not an object.
	if got := page.TimeTracks[0].Category; got != "Client work" {
		t.Errorf("category = %q, want \"Client work\"", got)
	}
	if got := page.TimeTracks[0].Notes; got != "Reviewed the migration plan" {
		t.Errorf("notes = %q, want the notes the track was filed with", got)
	}
	// The categories ride along, so filtering needs no second read.
	if len(page.Categories) != 2 || page.Categories[0].Title != "Client work" || page.Categories[1].Title != "Internal" {
		t.Errorf("expected both categories from the same read, got %+v", page.Categories)
	}
	if page.NextPage != "eyJwYWdlIjozfQ" {
		t.Errorf("expected the cursor for the next page, got %q", page.NextPage)
	}
}

// The last page carries no Link header, which is how a caller walking the list is told it
// has reached the end. A nil Link is the end of the list, not an error.
func TestTimeTracksService_ListPageOnLastPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("page") || r.URL.Query().Has("category_id") {
			t.Errorf("expected no empty query parameters on the first unfiltered read, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"time_tracks":[{"id":701,"type":"Calendar::TimeTrack","category":null}],
			"categories":[]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.TimeTracks().ListPage(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.TimeTracks) != 1 {
		t.Fatalf("expected one time track, got %+v", page.TimeTracks)
	}
	// An uncategorized track serves category: null, which is the empty string here.
	if got := page.TimeTracks[0].Category; got != "" {
		t.Errorf("category = %q, want empty for an uncategorized track", got)
	}
	if page.NextPage != "" {
		t.Errorf("expected no cursor past the last page, got %q", page.NextPage)
	}
}

// A cursor on an empty page is still the end of the list: the caller stops on the empty
// page rather than following a cursor into nothing.
func TestTimeTracksService_ListPageEmptyWithACursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/calendar/time_tracks.json?page=eyJwYWdlIjo5fQ>; rel="next"`)
		_, _ = w.Write([]byte(`{"time_tracks":[],"categories":[{"id":31,"title":"Client work"}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	page, err := client.TimeTracks().ListPage(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.TimeTracks) != 0 {
		t.Fatalf("expected an empty page, got %+v", page.TimeTracks)
	}
	// The cursor is reported as served; ending the list on an empty page is the caller's
	// move, and it must be able to see both facts.
	if page.NextPage != "eyJwYWdlIjo5fQ" {
		t.Errorf("expected the served cursor to be reported, got %q", page.NextPage)
	}
}

// List answers the payload and drops the cursor.
func TestTimeTracksService_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<http://`+r.Host+`/calendar/time_tracks.json?page=eyJwYWdlIjoyfQ>; rel="next"`)
		_, _ = w.Write([]byte(`{"time_tracks":[{"id":701,"type":"Calendar::TimeTrack"}],
			"categories":[{"id":31,"title":"Client work"}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	tracked, err := client.TimeTracks().List(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tracked.TimeTracks) != 1 || tracked.TimeTracks[0].Id != 701 {
		t.Errorf("expected time track 701, got %+v", tracked.TimeTracks)
	}
	if len(tracked.Categories) != 1 || tracked.Categories[0].Id != 31 {
		t.Errorf("expected the categories alongside the tracks, got %+v", tracked.Categories)
	}
}

// A category_id the calendar has no category for is a 404, not an empty list.
func TestTimeTracksService_ListPage_UnknownCategory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	missing := int64(9999)
	_, err := client.TimeTracks().ListPage(context.Background(), &generated.ListTimeTracksParams{CategoryId: &missing})
	if err == nil {
		t.Fatal("expected an error for a category the calendar does not have")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound || apiErr.Code != CodeNotFound {
		t.Errorf("expected a 404 *Error, got %v", err)
	}
}

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
	// The server ignores any body on start, so the SDK sends none.
	client := newMutationTestClientWithValidation(t, "POST", "/calendar/ongoing_time_track.json",
		nil,
		`{"id":1,"type":"TimeTrack"}`,
	)

	result, err := client.TimeTracks().Start(context.Background())
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
			for _, k := range []string{"starts_at", "category", "category_title", "notes", "title"} {
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

// Stopping and filing is one PUT, and the category rides on category_title — the
// category field the payload also carries is one the server throws away.
func TestTimeTracksService_StopAndFile(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PUT", "/calendar/time_tracks/%s.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			tt, ok := body["calendar_time_track"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_time_track wrapper")
			}
			if got := tt["category_title"]; got != "Client work" {
				t.Errorf("category_title = %v, want \"Client work\"", got)
			}
			if _, ok := tt["ends_at"]; !ok {
				t.Error("missing ends_at: filing a track is stopping it")
			}
			for _, k := range []string{"starts_at", "category", "notes", "title"} {
				if v, present := tt[k]; present {
					t.Errorf("body must only carry ends_at and category_title; got %s=%v", k, v)
				}
			}
		},
		`{"id":1,"type":"TimeTrack"}`,
	)

	if err := client.TimeTracks().StopAndFile(context.Background(), 1, "Client work"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Update carries a category the same way, and does not smuggle in the fields it wasn't given.
func TestTimeTracksService_Update_FilesUnderACategory(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PUT", "/calendar/time_tracks/%s.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			tt, ok := body["calendar_time_track"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_time_track wrapper")
			}
			for field, want := range map[string]string{
				"category_title": "Client work",
				"notes":          "Reviewed the migration plan",
			} {
				if got := tt[field]; got != want {
					t.Errorf("%s = %v, want %q", field, got, want)
				}
			}
			for _, k := range []string{"starts_at", "ends_at", "category", "title"} {
				if v, present := tt[k]; present {
					t.Errorf("body must not carry %s; got %v", k, v)
				}
			}
		},
		`{"id":1,"type":"TimeTrack"}`,
	)

	_, err := client.TimeTracks().Update(context.Background(), 1, generated.UpdateTimeTrackJSONRequestBody{
		CalendarTimeTrack: generated.UpdateTimeTrackPayload{
			CategoryTitle: "Client work",
			Notes:         "Reviewed the migration plan",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// StopAndFile with no category is Stop, and must not send a blank category_title:
// haystack ignores a blank one, but a caller reading the wire should not see a field
// that does nothing.
func TestTimeTracksService_StopAndFile_WithoutACategory(t *testing.T) {
	client := newMutationTestClientWithValidation(t, "PUT", "/calendar/time_tracks/%s.json",
		func(t *testing.T, body map[string]any) {
			t.Helper()
			tt, ok := body["calendar_time_track"].(map[string]any)
			if !ok {
				t.Fatal("missing calendar_time_track wrapper")
			}
			if v, present := tt["category_title"]; present {
				t.Errorf("category_title should be omitted, got %v", v)
			}
		},
		`{"id":1,"type":"TimeTrack"}`,
	)

	if err := client.TimeTracks().StopAndFile(context.Background(), 1, ""); err != nil {
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
	client := newMutationTestClientWithValidation(t, "PATCH", "/calendar/days/%s/journal_entry.json",
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
		`{"id":1,"type":"Calendar::JournalEntry","content":"Today was great","content_html":"<div class=\"trix-content\">Today was great</div>"}`,
	)

	recording, err := client.Journal().Update(context.Background(), "2026-03-09", "Today was great")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording == nil || recording.Content != "Today was great" || recording.ContentHtml == "" {
		t.Fatalf("expected the written entry back, got %#v", recording)
	}
}

func TestJournalService_UpdateEmptyContentRemovesEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	recording, err := client.Journal().Update(context.Background(), "2026-03-09", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording != nil {
		t.Errorf("expected nil when the entry was removed, got %#v", recording)
	}
}

func TestJournalService_GetContentPrefersHTMLAndNoLongerScrapes(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/calendar/days/2026-03-09/journal_entry.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":1,"type":"Calendar::JournalEntry","content":"plain","content_html":"<div class=\"trix-content\"><strong>rich</strong></div>"}`))
		case "/calendar/days/2026-03-10/journal_entry.json":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	content, err := client.Journal().GetContent(context.Background(), "2026-03-09")
	if err != nil || !strings.Contains(content, "<strong>rich</strong>") {
		t.Fatalf("expected the HTML content, got %q err=%v", content, err)
	}
	content, err = client.Journal().GetContent(context.Background(), "2026-03-10")
	if err != nil || content != "" {
		t.Fatalf("expected empty content for a day without an entry, got %q err=%v", content, err)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "/edit") {
			t.Errorf("edit page must not be fetched any more, got %s", p)
		}
	}
}

// --- Search ---

func TestSearchService_Search(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/advanced_search.json" {
			w.WriteHeader(404)
			return
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"matches":[{"topic":{"id":331,"name":"Kitchen remodel"},"posting_id":4471829,"entries":[{"id":5512,"summary":"The cabinets arrive on Tuesday","kind":"message"}]}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	result, err := client.Search().Search(context.Background(), SearchParams{Query: "cabinets", From: "Jane", Page: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("expected one match, got %d", len(result.Matches))
	}
	m := result.Matches[0]
	if m.Topic.Id != 331 || m.Topic.Name != "Kitchen remodel" || m.PostingId != 4471829 {
		t.Errorf("unexpected match %+v", m)
	}
	if len(m.Entries) != 1 || m.Entries[0].Summary != "The cabinets arrive on Tuesday" {
		t.Errorf("unexpected entries %+v", m.Entries)
	}
	for _, want := range []string{"q=cabinets", "page=2", "refine%5Bfrom%5D=Jane"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q should contain %q", gotQuery, want)
		}
	}
	if strings.Contains(gotQuery, "refine%5Bto%5D") {
		t.Errorf("unset refinements must not be sent, got %q", gotQuery)
	}
}

func TestSearchService_SearchPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "3" {
			_, _ = w.Write([]byte(`{"matches":[{"topic":{"id":331,"name":"Kitchen remodel"},"posting_id":4471829}]}`))
			return
		}
		w.Header().Set("Link", `<http://`+r.Host+`/advanced_search.json?q=cabinets&page=3>; rel="next"`)
		_, _ = w.Write([]byte(`{"matches":[{"topic":{"id":332,"name":"Cabinet estimate"},"posting_id":4471830}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	results, err := client.Search().SearchPage(context.Background(), SearchParams{Query: "cabinets", Page: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results.NextPage != 3 {
		t.Errorf("next page = %d, want 3", results.NextPage)
	}

	// The last page carries no Link header, which is how a caller walking the results is
	// told to stop asking.
	last, err := client.Search().SearchPage(context.Background(), SearchParams{Query: "cabinets", Page: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if last.NextPage != 0 || len(last.Result.Matches) != 1 {
		t.Errorf("last page = %+v", last)
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

		StartTimeZone: "America/New_York",
		EndTimeZone:   "America/New_York",
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

		StartTimeZone: "America/New_York",
		EndTimeZone:   "America/New_York",
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

// An all-day update carries dates without clock times.
func TestCalendarEventsService_Update_AllDayOmitsClockTimes(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			assertFormFields(t, values, map[string]string{
				"calendar_event[all_day]": "1",
			},
				"calendar_event[starts_at_time]",
				"calendar_event[ends_at_time]",
			)
		},
		200, `{"id": 99, "title": "Sarah's birthday", "type": "Calendar::Event", "all_day": true}`,
	)

	title := "Sarah's birthday"
	startsAt := "2026-09-02"
	endsAt := "2026-09-02"
	allDay := true
	empty := ""
	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Title:     &title,
		StartsAt:  &startsAt,
		EndsAt:    &endsAt,
		AllDay:    &allDay,
		StartTime: &empty,
		EndTime:   &empty,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// An event can start in one zone and finish in another, which is what a flight is.
func TestCalendarEventsService_Update_WithAZonePerEnd(t *testing.T) {
	departs, arrives := "Europe/Zagreb", "America/New_York"
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			for field, want := range map[string]string{
				"calendar_event[set_time_zone]":            "1",
				"calendar_event[starts_at_time_zone_name]": "Europe/Zagreb",
				"calendar_event[ends_at_time_zone_name]":   "America/New_York",
			} {
				if got := values.Get(field); got != want {
					t.Errorf("%s = %q, want %q", field, got, want)
				}
			}
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		StartTimeZone: &departs,
		EndTimeZone:   &arrives,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The deprecated single TimeZone still names both ends, and now carries the flag that makes
// HEY read it — which it never did before.
func TestCalendarEventsService_Create_WithTheDeprecatedTimeZone(t *testing.T) {
	client := newFormJSONTestClient(t, "POST", "/calendar/events.json",
		func(t *testing.T, values url.Values) {
			t.Helper()
			for field, want := range map[string]string{
				"calendar_event[set_time_zone]":            "1",
				"calendar_event[starts_at_time_zone_name]": "America/New_York",
				"calendar_event[ends_at_time_zone_name]":   "America/New_York",
			} {
				if got := values.Get(field); got != want {
					t.Errorf("%s = %q, want %q", field, got, want)
				}
			}
		},
		201, calendarEventJSON,
	)

	_, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
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
}

// Empty zones say the times are UTC, and put the event back to having no zones of its own.
func TestCalendarEventsService_Update_ClearingTheZones(t *testing.T) {
	none := ""
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if got := values.Get("calendar_event[set_time_zone]"); got != "0" {
				t.Errorf("set_time_zone = %q, want 0", got)
			}
			if got := values.Get("calendar_event[starts_at_time_zone_name]"); got != "" {
				t.Errorf("a zone was named anyway: %q", got)
			}
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		StartTimeZone: &none,
		EndTimeZone:   &none,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

// newNoContentTestClient answers a JSON delete with 204 and records what it was asked for.
func newNoContentTestClient(t *testing.T, wantMethod, wantPath string) (*Client, *url.Values) {
	t.Helper()
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != wantMethod {
			t.Errorf("expected %s, got %s", wantMethod, r.Method)
		}
		if r.URL.Path != wantPath {
			t.Errorf("expected path %s, got %s", wantPath, r.URL.Path)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("expected Accept: application/json, got %q", accept)
		}
		gotQuery = r.URL.Query()
		w.WriteHeader(204)
	}))
	t.Cleanup(server.Close)
	return NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0)), &gotQuery
}

// The delete asks for the JSON representation HEY serves it in — the .json path with a JSON
// Accept — rather than the redirect its web form answers with.
func TestCalendarEventsService_Delete(t *testing.T) {
	client, _ := newNoContentTestClient(t, "DELETE", "/calendar/events/99.json")

	err := client.CalendarEvents().Delete(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOccurrenceID(t *testing.T) {
	occurrence, err := ParseOccurrenceID("153688907_2026-08-21")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if occurrence.EventID != 153688907 {
		t.Errorf("EventID = %d, want 153688907", occurrence.EventID)
	}
	if got := occurrence.Date.Format("2006-01-02"); got != "2026-08-21" {
		t.Errorf("Date = %s, want 2026-08-21", got)
	}
	if got := occurrence.String(); got != "153688907_2026-08-21" {
		t.Errorf("String() = %q, want the occurrence id back", got)
	}
	if got := occurrence.DateParam(); got != "2026-08-21" {
		t.Errorf("DateParam() = %q, want 2026-08-21", got)
	}
}

func TestParseOccurrenceID_Malformed(t *testing.T) {
	for _, occurrenceID := range []string{
		"",
		"153688907",
		"153688907_",
		"153688907_2026-08",
		"153688907_2026-13-40",
		"153688907_20260821",
		"_2026-08-21",
		"0_2026-08-21",
		"summer_2026-08-21",
		"153688907_2026-08-21_extra",
	} {
		if _, err := ParseOccurrenceID(occurrenceID); err == nil {
			t.Errorf("%q was accepted as an occurrence id", occurrenceID)
		}
	}
}

func TestCalendarEventsService_UpdateOccurrence(t *testing.T) {
	newTitle := "Summer Friday"
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/153688907/occurrences/2026-08-21.json",
		func(t *testing.T, values url.Values) {
			t.Helper()
			for field, want := range map[string]string{
				"calendar_event[summary]": "Summer Friday",
				"apply_to_future":         "0",
				"repeat_frequency":        "custom",
			} {
				if got := values.Get(field); got != want {
					t.Errorf("%s = %q, want %q", field, got, want)
				}
			}
		},
		200, `{"id": 153688908, "title": "Summer Friday", "type": "Calendar::Event"}`,
	)

	occurrence, err := ParseOccurrenceID("153688907_2026-08-21")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	recording, err := client.CalendarEvents().UpdateOccurrence(context.Background(), occurrence, OccurrenceScopeThisEvent,
		UpdateCalendarEventOccurrenceParams{
			UpdateCalendarEventParams: UpdateCalendarEventParams{Title: &newTitle},
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recording.Id != 153688908 {
		t.Errorf("expected the realized occurrence, got %d", recording.Id)
	}
}

// Naming no frequency has to mean "leave the series alone", because HEY reads a silent update as
// "stop repeating" and cancels every other occurrence.
func TestCalendarEventsService_UpdateOccurrence_KeepsTheSeriesRepeating(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s/occurrences/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			if got := values.Get("repeat_frequency"); got != "custom" {
				t.Errorf("repeat_frequency = %q, want custom", got)
			}
		},
		200, `{"id": 153688908, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().UpdateOccurrence(context.Background(),
		EventOccurrence{EventID: 153688907, Date: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)},
		OccurrenceScopeThisEvent, UpdateCalendarEventOccurrenceParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCalendarEventsService_UpdateOccurrence_ThisAndFollowing(t *testing.T) {
	startTime := "10:30"
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s/occurrences/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			for field, want := range map[string]string{
				"apply_to_future":                "1",
				"repeat_frequency":               "every_week",
				"calendar_event[starts_at_time]": "10:30:00",
			} {
				if got := values.Get(field); got != want {
					t.Errorf("%s = %q, want %q", field, got, want)
				}
			}
		},
		200, `{"id": 153688908, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().UpdateOccurrence(context.Background(),
		EventOccurrence{EventID: 153688907, Date: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)},
		OccurrenceScopeThisAndFollowing, UpdateCalendarEventOccurrenceParams{
			UpdateCalendarEventParams: UpdateCalendarEventParams{
				StartTime: &startTime,
				Repeat:    &RepeatParams{Frequency: RepeatEveryWeek},
			},
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// assertFormFields checks the fields a write is expected to carry, and that the ones it is
// expected to leave out are absent — for the content and countdown parameters an absent key and
// an empty one mean different things to HEY.
func assertFormFields(t *testing.T, values url.Values, want map[string]string, absent ...string) {
	t.Helper()
	for field, wanted := range want {
		if got := values.Get(field); got != wanted {
			t.Errorf("%s = %q, want %q", field, got, wanted)
		}
	}
	for _, field := range absent {
		if values.Has(field) {
			t.Errorf("%s was sent as %q, want it left out", field, values.Get(field))
		}
	}
}

func TestCalendarEventsService_Create_WithContentAndGuests(t *testing.T) {
	client := newFormJSONTestClient(t, "POST", "/calendar/events.json",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, map[string]string{
				"calendar_event[description]":                     "<div>Bring the <strong>roadmap</strong>.</div>",
				"calendar_event[location]":                        "Meeting Room 2",
				"calendar_event[url]":                             "https://meet.google.com/abc-defg-hij",
				"calendar_event[entry_id]":                        "884213",
				"calendar_event[highlighted]":                     "1",
				"calendar_event[highlight_id]":                    "",
				"countdown_interval_duration_value":               "2",
				"countdown_interval_duration_unit":                "604800",
				"repeat_frequency":                                "every_other_week",
				"calendar_recurrence_schedule[recurs_until_type]": "date",
				"calendar_recurrence_schedule[recurs_until_date]": "2026-12-18",
			}, "calendar_recurrence_schedule[recurs_count]")

			guests := values["calendar_event[attendance_email_addresses][]"]
			if len(guests) != 2 || guests[0] != "marta.kowalska@example.com" || guests[1] != "yusuf.demir@example.org" {
				t.Errorf("guest list = %v", guests)
			}
		},
		201, calendarEventJSON,
	)

	circled := true
	_, err := client.CalendarEvents().Create(context.Background(), CreateCalendarEventParams{
		CalendarID: 1,
		Title:      "Quarterly roadmap review",
		StartsAt:   "2026-09-10",
		AllDay:     true,
		Content: EventContentParams{
			Notes:    "<div>Bring the <strong>roadmap</strong>.</div>",
			Location: "Meeting Room 2",
			Link:     "https://meet.google.com/abc-defg-hij",
			EntryID:  884213,
		},
		Attendees:   []string{"marta.kowalska@example.com", "yusuf.demir@example.org"},
		Highlighted: &circled,
		Countdown:   CountdownParams{Value: 2, Unit: CountdownUnitWeeks},
		Repeat: &RepeatParams{
			Frequency: RepeatEveryOtherWeek,
			Until:     RepeatUntilDate,
			UntilDate: "2026-12-18",
			Count:     9,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// HEY defaults all four content fields to nothing on every write, so an update that only means to
// move an event still has to say what its notes are. Sending them empty is the SDK being honest
// about that rather than pretending the fields were left alone.
func TestCalendarEventsService_Update_SendsTheWholeContentEveryTime(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, map[string]string{
				"calendar_event[starts_at]":   "2026-09-11",
				"calendar_event[description]": "",
				"calendar_event[location]":    "",
				"calendar_event[url]":         "",
				"calendar_event[entry_id]":    "",
			})
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	startsAt := "2026-09-11"
	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		StartsAt: &startsAt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Turning the circle off needs the empty highlight_id: without it HEY reads highlighted=0 as a
// request to build a highlight, which turns the circle on. It goes out with the flag either way.
func TestCalendarEventsService_Update_Highlighted(t *testing.T) {
	for circled, wantFlag := range map[bool]string{true: "1", false: "0"} {
		client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
			func(t *testing.T, values url.Values) {
				t.Helper()
				assertFormFields(t, values, map[string]string{
					"calendar_event[highlighted]":  wantFlag,
					"calendar_event[highlight_id]": "",
				})
			},
			200, `{"id": 99, "type": "Calendar::Event"}`,
		)

		_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
			Highlighted: &circled,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

// Nothing is sent for the circle when nobody asked about it, because HEY only reads the flag when
// it is submitted.
func TestCalendarEventsService_Update_LeavesTheCircleAloneUnasked(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, nil,
				"calendar_event[highlighted]", "calendar_event[highlight_id]")
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The zero countdown sends no countdown at all, which is how HEY is told to delete it. There is no
// third state, so this is the same request as one that never had a countdown in mind.
func TestCalendarEventsService_Update_ZeroCountdownSendsNothing(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, nil,
				"countdown_interval_duration_value", "countdown_interval_duration_unit")
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A countdown with no unit named is counted in days, which is the web app's own default.
func TestCalendarEventsService_Update_CountdownDefaultsToDays(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, map[string]string{
				"countdown_interval_duration_value": "10",
				"countdown_interval_duration_unit":  "86400",
			})
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Countdown: CountdownParams{Value: 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A nil guest list leaves the roster alone; an empty one clears it, and needs a blank value on
// the wire to say so, since a form cannot carry an empty array.
func TestCalendarEventsService_Update_Attendees(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, nil, "calendar_event[attendance_email_addresses][]")
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	client = newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			guests := values["calendar_event[attendance_email_addresses][]"]
			if len(guests) != 1 || guests[0] != "" {
				t.Errorf("guest list = %v, want a single blank so HEY sees the key", guests)
			}
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err = client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Attendees: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A count-limited recurrence sends the count and no until-date, since HEY reads only the one
// matching the type.
func TestCalendarEventsService_Update_RepeatUntilCount(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, map[string]string{
				"repeat_frequency": "every_weekday",
				"calendar_recurrence_schedule[recurs_until_type]": "count",
				"calendar_recurrence_schedule[recurs_count]":      "12",
			}, "calendar_recurrence_schedule[recurs_until_date]")
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{
		Repeat: &RepeatParams{Frequency: RepeatEveryWeekday, Until: RepeatUntilCount, Count: 12},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A whole-event update that says nothing about the recurrence leaves it alone — the one place
// where the occurrence update reads the same silence differently.
func TestCalendarEventsService_Update_WithoutRepeatLeavesTheScheduleAlone(t *testing.T) {
	client := newFormJSONTestClient(t, "PATCH", "/calendar/events/%s",
		func(t *testing.T, values url.Values) {
			t.Helper()
			assertFormFields(t, values, nil, "repeat_frequency")
		},
		200, `{"id": 99, "type": "Calendar::Event"}`,
	)

	_, err := client.CalendarEvents().Update(context.Background(), 99, UpdateCalendarEventParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A delete carries no body, so the scope rides in the query string, as it does in HEY's own
// occurrence links. Both scopes are sent explicitly: the narrower one is the zero value, and
// leaving it off would rely on the server's default instead of stating it.
func TestCalendarEventsService_DeleteOccurrence(t *testing.T) {
	for scope, wantApplyToFuture := range map[OccurrenceScope]string{
		OccurrenceScopeThisEvent:        "false",
		OccurrenceScopeThisAndFollowing: "true",
	} {
		client, gotQuery := newNoContentTestClient(t, "DELETE", "/calendar/events/153688907/occurrences/2026-08-21.json")

		err := client.CalendarEvents().DeleteOccurrence(context.Background(),
			EventOccurrence{EventID: 153688907, Date: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)}, scope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := gotQuery.Get("apply_to_future"); got != wantApplyToFuture {
			t.Errorf("apply_to_future = %q, want %q for %s", got, wantApplyToFuture, scope)
		}
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
	client, _ := newNoContentTestClient(t, "DELETE", "/accounts/1/domains/extenzions/10.json")

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

func TestBoxesService_GetPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/boxes/5.json" {
			t.Errorf("path = %q, want /boxes/5.json", r.URL.Path)
		}
		if page := r.URL.Query().Get("page"); page != "current-cursor" {
			t.Errorf("page = %q, want current-cursor", page)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "214")
		w.Header().Set("Link", `<http://`+r.Host+`/boxes/5.json?page=next-cursor>; rel="next"`)
		_, _ = io.WriteString(w, `{"id":5,"name":"Imbox","postings":[{"id":1,"kind":"topic"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	cursor := "current-cursor"
	page, err := client.Boxes().GetPage(context.Background(), 5, &generated.GetBoxParams{Page: &cursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Box == nil || page.Box.Name != "Imbox" || len(page.Box.Postings) != 1 || page.TotalCount != 214 || page.NextPage != "next-cursor" {
		t.Errorf("unexpected box page: %+v", page)
	}
}

// The last page carries no Link header, which is how a caller walking a box is told to
// stop asking for more.
func TestBoxesService_GetPageOnLastPage(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/boxes/%s": `{"id":5,"name":"Imbox","postings":[{"id":1,"kind":"topic"}]}`,
	})

	page, err := client.Boxes().GetPage(context.Background(), 5, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.NextPage != "" {
		t.Errorf("next page = %q, want none", page.NextPage)
	}
}

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

func TestBoxesService_GetGroupPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/boxes/5/groups/11.json" {
			t.Errorf("path = %q, want /boxes/5/groups/11.json", r.URL.Path)
		}
		if page := r.URL.Query().Get("page"); page != "current-cursor" {
			t.Errorf("page = %q, want current-cursor", page)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "150")
		w.Header().Set("Link", `<http://`+r.Host+`/boxes/5/groups/11.json?page=next-cursor>; rel="next"`)
		_, _ = io.WriteString(w, `{"id":11,"box_id":5,"postings":[{"id":1,"kind":"topic","box_group_id":11}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	cursor := "current-cursor"
	page, err := client.Boxes().GetGroupPage(context.Background(), 5, 11, &generated.GetBoxGroupParams{Page: &cursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Group == nil || page.Group.Id != 11 || page.Group.BoxId != 5 || page.TotalCount != 150 || page.NextPage != "next-cursor" {
		t.Errorf("unexpected group page: %+v", page)
	}
	if len(page.Group.Postings) != 1 || page.Group.Postings[0].BoxGroupId != 11 {
		t.Errorf("unexpected postings: %+v", page.Group.Postings)
	}
}

func TestBoxesService_GetGroup(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/boxes/%s/groups/%s.json": `{"id":11,"box_id":5,"postings":[]}`,
	})

	group, err := client.Boxes().GetGroup(context.Background(), 5, 11, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.Id != 11 || len(group.Postings) != 0 {
		t.Errorf("expected empty group 11, got %+v", group)
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

func TestFoldersService_GetPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/folders/12.json" {
			t.Errorf("path = %q, want /folders/12.json", r.URL.Path)
		}
		if page := r.URL.Query().Get("page"); page != "current-cursor" {
			t.Errorf("page = %q, want current-cursor", page)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "42")
		w.Header().Set("Link", `<http://`+r.Host+`/folders/12.json?page=next-cursor>; rel="next"`)
		_, _ = io.WriteString(w, `{"id":12,"name":"Receipts","postings":[{"id":1,"kind":"topic"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	cursor := "current-cursor"
	page, err := client.Folders().GetPage(context.Background(), 12, &generated.GetFolderParams{Page: &cursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Folder == nil || page.Folder.Name != "Receipts" || page.TotalCount != 42 || page.NextPage != "next-cursor" {
		t.Errorf("unexpected folder page: %+v", page)
	}
}

func TestGearedPageFromLink(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "single next link", header: `<https://app.hey.com/folders/12.json?page=next>; rel="next"`, want: "next"},
		{name: "next link last", header: `<https://app.hey.com/folders/12.json?page=previous>; rel="prev", <https://app.hey.com/folders/12.json?page=a,b>; rel="next"`, want: "a,b"},
		{name: "next link first", header: `<https://app.hey.com/folders/12.json?page=next>; rel="next", <https://app.hey.com/folders/12.json?page=previous>; rel="prev"`, want: "next"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gearedPageFromLink(tt.header); got != tt.want {
				t.Errorf("gearedPageFromLink(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
	for _, header := range []string{"", "not a URL", `<https://app.hey.com/folders/12.json>; rel="next"`, `<https://app.hey.com/folders/12.json?page=previous>; rel="prev", <https://app.hey.com/folders/12.json?page=last>; rel="last"`} {
		if got := gearedPageFromLink(header); got != "" {
			t.Errorf("gearedPageFromLink(%q) = %q, want empty", header, got)
		}
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

func TestCollectionsService_Get(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/collections/%s": `{"id":33,"name":"Kitchen remodel","app_url":"https://app.hey.com/collections/33","postings":[{"id":91,"kind":"topic","account_id":7,"seen":true,"name":"Contractor quotes"}]}`,
	})

	collection, err := client.Collections().Get(context.Background(), 33, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if collection.Id != 33 || collection.Name != "Kitchen remodel" || len(collection.Postings) != 1 {
		t.Fatalf("unexpected collection: %+v", collection)
	}
	posting := collection.Postings[0]
	if posting.Id != 91 || posting.Kind != "topic" || posting.AccountId != 7 || !posting.Seen || posting.Name != "Contractor quotes" {
		t.Errorf("unexpected posting: %+v", posting)
	}
}

func TestCollectionsService_GetPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/33.json" {
			t.Errorf("path = %q, want /collections/33.json", r.URL.Path)
		}
		if page := r.URL.Query().Get("page"); page != "current-cursor" {
			t.Errorf("page = %q, want current-cursor", page)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "42")
		w.Header().Set("Link", `<http://`+r.Host+`/collections/33.json?page=next-cursor>; rel="next"`)
		_, _ = io.WriteString(w, `{"id":33,"name":"Kitchen remodel","postings":[{"id":91,"kind":"topic"}]}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	cursor := "current-cursor"
	page, err := client.Collections().GetPage(context.Background(), 33, &generated.GetCollectionParams{Page: &cursor})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Collection == nil || page.Collection.Name != "Kitchen remodel" || len(page.Collection.Postings) != 1 || page.TotalCount != 42 || page.NextPage != "next-cursor" {
		t.Errorf("unexpected collection page: %+v", page)
	}
}

func TestCollectionsService_GetNotFound(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{})

	_, err := client.Collections().Get(context.Background(), 33, nil)
	if err == nil || AsError(err).Code != CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
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
	var sent generated.CreateContactRequestContent
	client := newJSONWriteTestClient(t, http.MethodPost, "/contacts.json", &sent, http.StatusCreated,
		`{"id":91824,"name":"Jane Dawson","email_address":"jane.dawson@example.com"}`)

	contact, err := client.Contacts().Create(context.Background(), ContactParams{
		Name:                "Jane Dawson",
		EmailAddress:        "jane.dawson@example.com",
		AliasEmailAddresses: []string{"j.dawson@example.org"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contact == nil || contact.Id != 91824 {
		t.Fatalf("expected contact 91824, got %+v", contact)
	}
	if sent.Contact.Name != "Jane Dawson" {
		t.Errorf("expected the contact name, got %q", sent.Contact.Name)
	}
	if sent.Contact.EmailAddress != "jane.dawson@example.com" {
		t.Errorf("expected the email address, got %q", sent.Contact.EmailAddress)
	}
	if aliases := sent.Contact.AliasEmailAddresses; len(aliases) != 1 {
		t.Errorf("expected one alias, got %v", aliases)
	}
	if sent.ActingUserId != 0 {
		t.Errorf("no account asked for, so none should be sent, got %d", sent.ActingUserId)
	}
}

// One identity can hold several accounts, each with its own contacts. Without an account
// the server files the contact under the first one, which is rarely what a two-account
// user means.
func TestContactsService_CreateOnAChosenAccount(t *testing.T) {
	var sent generated.CreateContactRequestContent
	client := newJSONWriteTestClient(t, http.MethodPost, "/contacts.json", &sent, http.StatusCreated,
		`{"id":91824,"name":"Jane Dawson"}`)

	_, err := client.Contacts().Create(context.Background(), ContactParams{
		Name:          "Jane Dawson",
		EmailAddress:  "jane.dawson@example.com",
		AccountUserID: 4849,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sent.ActingUserId != 4849 {
		t.Errorf("expected the account's user, got %d", sent.ActingUserId)
	}
}

// An email address that already belongs to someone else is the case HEY answers by
// sending the web to a merge form. The SDK hands back the contacts to merge with.
func TestContactsService_CreateConflict(t *testing.T) {
	client := newJSONStatusTestClient(t, http.StatusConflict,
		`{"errors":["Some email addresses are already in use for other contacts"],"contact_id":9,"conflicting_contact_ids":[4,5]}`)

	_, err := client.Contacts().Create(context.Background(), ContactParams{Name: "Jane", EmailAddress: "jane@example.com"})

	var conflict *ContactConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected a contact conflict, got %v", err)
	}
	if len(conflict.ConflictingContactIDs) != 2 || conflict.ConflictingContactIDs[0] != 4 {
		t.Errorf("expected the contacts to merge with, got %v", conflict.ConflictingContactIDs)
	}
	if conflict.ContactID != 9 {
		t.Errorf("expected the contact that was written, got %d", conflict.ContactID)
	}

	var heyErr *Error
	if !errors.As(err, &heyErr) || heyErr.Code != CodeConflict {
		t.Errorf("a conflict should still read as a hey conflict error, got %v", err)
	}
	if heyErr.Message != "Some email addresses are already in use for other contacts" {
		t.Errorf("expected the server's own message, got %q", heyErr.Message)
	}
}

// HEY can answer a write with a different contact than the one addressed: giving a
// contact one of its own aliases as the main address promotes the alias, and the alias
// becomes the primary. Callers have to read the id back rather than assume it.
func TestContactsService_UpdatePromotesAnAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":7,"name":"Jane","email_address":"jane@x.com","aliases":[{"id":8,"email_address":"jd@x.com"}]}`))
		default:
			_, _ = w.Write([]byte(`{"id":8,"name":"Jane","email_address":"jd@x.com"}`))
		}
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	contact, err := client.Contacts().Update(context.Background(), 7, ContactParams{EmailAddress: "jd@x.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contact == nil || contact.Id != 8 {
		t.Fatalf("expected the promoted alias, got %+v", contact)
	}
}

// A 409 the SDK cannot read still has to produce a usable error.
func TestContactsService_CreateConflictWithoutAMessage(t *testing.T) {
	client := newJSONStatusTestClient(t, http.StatusConflict, `{"conflicting_contact_ids":[4]}`)

	_, err := client.Contacts().Create(context.Background(), ContactParams{Name: "Jane", EmailAddress: "jane@example.com"})

	var conflict *ContactConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected a contact conflict, got %v", err)
	}
	if conflict.Error() == "" {
		t.Error("a conflict with no message should still say something")
	}
}

func TestContactsService_CreateInvalid(t *testing.T) {
	client := newJSONStatusTestClient(t, http.StatusUnprocessableEntity,
		`{"errors":["Email address is already in use for another contact"]}`)

	_, err := client.Contacts().Create(context.Background(), ContactParams{Name: "Jane"})

	var heyErr *Error
	if !errors.As(err, &heyErr) || heyErr.Code != CodeValidation {
		t.Fatalf("expected a validation error, got %v", err)
	}
	if heyErr.Message != "Email address is already in use for another contact" {
		t.Errorf("expected the server's own message, got %q", heyErr.Message)
	}
}

func TestContactsService_HideAndReveal(t *testing.T) {
	hider := newJSONWriteTestClient(t, http.MethodDelete, "/contacts/91824.json", nil, http.StatusNoContent, "")
	if err := hider.Contacts().Hide(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error hiding: %v", err)
	}

	revealer := newJSONWriteTestClient(t, http.MethodPost, "/contacts/91824/reveal.json", nil, http.StatusOK,
		`{"id":91824,"name":"Jane Dawson"}`)
	contact, err := revealer.Contacts().Reveal(context.Background(), 91824)
	if err != nil {
		t.Fatalf("unexpected error revealing: %v", err)
	}
	if contact == nil || contact.Id != 91824 {
		t.Errorf("expected the revealed contact, got %+v", contact)
	}
}

func TestContactsService_Notes(t *testing.T) {
	reader := newServiceTestClient(t, map[string]string{
		"/contacts/91824/note.json": `{"contact_id":91824,"note":"Prefers a call","note_html":"<div>Prefers a call</div>"}`,
	})
	note, err := reader.Contacts().Note(context.Background(), 91824)
	if err != nil {
		t.Fatalf("unexpected error reading the note: %v", err)
	}
	if note == nil || note.Note != "Prefers a call" {
		t.Fatalf("expected the note, got %+v", note)
	}

	var sent generated.ContactNoteRequestContent
	setter := newJSONWriteTestClient(t, http.MethodPatch, "/contacts/91824/note.json", &sent, http.StatusOK,
		`{"contact_id":91824,"note":"Prefers a call to an email","note_html":"<div>Prefers a call to an email</div>"}`)
	written, err := setter.Contacts().SetNote(context.Background(), 91824, "Prefers a call to an email")
	if err != nil {
		t.Fatalf("unexpected error setting the note: %v", err)
	}
	if sent.Contact.Note != "Prefers a call to an email" {
		t.Errorf("expected the note, got %q", sent.Contact.Note)
	}
	if written == nil || written.Note != "Prefers a call to an email" {
		t.Errorf("expected the note as written, got %+v", written)
	}

	deleter := newJSONWriteTestClient(t, http.MethodDelete, "/contacts/91824/note.json", nil, http.StatusNoContent, "")
	if err := deleter.Contacts().DeleteNote(context.Background(), 91824); err != nil {
		t.Fatalf("unexpected error deleting the note: %v", err)
	}
}

// newJSONWriteTestClient answers a fixed body for one method and path, decoding the
// request body into want when it is not nil.
func newJSONWriteTestClient(t *testing.T, method, path string, want any, status int, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			t.Errorf("expected %s %s, got %s %s", method, path, r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if want != nil {
			if err := json.NewDecoder(r.Body).Decode(want); err != nil {
				t.Errorf("decoding the request body: %v", err)
			}
		}
		if body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
}

// newJSONStatusTestClient answers every request with the same status and body.
func newJSONStatusTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
}

// --- Time track categories and exports ---

func TestTimeTracksService_Categories(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/calendar/time_tracks/categories.json": `[{"id":7,"title":"Consulting"},{"id":9,"title":"Writing"}]`,
	})

	categories, err := client.TimeTracks().Categories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(categories) != 2 || categories[0].Id != 7 || categories[0].Title != "Consulting" {
		t.Errorf("unexpected categories %+v", categories)
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
	client := newServiceTestClient(t, map[string]string{
		"/clips.json": `[{"id":41,"content":"Remember the wire transfer","entry_id":5512,"topic":{"id":331,"name":"Wire transfer snafu","app_url":"https://app.hey.com/topics/331"}}]`,
	})

	clips, err := client.Clips().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clips) != 1 || clips[0].Id != 41 || clips[0].Content != "Remember the wire transfer" || clips[0].Topic.Id != 331 {
		t.Errorf("unexpected clips %+v", clips)
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
	client := newServiceTestClient(t, map[string]string{
		"/snippets.json": `[{"id":3,"name":"Rob's \"sig\" & co","content":"Cheers, Rob","content_html":"<div class=\"trix-content\">Cheers, Rob</div>"}]`,
	})

	snippets, err := client.Snippets().List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 1 || snippets[0].Id != 3 || snippets[0].Name != "Rob's \"sig\" & co" {
		t.Errorf("unexpected snippets %+v", snippets)
	}
	if snippets[0].Content != "Cheers, Rob" || !strings.Contains(snippets[0].ContentHtml, "trix-content") {
		t.Errorf("expected text and HTML content, got %+v", snippets[0])
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
	client := newServiceTestClient(t, map[string]string{
		"/workflows/%s.json": `{"id":8801,"name":"Hiring","stages":[{"id":5512,"name":"Applied"},{"id":5513,"name":"Interviewing"}]}`,
	})

	stages, err := client.Workflows().Stages(context.Background(), 8801)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stages) != 2 || stages[0].Id != 5512 || stages[0].Name != "Applied" || stages[1].Name != "Interviewing" {
		t.Errorf("unexpected stages %+v", stages)
	}

	workflow, err := client.Workflows().Get(context.Background(), 8801)
	if err != nil || workflow == nil || workflow.Name != "Hiring" || len(workflow.Stages) != 2 {
		t.Errorf("unexpected workflow %+v err=%v", workflow, err)
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

	mover := newFormTestClient(t, "PATCH", "/topics/%s/workflows/%s/stagings", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("workflow_staging[workflow_stage_id]") != "5513" {
			t.Errorf("expected the destination stage id, got %q", values.Get("workflow_staging[workflow_stage_id]"))
		}
	}, "")
	if err := mover.Workflows().MoveTopic(context.Background(), 4471829, 8801, 5513); err != nil {
		t.Fatalf("unexpected error moving a staged topic: %v", err)
	}
}

func TestWorkflowsService_StageTopicMovesToRequestedStage(t *testing.T) {
	const stagingPath = "/topics/4471829/workflows/8801/stagings"
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != stagingPath {
			t.Errorf("expected path %s, got %s", stagingPath, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Errorf("expected the form representation, got Accept %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		switch requestCount {
		case 1:
			if r.Method != http.MethodPost {
				t.Errorf("expected the staging request to use POST, got %s", r.Method)
			}
			if len(r.PostForm) != 0 {
				t.Errorf("expected the staging request body to be empty, got %v", r.PostForm)
			}
		case 2:
			if r.Method != http.MethodPatch {
				t.Errorf("expected the move request to use PATCH, got %s", r.Method)
			}
			if got := r.PostForm.Get("workflow_staging[workflow_stage_id]"); got != "5512" {
				t.Errorf("expected the destination stage id, got %q", got)
			}
		default:
			t.Errorf("unexpected request %d", requestCount)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	if err := client.Workflows().StageTopic(context.Background(), 4471829, 8801, 5512); err != nil {
		t.Fatalf("unexpected error staging a topic: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected staging and move requests, got %d requests", requestCount)
	}
}

func TestWorkflowsService_StageTopicSurfacesStageSelectionError(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	if err := client.Workflows().StageTopic(context.Background(), 4471829, 8801, 9999); err == nil {
		t.Fatal("expected the stage-selection error to surface")
	}
	if requestCount != 2 {
		t.Fatalf("expected staging and move requests, got %d requests", requestCount)
	}
}

func TestWorkflowsService_UpdateAndDelete(t *testing.T) {
	renamer := newFormTestClient(t, "PATCH", "/workflows/%s", func(t *testing.T, values url.Values) {
		t.Helper()
		if values.Get("workflow[name]") != "Recruiting" {
			t.Errorf("expected the workflow name, got %q", values.Get("workflow[name]"))
		}
	}, "/workflows/8801")
	if err := renamer.Workflows().Update(context.Background(), 8801, "Recruiting"); err != nil {
		t.Fatalf("unexpected error renaming: %v", err)
	}

	deleter := newFormTestClient(t, "DELETE", "/workflows/%s", nil, "/workflows")
	if err := deleter.Workflows().Delete(context.Background(), 8801); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	stageDeleter := newFormTestClient(t, "DELETE", "/workflows/%s/stages/%s", nil, "/workflows/8801")
	if err := stageDeleter.Workflows().DeleteStage(context.Background(), 8801, 5512); err != nil {
		t.Fatalf("unexpected error deleting a stage: %v", err)
	}

	unfiler := newFormTestClient(t, "DELETE", "/topics/%s/workflows/%s/stagings", nil, "/topics/4471829")
	if err := unfiler.Workflows().UnstageTopic(context.Background(), 4471829, 8801); err != nil {
		t.Fatalf("unexpected error unstaging a topic: %v", err)
	}
}

func TestWorkflowsService_WriteErrorsSurface(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))
	if err := c.Workflows().Delete(context.Background(), 8801); err == nil {
		t.Fatal("expected a 403 to surface as an error")
	}
	if err := c.Workflows().MoveTopic(context.Background(), 4471829, 8801, 5513); err == nil {
		t.Fatal("expected a 403 moving a staged topic to surface as an error")
	}
	if err := c.Workflows().UnstageTopic(context.Background(), 4471829, 8801); err == nil {
		t.Fatal("expected a 403 to surface as an error")
	}
}

// --- Publications ---

func TestPublicationsService_GetPublished(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/%s/publication.json": `{"published":true,"url":"https://public.hey.com/p/abc123"}`,
	})

	publication, err := client.Publications().Get(context.Background(), 4471829)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !publication.Published || publication.Url != "https://public.hey.com/p/abc123" {
		t.Errorf("expected a published thread with its link, got %+v", publication)
	}
}

func TestPublicationsService_GetUnpublished(t *testing.T) {
	client := newServiceTestClient(t, map[string]string{
		"/topics/%s/publication.json": `{"published":false}`,
	})

	publication, err := client.Publications().Get(context.Background(), 4471829)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if publication.Published || publication.Url != "" {
		t.Errorf("expected an unpublished thread, got %+v", publication)
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
