package hey

import (
	"context"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// The answers HEY gives for a delivered message (entries/_sent.jbuilder), before and after
// it named the entry it delivered.
const (
	sentNowJSON = `{"id":2201,"topic_id":880,"subject":"Lunch on Friday","delayed":false}`

	sentDelayedJSON = `{"id":2201,"topic_id":880,"subject":"Lunch on Friday","delayed":true,` +
		`"notice":"Message sent","undo_action":"https://app.hey.com/topics/880/undo_send","undo_timeout":12}`

	// A HEY that predates the ids: nothing without Undo Send, the undo members with it.
	sentBeforeIDsJSON        = `{}`
	sentDelayedBeforeIDsJSON = `{"notice":"Message sent","undo_action":"https://app.hey.com/topics/880/undo_send","undo_timeout":12}`
)

// sendingWrapper is one of the calls that deliver a message, and the route it is sent on.
type sendingWrapper struct {
	name   string
	method string
	path   string
	send   func(*Client) (*generated.SentMessage, error)
}

func sendingWrappers() []sendingWrapper {
	ctx := context.Background()
	to := []string{"maria@example.com"}
	return []sendingWrapper{
		{"Messages.Create", "POST", "/messages.json", func(c *Client) (*generated.SentMessage, error) {
			return c.Messages().Create(ctx, "Lunch on Friday", "<div>Are you free at noon?</div>", to, nil, nil)
		}},
		{"Messages.Send", "POST", "/messages.json", func(c *Client) (*generated.SentMessage, error) {
			return c.Messages().Send(ctx, MessageContent{Subject: "Lunch on Friday", Content: "<div>Are you free at noon?</div>", To: to, ActingSenderID: 314})
		}},
		{"Messages.SendDraft", "PUT", "/messages/2201.json", func(c *Client) (*generated.SentMessage, error) {
			return c.Messages().SendDraft(ctx, 2201, DraftContent{Subject: "Lunch on Friday", Content: "<div>Are you free at noon?</div>", To: to})
		}},
		{"Entries.CreateReply", "POST", "/entries/1990/replies.json", func(c *Client) (*generated.SentMessage, error) {
			return c.Entries().CreateReply(ctx, 1990, 0, "Lunch on Friday", "<div>Noon works.</div>", to, nil, nil)
		}},
	}
}

func TestSendingWrappersAnswerTheEntryHEYDelivered(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		t.Run(wrapper.name, func(t *testing.T) {
			client := newDraftTestClient(t, map[string]draftTestRoute{
				wrapper.path: {method: wrapper.method, body: sentNowJSON},
			})

			sent, err := wrapper.send(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := generated.SentMessage{Id: 2201, TopicId: 880, Subject: "Lunch on Friday"}
			if *sent != want {
				t.Errorf("sent = %+v, want %+v", *sent, want)
			}
		})
	}
}

func TestSendingWrappersAnswerAnUndoableDelivery(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		t.Run(wrapper.name, func(t *testing.T) {
			client := newDraftTestClient(t, map[string]draftTestRoute{
				wrapper.path: {method: wrapper.method, body: sentDelayedJSON},
			})

			sent, err := wrapper.send(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := generated.SentMessage{
				Id: 2201, TopicId: 880, Subject: "Lunch on Friday", Delayed: true,
				Notice: "Message sent", UndoAction: "https://app.hey.com/topics/880/undo_send", UndoTimeout: 12,
			}
			if *sent != want {
				t.Errorf("sent = %+v, want %+v", *sent, want)
			}
		})
	}
}

func TestSendingWrappersAnswerWithoutIDsFromAHEYThatServesNone(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		t.Run(wrapper.name, func(t *testing.T) {
			client := newDraftTestClient(t, map[string]draftTestRoute{
				wrapper.path: {method: wrapper.method, body: sentBeforeIDsJSON},
			})

			sent, err := wrapper.send(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent == nil || *sent != (generated.SentMessage{}) {
				t.Errorf("sent = %+v, want an empty answer", sent)
			}
		})
	}
}

func TestSendingWrappersReadDelayFromTheUndoOfAHEYThatServesNoIDs(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		t.Run(wrapper.name, func(t *testing.T) {
			client := newDraftTestClient(t, map[string]draftTestRoute{
				wrapper.path: {method: wrapper.method, body: sentDelayedBeforeIDsJSON},
			})

			sent, err := wrapper.send(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := generated.SentMessage{
				Delayed: true, Notice: "Message sent", UndoAction: "https://app.hey.com/topics/880/undo_send", UndoTimeout: 12,
			}
			if *sent != want {
				t.Errorf("sent = %+v, want %+v", *sent, want)
			}
		})
	}
}

// When HEY states delayed its value stands; the undo only stands in for a delayed HEY leaves out.
func TestSendingWrappersKeepTheDelayHEYStates(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		t.Run(wrapper.name, func(t *testing.T) {
			client := newDraftTestClient(t, map[string]draftTestRoute{
				wrapper.path: {method: wrapper.method, body: `{"id":2201,"topic_id":880,"delayed":false,"undo_action":"https://app.hey.com/topics/880/undo_send"}`},
			})

			sent, err := wrapper.send(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sent.Delayed {
				t.Errorf("delayed = true, want the false HEY stated")
			}
		})
	}
}

// The message has gone out by the time its answer is read, so an answer the SDK cannot
// read must not turn into an error: a caller told the send failed would send it again.
func TestSendingWrappersDoNotFailADeliveryOverAnUnreadableAnswer(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		for _, answer := range []struct {
			name   string
			status int
			body   string
		}{
			{"no body", 200, ""},
			{"not JSON", 200, "<html>Message sent</html>"},
			{"the wrong types", 200, `{"id":"2201","topic_id":[880]}`},
			{"no content", 204, ""},
		} {
			t.Run(wrapper.name+"/"+answer.name, func(t *testing.T) {
				client := newDraftTestClient(t, map[string]draftTestRoute{
					wrapper.path: {method: wrapper.method, status: answer.status, body: answer.body},
				})

				sent, err := wrapper.send(client)
				if err != nil {
					t.Fatalf("a delivered message must not answer an error, got %v", err)
				}
				if sent == nil || *sent != (generated.SentMessage{}) {
					t.Errorf("sent = %+v, want an empty answer", sent)
				}
			})
		}
	}
}

func TestSendingWrappersKeepTheErrorOfARefusedDelivery(t *testing.T) {
	for _, wrapper := range sendingWrappers() {
		t.Run(wrapper.name, func(t *testing.T) {
			client := newDraftTestClient(t, map[string]draftTestRoute{
				wrapper.path: {method: wrapper.method, status: 404, body: `{"error":"not found"}`, requestID: "req-sent-404"},
			})

			sent, err := wrapper.send(client)
			if sent != nil {
				t.Errorf("sent = %+v, want nil for a refused delivery", sent)
			}
			e := AsError(err)
			if e == nil || e.Code != CodeNotFound || e.RequestID != "req-sent-404" {
				t.Fatalf("expected a not-found error carrying the request id, got %#v", err)
			}
		})
	}
}
