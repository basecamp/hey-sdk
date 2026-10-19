//! A 422 keeps the reasons HEY gave: `Error::field_errors`, and the hint that joins them.

mod support;

use hey_sdk::http::{HeaderMap, Method, StatusCode};
use hey_sdk::services::HabitParams;
use hey_sdk::{Error, ErrorCode, FieldError};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

/// The body HEY answers a refused habit with: the model's full messages
/// (`Calendar::HabitsController` renders `habit.errors.full_messages`).
fn habit_refusal() -> serde_json::Value {
    json!({ "errors": ["Title can't be blank", "Days must include at least one day"] })
}

/// A reason as the tests compare it: the field, when named, and the message.
type Reason = (Option<String>, String);

fn reason(field: Option<&str>, message: &str) -> Reason {
    (field.map(ToString::to_string), message.to_string())
}

fn read(reasons: &[FieldError]) -> Vec<Reason> {
    reasons
        .iter()
        .map(|reason| (reason.field.clone(), reason.message.clone()))
        .collect()
}

fn refused(body: &[u8]) -> Error {
    Error::from_response(
        StatusCode::UNPROCESSABLE_ENTITY,
        &Method::POST,
        &HeaderMap::new(),
        body,
    )
}

#[tokio::test]
async fn a_refused_habit_keeps_each_reason_hey_gave() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendar/habits.json"))
        .respond_with(
            ResponseTemplate::new(422)
                .insert_header("x-request-id", "req-422")
                .set_body_json(habit_refusal()),
        )
        .expect(1)
        .mount(&server)
        .await;

    let error = client(&server)
        .habits()
        .create_habit(&HabitParams::default())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Validation);
    assert_eq!(error.http_status(), Some(422));
    assert!(!error.is_retryable());
    assert_eq!(error.request_id(), Some("req-422"));
    assert_eq!(
        read(&error.field_errors()),
        vec![
            reason(None, "Title can't be blank"),
            reason(None, "Days must include at least one day"),
        ]
    );
    assert_eq!(
        error.hint(),
        Some("Title can't be blank; Days must include at least one day")
    );
}

#[tokio::test]
async fn a_raw_write_refused_keeps_each_reason_hey_gave() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendar/habits.json"))
        .respond_with(ResponseTemplate::new(422).set_body_json(habit_refusal()))
        .mount(&server)
        .await;

    let error = client(&server)
        .post("/calendar/habits.json", &json!({ "calendar_habit": {} }))
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Validation);
    assert_eq!(
        read(&error.field_errors()),
        vec![
            reason(None, "Title can't be blank"),
            reason(None, "Days must include at least one day"),
        ]
    );
}

#[tokio::test]
async fn a_refused_contact_note_keeps_its_reasons_and_request_id() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/contacts/91825/note.json"))
        .respond_with(
            ResponseTemplate::new(422)
                .insert_header("x-request-id", "req-note")
                .set_body_json(json!({ "errors": ["Note is too long"] })),
        )
        .mount(&server)
        .await;

    let error = client(&server)
        .contacts()
        .set_note(91825, "Prefers a call")
        .await
        .unwrap_err();

    assert_eq!(error.to_string(), "Note is too long");
    assert_eq!(error.request_id(), Some("req-note"));
    assert_eq!(
        read(&error.field_errors()),
        vec![reason(None, "Note is too long")]
    );
}

#[test]
fn every_shape_of_reason_reads() {
    let cases: [(&[u8], Vec<Reason>); 7] = [
        (
            br#"{"errors":[{"field":"subject","message":"is too long"},{"attribute":"content","message":"can't be blank"}]}"#,
            vec![
                reason(Some("subject"), "is too long"),
                reason(Some("content"), "can't be blank"),
            ],
        ),
        (
            br#"{"errors":["Title can't be blank", 7, {"field":"days"}, "", null]}"#,
            vec![reason(None, "Title can't be blank")],
        ),
        (
            br#"{"error":"Subject can't be blank"}"#,
            vec![reason(None, "Subject can't be blank")],
        ),
        (
            br#"{"errors":{"subject":["is too long"]},"message":"Invalid message"}"#,
            vec![reason(None, "Invalid message")],
        ),
        (b"", vec![]),
        (b"<html>Unprocessable</html>", vec![]),
        (br#"{"errors":[]}"#, vec![]),
    ];

    for (body, expected) in cases {
        assert_eq!(
            read(&refused(body).field_errors()),
            expected,
            "{}",
            String::from_utf8_lossy(body)
        );
    }
}

#[test]
fn a_reason_naming_its_field_reads_with_it() {
    let error = refused(br#"{"errors":[{"field":"subject","message":"is too long"}]}"#);

    assert_eq!(error.field_errors()[0].to_string(), "subject: is too long");
    assert_eq!(error.hint(), Some("subject: is too long"));
}

#[test]
fn only_a_422_has_field_errors() {
    let conflict = Error::from_response(
        StatusCode::CONFLICT,
        &Method::POST,
        &HeaderMap::new(),
        br#"{"errors":["Email address is taken"]}"#,
    );

    assert!(conflict.field_errors().is_empty());
}
