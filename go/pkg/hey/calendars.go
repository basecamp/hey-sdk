package hey

import (
	"context"
	"fmt"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CalendarsService handles calendar operations.
type CalendarsService struct {
	client *Client
}

// NewCalendarsService creates a new CalendarsService.
func NewCalendarsService(client *Client) *CalendarsService {
	return &CalendarsService{client: client}
}

// List returns all calendars.
func (s *CalendarsService) List(ctx context.Context) (result *generated.CalendarListPayload, err error) {
	op := OperationInfo{
		Service: "Calendars", Operation: "ListCalendars",
		ResourceType: "calendar", IsMutation: false,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	s.client.initGeneratedClient()
	resp, err := s.client.gen.ListCalendarsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Toggle switches a calendar in or out of the identity's selection and returns the ids the
// selection is left holding. The selection is what CalendarPeriodsService reads are scoped
// to, so this is how a client changes which calendars a day, week or year is drawn from.
func (s *CalendarsService) Toggle(ctx context.Context, calendarID int64) (selectedIDs []int64, err error) {
	op := OperationInfo{
		Service: "Calendars", Operation: "ToggleCalendar",
		ResourceType: "calendar", IsMutation: true, ResourceID: calendarID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ToggleCalendarWithResponse(ctx, calendarID)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 != nil {
			selectedIDs = resp.JSON200.SelectedCalendarIds
		}
		return nil
	})
	return selectedIDs, err
}

// ListedCalendar is a calendar as the calendars list serves it, wrapped with what a live
// follower needs: RecordingChangesURL is where the calendar's recording changes feed
// starts (read it with CalendarChangesCursorFrom), and SignedStreamName subscribes the
// calendar's stream over Action Cable — a frame arriving there means the calendar
// changed, and the name is stable for the calendar's life. The level-1 changes feed's
// added bucket carries the same shape, so a calendar learned of either way arrives
// subscribable.
type ListedCalendar struct {
	Calendar            generated.Calendar `json:"calendar"`
	RecordingChangesURL string             `json:"recording_changes_url"`
	SignedStreamName    string             `json:"signed_stream_name"`
}

// CalendarList is the full calendars index: every calendar with its changes URL and
// signed stream name, the calendar-level changes feed's own URL, and which calendars the
// user has selected for display.
type CalendarList struct {
	Calendars           []ListedCalendar `json:"calendars"`
	CalendarChangesURL  string           `json:"calendar_changes_url"`
	SelectedCalendarIDs []int64          `json:"selected_calendar_ids"`
}

// ListWithChanges returns all calendars along with everything List throws away: each
// calendar's recording changes URL and signed stream name, the calendar changes URL,
// and the selected calendar IDs.
func (s *CalendarsService) ListWithChanges(ctx context.Context) (result *CalendarList, err error) {
	op := OperationInfo{
		Service: "Calendars", Operation: "ListCalendars",
		ResourceType: "calendar", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.Get(ctx, "/calendars.json")
		if rerr != nil {
			return rerr
		}

		list := &CalendarList{}
		if derr := resp.UnmarshalData(list); derr != nil {
			return fmt.Errorf("failed to decode the calendar list: %w", derr)
		}
		result = list
		return nil
	})
	return result, err
}

// GetRecordings returns recordings for a specific calendar.
func (s *CalendarsService) GetRecordings(ctx context.Context, calendarID int64, params *generated.GetCalendarRecordingsParams) (result *generated.CalendarRecordingsResponse, err error) {
	op := OperationInfo{
		Service: "Calendars", Operation: "GetCalendarRecordings",
		ResourceType: "recording", IsMutation: false, ResourceID: calendarID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	s.client.initGeneratedClient()
	resp, err := s.client.gen.GetCalendarRecordingsWithResponse(ctx, calendarID, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
