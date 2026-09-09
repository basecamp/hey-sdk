mod support;

use hey_sdk::services::TodoChanges;
use hey_sdk::{Date, ErrorCode};
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_new_todo_carries_its_day_as_a_bare_date() {
    let server = MockServer::start().await;
    mock_create(&server).await;

    let todo = client(&server)
        .calendar_todos()
        .create_todo("Renew the passport", Date::new(2026, 3, 4))
        .await
        .unwrap();

    assert_eq!(todo.id, 1);
    assert_eq!(
        sent_body(&server).await,
        json!({ "calendar_todo": { "title": "Renew the passport", "starts_at": "2026-03-04" } })
    );
}

#[tokio::test]
async fn a_new_todo_without_a_day_is_filed_on_today() {
    let server = MockServer::start().await;
    mock_create(&server).await;

    client(&server)
        .calendar_todos()
        .create_todo("Water the plants", None)
        .await
        .unwrap();

    let body = sent_body(&server).await;
    assert_eq!(
        body["calendar_todo"]["starts_at"],
        json!(Date::today().to_string())
    );
}

#[tokio::test]
async fn an_edit_sends_only_what_it_changes() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/calendar/todos/1.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "id": 1, "type": "Calendar::Todo" })),
        )
        .mount(&server)
        .await;
    let rename = TodoChanges {
        title: Some("Renew the passport".to_string()),
        ..TodoChanges::default()
    };

    client(&server)
        .calendar_todos()
        .update_todo(1, &rename)
        .await
        .unwrap();

    assert_eq!(
        sent_body(&server).await,
        json!({ "calendar_todo": { "title": "Renew the passport" } })
    );
}

#[tokio::test]
async fn a_rescheduling_edit_carries_a_bare_date_too() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/calendar/todos/1.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "id": 1, "type": "Calendar::Todo" })),
        )
        .mount(&server)
        .await;
    let changes = TodoChanges {
        starts_at: Date::new(2026, 3, 13),
        focused: Some(true),
        ..TodoChanges::default()
    };

    client(&server)
        .calendar_todos()
        .update_todo(1, &changes)
        .await
        .unwrap();

    assert_eq!(
        sent_body(&server).await,
        json!({ "calendar_todo": { "starts_at": "2026-03-13", "focused": true } })
    );
}

#[tokio::test]
async fn an_edit_that_changes_nothing_is_refused_before_it_is_sent() {
    let server = MockServer::start().await;

    let refused = client(&server)
        .calendar_todos()
        .update_todo(1, &TodoChanges::default())
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert_eq!(
        refused.message(),
        "update calendar todo 1: nothing to change"
    );
    assert!(server.received_requests().await.unwrap().is_empty());
}

/// An empty title is no title — HEY refuses a todo without one — so it changes nothing
/// rather than clearing the title the todo has.
#[tokio::test]
async fn an_empty_title_changes_nothing_rather_than_clearing_it() {
    let server = MockServer::start().await;

    let refused = client(&server)
        .calendar_todos()
        .update_todo(
            1,
            &TodoChanges {
                title: Some(String::new()),
                ..TodoChanges::default()
            },
        )
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert!(server.received_requests().await.unwrap().is_empty());
}

async fn mock_create(server: &MockServer) {
    Mock::given(method("POST"))
        .and(path("/calendar/todos.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "id": 1, "type": "Calendar::Todo" })),
        )
        .mount(server)
        .await;
}

async fn sent_body(server: &MockServer) -> Value {
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    serde_json::from_slice(&requests[0].body).unwrap()
}
