mod support;

use std::sync::{Arc, Mutex};
use std::time::Duration;

use hey_sdk::Error;
use hey_sdk::observability::{Hooks, OperationInfo, OperationState, RequestInfo};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

/// The redirect lands on the sharing panel rather than carrying the link, so publishing
/// posts the form and then reads the publication back. The read-back is quiet, so the hooks
/// hear one `CreateTopicPublication` and see both requests it took, the way Go's
/// un-instrumented read does.
#[tokio::test]
async fn publishing_a_thread_reads_the_public_link_back_under_the_operation_that_asked_for_it() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/5/publication"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/topics/5"))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/topics/5/publication.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(
                json!({ "published": true, "url": "https://public.hey.com/p/abc123" }),
            ),
        )
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    let publication = builder(&server)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .publications()
        .publish(5)
        .await
        .unwrap();

    assert!(publication.published);
    assert_eq!(
        publication.url.as_deref(),
        Some("https://public.hey.com/p/abc123")
    );
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/topics/5/publication");
    assert!(requests[0].body.is_empty());
    assert_eq!(requests[1].url.path(), "/topics/5/publication.json");
    assert_eq!(
        recorder.transcript(),
        [
            "start Publications.CreateTopicPublication publication Some(5)",
            "request POST /topics/5/publication",
            "end Publications.CreateTopicPublication",
            "request GET /topics/5/publication.json",
        ]
    );
}

/// An account that may not publish is refused by the form post, so nothing is read back.
#[tokio::test]
async fn a_thread_that_may_not_be_published_is_refused_without_a_read_back() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/5/publication"))
        .respond_with(ResponseTemplate::new(403))
        .mount(&server)
        .await;

    let error = client(&server).publications().publish(5).await.unwrap_err();

    assert_eq!(error.http_status(), Some(403));
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn unpublishing_a_thread_deletes_its_publication() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/topics/4471829/publication"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/topics/4471829"))
        .mount(&server)
        .await;

    client(&server)
        .publications()
        .unpublish(4471829)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/topics/4471829/publication");
    assert!(requests[0].body.is_empty());
}

/// What the client's hooks were told, start and end, in the order they were told it.
#[derive(Default)]
struct Recorder {
    transcript: Mutex<Vec<String>>,
}

impl Recorder {
    fn new() -> Arc<Recorder> {
        Arc::new(Recorder::default())
    }

    fn transcript(&self) -> Vec<String> {
        self.transcript.lock().unwrap().clone()
    }

    fn record(&self, entry: String) {
        self.transcript.lock().unwrap().push(entry);
    }
}

impl Hooks for Recorder {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.record(format!(
            "start {}.{} {} {:?}",
            op.service, op.operation, op.resource_type, op.resource_id
        ));
        None
    }

    fn on_request_start(&self, info: &RequestInfo) {
        self.record(format!("request {} {}", info.method, info.url.path()));
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        _state: OperationState,
        _outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        self.record(format!("end {}.{}", op.service, op.operation));
    }
}
