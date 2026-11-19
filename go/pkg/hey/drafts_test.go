package hey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// draftTestRoute is one route a draft test serves: the validation to run on the
// request and the response to answer with. A zero status means 200.
type draftTestRoute struct {
	method    string
	status    int
	location  string
	body      string
	link      string
	requestID string
	validate  func(t *testing.T, body map[string]any)
}

// newDraftTestClient serves /identity.json for the sender lookup and the given routes,
// keyed by path pattern (pathMatch), and fails the test on anything else.
func newDraftTestClient(t *testing.T, routes map[string]draftTestRoute, opts ...ClientOption) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(identityJSON))
			return
		}
		for pattern, route := range routes {
			if !pathMatch(pattern, r.URL.Path) || r.Method != route.method {
				continue
			}
			if route.validate != nil {
				data, _ := io.ReadAll(r.Body)
				var body map[string]any
				if err := json.Unmarshal(data, &body); err != nil {
					t.Errorf("unreadable request body: %v", err)
				}
				route.validate(t, body)
			}
			if route.location != "" {
				w.Header().Set("Location", route.location)
			}
			if route.link != "" {
				w.Header().Set("Link", route.link)
			}
			if route.requestID != "" {
				w.Header().Set("X-Request-Id", route.requestID)
			}
			status := route.status
			if status == 0 {
				status = 200
			}
			if route.body != "" {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(status)
			w.Write([]byte(route.body))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(server.Close)

	cfg := &Config{BaseURL: server.URL}
	return NewClient(cfg, &StaticTokenProvider{Token: "test-token"}, append([]ClientOption{
		WithMaxRetries(0),
		WithBaseDelay(1 * time.Millisecond),
		WithMaxJitter(1 * time.Millisecond),
	}, opts...)...)
}

func TestMessagesService_CreateDraft(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages.json": {
			method:   "POST",
			status:   204,
			location: "https://app.hey.com/messages/12345",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				entry, _ := body["entry"].(map[string]any)
				if entry["status"] != "drafted" {
					t.Errorf("entry.status = %v, want drafted", entry["status"])
				}
				if _, present := entry["addressed"]; !present {
					t.Error("a draft always carries addressed, empty included")
				}
				msg, _ := body["message"].(map[string]any)
				if msg["subject"] != "Quarterly planning" || msg["content"] != "<div>Agenda to follow.</div>" {
					t.Errorf("message = %v", msg)
				}
			},
		},
	})

	// No recipients: a draft with nobody on it yet is the normal case.
	id, err := client.Messages().CreateDraft(context.Background(), DraftContent{
		Subject: "Quarterly planning",
		Content: "<div>Agenda to follow.</div>",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 12345 {
		t.Errorf("draft entry id = %d, want 12345 (from Location)", id)
	}
}

func TestMessagesService_CreateDraft_SchedulesToTheHour(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages.json": {
			method:   "POST",
			status:   204,
			location: "https://app.hey.com/messages/7",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				entry, _ := body["entry"].(map[string]any)
				if entry["scheduled_delivery"] != "true" {
					t.Errorf("scheduled_delivery = %v, want \"true\"", entry["scheduled_delivery"])
				}
				if entry["scheduled_delivery_at_date"] != "2026-09-01" {
					t.Errorf("scheduled_delivery_at_date = %v", entry["scheduled_delivery_at_date"])
				}
				// Midnight must survive: the hour rides as a string so 0 is not omitted.
				if entry["scheduled_delivery_at_hour"] != "0" {
					t.Errorf("scheduled_delivery_at_hour = %v, want \"0\"", entry["scheduled_delivery_at_hour"])
				}
			},
		},
	})

	_, err := client.Messages().CreateDraft(context.Background(), DraftContent{
		Subject:  "Launch reminder",
		Content:  "<div>It ships today.</div>",
		To:       []string{"maria@example.com"},
		Schedule: &DraftSchedule{Date: "2026-09-01", Hour: 0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMessagesService_UpdateDraft_ReplacesRecipientsWithWhatIsSent(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345.json": {
			method:   "PUT",
			status:   204,
			location: "https://app.hey.com/messages/12345",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				entry, _ := body["entry"].(map[string]any)
				if entry["status"] != "drafted" {
					t.Errorf("entry.status = %v, want drafted", entry["status"])
				}
				// addressed present but empty is how recipients are removed: HEY
				// replaces the recipient set with what is sent, and an omitted
				// addressed would keep them instead.
				addressed, present := entry["addressed"].(map[string]any)
				if !present {
					t.Fatal("an update always carries addressed, or removed recipients would survive")
				}
				if len(addressed) != 0 {
					t.Errorf("addressed = %v, want empty for a draft with no recipients", addressed)
				}
				if _, scheduled := entry["scheduled_delivery"]; scheduled {
					t.Error("no schedule was asked for; sending one would set it")
				}
			},
		},
	})

	err := client.Messages().UpdateDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning (v2)",
		Content: "<div>Rewritten agenda.</div>",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMessagesService_SendDraft(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345.json": {
			method: "PUT",
			body:   `{"id":99}`,
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				entry, _ := body["entry"].(map[string]any)
				if _, present := entry["status"]; present {
					t.Errorf("sending must omit entry.status, got %v", entry["status"])
				}
				addressed, _ := entry["addressed"].(map[string]any)
				directly, _ := addressed["directly"].([]any)
				if len(directly) != 1 || directly[0] != "maria@example.com" {
					t.Errorf("directly = %v", addressed["directly"])
				}
			},
		},
	})

	_, err := client.Messages().SendDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning",
		Content: "<div>Final agenda attached.</div>",
		To:      []string{"maria@example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDraftLifecycleCarriesTheChosenActingSender(t *testing.T) {
	// A draft saved as an alternate identity (the reply prefill's sender) must stay
	// that identity through every revision and the final send: HEY revises a draft
	// from the whole of what is sent, acting_sender_id included, so a lifecycle call
	// that fell back to the account default would hand the draft back to it.
	senderOn := func(want float64) func(*testing.T, map[string]any) {
		return func(t *testing.T, body map[string]any) {
			t.Helper()
			if got, _ := body["acting_sender_id"].(float64); got != want {
				t.Errorf("acting_sender_id = %v, want %v", body["acting_sender_id"], want)
			}
		}
	}
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages.json": {
			method: "POST", status: 204,
			location: "https://app.hey.com/messages/12345",
			validate: senderOn(4242),
		},
		"/messages/12345.json": {
			method: "PUT", status: 204,
			location: "https://app.hey.com/messages/12345",
			validate: senderOn(4242),
		},
		"/messages/67890.json": {
			method: "PUT", body: `{"id":99}`,
			validate: senderOn(42), // zero falls back to the account default
		},
	})

	chosen := DraftContent{Subject: "From the support address", Content: "<div>…</div>", ActingSenderID: 4242}
	if _, err := client.Messages().CreateDraft(context.Background(), chosen); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if err := client.Messages().UpdateDraft(context.Background(), 12345, chosen); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	chosen.ActingSenderID = 4242
	chosen.To = []string{"maria@example.com"}
	if _, err := client.Messages().SendDraft(context.Background(), 12345, chosen); err != nil {
		t.Fatalf("SendDraft: %v", err)
	}

	unchosen := DraftContent{Subject: "As myself", Content: "<div>…</div>", To: []string{"maria@example.com"}}
	if _, err := client.Messages().SendDraft(context.Background(), 67890, unchosen); err != nil {
		t.Fatalf("SendDraft (default sender): %v", err)
	}
}

func TestMessagesService_UpdateDraft_SurfacesTheReasonsFor422(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345.json": {
			method:    "PUT",
			status:    422,
			body:      `{"errors":["Subject is too long (maximum is 255 characters)","Content can't be blank"]}`,
			requestID: "req-draft-422",
		},
	})

	err := client.Messages().UpdateDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning",
	})
	assertDraftValidationError(t, err, "Subject is too long (maximum is 255 characters); Content can't be blank", "req-draft-422")
}

func TestMessagesService_SendDraft_SurfacesTheReasonsFor422(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345.json": {
			method:    "PUT",
			status:    422,
			body:      `{"errors":["Recipient maria@example is not a valid email address"]}`,
			requestID: "req-draft-422",
		},
	})

	_, err := client.Messages().SendDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning",
		Content: "<div>Agenda.</div>",
		To:      []string{"maria@example"},
	})
	assertDraftValidationError(t, err, "Recipient maria@example is not a valid email address", "req-draft-422")
}

func TestMessagesService_SendDraft_KeepsTheGenericMessageForABodyless422(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345.json": {method: "PUT", status: 422, requestID: "req-draft-bare"},
	})

	_, err := client.Messages().SendDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning",
		Content: "<div>Agenda.</div>",
		To:      []string{"maria@example.com"},
	})
	assertDraftValidationError(t, err, "validation error", "req-draft-bare")
}

// assertDraftValidationError checks a draft write's 422 keeps the structured contract
// CheckResponse gives every other status — code, status and request id — with HEY's
// reasons as the message.
func assertDraftValidationError(t *testing.T, err error, message, requestID string) {
	t.Helper()
	e := AsError(err)
	if e == nil || e.Code != CodeValidation {
		t.Fatalf("expected a validation error, got %#v", err)
	}
	if e.Message != message {
		t.Errorf("Message = %q, want %q", e.Message, message)
	}
	if e.HTTPStatus != 422 {
		t.Errorf("HTTPStatus = %d, want 422", e.HTTPStatus)
	}
	if e.RequestID != requestID {
		t.Errorf("RequestID = %q, want %q", e.RequestID, requestID)
	}
	if e.Retryable {
		t.Error("a validation error is not retryable")
	}
}

func TestMessagesService_SendDraft_RequiresRecipients(t *testing.T) {
	client := newDraftTestClient(t, nil)

	_, err := client.Messages().SendDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning",
		Content: "<div>Agenda.</div>",
	})
	if e := AsError(err); e == nil || e.Code != CodeUsage {
		t.Fatalf("expected a usage error, got %#v", err)
	}
}

func TestMessagesService_GetEdit(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345/edit.json": {
			method: "GET",
			body: `{"id":12345,"subject":"Quarterly planning","content":"<div>Agenda to follow.</div>",
				"addressed":{"directly":[{"id":7,"name":"Maria Delgado","email_address":"maria@example.com"}]}}`,
		},
	})

	edit, err := client.Messages().GetEdit(context.Background(), 12345)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if edit == nil || edit.Id != 12345 || edit.Subject != "Quarterly planning" {
		t.Fatalf("edit = %+v", edit)
	}
	if len(edit.Addressed.Directly) != 1 || edit.Addressed.Directly[0].EmailAddress != "maria@example.com" {
		t.Errorf("addressed = %+v", edit.Addressed)
	}
}

func TestEntriesService_DeleteDraft(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/entries/drafts/12345.json": {method: "DELETE", status: 204},
	})

	if err := client.Entries().DeleteDraft(context.Background(), 12345); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_ListDraftsPage(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/entries/drafts.json": {
			method: "GET",
			body:   `[{"id":1,"subject":"Quarterly planning"}]`,
			link:   `<https://app.hey.com/entries/drafts.json?page=draft-cursor-2>; rel="next"`,
		},
	})

	page, err := client.Entries().ListDraftsPage(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Drafts) != 1 || page.Drafts[0].Id != 1 {
		t.Errorf("drafts = %+v", page.Drafts)
	}
	if page.NextPage != "draft-cursor-2" {
		t.Errorf("next page = %q, want the Link header's cursor", page.NextPage)
	}
}

func TestEntriesService_ListDraftsPage_LastPageEndsTheWalk(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/entries/drafts.json": {method: "GET", body: `[]`},
	})

	page, err := client.Entries().ListDraftsPage(context.Background(), "draft-cursor-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.NextPage != "" {
		t.Errorf("next page = %q, want empty on the last page", page.NextPage)
	}
}

func TestEntriesService_CreateReplyDraft(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/entries/10/replies.json": {
			method:   "POST",
			status:   204,
			location: "https://app.hey.com/messages/777",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				entry, _ := body["entry"].(map[string]any)
				if entry["status"] != "drafted" {
					t.Errorf("entry.status = %v, want drafted", entry["status"])
				}
				msg, _ := body["message"].(map[string]any)
				if msg["content"] != "<div>Drafting a reply.</div>" {
					t.Errorf("content = %v", msg["content"])
				}
				// HEY does not derive a subject for a reply: a draft saved without
				// one shows as "No subject" in Drafts.
				if msg["subject"] != "Re: Original subject" {
					t.Errorf("subject = %v, want the prefilled Re: subject", msg["subject"])
				}
			},
		},
	})

	// No recipients: unlike CreateReply, a reply draft is allowed to have nobody on it.
	id, err := client.Entries().CreateReplyDraft(context.Background(), 10, 0, "Re: Original subject", "<div>Drafting a reply.</div>", nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 777 {
		t.Errorf("draft entry id = %d, want 777 (from Location)", id)
	}
}

func TestEntriesService_CreateReplyDraft_SendsTheChosenActingSender(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/entries/10/replies.json": {
			method:   "POST",
			status:   204,
			location: "https://app.hey.com/messages/777",
			validate: func(t *testing.T, body map[string]any) {
				t.Helper()
				if got, _ := body["acting_sender_id"].(float64); got != 4242 {
					t.Errorf("acting_sender_id = %v, want the chosen sender 4242", body["acting_sender_id"])
				}
			},
		},
	})

	if _, err := client.Entries().CreateReplyDraft(context.Background(), 10, 4242, "Re: Original subject", "<div>Drafting a reply.</div>", nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEntriesService_NewReply(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/entries/10/replies/new.json": {
			method: "GET",
			body: `{"content":"<div>quoted</div>","is_reply":true,
				"addressed":{"directly":[{"id":7,"name":"Maria Delgado","email_address":"maria@example.com"}]}}`,
		},
	})

	reply, err := client.Entries().NewReply(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == nil || !reply.IsReply {
		t.Fatalf("reply = %+v", reply)
	}
	if len(reply.Addressed.Directly) != 1 || reply.Addressed.Directly[0].EmailAddress != "maria@example.com" {
		t.Errorf("addressed = %+v", reply.Addressed)
	}
}

func TestDraftEntryIDFromLocation(t *testing.T) {
	cases := []struct {
		name     string
		location string
		want     int64
	}{
		{"absolute", "https://app.hey.com/messages/12345", 12345},
		{"relative", "/messages/98", 98},
		{"missing header", "", 0},
		{"no id", "https://app.hey.com/imbox", 0},
		{"negative", "https://app.hey.com/messages/-3", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tc.location != "" {
				resp.Header.Set("Location", tc.location)
			}
			id, err := draftEntryIDFromLocation(resp)
			if tc.want == 0 {
				if err == nil {
					t.Fatalf("expected an error, got id %d", id)
				}
				return
			}
			if err != nil || id != tc.want {
				t.Fatalf("id = %d, err = %v, want %d", id, err, tc.want)
			}
		})
	}
}

func TestDraftSentAs_SenderElseCreator(t *testing.T) {
	creator := generated.Contact{Id: 11, EmailAddress: "maria@hey.com"}
	alternate := generated.Contact{Id: 22, EmailAddress: "support@example.com"}

	if got := DraftSentAs(&generated.MessageEditState{Sender: alternate, Creator: creator}); got.Id != 22 {
		t.Errorf("a draft with a sender sends as it, got %+v", got)
	}
	// HEY names a sender only when it differs from the creator, so a draft sent
	// from the person's own address carries none.
	if got := DraftSentAs(&generated.MessageEditState{Creator: creator}); got.Id != 11 {
		t.Errorf("a draft without a sender sends as its creator, got %+v", got)
	}
	if got := DraftSentAs(&generated.MessageEditState{}); got.Id != 0 {
		t.Errorf("a draft naming neither answers zero, got %+v", got)
	}
	if got := DraftSentAs(nil); got.Id != 0 {
		t.Errorf("nil answers zero, got %+v", got)
	}
}

func TestReplySentAs_SenderElseCreator(t *testing.T) {
	creator := generated.Contact{Id: 11}
	if got := ReplySentAs(&generated.MessageDraft{Sender: generated.Contact{Id: 22}, Creator: creator}); got.Id != 22 {
		t.Errorf("the thread's resolved sender wins, got %+v", got)
	}
	if got := ReplySentAs(&generated.MessageDraft{Creator: creator}); got.Id != 11 {
		t.Errorf("without one the reply goes as the creator, got %+v", got)
	}
	if got := ReplySentAs(nil); got.Id != 0 {
		t.Errorf("nil answers zero, got %+v", got)
	}
}

func TestDraftContentFromEdit(t *testing.T) {
	var edit generated.MessageEditState
	if err := json.Unmarshal([]byte(`{"id":12345,"subject":"Quarterly planning","content":"<div>Agenda</div>",
		"creator":{"id":11,"email_address":"maria@hey.com"},
		"addressed":{"directly":[{"id":7,"email_address":"ana@example.com"},{"id":8}],
			"copied":[{"id":9,"email_address":"lee@example.com"}],
			"blindcopied":[{"id":10,"email_address":"kim@example.com"}]},
		"scheduled_delivery_at":"2026-11-20T09:00:00-05:00"}`), &edit); err != nil {
		t.Fatal(err)
	}

	got := DraftContentFromEdit(&edit)
	if got.Subject != "Quarterly planning" || got.Content != "<div>Agenda</div>" {
		t.Errorf("subject and body = %q, %q", got.Subject, got.Content)
	}
	if !reflect.DeepEqual(got.To, []string{"ana@example.com"}) || !reflect.DeepEqual(got.CC, []string{"lee@example.com"}) || !reflect.DeepEqual(got.BCC, []string{"kim@example.com"}) {
		t.Errorf("recipients = %v %v %v; a contact served without an address is skipped", got.To, got.CC, got.BCC)
	}
	// The creator, not zero: zero would move the draft to the account's default sender.
	if got.ActingSenderID != 11 {
		t.Errorf("ActingSenderID = %d, want the creator's 11", got.ActingSenderID)
	}
	// HEY's API reads a schedule's date and hour in UTC.
	if got.Schedule == nil || *got.Schedule != (DraftSchedule{Date: "2026-11-20", Hour: 14}) {
		t.Errorf("schedule = %+v, want 2026-11-20 at 14 UTC", got.Schedule)
	}

	edit.Sender = generated.Contact{Id: 22}
	edit.ScheduledDeliveryAt = time.Time{}
	got = DraftContentFromEdit(&edit)
	if got.ActingSenderID != 22 || got.Schedule != nil {
		t.Errorf("ActingSenderID = %d, schedule = %+v; want the sender's 22 and no schedule", got.ActingSenderID, got.Schedule)
	}
	if got := DraftContentFromEdit(nil); !reflect.DeepEqual(got, DraftContent{}) {
		t.Errorf("nil = %+v", got)
	}
}

// An edit read back and revised unchanged sends the identity it was saved as, so a
// draft from the person's own non-default address stays there.
func TestUpdateDraftFromEditKeepsTheCreatorsIdentity(t *testing.T) {
	client := newDraftTestClient(t, map[string]draftTestRoute{
		"/messages/12345/edit.json": {
			method: "GET",
			body:   `{"id":12345,"subject":"Hi","creator":{"id":424242,"email_address":"maria@hey.com"}}`,
		},
		"/messages/12345.json": {
			method: "PUT", status: 204, location: "https://app.hey.com/messages/12345",
			validate: func(t *testing.T, body map[string]any) {
				if got := body["acting_sender_id"]; got != float64(424242) {
					t.Errorf("acting_sender_id = %v, want the creator's 424242, not the default sender", got)
				}
			},
		},
	})

	edit, err := client.Messages().GetEdit(context.Background(), 12345)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Messages().UpdateDraft(context.Background(), 12345, DraftContentFromEdit(edit)); err != nil {
		t.Fatal(err)
	}
}
