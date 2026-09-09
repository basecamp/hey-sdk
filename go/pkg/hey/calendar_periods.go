package hey

import (
	"context"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CalendarPeriodsService reads the calendar as the periods it is drawn in: a day, a week,
// a year. Every read is scoped to the calendars the identity has switched on, which
// CalendarsService.Toggle changes.
//
// A period is not the same answer as CalendarsService.GetRecordings. A calendar lists the
// recordings it holds, recurring ones included as the single rows they are stored as; a
// period expands those into the occurrences that fall inside its window. Draw a week from
// a calendar's recordings and a weekly meeting shows up once.
type CalendarPeriodsService struct {
	client *Client
}

// NewCalendarPeriodsService creates a new CalendarPeriodsService.
func NewCalendarPeriodsService(client *Client) *CalendarPeriodsService {
	return &CalendarPeriodsService{client: client}
}

// Day returns one day. The date is YYYY-MM-DD, or the literal "now" for today.
func (s *CalendarPeriodsService) Day(ctx context.Context, date string) (day *generated.CalendarPeriod, err error) {
	op := OperationInfo{
		Service: "CalendarPeriods", Operation: "GetCalendarDay",
		ResourceType: "calendar_day", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetCalendarDayWithResponse(ctx, date)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		day = resp.JSON200
		return nil
	})
	return day, err
}

// Days returns the days from a date onwards. HEY picks how many — this is a window rather
// than a page, so the way to read on is to ask again from the last day it answered. An
// empty date starts from today.
func (s *CalendarPeriodsService) Days(ctx context.Context, startsAt string) (days []generated.CalendarPeriod, err error) {
	op := OperationInfo{
		Service: "CalendarPeriods", Operation: "ListCalendarDays",
		ResourceType: "calendar_day", IsMutation: false,
	}

	params := &generated.ListCalendarDaysParams{}
	if startsAt != "" {
		params.StartsAt = &startsAt
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListCalendarDaysWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 != nil {
			days = resp.JSON200.Days
		}
		return nil
	})
	return days, err
}

// Week returns the week any date falls in. The date is YYYY-MM-DD.
func (s *CalendarPeriodsService) Week(ctx context.Context, date string) (week *generated.CalendarPeriod, err error) {
	op := OperationInfo{
		Service: "CalendarPeriods", Operation: "GetCalendarWeek",
		ResourceType: "calendar_week", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetCalendarWeekWithResponse(ctx, date)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		week = resp.JSON200
		return nil
	})
	return week, err
}

// Weeks returns nine weeks. `startsAt` names the first of them; `centeredAt` centers them
// on a date instead, which is what the web app's scrolling week view asks for. Both empty
// centers on today, and `startsAt` wins if both are given.
func (s *CalendarPeriodsService) Weeks(ctx context.Context, startsAt, centeredAt string) (weeks []generated.CalendarPeriod, err error) {
	op := OperationInfo{
		Service: "CalendarPeriods", Operation: "ListCalendarWeeks",
		ResourceType: "calendar_week", IsMutation: false,
	}

	params := &generated.ListCalendarWeeksParams{}
	if startsAt != "" {
		params.StartsAt = &startsAt
	}
	if centeredAt != "" {
		params.CenteredAt = &centeredAt
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListCalendarWeeksWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 != nil {
			weeks = resp.JSON200.Weeks
		}
		return nil
	})
	return weeks, err
}

// Year returns the year any date falls in, as the grid it is drawn as: one entry per day
// and the events that span more than one. A year does not carry every recording it holds.
func (s *CalendarPeriodsService) Year(ctx context.Context, date string) (year *generated.CalendarYear, err error) {
	op := OperationInfo{
		Service: "CalendarPeriods", Operation: "GetCalendarYear",
		ResourceType: "calendar_year", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetCalendarYearWithResponse(ctx, date)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		year = resp.JSON200
		return nil
	})
	return year, err
}
