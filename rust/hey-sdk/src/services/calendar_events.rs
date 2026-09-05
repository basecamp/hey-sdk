//! Writing calendar events, and reaching one day of a repeating one. HEY's calendar writes
//! are Rails form posts rather than JSON, so everything here is form-encoded under
//! `calendar_event[...]`.
//!
//! [`CalendarEvents::create`], [`CalendarEvents::update_event`] and
//! [`CalendarEvents::update_occurrence`] post to the `.json` path, so a current server
//! answers the written recording while one without the JSON branch redirects and only the
//! id comes back. Listing events is [`crate::services::calendars::Calendars`]' job.

use std::fmt;
use std::str::FromStr;
use std::time::Duration;

use reqwest::Method;

use crate::client::Response;
use crate::error::Error;
use crate::form::FormResponse;
use crate::generated::types::Recording;
use crate::observability::OperationInfo;
use crate::services::write_info;
use crate::types::Date;

pub use crate::generated::services::calendar_events::*;

const ATTENDEES: &str = "calendar_event[attendance_email_addresses][]";
const ALL_DAY_REMINDERS: &str = "all_day_reminder_durations[]";
const TIMED_REMINDERS: &str = "timed_reminder_durations[]";

/// An event's content, which is a replacement rather than a patch.
///
/// HEY reads these four out of the submitted parameters and then defaults every one of them
/// to nothing, so a write that says nothing about a field clears it. There is no way to send
/// a subset: the fields left empty here are the fields the event loses. An update therefore
/// has to read the event first and pass back whatever it means to keep — including through
/// [`CalendarEvents::update_occurrence`], which takes the same parameters.
///
/// The title is not in here, and that is not an oversight: HEY leaves the summary alone when
/// it is not submitted, so it stays a partial field like the rest of a partial write.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct EventContent {
    /// HEY's `calendar_event[description]` — Trix rich text, so HTML going in.
    ///
    /// It does not round-trip. HEY serves the notes back as plain text and omits the key
    /// entirely when they are blank, so echoing a read back flattens the markup rather than
    /// preserving it. Keeping formatted notes through an update means holding the HTML the
    /// caller sent, not the text HEY answered.
    pub notes: String,
    /// A plain string. HEY truncates it at 3900 characters rather than refusing it.
    pub location: String,
    /// Validated as a URL and capped at 2500 characters, so a malformed one is a 422 rather
    /// than a silent drop.
    ///
    /// It is not the `join_link` on a read. HEY derives that by scanning the notes, the
    /// location and this for a known meeting service, and it is response-only — there is
    /// nothing to submit it with.
    pub link: Option<String>,
    /// The email attached to the event, HEY's `calendar_event[entry_id]`. A read serves the
    /// attachment back as `attached_entry`, so a caller keeping one passes that entry's id.
    pub entry_id: Option<i64>,
}

/// A countdown's unit, written as the number of seconds HEY's own form submits and the only
/// form it reads.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum CountdownUnit {
    #[default]
    Days = 86_400,
    Weeks = 604_800,
    Months = 2_629_746,
}

/// The countdown HEY runs up to an event.
///
/// Like [`EventContent`] it is resend-or-lose-it: HEY reads the pair on every editable write
/// and a missing value deletes the countdown, so a zero `value` means the event has none
/// once the write lands. A countdown is a child recording of its own rather than a field on
/// the event, so it is not on the event's JSON and cannot be read back from one. 1 through 30
/// is what the web app offers.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Countdown {
    pub value: u32,
    pub unit: CountdownUnit,
}

/// How often an event repeats. HEY has no day-of-week parameter, so
/// [`RepeatFrequency::EveryWeekday`] — a hardcoded Monday to Friday — is the only weekday
/// set that can be expressed.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum RepeatFrequency {
    EveryDay,
    EveryWeekday,
    EveryWeek,
    EveryOtherWeek,
    EveryDayOfMonth,
    EveryYear,
    /// Keeps whatever schedule the event already has instead of naming a new one. It is how
    /// a write says the recurrence is none of its business.
    #[default]
    Custom,
}

/// When a recurrence stops.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RepeatUntil {
    Forever,
    Date,
    Count,
}

/// An event's recurrence.
///
/// `Repeat::default()` is [`RepeatFrequency::Custom`] with no end, which says "keep the
/// schedule the event already has". On an occurrence update that makes
/// `Some(Repeat::default())` and `None` mean the same thing — leave the series' schedule
/// alone — since [`CalendarEvents::update_occurrence`] sends `Custom` for a `None` one
/// anyway. Everywhere else the two differ: a `None` writes no recurrence field at all.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Repeat {
    pub frequency: RepeatFrequency,
    pub until: Option<RepeatUntil>,
    /// Read only when `until` is [`RepeatUntil::Date`].
    pub until_date: Option<Date>,
    /// Read only when `until` is [`RepeatUntil::Count`].
    pub count: Option<u32>,
}

/// A new calendar event.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct CreateCalendarEventParams {
    pub calendar_id: i64,
    pub title: String,
    /// `YYYY-MM-DD`.
    pub starts_at: String,
    /// `YYYY-MM-DD`. Defaults to `starts_at`.
    pub ends_at: String,
    pub all_day: bool,
    /// `HH:MM`, required unless the event is all-day.
    pub start_time: String,
    /// `HH:MM`, required unless the event is all-day.
    pub end_time: String,
    /// The IANA names of the zones the clock times are written in — "Europe/Zagreb",
    /// "America/New_York". Leave them empty and the times are read in UTC, which is the zone
    /// HEY parses an API request in. HEY keeps a zone per end, as its own form offers, so an
    /// event can start in one and finish in another.
    pub start_time_zone: String,
    pub end_time_zone: String,
    /// One zone for both ends.
    ///
    /// Deprecated: use `start_time_zone` and `end_time_zone`. It stands in for whichever of
    /// them is empty, so a caller that only ever wanted one zone keeps working.
    pub time_zone: String,
    /// How long before the event each reminder goes out. HEY takes several in one write and
    /// de-duplicates them, and accepts any duration rather than only the presets the web app
    /// offers. Only the list matching the event's all-day flag is read, and an empty list is
    /// an event with no reminders.
    pub reminders: Vec<Duration>,
    /// The notes, location, link and attached entry. Nothing exists to lose on a create, so
    /// the default is simply an event with none of them.
    pub content: EventContent,
    /// The guest list. Submitting one makes the caller the organizer and sends invitations.
    pub attendees: Option<Vec<String>>,
    /// Circles the event. HEY reads it only when it is submitted, so `None` is "not circled"
    /// on a create.
    pub highlighted: Option<bool>,
    /// Counts down to the event. The default creates none.
    pub countdown: Countdown,
    /// Makes the event recurring. `None` is a one-off.
    pub repeat: Option<Repeat>,
}

/// A revision of a calendar event. The optional fields are a partial update: only the ones
/// named are sent, and the rest are left as they are.
///
/// The rest are not, and the reason is on HEY's side. It reads the zones, the content
/// fields, the reminders and the countdown out of the submitted parameters on every write
/// and defaults each of them to nothing, so an update saying nothing about one clears it. A
/// caller keeping any of them has to read the event and send them back.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct UpdateCalendarEventParams {
    /// Moves the event to another calendar, which is how the web app's calendar select
    /// relocates one.
    ///
    /// It has to be a calendar the identity can file on — one it owns or shares, and not a
    /// subscription. The personal calendar is the one that catches you out: it is in the
    /// list the identity serves, and filing on it answers 404 all the same.
    pub calendar_id: Option<i64>,
    pub title: Option<String>,
    /// `YYYY-MM-DD`.
    pub starts_at: Option<String>,
    /// `YYYY-MM-DD`.
    pub ends_at: Option<String>,
    pub all_day: Option<bool>,
    /// `HH:MM`. Clock times belong to a timed event, so an all-day revision leaves them off
    /// however they are set here.
    pub start_time: Option<String>,
    /// `HH:MM`.
    pub end_time: Option<String>,
    /// The zones the clock times are written in, as on a create. Empty strings say the times
    /// are UTC and clear the zones the event was saved with; `None` leaves them out of the
    /// request, which HEY also reads as clearing them.
    pub start_time_zone: Option<String>,
    pub end_time_zone: Option<String>,
    /// One zone for both ends.
    ///
    /// Deprecated: use `start_time_zone` and `end_time_zone`. It stands in for whichever of
    /// them is `None`.
    pub time_zone: Option<String>,
    /// Resend-or-lose-it, like the zones: HEY reads the list on every write and unschedules
    /// everything when it is empty, so an update that leaves it out removes the reminders the
    /// event had.
    pub reminders: Vec<Duration>,
    /// The notes, location, link and attached entry, and a replacement rather than a patch —
    /// every field left empty is cleared on the event. Read [`EventContent`] before using it.
    pub content: EventContent,
    /// Replaces the guest list. `None` leaves it alone; an empty list removes every guest.
    pub attendees: Option<Vec<String>>,
    /// Circles or uncircles the event, HEY's "Circle event". `None` leaves it as it is.
    pub highlighted: Option<bool>,
    /// Resend-or-lose-it too: a zero value deletes the event's countdown, because that is
    /// what HEY does with a write that names no countdown value.
    pub countdown: Countdown,
    /// Changes the recurrence. `None` leaves it untouched on a whole-event update — but not
    /// on an occurrence update; see [`UpdateOccurrenceParams`].
    pub repeat: Option<Repeat>,
}

/// A revision of one day of a repeating event. They are a whole-event update's parameters,
/// read the same way but for `repeat`.
///
/// A `None` repeat leaves a whole event's recurrence alone. Here it would end it: HEY reads
/// an occurrence update that names no frequency as "stop repeating", drops the series'
/// schedule and cancels every other occurrence. So [`CalendarEvents::update_occurrence`]
/// sends [`RepeatFrequency::Custom`] for a `None` one, which keeps the schedule the series
/// already has — the recurrence is not usually the business of an update to one day of it.
/// Naming a repeat means changing the series' schedule on purpose.
pub type UpdateOccurrenceParams = UpdateCalendarEventParams;

/// A partial revision of an event: only the fields named are sent, and HEY leaves the
/// rest as they are.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct CalendarEventUpdate {
    pub title: Option<String>,
    /// `YYYY-MM-DD`.
    pub starts_at: Option<String>,
    /// `YYYY-MM-DD`.
    pub ends_at: Option<String>,
    pub all_day: Option<bool>,
    /// `HH:MM`. Clock times belong to a timed event, so an all-day revision leaves them
    /// off however they are set here.
    pub start_time: Option<String>,
    /// `HH:MM`.
    pub end_time: Option<String>,
}

/// One day of a repeating event.
///
/// A repeating event's days are served as virtual occurrences: they carry an id of 0, the
/// series in `parent_id`, and their only handle in `occurrence_id`, which reads
/// `<event id>_<YYYY-MM-DD>`. So an occurrence is addressed by the series it belongs to
/// plus the day it falls on, and the writes that take an id cannot touch one.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct OccurrenceId {
    pub event_id: i64,
    pub date: Date,
}

/// How much of a repeating event a write to one of its occurrences reaches. Either way the
/// series' earlier days are left alone.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum OccurrenceScope {
    /// The named day alone, which HEY records as an exception in the series' schedule.
    /// Written `this_event`.
    #[default]
    ThisOnly,
    /// The named day and every one after it. On an update HEY splits the series: it records
    /// a new repeating event starting at this day, cancels the occurrences from here on, and
    /// either truncates the old series to the day before or destroys it if this was its
    /// first day. The answer is still the old occurrence, not the new series, so a caller
    /// wanting the new event has to read the period again. Written `this_and_following`.
    ThisAndFollowing,
}

impl<'a> CalendarEvents<'a> {
    /// Creates an event and answers it as a recording.
    pub async fn create(&self, params: &CreateCalendarEventParams) -> Result<Recording, Error> {
        self.write(
            Method::POST,
            "/calendar/events.json".to_string(),
            write_info(
                "CalendarEvents",
                "CreateCalendarEvent",
                "calendar_event",
                None,
            ),
            &create_fields(params),
        )
        .await
    }

    /// Revises an event. HEY reads a calendar write out of the submitted form parameters,
    /// so this is a partial update only in what it names: fields left out keep their
    /// value, while the notes, attendees, reminders and countdown a caller says nothing
    /// about are cleared.
    ///
    /// [`CalendarEvents::update_event`] takes those alongside the dates and answers the
    /// recording; this one names the six fields a revision usually means and nothing else.
    pub async fn update(&self, event_id: i64, update: &CalendarEventUpdate) -> Result<(), Error> {
        let fields = update_fields(update);
        let mut operation = self
            .client()
            .request(Method::PATCH, format!("/calendar/events/{event_id}"));
        operation
            .info(write_info(
                "CalendarEvents",
                "UpdateCalendarEvent",
                "calendar_event",
                Some(event_id),
            ))
            .form(&borrowed(&fields))
            .accept("application/json");
        self.client().send_unit(operation).await
    }

    /// Revises an event from the whole of `params` and answers it as a recording.
    pub async fn update_event(
        &self,
        event_id: i64,
        params: &UpdateCalendarEventParams,
    ) -> Result<Recording, Error> {
        self.write(
            Method::PATCH,
            format!("/calendar/events/{event_id}.json"),
            write_info(
                "CalendarEvents",
                "UpdateCalendarEvent",
                "calendar_event",
                Some(event_id),
            ),
            &update_event_fields(params),
        )
        .await
    }

    /// Revises one day of a repeating event and answers it as a recording.
    ///
    /// A date that is not an occurrence of that series is a 404, as is an event the caller
    /// cannot edit — the occurrence routes want edit rights on both the day and the series,
    /// which is stricter than the whole-event update.
    pub async fn update_occurrence(
        &self,
        occurrence: &OccurrenceId,
        scope: OccurrenceScope,
        params: &UpdateOccurrenceParams,
    ) -> Result<Recording, Error> {
        let mut fields = update_event_fields(params);
        fields.push((
            "apply_to_future",
            checkbox(scope == OccurrenceScope::ThisAndFollowing),
        ));
        if params.repeat.is_none() {
            fields.push(("repeat_frequency", RepeatFrequency::Custom.to_string()));
        }

        self.write(
            Method::PATCH,
            occurrence.path(),
            write_info(
                "CalendarEvents",
                "UpdateCalendarEventOccurrence",
                "calendar_event",
                Some(occurrence.event_id),
            ),
            &fields,
        )
        .await
    }

    /// Removes one day of a repeating event, or that day and every one after it. The
    /// generated [`CalendarEvents::delete_occurrence`] takes the same request in its
    /// parts; this one takes the occurrence id HEY serves and a scope.
    pub async fn delete_occurrence_scoped(
        &self,
        occurrence: &OccurrenceId,
        scope: OccurrenceScope,
    ) -> Result<(), Error> {
        let params = DeleteCalendarEventOccurrenceParams {
            apply_to_future: apply_to_future(scope),
        };
        self.delete_occurrence(occurrence.event_id, &occurrence.date.to_string(), &params)
            .await
    }

    /// Posts a calendar write and reads back what it wrote. HEY parses these out of form
    /// parameters, and answers the recording on the `.json` path.
    async fn write(
        &self,
        method: Method,
        path: String,
        info: OperationInfo,
        fields: &[(&'static str, String)],
    ) -> Result<Recording, Error> {
        let mut operation = self.client().form(method, &path)?;
        operation.info(info);
        operation.form(&borrowed(fields));
        recording_from_form_response(&self.client().execute(operation).await?)
    }
}

fn create_fields(params: &CreateCalendarEventParams) -> Vec<(&'static str, String)> {
    let mut ends_at = params.ends_at.as_str();
    if ends_at.is_empty() {
        ends_at = &params.starts_at;
    }

    let mut fields = vec![
        (
            "calendar_event[calendar_id]",
            params.calendar_id.to_string(),
        ),
        ("calendar_event[summary]", params.title.clone()),
        ("calendar_event[starts_at]", params.starts_at.clone()),
        ("calendar_event[ends_at]", ends_at.to_string()),
    ];
    push_content(&mut fields, &params.content);
    push_attendees(&mut fields, params.attendees.as_deref());
    push_highlighted(&mut fields, params.highlighted);
    push_countdown(&mut fields, params.countdown);
    push_repeat(&mut fields, params.repeat.as_ref());

    if params.all_day {
        fields.push(("calendar_event[all_day]", checkbox(true)));
        push_reminders(&mut fields, ALL_DAY_REMINDERS, &params.reminders);
    } else {
        fields.push(("calendar_event[all_day]", checkbox(false)));
        fields.push((
            "calendar_event[starts_at_time]",
            format!("{}:00", params.start_time),
        ));
        fields.push((
            "calendar_event[ends_at_time]",
            format!("{}:00", params.end_time),
        ));
        push_time_zones(
            &mut fields,
            time_zone_or(&params.start_time_zone, &params.time_zone),
            time_zone_or(&params.end_time_zone, &params.time_zone),
        );
        push_reminders(&mut fields, TIMED_REMINDERS, &params.reminders);
    }
    fields
}

/// Writes the content fields. All four go out every time, because HEY clears the ones it is
/// not sent — see [`EventContent`].
fn push_content(fields: &mut Vec<(&'static str, String)>, content: &EventContent) {
    fields.push(("calendar_event[description]", content.notes.clone()));
    fields.push(("calendar_event[location]", content.location.clone()));
    fields.push((
        "calendar_event[url]",
        content.link.clone().unwrap_or_default(),
    ));
    // A zero id names no entry, so it goes out blank rather than as "0" — which HEY would
    // try to look up and refuse.
    fields.push((
        "calendar_event[entry_id]",
        content
            .entry_id
            .filter(|entry_id| *entry_id != 0)
            .map(|entry_id| entry_id.to_string())
            .unwrap_or_default(),
    ));
}

/// Writes the guest list, which HEY replaces wholesale rather than merging. Existing guests
/// are matched by address and keep the status they answered with; an address HEY cannot parse
/// is dropped without comment. Submitting the list at all also makes the caller the event's
/// organizer and sends iCal invitations, and a read's `manage_attendance` says whether the
/// caller may submit it in the first place.
///
/// `None` leaves the roster alone — HEY only touches it when the parameter is present. An
/// empty list clears it, and needs a blank value on the wire to say so, since a form carries
/// no empty array. That blank is what HEY's own form posts.
fn push_attendees(fields: &mut Vec<(&'static str, String)>, attendees: Option<&[String]>) {
    if let Some(addresses) = attendees {
        if addresses.is_empty() {
            fields.push((ATTENDEES, String::new()));
        } else {
            for address in addresses {
                fields.push((ATTENDEES, address.clone()));
            }
        }
    }
}

/// Circles or uncircles the event. HEY reads the flag only when it is submitted, so `None`
/// leaves the circle as it was.
///
/// The empty `highlight_id` is what makes "off" mean off. HEY builds a new highlight when the
/// flag is off and that key is absent — turning the circle on — and destroys the existing one
/// when the key is there and empty. It is never served on a read, so a caller could not
/// construct the right form from one; sending it unconditionally is how this stays a bool.
fn push_highlighted(fields: &mut Vec<(&'static str, String)>, highlighted: Option<bool>) {
    if let Some(highlighted) = highlighted {
        fields.push(("calendar_event[highlighted]", checkbox(highlighted)));
        fields.push(("calendar_event[highlight_id]", String::new()));
    }
}

fn checkbox(value: bool) -> String {
    if value {
        "1".to_string()
    } else {
        "0".to_string()
    }
}

/// Writes the countdown pair. A zero value sends no value at all, which is how HEY is told to
/// delete the countdown — there is no "leave it alone".
fn push_countdown(fields: &mut Vec<(&'static str, String)>, countdown: Countdown) {
    if countdown.value > 0 {
        fields.push((
            "countdown_interval_duration_value",
            countdown.value.to_string(),
        ));
        fields.push((
            "countdown_interval_duration_unit",
            countdown.unit.seconds().to_string(),
        ));
    }
}

/// Writes the recurrence. Nothing is written for a `None` one, which on a whole-event update
/// leaves the recurrence untouched — unlike the occurrence update, where HEY reads the same
/// silence as "stop repeating".
fn push_repeat(fields: &mut Vec<(&'static str, String)>, repeat: Option<&Repeat>) {
    if let Some(repeat) = repeat {
        fields.push(("repeat_frequency", repeat.frequency.to_string()));
        if let Some(until) = repeat.until {
            fields.push((
                "calendar_recurrence_schedule[recurs_until_type]",
                until.to_string(),
            ));
        }
        if repeat.until == Some(RepeatUntil::Date) {
            fields.push((
                "calendar_recurrence_schedule[recurs_until_date]",
                repeat
                    .until_date
                    .map(|date| date.to_string())
                    .unwrap_or_default(),
            ));
        }
        if repeat.until == Some(RepeatUntil::Count) {
            fields.push((
                "calendar_recurrence_schedule[recurs_count]",
                repeat.count.unwrap_or_default().to_string(),
            ));
        }
    }
}

/// Writes the zones a timed event's clock times are written in, and the flag that makes HEY
/// honour them. The flag is not decoration: without it both names are dropped and the times
/// are read in UTC, so 08:00 sent from Zagreb is stored as 08:00Z.
///
/// Naming no zone is a complete answer rather than an omission — convert to UTC and say
/// nothing. An all-day event does not come through here at all, since a date has no zone.
fn push_time_zones(fields: &mut Vec<(&'static str, String)>, start: &str, end: &str) {
    if start.is_empty() && end.is_empty() {
        fields.push(("calendar_event[set_time_zone]", checkbox(false)));
    } else {
        let mut starts_in = start;
        let mut ends_in = end;
        if starts_in.is_empty() {
            starts_in = ends_in;
        }
        if ends_in.is_empty() {
            ends_in = starts_in;
        }
        fields.push(("calendar_event[set_time_zone]", checkbox(true)));
        fields.push((
            "calendar_event[starts_at_time_zone_name]",
            starts_in.to_string(),
        ));
        fields.push((
            "calendar_event[ends_at_time_zone_name]",
            ends_in.to_string(),
        ));
    }
}

/// Lets the deprecated single time zone stand in for an end the caller did not name.
fn time_zone_or<'a>(zone: &'a str, both: &'a str) -> &'a str {
    if zone.is_empty() { both } else { zone }
}

fn push_reminders(
    fields: &mut Vec<(&'static str, String)>,
    key: &'static str,
    reminders: &[Duration],
) {
    for reminder in reminders {
        fields.push((key, reminder.as_secs().to_string()));
    }
}

fn update_fields(update: &CalendarEventUpdate) -> Vec<(&'static str, String)> {
    let mut fields = Vec::new();
    if let Some(title) = &update.title {
        fields.push(("calendar_event[summary]", title.clone()));
    }
    if let Some(starts_at) = &update.starts_at {
        fields.push(("calendar_event[starts_at]", starts_at.clone()));
    }
    if let Some(ends_at) = &update.ends_at {
        fields.push(("calendar_event[ends_at]", ends_at.clone()));
    }
    if let Some(all_day) = update.all_day {
        fields.push(("calendar_event[all_day]", checkbox(all_day)));
    }
    if update.all_day != Some(true) {
        if let Some(start_time) = &update.start_time {
            fields.push(("calendar_event[starts_at_time]", format!("{start_time}:00")));
        }
        if let Some(end_time) = &update.end_time {
            fields.push(("calendar_event[ends_at_time]", format!("{end_time}:00")));
        }
    }
    fields
}

/// Form-encodes a whole-event update. The occurrence update takes the same fields, so it
/// builds its body from here and adds the two parameters that only mean something to a
/// series.
fn update_event_fields(params: &UpdateCalendarEventParams) -> Vec<(&'static str, String)> {
    let mut fields = Vec::new();
    if let Some(title) = &params.title {
        fields.push(("calendar_event[summary]", title.clone()));
    }
    if let Some(starts_at) = &params.starts_at {
        fields.push(("calendar_event[starts_at]", starts_at.clone()));
    }
    if let Some(ends_at) = &params.ends_at {
        fields.push(("calendar_event[ends_at]", ends_at.clone()));
    }
    if let Some(all_day) = params.all_day {
        fields.push(("calendar_event[all_day]", checkbox(all_day)));
    }
    if params.all_day != Some(true) {
        if let Some(start_time) = &params.start_time {
            fields.push(("calendar_event[starts_at_time]", format!("{start_time}:00")));
        }
        if let Some(end_time) = &params.end_time {
            fields.push(("calendar_event[ends_at_time]", format!("{end_time}:00")));
        }
    }
    if let Some(calendar_id) = params.calendar_id {
        fields.push(("calendar_event[calendar_id]", calendar_id.to_string()));
    }
    push_content(&mut fields, &params.content);
    push_attendees(&mut fields, params.attendees.as_deref());
    push_highlighted(&mut fields, params.highlighted);
    push_countdown(&mut fields, params.countdown);
    push_repeat(&mut fields, params.repeat.as_ref());

    let starts_in = params
        .start_time_zone
        .as_ref()
        .or(params.time_zone.as_ref());
    let ends_in = params.end_time_zone.as_ref().or(params.time_zone.as_ref());
    if starts_in.is_some() || ends_in.is_some() {
        push_time_zones(
            &mut fields,
            starts_in.map(String::as_str).unwrap_or_default(),
            ends_in.map(String::as_str).unwrap_or_default(),
        );
    }

    let reminders_key = if params.all_day == Some(true) {
        ALL_DAY_REMINDERS
    } else {
        TIMED_REMINDERS
    };
    push_reminders(&mut fields, reminders_key, &params.reminders);
    fields
}

fn borrowed<'a>(fields: &'a [(&'static str, String)]) -> Vec<(&'static str, &'a str)> {
    fields
        .iter()
        .map(|(name, value)| (*name, value.as_str()))
        .collect()
}

/// The recording a JSON write answers with, falling back to the redirect an older server
/// sends, whose URL still carries the recording's id.
fn recording_from_form_response(answered: &Response) -> Result<Recording, Error> {
    let written = FormResponse::new(answered);
    if written.body.is_empty() {
        Ok(Recording {
            id: written.extract_id()?,
            ..Recording::default()
        })
    } else {
        Ok(serde_json::from_str(&written.body)?)
    }
}

/// The scope goes out either way, as Go's own delete does. HEY reads a missing
/// `apply_to_future` as false, so leaving it off would mean the same thing — but saying it
/// is what makes the request read as the caller's own choice rather than a default.
fn apply_to_future(scope: OccurrenceScope) -> Option<bool> {
    Some(scope == OccurrenceScope::ThisAndFollowing)
}

impl CountdownUnit {
    pub fn seconds(self) -> u32 {
        self as u32
    }
}

impl RepeatFrequency {
    pub fn as_str(&self) -> &'static str {
        match self {
            RepeatFrequency::EveryDay => "every_day",
            RepeatFrequency::EveryWeekday => "every_weekday",
            RepeatFrequency::EveryWeek => "every_week",
            RepeatFrequency::EveryOtherWeek => "every_other_week",
            RepeatFrequency::EveryDayOfMonth => "every_day_of_month",
            RepeatFrequency::EveryYear => "every_year",
            RepeatFrequency::Custom => "custom",
        }
    }
}

impl fmt::Display for RepeatFrequency {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

impl RepeatUntil {
    pub fn as_str(&self) -> &'static str {
        match self {
            RepeatUntil::Forever => "forever",
            RepeatUntil::Date => "date",
            RepeatUntil::Count => "count",
        }
    }
}

impl fmt::Display for RepeatUntil {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

impl OccurrenceId {
    fn path(&self) -> String {
        format!(
            "/calendar/events/{}/occurrences/{}.json",
            self.event_id, self.date
        )
    }
}

impl FromStr for OccurrenceId {
    type Err = Error;

    fn from_str(source: &str) -> Result<OccurrenceId, Error> {
        let (event, day) = source.split_once('_').ok_or_else(|| {
            Error::usage(format!(
                "occurrence id {source:?} is not <event id>_<YYYY-MM-DD>"
            ))
        })?;
        let event_id = match event.parse() {
            Ok(event_id) if event_id > 0 => event_id,
            _ => {
                return Err(Error::usage(format!(
                    "occurrence id {source:?} names no event"
                )));
            }
        };
        let date = day.parse().map_err(|error| {
            Error::usage(format!("occurrence id {source:?} names no date: {error}"))
        })?;
        Ok(OccurrenceId { event_id, date })
    }
}

impl fmt::Display for OccurrenceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}_{}", self.event_id, self.date)
    }
}

impl FromStr for OccurrenceScope {
    type Err = Error;

    fn from_str(source: &str) -> Result<OccurrenceScope, Error> {
        match source {
            "this_event" => Ok(OccurrenceScope::ThisOnly),
            "this_and_following" => Ok(OccurrenceScope::ThisAndFollowing),
            _ => Err(Error::usage(format!(
                "occurrence scope {source:?} is neither \"this_event\" nor \"this_and_following\""
            ))),
        }
    }
}
