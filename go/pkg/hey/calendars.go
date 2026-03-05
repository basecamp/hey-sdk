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
