package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CalendarEventsService handles calendar event operations.
//
// Calendar events take form-encoded bodies because that is the shape the HEY endpoints
// parse. Create posts to the .json path, so a current server answers the created recording;
// a server without the JSON branch redirects instead and only the id comes back.
// Listing events is done through CalendarsService.GetRecordings.
type CalendarEventsService struct {
	client *Client
}

// NewCalendarEventsService creates a new CalendarEventsService.
func NewCalendarEventsService(client *Client) *CalendarEventsService {
	return &CalendarEventsService{client: client}
}

// CreateCalendarEventParams contains the parameters for creating a calendar event.
type CreateCalendarEventParams struct {
	// CalendarID is the ID of the calendar to create the event in.
	CalendarID int64
	// Title is the event summary/title.
	Title string
	// StartsAt is the start date in YYYY-MM-DD format.
	StartsAt string
	// EndsAt is the end date in YYYY-MM-DD format. Defaults to StartsAt if empty.
	EndsAt string
	// AllDay indicates whether this is an all-day event.
	AllDay bool
	// StartTime is the start time in HH:MM format (required if not all-day).
	StartTime string
	// EndTime is the end time in HH:MM format (required if not all-day).
	EndTime string
	// StartTimeZone and EndTimeZone are the IANA names of the zones the clock times above are
	// written in — "Europe/Zagreb", "America/New_York". Leave them empty and the times are
	// read in UTC, which is the zone HEY parses an API request in. HEY keeps a zone per end,
	// as its own form offers, so an event can start in one and finish in another.
	StartTimeZone string
	EndTimeZone   string
	// TimeZone names one zone for both ends.
	//
	// Deprecated: use StartTimeZone and EndTimeZone. It stands in for whichever of them is
	// empty, so a caller that only ever wanted one zone keeps working.
	TimeZone string
	// Reminders is a list of durations before the event to send reminders.
	// The API accepts any duration, not just the presets in the web UI.
	Reminders []time.Duration
}

// UpdateCalendarEventParams contains the parameters for updating a calendar event.
// Only non-nil fields are updated.
//
// The zones are the exception, and the reason is on HEY's side: it reads the pair out of the
// submitted parameters or nils both, so an update that says nothing about them clears whatever
// the event had. A caller keeping a zoned event's zones has to send them again.
type UpdateCalendarEventParams struct {
	// CalendarID moves the event to another calendar. An update takes the same calendar as a
	// create does, which is how the web app's calendar select relocates an event.
	//
	// It has to be a calendar the identity can file on — one it owns or shares, and not a
	// subscription. The personal calendar is the one that catches you out: it is in the list
	// Identity serves, and filing on it answers 404 all the same.
	CalendarID *int64
	Title      *string
	StartsAt   *string
	EndsAt     *string
	AllDay     *bool
	StartTime  *string
	EndTime    *string
	// StartTimeZone and EndTimeZone are the zones the clock times are written in, as on a
	// create. Empty strings say the times are UTC and clear the zones the event was saved
	// with; nil leaves them out of the request, which HEY also reads as clearing them.
	StartTimeZone *string
	EndTimeZone   *string
	// TimeZone names one zone for both ends.
	//
	// Deprecated: use StartTimeZone and EndTimeZone. It stands in for whichever of them is
	// nil, so a caller that only ever wanted one zone keeps working.
	TimeZone  *string
	Reminders []time.Duration
}

// setTimeZones writes the zones a timed event's clock times are written in, and the flag that
// makes HEY honour them. The flag is not decoration: without it both names are dropped and the
// times are read in UTC, so 08:00 sent from Zagreb is stored as 08:00Z.
//
// Naming no zone is a complete answer rather than an omission — convert to UTC and say nothing.
// An all-day event does not come through here at all, since a date has no zone.
// timeZoneOr and timeZonePointerOr let the deprecated TimeZone stand in for an end the caller
// did not name.
func timeZoneOr(zone, both string) string {
	if zone == "" {
		return both
	}
	return zone
}

func timeZonePointerOr(zone, both *string) *string {
	if zone == nil {
		return both
	}
	return zone
}

func setTimeZones(values url.Values, start, end string) {
	if start == "" && end == "" {
		values.Set("calendar_event[set_time_zone]", "0")
		return
	}
	// An empty name is read as no name, which lands that end back in UTC.
	if start == "" {
		start = end
	}
	if end == "" {
		end = start
	}
	values.Set("calendar_event[set_time_zone]", "1")
	values.Set("calendar_event[starts_at_time_zone_name]", start)
	values.Set("calendar_event[ends_at_time_zone_name]", end)
}

// Create creates a new calendar event and returns it as a recording.
//
// A server carrying the JSON create branch answers 201 with the whole recording. An older
// one redirects to the event instead, and the result then carries only the id.
func (s *CalendarEventsService) Create(ctx context.Context, params CreateCalendarEventParams) (recording *generated.Recording, err error) {
	op := OperationInfo{
		Service: "CalendarEvents", Operation: "CreateCalendarEvent",
		ResourceType: "calendar_event", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	if params.EndsAt == "" {
		params.EndsAt = params.StartsAt
	}

	values := url.Values{}
	values.Set("calendar_event[calendar_id]", fmt.Sprintf("%d", params.CalendarID))
	values.Set("calendar_event[summary]", params.Title)
	values.Set("calendar_event[starts_at]", params.StartsAt)
	values.Set("calendar_event[ends_at]", params.EndsAt)

	if params.AllDay {
		values.Set("calendar_event[all_day]", "1")
		for _, r := range params.Reminders {
			values.Add("all_day_reminder_durations[]", fmt.Sprintf("%d", int64(r.Seconds())))
		}
	} else {
		values.Set("calendar_event[all_day]", "0")
		values.Set("calendar_event[starts_at_time]", params.StartTime+":00")
		values.Set("calendar_event[ends_at_time]", params.EndTime+":00")
		setTimeZones(values,
			timeZoneOr(params.StartTimeZone, params.TimeZone),
			timeZoneOr(params.EndTimeZone, params.TimeZone))
		for _, r := range params.Reminders {
			values.Add("timed_reminder_durations[]", fmt.Sprintf("%d", int64(r.Seconds())))
		}
	}

	resp, err := s.client.PostForm(ctx, "/calendar/events.json", values)
	if err != nil {
		return nil, err
	}
	return recordingFromFormResponse(resp)
}

// recordingFromFormResponse reads the recording a JSON write answers with, falling back to
// the redirect an older server sends, whose URL still carries the recording's id.
func recordingFromFormResponse(resp *FormResponse) (*generated.Recording, error) {
	if len(resp.Body) > 0 {
		recording := &generated.Recording{}
		if err := json.Unmarshal([]byte(resp.Body), recording); err != nil {
			return nil, fmt.Errorf("failed to decode the recording: %w", err)
		}
		return recording, nil
	}

	id, err := resp.ExtractID()
	if err != nil {
		return nil, err
	}
	return &generated.Recording{Id: id}, nil
}

// Update updates an existing calendar event and returns it as a recording.
//
// A server carrying the JSON update branch answers 200 with the whole recording. An older
// one redirects to the event instead, and the result then carries only the id.
func (s *CalendarEventsService) Update(ctx context.Context, eventID int64, params UpdateCalendarEventParams) (recording *generated.Recording, err error) {
	op := OperationInfo{
		Service: "CalendarEvents", Operation: "UpdateCalendarEvent",
		ResourceType: "calendar_event", IsMutation: true, ResourceID: eventID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	values := url.Values{}
	if params.Title != nil {
		values.Set("calendar_event[summary]", *params.Title)
	}
	if params.StartsAt != nil {
		values.Set("calendar_event[starts_at]", *params.StartsAt)
	}
	if params.EndsAt != nil {
		values.Set("calendar_event[ends_at]", *params.EndsAt)
	}
	if params.AllDay != nil {
		if *params.AllDay {
			values.Set("calendar_event[all_day]", "1")
		} else {
			values.Set("calendar_event[all_day]", "0")
		}
	}
	if params.StartTime != nil {
		values.Set("calendar_event[starts_at_time]", *params.StartTime+":00")
	}
	if params.EndTime != nil {
		values.Set("calendar_event[ends_at_time]", *params.EndTime+":00")
	}
	if params.CalendarID != nil {
		values.Set("calendar_event[calendar_id]", fmt.Sprintf("%d", *params.CalendarID))
	}
	startZone := timeZonePointerOr(params.StartTimeZone, params.TimeZone)
	endZone := timeZonePointerOr(params.EndTimeZone, params.TimeZone)
	if startZone != nil || endZone != nil {
		var start, end string
		if startZone != nil {
			start = *startZone
		}
		if endZone != nil {
			end = *endZone
		}
		setTimeZones(values, start, end)
	}
	if params.Reminders != nil {
		allDay := params.AllDay != nil && *params.AllDay
		key := "timed_reminder_durations[]"
		if allDay {
			key = "all_day_reminder_durations[]"
		}
		for _, r := range params.Reminders {
			values.Add(key, fmt.Sprintf("%d", int64(r.Seconds())))
		}
	}

	resp, err := s.client.PatchForm(ctx, fmt.Sprintf("/calendar/events/%d.json", eventID), values)
	if err != nil {
		return nil, err
	}
	return recordingFromFormResponse(resp)
}

// Delete deletes a calendar event.
func (s *CalendarEventsService) Delete(ctx context.Context, eventID int64) (err error) {
	op := OperationInfo{
		Service: "CalendarEvents", Operation: "DeleteCalendarEvent",
		ResourceType: "calendar_event", IsMutation: true, ResourceID: eventID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	_, err = s.client.DeleteForm(ctx, fmt.Sprintf("/calendar/events/%d", eventID))
	return err
}
