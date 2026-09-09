mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::undo_send_id;
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_draft_resolves_the_postings_into_the_entries_the_reply_goes_to() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/bulk_replies/new.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "content": "<div>Jane Doe</div>",
            "entries": [
                { "id": 11, "recipients": [ "ann@example.com" ] },
                { "id": 22, "recipients": [ "bob@example.com" ] }
            ]
        })))
        .mount(&server)
        .await;

    let draft = client(&server)
        .bulk_replies()
        .draft(&[7, 8, 9])
        .await
        .unwrap();

    assert_eq!(draft.content, "<div>Jane Doe</div>");
    assert_eq!(
        draft
            .entries
            .iter()
            .map(|entry| entry.id)
            .collect::<Vec<_>>(),
        vec![11, 22]
    );
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("posting_ids=7%2C8%2C9"));
}

#[tokio::test]
async fn sending_a_bulk_reply_answers_what_was_queued() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/bulk_replies.json"))
        .respond_with(ResponseTemplate::new(201).set_body_json(json!({
            "id": 9,
            "entries_count": 2,
            "delayed": true,
            "undo_send_url": "https://app.hey.com/bulk_replies/9/undo_send"
        })))
        .mount(&server)
        .await;

    let delivery = client(&server)
        .bulk_replies()
        .send(&[11, 22], "On it, thanks.")
        .await
        .unwrap();

    assert_eq!(delivery.id, 9);
    assert_eq!(delivery.entries_count, 2);
    assert!(delivery.delayed);
    assert_eq!(
        undo_send_id(delivery.undo_send_url.as_deref().unwrap()).unwrap(),
        9
    );
    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        serde_json::from_slice::<serde_json::Value>(&requests[0].body).unwrap(),
        json!({ "entry_ids": [ 11, 22 ], "message": { "content": "On it, thanks." } })
    );
}

#[tokio::test]
async fn a_bulk_reply_with_nothing_selected_is_refused_before_anything_is_sent() {
    let server = MockServer::start().await;

    let no_postings = client(&server).bulk_replies().draft(&[]).await.unwrap_err();
    let no_entries = client(&server)
        .bulk_replies()
        .send(&[], "On it")
        .await
        .unwrap_err();

    assert_eq!(no_postings.code(), ErrorCode::Usage);
    assert_eq!(no_postings.message(), "at least one posting is required");
    assert_eq!(no_entries.code(), ErrorCode::Usage);
    assert_eq!(no_entries.message(), "at least one entry is required");
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn calling_a_bulk_reply_back_posts_an_empty_form_to_its_undo_path() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/bulk_replies/9/undo_send"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/bulk_replies/9"))
        .mount(&server)
        .await;

    client(&server).bulk_replies().undo(9).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/bulk_replies/9/undo_send");
    assert_eq!(requests[0].headers["accept"], "*/*");
    assert!(requests[0].body.is_empty());
}

/// Once the replies have gone out there is nothing left to call back, and HEY says so.
#[tokio::test]
async fn calling_back_a_reply_that_has_gone_out_surfaces_heys_refusal() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/bulk_replies/9/undo_send"))
        .respond_with(ResponseTemplate::new(422))
        .mount(&server)
        .await;

    let error = client(&server).bulk_replies().undo(9).await.unwrap_err();

    assert_eq!(error.http_status(), Some(422));
}

#[test]
fn an_undo_url_names_the_bulk_reply_it_would_call_back() {
    assert_eq!(
        undo_send_id("https://app.hey.com/bulk_replies/9/undo_send").unwrap(),
        9
    );
    assert_eq!(undo_send_id("/bulk_replies/9/undo_send").unwrap(), 9);
    assert_eq!(
        undo_send_id("https://app.hey.com/bulk_replies/9/undo_send?token=abc").unwrap(),
        9
    );
}

#[test]
fn anything_that_is_not_an_undo_url_is_refused_rather_than_guessed_at() {
    for candidate in [
        "https://app.hey.com/topics/9",
        "https://app.hey.com/bulk_replies/9",
        "https://app.hey.com/bulk_replies/9/something_else",
        "https://app.hey.com/bulk_replies/nine/undo_send",
        "not a url at all: %",
        "",
    ] {
        let error = undo_send_id(candidate).unwrap_err();
        assert_eq!(error.code(), ErrorCode::Usage);
        assert_eq!(
            error.message(),
            format!("not a bulk reply undo URL: {candidate}")
        );
    }
}
