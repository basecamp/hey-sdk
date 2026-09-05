mod support;

use std::sync::Mutex;

use async_trait::async_trait;
use bytes::Bytes;
use hey_sdk::{Error, ErrorCode, FormResponse, TokenProvider};
use reqwest::StatusCode;
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

#[tokio::test]
async fn a_form_post_captures_the_redirect_rather_than_following_it() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/workflows"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/workflows/8801"))
        .mount(&server)
        .await;

    let created = client(&server)
        .post_form("/workflows", &[("workflow[name]", "Launch")])
        .await
        .unwrap();

    assert_eq!(created.status, StatusCode::FOUND);
    assert_eq!(created.location.as_deref(), Some("/workflows/8801"));
    assert_eq!(created.body, "");
    assert_eq!(created.extract_id().unwrap(), 8801);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].url.path(), "/workflows");
    assert_eq!(requests[0].headers["accept"], "*/*");
    assert_eq!(
        requests[0].headers["content-type"],
        "application/x-www-form-urlencoded"
    );
    assert_eq!(requests[0].body, b"workflow%5Bname%5D=Launch");
}

/// The same endpoint reached on a `.json` path answers the record instead of redirecting.
#[tokio::test]
async fn a_form_endpoint_that_answers_a_document_hands_it_back() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/workflows/8801"))
        .respond_with(ResponseTemplate::new(200).set_body_string("<html>Renamed</html>"))
        .mount(&server)
        .await;

    let answered = client(&server)
        .patch_form("/workflows/8801", &[("workflow[name]", "Renamed")])
        .await
        .unwrap();

    assert_eq!(answered.status, StatusCode::OK);
    assert_eq!(answered.location, None);
    assert_eq!(answered.body, "<html>Renamed</html>");
}

#[tokio::test]
async fn a_form_delete_sends_neither_a_body_nor_a_content_type() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/workflows/8801"))
        .respond_with(ResponseTemplate::new(303).insert_header("Location", "/workflows"))
        .mount(&server)
        .await;

    let removed = client(&server)
        .delete_form("/workflows/8801")
        .await
        .unwrap();

    assert_eq!(removed.status, StatusCode::SEE_OTHER);
    assert_eq!(removed.location.as_deref(), Some("/workflows"));
    let requests = server.received_requests().await.unwrap();
    assert!(requests[0].body.is_empty());
    assert!(!requests[0].headers.contains_key("content-type"));
}

#[tokio::test]
async fn a_multipart_body_goes_out_under_the_boundary_it_was_built_with() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/attachments"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/attachments/9"))
        .mount(&server)
        .await;
    let body = Bytes::from_static(
        b"--frontier\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\nx\r\n--frontier--\r\n",
    );

    let created = client(&server)
        .post_multipart(
            "/attachments",
            "multipart/form-data; boundary=frontier".to_string(),
            body.clone(),
        )
        .await
        .unwrap();

    assert_eq!(created.extract_id().unwrap(), 9);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].headers["content-type"],
        "multipart/form-data; boundary=frontier"
    );
    assert_eq!(requests[0].body, body);
}

#[tokio::test]
async fn a_form_refused_for_stale_credentials_is_sent_again_with_the_same_body() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/workflows"))
        .and(header("authorization", "Bearer stale-token"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;
    Mock::given(method("POST"))
        .and(path("/workflows"))
        .and(header("authorization", "Bearer fresh-token"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/workflows/12"))
        .mount(&server)
        .await;

    let client = builder(&server)
        .token_provider(RefreshingToken::new())
        .build()
        .unwrap();
    let created = client
        .post_form("/workflows", &[("workflow[name]", "Launch")])
        .await
        .unwrap();

    assert_eq!(created.extract_id().unwrap(), 12);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert_eq!(requests[0].body, requests[1].body);
    assert_eq!(requests[1].body, b"workflow%5Bname%5D=Launch");
}

/// A form request that fails reads as the failure it was — Go flattens every status but 401
/// into "Form request failed (HTTP 503)" and loses the request id with it — and is never
/// resent whatever its method: the first attempt may have gone through.
#[tokio::test]
async fn a_form_request_that_fails_keeps_what_hey_answered_and_is_not_resent() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/workflows"))
        .respond_with(ResponseTemplate::new(503).insert_header("x-request-id", "req-8801"))
        .mount(&server)
        .await;
    Mock::given(method("DELETE"))
        .and(path("/workflows/8801"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;

    let client = builder(&server).max_retries(2).build().unwrap();
    let created = client.post_form("/workflows", &[]).await.unwrap_err();
    let removed = client.delete_form("/workflows/8801").await.unwrap_err();

    assert_eq!(created.code(), ErrorCode::Api);
    assert_eq!(created.http_status(), Some(503));
    assert_eq!(created.message(), "API error: 503 Service Unavailable");
    assert_eq!(created.request_id(), Some("req-8801"));
    assert!(created.is_retryable());
    assert_eq!(removed.http_status(), Some(503));
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[test]
fn the_id_is_the_rightmost_numeric_segment_of_the_redirect() {
    assert_eq!(
        redirected_to("/calendar/events/42").extract_id().unwrap(),
        42
    );
    assert_eq!(
        redirected_to("https://app.hey.com/calendar/events/99")
            .extract_id()
            .unwrap(),
        99
    );
    assert_eq!(
        redirected_to("/calendar/events/7/").extract_id().unwrap(),
        7
    );
    assert_eq!(
        redirected_to("/calendar/events/13?edit=1")
            .extract_id()
            .unwrap(),
        13
    );

    let nameless = redirected_to("/calendar").extract_id().unwrap_err();
    assert_eq!(
        nameless.message(),
        "no numeric ID found in location: /calendar"
    );

    let nowhere = FormResponse {
        location: None,
        status: StatusCode::FOUND,
        body: String::new(),
    };
    assert_eq!(
        nowhere.extract_id().unwrap_err().message(),
        "no location header in response"
    );
}

/// A caller's own HTTP client replaces the one ordinary requests go out on, not the one the
/// form requests use: that one has to refuse redirects, and a caller supplying a client that
/// follows them would otherwise silently lose the `Location` every form write is made for.
#[tokio::test]
async fn a_supplied_http_client_does_not_become_the_redirect_capturing_one() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/workflows"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/workflows/8801"))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/workflows/8801"))
        .respond_with(ResponseTemplate::new(200).set_body_string("followed"))
        .mount(&server)
        .await;

    let created = builder(&server)
        .http_client(reqwest::Client::new())
        .build()
        .unwrap()
        .post_form("/workflows", &[("workflow[name]", "Launch")])
        .await
        .unwrap();

    assert_eq!(created.status, StatusCode::FOUND);
    assert_eq!(created.location.as_deref(), Some("/workflows/8801"));
    assert_eq!(created.extract_id().unwrap(), 8801);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

fn redirected_to(location: &str) -> FormResponse {
    FormResponse {
        location: Some(location.to_string()),
        status: StatusCode::FOUND,
        body: String::new(),
    }
}

/// A provider whose credentials can be renewed: the first 401 swaps the stale token for a
/// fresh one, which is what the resent request goes out with.
struct RefreshingToken {
    token: Mutex<String>,
}

impl RefreshingToken {
    fn new() -> RefreshingToken {
        RefreshingToken {
            token: Mutex::new("stale-token".to_string()),
        }
    }
}

#[async_trait]
impl TokenProvider for RefreshingToken {
    async fn access_token(&self) -> Result<String, Error> {
        Ok(self.token.lock().unwrap().clone())
    }

    async fn refresh(&self) -> bool {
        *self.token.lock().unwrap() = "fresh-token".to_string();
        true
    }
}
