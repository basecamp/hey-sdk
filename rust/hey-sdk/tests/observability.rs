mod support;

use std::sync::{Arc, Mutex};
use std::time::Duration;

use hey_sdk::cache::InMemoryCache;
use hey_sdk::observability::{
    ChainHooks, Hooks, NoopHooks, OperationInfo, OperationState, RequestInfo, RequestResult,
};
use hey_sdk::{Client, Config, Error, ErrorCode, StaticTokenProvider, TokenProvider};

use async_trait::async_trait;
use serde_json::json;
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::builder;

#[tokio::test]
async fn a_read_is_announced_once_around_the_request_it_takes() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    builder(&server)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap();

    let started = recorder.operations();
    assert_eq!(started.len(), 1);
    assert_eq!(started[0].service, "Boxes");
    assert_eq!(started[0].operation, "ListBoxes");
    assert_eq!(started[0].resource_type, "box");
    assert!(!started[0].is_mutation);
    assert_eq!(started[0].resource_id, None);
    assert_eq!(recorder.endings(), [None]);
    assert_eq!(recorder.attempts(), ["GET /boxes.json 1"]);
    assert_eq!(recorder.answers(), ["1 200 retryable=false"]);
    assert!(recorder.retries().is_empty());
}

#[tokio::test]
async fn a_write_says_so_and_names_the_record_it_changes() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/calendar/habits/456.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    builder(&server)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .habits()
        .delete(456)
        .await
        .unwrap();

    let started = recorder.operations();
    assert_eq!(started.len(), 1);
    assert_eq!(started[0].service, "Habits");
    assert_eq!(started[0].operation, "DeleteHabit");
    assert_eq!(started[0].resource_type, "habit");
    assert!(started[0].is_mutation);
    assert_eq!(started[0].resource_id, Some(456));
}

#[tokio::test]
async fn a_read_of_one_record_names_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/123.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 123, "kind": "imbox", "name": "Imbox" })),
        )
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    builder(&server)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .boxes()
        .get(123, &Default::default())
        .await
        .unwrap();

    assert_eq!(recorder.operations()[0].resource_id, Some(123));
}

#[tokio::test]
async fn every_attempt_is_announced_with_its_number_and_each_resend_with_its_cause() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(503))
        .up_to_n_times(2)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    builder(&server)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap();

    assert_eq!(
        recorder.attempts(),
        [
            "GET /boxes.json 1",
            "GET /boxes.json 2",
            "GET /boxes.json 3"
        ]
    );
    assert_eq!(
        recorder.answers(),
        [
            "1 503 retryable=true",
            "2 503 retryable=true",
            "3 200 retryable=false"
        ]
    );
    assert_eq!(
        recorder.retries(),
        [
            "1 -> 2 api_error API error: 503 Service Unavailable retryable",
            "2 -> 3 api_error API error: 503 Service Unavailable retryable"
        ]
    );
    assert_eq!(recorder.endings(), [None]);
}

#[tokio::test]
async fn a_resend_after_a_refreshed_credential_is_a_retry_of_its_own() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("authorization", "Bearer stale-token"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("authorization", "Bearer fresh-token"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    builder(&server)
        .token_provider(RefreshingToken::new())
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap();

    assert_eq!(
        recorder.attempts(),
        ["GET /boxes.json 1", "GET /boxes.json 2"]
    );
    assert_eq!(
        recorder.retries(),
        ["1 -> 2 auth_required Token refreshed retryable"]
    );
}

#[tokio::test]
async fn a_request_that_never_arrives_is_a_retry_with_a_network_error() {
    let recorder = Recorder::new();
    let client = Client::builder(Config::default().with_base_url("http://127.0.0.1:1"))
        .token_provider(StaticTokenProvider::new("t"))
        .base_delay(Duration::from_millis(1))
        .max_jitter(Duration::ZERO)
        .max_retries(1)
        .hooks(recorder.clone())
        .build()
        .unwrap();

    let error = client.boxes().list().await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Network);
    assert_eq!(
        recorder.answers(),
        ["1 - retryable=true", "2 - retryable=true"]
    );
    assert_eq!(recorder.retries().len(), 1);
    assert!(
        recorder.retries()[0].starts_with("1 -> 2 network Network error"),
        "reported {:?}",
        recorder.retries()[0]
    );
    assert_eq!(recorder.endings(), [Some("Network error".to_string())]);
}

#[tokio::test]
async fn a_gate_that_refuses_stops_the_operation_before_anything_is_sent() {
    let server = MockServer::start().await;
    let recorder = Recorder::new();
    let hooks = ChainHooks::of(vec![Arc::new(Refusing), recorder.clone()]);

    let error = builder(&server)
        .hooks(hooks)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert_eq!(error.message(), "blocked");
    assert!(server.received_requests().await.unwrap().is_empty());
    assert!(recorder.operations().is_empty());
    assert!(recorder.endings().is_empty());
}

#[tokio::test]
async fn the_end_hook_is_handed_what_the_start_hook_made() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let stateful = Arc::new(Stateful::default());

    let client = builder(&server).hooks(stateful.clone()).build().unwrap();
    client.boxes().list().await.unwrap();
    client.boxes().list().await.unwrap();

    assert_eq!(stateful.carried(), ["ListBoxes 1", "ListBoxes 2"]);
}

#[tokio::test]
async fn a_body_read_from_the_cache_says_so() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1\""))
        .respond_with(ResponseTemplate::new(304).insert_header("ETag", "\"v1\""))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    let client = builder(&server)
        .cache(InMemoryCache::new())
        .hooks(recorder.clone())
        .build()
        .unwrap();
    client.boxes().list().await.unwrap();
    client.boxes().list().await.unwrap();

    assert_eq!(recorder.cached(), [false, true]);
}

#[tokio::test]
async fn a_rate_limited_request_carries_the_wait_the_server_asked_for() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(429).insert_header("Retry-After", "17"))
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    let error = builder(&server)
        .max_retries(0)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::RateLimit);
    assert_eq!(recorder.waits(), [Some(17)]);
    assert_eq!(recorder.answers(), ["1 429 retryable=true"]);
    assert_eq!(recorder.endings(), [Some(error.message().to_string())]);
}

#[tokio::test]
async fn a_client_told_to_use_noop_hooks_reports_nothing() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;

    builder(&server)
        .hooks(NoopHooks)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap();

    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn a_wrapper_can_announce_itself_as_something_other_than_the_route_it_sends() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/calendar/time_tracks/701.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "id": 701 })))
        .mount(&server)
        .await;
    let recorder = Recorder::new();
    let client = builder(&server).hooks(recorder.clone()).build().unwrap();

    let mut operation = client.operation(&hey_sdk::routes::UPDATE_TIME_TRACK, &[&701]);
    operation
        .operation_name("StopTimeTrack")
        .resource_type("time_track")
        .resource_id(701);
    client.send_unit(operation).await.unwrap();

    let started = recorder.operations();
    assert_eq!(started[0].service, "TimeTracks");
    assert_eq!(started[0].operation, "StopTimeTrack");
    assert_eq!(started[0].resource_type, "time_track");
    assert_eq!(started[0].resource_id, Some(701));
    assert!(started[0].is_mutation);
}

/// Everything the hooks were told, kept so a test can assert on it afterwards.
#[derive(Default)]
struct Recorder {
    operations: Mutex<Vec<OperationInfo>>,
    endings: Mutex<Vec<Option<String>>>,
    attempts: Mutex<Vec<String>>,
    answers: Mutex<Vec<String>>,
    cached: Mutex<Vec<bool>>,
    waits: Mutex<Vec<Option<u64>>>,
    retries: Mutex<Vec<String>>,
}

impl Recorder {
    fn new() -> Arc<Recorder> {
        Arc::new(Recorder::default())
    }

    fn operations(&self) -> Vec<OperationInfo> {
        self.operations.lock().unwrap().clone()
    }

    fn endings(&self) -> Vec<Option<String>> {
        self.endings.lock().unwrap().clone()
    }

    fn attempts(&self) -> Vec<String> {
        self.attempts.lock().unwrap().clone()
    }

    fn answers(&self) -> Vec<String> {
        self.answers.lock().unwrap().clone()
    }

    fn cached(&self) -> Vec<bool> {
        self.cached.lock().unwrap().clone()
    }

    fn waits(&self) -> Vec<Option<u64>> {
        self.waits.lock().unwrap().clone()
    }

    fn retries(&self) -> Vec<String> {
        self.retries.lock().unwrap().clone()
    }
}

impl Hooks for Recorder {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.operations.lock().unwrap().push(op.clone());
        None
    }

    fn on_operation_end(
        &self,
        _op: &OperationInfo,
        _state: OperationState,
        outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        let ending = outcome.err().map(|error| error.message().to_string());
        self.endings.lock().unwrap().push(ending);
    }

    fn on_request_start(&self, info: &RequestInfo) {
        self.attempts.lock().unwrap().push(format!(
            "{} {} {}",
            info.method,
            info.url.path(),
            info.attempt
        ));
    }

    fn on_request_end(&self, info: &RequestInfo, result: &RequestResult<'_>) {
        let status = match result.status {
            Some(status) => status.as_u16().to_string(),
            None => "-".to_string(),
        };
        self.answers.lock().unwrap().push(format!(
            "{} {status} retryable={}",
            info.attempt, result.retryable
        ));
        self.cached.lock().unwrap().push(result.from_cache);
        self.waits.lock().unwrap().push(result.retry_after);
    }

    fn on_retry(&self, info: &RequestInfo, next_attempt: u32, cause: &Error) {
        let retryable = match cause.is_retryable() {
            true => "retryable",
            false => "final",
        };
        self.retries.lock().unwrap().push(format!(
            "{} -> {next_attempt} {} {} {retryable}",
            info.attempt,
            cause.code(),
            cause.message()
        ));
    }
}

/// Hooks that hand their end a token their start made, to prove the two are paired.
#[derive(Default)]
struct Stateful {
    issued: Mutex<u32>,
    carried: Mutex<Vec<String>>,
}

impl Stateful {
    fn carried(&self) -> Vec<String> {
        self.carried.lock().unwrap().clone()
    }
}

impl Hooks for Stateful {
    fn on_operation_start(&self, _op: &OperationInfo) -> OperationState {
        let mut issued = self.issued.lock().unwrap();
        *issued += 1;
        Some(Box::new(*issued))
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        state: OperationState,
        _outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        let token = state
            .and_then(|state| state.downcast::<u32>().ok())
            .expect("the start hook's token");
        self.carried
            .lock()
            .unwrap()
            .push(format!("{} {token}", op.operation));
    }
}

struct Refusing;

#[async_trait]
impl Hooks for Refusing {
    async fn on_operation_gate(&self, _op: &OperationInfo) -> Result<(), Error> {
        Err(Error::usage("blocked"))
    }
}

/// A provider whose credentials can be renewed, so the first 401 is answered by a resend.
struct RefreshingToken {
    token: Mutex<String>,
}

impl RefreshingToken {
    fn new() -> RefreshingToken {
        RefreshingToken {
            token: Mutex::new("stale-token".to_string()),
        }
    }
}

#[async_trait]
impl TokenProvider for RefreshingToken {
    async fn access_token(&self) -> Result<String, Error> {
        Ok(self.token.lock().unwrap().clone())
    }

    async fn refresh(&self) -> bool {
        *self.token.lock().unwrap() = "fresh-token".to_string();
        true
    }
}
