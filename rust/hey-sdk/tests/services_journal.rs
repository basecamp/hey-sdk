mod support;

use std::sync::Arc;

use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{Operations, builder, client};

#[tokio::test]
async fn the_content_of_a_day_is_its_html_and_falls_back_to_its_text() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-08-19/journal_entry.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 13,
            "type": "Calendar::JournalEntry",
            "content": "Quarterly planning",
            "content_html": "<div>Quarterly planning</div>"
        })))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-08-20/journal_entry.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(
            json!({ "id": 14, "type": "Calendar::JournalEntry", "content": "Shipped it" }),
        ))
        .mount(&server)
        .await;
    let client = client(&server);

    let html = client.journal().get_content("2026-08-19").await.unwrap();
    let text = client.journal().get_content("2026-08-20").await.unwrap();

    assert_eq!(html.as_deref(), Some("<div>Quarterly planning</div>"));
    assert_eq!(text.as_deref(), Some("Shipped it"));
}

// A day nobody wrote about answers 204 and no body, which is a day without an entry rather
// than an answer the SDK could not read.
#[tokio::test]
async fn a_day_without_an_entry_reads_as_nothing() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-08-19/journal_entry.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let client = client(&server);

    let entry = client.journal().entry("2026-08-19").await.unwrap();
    let content = client.journal().get_content("2026-08-19").await.unwrap();

    assert!(entry.is_none());
    assert!(content.is_none());
}

#[tokio::test]
async fn writing_a_day_answers_the_entry_and_emptying_it_removes_it() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/calendar/days/2026-08-19/journal_entry.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 13,
            "type": "Calendar::JournalEntry",
            "content": "Quarterly planning"
        })))
        .mount(&server)
        .await;
    Mock::given(method("PATCH"))
        .and(path("/calendar/days/2026-08-20/journal_entry.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let client = client(&server);

    let written = client
        .journal()
        .update_content("2026-08-19", "Quarterly planning")
        .await
        .unwrap();
    let removed = client
        .journal()
        .update_content("2026-08-20", "")
        .await
        .unwrap();

    assert_eq!(written.unwrap().id, 13);
    assert!(removed.is_none());
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "calendar_journal_entry": { "content": "Quarterly planning" } })
    );
    assert_eq!(
        sent_json(&server, 1).await,
        json!({ "calendar_journal_entry": { "content": "" } })
    );
}

/// HEY serves `content_html` blank on an entry it has no rendered body for, and blank is
/// not the entry: the plain text is.
#[tokio::test]
async fn a_blank_html_body_falls_back_to_the_text_as_a_missing_one_does() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-08-19/journal_entry.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 13,
            "type": "Calendar::JournalEntry",
            "content": "Shipped it",
            "content_html": ""
        })))
        .mount(&server)
        .await;

    let content = client(&server)
        .journal()
        .get_content("2026-08-19")
        .await
        .unwrap();

    assert_eq!(content.as_deref(), Some("Shipped it"));
}

/// Reading a day's content is its own thing to the hooks, not the entry read it is built
/// on: a gating policy can allow one without the other.
#[tokio::test]
async fn reading_a_days_content_announces_itself_as_that() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-08-19/journal_entry.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let operations = Arc::new(Operations::default());
    let client = builder(&server).hooks(operations.clone()).build().unwrap();

    client.journal().get_content("2026-08-19").await.unwrap();
    client.journal().entry("2026-08-19").await.unwrap();

    assert_eq!(
        ["Journal.GetJournalContent", "Journal.GetJournalEntry"],
        operations.started().as_slice()
    );
}

async fn sent_json(server: &MockServer, index: usize) -> Value {
    let requests = server.received_requests().await.unwrap();
    serde_json::from_slice(&requests[index].body).unwrap()
}
