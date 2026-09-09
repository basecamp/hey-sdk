//! The calendars index as a live follower needs it, and the selection every period read is
//! scoped to.

use serde::{Deserialize, Serialize};

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::Calendar;

pub use crate::generated::services::calendars::*;

/// A calendar as the index serves it, wrapped with what a live follower needs.
///
/// `recording_changes_url` is where the calendar's own recording changes feed starts; read
/// it with [`crate::services::CalendarChangesCursor::from_url`]. `signed_stream_name`
/// subscribes the calendar's stream over Action Cable — a frame arriving there means the
/// calendar changed, and the name is stable for the calendar's life. The calendar changes
/// feed's added bucket carries this same shape, so a calendar learned of either way arrives
/// subscribable.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct ListedCalendar {
    #[serde(default)]
    pub calendar: Calendar,
    #[serde(default)]
    pub recording_changes_url: Option<String>,
    #[serde(default)]
    pub signed_stream_name: Option<String>,
}

/// The full calendars index: every calendar with its changes URL and signed stream name,
/// the calendar-level changes feed's own URL, and the calendars the reader has switched on.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct CalendarList {
    #[serde(default)]
    pub calendars: Vec<ListedCalendar>,
    #[serde(default)]
    pub calendar_changes_url: Option<String>,
    #[serde(default)]
    pub selected_calendar_ids: Vec<i64>,
}

impl<'a> Calendars<'a> {
    /// Lists the calendars with everything [`Calendars::list`] throws away: each calendar's
    /// recording changes URL and signed stream name, and the calendar changes URL. It is
    /// the same read, decoded into the fuller shape the wire already carries.
    pub async fn list_with_changes(&self) -> Result<CalendarList, Error> {
        let operation = self.client().operation(&routes::LIST_CALENDARS, &[]);
        self.client().send(operation).await
    }

    /// Switches a calendar in or out of the reader's selection and answers the ids the
    /// selection is left holding. Every
    /// [`CalendarPeriods`](crate::services::calendar_periods::CalendarPeriods) read is scoped
    /// to that selection, so this is how a client changes which calendars a day, week or
    /// year is drawn from.
    pub async fn toggle_selection(&self, calendar_id: i64) -> Result<Vec<i64>, Error> {
        Ok(self.toggle(calendar_id).await?.selected_calendar_ids)
    }
}
