mod support;

use std::sync::Arc;

use hey_sdk::services::CalendarChangesCursor;
use hey_sdk::{DateTime, ErrorCode};
use serde_json::json;
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{Outcomes, builder, client};

const CALENDAR_FEED: &str = "/calendar/changes.json";
const RECORDING_FEED: &str = "/calendars/512/recording/changes.json";
const SINCE: &str = "since=2026-08-18T09%3A00%3A00.000Z";

#[test]
fn a_cursor_is_read_out_of_the_url_the_server_issued() {
    let recordings = CalendarChangesCursor::from_url(
        "https://app.hey.com/calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1",
    )
    .unwrap();

    assert_eq!(
        recordings.since.as_deref(),
        Some("2026-08-18T09:00:00.000Z")
    );
    assert_eq!(recordings.version.as_deref(), Some("1"));
    assert_eq!(recordings.page, None);

    let calendars = CalendarChangesCursor::from_url(
        "https://app.hey.com/calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z&page=2",
    )
    .unwrap();

    assert_eq!(calendars.since.as_deref(), Some("2026-08-18T09:00:00.000Z"));
    assert_eq!(calendars.page.as_deref(), Some("2"));
    assert_eq!(calendars.version, None);

    let unreadable = CalendarChangesCursor::from_url("://not a url").unwrap_err();

    assert_eq!(unreadable.code(), ErrorCode::Usage);
}

#[tokio::test]
async fn the_calendar_feed_answers_what_changed_and_where_to_resume() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path(CALENDAR_FEED))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</calendar/changes.json?since=2026-08-18T09%3A14%3A22.031Z>; rel="next""#,
                )
                .set_body_json(json!({
                    "added": [{
                        "calendar": { "id": 514, "name": "Book Club" },
                        "recording_changes_url": "https://app.hey.com/calendars/514/recording/changes.json?since=2026-08-18T09%3A14%3A00.000Z&v=1",
                        "signed_stream_name": "book-club-stream"
                    }],
                    "updated": [{ "id": 512, "name": "Personal", "personal": true }],
                    "deleted": [{ "id": 511, "deleted_at": "2026-08-18T09:14:00.000Z" }]
                })),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .calendars()
        .calendar_changes(&calendar_cursor(&server))
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some(SINCE));
    assert_eq!(changes.added.len(), 1);
    assert_eq!(changes.added[0].calendar.id, 514);
    assert_eq!(
        changes.added[0].signed_stream_name.as_deref(),
        Some("book-club-stream")
    );
    assert_eq!(changes.updated.len(), 1);
    assert_eq!(changes.updated[0].id, 512);
    assert_eq!(changes.deleted.len(), 1);
    assert_eq!(changes.deleted[0].id, 511);
    assert_eq!(
        changes.deleted[0].deleted_at,
        moment("2026-08-18T09:14:00Z")
    );
    assert_eq!(changes.next_page, None);
    assert_eq!(
        changes.next_cursor.unwrap().since.as_deref(),
        Some("2026-08-18T09:14:22.031Z")
    );
}

#[tokio::test]
async fn every_calendar_page_is_read_and_the_last_one_names_the_cursor_to_resume_from() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path(CALENDAR_FEED))
        .and(query_param_is_missing("page"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</calendar/changes.json?since=2026-08-18T09%3A00%3A00.000Z&page=2>; rel="next""#,
                )
                .set_body_json(json!({ "updated": [{ "id": 512, "name": "Personal" }] })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path(CALENDAR_FEED))
        .and(query_param("page", "2"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</calendar/changes.json?since=2026-08-18T09%3A30%3A00.000Z>; rel="next""#,
                )
                .set_body_json(json!({ "updated": [{ "id": 513, "name": "Family" }] })),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .calendars()
        .all_calendar_changes(&calendar_cursor(&server))
        .await
        .unwrap();

    let ids: Vec<i64> = changes.updated.iter().map(|calendar| calendar.id).collect();
    assert_eq!(ids, [512, 513]);
    assert_eq!(
        changes.next_cursor.unwrap().since.as_deref(),
        Some("2026-08-18T09:30:00.000Z")
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn the_recording_feed_keeps_the_type_keys_and_dedupes_the_deletions_across_them() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path(RECORDING_FEED))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</calendars/512/recording/changes.json?since=2026-08-18T09%3A14%3A22.031Z&v=1>; rel="next""#,
                )
                .set_body_json(json!({
                    "added": { "Calendar::Event": [{ "id": 88001, "title": "Dentist appointment" }] },
                    "updated": { "Calendar::JournalEntry": [{ "id": 88002 }] },
                    "deleted": {
                        "Calendar::Habit": [
                            { "id": 88005, "deleted_at": "2026-08-18T09:12:00.000Z", "type": "Calendar::Habit" },
                            { "id": 88003, "deleted_at": "2026-08-18T09:14:00.000Z", "type": "Calendar::Todo" }
                        ],
                        "Calendar::Todo": [
                            { "id": 88005, "deleted_at": "2026-08-18T09:12:00.000Z", "type": "Calendar::Habit" },
                            { "id": 88003, "deleted_at": "2026-08-18T09:14:00.000Z", "type": "Calendar::Todo" }
                        ]
                    }
                })),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .calendars()
        .recording_changes(512, &recording_cursor(&server))
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.query(),
        Some(format!("{SINCE}&v=1").as_str())
    );
    let added = &changes.added["Calendar::Event"];
    assert_eq!(added.len(), 1);
    assert_eq!(added[0].id, 88001);
    assert_eq!(added[0].title.as_deref(), Some("Dentist appointment"));
    assert_eq!(changes.updated["Calendar::JournalEntry"][0].id, 88002);
    assert_eq!(changes.deleted.len(), 2);
    assert_eq!(changes.deleted[0].id, 88005);
    assert_eq!(changes.deleted[0].r#type, "Calendar::Habit");
    assert_eq!(changes.deleted[1].id, 88003);
    assert_eq!(changes.deleted[1].r#type, "Calendar::Todo");
    assert_eq!(changes.next_page, None);
    assert_eq!(
        changes.next_cursor.unwrap().since.as_deref(),
        Some("2026-08-18T09:14:22.031Z")
    );
}

#[tokio::test]
async fn every_recording_page_is_read_and_merged_under_its_type_key() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path(RECORDING_FEED))
        .and(query_param_is_missing("page"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</calendars/512/recording/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=1&page=2>; rel="next""#,
                )
                .set_body_json(json!({
                    "added": { "Calendar::Event": [{ "id": 88001, "title": "Dentist appointment" }] },
                    "deleted": { "Calendar::Todo": [{ "id": 88003, "deleted_at": "2026-08-18T09:14:00.000Z", "type": "Calendar::Todo" }] }
                })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path(RECORDING_FEED))
        .and(query_param("page", "2"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</calendars/512/recording/changes.json?since=2026-08-18T09%3A30%3A00.000Z&v=1>; rel="next""#,
                )
                .set_body_json(json!({
                    "added": { "Calendar::Event": [{ "id": 88004, "title": "Parent-teacher conference" }] },
                    "deleted": { "Calendar::Habit": [{ "id": 88005, "deleted_at": "2026-08-18T09:20:00.000Z", "type": "Calendar::Habit" }] }
                })),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .calendars()
        .all_recording_changes(512, &recording_cursor(&server))
        .await
        .unwrap();

    let events: Vec<i64> = changes.added["Calendar::Event"]
        .iter()
        .map(|event| event.id)
        .collect();
    assert_eq!(events, [88001, 88004]);
    let deleted: Vec<i64> = changes.deleted.iter().map(|record| record.id).collect();
    assert_eq!(deleted, [88003, 88005]);
    assert_eq!(
        changes.next_cursor.unwrap().since.as_deref(),
        Some("2026-08-18T09:30:00.000Z")
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_feed_that_named_no_link_leaves_the_caller_the_cursor_it_has() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path(RECORDING_FEED))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .mount(&server)
        .await;

    let changes = client(&server)
        .calendars()
        .recording_changes(512, &recording_cursor(&server))
        .await
        .unwrap();

    assert_eq!(changes.next_page, None);
    assert_eq!(changes.next_cursor, None);
    assert!(changes.added.is_empty());
}

#[tokio::test]
async fn a_cursor_the_recording_feed_has_left_behind_asks_for_a_full_sync() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path(RECORDING_FEED))
        .respond_with(ResponseTemplate::new(409))
        .mount(&server)
        .await;

    let outcomes = Arc::new(Outcomes::default());
    let client = builder(&server).hooks(outcomes.clone()).build().unwrap();
    let one = client
        .calendars()
        .recording_changes(512, &recording_cursor(&server))
        .await
        .unwrap();
    let all = client
        .calendars()
        .all_recording_changes(512, &recording_cursor(&server))
        .await
        .unwrap();

    assert!(one.full_sync_required);
    assert!(one.added.is_empty() && one.updated.is_empty() && one.deleted.is_empty());
    assert!(all.full_sync_required);
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
    // The caller is told to sync in full; the hooks are told what HEY actually answered.
    assert_eq!(outcomes.statuses(), [Some(409), Some(409)]);
}

#[tokio::test]
async fn a_link_that_leaves_the_hey_origin_is_refused_by_both_feeds() {
    let server = MockServer::start().await;
    let elsewhere = r#"<https://evil.example.com/calendar/changes.json?since=2026-08-18T09%3A14%3A22.031Z>; rel="next""#;
    for feed in [CALENDAR_FEED, RECORDING_FEED] {
        Mock::given(method("GET"))
            .and(path(feed))
            .respond_with(
                ResponseTemplate::new(200)
                    .insert_header("Link", elsewhere)
                    .set_body_json(json!({})),
            )
            .mount(&server)
            .await;
    }

    let client = client(&server);
    let calendars = client
        .calendars()
        .calendar_changes(&calendar_cursor(&server))
        .await
        .unwrap_err();
    let recordings = client
        .calendars()
        .recording_changes(512, &recording_cursor(&server))
        .await
        .unwrap_err();

    assert_eq!(calendars.code(), ErrorCode::Usage);
    assert_eq!(
        calendars.message(),
        "changes Link header points to a different origin: https://evil.example.com/calendar/changes.json?since=2026-08-18T09%3A14%3A22.031Z"
    );
    assert_eq!(recordings.code(), ErrorCode::Usage);
}

#[tokio::test]
async fn a_feed_read_without_the_cursor_the_server_issued_is_refused_before_it_is_sent() {
    let server = MockServer::start().await;
    let empty =
        CalendarChangesCursor::from_url(&format!("{}{CALENDAR_FEED}", server.uri())).unwrap();

    let client = client(&server);
    let calendars = client
        .calendars()
        .calendar_changes(&empty)
        .await
        .unwrap_err();
    let recordings = client
        .calendars()
        .recording_changes(512, &empty)
        .await
        .unwrap_err();
    let versionless = client
        .calendars()
        .recording_changes(512, &calendar_cursor(&server))
        .await
        .unwrap_err();
    let all_versionless = client
        .calendars()
        .all_recording_changes(512, &calendar_cursor(&server))
        .await
        .unwrap_err();

    assert_eq!(
        calendars.message(),
        "a since cursor is required — start from the list's calendar_changes_url"
    );
    assert_eq!(
        recordings.message(),
        "a since cursor is required — start from the calendar's recording_changes_url"
    );
    assert_eq!(
        versionless.message(),
        "a feed version is required — read the calendar's recording_changes_url with CalendarChangesCursor::from_url"
    );
    assert_eq!(all_versionless.code(), ErrorCode::Usage);
    assert!(server.received_requests().await.unwrap().is_empty());
}

fn calendar_cursor(server: &MockServer) -> CalendarChangesCursor {
    CalendarChangesCursor::from_url(&format!("{}{CALENDAR_FEED}?{SINCE}", server.uri())).unwrap()
}

fn recording_cursor(server: &MockServer) -> CalendarChangesCursor {
    CalendarChangesCursor::from_url(&format!("{}{RECORDING_FEED}?{SINCE}&v=1", server.uri()))
        .unwrap()
}

fn moment(timestamp: &str) -> DateTime {
    timestamp.parse().unwrap()
}
