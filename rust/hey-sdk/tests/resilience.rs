mod support;

use std::sync::{Arc, Mutex};
use std::time::Duration;

use hey_sdk::observability::{Hooks, OperationInfo, OperationState, RequestInfo, RequestResult};
use hey_sdk::resilience::{
    BulkheadConfig, CircuitBreakerConfig, RateLimitConfig, ResilienceConfig,
};
use hey_sdk::{Error, ErrorCode};

use async_trait::async_trait;
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::builder;

#[tokio::test]
async fn a_scope_that_keeps_failing_is_given_up_on_before_the_next_call_is_sent() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(500))
        .mount(&server)
        .await;
    let client = builder(&server)
        .max_retries(0)
        .circuit_breaker(CircuitBreakerConfig {
            failure_threshold: 2,
            ..CircuitBreakerConfig::default()
        })
        .build()
        .unwrap();

    assert_eq!(
        ErrorCode::Api,
        client.boxes().list().await.unwrap_err().code()
    );
    assert_eq!(
        ErrorCode::Api,
        client.boxes().list().await.unwrap_err().code()
    );

    let refused = client.boxes().list().await.unwrap_err();
    assert_eq!(ErrorCode::CircuitOpen, refused.code());
    assert_eq!("circuit breaker is open", refused.message());
    assert_eq!(2, server.received_requests().await.unwrap().len());
}

#[tokio::test]
async fn a_breaker_that_is_open_for_one_operation_leaves_the_others_alone() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(500))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes/123.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 123, "kind": "imbox", "name": "Imbox" })),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .max_retries(0)
        .circuit_breaker(CircuitBreakerConfig {
            failure_threshold: 1,
            ..CircuitBreakerConfig::default()
        })
        .build()
        .unwrap();

    client.boxes().list().await.unwrap_err();

    assert_eq!(
        ErrorCode::CircuitOpen,
        client.boxes().list().await.unwrap_err().code()
    );
    assert!(client.boxes().get(123, &Default::default()).await.is_ok());
}

#[tokio::test]
async fn a_scope_already_running_all_it_may_refuses_the_call_beside_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([]))
                .set_delay(Duration::from_millis(200)),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .bulkhead(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::ZERO,
        })
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());

    assert!(first.is_ok());
    let refused = second.unwrap_err();
    assert_eq!(ErrorCode::BulkheadFull, refused.code());
    assert_eq!("bulkhead is full", refused.message());
    assert_eq!(1, server.received_requests().await.unwrap().len());

    assert!(client.boxes().list().await.is_ok());
}

/// A bulkhead with a wait to spend holds the call beside it at the gate rather than turning
/// it away, and sends it as soon as the call in flight gives its place back.
#[tokio::test]
async fn a_scope_with_a_wait_to_spend_holds_the_call_beside_it_until_there_is_room() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([]))
                .set_delay(Duration::from_millis(50)),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .bulkhead(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::from_millis(500),
        })
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());

    assert!(first.is_ok());
    assert!(second.is_ok());
    assert_eq!(2, server.received_requests().await.unwrap().len());
}

#[tokio::test]
async fn a_scope_that_is_still_busy_when_the_wait_runs_out_refuses_the_call_waiting_on_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([]))
                .set_delay(Duration::from_millis(700)),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .bulkhead(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::from_millis(500),
        })
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());

    assert!(first.is_ok());
    let refused = second.unwrap_err();
    assert_eq!(ErrorCode::BulkheadFull, refused.code());
    assert_eq!("bulkhead is full", refused.message());
    assert_eq!(1, server.received_requests().await.unwrap().len());
}

#[tokio::test]
async fn a_client_that_has_spent_its_budget_refuses_before_it_sends() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let client = builder(&server)
        .rate_limit(RateLimitConfig {
            requests_per_second: 0.0001,
            burst_size: 1,
            ..RateLimitConfig::default()
        })
        .build()
        .unwrap();

    client.boxes().list().await.unwrap();

    let refused = client.boxes().list().await.unwrap_err();
    assert_eq!(ErrorCode::RateLimit, refused.code());
    assert_eq!("rate limit exceeded", refused.message());
    assert_eq!(None, refused.http_status());
    assert_eq!(1, server.received_requests().await.unwrap().len());
}

#[tokio::test]
async fn the_wait_hey_asked_for_holds_the_next_call_back() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(429).insert_header("Retry-After", "30"))
        .mount(&server)
        .await;
    let client = builder(&server)
        .max_retries(0)
        .rate_limit(RateLimitConfig::default())
        .build()
        .unwrap();

    let answered = client.boxes().list().await.unwrap_err();
    assert_eq!(ErrorCode::RateLimit, answered.code());
    assert_eq!(Some(429), answered.http_status());

    let refused = client.boxes().list().await.unwrap_err();
    assert_eq!(ErrorCode::RateLimit, refused.code());
    assert_eq!("rate limit exceeded", refused.message());
    assert_eq!(None, refused.http_status());
    assert_eq!(1, server.received_requests().await.unwrap().len());
}

/// Go's chain gates through the first member that can gate and stops, so installing two
/// layers there quietly runs only the outer one. Every layer installed here is asked.
#[tokio::test]
async fn every_layer_installed_gates_the_call() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(500))
        .mount(&server)
        .await;
    let client = builder(&server)
        .max_retries(0)
        .circuit_breaker(CircuitBreakerConfig {
            failure_threshold: 1,
            ..CircuitBreakerConfig::default()
        })
        .rate_limit(RateLimitConfig::default())
        .build()
        .unwrap();

    client.boxes().list().await.unwrap_err();

    let refused = client.boxes().list().await.unwrap_err();
    assert_eq!(ErrorCode::CircuitOpen, refused.code());
    assert_eq!(1, server.received_requests().await.unwrap().len());
}

#[tokio::test]
async fn the_hooks_that_were_already_on_hear_everything_they_would_have() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let recorder = Arc::new(Recorder::default());

    builder(&server)
        .hooks(recorder.clone())
        .resilience(ResilienceConfig::default())
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap();

    assert_eq!(
        [
            "gate Boxes.ListBoxes",
            "start Boxes.ListBoxes",
            "request start GET",
            "request end 200",
            "end Boxes.ListBoxes carrying its own",
        ],
        recorder.entries().as_slice()
    );
}

/// A caller can walk away from a call — a `timeout` that expired, a `select!` that took
/// another branch — and the future is dropped where it stands. The operation still has to
/// end: a start with no end leaves the scope's bulkhead a permit short for the life of the
/// client, and the next call to that scope would be refused for room that is not taken.
#[tokio::test]
async fn a_call_the_caller_gave_up_on_still_ends_and_gives_back_its_permit() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([]))
                .set_delay(Duration::from_millis(300)),
        )
        .mount(&server)
        .await;
    let lifecycle = Arc::new(Lifecycle::default());
    let client = builder(&server)
        .hooks(lifecycle.clone())
        .bulkhead(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::ZERO,
        })
        .build()
        .unwrap();

    let abandoned = tokio::time::timeout(Duration::from_millis(30), client.boxes().list()).await;

    assert!(abandoned.is_err(), "the call was meant to outlast the wait");
    assert_eq!(
        [
            "start Boxes.ListBoxes",
            "end Boxes.ListBoxes: operation cancelled"
        ],
        lifecycle.entries().as_slice()
    );

    client.boxes().list().await.unwrap();

    assert_eq!(
        [
            "start Boxes.ListBoxes",
            "end Boxes.ListBoxes: operation cancelled",
            "start Boxes.ListBoxes",
            "end Boxes.ListBoxes: ok",
        ],
        lifecycle.entries().as_slice()
    );
}

/// Every operation's start and how it ended, for the calls where the ending is the point.
#[derive(Default)]
struct Lifecycle {
    entries: Mutex<Vec<String>>,
}

impl Lifecycle {
    fn entries(&self) -> Vec<String> {
        self.entries.lock().unwrap().clone()
    }
}

impl Hooks for Lifecycle {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.entries
            .lock()
            .unwrap()
            .push(format!("start {}.{}", op.service, op.operation));
        None
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        _state: OperationState,
        outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        let ended = match outcome {
            Ok(()) => "ok".to_string(),
            Err(error) => error.message().to_string(),
        };
        self.entries
            .lock()
            .unwrap()
            .push(format!("end {}.{}: {ended}", op.service, op.operation));
    }
}

/// Hooks installed before the resilience layers, kept as their inner hooks.
#[derive(Default)]
struct Recorder {
    entries: Mutex<Vec<String>>,
}

impl Recorder {
    fn entries(&self) -> Vec<String> {
        self.entries.lock().unwrap().clone()
    }

    fn record(&self, entry: String) {
        self.entries.lock().unwrap().push(entry);
    }
}

#[async_trait]
impl Hooks for Recorder {
    async fn on_operation_gate(&self, op: &OperationInfo) -> Result<(), Error> {
        self.record(format!("gate {}.{}", op.service, op.operation));
        Ok(())
    }

    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.record(format!("start {}.{}", op.service, op.operation));
        Some(Box::new("its own"))
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        state: OperationState,
        _outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        let carried = match state.and_then(|state| state.downcast::<&str>().ok()) {
            Some(carried) => *carried,
            None => "nothing",
        };
        self.record(format!(
            "end {}.{} carrying {carried}",
            op.service, op.operation
        ));
    }

    fn on_request_start(&self, info: &RequestInfo) {
        self.record(format!("request start {}", info.method));
    }

    fn on_request_end(&self, _info: &RequestInfo, result: &RequestResult<'_>) {
        self.record(format!("request end {}", result.status.unwrap().as_u16()));
    }
}
