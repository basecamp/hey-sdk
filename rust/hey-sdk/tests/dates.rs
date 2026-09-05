use chrono::{FixedOffset, TimeZone, Utc, Weekday};
use hey_sdk::types::optional_date;
use hey_sdk::{Date, DateTime, ErrorCode};
use serde::{Deserialize, Serialize};

#[test]
fn a_date_reads_and_prints_as_year_month_day() {
    let date = Date::parse("2026-03-04").unwrap();

    assert_eq!(Date::new(2026, 3, 4).unwrap(), date);
    assert_eq!("2026-03-04", date.to_string());
    assert_eq!(2026, date.year());
    assert_eq!(3, date.month());
    assert_eq!(4, date.day());
}

#[test]
fn a_date_that_is_not_one_is_refused() {
    for source in [
        "",
        "2026-13-01",
        "2026-02-30",
        "04/03/2026",
        "2026-03-04T00:00:00Z",
    ] {
        let error = Date::parse(source).unwrap_err();
        assert_eq!(ErrorCode::Usage, error.code(), "{source}");
        assert!(error.message().contains(source), "{source}");
    }

    assert_eq!(None, Date::new(2026, 2, 30));
}

/// HEY always writes the day and the month zero-padded, and Go's `time.Parse` insists on
/// it. Reading one that is not costs nothing and still prints back padded.
#[test]
fn an_unpadded_date_is_read_and_printed_back_padded() {
    assert_eq!("2026-03-04", Date::parse("2026-3-4").unwrap().to_string());
}

#[test]
fn dates_sort_and_compare_chronologically() {
    let earlier = Date::new(2026, 3, 4).unwrap();
    let later = Date::new(2026, 3, 5).unwrap();

    assert!(earlier < later);
    assert!(later > earlier);
    assert_eq!(earlier, Date::new(2026, 3, 4).unwrap());
}

#[test]
fn a_date_comes_from_the_instant_it_falls_on() {
    let just_before_midnight: DateTime = Utc.with_ymd_and_hms(2026, 3, 4, 23, 30, 0).unwrap();

    assert_eq!(
        Date::new(2026, 3, 4).unwrap(),
        Date::from_datetime(&just_before_midnight)
    );

    let tokyo = FixedOffset::east_opt(9 * 3600).unwrap();
    assert_eq!(
        Date::new(2026, 3, 5).unwrap(),
        Date::from_datetime_in(&just_before_midnight, &tokyo)
    );

    let honolulu = FixedOffset::west_opt(10 * 3600).unwrap();
    assert_eq!(
        Date::new(2026, 3, 4).unwrap(),
        Date::from_datetime_in(&just_before_midnight, &honolulu)
    );
}

#[test]
fn today_is_the_current_date_in_the_zone_that_was_asked_for() {
    assert_eq!(Date(Utc::now().date_naive()), Date::today_utc());
    assert_eq!(Date(chrono::Local::now().date_naive()), Date::today());
}

#[test]
fn a_date_becomes_the_instant_it_starts_at() {
    let midnight = Date::new(2026, 3, 4).unwrap().at_midnight_utc();

    assert_eq!(Utc.with_ymd_and_hms(2026, 3, 4, 0, 0, 0).unwrap(), midnight);
    assert_eq!(
        Date::new(2026, 3, 4).unwrap(),
        Date::from_datetime(&midnight)
    );
}

#[test]
fn adding_days_crosses_months_and_years() {
    let date = Date::new(2026, 3, 4).unwrap();

    assert_eq!(Date::new(2026, 3, 5).unwrap(), date.add_days(1).unwrap());
    assert_eq!(Date::new(2026, 3, 3).unwrap(), date.add_days(-1).unwrap());
    assert_eq!(Date::new(2026, 3, 4).unwrap(), date.add_days(0).unwrap());
    assert_eq!(Date::new(2027, 3, 4).unwrap(), date.add_days(365).unwrap());
    assert_eq!(
        Date::new(2024, 3, 1).unwrap(),
        Date::new(2024, 2, 29).unwrap().add_days(1).unwrap()
    );

    assert_eq!(None, Date::new(2026, 3, 4).unwrap().add_days(i64::MAX));
}

/// Go's `time.AddDate` normalizes an overflowing day rather than clamping it, so the last
/// of January plus a month lands in March. Anything ported from the Go SDK counts on that.
#[test]
fn adding_a_month_to_the_31st_rolls_over_rather_than_clamping() {
    assert_eq!(
        Date::new(2023, 3, 3).unwrap(),
        Date::new(2023, 1, 31).unwrap().add_months(1).unwrap()
    );
    assert_eq!(
        Date::new(2024, 3, 2).unwrap(),
        Date::new(2024, 1, 31).unwrap().add_months(1).unwrap()
    );
    assert_eq!(
        Date::new(2026, 7, 1).unwrap(),
        Date::new(2026, 5, 31).unwrap().add_months(1).unwrap()
    );
    assert_eq!(
        Date::new(2027, 3, 3).unwrap(),
        Date::new(2026, 8, 31).unwrap().add_months(6).unwrap()
    );
    assert_eq!(
        Date::new(2027, 3, 3).unwrap(),
        Date::new(2026, 12, 31).unwrap().add_months(2).unwrap()
    );
}

#[test]
fn adding_months_walks_backwards_and_across_the_year() {
    assert_eq!(
        Date::new(2026, 2, 15).unwrap(),
        Date::new(2026, 3, 15).unwrap().add_months(-1).unwrap()
    );
    assert_eq!(
        Date::new(2025, 12, 15).unwrap(),
        Date::new(2026, 1, 15).unwrap().add_months(-1).unwrap()
    );
    assert_eq!(
        Date::new(2024, 12, 1).unwrap(),
        Date::new(2026, 1, 1).unwrap().add_months(-13).unwrap()
    );
    assert_eq!(
        Date::new(2027, 3, 15).unwrap(),
        Date::new(2026, 3, 15).unwrap().add_months(12).unwrap()
    );
    assert_eq!(
        Date::new(2026, 3, 15).unwrap(),
        Date::new(2026, 3, 15).unwrap().add_months(0).unwrap()
    );
}

/// The same rollover as a month: a leap day plus a year is the first of March.
#[test]
fn adding_a_year_to_a_leap_day_rolls_over_into_march() {
    assert_eq!(
        Date::new(2025, 3, 1).unwrap(),
        Date::new(2024, 2, 29).unwrap().add_years(1).unwrap()
    );
    assert_eq!(
        Date::new(2028, 2, 29).unwrap(),
        Date::new(2024, 2, 29).unwrap().add_years(4).unwrap()
    );
    assert_eq!(
        Date::new(2025, 3, 4).unwrap(),
        Date::new(2026, 3, 4).unwrap().add_years(-1).unwrap()
    );
}

#[test]
fn days_since_counts_forwards_and_backwards() {
    let earlier = Date::new(2026, 3, 1).unwrap();
    let later = Date::new(2026, 3, 4).unwrap();

    assert_eq!(3, later.days_since(earlier));
    assert_eq!(-3, earlier.days_since(later));
    assert_eq!(0, later.days_since(later));
    assert_eq!(
        366,
        Date::new(2025, 1, 1)
            .unwrap()
            .days_since(Date::new(2024, 1, 1).unwrap())
    );
}

#[test]
fn a_date_knows_its_weekday() {
    assert_eq!(Weekday::Wed, Date::new(2026, 3, 4).unwrap().weekday());
    assert_eq!(Weekday::Sun, Date::new(2026, 3, 8).unwrap().weekday());
}

#[test]
fn a_date_round_trips_through_json() {
    let date: Date = serde_json::from_str("\"2026-03-04\"").unwrap();

    assert_eq!(Date::new(2026, 3, 4).unwrap(), date);
    assert_eq!("\"2026-03-04\"", serde_json::to_string(&date).unwrap());
}

#[test]
fn a_plain_date_refuses_null_and_the_empty_string() {
    assert!(serde_json::from_str::<Date>("null").is_err());
    assert!(serde_json::from_str::<Date>("\"\"").is_err());
    assert!(serde_json::from_str::<Date>("\"nonsense\"").is_err());
}

#[test]
fn an_optional_date_is_absent_when_it_arrives_as_null_or_blank() {
    #[derive(Debug, PartialEq, Serialize, Deserialize)]
    struct Account {
        #[serde(default, with = "optional_date")]
        trial_ends_on: Option<Date>,
    }

    let blank: Account = serde_json::from_str(r#"{"trial_ends_on":""}"#).unwrap();
    assert_eq!(None, blank.trial_ends_on);

    let null: Account = serde_json::from_str(r#"{"trial_ends_on":null}"#).unwrap();
    assert_eq!(None, null.trial_ends_on);

    let absent: Account = serde_json::from_str("{}").unwrap();
    assert_eq!(None, absent.trial_ends_on);

    let set: Account = serde_json::from_str(r#"{"trial_ends_on":"2026-03-04"}"#).unwrap();
    assert_eq!(Date::new(2026, 3, 4), set.trial_ends_on);

    assert_eq!(
        r#"{"trial_ends_on":"2026-03-04"}"#,
        serde_json::to_string(&set).unwrap()
    );
    assert_eq!(
        r#"{"trial_ends_on":null}"#,
        serde_json::to_string(&null).unwrap()
    );

    assert!(serde_json::from_str::<Account>(r#"{"trial_ends_on":"nonsense"}"#).is_err());
}

/// The generated models read HEY's dates the same way, which is what a real account off
/// the identity endpoint depends on: HEY blanks `trial_ends_on` rather than leaving it out.
#[test]
fn a_generated_optional_date_reads_the_blank_hey_sends() {
    let blank: hey_sdk::models::Account =
        serde_json::from_str(r#"{"id":1,"name":"Acme","trial":false,"trial_ends_on":""}"#).unwrap();
    assert_eq!(None, blank.trial_ends_on);

    let trialing: hey_sdk::models::Account =
        serde_json::from_str(r#"{"id":1,"trial":true,"trial_ends_on":"2026-03-04"}"#).unwrap();
    assert_eq!(Date::new(2026, 3, 4), trialing.trial_ends_on);
    assert_eq!(
        r#"{"id":1,"trial":true,"trial_ends_on":"2026-03-04"}"#,
        serde_json::to_string(&trialing).unwrap()
    );
}

/// HEY writes a moment with the offset it happened at rather than in UTC, and the models
/// read it back as the same instant.
#[test]
fn a_generated_moment_reads_the_offset_hey_wrote_it_in() {
    let period: hey_sdk::models::CalendarPeriod = serde_json::from_str(
        r#"{"starts_at":"2026-03-04T08:00:00+02:00","ends_at":"2026-03-05T08:00:00+02:00","recordings":{}}"#,
    )
    .unwrap();

    assert_eq!(
        Utc.with_ymd_and_hms(2026, 3, 4, 6, 0, 0).unwrap(),
        period.starts_at
    );
    assert_eq!(
        Utc.with_ymd_and_hms(2026, 3, 5, 6, 0, 0).unwrap(),
        period.ends_at
    );
}

/// A moment has no zero value worth having: the epoch is a wrong answer, not an empty one,
/// so a required date or date-time HEY left out fails the read rather than defaulting.
#[test]
fn a_generated_moment_that_is_missing_is_refused_rather_than_defaulted() {
    let missing = serde_json::from_str::<hey_sdk::models::CalendarPeriod>(r#"{"recordings":{}}"#);

    assert!(missing.is_err(), "an epoch is not an answer");
}

/// Go's `encoding/json` reads a `null` into a non-pointer as a no-op — the field keeps its
/// zero value — and HEY writes `null` where it has nothing for a field the model calls
/// required. So do these.
#[test]
fn a_generated_required_field_reads_a_null_as_its_zero_value() {
    let blanked: hey_sdk::models::Mailbox =
        serde_json::from_str(r#"{"id":null,"name":null}"#).unwrap();

    assert_eq!(0, blanked.id);
    assert_eq!("", blanked.name);
    assert_eq!("", blanked.kind);
}
