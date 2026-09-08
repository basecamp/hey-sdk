mod support;

use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

const PERSONAL_CHANGES_URL: &str = "https://app.hey.com/calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1";
const PERSONAL_STREAM: &str = "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhNZyJ9--personal";
const LIST_CHANGES_URL: &str =
    "https://app.hey.com/calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z";

#[tokio::test]
async fn the_list_carries_the_changes_urls_and_stream_names_the_generated_one_drops() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendars.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "calendars": [
                {
                    "calendar": { "id": 512, "name": "Personal", "personal": true },
                    "recording_changes_url": PERSONAL_CHANGES_URL,
                    "signed_stream_name": PERSONAL_STREAM
                },
                {
                    "calendar": { "id": 513, "name": "Family" },
                    "recording_changes_url": "https://app.hey.com/calendars/513/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1",
                    "signed_stream_name": "IloybGtPaTh2YUdWNUwwTmhiR1Z1WkdGeUx6VXhNdyJ9--family"
                }
            ],
            "calendar_changes_url": LIST_CHANGES_URL,
            "selected_calendar_ids": [512, 513]
        })))
        .mount(&server)
        .await;

    let list = client(&server)
        .calendars()
        .list_with_changes()
        .await
        .unwrap();

    assert_eq!(list.calendars.len(), 2);
    let personal = &list.calendars[0];
    assert_eq!(personal.calendar.id, 512);
    assert_eq!(personal.calendar.name.as_deref(), Some("Personal"));
    assert_eq!(personal.calendar.personal, Some(true));
    assert_eq!(
        personal.recording_changes_url.as_deref(),
        Some(PERSONAL_CHANGES_URL)
    );
    assert_eq!(
        personal.signed_stream_name.as_deref(),
        Some(PERSONAL_STREAM)
    );
    assert_eq!(list.calendar_changes_url.as_deref(), Some(LIST_CHANGES_URL));
    assert_eq!(list.selected_calendar_ids, [512, 513]);
}

#[tokio::test]
async fn a_toggle_answers_the_selection_it_left_behind() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendars/42/toggle.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "selected_calendar_ids": [41, 42] })),
        )
        .mount(&server)
        .await;

    let selected = client(&server)
        .calendars()
        .toggle_selection(42)
        .await
        .unwrap();

    assert_eq!(selected, [41, 42]);
}
