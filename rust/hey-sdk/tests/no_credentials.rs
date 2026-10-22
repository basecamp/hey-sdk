//! A client built with [`NoCredentials`], for the operations HEY takes unsigned: exchanging
//! a code, refreshing, revoking. It sends those, and refuses anything else before it goes
//! out.

mod support;

use hey_sdk::models::{CreateTokenRequestContent, RevokeTokenRequestContent};
use hey_sdk::{Client, Config, ErrorCode, NoCredentials, SensitiveString};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::http_client;

fn unsigned_client(server: &MockServer) -> Client {
    Client::builder(Config::default().with_base_url(server.uri()))
        .auth_strategy(NoCredentials)
        .http_client(http_client())
        .build()
        .unwrap()
}

#[tokio::test]
async fn a_client_without_credentials_trades_and_revokes_tokens() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "access_token": "new-access",
            "token_type": "bearer",
        })))
        .expect(1)
        .mount(&server)
        .await;
    Mock::given(method("DELETE"))
        .and(path("/oauth/tokens.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .expect(1)
        .mount(&server)
        .await;
    let client = unsigned_client(&server);

    let token = client
        .oauth()
        .create_token(&CreateTokenRequestContent {
            grant_type: "refresh_token".to_string(),
            client_id: "client".to_string(),
            install_id: "install".to_string(),
            refresh_token: Some(SensitiveString::new("refresh")),
            ..CreateTokenRequestContent::default()
        })
        .await
        .unwrap();
    client
        .oauth()
        .revoke_token(&RevokeTokenRequestContent {
            client_id: "client".to_string(),
            token: token.access_token.clone(),
        })
        .await
        .unwrap();

    assert_eq!(token.access_token.expose(), "new-access");
    for request in server.received_requests().await.unwrap() {
        assert!(request.headers.get("authorization").is_none());
    }
}

/// An operation HEY signs is refused before anything goes out, as an `auth_required` that
/// says why.
#[tokio::test]
async fn a_client_without_credentials_refuses_a_signed_operation_without_sending_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .expect(0)
        .mount(&server)
        .await;

    let error = unsigned_client(&server).boxes().list().await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Auth);
    assert_eq!(error.code().as_str(), "auth_required");
    assert!(error.to_string().contains("no credentials"), "{error}");
    assert_eq!(error.http_status(), None);
    assert!(!error.is_retryable());
    assert!(server.received_requests().await.unwrap().is_empty());
}
