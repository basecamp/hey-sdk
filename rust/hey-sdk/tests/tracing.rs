#![cfg(feature = "tracing")]

mod support;

use std::collections::BTreeMap;
use std::fmt::Debug;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use serde_json::json;
use tracing::Subscriber;
use tracing::field::{Field, Visit};
use tracing::span::{Attributes, Id, Record};
use tracing_subscriber::layer::{Context, Layer, SubscriberExt};
use tracing_subscriber::registry::LookupSpan;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[derive(Debug, Clone, Default)]
struct Span {
    name: String,
    parent: Option<u64>,
    fields: BTreeMap<String, String>,
    closed: bool,
}

/// Every span the SDK opened, with the fields it recorded, in the order they were opened.
#[derive(Clone, Default)]
struct Capture {
    spans: Arc<Mutex<BTreeMap<u64, Span>>>,
}

impl Capture {
    fn install(&self) -> tracing::subscriber::DefaultGuard {
        tracing::subscriber::set_default(tracing_subscriber::registry().with(self.clone()))
    }

    fn named(&self, name: &str) -> Vec<Span> {
        self.spans
            .lock()
            .unwrap()
            .values()
            .filter(|span| span.name == name)
            .cloned()
            .collect()
    }

    fn operations(&self) -> Vec<Span> {
        self.named("hey.operation")
    }

    fn attempts(&self) -> Vec<Span> {
        self.named("hey.attempt")
    }

    fn id_of_named(&self, name: &str) -> u64 {
        *self
            .spans
            .lock()
            .unwrap()
            .iter()
            .find(|(_, span)| span.name == name)
            .map(|(id, _)| id)
            .unwrap()
    }

    fn id_of(&self, name: &str, operation: &str) -> u64 {
        *self
            .spans
            .lock()
            .unwrap()
            .iter()
            .find(|(_, span)| {
                span.name == name
                    && span
                        .fields
                        .get("operation")
                        .is_some_and(|op| op == operation)
            })
            .map(|(id, _)| id)
            .unwrap()
    }
}

struct Fields<'a>(&'a mut BTreeMap<String, String>);

impl Visit for Fields<'_> {
    fn record_debug(&mut self, field: &Field, value: &dyn Debug) {
        self.0
            .insert(field.name().to_string(), format!("{value:?}"));
    }

    fn record_str(&mut self, field: &Field, value: &str) {
        self.0.insert(field.name().to_string(), value.to_string());
    }

    fn record_u64(&mut self, field: &Field, value: u64) {
        self.0.insert(field.name().to_string(), value.to_string());
    }

    fn record_i64(&mut self, field: &Field, value: i64) {
        self.0.insert(field.name().to_string(), value.to_string());
    }
}

impl<S: Subscriber + for<'a> LookupSpan<'a>> Layer<S> for Capture {
    fn on_new_span(&self, attrs: &Attributes<'_>, id: &Id, ctx: Context<'_, S>) {
        let mut span = Span {
            name: attrs.metadata().name().to_string(),
            parent: ctx
                .span(id)
                .and_then(|span| span.parent().map(|parent| parent.id().into_u64())),
            ..Span::default()
        };
        attrs.record(&mut Fields(&mut span.fields));
        self.spans.lock().unwrap().insert(id.into_u64(), span);
    }

    fn on_record(&self, id: &Id, values: &Record<'_>, _ctx: Context<'_, S>) {
        if let Some(span) = self.spans.lock().unwrap().get_mut(&id.into_u64()) {
            values.record(&mut Fields(&mut span.fields));
        }
    }

    fn on_close(&self, id: Id, _ctx: Context<'_, S>) {
        if let Some(span) = self.spans.lock().unwrap().get_mut(&id.into_u64()) {
            span.closed = true;
        }
    }
}

/// An attempt span as a test reads it: its parent and its fields.
type Attempt<'a> = (Option<u64>, Vec<(&'a str, &'a str)>);

fn fields(span: &Span) -> Vec<(&str, &str)> {
    span.fields
        .iter()
        .map(|(name, value)| (name.as_str(), value.as_str()))
        .collect()
}

#[tokio::test]
async fn one_span_per_operation_names_it_and_records_what_hey_answered() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/123.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("x-request-id", "req-4242")
                .set_body_json(json!({ "id": 123, "kind": "imbox", "name": "Imbox" })),
        )
        .mount(&server)
        .await;
    let capture = Capture::default();
    let _guard = capture.install();

    client(&server)
        .boxes()
        .get(123, &Default::default())
        .await
        .unwrap();

    let operations = capture.operations();
    assert_eq!(operations.len(), 1);
    assert_eq!(
        fields(&operations[0]),
        [
            ("http.status", "200"),
            ("operation", "GetBox"),
            ("request_id", "req-4242"),
            ("service", "Boxes"),
        ]
    );
    assert_eq!(operations[0].parent, None);
    assert!(operations[0].closed);
    let attempts = capture.attempts();
    assert_eq!(attempts.len(), 1);
    assert_eq!(
        fields(&attempts[0]),
        [("attempt", "1"), ("http.status", "200")]
    );
    assert_eq!(
        attempts[0].parent,
        Some(capture.id_of("hey.operation", "GetBox"))
    );
}

#[tokio::test]
async fn every_attempt_is_a_child_span_numbered_in_order() {
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
    let capture = Capture::default();
    let _guard = capture.install();

    client(&server).boxes().list().await.unwrap();

    let operation = capture.id_of("hey.operation", "ListBoxes");
    let attempts = capture.attempts();
    let numbered: Vec<Attempt<'_>> = attempts
        .iter()
        .map(|span| (span.parent, fields(span)))
        .collect();
    assert_eq!(
        numbered,
        [
            (
                Some(operation),
                vec![("attempt", "1"), ("http.status", "503")]
            ),
            (
                Some(operation),
                vec![("attempt", "2"), ("http.status", "503")]
            ),
            (
                Some(operation),
                vec![("attempt", "3"), ("http.status", "200")]
            ),
        ]
    );
    assert!(attempts.iter().all(|span| span.closed));
    assert_eq!(capture.operations()[0].fields["http.status"], "200");
}

#[tokio::test]
async fn a_failure_records_the_status_it_failed_with() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/9.json"))
        .respond_with(ResponseTemplate::new(404).insert_header("x-request-id", "req-404"))
        .mount(&server)
        .await;
    let capture = Capture::default();
    let _guard = capture.install();

    client(&server)
        .boxes()
        .get(9, &Default::default())
        .await
        .unwrap_err();

    let operation = &capture.operations()[0];
    assert_eq!(operation.fields["http.status"], "404");
    assert_eq!(operation.fields["request_id"], "req-404");
    assert!(operation.closed);
}

/// Two operations in flight at once each keep their own span: the status one answered
/// never lands on the other.
#[tokio::test]
async fn concurrent_operations_keep_their_own_spans() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/1.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_millis(50))
                .set_body_json(json!({ "id": 1, "kind": "imbox", "name": "Imbox" })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes/2.json"))
        .respond_with(ResponseTemplate::new(404))
        .mount(&server)
        .await;
    let capture = Capture::default();
    let _guard = capture.install();
    let client = client(&server);

    let params = Default::default();
    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.get(1, &params), boxes.get(2, &params));
    first.unwrap();
    second.unwrap_err();

    let operations = capture.operations();
    assert_eq!(operations.len(), 2);
    let mut statuses: Vec<(&str, &str)> = operations
        .iter()
        .map(|span| {
            (
                span.fields["operation"].as_str(),
                span.fields["http.status"].as_str(),
            )
        })
        .collect();
    statuses.sort();
    assert_eq!(statuses, [("GetBox", "200"), ("GetBox", "404")]);
    let mut parents: Vec<u64> = capture
        .attempts()
        .iter()
        .map(|span| span.parent.unwrap())
        .collect();
    parents.sort_unstable();
    parents.dedup();
    assert_eq!(parents.len(), 2, "each attempt hangs off its own operation");
}

/// A caller that drops the call mid-flight drops the span with it: it closes, with no
/// status, rather than staying open for the life of the subscriber.
#[tokio::test]
async fn a_cancelled_operation_closes_its_span_without_an_answer() {
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
    let capture = Capture::default();
    let _guard = capture.install();
    let client = client(&server);

    let abandoned = tokio::time::timeout(Duration::from_millis(50), client.boxes().list()).await;
    assert!(abandoned.is_err());

    let operation = &capture.operations()[0];
    assert!(operation.closed);
    assert!(!operation.fields.contains_key("http.status"));
    let attempt = &capture.attempts()[0];
    assert!(attempt.closed);
    assert!(!attempt.fields.contains_key("http.status"));
}

/// A quiet send is one request inside another operation and opens no operation span of
/// its own: its attempt hangs off whatever span its caller is in, the way the hooks hear of
/// one operation rather than two.
#[tokio::test]
async fn a_quiet_send_runs_in_the_span_its_caller_is_in() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/5/publication"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/topics/5"))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/topics/5/publication.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "published": true, "url": "https://public.hey.com/p/abc" })),
        )
        .mount(&server)
        .await;
    let capture = Capture::default();
    let _guard = capture.install();
    let client = client(&server);

    let caller = tracing::info_span!("caller");
    tracing::Instrument::instrument(client.publications().publish(5), caller)
        .await
        .unwrap();

    let caller = capture.id_of_named("caller");
    let operations = capture.operations();
    assert_eq!(operations.len(), 1);
    assert_eq!(operations[0].fields["operation"], "CreateTopicPublication");
    assert_eq!(operations[0].parent, Some(caller));
    let operation = capture.id_of("hey.operation", "CreateTopicPublication");
    let parents: Vec<Option<u64>> = capture.attempts().iter().map(|span| span.parent).collect();
    assert_eq!(parents, [Some(operation), Some(caller)]);
}

/// Nothing the caller passed in is recorded: no URL, no query, no body.
#[tokio::test]
async fn the_span_names_the_operation_and_nothing_the_caller_passed() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/topics/search.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let capture = Capture::default();
    let _guard = capture.install();

    let _ = client(&server).get("/topics/search?q=secret%20plans").await;

    let operations = capture.operations();
    let names: Vec<&str> = operations[0].fields.keys().map(String::as_str).collect();
    assert_eq!(names, ["http.status", "operation", "service"]);
    for span in capture.attempts() {
        assert_eq!(
            span.fields.keys().map(String::as_str).collect::<Vec<_>>(),
            ["attempt", "http.status"]
        );
    }
}
