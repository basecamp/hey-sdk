//! Nothing a `{:?}` or a `{}` of the SDK's types can put in a log: no credential, no
//! request body, no OAuth secret.

mod support;

use hey_sdk::http::Method;
use hey_sdk::oauth::{ExchangeRequest, RefreshRequest};
use hey_sdk::security::redact_headers;
use hey_sdk::{Client, Config, SensitiveString, StaticTokenProvider};
use serde_json::json;

fn client() -> Client {
    Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(StaticTokenProvider::new("bearer-secret"))
        .http_client(support::http_client())
        .build()
        .unwrap()
}

#[test]
fn a_token_provider_prints_no_token() {
    let provider = StaticTokenProvider::new("bearer-secret");
    let printed = format!("{provider:?}");
    assert!(!printed.contains("bearer-secret"), "{printed}");
    assert!(printed.contains("[REDACTED]"), "{printed}");
    assert_eq!(
        format!("{}", SensitiveString::from("bearer-secret")),
        "[REDACTED]"
    );
}

#[test]
fn an_operation_prints_the_shape_of_its_body_and_not_the_body() {
    let client = client();
    let mut operation = client.request(Method::POST, "/messages");
    operation
        .json(&json!({ "subject": "Quarterly plans", "content": "the private draft" }))
        .unwrap();
    let printed = format!("{operation:?}");
    assert!(!printed.contains("private draft"), "{printed}");
    assert!(!printed.contains("Quarterly"), "{printed}");
    assert!(printed.contains("application/json"), "{printed}");

    let mut form = client.request(Method::POST, "/workflows");
    form.form(&[("workflow[name]", "Secret launch")]);
    let printed = format!("{form:?}");
    assert!(!printed.contains("Secret"), "{printed}");

    let mut search = client.request(Method::GET, "/topics/search");
    search.query("q", "secret plans");
    let printed = format!("{search:?}");
    assert!(!printed.contains("secret plans"), "{printed}");
    assert!(printed.contains("\"q\""), "{printed}");

    let written_in = client.request(Method::GET, "/topics/search?q=secret%20plans");
    let printed = format!("{written_in:?}");
    assert!(!printed.contains("secret"), "{printed}");
    assert!(printed.contains("/topics/search"), "{printed}");
}

#[test]
fn the_oauth_requests_print_no_secret() {
    let exchange = ExchangeRequest {
        token_endpoint: "https://hey.test/oauth/token".to_string(),
        client_id: "client-1".to_string(),
        client_secret: Some(SensitiveString::from("client-secret")),
        code: SensitiveString::from("auth-code"),
        code_verifier: SensitiveString::from("pkce-verifier"),
        redirect_uri: "https://app.test/callback".to_string(),
        install_id: "install-1".to_string(),
    };
    let printed = format!("{exchange:?}");
    assert!(!printed.contains("client-secret"), "{printed}");
    assert!(!printed.contains("pkce-verifier"), "{printed}");
    assert!(!printed.contains("auth-code"), "{printed}");

    let refresh = RefreshRequest {
        token_endpoint: "https://hey.test/oauth/token".to_string(),
        refresh_token: SensitiveString::from("refresh-secret"),
        client_id: "client-1".to_string(),
        client_secret: Some(SensitiveString::from("client-secret")),
        install_id: "install-1".to_string(),
    };
    let printed = format!("{refresh:?}");
    assert!(!printed.contains("refresh-secret"), "{printed}");
    assert!(!printed.contains("client-secret"), "{printed}");
}

#[test]
fn credential_headers_are_redacted_before_anything_sees_them() {
    let mut headers = hey_sdk::http::HeaderMap::new();
    headers.insert("authorization", "Bearer bearer-secret".parse().unwrap());
    headers.insert("cookie", "session=cookie-secret".parse().unwrap());
    headers.insert("x-request-id", "req-1".parse().unwrap());
    let redacted = redact_headers(&headers);
    let printed = format!("{redacted:?}");
    assert!(!printed.contains("bearer-secret"), "{printed}");
    assert!(!printed.contains("cookie-secret"), "{printed}");
    assert!(printed.contains("req-1"), "{printed}");
}

/// A network failure's error names no URL: the transport's own text is kept as the hint,
/// and with the shipped client that text carries no address.
#[cfg(feature = "reqwest")]
#[tokio::test]
async fn a_network_error_names_no_url() {
    let client = Client::builder(Config::default().with_base_url("http://127.0.0.1:1"))
        .token_provider(StaticTokenProvider::new("t"))
        .max_retries(0)
        .build()
        .unwrap();
    let mut operation = client.request(Method::GET, "/topics/search");
    operation.query("q", "secret plans");

    let error = client.send_unit(operation).await.unwrap_err();

    let printed = format!("{error} / {error:?}");
    assert!(!printed.contains("secret"), "{printed}");
    assert!(!printed.contains("topics"), "{printed}");
}
