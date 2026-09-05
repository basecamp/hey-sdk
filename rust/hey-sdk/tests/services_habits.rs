mod support;

use hey_sdk::services::HabitParams;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_new_habit_names_the_days_it_runs_on() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendar/habits.json"))
        .respond_with(ResponseTemplate::new(201).set_body_json(json!({
            "id": 7712,
            "type": "Calendar::Habit",
            "title": "Morning run",
            "days": [1, 3, 5]
        })))
        .mount(&server)
        .await;
    let params = HabitParams {
        name: "Morning run".to_string(),
        icon: "runner".to_string(),
        days: vec![1, 3, 5],
        ..HabitParams::default()
    };

    let habit = client(&server)
        .habits()
        .create_habit(&params)
        .await
        .unwrap();

    assert_eq!(habit.id, 7712);
    assert!(habit.is_calendar_habit());
    assert_eq!(habit.days, Some(vec![1, 3, 5]));
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "calendar_habit": { "name": "Morning run", "icon": "runner", "days": [1, 3, 5] } })
    );
}

#[tokio::test]
async fn an_edited_habit_leaves_out_what_it_was_not_given() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/calendar/habits/7712.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(
            json!({ "id": 7712, "type": "Calendar::Habit", "title": "Evening run" }),
        ))
        .mount(&server)
        .await;
    let params = HabitParams {
        name: "Evening run".to_string(),
        ..HabitParams::default()
    };

    let habit = client(&server)
        .habits()
        .update_habit(7712, &params)
        .await
        .unwrap();

    assert_eq!(habit.title.as_deref(), Some("Evening run"));
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "calendar_habit": { "name": "Evening run" } })
    );
}

async fn sent_json(server: &MockServer, index: usize) -> Value {
    let requests = server.received_requests().await.unwrap();
    serde_json::from_slice(&requests[index].body).unwrap()
}
