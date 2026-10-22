//! The routes HEY takes without the caller's credentials, and the ones that follow no
//! redirect: the OAuth token endpoint is both. Each behaviour comes from the route's own
//! flags, so a route of the test's own carries them too, to prove the client reads the flag
//! and not the operation's name.

mod support;

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;

use async_trait::async_trait;
use bytes::Bytes;
use hey_sdk::http::header::AUTHORIZATION;
use hey_sdk::http::{HeaderValue, Method, Request};
use hey_sdk::models::{CreateTokenRequestContent, RevokeTokenRequestContent};
use hey_sdk::routes::{Pagination, Retry, Route};
use hey_sdk::{AuthStrategy, Client, Config, Error, ErrorCode, SensitiveString, TokenProvider};
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client, http_client};

/// A read of the test's own that HEY takes unsigned, and a write of its own that follows no
/// redirect but is signed as usual: neither is an OAuth operation.
static UNSIGNED_READ: Route = Route {
    id: "PeekThing",
    service: "Things",
    method: Method::GET,
    path: "/things/peek.json",
    pattern: "/things/peek",
    resource: "Things",
    resource_type: "thing",
    params: &[],
    idempotent: true,
    readonly: true,
    html: false,
    empty_on: &[],
    lenient_success: false,
    unsigned: true,
    no_redirect: false,
    pagination: Pagination::None,
    retry: Retry {
        max: 1,
        base_delay_ms: 1000,
        retry_on: &[],
    },
};

static SIGNED_NO_REDIRECT: Route = Route {
    id: "SealThing",
    service: "Things",
    method: Method::PUT,
    path: "/things/seal.json",
    pattern: "/things/seal",
    resource: "Things",
    resource_type: "thing",
    params: &[],
    idempotent: true,
    readonly: false,
    html: false,
    empty_on: &[],
    lenient_success: false,
    unsigned: false,
    no_redirect: true,
    pagination: Pagination::None,
    retry: Retry {
        max: 3,
        base_delay_ms: 1000,
        retry_on: &[503],
    },
};

/// A token provider that counts what it is asked, and renews on every refresh: its token
/// changes only when it is refreshed.
#[derive(Default)]
struct Counting {
    asked: AtomicUsize,
    refreshes: AtomicUsize,
}

#[async_trait]
impl TokenProvider for Counting {
    async fn access_token(&self) -> Result<String, Error> {
        self.asked.fetch_add(1, Ordering::SeqCst);
        Ok(format!("token-{}", self.refreshes.load(Ordering::SeqCst)))
    }

    async fn refresh(&self) -> bool {
        self.refreshes.fetch_add(1, Ordering::SeqCst);
        true
    }
}

/// A strategy that signs the way a DPoP-bound client would: an `Authorization` and a
/// `DPoP` proof on every request, counted.
#[derive(Default)]
struct ProofOfPossession {
    signed: AtomicUsize,
}

#[async_trait]
impl AuthStrategy for ProofOfPossession {
    async fn authenticate(&self, request: &mut Request<Bytes>) -> Result<(), Error> {
        self.signed.fetch_add(1, Ordering::SeqCst);
        let headers = request.headers_mut();
        headers.insert(AUTHORIZATION, HeaderValue::from_static("DPoP bound-token"));
        headers.insert("dpop", HeaderValue::from_static("proof"));
        Ok(())
    }
}

fn grant() -> CreateTokenRequestContent {
    CreateTokenRequestContent {
        grant_type: "authorization_code".to_string(),
        client_id: "client".to_string(),
        install_id: "install".to_string(),
        code: Some(SensitiveString::new("the-code")),
        redirect_uri: Some("heyapp://oauth/callback".to_string()),
        code_verifier: Some(SensitiveString::new("the-verifier")),
        refresh_token: None,
    }
}

fn refresh_grant() -> CreateTokenRequestContent {
    CreateTokenRequestContent {
        grant_type: "refresh_token".to_string(),
        client_id: "client".to_string(),
        install_id: "install".to_string(),
        refresh_token: Some(SensitiveString::new("dead-refresh")),
        ..CreateTokenRequestContent::default()
    }
}

fn revocation() -> RevokeTokenRequestContent {
    RevokeTokenRequestContent {
        client_id: "client".to_string(),
        token: SensitiveString::new("the-token"),
    }
}

fn tokens() -> ResponseTemplate {
    ResponseTemplate::new(200).set_body_json(json!({
        "access_token": "new-access",
        "token_type": "bearer",
        "expires_in": 1_209_600,
        "refresh_token": "new-refresh",
    }))
}

async fn received(server: &MockServer) -> Vec<wiremock::Request> {
    server.received_requests().await.unwrap()
}

fn body(request: &wiremock::Request) -> Value {
    serde_json::from_slice(&request.body).unwrap()
}

#[tokio::test]
async fn create_token_posts_its_json_body_to_the_token_endpoint() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(tokens())
        .mount(&server)
        .await;

    let token = client(&server)
        .oauth()
        .create_token(&grant())
        .await
        .unwrap();

    assert_eq!(token.access_token.expose(), "new-access");
    assert_eq!(token.token_type, "bearer");
    assert_eq!(token.expires_in, Some(1_209_600));
    assert_eq!(
        token.refresh_token.as_ref().map(SensitiveString::expose),
        Some("new-refresh")
    );
    let requests = received(&server).await;
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].method.as_str(), "POST");
    assert_eq!(requests[0].url.path(), "/oauth/tokens.json");
    assert_eq!(requests[0].url.query(), None);
    assert_eq!(
        requests[0].headers.get("content-type").unwrap(),
        "application/json"
    );
    assert_eq!(
        body(&requests[0]),
        json!({
            "grant_type": "authorization_code",
            "client_id": "client",
            "install_id": "install",
            "code": "the-code",
            "redirect_uri": "heyapp://oauth/callback",
            "code_verifier": "the-verifier",
        })
    );
}

#[tokio::test]
async fn revoke_token_sends_its_json_body_on_a_delete_and_nothing_in_the_query() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/oauth/tokens.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .mount(&server)
        .await;

    client(&server)
        .oauth()
        .revoke_token(&revocation())
        .await
        .unwrap();

    let requests = received(&server).await;
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].method.as_str(), "DELETE");
    assert_eq!(requests[0].url.path(), "/oauth/tokens.json");
    assert_eq!(requests[0].url.query(), None);
    assert_eq!(
        body(&requests[0]),
        json!({ "client_id": "client", "token": "the-token" })
    );
}

/// The client holds a token, and the token endpoint gets none of it: the provider is not
/// even asked.
#[tokio::test]
async fn an_unsigned_route_goes_out_without_the_clients_token() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(tokens())
        .mount(&server)
        .await;
    Mock::given(method("DELETE"))
        .and(path("/oauth/tokens.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .mount(&server)
        .await;
    let provider = Arc::new(Counting::default());
    let client = builder(&server)
        .token_provider(provider.clone())
        .build()
        .unwrap();

    client.oauth().create_token(&grant()).await.unwrap();
    client.oauth().revoke_token(&revocation()).await.unwrap();

    for request in received(&server).await {
        assert!(
            request.headers.get("authorization").is_none(),
            "{} {} carried an Authorization",
            request.method,
            request.url
        );
    }
    assert_eq!(provider.asked.load(Ordering::SeqCst), 0);
}

/// A strategy that puts a DPoP proof beside the token is not asked either, so neither
/// header goes out.
#[tokio::test]
async fn an_unsigned_route_carries_no_proof_from_the_clients_strategy() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(tokens())
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let strategy = Arc::new(ProofOfPossession::default());
    let client = builder(&server)
        .auth_strategy(Shared(strategy.clone()))
        .build()
        .unwrap();

    client.oauth().create_token(&grant()).await.unwrap();
    assert_eq!(strategy.signed.load(Ordering::SeqCst), 0);
    // The same client signs a route that is not unsigned, so the strategy is in play.
    client.boxes().list().await.unwrap();

    let requests = received(&server).await;
    assert_eq!(requests.len(), 2);
    assert!(requests[0].headers.get("authorization").is_none());
    assert!(requests[0].headers.get("dpop").is_none());
    assert_eq!(
        requests[1].headers.get("authorization").unwrap(),
        "DPoP bound-token"
    );
    assert_eq!(requests[1].headers.get("dpop").unwrap(), "proof");
    assert_eq!(strategy.signed.load(Ordering::SeqCst), 1);
}

/// A strategy the test keeps a handle on.
struct Shared(Arc<ProofOfPossession>);

#[async_trait]
impl AuthStrategy for Shared {
    async fn authenticate(&self, request: &mut Request<Bytes>) -> Result<(), Error> {
        self.0.authenticate(request).await
    }
}

/// A 401 from the token endpoint judges the refresh token in the body, not credentials the
/// client could renew: it is sent once, and the provider's refresh — which would succeed —
/// is never asked.
#[tokio::test]
async fn a_401_on_an_unsigned_route_is_the_answer_and_refreshes_nothing() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(ResponseTemplate::new(401).set_body_json(
            json!({ "error": "invalid_grant", "error_description": "Invalid refresh token." }),
        ))
        .mount(&server)
        .await;
    let provider = Arc::new(Counting::default());
    let client = builder(&server)
        .token_provider(provider.clone())
        .max_retries(3)
        .build()
        .unwrap();

    let error = client
        .oauth()
        .create_token(&refresh_grant())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Auth);
    assert_eq!(error.code().as_str(), "auth_required");
    assert_eq!(error.http_status(), Some(401));
    assert_eq!(received(&server).await.len(), 1);
    assert_eq!(provider.refreshes.load(Ordering::SeqCst), 0);
}

/// The same 401 on a route the client signs is refreshed and resent, so the provider above
/// would have renewed had it been asked.
#[tokio::test]
async fn a_401_on_a_signed_route_is_still_refreshed() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(401))
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    let provider = Arc::new(Counting::default());
    let client = builder(&server)
        .token_provider(provider.clone())
        .build()
        .unwrap();

    client.boxes().list().await.unwrap();

    assert_eq!(received(&server).await.len(), 2);
    assert_eq!(provider.refreshes.load(Ordering::SeqCst), 1);
}

/// A provider that refreshes by trading its refresh token through the very client it signs
/// for, as an application holding one client would.
#[derive(Default)]
struct SelfRefreshing {
    client: OnceLock<Client>,
    token: Mutex<String>,
    refreshes: AtomicUsize,
}

#[async_trait]
impl TokenProvider for SelfRefreshing {
    async fn access_token(&self) -> Result<String, Error> {
        Ok(self.token.lock().unwrap().clone())
    }

    async fn refresh(&self) -> bool {
        self.refreshes.fetch_add(1, Ordering::SeqCst);
        let client = self.client.get().unwrap();
        match client.oauth().create_token(&refresh_grant()).await {
            Ok(token) => {
                *self.token.lock().unwrap() = token.access_token.into_inner();
                true
            }
            Err(_) => false,
        }
    }
}

/// The refresh holds the client's refresh turn while it runs, and the grant it sends from
/// inside goes out unsigned, so it does not wait for a turn of its own: it would never get
/// one.
#[tokio::test]
async fn a_refresh_may_trade_its_token_through_the_client_it_refreshes() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(401))
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(tokens())
        .mount(&server)
        .await;
    let provider = Arc::new(SelfRefreshing::default());
    *provider.token.lock().unwrap() = "stale".to_string();
    let client = builder(&server)
        .token_provider(provider.clone())
        .build()
        .unwrap();
    provider.client.set(client.clone()).ok().unwrap();

    tokio::time::timeout(Duration::from_secs(5), client.boxes().list())
        .await
        .expect("the refresh waited on itself")
        .unwrap();

    assert_eq!(provider.refreshes.load(Ordering::SeqCst), 1);
    let requests = received(&server).await;
    assert_eq!(requests.len(), 3);
    assert_eq!(requests[1].url.path(), "/oauth/tokens.json");
    assert_eq!(
        requests[2].headers.get("authorization").unwrap(),
        "Bearer new-access"
    );
}

/// The flag is what the client reads: a route of the test's own that is not an OAuth
/// operation goes out unsigned, and its 401 is the answer.
#[tokio::test]
async fn any_route_marked_unsigned_goes_out_unsigned() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/things/peek.json"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;
    let provider = Arc::new(Counting::default());
    let client = builder(&server)
        .token_provider(provider.clone())
        .build()
        .unwrap();

    let error = client
        .send::<Value>(client.operation(&UNSIGNED_READ, &[]))
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(401));
    let requests = received(&server).await;
    assert_eq!(requests.len(), 1);
    assert!(requests[0].headers.get("authorization").is_none());
    assert_eq!(provider.asked.load(Ordering::SeqCst), 0);
    assert_eq!(provider.refreshes.load(Ordering::SeqCst), 0);
}

/// A 307 keeps the body, which carries the code and its verifier: the grant is sent once,
/// the redirect is the error, and nothing reaches the `Location`. The 3xx's own body is not
/// read, so the error keeps none of it.
#[tokio::test]
async fn a_redirect_on_create_token_is_an_error_with_its_status() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/oauth/tokens.json"))
        .respond_with(
            ResponseTemplate::new(307)
                .insert_header(
                    "location",
                    format!("{}/elsewhere/oauth/tokens.json", server.uri()).as_str(),
                )
                .set_body_json(json!({ "error": "moved" })),
        )
        .mount(&server)
        .await;
    Mock::given(path("/elsewhere/oauth/tokens.json"))
        .respond_with(tokens())
        .expect(0)
        .mount(&server)
        .await;

    let error = client(&server)
        .oauth()
        .create_token(&grant())
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(307));
    assert!(!error.is_retryable());
    assert_eq!(error.body(), None);
    let requests = received(&server).await;
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].url.path(), "/oauth/tokens.json");
}

#[tokio::test]
async fn a_redirect_on_revoke_token_is_an_error_with_its_status() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/oauth/tokens.json"))
        .respond_with(
            ResponseTemplate::new(302).insert_header("location", "/elsewhere/oauth/tokens.json"),
        )
        .mount(&server)
        .await;
    Mock::given(path("/elsewhere/oauth/tokens.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .expect(0)
        .mount(&server)
        .await;

    let error = client(&server)
        .oauth()
        .revoke_token(&revocation())
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(302));
    assert_eq!(received(&server).await.len(), 1);
}

/// A signed route can follow no redirect too, and an idempotent one with a retry policy is
/// still sent once on a 3xx.
#[tokio::test]
async fn any_route_marked_no_redirect_takes_the_redirect_for_its_answer() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/things/seal.json"))
        .respond_with(
            ResponseTemplate::new(308).insert_header("location", "/elsewhere/things/seal.json"),
        )
        .mount(&server)
        .await;
    Mock::given(path("/elsewhere/things/seal.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .expect(0)
        .mount(&server)
        .await;
    let client = builder(&server).max_retries(3).build().unwrap();

    let error = client
        .send_unit(client.operation(&SIGNED_NO_REDIRECT, &[]))
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(308));
    let requests = received(&server).await;
    assert_eq!(requests.len(), 1);
    assert_eq!(
        requests[0].headers.get("authorization").unwrap(),
        &format!("Bearer {}", support::TOKEN)
    );
}

/// A route that carries neither flag still follows a redirect on HEY's own origin.
#[tokio::test]
async fn an_ordinary_route_still_follows_a_redirect() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(302).insert_header("location", "/moved/boxes.json"))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/moved/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .expect(1)
        .mount(&server)
        .await;

    client(&server).boxes().list().await.unwrap();
}

/// A client built with no credentials of any kind is still refused at build time, so a
/// forgotten token is caught before the first call.
#[test]
fn a_builder_with_no_credentials_at_all_is_refused() {
    let error = Client::builder(Config::default())
        .http_client(http_client())
        .build()
        .err()
        .unwrap();

    assert_eq!(error.code(), ErrorCode::Usage);
}
