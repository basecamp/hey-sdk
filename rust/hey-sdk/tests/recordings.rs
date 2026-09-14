//! `Recording`: the calendar's polymorphic record and the kind predicates generated for it.

use hey_sdk::models::Recording;
use serde_json::json;

// HEY serves both direct and namespaced calendar discriminator values, so each generated
// helper recognizes either spelling.
#[test]
fn a_recording_recognises_its_type_by_the_ruby_class_name() {
    let event: Recording = serde_json::from_value(json!({
        "id": 161_645_836,
        "type": "Calendar::Event",
        "title": "Weekly Catchup"
    }))
    .unwrap();

    assert_eq!(event.r#type, "Calendar::Event");
    assert!(event.is_calendar_event());
    assert!(!event.is_calendar_todo());
}

#[test]
fn every_recording_variant_helper_accepts_both_modeled_wire_types() {
    assert!(recording("CalendarEvent").is_calendar_event());
    assert!(recording("Calendar::Event").is_calendar_event());
    assert!(recording("CalendarTodo").is_calendar_todo());
    assert!(recording("Calendar::Todo").is_calendar_todo());
    assert!(recording("CalendarJournalEntry").is_calendar_journal_entry());
    assert!(recording("Calendar::JournalEntry").is_calendar_journal_entry());
    assert!(recording("CalendarHabit").is_calendar_habit());
    assert!(recording("Calendar::Habit").is_calendar_habit());
    assert!(recording("CalendarTimeTrack").is_calendar_time_track());
    assert!(recording("Calendar::TimeTrack").is_calendar_time_track());
    assert!(recording("CalendarCountdown").is_calendar_countdown());
    assert!(recording("Calendar::Countdown").is_calendar_countdown());
    assert!(recording("CalendarDayBackground").is_calendar_day_background());
    assert!(recording("Calendar::DayBackground").is_calendar_day_background());
}

fn recording(wire_type: &str) -> Recording {
    let mut recording = Recording::default();
    recording.r#type = wire_type.to_string();
    recording
}
