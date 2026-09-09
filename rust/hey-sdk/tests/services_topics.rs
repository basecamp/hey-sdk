mod support;

use hey_sdk::ErrorCode;
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
