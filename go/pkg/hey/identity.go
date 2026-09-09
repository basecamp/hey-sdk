package hey

import (
	"context"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// TimeFormat is the clock HEY renders times in.
type TimeFormat string

const (
	TimeFormatTwelveHour     TimeFormat = "twelve_hour"
	TimeFormatTwentyFourHour TimeFormat = "twenty_four_hour"
)

// IdentityService handles identity and navigation operations.
type IdentityService struct {
	client *Client
}

// NewIdentityService creates a new IdentityService.
func NewIdentityService(client *Client) *IdentityService {
	return &IdentityService{client: client}
}

// GetIdentity returns the current user's identity.
func (s *IdentityService) GetIdentity(ctx context.Context) (result *generated.Identity, err error) {
	op := OperationInfo{
		Service: "Identity", Operation: "GetIdentity",
		ResourceType: "identity", IsMutation: false,
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
	resp, err := s.client.gen.GetIdentityWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// UpdateFirstWeekDay sets which day the current identity's calendar weeks start on, and
// returns the day HEY stored.
func (s *IdentityService) UpdateFirstWeekDay(ctx context.Context, day time.Weekday) (result time.Weekday, err error) {
	op := OperationInfo{
		Service: "Identity", Operation: "UpdateFirstWeekDay",
		ResourceType: "identity", IsMutation: true,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.UpdateFirstWeekDayJSONRequestBody{
			IdentityPreference: generated.FirstWeekDayParams{FirstWeekDay: strings.ToLower(day.String())},
		}
		resp, rerr := s.client.genClient().UpdateFirstWeekDayWithResponse(ctx, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = time.Weekday(resp.JSON200.FirstWeekDay)
		return nil
	})
	return result, err
}

// UpdateTimeFormat sets whether HEY renders times on a 12-hour or 24-hour clock, and
// returns the format HEY stored.
func (s *IdentityService) UpdateTimeFormat(ctx context.Context, format TimeFormat) (result TimeFormat, err error) {
	op := OperationInfo{
		Service: "Identity", Operation: "UpdateTimeFormat",
		ResourceType: "identity", IsMutation: true,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.UpdateTimeFormatJSONRequestBody{
			TwentyFourHourTimeFormat: format == TimeFormatTwentyFourHour,
		}
		resp, rerr := s.client.genClient().UpdateTimeFormatWithResponse(ctx, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = TimeFormat(resp.JSON200.TimeFormat)
		return nil
	})
	return result, err
}

// GetNavigation returns the navigation structure for the current user.
func (s *IdentityService) GetNavigation(ctx context.Context) (result *generated.NavigationResponse, err error) {
	op := OperationInfo{
		Service: "Identity", Operation: "GetNavigation",
		ResourceType: "navigation", IsMutation: false,
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
	resp, err := s.client.gen.GetNavigationWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
