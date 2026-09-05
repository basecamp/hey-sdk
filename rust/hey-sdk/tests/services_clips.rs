mod support;

use std::sync::{Arc, Mutex};

use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

#[tokio::test]
async fn clipping_an_entry_posts_the_form_and_takes_the_redirect_for_the_answer() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/clips"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/clips"))
        .mount(&server)
        .await;
    let recorder = Recorder::new();

    builder(&server)
        .hooks(recorder.clone())
        .build()
        .unwrap()
        .clips()
        .create(5512, "The cabinets arrive on Tuesday")
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].url.path(), "/clips");
    assert_eq!(requests[0].headers["accept"], "*/*");
    assert_eq!(
        requests[0].headers["content-type"],
        "application/x-www-form-urlencoded"
    );
    assert_eq!(
        form(&requests[0].body),
        [
            ("clip[entry_id]".to_string(), "5512".to_string()),
            (
                "clip[content]".to_string(),
                "The cabinets arrive on Tuesday".to_string()
            ),
        ]
    );
    assert_eq!(recorder.operations(), ["Clips.CreateClip clip Some(5512)"]);
}

#[tokio::test]
async fn throwing_a_clip_away_deletes_it_by_id_and_sends_no_body() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/clips/9182"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/clips"))
        .mount(&server)
        .await;

    client(&server).clips().delete(9182).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/clips/9182");
    assert!(requests[0].body.is_empty());
    assert!(!requests[0].headers.contains_key("content-type"));
}

fn form(body: &[u8]) -> Vec<(String, String)> {
    url::form_urlencoded::parse(body).into_owned().collect()
}

/// What the client's hooks were told each operation was.
#[derive(Default)]
struct Recorder {
    operations: Mutex<Vec<String>>,
}

impl Recorder {
    fn new() -> Arc<Recorder> {
        Arc::new(Recorder::default())
    }

    fn operations(&self) -> Vec<String> {
        self.operations.lock().unwrap().clone()
    }
}

impl Hooks for Recorder {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.operations.lock().unwrap().push(format!(
            "{}.{} {} {:?}",
            op.service, op.operation, op.resource_type, op.resource_id
        ));
        None
    }
}
