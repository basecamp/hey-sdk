mod support;

use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_day_is_one_period_with_its_recordings_grouped_by_type() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-08-22.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(day()))
        .mount(&server)
        .await;

    let day = client(&server)
        .calendar_periods()
        .day("2026-08-22")
        .await
        .unwrap();

    assert_eq!(day.kind, "day");
    let events = &day.recordings["Calendar::Event"];
    assert_eq!(events.len(), 1);
    assert_eq!(
        events[0].occurrence_id.as_deref(),
        Some("161645836_2026-08-22")
    );
    assert_eq!(
        day.recordings["Calendar::Todo"][0].title.as_deref(),
        Some("Renew the domain")
    );
}

#[tokio::test]
async fn a_day_can_be_asked_for_as_now() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/now.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(day()))
        .mount(&server)
        .await;

    client(&server).calendar_periods().day("now").await.unwrap();

    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn days_read_a_window_from_a_date_and_leave_an_empty_one_off_the_wire() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "days": [day()] })))
        .mount(&server)
        .await;

    let client = client(&server);
    let from_a_date = client.calendar_periods().days("2026-08-22").await.unwrap();
    let from_today = client.calendar_periods().days("").await.unwrap();

    assert_eq!(from_a_date.len(), 1);
    assert_eq!(from_a_date[0].kind, "day");
    assert_eq!(from_today.len(), 1);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("starts_at=2026-08-22"));
    assert_eq!(requests[1].url.query(), None);
}

#[tokio::test]
async fn weeks_center_on_a_date() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/weeks.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "weeks": [week()] })))
        .mount(&server)
        .await;

    let weeks = client(&server)
        .calendar_periods()
        .weeks("", "2026-08-22")
        .await
        .unwrap();

    assert_eq!(weeks.len(), 1);
    assert_eq!(weeks[0].kind, "week");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("centered_at=2026-08-22"));
}

#[tokio::test]
async fn a_week_is_one_period() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/weeks/2026-08-22.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(week()))
        .mount(&server)
        .await;

    let week = client(&server)
        .calendar_periods()
        .week("2026-08-22")
        .await
        .unwrap();

    assert_eq!(week.kind, "week");
}

#[tokio::test]
async fn a_year_is_the_grid_it_is_drawn_as() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/years/2026-08-22.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "starts_at": "2026-01-01T00:00:00Z",
            "ends_at": "2026-12-31T23:59:59Z",
            "kind": "year",
            "padding_days_count": 3,
            "days": [
                { "starts_at": "2026-01-01T00:00:00Z", "backgrounded": false },
                { "starts_at": "2026-01-02T00:00:00Z", "backgrounded": true }
            ],
            "spanned_events": [
                { "id": 5150, "type": "Calendar::Event", "title": "Summer Break", "all_day": true,
                  "starts_at": "2026-07-06T00:00:00Z", "ends_at": "2026-07-17T23:59:59Z" }
            ]
        })))
        .mount(&server)
        .await;

    let year = client(&server)
        .calendar_periods()
        .year("2026-08-22")
        .await
        .unwrap();

    assert_eq!(year.kind, "year");
    assert_eq!(year.padding_days_count, 3);
    assert_eq!(year.days.len(), 2);
    assert!(year.days[1].backgrounded);
    assert_eq!(year.spanned_events.len(), 1);
    assert_eq!(
        year.spanned_events[0].title.as_deref(),
        Some("Summer Break")
    );
}

fn day() -> Value {
    json!({
        "starts_at": "2026-08-22T00:00:00Z",
        "ends_at": "2026-08-22T23:59:59Z",
        "kind": "day",
        "recordings": {
            "Calendar::Event": [
                { "id": 161645836, "type": "Calendar::Event", "title": "Weekly Catchup",
                  "starts_at": "2026-08-22T14:00:00Z", "ends_at": "2026-08-22T14:30:00Z",
                  "recurring": true, "occurrence_id": "161645836_2026-08-22" }
            ],
            "Calendar::Todo": [
                { "id": 90210, "type": "Calendar::Todo", "title": "Renew the domain",
                  "starts_at": "2026-08-22T00:00:00Z", "ends_at": "2026-08-22T23:59:59Z" }
            ]
        }
    })
}

fn week() -> Value {
    json!({
        "starts_at": "2026-08-17T00:00:00Z",
        "ends_at": "2026-08-23T23:59:59Z",
        "kind": "week",
        "recordings": {}
    })
}
