package hey

import (
	"context"
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
