package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const dayJSON = `{
  "starts_at": "2026-08-22T00:00:00Z",
  "ends_at": "2026-08-22T23:59:59Z",
  "kind": "day",
  "recordings": {
    "CalendarEvent": [
      { "id": 161645836, "type": "CalendarEvent", "title": "Weekly Catchup",
        "starts_at": "2026-08-22T14:00:00Z", "ends_at": "2026-08-22T14:30:00Z",
        "recurring": true, "occurrence_id": "161645836_2026-08-22" }
    ],
    "CalendarTodo": [
      { "id": 90210, "type": "CalendarTodo", "title": "Renew the domain",
        "starts_at": "2026-08-22T00:00:00Z", "ends_at": "2026-08-22T23:59:59Z" }
    ]
  }
}`

func periodsClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))
}

// A day is one period, not a list: the read answers the day itself, with its recordings
// grouped by type the way every other calendar read groups them.
func TestCalendarPeriodsDayAnswersOnePeriod(t *testing.T) {
	var gotPath string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dayJSON))
	})

	day, err := client.CalendarPeriods().Day(context.Background(), "2026-08-22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/calendar/days/2026-08-22.json" {
		t.Errorf("path = %q", gotPath)
	}
	if day.Kind != "day" {
		t.Errorf("kind = %q, want %q", day.Kind, "day")
	}
	if want := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC); !day.StartsAt.Equal(want) {
		t.Errorf("starts_at = %v, want %v", day.StartsAt, want)
	}
	events := day.Recordings["CalendarEvent"]
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	// The occurrence, not the row it recurs from: this is what a period read carries that
	// a calendar's recordings do not.
	if events[0].OccurrenceId != "161645836_2026-08-22" {
		t.Errorf("occurrence_id = %q", events[0].OccurrenceId)
	}
	if todos := day.Recordings["CalendarTodo"]; len(todos) != 1 || todos[0].Title != "Renew the domain" {
		t.Errorf("todos = %+v", todos)
	}
}

// "now" is a date the day read accepts, so a client asking for today does not have to
// decide what today is in the reader's time zone.
func TestCalendarPeriodsDayTakesNow(t *testing.T) {
	var gotPath string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dayJSON))
	})

	if _, err := client.CalendarPeriods().Day(context.Background(), "now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/calendar/days/now.json" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestCalendarPeriodsDaysReadsAWindowFromADate(t *testing.T) {
	var gotPath, gotQuery string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"days":[` + dayJSON + `]}`))
	})

	days, err := client.CalendarPeriods().Days(context.Background(), "2026-08-22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/calendar/days.json" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "starts_at=2026-08-22" {
		t.Errorf("query = %q", gotQuery)
	}
	if len(days) != 1 || days[0].Kind != "day" {
		t.Errorf("days = %+v", days)
	}
}

// An empty date means today, which the server decides. Sending starts_at= would ask it to
// parse an empty string instead.
func TestCalendarPeriodsDaysOmitsAnEmptyDate(t *testing.T) {
	var gotQuery string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"days":[]}`))
	})

	if _, err := client.CalendarPeriods().Days(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want empty", gotQuery)
	}
}

func TestCalendarPeriodsWeekAnswersOnePeriod(t *testing.T) {
	var gotPath string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"starts_at":"2026-08-17T00:00:00Z","ends_at":"2026-08-23T23:59:59Z","kind":"week","recordings":{}}`))
	})

	week, err := client.CalendarPeriods().Week(context.Background(), "2026-08-22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/calendar/weeks/2026-08-22.json" {
		t.Errorf("path = %q", gotPath)
	}
	if week.Kind != "week" {
		t.Errorf("kind = %q, want %q", week.Kind, "week")
	}
}

func TestCalendarPeriodsWeeksCentersOnADate(t *testing.T) {
	var gotPath, gotQuery string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"weeks":[{"starts_at":"2026-08-17T00:00:00Z","ends_at":"2026-08-23T23:59:59Z","kind":"week","recordings":{}}]}`))
	})

	weeks, err := client.CalendarPeriods().Weeks(context.Background(), "", "2026-08-22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/calendar/weeks.json" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "centered_at=2026-08-22" {
		t.Errorf("query = %q", gotQuery)
	}
	if len(weeks) != 1 {
		t.Fatalf("expected 1 week, got %d", len(weeks))
	}
}

// A year is the grid it is drawn as — a day per cell and the events that span more than
// one — rather than every recording the year holds.
func TestCalendarPeriodsYearAnswersTheGrid(t *testing.T) {
	var gotPath string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "starts_at": "2026-01-01T00:00:00Z",
		  "ends_at": "2026-12-31T23:59:59Z",
		  "kind": "year",
		  "padding_days_count": 3,
		  "days": [
		    { "starts_at": "2026-01-01T00:00:00Z", "backgrounded": false },
		    { "starts_at": "2026-01-02T00:00:00Z", "backgrounded": true }
		  ],
		  "spanned_events": [
		    { "id": 5150, "type": "CalendarEvent", "title": "Summer Break", "all_day": true,
		      "starts_at": "2026-07-06T00:00:00Z", "ends_at": "2026-07-17T23:59:59Z" }
		  ]
		}`))
	})

	year, err := client.CalendarPeriods().Year(context.Background(), "2026-08-22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/calendar/years/2026-08-22.json" {
		t.Errorf("path = %q", gotPath)
	}
	if year.Kind != "year" {
		t.Errorf("kind = %q, want %q", year.Kind, "year")
	}
	if year.PaddingDaysCount != 3 {
		t.Errorf("padding_days_count = %d, want 3", year.PaddingDaysCount)
	}
	if len(year.Days) != 2 || !year.Days[1].Backgrounded {
		t.Errorf("days = %+v", year.Days)
	}
	if len(year.SpannedEvents) != 1 || year.SpannedEvents[0].Title != "Summer Break" {
		t.Errorf("spanned_events = %+v", year.SpannedEvents)
	}
}

// A toggle answers the selection it left behind, so a client learns what every period read
// after it will be scoped to without reading the calendars again.
func TestCalendarsToggleAnswersTheSelection(t *testing.T) {
	var gotMethod, gotPath string
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"selected_calendar_ids":[41,42]}`))
	})

	ids, err := client.Calendars().Toggle(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/calendars/42/toggle.json" {
		t.Errorf("path = %q", gotPath)
	}
	if len(ids) != 2 || ids[0] != 41 || ids[1] != 42 {
		t.Errorf("selected ids = %v, want [41 42]", ids)
	}
}

func TestCalendarPeriodsReportNotFound(t *testing.T) {
	client := periodsClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := client.CalendarPeriods().Day(context.Background(), "not-a-date")
	if sdkErr, ok := err.(*Error); !ok || sdkErr.Code != CodeNotFound {
		t.Errorf("err = %v, want %s", err, CodeNotFound)
	}
}
