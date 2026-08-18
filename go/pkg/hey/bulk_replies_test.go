package hey

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

func TestBulkRepliesService_Draft(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/bulk_replies/new.json" {
			t.Errorf("expected GET /bulk_replies/new.json, got %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("posting_ids")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"<div>Rob</div>","entries":[
			{"id":11,"topic_id":21,"topic_name":"Life insurance","addressed":{"directly":[{"id":31,"email_address":"a@example.com"}]}},
			{"id":12,"topic_id":22,"topic_name":"Book cover","addressed":{"directly":[{"id":32,"email_address":"b@example.com"}]}}]}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	draft, err := client.BulkReplies().Draft(context.Background(), []int64{101, 102})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "101,102" {
		t.Errorf("expected the postings as a comma separated list, got %q", gotQuery)
	}
	if draft == nil || len(draft.Entries) != 2 {
		t.Fatalf("expected two entries, got %+v", draft)
	}
	if draft.Entries[0].TopicName != "Life insurance" {
		t.Errorf("expected the thread name, got %q", draft.Entries[0].TopicName)
	}
	if len(draft.Entries[0].Addressed.Directly) != 1 {
		t.Errorf("expected the recipients for the thread, got %+v", draft.Entries[0].Addressed)
	}
	if draft.Content == "" {
		t.Error("expected the prefilled content")
	}
}

// The postings are what a caller holds; without them there is nothing to resolve, and the
// server would answer every thread in the box.
func TestBulkRepliesService_DraftRequiresPostings(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://example.invalid"}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	_, err := client.BulkReplies().Draft(context.Background(), nil)

	var heyErr *Error
	if !errors.As(err, &heyErr) || heyErr.Code != CodeUsage {
		t.Fatalf("expected a usage error, got %v", err)
	}
}

func TestBulkRepliesService_Send(t *testing.T) {
	var sent generated.BulkReplyRequestContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bulk_replies.json" {
			t.Errorf("expected POST /bulk_replies.json, got %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decoding the request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":9,"entries_count":2,"delayed":true,"undo_send_url":"https://app.hey.com/bulk_replies/9/undo_send"}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	delivery, err := client.BulkReplies().Send(context.Background(), []int64{11, 12}, "<div>On my way</div>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sent.EntryIds) != 2 || sent.EntryIds[0] != 11 {
		t.Errorf("expected the entries, got %v", sent.EntryIds)
	}
	if sent.Message.Content != "<div>On my way</div>" {
		t.Errorf("expected the content, got %q", sent.Message.Content)
	}
	if delivery == nil || delivery.EntriesCount != 2 || !delivery.Delayed {
		t.Fatalf("expected a delayed delivery of two replies, got %+v", delivery)
	}

	// A caller holding the URL rather than the delivery can still undo it.
	id, err := UndoSendID(delivery.UndoSendUrl)
	if err != nil || id != 9 {
		t.Errorf("expected bulk reply 9 from %q, got %d (%v)", delivery.UndoSendUrl, id, err)
	}
}

func TestBulkRepliesService_SendRequiresEntries(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://example.invalid"}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	_, err := client.BulkReplies().Send(context.Background(), nil, "content")

	var heyErr *Error
	if !errors.As(err, &heyErr) || heyErr.Code != CodeUsage {
		t.Fatalf("expected a usage error, got %v", err)
	}
}

// Undo answers a redirect, not JSON — the SDK follows it rather than decoding a body.
func TestBulkRepliesService_Undo(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			path = r.URL.Path
			w.Header().Set("Location", "/bulk_replies/9")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

	if err := client.BulkReplies().Undo(context.Background(), 9); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/bulk_replies/9/undo_send" {
		t.Errorf("expected the undo path, got %q", path)
	}
}

func TestUndoSendIDRejectsOtherURLs(t *testing.T) {
	for _, candidate := range []string{"https://app.hey.com/topics/9", "not a url at all: %", ""} {
		if _, err := UndoSendID(candidate); err == nil {
			t.Errorf("expected %q to be rejected", candidate)
		}
	}
}
