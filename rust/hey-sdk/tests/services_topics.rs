//! Wire-level tests for the `topics` service: what each call sends and how it reads what HEY answers, against literal bodies.

mod support;

use std::sync::Arc;

use hey_sdk::ErrorCode;
use hey_sdk::models::{
    CreateTopicCommentRequestContent, RenameTopicRequestContent, TopicCommentPayload,
    TopicNamePayload,
};
use hey_sdk::services::undone_entry_id;
use serde_json::{Value, json};
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{Operations, Outcomes, builder, client};

#[tokio::test]
async fn rename_sends_the_name_including_an_empty_one_and_needs_no_read_back() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/topics/9.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let client = client(&server);
    for name in ["Kitchen renovation", "", "Renovation: \"café\" & garden"] {
        client
            .topics()
            .rename(
                9,
                &RenameTopicRequestContent {
                    topic: TopicNamePayload { name: name.into() },
                },
            )
            .await
            .unwrap();
    }
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 3);
    for (request, name) in
        requests
            .iter()
            .zip(["Kitchen renovation", "", "Renovation: \"café\" & garden"])
    {
        assert_eq!(
            request.body_json::<Value>().unwrap(),
            json!({"topic": {"name": name}})
        );
    }
}

#[tokio::test]
async fn rename_reports_a_topic_that_is_not_accessible() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/topics/9.json"))
        .respond_with(ResponseTemplate::new(404))
        .mount(&server)
        .await;
    let error = client(&server)
        .topics()
        .rename(
            9,
            &RenameTopicRequestContent {
                topic: TopicNamePayload {
                    name: "Kitchen renovation".into(),
                },
            },
        )
        .await
        .unwrap_err();
    assert_eq!(error.code(), ErrorCode::NotFound);
}

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

/// HEY answers an undo with a 303 to the draft. The redirect is the answer: it names the
/// entry called back, and following it would only read a page.
#[tokio::test]
async fn an_undo_answers_the_entry_hey_called_back_without_following_the_redirect() {
    for location in [
        "/messages/77/edit",
        "/topics/9?expanded_draft=77",
        "https://app.hey.com/messages/77/edit?native_undo=true",
    ] {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/topics/9/undo_send.json"))
            .respond_with(ResponseTemplate::new(303).insert_header("Location", location))
            .mount(&server)
            .await;
        let operations = Arc::new(Operations::default());
        let client = builder(&server)
            .hooks(operations.clone())
            .max_retries(3)
            .build()
            .unwrap();

        let entry_id = client.topics().undo_topic_send(9).await.unwrap();

        assert_eq!(entry_id, Some(77), "{location}");
        let requests = server.received_requests().await.unwrap();
        assert_eq!(
            requests.len(),
            1,
            "the redirect to {location} is the answer, not followed"
        );
        assert_eq!(requests[0].method.as_str(), "POST");
        assert_eq!(requests[0].url.path(), "/topics/9/undo_send.json");
        assert_eq!(requests[0].headers["accept"], "application/json");
        assert_eq!(
            requests[0].headers["authorization"],
            format!("Bearer {}", support::TOKEN)
        );
        assert!(requests[0].body.is_empty());
        assert_eq!(operations.started(), ["Topics.UndoTopicSend"]);
    }
}

/// A redirect that names no entry is still an undo: HEY acted. The caller learns only that
/// it can't say which entry it was.
#[tokio::test]
async fn an_undo_whose_redirect_names_no_entry_answers_none() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/undo_send.json"))
        .respond_with(ResponseTemplate::new(303).insert_header("Location", "/somewhere/else"))
        .mount(&server)
        .await;

    let entry_id = client(&server).topics().undo_topic_send(9).await.unwrap();

    assert_eq!(entry_id, None);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// Once the delay has run out HEY answers 400: too late, told apart from an undo by its
/// status, and never asked twice. The hooks hear the operation end with that 400.
#[tokio::test]
async fn an_undo_with_nothing_left_to_call_back_is_refused_with_heys_400() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/undo_send.json"))
        .respond_with(ResponseTemplate::new(400).insert_header("X-Request-Id", "req-undo-400"))
        .mount(&server)
        .await;
    let outcomes = Arc::new(Outcomes::default());
    let client = builder(&server)
        .hooks(outcomes.clone())
        .max_retries(3)
        .build()
        .unwrap();

    let error = client.topics().undo_topic_send(9).await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Api);
    assert_eq!(error.http_status(), Some(400));
    assert_eq!(
        error.message(),
        "nothing to undo: the message was already sent, or none was held"
    );
    assert_eq!(error.request_id(), Some("req-undo-400"));
    assert!(!error.is_retryable());
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
    assert_eq!(outcomes.statuses(), [Some(400)]);
}

/// A second undo calls back the next delayed entry, so a failure is never sent again,
/// whatever the client's retry budget.
#[tokio::test]
async fn an_undo_is_not_sent_again_after_a_failure() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/undo_send.json"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;
    let client = builder(&server).max_retries(3).build().unwrap();

    let error = client.topics().undo_topic_send(9).await.unwrap_err();

    assert_eq!(error.http_status(), Some(503));
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// Any other refusal is HEY's own, as it reports it.
#[tokio::test]
async fn an_undo_on_a_topic_out_of_reach_stays_not_found() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/9/undo_send.json"))
        .respond_with(ResponseTemplate::new(404))
        .mount(&server)
        .await;

    let error = client(&server)
        .topics()
        .undo_topic_send(9)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
}

#[test]
fn an_undo_redirect_names_the_entry_called_back() {
    for location in [
        "/messages/77/edit",
        "https://app.hey.com/messages/77/edit",
        "https://app.hey.com/messages/77/edit?native_undo=true&notice=x",
        "/topics/9?expanded_draft=77",
        "https://app.hey.com/topics/9?expanded_draft=77#__entry_77",
    ] {
        assert_eq!(undone_entry_id(location), Some(77), "{location}");
    }
}

#[test]
fn any_other_location_names_no_entry() {
    for location in [
        "",
        "/topics/9", // where HEY's pages send a browser with nothing to undo
        "/topics/9?expanded_draft=",
        "/topics/9?expanded_draft=abc",
        "/topics/9?expanded_draft=-4",
        "/messages/77", // the message, not its draft
        "/messages/abc/edit",
        "/messages/0/edit",
        "/entries/77/edit",
        "/topics/9/entries?expanded_draft=77",
        "not a url at all: %",
    ] {
        assert_eq!(undone_entry_id(location), None, "{location}");
    }
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
            "content": "<div>Can you take a look at the <strong>spine</strong>?</div>",
            "visible_to": [{ "id": 140_958_377, "name": "Andrea LaRowe", "email_address": "andrea@example.com" }],
            "collection_only": false
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
    assert_eq!(
        entry
            .visible_to
            .as_deref()
            .map(|contacts| contacts.iter().map(|c| c.id).collect::<Vec<_>>()),
        Some(vec![140_958_377])
    );
    assert_eq!(entry.collection_only, Some(false));
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

#[tokio::test]
async fn the_audience_of_a_note_is_read_without_posting_one() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/topics/9/comments/new.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "visible_to": [
                { "id": 140_958_377, "name": "Andrea LaRowe", "email_address": "andrea@example.com" },
                { "id": 197_214_974, "name": "Jason Fried", "email_address": "jason@example.com" }
            ],
            "collection_only": false
        })))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/topics/10/comments/new.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "visible_to": [], "collection_only": true })),
        )
        .mount(&server)
        .await;
    let topics = client(&server);

    let shared = topics.topics().get_comment_audience(9).await.unwrap();
    let private = topics.topics().get_comment_audience(10).await.unwrap();

    assert_eq!(shared.visible_to.map(|contacts| contacts.len()), Some(2));
    assert_eq!(shared.collection_only, Some(false));
    assert_eq!(private.visible_to, Some(vec![]));
    assert_eq!(private.collection_only, Some(true));
}
