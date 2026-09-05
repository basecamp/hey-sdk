mod support;

use std::sync::{Arc, Mutex};

use hey_sdk::ErrorCode;
use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

#[tokio::test]
async fn starting_a_second_track_is_a_conflict_carrying_heys_own_message() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendar/ongoing_time_track.json"))
        .respond_with(
            ResponseTemplate::new(409)
                .set_body_json(json!({ "error": "Ongoing time track already in progress" })),
        )
        .mount(&server)
        .await;

    let refused = client(&server)
        .time_tracks()
        .start_tracking()
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Conflict);
    assert_eq!(refused.http_status(), Some(409));
    assert_eq!(refused.message(), "Ongoing time track already in progress");
}

#[tokio::test]
async fn starting_the_first_track_answers_the_recording() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendar/ongoing_time_track.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 1, "type": "Calendar::TimeTrack" })),
        )
        .mount(&server)
        .await;

    let track = client(&server)
        .time_tracks()
        .start_tracking()
        .await
        .unwrap();

    assert_eq!(track.id, 1);
}

#[tokio::test]
async fn stopping_a_track_ends_it_and_touches_nothing_else() {
    let server = MockServer::start().await;
    mock_update(&server).await;

    client(&server).time_tracks().stop(1).await.unwrap();

    let track = sent_time_track(&server).await;
    assert!(track["ends_at"].is_string());
    assert_eq!(track.as_object().unwrap().len(), 1);
}

#[tokio::test]
async fn filing_a_track_on_the_way_out_is_the_same_one_request() {
    let server = MockServer::start().await;
    mock_update(&server).await;

    client(&server)
        .time_tracks()
        .stop_and_file(1, Some("Client work"))
        .await
        .unwrap();

    let track = sent_time_track(&server).await;
    assert_eq!(track["category_title"], "Client work");
    assert!(track["ends_at"].is_string());
    assert_eq!(track.as_object().unwrap().len(), 2);
}

/// An empty title names no category, and stops the track without filing it — the same as
/// naming none at all.
#[tokio::test]
async fn an_empty_category_files_the_track_nowhere() {
    let server = MockServer::start().await;
    mock_update(&server).await;

    client(&server)
        .time_tracks()
        .stop_and_file(1, Some(""))
        .await
        .unwrap();

    let track = sent_time_track(&server).await;
    assert!(track["ends_at"].is_string());
    assert_eq!(track.as_object().unwrap().len(), 1);
}

#[tokio::test]
async fn a_stop_announces_itself_as_stopping_rather_than_updating() {
    let server = MockServer::start().await;
    mock_update(&server).await;
    let recorder = Recorder::new();
    let client = builder(&server).hooks(recorder.clone()).build().unwrap();

    client.time_tracks().stop(1).await.unwrap();
    client
        .time_tracks()
        .stop_and_file(2, Some("Client work"))
        .await
        .unwrap();

    assert_eq!(recorder.operations(), ["StopTimeTrack", "StopTimeTrack"]);
    assert_eq!(recorder.resource_ids(), [Some(1), Some(2)]);
}

async fn mock_update(server: &MockServer) {
    Mock::given(method("PUT"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 1, "type": "Calendar::TimeTrack" })),
        )
        .mount(server)
        .await;
}

async fn sent_time_track(server: &MockServer) -> Value {
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].url.path(), "/calendar/time_tracks/1.json");
    let body: Value = serde_json::from_slice(&requests[0].body).unwrap();
    body["calendar_time_track"].clone()
}

#[derive(Clone, Default)]
struct Recorder {
    started: Arc<Mutex<Vec<OperationInfo>>>,
}

impl Recorder {
    fn new() -> Recorder {
        Recorder::default()
    }

    fn operations(&self) -> Vec<String> {
        self.started
            .lock()
            .unwrap()
            .iter()
            .map(|info| info.operation.to_string())
            .collect()
    }

    fn resource_ids(&self) -> Vec<Option<i64>> {
        self.started
            .lock()
            .unwrap()
            .iter()
            .map(|info| info.resource_id)
            .collect()
    }
}

impl Hooks for Recorder {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.started.lock().unwrap().push(op.clone());
        None
    }
}
