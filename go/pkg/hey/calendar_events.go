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
	// TimeZone is the IANA timezone name (e.g., "America/New_York").
	// Required for timed events.
	TimeZone string
	// Reminders is a list of durations before the event to send reminders.
	// The API accepts any duration, not just the presets in the web UI.
	Reminders []time.Duration
}

// UpdateCalendarEventParams contains the parameters for updating a calendar event.
// Only non-nil fields are updated.
type UpdateCalendarEventParams struct {
	Title     *string
	StartsAt  *string
	EndsAt    *string
	AllDay    *bool
	StartTime *string
	EndTime   *string
	TimeZone  *string
	Reminders []time.Duration
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
		values.Set("calendar_event[starts_at_time_zone_name]", params.TimeZone)
		values.Set("calendar_event[ends_at_time_zone_name]", params.TimeZone)
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

// recordingFromData reads the recording a JSON write answers with. A server without the JSON
// branch redirects to an HTML page instead, which leaves nothing to hand back.
func recordingFromData(data []byte) *generated.Recording {
	recording := &generated.Recording{}
	if err := json.Unmarshal(data, recording); err != nil || recording.Id == 0 {
		return nil
	}
	return recording
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
	if params.TimeZone != nil {
		values.Set("calendar_event[starts_at_time_zone_name]", *params.TimeZone)
		values.Set("calendar_event[ends_at_time_zone_name]", *params.TimeZone)
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
