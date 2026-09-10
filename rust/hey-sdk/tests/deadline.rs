//! The operation limit: one deadline over the whole call, distinct from the transport's
//! timeout, and what a call cut off at it leaves behind.

mod support;

use std::sync::Arc;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use hey_sdk::resilience::{BulkheadConfig, ResilienceConfig};
use hey_sdk::{Client, Config, Error, ErrorCode, StaticTokenProvider, TokenProvider};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use std::sync::Mutex;

use hey_sdk::observability::{Hooks, RequestInfo, RequestResult};

use support::{Outcomes, builder};

/// The operation limit is not the transport timeout: the transport would have waited the
/// full thirty seconds for this answer, and the operation stops at its own limit.
#[tokio::test]
async fn an_operation_ends_at_its_limit_while_the_transport_would_still_wait() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_secs(5))
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .timeout(Duration::from_secs(30))
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let started = Instant::now();
    let error = client.boxes().list().await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Network);
    assert!(error.is_retryable());
    assert!(error.message().contains("timed out"), "{error}");
    assert!(started.elapsed() < Duration::from_secs(2));
}

/// The limit covers the waits between attempts, not only the attempts.
#[tokio::test]
async fn the_limit_covers_the_backoff_between_attempts() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;
    let client = Client::builder(Config::default().with_base_url(server.uri()))
        .token_provider(StaticTokenProvider::new("t"))
        .http_client(support::http_client())
        .base_delay(Duration::from_secs(5))
        .max_jitter(Duration::ZERO)
        .operation_timeout(Duration::from_millis(200))
        .build()
        .unwrap();

    let started = Instant::now();
    let error = client.boxes().list().await.unwrap_err();

    assert!(error.message().contains("timed out"), "{error}");
    assert!(started.elapsed() < Duration::from_secs(2));
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// The limit covers fetching the credentials, before anything is sent.
#[tokio::test]
async fn the_limit_covers_fetching_the_credentials() {
    struct Slow;

    #[async_trait]
    impl TokenProvider for Slow {
        async fn access_token(&self) -> Result<String, Error> {
            tokio::time::sleep(Duration::from_secs(5)).await;
            Ok("late".to_string())
        }
    }

    let server = MockServer::start().await;
    let client = Client::builder(Config::default().with_base_url(server.uri()))
        .token_provider(Slow)
        .http_client(support::http_client())
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let started = Instant::now();
    let error = client.boxes().list().await.unwrap_err();

    assert!(error.message().contains("timed out"), "{error}");
    assert!(started.elapsed() < Duration::from_secs(2));
    assert!(server.received_requests().await.unwrap().is_empty());
}

/// An operation cut off at its limit still ends for the hooks, and gives back what it
/// held: the next call into the same scope gets the bulkhead's one permit.
#[tokio::test]
async fn an_operation_cut_off_at_its_limit_ends_and_gives_back_its_permit() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_secs(5))
                .set_body_json(json!([])),
        )
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let outcomes = Arc::new(Outcomes::default());
    let client = builder(&server)
        .hooks(outcomes.clone())
        .resilience(ResilienceConfig {
            bulkhead: Some(BulkheadConfig {
                max_concurrent: 1,
                max_wait: Duration::ZERO,
            }),
            ..ResilienceConfig::default()
        })
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let error = client.boxes().list().await.unwrap_err();
    client.boxes().list().await.unwrap();

    assert!(error.message().contains("timed out"), "{error}");
    assert_eq!(outcomes.statuses().len(), 2);
}

/// The limit holds over the bytes of a download too, though the hooks hear the operation
/// end when the answer arrives.
#[tokio::test]
async fn the_limit_covers_the_body_of_a_download() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/blobs/1"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_secs(5))
                .set_body_bytes(vec![0u8; 16]),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let started = Instant::now();
    let mut sink = Vec::new();
    let error = client
        .download_blob("/blobs/1", &mut sink)
        .await
        .unwrap_err();

    assert!(error.message().contains("timed out"), "{error}");
    assert!(started.elapsed() < Duration::from_secs(2));
}

#[test]
fn a_limit_too_long_to_keep_time_by_is_refused() {
    let error = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(StaticTokenProvider::new("t"))
        .operation_timeout(Duration::MAX)
        .build()
        .err()
        .expect("a limit past the end of time is a usage error");

    assert_eq!(error.code(), ErrorCode::Usage);
}

#[test]
fn a_zero_limit_is_refused() {
    let error = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(StaticTokenProvider::new("t"))
        .operation_timeout(Duration::ZERO)
        .build()
        .err()
        .expect("a zero limit is a usage error");

    assert_eq!(error.code(), ErrorCode::Usage);
}

/// Every request the hooks were told the start of, and how it ended.
#[derive(Default)]
struct Requests {
    started: Mutex<u32>,
    ended: Mutex<Vec<String>>,
}

impl Hooks for Requests {
    fn on_request_start(&self, _info: &RequestInfo) {
        *self.started.lock().unwrap() += 1;
    }

    fn on_request_end(&self, _info: &RequestInfo, result: &RequestResult<'_>) {
        self.ended
            .lock()
            .unwrap()
            .push(match (result.status, result.error) {
                (Some(status), _) => status.as_u16().to_string(),
                (None, Some(error)) => error.message().to_string(),
                (None, None) => "nothing".to_string(),
            });
    }
}

/// A request the limit cuts off mid-flight still ends for the hooks, as cancelled, so a
/// hook counting requests in flight is never left one short.
#[tokio::test]
async fn a_request_cut_off_by_the_limit_ends_for_the_hooks() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_secs(5))
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;
    let requests = Arc::new(Requests::default());
    let client = builder(&server)
        .hooks(requests.clone())
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    client.boxes().list().await.unwrap_err();

    assert_eq!(*requests.started.lock().unwrap(), 1);
    assert_eq!(*requests.ended.lock().unwrap(), ["operation cancelled"]);
}

/// The hooks hear an operation that ran out of time end with the timeout the caller
/// gets, not as a cancellation: a policy that reads the outcome sees what happened.
#[tokio::test]
async fn the_hooks_hear_the_timeout_the_caller_gets() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_secs(5))
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;
    let endings = Arc::new(Endings::default());
    let client = builder(&server)
        .hooks(endings.clone())
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let error = client.boxes().list().await.unwrap_err();

    assert_eq!(
        *endings.messages.lock().unwrap(),
        [error.message().to_string()]
    );
    assert!(error.message().contains("timed out"), "{error}");
}

/// A convenience that sends two requests is one operation to its caller, and gets one
/// limit over both — not one each.
#[tokio::test]
async fn a_convenience_of_two_requests_gets_one_limit() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/5/publication"))
        .respond_with(
            ResponseTemplate::new(302)
                .set_delay(Duration::from_millis(70))
                .insert_header("Location", "/topics/5"),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/topics/5/publication.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_millis(70))
                .set_body_json(json!({ "published": true, "url": "https://public.hey.com/p/a" })),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let started = Instant::now();
    let error = client.publications().publish(5).await.unwrap_err();

    assert!(error.message().contains("timed out"), "{error}");
    assert!(started.elapsed() < Duration::from_millis(500));
}

/// The hooks hear the operation inside a two-request convenience end with the timeout,
/// not as a cancellation: the inner send runs against the same deadline as the whole.
#[tokio::test]
async fn the_hooks_hear_the_timeout_inside_a_two_request_convenience() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/5/publication"))
        .respond_with(
            ResponseTemplate::new(302)
                .set_delay(Duration::from_secs(5))
                .insert_header("Location", "/topics/5"),
        )
        .mount(&server)
        .await;
    let endings = Arc::new(Endings::default());
    let client = builder(&server)
        .hooks(endings.clone())
        .operation_timeout(Duration::from_millis(100))
        .build()
        .unwrap();

    let error = client.publications().publish(5).await.unwrap_err();

    assert!(error.message().contains("timed out"), "{error}");
    assert_eq!(
        *endings.messages.lock().unwrap(),
        [error.message().to_string()]
    );
}

/// A walk over many pages is one operation with one limit, not a limit per page.
#[tokio::test]
async fn a_walk_over_many_pages_gets_one_limit() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_millis(60))
                .insert_header("Link", r#"</items?page=next>; rel="next""#)
                .set_body_json(json!([{ "id": 1 }])),
        )
        .mount(&server)
        .await;
    let client = builder(&server)
        .operation_timeout(Duration::from_millis(150))
        .build()
        .unwrap();

    let started = Instant::now();
    let error = client.get_all("/items").await.unwrap_err();

    assert!(error.message().contains("timed out"), "{error}");
    assert!(started.elapsed() < Duration::from_millis(600));
    assert!(server.received_requests().await.unwrap().len() <= 3);
}

#[derive(Default)]
struct Endings {
    messages: Mutex<Vec<String>>,
}

impl Hooks for Endings {
    fn on_operation_end(
        &self,
        _op: &hey_sdk::observability::OperationInfo,
        _state: hey_sdk::observability::OperationState,
        outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        if let Err(error) = outcome {
            self.messages
                .lock()
                .unwrap()
                .push(error.message().to_string());
        }
    }
}
