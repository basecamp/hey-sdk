package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
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

// EventContentParams is an event's content, and it is a replacement rather than a patch.
//
// HEY reads these four out of the submitted parameters and then defaults every one of them to
// nil, so a write that says nothing about a field clears it. There is no way to send a subset:
// the fields left out of the struct are the fields the event loses. An update therefore has to
// read the event first and pass back whatever it means to keep — including through
// UpdateOccurrence, which takes the same parameters.
//
// Title is not in here, and that is not an oversight: HEY leaves the summary alone when it is
// not submitted, so it stays a *string on an update like the rest of a partial write.
type EventContentParams struct {
	// Notes is HEY's calendar_event[description] — Trix rich text, so HTML going in.
	//
	// It does not round-trip. HEY serves the notes back as plain text and omits the key
	// entirely when they are blank, so echoing a read back flattens the markup rather than
	// preserving it. Keeping formatted notes through an update means holding the HTML the
	// caller sent, not the text HEY answered.
	Notes string
	// Location is a plain string. HEY truncates it at 3900 characters rather than refusing it.
	Location string
	// Link is validated as a URL and capped at 2500 characters, so a malformed one is a 422
	// rather than a silent drop.
	//
	// It is not the join_link on a read. HEY derives that by scanning the notes, the location
	// and this for a known meeting service, and it is response-only — there is nothing to
	// submit it with.
	Link string
	// EntryID attaches an email to the event, HEY's calendar_event[entry_id]. A read serves the
	// attachment back as attached_entry, so a caller keeping one passes attached_entry.id here.
	EntryID int64
}

// CountdownUnit is one countdown unit as a number of seconds, which is the form HEY's own form
// submits and the only one it reads.
type CountdownUnit int

const (
	CountdownUnitDays   CountdownUnit = 86400
	CountdownUnitWeeks  CountdownUnit = 604800
	CountdownUnitMonths CountdownUnit = 2629746
)

// CountdownParams is the countdown HEY runs up to an event, and like EventContentParams it is
// resend-or-lose-it: HEY reads the pair on every editable write and a missing value deletes the
// countdown. The zero value therefore means the event has no countdown once the write lands.
//
// A countdown is a child recording of its own rather than a field on the event, so it is not on
// the event's JSON and cannot be read back from one. Value 1–30 is what the web app offers.
type CountdownParams struct {
	Value int
	Unit  CountdownUnit
}

// RepeatFrequency is how often an event repeats. HEY has no day-of-week parameter, so
// RepeatEveryWeekday — a hardcoded Monday to Friday — is the only weekday set expressible.
type RepeatFrequency string

const (
	RepeatEveryDay        RepeatFrequency = "every_day"
	RepeatEveryWeekday    RepeatFrequency = "every_weekday"
	RepeatEveryWeek       RepeatFrequency = "every_week"
	RepeatEveryOtherWeek  RepeatFrequency = "every_other_week"
	RepeatEveryDayOfMonth RepeatFrequency = "every_day_of_month"
	RepeatEveryYear       RepeatFrequency = "every_year"
	// RepeatCustom keeps whatever schedule the event already has instead of naming a new one.
	// It is how a write says the recurrence is none of its business.
	RepeatCustom RepeatFrequency = "custom"
)

// RepeatUntil says when a recurrence stops.
type RepeatUntil string

const (
	RepeatUntilForever RepeatUntil = "forever"
	RepeatUntilDate    RepeatUntil = "date"
	RepeatUntilCount   RepeatUntil = "count"
)

// RepeatParams is an event's recurrence.
type RepeatParams struct {
	Frequency RepeatFrequency
	Until     RepeatUntil
	// UntilDate is YYYY-MM-DD and is read only when Until is RepeatUntilDate.
	UntilDate string
	// Count is read only when Until is RepeatUntilCount.
	Count int
}

// setEventContent writes the content fields. All four go out every time, because HEY clears the
// ones it is not sent — see EventContentParams.
func setEventContent(values url.Values, content EventContentParams) {
	values.Set("calendar_event[description]", content.Notes)
	values.Set("calendar_event[location]", content.Location)
	values.Set("calendar_event[url]", content.Link)
	if content.EntryID == 0 {
		values.Set("calendar_event[entry_id]", "")
	} else {
		values.Set("calendar_event[entry_id]", fmt.Sprintf("%d", content.EntryID))
	}
}

// setAttendees writes the guest list, which HEY replaces wholesale rather than merging. Existing
// guests are matched by address and keep the status they answered with; an address HEY cannot
// parse is dropped without comment. Submitting the list at all also makes the caller the event's
// organizer and sends iCal invitations, and a read's manage_attendance says whether the caller
// may submit it in the first place.
//
// A nil list leaves the roster alone — HEY only touches it when the parameter is present. An
// empty one clears it, and needs a blank value on the wire to say so, since a form carries no
// empty array. That blank is what HEY's own form posts.
func setAttendees(values url.Values, attendees []string) {
	if attendees == nil {
		return
	}
	if len(attendees) == 0 {
		values.Add("calendar_event[attendance_email_addresses][]", "")
		return
	}
	for _, address := range attendees {
		values.Add("calendar_event[attendance_email_addresses][]", address)
	}
}

// setHighlighted circles or uncircles the event. HEY reads the flag only when it is submitted, so
// nil leaves the circle as it was.
//
// The empty highlight_id is what makes "off" mean off. HEY builds a new highlight when the flag
// is off and that key is absent — turning the circle on — and destroys the existing one when the
// key is there and empty. It is never served on a read, so a caller could not construct the
// right form from one; sending it unconditionally is how this stays a bool.
func setHighlighted(values url.Values, highlighted *bool) {
	if highlighted == nil {
		return
	}
	if *highlighted {
		values.Set("calendar_event[highlighted]", "1")
	} else {
		values.Set("calendar_event[highlighted]", "0")
	}
	values.Set("calendar_event[highlight_id]", "")
}

// setCountdown writes the countdown pair. A zero value sends no value at all, which is how HEY
// is told to delete the countdown — there is no "leave it alone".
func setCountdown(values url.Values, countdown CountdownParams) {
	if countdown.Value <= 0 {
		return
	}
	unit := countdown.Unit
	if unit == 0 {
		unit = CountdownUnitDays
	}
	values.Set("countdown_interval_duration_value", fmt.Sprintf("%d", countdown.Value))
	values.Set("countdown_interval_duration_unit", fmt.Sprintf("%d", int(unit)))
}

// setRepeat writes the recurrence. Nothing is written for a nil one, which on a whole-event
// update leaves the recurrence untouched — unlike the occurrence update, where HEY reads the
// same silence as "stop repeating".
func setRepeat(values url.Values, repeat *RepeatParams) {
	if repeat == nil {
		return
	}
	values.Set("repeat_frequency", string(repeat.Frequency))
	if repeat.Until != "" {
		values.Set("calendar_recurrence_schedule[recurs_until_type]", string(repeat.Until))
	}
	if repeat.Until == RepeatUntilDate {
		values.Set("calendar_recurrence_schedule[recurs_until_date]", repeat.UntilDate)
	}
	if repeat.Until == RepeatUntilCount {
		values.Set("calendar_recurrence_schedule[recurs_count]", fmt.Sprintf("%d", repeat.Count))
	}
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
	// Reminders is a list of durations before the event to send reminders. HEY takes several in
	// one write and de-duplicates them, and accepts any duration rather than only the presets
	// the web app offers. Only the list matching the event's all-day flag is read.
	//
	// Empty means no reminders. On an update that is not "leave them alone" but "remove them",
	// so a partial update that forgets them silently unschedules every one.
	Reminders []time.Duration
	// Content is the notes, location, link and attached entry. Nothing exists to lose on a
	// create, so the zero value is simply an event with none of them.
	Content EventContentParams
	// Attendees is the guest list. Submitting one makes the caller the organizer and sends
	// invitations.
	Attendees []string
	// Highlighted circles the event. HEY reads it only when it is submitted, so nil is "not
	// circled" on a create.
	Highlighted *bool
	// Countdown counts down to the event. The zero value creates none.
	Countdown CountdownParams
	// Repeat makes the event recurring. A nil one is a one-off.
	Repeat *RepeatParams
}

// UpdateCalendarEventParams contains the parameters for updating a calendar event. The pointer
// fields are a partial update: only the non-nil ones are sent, and the rest are left as they are.
//
// The value fields are not, and the reason is on HEY's side. It reads the zones, the content
// fields, the reminders and the countdown out of the submitted parameters on every write and
// defaults each of them to nothing, so an update saying nothing about one clears it. Those are
// marked below; a caller keeping any of them has to read the event and send them back.
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
	TimeZone *string
	// Reminders is resend-or-lose-it, like the zones. HEY reads the list on every write and
	// unschedules everything when it is empty, so an update that leaves it out removes the
	// reminders the event had. Several durations go in one write and HEY de-duplicates them;
	// only the list matching the event's all-day flag is read.
	Reminders []time.Duration
	// Content is the notes, location, link and attached entry, and it is a replacement rather
	// than a patch — every field left empty is cleared on the event. See EventContentParams
	// before using it: keeping any of the four means reading the event and passing it back.
	Content EventContentParams
	// Attendees replaces the guest list. Nil leaves it alone; an empty non-nil slice removes
	// every guest. See setAttendees for what submitting it commits the caller to.
	Attendees []string
	// Highlighted circles or uncircles the event, HEY's "Circle event". Nil leaves it as it is.
	Highlighted *bool
	// Countdown is resend-or-lose-it too: the zero value deletes the event's countdown, because
	// that is what HEY does with a write that names no countdown value.
	Countdown CountdownParams
	// Repeat changes the recurrence. Nil leaves it untouched on a whole-event update — but not
	// on an occurrence update, where HEY reads silence as "stop repeating"; see
	// UpdateCalendarEventOccurrenceParams.
	Repeat *RepeatParams
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
	setEventContent(values, params.Content)
	setAttendees(values, params.Attendees)
	setHighlighted(values, params.Highlighted)
	setCountdown(values, params.Countdown)
	setRepeat(values, params.Repeat)

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

	resp, err := s.client.PatchForm(ctx, fmt.Sprintf("/calendar/events/%d.json", eventID), updateEventValues(params))
	if err != nil {
		return nil, err
	}
	return recordingFromFormResponse(resp)
}

// updateEventValues form-encodes an update. The occurrence update takes the same fields, so it
// builds its body from here and adds the two parameters that only mean something to a series.
func updateEventValues(params UpdateCalendarEventParams) url.Values {
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
	// Clock times belong to timed events. All-day updates carry dates without clock times,
	// matching all-day creates.
	allDay := params.AllDay != nil && *params.AllDay
	if !allDay {
		if params.StartTime != nil {
			values.Set("calendar_event[starts_at_time]", *params.StartTime+":00")
		}
		if params.EndTime != nil {
			values.Set("calendar_event[ends_at_time]", *params.EndTime+":00")
		}
	}
	if params.CalendarID != nil {
		values.Set("calendar_event[calendar_id]", fmt.Sprintf("%d", *params.CalendarID))
	}
	setEventContent(values, params.Content)
	setAttendees(values, params.Attendees)
	setHighlighted(values, params.Highlighted)
	setCountdown(values, params.Countdown)
	setRepeat(values, params.Repeat)
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

	return values
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

const occurrenceDateLayout = "2006-01-02"

// EventOccurrence names one day of a repeating calendar event.
//
// A repeating event's days are served as virtual occurrences: they carry an id of 0, the series
// in parent_id, and their only handle in occurrence_id, which reads "<event id>_<YYYY-MM-DD>".
// So an occurrence is addressed by the series it belongs to plus the day it falls on, and
// Update and Delete — which take an id — cannot touch one.
type EventOccurrence struct {
	// EventID is the repeating event the occurrence belongs to, HEY's parent_id.
	EventID int64
	// Date is the day the occurrence falls on. Only the calendar date is read.
	Date time.Time
}

// ParseOccurrenceID reads the occurrence_id HEY serves for a virtual occurrence.
func ParseOccurrenceID(occurrenceID string) (EventOccurrence, error) {
	event, day, split := strings.Cut(occurrenceID, "_")
	if !split {
		return EventOccurrence{}, fmt.Errorf("occurrence id %q is not <event id>_<YYYY-MM-DD>", occurrenceID)
	}

	eventID, err := strconv.ParseInt(event, 10, 64)
	if err != nil || eventID <= 0 {
		return EventOccurrence{}, fmt.Errorf("occurrence id %q names no event", occurrenceID)
	}

	date, err := time.Parse(occurrenceDateLayout, day)
	if err != nil {
		return EventOccurrence{}, fmt.Errorf("occurrence id %q names no date: %w", occurrenceID, err)
	}

	return EventOccurrence{EventID: eventID, Date: date}, nil
}

// String is the occurrence_id again, so an occurrence read out of one can be handed back as one.
func (o EventOccurrence) String() string {
	return fmt.Sprintf("%d_%s", o.EventID, o.DateParam())
}

// DateParam is the date as the occurrence routes take it.
func (o EventOccurrence) DateParam() string {
	return o.Date.Format(occurrenceDateLayout)
}

func (o EventOccurrence) path() string {
	return fmt.Sprintf("/calendar/events/%d/occurrences/%s.json", o.EventID, o.DateParam())
}

// OccurrenceScope says how much of a repeating event a write to one of its occurrences reaches.
// Whichever is chosen, the series' earlier days are left alone; the whole series is reached by
// Update and Delete on the event id instead.
type OccurrenceScope string

const (
	// OccurrenceScopeThisEvent changes or removes the named day alone. It is the zero value's
	// meaning too, so a caller who says nothing gets the narrower of the two.
	OccurrenceScopeThisEvent OccurrenceScope = "this_event"
	// OccurrenceScopeThisAndFollowing changes or removes the named day and every one after it.
	// On an update HEY does this by splitting the series: it records a new repeating event
	// starting at this day, cancels the occurrences from here on, and either truncates the old
	// series to the day before or destroys it if this was its first day. The response is still
	// the old occurrence, not the new series, so a caller wanting the new event has to read the
	// period again.
	OccurrenceScopeThisAndFollowing OccurrenceScope = "this_and_following"
)

func (s OccurrenceScope) applyToFuture() string {
	if s == OccurrenceScopeThisAndFollowing {
		return "1"
	}
	return "0"
}

// UpdateCalendarEventOccurrenceParams contains the parameters for updating one occurrence of a
// repeating event. They are a whole-event update's, read the same way but for Repeat.
//
// A nil Repeat leaves a whole event's recurrence alone. Here it would end it: HEY reads an
// occurrence update that names no frequency as "stop repeating", drops the series' schedule and
// cancels every other occurrence. So UpdateOccurrence sends RepeatCustom for a nil one, which
// keeps the schedule the series already has — the recurrence is not usually the business of an
// update to one day of it. Naming a Repeat means changing the series' schedule on purpose.
type UpdateCalendarEventOccurrenceParams struct {
	UpdateCalendarEventParams
}

// UpdateOccurrence updates one day of a repeating event and returns it as a recording.
//
// PATCH /calendar/events/{event id}/occurrences/{YYYY-MM-DD}.json. A date that is not an
// occurrence of that series is a 404, as is an event the caller cannot edit — the occurrence
// routes want edit rights on both the day and the series, which is stricter than the
// whole-event update.
func (s *CalendarEventsService) UpdateOccurrence(ctx context.Context, occurrence EventOccurrence, scope OccurrenceScope, params UpdateCalendarEventOccurrenceParams) (recording *generated.Recording, err error) {
	op := OperationInfo{
		Service: "CalendarEvents", Operation: "UpdateCalendarEventOccurrence",
		ResourceType: "calendar_event", IsMutation: true, ResourceID: occurrence.EventID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	values := updateEventValues(params.UpdateCalendarEventParams)
	values.Set("apply_to_future", scope.applyToFuture())
	if params.Repeat == nil {
		values.Set("repeat_frequency", string(RepeatCustom))
	}

	resp, err := s.client.PatchForm(ctx, occurrence.path(), values)
	if err != nil {
		return nil, err
	}
	return recordingFromFormResponse(resp)
}

// DeleteOccurrence removes one day of a repeating event, or that day and every one after it.
//
// DELETE /calendar/events/{event id}/occurrences/{YYYY-MM-DD}.json, with the scope in the
// query string because a delete carries no body. HEY answers 204, so there is nothing to read
// back: a single day becomes an exception in the series' schedule, and the wider scope
// truncates the series at the day before — or destroys it, if this was its first day.
func (s *CalendarEventsService) DeleteOccurrence(ctx context.Context, occurrence EventOccurrence, scope OccurrenceScope) (err error) {
	op := OperationInfo{
		Service: "CalendarEvents", Operation: "DeleteCalendarEventOccurrence",
		ResourceType: "calendar_event", IsMutation: true, ResourceID: occurrence.EventID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	_, err = s.client.DeleteForm(ctx, occurrence.path()+"?apply_to_future="+scope.applyToFuture())
	return err
}
