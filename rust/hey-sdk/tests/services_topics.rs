//! Wire-level tests for the `topics` service: what each call sends and how it reads what HEY answers, against literal bodies.

mod support;

use hey_sdk::ErrorCode;
use hey_sdk::models::{CreateTopicCommentRequestContent, TopicCommentPayload};
use serde_json::{Value, json};
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn confirmation_is_asked_for_only_when_it_is_given() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/topics/9/status/trashed.json"))
        .and(query_param("confirm_destroy", "1"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    Mock::given(method("PUT"))
        .and(path("/topics/9/status/trashed.json"))
        .and(query_param_is_missing("confirm_destroy"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let topics = client(&server);

    topics.topics().trash_topic(9, true).await.unwrap();
    topics.topics().trash_topic(9, false).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("confirm_destroy=1"));
    assert_eq!(requests[1].url.query(), None);
}

/// HEY says it will not trash a shared topic unasked by answering with the topic's removal
/// confirmation page, which is read rather than followed.
#[tokio::test]
async fn a_shared_topic_comes_back_asking_to_be_confirmed() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/topics/9/status/trashed.json"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/topics/9/removal/new"))
        .mount(&server)
        .await;

    let error = client(&server)
        .topics()
        .trash_topic(9, false)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert_eq!(
        error.message(),
        "topic 9 is shared; HEY wants confirmation before trashing it"
    );
    assert_eq!(
        error.hint(),
        Some("Call trash_topic with confirm_destroy = true to trash it and remove your access")
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// A redirect anywhere else is HEY sending the caller back to where the topic was, with the
/// trashing done.
#[tokio::test]
async fn a_redirect_back_to_the_box_is_the_trashing_going_through() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/topics/9/status/trashed.json"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/imbox"))
        .mount(&server)
        .await;

    client(&server).topics().trash_topic(9, true).await.unwrap();

    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn a_topic_that_is_not_there_stays_a_failure() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/topics/9/status/trashed.json"))
        .respond_with(ResponseTemplate::new(404))
        .mount(&server)
        .await;

    let error = client(&server)
        .topics()
        .trash_topic(9, true)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
}

/// Only a redirect is read for an answer; every other refusal is a failure like any other.
#[tokio::test]
async fn a_refusal_that_is_not_a_redirect_stays_a_failure() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/topics/9/status/trashed.json"))
        .respond_with(ResponseTemplate::new(406))
        .mount(&server)
        .await;

    let error = client(&server)
        .topics()
        .trash_topic(9, true)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Api);
    assert_eq!(error.http_status(), Some(406));
}

#[tokio::test]
async fn a_topic_is_moved_to_a_box_by_its_id() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/moves.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    client(&server).topics().move_to_box(9, 5).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(
        requests[0].body_json::<serde_json::Value>().unwrap(),
        serde_json::json!({ "box_id": 5 })
    );
}

fn note(content: &str) -> CreateTopicCommentRequestContent {
    CreateTopicCommentRequestContent {
        comment: TopicCommentPayload {
            content: content.to_string(),
        },
    }
}

#[tokio::test]
async fn a_note_is_added_to_a_topic_and_comes_back_as_its_entry() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/comments.json"))
        .respond_with(ResponseTemplate::new(201).set_body_json(json!({
            "id": 1_019_246_358,
            "kind": "comment",
            "topic_id": 9,
            "summary": "Can you take a look at the spine?",
            "created_at": "2026-10-01T16:59:04.735Z",
            "creator": { "id": 197_214_974, "name": "Jason Fried", "email_address": "jason@example.com" },
            "app_url": "https://app.hey.com/topics/9#__entry_1019246358",
            "content": "<div>Can you take a look at the <strong>spine</strong>?</div>"
        })))
        .mount(&server)
        .await;

    let entry = client(&server)
        .topics()
        .create_comment(
            9,
            &note("<div>Can you take a look at the <strong>spine</strong>?</div>"),
        )
        .await
        .unwrap();

    assert_eq!(entry.id, 1_019_246_358);
    assert_eq!(entry.kind.as_deref(), Some("comment"));
    assert_eq!(entry.topic_id, Some(9));
    assert_eq!(
        entry.content.as_deref(),
        Some("<div>Can you take a look at the <strong>spine</strong>?</div>")
    );
    assert_eq!(
        entry.creator.and_then(|creator| creator.name).as_deref(),
        Some("Jason Fried")
    );
    let requests = server.received_requests().await.unwrap();
    let sent: Value = serde_json::from_slice(&requests[0].body).unwrap();
    assert_eq!(
        sent,
        json!({ "comment": { "content": "<div>Can you take a look at the <strong>spine</strong>?</div>" } })
    );
}

#[tokio::test]
async fn a_blank_note_is_refused_as_invalid() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/comments.json"))
        .respond_with(
            ResponseTemplate::new(422)
                .set_body_json(json!({ "errors": ["Content can't be blank"] })),
        )
        .mount(&server)
        .await;

    let error = client(&server)
        .topics()
        .create_comment(9, &note(""))
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Validation);
    assert_eq!(error.http_status(), Some(422));
    assert_eq!(
        error.body_json::<Value>(),
        Some(json!({ "errors": ["Content can't be blank"] }))
    );
}

#[tokio::test]
async fn a_note_on_a_topic_out_of_reach_is_not_found() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/comments.json"))
        .respond_with(ResponseTemplate::new(404))
        .mount(&server)
        .await;

    let error = client(&server)
        .topics()
        .create_comment(9, &note("<div>Following up</div>"))
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
    assert_eq!(error.http_status(), Some(404));
}
