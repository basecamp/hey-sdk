package hey

import (
	"testing"
)

func TestRouterMatchPath(t *testing.T) {
	r := DefaultRouter()

	tests := []struct {
		name     string
		input    string
		wantNil  bool
		wantOp   string
		wantRes  string
		wantRsrc string
	}{
		{
			name:     "list boxes",
			input:    "/boxes",
			wantOp:   "ListBoxes",
			wantRsrc: "Boxes",
		},
		{
			name:     "get box by ID",
			input:    "/boxes/123",
			wantOp:   "GetBox",
			wantRes:  "123",
			wantRsrc: "Boxes",
		},
		{
			name:     "get box with .json suffix",
			input:    "/boxes/123.json",
			wantOp:   "GetBox",
			wantRes:  "123",
			wantRsrc: "Boxes",
		},
		{
			name:     "get topic",
			input:    "/topics/456",
			wantOp:   "GetTopic",
			wantRes:  "456",
			wantRsrc: "Topics",
		},
		{
			name:     "topic entries",
			input:    "/topics/456/entries",
			wantOp:   "GetTopicEntries",
			wantRsrc: "Topics",
		},
		{
			name:     "get message",
			input:    "/messages/789",
			wantOp:   "GetMessage",
			wantRes:  "789",
			wantRsrc: "Messages",
		},
		{
			name:     "get contact",
			input:    "/contacts/42",
			wantOp:   "GetContact",
			wantRes:  "42",
			wantRsrc: "Contacts",
		},
		{
			name:     "imbox",
			input:    "/imbox",
			wantOp:   "GetImbox",
			wantRsrc: "Boxes",
		},
		{
			name:     "identity",
			input:    "/identity",
			wantOp:   "GetIdentity",
			wantRsrc: "Identity",
		},
		{
			name:     "journal entry",
			input:    "/calendar/days/2025-01-15/journal_entry",
			wantOp:   "GetJournalEntry",
			wantRsrc: "Calendar Journal",
		},
		{
			name:     "time track update",
			input:    "/calendar/time_tracks/999",
			wantOp:   "UpdateTimeTrack",
			wantRsrc: "Calendar Time Tracks",
		},
		{
			name:     "calendar habit create",
			input:    "/calendar/habits",
			wantOp:   "CreateHabit",
			wantRsrc: "Calendar Habits",
		},
		{
			name:     "calendar habit delete",
			input:    "/calendar/habits/123",
			wantOp:   "DeleteHabit",
			wantRes:  "123",
			wantRsrc: "Calendar Habits",
		},
		{
			name:     "search",
			input:    "/search",
			wantOp:   "Search",
			wantRsrc: "Search",
		},
		{
			name:    "unknown path",
			input:   "/nonexistent/path",
			wantNil: true,
		},
		{
			name:    "empty path",
			input:   "/",
			wantNil: true,
		},
		{
			name:     "trailing slash stripped",
			input:    "/boxes/",
			wantOp:   "ListBoxes",
			wantRsrc: "Boxes",
		},
		{
			name:     "json suffix with trailing slash",
			input:    "/boxes/123.json/",
			wantOp:   "GetBox",
			wantRes:  "123",
			wantRsrc: "Boxes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := r.MatchPath(tt.input)
			if tt.wantNil {
				if m != nil {
					t.Errorf("MatchPath(%q) = %+v, want nil", tt.input, m)
				}
				return
			}
			if m == nil {
				t.Fatalf("MatchPath(%q) = nil, want non-nil", tt.input)
			}
			if tt.wantOp != "" && m.Operation != tt.wantOp {
				t.Errorf("Operation = %q, want %q", m.Operation, tt.wantOp)
			}
			if tt.wantRes != "" && m.ResourceID() != tt.wantRes {
				t.Errorf("ResourceID() = %q, want %q", m.ResourceID(), tt.wantRes)
			}
			if tt.wantRsrc != "" && m.Resource != tt.wantRsrc {
				t.Errorf("Resource = %q, want %q", m.Resource, tt.wantRsrc)
			}
		})
	}
}

func TestResourceIDNil(t *testing.T) {
	var m *Match
	if m.ResourceID() != "" {
		t.Errorf("nil Match.ResourceID() = %q, want empty", m.ResourceID())
	}
}

func TestRouterOperations(t *testing.T) {
	r := DefaultRouter()

	// Calendar todo completions should have both POST and DELETE
	m := r.MatchPath("/calendar/todos/123/completions")
	if m == nil {
		t.Fatal("expected match for completions path")
	}
	if _, ok := m.Operations["POST"]; !ok {
		t.Error("expected POST operation (CompleteCalendarTodo)")
	}
	if _, ok := m.Operations["DELETE"]; !ok {
		t.Error("expected DELETE operation (UncompleteCalendarTodo)")
	}
}
