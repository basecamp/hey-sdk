package hey

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// CalendarEventsService handles calendar event operations.
//
// Calendar events use form-encoded requests because the HEY API does not
// expose JSON endpoints for event mutations (POST /calendar/events.json
// returns 404). Listing events is done through CalendarsService.GetRecordings.
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

// Create creates a new calendar event.
// Returns the ID of the created event.
func (s *CalendarEventsService) Create(ctx context.Context, params CreateCalendarEventParams) (id int64, err error) {
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

	resp, err := s.client.PostForm(ctx, "/calendar/events", values)
	if err != nil {
		return 0, err
	}
	return resp.ExtractID()
}

// Update updates an existing calendar event.
func (s *CalendarEventsService) Update(ctx context.Context, eventID int64, params UpdateCalendarEventParams) (err error) {
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

	_, err = s.client.PatchForm(ctx, fmt.Sprintf("/calendar/events/%d", eventID), values)
	return err
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
