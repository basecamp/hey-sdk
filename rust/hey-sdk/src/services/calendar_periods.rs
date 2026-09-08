//! Reading the calendar as the periods it is drawn in: a day, a week, a year. Every read is
//! scoped to the calendars the reader has switched on, which
//! [`Calendars::toggle_selection`](crate::services::calendars::Calendars::toggle_selection)
//! changes.
//!
//! A period is not the same answer as
//! [`Calendars::get_recordings`](crate::services::calendars::Calendars::get_recordings). A
//! calendar lists the
//! recordings it holds, recurring ones included as the single rows they are stored as; a
//! period expands those into the occurrences that fall inside its window. Draw a week from a
//! calendar's recordings and a weekly meeting shows up once.

use crate::error::Error;
use crate::generated::types::{CalendarPeriod, CalendarYear};

pub use crate::generated::services::calendar_periods::*;

impl<'a> CalendarPeriods<'a> {
    /// Reads one day. The date is `YYYY-MM-DD`, or the literal `now` for today, which
    /// leaves it to HEY to decide what today is where the reader is.
    pub async fn day(&self, date: &str) -> Result<CalendarPeriod, Error> {
        self.get_day(date).await
    }

    /// Reads the days from a date onwards. HEY picks how many, so this is a window rather
    /// than a page: read on by asking again from the last day it answered. An empty date
    /// starts from today.
    pub async fn days(&self, starts_at: &str) -> Result<Vec<CalendarPeriod>, Error> {
        let params = ListCalendarDaysParams {
            starts_at: optional(starts_at),
        };
        Ok(self.list_days(&params).await?.days)
    }

    /// Reads the week a date falls in. The date is `YYYY-MM-DD`.
    pub async fn week(&self, date: &str) -> Result<CalendarPeriod, Error> {
        self.get_week(date).await
    }

    /// Reads nine weeks. `starts_at` names the first of them; `centered_at` centers them on
    /// a date instead, which is what the web app's scrolling week view asks for. Both empty
    /// centers on today, and HEY takes `starts_at` when it is given both.
    pub async fn weeks(
        &self,
        starts_at: &str,
        centered_at: &str,
    ) -> Result<Vec<CalendarPeriod>, Error> {
        let params = ListCalendarWeeksParams {
            starts_at: optional(starts_at),
            centered_at: optional(centered_at),
        };
        Ok(self.list_weeks(&params).await?.weeks)
    }

    /// Reads the year a date falls in, as the grid it is drawn as: one entry per day and the
    /// events that span more than one. A year does not carry every recording it holds.
    pub async fn year(&self, date: &str) -> Result<CalendarYear, Error> {
        self.get_year(date).await
    }
}

/// An empty date is left off the wire: sending `starts_at=` would ask HEY to parse an empty
/// string rather than pick the default itself.
fn optional(value: &str) -> Option<String> {
    if value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}
