use hey_sdk::models::Recording;
use serde_json::json;

// The discriminator on the wire is the recordable's Ruby class name, so the helper has to
// answer to `Calendar::Event`, not to the name its own method is spelled with.
#[test]
fn a_recording_recognises_its_type_by_the_ruby_class_name() {
    let event: Recording = serde_json::from_value(json!({
        "id": 161645836,
        "type": "Calendar::Event",
        "title": "Weekly Catchup"
    }))
    .unwrap();

    assert_eq!(event.r#type, "Calendar::Event");
    assert!(event.is_calendar_event());
    assert!(!event.is_calendar_todo());
}

#[test]
fn every_recording_variant_has_a_helper_that_answers_to_its_wire_type() {
    assert!(recording("Calendar::Event").is_calendar_event());
    assert!(recording("Calendar::Todo").is_calendar_todo());
    assert!(recording("Calendar::JournalEntry").is_calendar_journal_entry());
    assert!(recording("Calendar::Habit").is_calendar_habit());
    assert!(recording("Calendar::TimeTrack").is_calendar_time_track());
    assert!(recording("Calendar::Countdown").is_calendar_countdown());
    assert!(recording("Calendar::DayBackground").is_calendar_day_background());
}

fn recording(wire_type: &str) -> Recording {
    Recording {
        r#type: wire_type.to_string(),
        ..Default::default()
    }
}
