//! The two identity preferences HEY lets a client write.

use std::str::FromStr;

use chrono::Weekday;

use crate::error::Error;
use crate::generated::types::{
    FirstWeekDayParams, UpdateFirstWeekDayRequestContent, UpdateTimeFormatRequestContent,
};

pub use crate::generated::services::identity::*;

/// The clock HEY renders times on.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TimeFormat {
    TwelveHour,
    TwentyFourHour,
}

impl TimeFormat {
    pub fn as_str(&self) -> &'static str {
        match self {
            TimeFormat::TwelveHour => "twelve_hour",
            TimeFormat::TwentyFourHour => "twenty_four_hour",
        }
    }
}

impl<'a> Identity<'a> {
    /// Sets which day the identity's calendar weeks start on, and answers the day HEY
    /// stored. The write reaches every HEY client — web, mobile and this SDK read the same
    /// identity preference.
    pub async fn set_first_week_day(&self, day: Weekday) -> Result<Weekday, Error> {
        let body = UpdateFirstWeekDayRequestContent {
            identity_preference: FirstWeekDayParams {
                first_week_day: day_name(day).to_string(),
            },
        };
        let stored = self.update_first_week_day(&body).await?;
        weekday_at(stored.first_week_day)
    }

    /// Sets whether HEY renders times on a 12-hour or a 24-hour clock, and answers the
    /// format HEY stored.
    pub async fn set_time_format(&self, format: TimeFormat) -> Result<TimeFormat, Error> {
        let body = UpdateTimeFormatRequestContent {
            twenty_four_hour_time_format: format == TimeFormat::TwentyFourHour,
        };
        self.update_time_format(&body).await?.time_format.parse()
    }
}

/// HEY takes the day as its lowercased English name.
fn day_name(day: Weekday) -> &'static str {
    match day {
        Weekday::Mon => "monday",
        Weekday::Tue => "tuesday",
        Weekday::Wed => "wednesday",
        Weekday::Thu => "thursday",
        Weekday::Fri => "friday",
        Weekday::Sat => "saturday",
        Weekday::Sun => "sunday",
    }
}

/// HEY answers the stored day as an index the way it serves the identity: 0 is Sunday.
fn weekday_at(index: i32) -> Result<Weekday, Error> {
    match index {
        0 => Ok(Weekday::Sun),
        1 => Ok(Weekday::Mon),
        2 => Ok(Weekday::Tue),
        3 => Ok(Weekday::Wed),
        4 => Ok(Weekday::Thu),
        5 => Ok(Weekday::Fri),
        6 => Ok(Weekday::Sat),
        _ => Err(Error::api(
            0,
            format!("first week day {index} is not a day of the week"),
        )),
    }
}

impl FromStr for TimeFormat {
    type Err = Error;

    fn from_str(source: &str) -> Result<TimeFormat, Error> {
        match source {
            "twelve_hour" => Ok(TimeFormat::TwelveHour),
            "twenty_four_hour" => Ok(TimeFormat::TwentyFourHour),
            _ => Err(Error::usage(format!(
                "time format {source:?} is neither \"twelve_hour\" nor \"twenty_four_hour\""
            ))),
        }
    }
}
