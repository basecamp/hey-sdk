package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newCalendarChangesTestClient serves the calendar changes feeds with the given handler,
// and answers with the cursors the calendars list would carry.
func newCalendarChangesTestClient(t *testing.T, handler http.HandlerFunc) (c *Client, calendarCursor, recordingCursor CalendarChangesCursor) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c = NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithBaseDelay(time.Millisecond), WithMaxJitter(time.Millisecond))

	calendarCursor, err := CalendarChangesCursorFrom(server.URL + "/calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	recordingCursor, err = CalendarChangesCursorFrom(server.URL + "/calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return c, calendarCursor, recordingCursor
}

func TestCalendarChangesCursorFrom(t *testing.T) {
	cursor, err := CalendarChangesCursorFrom("https://app.hey.com/calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor.Since != "2026-08-18T09:00:00.000Z" {
		t.Errorf("since = %q, want it decoded", cursor.Since)
	}
	if cursor.Version != "1" || cursor.Page != "" {
		t.Errorf("cursor = %+v, want v 1 and no page", cursor)
	}

	cursor, err = CalendarChangesCursorFrom("https://app.hey.com/calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z&page=2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor.Since != "2026-08-18T09:00:00.000Z" || cursor.Page != "2" || cursor.Version != "" {
		t.Errorf("cursor = %+v, want the since and page with no version", cursor)
	}

	if _, err = CalendarChangesCursorFrom("://not a url"); err == nil {
		t.Error("expected an error for a garbage URL")
	}
}

func TestCalendarsService_ListWithChanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"calendars": [
				{"calendar": {"id": 512, "name": "Personal", "personal": true},
				 "recording_changes_url": "https://app.hey.com/calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1",
				 "signed_stream_name": "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhNZyJ9--personal"},
				{"calendar": {"id": 513, "name": "Family"},
				 "recording_changes_url": "https://app.hey.com/calendars/513/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1",
				 "signed_stream_name": "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhNdyJ9--family"}
			],
			"calendar_changes_url": "https://app.hey.com/calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z",
			"selected_calendar_ids": [512, 513]
		}`))
	}))
	t.Cleanup(server.Close)
	c := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithBaseDelay(time.Millisecond), WithMaxJitter(time.Millisecond))

	list, err := c.Calendars().ListWithChanges(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Calendars) != 2 {
		t.Fatalf("calendars = %+v, want two", list.Calendars)
	}
	personal := list.Calendars[0]
	if personal.Calendar.Id != 512 || personal.Calendar.Name != "Personal" || !personal.Calendar.Personal {
		t.Errorf("calendar = %+v, want the personal calendar", personal.Calendar)
	}
	if personal.SignedStreamName != "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhNZyJ9--personal" {
		t.Errorf("signed stream name = %q, want the wrapper's", personal.SignedStreamName)
	}
	if !strings.Contains(personal.RecordingChangesURL, "/calendars/512/recording/changes.json") {
		t.Errorf("recording changes URL = %q, want the calendar's own feed", personal.RecordingChangesURL)
	}
	if !strings.Contains(list.CalendarChangesURL, "/calendar/changes.json") {
		t.Errorf("calendar changes URL = %q, want the list-level feed", list.CalendarChangesURL)
	}
	if len(list.SelectedCalendarIDs) != 2 || list.SelectedCalendarIDs[0] != 512 {
		t.Errorf("selected calendar ids = %v, want both calendars", list.SelectedCalendarIDs)
	}
}

func TestCalendarsService_CalendarChanges(t *testing.T) {
	var requested string
	c, cursor, _ := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<`+r.URL.Path+`?since=2026-08-18T09%3A14%3A22.031Z>; rel="next"`)
		_, _ = w.Write([]byte(`{
			"added": [{"calendar": {"id": 514, "name": "Book Club"},
			           "recording_changes_url": "https://app.hey.com/calendars/514/recording/changes.json?since=2026-08-18T09%3A14%3A00.000Z&v=1",
			           "signed_stream_name": "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhOQSJ9--bookclub"}],
			"updated": [{"id": 512, "name": "Personal", "personal": true}],
			"deleted": [{"id": 511, "deleted_at": "2026-08-18T09:14:00.000Z"}]
		}`))
	})

	changes, err := c.Calendars().CalendarChanges(context.Background(), cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requested != "/calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z" {
		t.Errorf("requested %q, want the feed with the cursor and nothing the cursor lacks", requested)
	}
	if len(changes.Added) != 1 || changes.Added[0].Calendar.Id != 514 {
		t.Fatalf("added = %+v, want calendar 514", changes.Added)
	}
	if changes.Added[0].SignedStreamName != "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhOQSJ9--bookclub" {
		t.Errorf("signed stream name = %q, want the added calendar subscribable", changes.Added[0].SignedStreamName)
	}
	if !strings.Contains(changes.Added[0].RecordingChangesURL, "/calendars/514/recording/changes.json") {
		t.Errorf("recording changes URL = %q, want the added calendar's feed", changes.Added[0].RecordingChangesURL)
	}
	if len(changes.Updated) != 1 || changes.Updated[0].Id != 512 {
		t.Errorf("updated = %+v, want calendar 512", changes.Updated)
	}
	wantDeletedAt := time.Date(2026, 8, 18, 9, 14, 0, 0, time.UTC)
	if len(changes.Deleted) != 1 || changes.Deleted[0].ID != 511 || !changes.Deleted[0].DeletedAt.Equal(wantDeletedAt) {
		t.Errorf("deleted = %+v, want calendar 511 deleted at %v", changes.Deleted, wantDeletedAt)
	}
	if changes.NextPage != nil {
		t.Errorf("next page = %+v, want none: a since link is a cursor, not a page", changes.NextPage)
	}
	if changes.NextCursor == nil || changes.NextCursor.Since != "2026-08-18T09:14:22.031Z" {
		t.Errorf("cursor = %+v, want the since from the Link header", changes.NextCursor)
	}
}

func TestCalendarsService_AllCalendarChangesFollowsPages(t *testing.T) {
	var requested []string
	c, cursor, _ := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<`+r.URL.Path+`?since=2026-08-18T09%3A00%3A00.000Z&page=2>; rel="next"`)
			_, _ = w.Write([]byte(`{"updated":[{"id":512,"name":"Personal"}]}`))
		} else {
			w.Header().Set("Link", `<`+r.URL.Path+`?since=2026-08-18T09%3A30%3A00.000Z>; rel="next"`)
			_, _ = w.Write([]byte(`{"updated":[{"id":513,"name":"Family"}]}`))
		}
	})

	changes, err := c.Calendars().AllCalendarChanges(context.Background(), cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requested) != 2 {
		t.Fatalf("requests = %v, want two pages", requested)
	}
	if !strings.Contains(requested[1], "page=2") {
		t.Errorf("second request = %q, want the page from the Link header", requested[1])
	}
	if len(changes.Updated) != 2 || changes.Updated[0].Id != 512 || changes.Updated[1].Id != 513 {
		t.Errorf("updated = %+v, want both pages", changes.Updated)
	}
	if changes.NextCursor == nil || changes.NextCursor.Since != "2026-08-18T09:30:00.000Z" {
		t.Errorf("cursor = %+v, want the last page's since", changes.NextCursor)
	}
}

func TestCalendarsService_RecordingChanges(t *testing.T) {
	var requested string
	c, _, cursor := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<`+r.URL.Path+`?since=2026-08-18T09%3A14%3A22.031Z&v=1>; rel="next"`)
		_, _ = w.Write([]byte(`{
			"added": {"Calendar::Event": [{"id": 88001, "title": "Dentist appointment"}]},
			"updated": {"Calendar::JournalEntry": [{"id": 88002}]},
			"deleted": {
				"Calendar::Habit": [
					{"id": 88005, "deleted_at": "2026-08-18T09:12:00.000Z", "type": "Calendar::Habit"},
					{"id": 88003, "deleted_at": "2026-08-18T09:14:00.000Z", "type": "Calendar::Todo"}
				],
				"Calendar::Todo": [
					{"id": 88005, "deleted_at": "2026-08-18T09:12:00.000Z", "type": "Calendar::Habit"},
					{"id": 88003, "deleted_at": "2026-08-18T09:14:00.000Z", "type": "Calendar::Todo"}
				]
			}
		}`))
	})

	changes, err := c.Calendars().RecordingChanges(context.Background(), 512, cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requested != "/calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1" {
		t.Errorf("requested %q, want the calendar's own feed with the cursor's since and version", requested)
	}
	added := changes.Added["Calendar::Event"]
	if len(added) != 1 || added[0].Id != 88001 || added[0].Title != "Dentist appointment" {
		t.Errorf("added = %+v, want event 88001 under its type key", changes.Added)
	}
	updated := changes.Updated["Calendar::JournalEntry"]
	if len(updated) != 1 || updated[0].Id != 88002 {
		t.Errorf("updated = %+v, want journal entry 88002 under its type key", changes.Updated)
	}
	if len(changes.Deleted) != 2 {
		t.Fatalf("deleted = %+v, want the wire's repeats deduped down to two", changes.Deleted)
	}
	if changes.Deleted[0].ID != 88005 || changes.Deleted[0].Type != "Calendar::Habit" {
		t.Errorf("deleted[0] = %+v, want habit 88005 with its own type", changes.Deleted[0])
	}
	if changes.Deleted[1].ID != 88003 || changes.Deleted[1].Type != "Calendar::Todo" {
		t.Errorf("deleted[1] = %+v, want todo 88003 with its own type", changes.Deleted[1])
	}
	if changes.NextPage != nil {
		t.Errorf("next page = %+v, want none: a since link is a cursor, not a page", changes.NextPage)
	}
	if changes.NextCursor == nil || changes.NextCursor.Since != "2026-08-18T09:14:22.031Z" {
		t.Errorf("cursor = %+v, want the since from the Link header", changes.NextCursor)
	}
}

func TestCalendarsService_AllRecordingChangesFollowsPages(t *testing.T) {
	var requested []string
	c, _, cursor := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<`+r.URL.Path+`?since=2026-08-18T09%3A00%3A00.000Z&v=1&page=2>; rel="next"`)
			_, _ = w.Write([]byte(`{
				"added": {"Calendar::Event": [{"id": 88001, "title": "Dentist appointment"}]},
				"deleted": {"Calendar::Todo": [{"id": 88003, "deleted_at": "2026-08-18T09:14:00.000Z", "type": "Calendar::Todo"}]}
			}`))
		} else {
			w.Header().Set("Link", `<`+r.URL.Path+`?since=2026-08-18T09%3A30%3A00.000Z&v=1>; rel="next"`)
			_, _ = w.Write([]byte(`{
				"added": {"Calendar::Event": [{"id": 88004, "title": "Parent-teacher conference"}]},
				"deleted": {"Calendar::Habit": [{"id": 88005, "deleted_at": "2026-08-18T09:20:00.000Z", "type": "Calendar::Habit"}]}
			}`))
		}
	})

	changes, err := c.Calendars().AllRecordingChanges(context.Background(), 512, cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requested) != 2 {
		t.Fatalf("requests = %v, want two pages", requested)
	}
	if !strings.Contains(requested[1], "page=2") {
		t.Errorf("second request = %q, want the page from the Link header", requested[1])
	}
	added := changes.Added["Calendar::Event"]
	if len(added) != 2 || added[0].Id != 88001 || added[1].Id != 88004 {
		t.Errorf("added = %+v, want both pages merged under the type key", changes.Added)
	}
	if len(changes.Deleted) != 2 || changes.Deleted[0].ID != 88003 || changes.Deleted[1].ID != 88005 {
		t.Errorf("deleted = %+v, want both pages in read order", changes.Deleted)
	}
	if changes.NextCursor == nil || changes.NextCursor.Since != "2026-08-18T09:30:00.000Z" {
		t.Errorf("cursor = %+v, want the last page's since", changes.NextCursor)
	}
}

func TestCalendarsService_RecordingChangesOnLastPage(t *testing.T) {
	c, _, cursor := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	changes, err := c.Calendars().RecordingChanges(context.Background(), 512, cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes.NextPage != nil || changes.NextCursor != nil {
		t.Errorf("changes = %+v, want no cursors so the caller keeps the one it has", changes)
	}
}

func TestCalendarsService_RecordingChangesTooFarBehind(t *testing.T) {
	var requests int
	c, _, cursor := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusConflict)
	})

	changes, err := c.Calendars().RecordingChanges(context.Background(), 512, cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changes.FullSyncRequired {
		t.Error("a 409 should ask for a full sync rather than error")
	}
	if len(changes.Added)+len(changes.Updated)+len(changes.Deleted) != 0 {
		t.Errorf("changes = %+v, want empty buckets next to the flag", changes)
	}

	changes, err = c.Calendars().AllRecordingChanges(context.Background(), 512, cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changes.FullSyncRequired {
		t.Error("AllRecordingChanges should surface the full-sync flag")
	}
	if requests != 2 {
		t.Errorf("requests = %d, want the loop to stop on the first 409", requests)
	}
}

func TestCalendarsService_ChangesRefusesForeignLink(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://evil.example.com/calendar/changes.json?since=2026-08-18T09%3A14%3A22.031Z>; rel="next"`)
		_, _ = w.Write([]byte(`{}`))
	}

	t.Run("calendar feed", func(t *testing.T) {
		c, cursor, _ := newCalendarChangesTestClient(t, handler)
		if _, err := c.Calendars().CalendarChanges(context.Background(), cursor); err == nil || !strings.Contains(err.Error(), "different origin") {
			t.Fatalf("err = %v, want a refusal naming the origin", err)
		}
	})

	t.Run("recording feed", func(t *testing.T) {
		c, _, cursor := newCalendarChangesTestClient(t, handler)
		if _, err := c.Calendars().RecordingChanges(context.Background(), 512, cursor); err == nil || !strings.Contains(err.Error(), "different origin") {
			t.Fatalf("err = %v, want a refusal naming the origin", err)
		}
	})
}

func TestCalendarsService_ChangesRequiresSince(t *testing.T) {
	c, _, _ := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent without a cursor")
	})

	if _, err := c.Calendars().CalendarChanges(context.Background(), CalendarChangesCursor{}); err == nil {
		t.Fatal("expected error for a calendar cursor with no since")
	}
	if _, err := c.Calendars().RecordingChanges(context.Background(), 512, CalendarChangesCursor{}); err == nil {
		t.Fatal("expected error for a recording cursor with no since")
	}
}

func TestCalendarsService_RecordingChangesRequiresVersion(t *testing.T) {
	c, _, _ := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent without a version — the server would answer 409 forever")
	})

	cursor := CalendarChangesCursor{Since: "2026-08-18T09:00:00.000Z"}
	_, err := c.Calendars().RecordingChanges(context.Background(), 512, cursor)
	if err == nil || !strings.Contains(err.Error(), "recording_changes_url") {
		t.Fatalf("err = %v, want a usage error pointing at the server-issued URL", err)
	}

	if _, err := c.Calendars().AllRecordingChanges(context.Background(), 512, cursor); err == nil {
		t.Fatal("expected AllRecordingChanges to refuse a version-less cursor too")
	}
}
