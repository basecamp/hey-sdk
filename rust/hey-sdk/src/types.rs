use std::fmt;
use std::str::FromStr;

use chrono::{Datelike, NaiveDate, NaiveTime, TimeDelta, TimeZone, Utc, Weekday};
use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::error::Error;

/// An instant HEY reports, always with its offset.
pub type DateTime = chrono::DateTime<Utc>;

/// A calendar date without a time zone, as HEY writes `starts_on` and `ends_on`.
///
/// Two dates compare and sort chronologically, so use `<`, `>` and `==` rather than
/// looking for `before` and `after`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Default)]
pub struct Date(pub NaiveDate);

impl Date {
    pub fn new(year: i32, month: u32, day: u32) -> Option<Date> {
        NaiveDate::from_ymd_opt(year, month, day).map(Date)
    }

    /// Reads `YYYY-MM-DD`. An empty string is not a date and is rejected as one.
    pub fn parse(source: &str) -> Result<Date, Error> {
        source
            .parse()
            .map_err(|error| Error::usage(format!("invalid date {source:?}: {error}")))
    }

    /// Today where this machine is.
    pub fn today() -> Date {
        Date(chrono::Local::now().date_naive())
    }

    /// Today in UTC.
    pub fn today_utc() -> Date {
        Date(Utc::now().date_naive())
    }

    /// The date an instant falls on in UTC.
    pub fn from_datetime(moment: &DateTime) -> Date {
        Date(moment.date_naive())
    }

    /// The date an instant falls on in the given time zone, which is a different day from
    /// [`Date::from_datetime`] either side of midnight.
    pub fn from_datetime_in<Tz: TimeZone>(moment: &DateTime, zone: &Tz) -> Date {
        Date(moment.with_timezone(zone).date_naive())
    }

    pub fn year(&self) -> i32 {
        self.0.year()
    }

    pub fn month(&self) -> u32 {
        self.0.month()
    }

    pub fn day(&self) -> u32 {
        self.0.day()
    }

    pub fn weekday(&self) -> Weekday {
        self.0.weekday()
    }

    /// Midnight UTC on this date.
    ///
    /// UTC is the only zone this crate turns a date into an instant in: naming another
    /// would mean a time zone database, and the crate carries none. To land on midnight
    /// somewhere else, build the instant with a `chrono::TimeZone` of your own —
    /// `zone.from_local_datetime(&date.0.and_time(NaiveTime::MIN))`.
    pub fn at_midnight_utc(&self) -> DateTime {
        self.0.and_time(NaiveTime::MIN).and_utc()
    }

    pub fn add_days(&self, days: i64) -> Option<Date> {
        self.0
            .checked_add_signed(TimeDelta::try_days(days)?)
            .map(Date)
    }

    /// Adds months the way Go's `time.AddDate` does: the day of the month is kept and a
    /// month too short to hold it rolls over into the next one, so 31 January plus one
    /// month is 3 March in a common year rather than 28 February.
    pub fn add_months(&self, months: i32) -> Option<Date> {
        let target = i64::from(self.0.year()) * 12 + i64::from(self.0.month0()) + i64::from(months);
        let year = i32::try_from(target.div_euclid(12)).ok()?;
        let month = target.rem_euclid(12) as u32 + 1;
        Date::new(year, month, 1)?.add_days(i64::from(self.0.day() - 1))
    }

    /// Adds years with the same rollover as [`Date::add_months`]: 29 February plus one
    /// year is 1 March.
    pub fn add_years(&self, years: i32) -> Option<Date> {
        self.add_months(years.checked_mul(12)?)
    }

    /// The whole days from `earlier` to this date, negative when this date came first.
    pub fn days_since(&self, earlier: Date) -> i64 {
        self.0.signed_duration_since(earlier.0).num_days()
    }
}

impl From<NaiveDate> for Date {
    fn from(date: NaiveDate) -> Date {
        Date(date)
    }
}

impl From<Date> for NaiveDate {
    fn from(date: Date) -> NaiveDate {
        date.0
    }
}

impl fmt::Display for Date {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0.format("%Y-%m-%d"))
    }
}

impl FromStr for Date {
    type Err = chrono::ParseError;

    fn from_str(source: &str) -> Result<Date, chrono::ParseError> {
        NaiveDate::parse_from_str(source, "%Y-%m-%d").map(Date)
    }
}

impl Serialize for Date {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        serializer.serialize_str(&self.to_string())
    }
}

impl<'de> Deserialize<'de> for Date {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Date, D::Error> {
        let text = String::deserialize(deserializer)?;
        text.parse().map_err(serde::de::Error::custom)
    }
}

/// Reads an `Option<Date>` that may arrive as `null` or as `""`, both of which mean no
/// date. Reach for it with `#[serde(default, with = "optional_date")]` on a field HEY
/// blanks rather than omits; a plain `Option<Date>` accepts `null` but not `""`.
pub mod optional_date {
    use serde::{Deserialize, Deserializer, Serialize, Serializer};

    use super::Date;

    pub fn serialize<S: Serializer>(date: &Option<Date>, serializer: S) -> Result<S::Ok, S::Error> {
        date.serialize(serializer)
    }

    pub fn deserialize<'de, D: Deserializer<'de>>(
        deserializer: D,
    ) -> Result<Option<Date>, D::Error> {
        match Option::<String>::deserialize(deserializer)? {
            Some(text) if !text.is_empty() => {
                text.parse().map(Some).map_err(serde::de::Error::custom)
            }
            _ => Ok(None),
        }
    }
}

/// Reads a required field that arrived as `null` as its type's default. Reach for it with
/// `#[serde(default, deserialize_with = "crate::types::null_as_default::deserialize")]`,
/// which the generator puts on every required field of a type that has a zero value.
///
/// HEY writes `null` where it has nothing for a field the model calls required, and Go's
/// `encoding/json` reads that into a non-pointer as a no-op — the field keeps its zero
/// value. `#[serde(default)]` alone only covers the field being absent, so without this a
/// `null` fails the whole response where Go reads it as `""` or `0`.
pub mod null_as_default {
    use serde::{Deserialize, Deserializer};

    pub fn deserialize<'de, D: Deserializer<'de>, T: Deserialize<'de> + Default>(
        deserializer: D,
    ) -> Result<T, D::Error> {
        Ok(Option::<T>::deserialize(deserializer)?.unwrap_or_default())
    }
}

/// A string that must not end up in logs, such as an email address. It prints as
/// `[REDACTED]`; call [`SensitiveString::expose`] to read it.
#[derive(Clone, PartialEq, Eq, Hash, Default, Serialize, Deserialize)]
#[serde(transparent)]
pub struct SensitiveString(String);

impl SensitiveString {
    pub fn new(value: impl Into<String>) -> SensitiveString {
        SensitiveString(value.into())
    }

    pub fn expose(&self) -> &str {
        &self.0
    }

    pub fn into_inner(self) -> String {
        self.0
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

impl From<String> for SensitiveString {
    fn from(value: String) -> SensitiveString {
        SensitiveString(value)
    }
}

impl From<&str> for SensitiveString {
    fn from(value: &str) -> SensitiveString {
        SensitiveString(value.to_string())
    }
}

impl fmt::Debug for SensitiveString {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.0.is_empty() {
            f.write_str("\"\"")
        } else {
            f.write_str("[REDACTED]")
        }
    }
}

impl fmt::Display for SensitiveString {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.0.is_empty() {
            Ok(())
        } else {
            f.write_str("[REDACTED]")
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dates_round_trip_through_json() {
        let date: Date = serde_json::from_str("\"2026-03-04\"").unwrap();
        assert_eq!(date, Date::new(2026, 3, 4).unwrap());
        assert_eq!(serde_json::to_string(&date).unwrap(), "\"2026-03-04\"");
    }

    #[test]
    fn sensitive_strings_hide_their_value() {
        let secret = SensitiveString::new("jane@example.com");
        assert_eq!(format!("{secret:?}"), "[REDACTED]");
        assert_eq!(secret.to_string(), "[REDACTED]");
        assert_eq!(secret.expose(), "jane@example.com");
        assert_eq!(
            serde_json::to_string(&secret).unwrap(),
            "\"jane@example.com\""
        );
    }
}
