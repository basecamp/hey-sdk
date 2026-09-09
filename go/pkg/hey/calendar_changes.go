package hey

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CalendarChangesCursor is where a read of a calendar changes feed starts — either the
// calendar-level feed behind a CalendarList's CalendarChangesURL or a calendar's own
// recording feed behind its RecordingChangesURL; both speak the same cursor. Since is an
// ISO 8601 timestamp with milliseconds and is exclusive; Version is the contract version
// the caller speaks. The two server-issued URLs differ: a recording_changes_url carries
// v=1, which the recording feed requires, while a calendar_changes_url carries no version
// at all. The server issues the pair in those URLs to begin with — read them with
// CalendarChangesCursorFrom rather than picking the query apart.
type CalendarChangesCursor struct {
	Since   string
	Version string
	Page    string
	PerPage string
}

// CalendarChangesCursorFrom reads a cursor out of a changes URL the server issued: a
// CalendarList's CalendarChangesURL, a calendar's RecordingChangesURL, or a Link header
// either feed answered with. The recording feed refuses a cursor without the version the
// server put in its URL, so this is the only sound way to build one for it.
func CalendarChangesCursorFrom(changesURL string) (CalendarChangesCursor, error) {
	parsed, err := url.Parse(changesURL)
	if err != nil {
		return CalendarChangesCursor{}, fmt.Errorf("failed to read changes URL %q: %w", changesURL, err)
	}

	query := parsed.Query()
	return CalendarChangesCursor{
		Since:   query.Get("since"),
		Version: query.Get("v"),
		Page:    query.Get("page"),
		PerPage: query.Get("per_page"),
	}, nil
}

// query renders the cursor for a request path. The version is never invented here: a
// cursor read from a server-issued URL carries whichever version the server speaks.
func (c CalendarChangesCursor) query() string {
	values := url.Values{}
	values.Set("since", c.Since)
	if c.Version != "" {
		values.Set("v", c.Version)
	}
	if c.Page != "" {
		values.Set("page", c.Page)
	}
	if c.PerPage != "" {
		values.Set("per_page", c.PerPage)
	}
	return values.Encode()
}

// DeletedCalendar is a calendar the changes feed reports gone.
type DeletedCalendar struct {
	ID        int64     `json:"id"`
	DeletedAt time.Time `json:"deleted_at"`
}

// CalendarChanges is everything that happened to the calendar list since a cursor.
// Added calendars arrive as ListedCalendar, so a new calendar comes with the changes URL
// and signed stream name a live follower needs.
//
// NextPage is set while this increment has more pages to read now. NextCursor is set on
// the last page and is where the next read should resume; it is nil when nothing changed,
// in which case the cursor that produced this page still stands. Unlike the recording
// feed, this feed never falls too far behind, so there is no FullSyncRequired here.
type CalendarChanges struct {
	Added      []ListedCalendar
	Updated    []generated.Calendar
	Deleted    []DeletedCalendar
	NextPage   *CalendarChangesCursor
	NextCursor *CalendarChangesCursor
}

// AllCalendarChanges reads the calendar changes feed from a cursor to its end.
func (s *CalendarsService) AllCalendarChanges(ctx context.Context, cursor CalendarChangesCursor) (*CalendarChanges, error) {
	all := &CalendarChanges{}

	for page := 1; page <= s.client.httpOpts.MaxPages; page++ {
		changes, err := s.CalendarChanges(ctx, cursor)
		if err != nil {
			return nil, err
		}

		all.Added = append(all.Added, changes.Added...)
		all.Updated = append(all.Updated, changes.Updated...)
		all.Deleted = append(all.Deleted, changes.Deleted...)
		all.NextCursor = changes.NextCursor

		if changes.NextPage == nil {
			return all, nil
		}
		cursor = *changes.NextPage
	}

	s.client.logger.Warn("calendar changes pagination capped", "maxPages", s.client.httpOpts.MaxPages)
	return all, nil
}

// CalendarChanges returns one page of the calendar changes feed.
func (s *CalendarsService) CalendarChanges(ctx context.Context, cursor CalendarChangesCursor) (result *CalendarChanges, err error) {
	if cursor.Since == "" {
		return nil, ErrUsage("a since cursor is required — start from the list's calendar_changes_url")
	}

	op := OperationInfo{
		Service: "Calendars", Operation: "GetCalendarChanges",
		ResourceType: "calendar", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		path := "/calendar/changes.json?" + cursor.query()
		requested, berr := s.client.buildURL(path)
		if berr != nil {
			return berr
		}

		// Cursor URLs never repeat, so a cached response would never be revalidated —
		// a long-running watch would grow the cache one dead entry per read.
		resp, rerr := s.client.Get(contextWithoutCache(ctx), path)
		if rerr != nil {
			return rerr
		}

		var payload struct {
			Added   []ListedCalendar     `json:"added"`
			Updated []generated.Calendar `json:"updated"`
			Deleted []DeletedCalendar    `json:"deleted"`
		}
		if derr := resp.UnmarshalData(&payload); derr != nil {
			return fmt.Errorf("failed to decode the calendar changes: %w", derr)
		}

		changes := &CalendarChanges{
			Added:   payload.Added,
			Updated: payload.Updated,
			Deleted: payload.Deleted,
		}

		var nerr error
		changes.NextPage, changes.NextCursor, nerr = nextCalendarChangesCursor(requested, resp.Headers)
		if nerr != nil {
			return nerr
		}

		result = changes
		return nil
	})
	return result, err
}

// DeletedRecording is a recording the changes feed reports gone. Type is the recordable
// type key the recording was grouped under while it existed.
type DeletedRecording struct {
	ID        int64     `json:"id"`
	DeletedAt time.Time `json:"deleted_at"`
	Type      string    `json:"type"`
}

// RecordingChanges is everything that happened to a calendar's recordings since a cursor.
// Added and Updated keep the wire's grouping by recordable type key — "Calendar::Event",
// "Calendar::Habit", "Calendar::Habit::Completion", "Calendar::DayTitle",
// "Calendar::DayBackground", "Calendar::TimeTrack", "Calendar::Todo",
// "Calendar::Countdown", "Calendar::JournalEntry" — the server owns that vocabulary.
// Deleted is one deduplicated slice: the wire groups deletions by type key too, but it
// repeats the whole deleted collection under every key it groups, so the map shape carries
// no information beyond each record's own Type — which is authoritative here.
//
// NextPage is set while this increment has more pages to read now. NextCursor is set on
// the last page and is where the next read should resume; it is nil when nothing changed,
// in which case the cursor that produced this page still stands. FullSyncRequired is set
// when the cursor is too far behind for an increment to carry the difference — or speaks
// a version the feed no longer does — and the calendar has to be read in full instead.
type RecordingChanges struct {
	Added            map[string][]generated.Recording
	Updated          map[string][]generated.Recording
	Deleted          []DeletedRecording
	NextPage         *CalendarChangesCursor
	NextCursor       *CalendarChangesCursor
	FullSyncRequired bool
}

// AllRecordingChanges reads a calendar's recording changes feed from a cursor to its end.
func (s *CalendarsService) AllRecordingChanges(ctx context.Context, calendarID int64, cursor CalendarChangesCursor) (*RecordingChanges, error) {
	all := &RecordingChanges{}

	for page := 1; page <= s.client.httpOpts.MaxPages; page++ {
		changes, err := s.RecordingChanges(ctx, calendarID, cursor)
		if err != nil {
			return nil, err
		}
		if changes.FullSyncRequired {
			return changes, nil
		}

		all.Added = mergeRecordingBuckets(all.Added, changes.Added)
		all.Updated = mergeRecordingBuckets(all.Updated, changes.Updated)
		all.Deleted = append(all.Deleted, changes.Deleted...)
		all.NextCursor = changes.NextCursor

		if changes.NextPage == nil {
			return all, nil
		}
		cursor = *changes.NextPage
	}

	s.client.logger.Warn("recording changes pagination capped", "maxPages", s.client.httpOpts.MaxPages)
	return all, nil
}

// RecordingChanges returns one page of a calendar's recording changes feed.
func (s *CalendarsService) RecordingChanges(ctx context.Context, calendarID int64, cursor CalendarChangesCursor) (result *RecordingChanges, err error) {
	if cursor.Since == "" {
		return nil, ErrUsage("a since cursor is required — start from the calendar's recording_changes_url")
	}
	// Without the version the server answers 409 forever, which would read as an endless
	// full-sync loop rather than the usage error it is.
	if cursor.Version == "" {
		return nil, ErrUsage("a feed version is required — parse the calendar's recording_changes_url with CalendarChangesCursorFrom")
	}

	op := OperationInfo{
		Service: "Calendars", Operation: "GetCalendarRecordingChanges",
		ResourceType: "recording", IsMutation: false, ResourceID: calendarID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		path := fmt.Sprintf("/calendars/%d/recording/changes.json?%s", calendarID, cursor.query())
		requested, berr := s.client.buildURL(path)
		if berr != nil {
			return berr
		}

		// Cursor URLs never repeat, so a cached response would never be revalidated —
		// a long-running watch would grow the cache one dead entry per read.
		resp, rerr := s.client.Get(contextWithoutCache(ctx), path)
		if rerr != nil {
			// Too far behind for an increment, or a feed version mismatch: the server says
			// so with a 409, and the answer to both is a full read and a fresh cursor.
			if AsError(rerr).HTTPStatus == http.StatusConflict {
				result = &RecordingChanges{FullSyncRequired: true}
				return nil
			}
			return rerr
		}

		var payload struct {
			Added   map[string][]generated.Recording `json:"added"`
			Updated map[string][]generated.Recording `json:"updated"`
			Deleted map[string][]DeletedRecording    `json:"deleted"`
		}
		if derr := resp.UnmarshalData(&payload); derr != nil {
			return fmt.Errorf("failed to decode the recording changes: %w", derr)
		}

		changes := &RecordingChanges{
			Added:   payload.Added,
			Updated: payload.Updated,
			Deleted: flattenDeletedRecordings(payload.Deleted),
		}

		var nerr error
		changes.NextPage, changes.NextCursor, nerr = nextCalendarChangesCursor(requested, resp.Headers)
		if nerr != nil {
			return nerr
		}

		result = changes
		return nil
	})
	return result, err
}

func mergeRecordingBuckets(into, from map[string][]generated.Recording) map[string][]generated.Recording {
	if len(from) == 0 {
		return into
	}
	if into == nil {
		into = map[string][]generated.Recording{}
	}
	for key, recordings := range from {
		into[key] = append(into[key], recordings...)
	}
	return into
}

// flattenDeletedRecordings folds the wire's per-type deleted buckets into one slice. The
// server repeats the whole deleted collection under every type key it groups, so the same
// deletion arrives once per key; the ID dedupe removes the repeats, and each record's own
// Type says what it was.
func flattenDeletedRecordings(buckets map[string][]DeletedRecording) []DeletedRecording {
	var deleted []DeletedRecording
	seen := map[int64]bool{}
	for _, key := range slices.Sorted(maps.Keys(buckets)) {
		for _, record := range buckets[key] {
			if !seen[record.ID] {
				seen[record.ID] = true
				deleted = append(deleted, record)
			}
		}
	}
	return deleted
}

// nextCalendarChangesCursor reads the feed's Link header: while an increment has more
// pages the link carries a page cursor, and the last page carries a fresh since cursor
// instead. Both are nil when there is no link — the caller's cursor still stands.
func nextCalendarChangesCursor(requestedURL string, headers http.Header) (page, since *CalendarChangesCursor, err error) {
	link := parseNextLink(headers.Get("Link"))
	if link == "" {
		return nil, nil, nil
	}

	next := resolveURL(requestedURL, link)
	if !isSameOrigin(next, requestedURL) {
		return nil, nil, fmt.Errorf("changes Link header points to a different origin: %s", next)
	}

	cursor, cerr := CalendarChangesCursorFrom(next)
	if cerr != nil {
		return nil, nil, cerr
	}
	if cursor.Page != "" {
		return &cursor, nil, nil
	}
	return nil, &cursor, nil
}
