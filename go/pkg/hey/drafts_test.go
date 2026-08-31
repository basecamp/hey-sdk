package hey

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// draftTestRoute is one route a draft test serves: the validation to run on the
// request and the response to answer with. A zero status means 200.
type draftTestRoute struct {
	method   string
	status   int
	location string
	body     string
	link     string
	validate func(t *testing.T, body map[string]any)
}

// newDraftTestClient serves /identity.json for the sender lookup and the given routes,
// keyed by path pattern (pathMatch), and fails the test on anything else.
func newDraftTestClient(t *testing.T, routes map[string]draftTestRoute) *Client {
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
	return NewClient(cfg, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
		WithBaseDelay(1*time.Millisecond),
		WithMaxJitter(1*time.Millisecond),
	)
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

	err := client.Messages().SendDraft(context.Background(), 12345, DraftContent{
		Subject: "Quarterly planning",
		Content: "<div>Final agenda attached.</div>",
		To:      []string{"maria@example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMessagesService_SendDraft_RequiresRecipients(t *testing.T) {
	client := newDraftTestClient(t, nil)

	err := client.Messages().SendDraft(context.Background(), 12345, DraftContent{
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
