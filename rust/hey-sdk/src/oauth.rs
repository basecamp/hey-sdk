use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use bytes::{Bytes, BytesMut};
use chrono::{DateTime, TimeDelta, Utc};
use reqwest::StatusCode;
use reqwest::header::{ACCEPT, CONTENT_TYPE};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use url::{Url, form_urlencoded};

use crate::error::{Error, MAX_ERROR_MESSAGE_BYTES, truncate};
use crate::security::require_secure_endpoint;
use crate::types::SensitiveString;

const FORM_CONTENT_TYPE: &str = "application/x-www-form-urlencoded";
const MAX_DISCOVERY_ERROR_BYTES: usize = 4096;
const MAX_TOKEN_RESPONSE_BYTES: usize = 1 << 20;

/// What an OAuth 2.0 server publishes about itself at its well-known endpoint.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ServerMetadata {
    pub issuer: String,
    pub authorization_endpoint: String,
    pub token_endpoint: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub registration_endpoint: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub scopes_supported: Option<Vec<String>>,
}

/// A token response. `expires_at` is worked out from `expires_in` when the token arrives,
/// so it survives being stored.
///
/// The two tokens are [`SensitiveString`]s: serde-transparent, so the wire is what the
/// server sent, but `[REDACTED]` under `{:?}`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Token {
    pub access_token: SensitiveString,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub refresh_token: Option<SensitiveString>,
    #[serde(default)]
    pub token_type: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub expires_in: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub scope: Option<String>,
    #[serde(skip)]
    pub expires_at: Option<DateTime<Utc>>,
}

/// A code verifier and the challenge derived from it, for the S256 PKCE flow. The verifier
/// is the secret half — whoever holds it can redeem the code — so it prints as
/// `[REDACTED]`; the challenge goes out in the authorization URL and is public.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Pkce {
    pub verifier: SensitiveString,
    pub challenge: String,
}

/// Draws a fresh verifier from the system CSPRNG and derives its SHA-256 challenge.
pub fn generate_pkce() -> Pkce {
    let verifier = URL_SAFE_NO_PAD.encode(rand::random::<[u8; 32]>());
    let challenge = URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()));
    Pkce {
        verifier: SensitiveString::new(verifier),
        challenge,
    }
}

/// Draws a fresh `state` parameter from the system CSPRNG.
pub fn generate_state() -> String {
    URL_SAFE_NO_PAD.encode(rand::random::<[u8; 16]>())
}

/// The URL to send someone to so they can approve the client.
pub fn authorization_url(
    metadata: &ServerMetadata,
    client_id: &str,
    redirect_uri: &str,
    scope: Option<&str>,
    state: &str,
    pkce: &Pkce,
) -> Result<Url, Error> {
    let mut url = Url::parse(&metadata.authorization_endpoint)?;
    {
        let mut query = url.query_pairs_mut();
        query.append_pair("response_type", "code");
        query.append_pair("client_id", client_id);
        query.append_pair("redirect_uri", redirect_uri);
        if let Some(scope) = scope {
            query.append_pair("scope", scope);
        }
        query.append_pair("state", state);
        query.append_pair("code_challenge", &pkce.challenge);
        query.append_pair("code_challenge_method", "S256");
    }
    Ok(url)
}

/// Trades an authorization code for tokens. The code and the client secret are the two
/// halves worth stealing, so both print as `[REDACTED]`.
#[derive(Debug, Clone, Default)]
pub struct ExchangeRequest {
    pub token_endpoint: String,
    pub code: SensitiveString,
    pub redirect_uri: String,
    pub client_id: String,
    pub client_secret: Option<SensitiveString>,
    /// The verifier [`generate_pkce`] drew, which redeems the code and so is a secret of the
    /// same weight.
    pub code_verifier: SensitiveString,
}

/// Trades a refresh token for a new access token.
#[derive(Debug, Clone, Default)]
pub struct RefreshRequest {
    pub token_endpoint: String,
    pub refresh_token: SensitiveString,
    pub client_id: String,
    pub client_secret: Option<SensitiveString>,
}

/// Talks to an OAuth 2.0 server: discovery, code exchange and refresh.
#[derive(Debug, Clone)]
pub struct OAuthClient {
    http: reqwest::Client,
}

impl OAuthClient {
    pub fn new(http: reqwest::Client) -> OAuthClient {
        OAuthClient { http }
    }

    pub async fn discover(&self, base_url: &str) -> Result<ServerMetadata, Error> {
        let url = format!(
            "{}/.well-known/oauth-authorization-server",
            base_url.trim_end_matches('/')
        );
        let response = self
            .http
            .get(&url)
            .header(ACCEPT, "application/json")
            .send()
            .await?;

        let status = response.status();
        if status == StatusCode::OK {
            let body = read_capped(response, MAX_TOKEN_RESPONSE_BYTES).await?;
            Ok(serde_json::from_slice(&body)?)
        } else {
            let body = read_truncated(response, MAX_DISCOVERY_ERROR_BYTES).await;
            Err(Error::api(
                status.as_u16(),
                format!("OAuth discovery failed with status {status}"),
            )
            .with_hint(body))
        }
    }

    pub async fn exchange(&self, request: &ExchangeRequest) -> Result<Token, Error> {
        require(&request.token_endpoint, "token endpoint is required")?;
        require(request.code.expose(), "authorization code is required")?;
        require(&request.redirect_uri, "redirect URI is required")?;
        require(&request.client_id, "client ID is required")?;

        self.post_token_request(&request.token_endpoint, exchange_form(request))
            .await
    }

    pub async fn refresh(&self, request: &RefreshRequest) -> Result<Token, Error> {
        require(&request.token_endpoint, "token endpoint is required")?;
        require(request.refresh_token.expose(), "refresh token is required")?;

        self.post_token_request(&request.token_endpoint, refresh_form(request))
            .await
    }

    async fn post_token_request(&self, token_endpoint: &str, form: String) -> Result<Token, Error> {
        let url = Url::parse(token_endpoint)?;
        require_secure_endpoint(&url)?;

        let response = self
            .http
            .post(url)
            .header(ACCEPT, "application/json")
            .header(CONTENT_TYPE, FORM_CONTENT_TYPE)
            .body(form)
            .send()
            .await?;

        let status = response.status();
        let body = read_capped(response, MAX_TOKEN_RESPONSE_BYTES).await?;
        if status == StatusCode::OK {
            let mut token: Token = serde_json::from_slice(&body)?;
            token.expires_at = token.expires_in.and_then(expires_at);
            Ok(token)
        } else {
            Err(token_error(status, &body))
        }
    }
}

fn require(value: &str, message: &str) -> Result<(), Error> {
    if value.is_empty() {
        Err(Error::usage(message))
    } else {
        Ok(())
    }
}

fn exchange_form(request: &ExchangeRequest) -> String {
    let mut form = form_urlencoded::Serializer::new(String::new());
    form.append_pair("grant_type", "authorization_code");
    form.append_pair("code", request.code.expose());
    form.append_pair("redirect_uri", &request.redirect_uri);
    form.append_pair("client_id", &request.client_id);
    if let Some(secret) = &request.client_secret {
        form.append_pair("client_secret", secret.expose());
    }
    if !request.code_verifier.is_empty() {
        form.append_pair("code_verifier", request.code_verifier.expose());
    }
    form.finish()
}

fn refresh_form(request: &RefreshRequest) -> String {
    let mut form = form_urlencoded::Serializer::new(String::new());
    form.append_pair("grant_type", "refresh_token");
    form.append_pair("refresh_token", request.refresh_token.expose());
    if !request.client_id.is_empty() {
        form.append_pair("client_id", &request.client_id);
    }
    if let Some(secret) = &request.client_secret {
        form.append_pair("client_secret", secret.expose());
    }
    form.finish()
}

async fn read_capped(mut response: reqwest::Response, limit: usize) -> Result<Bytes, Error> {
    let too_large = || Error::api(0, format!("OAuth response body exceeds {limit} bytes"));
    if response
        .content_length()
        .is_some_and(|length| length > limit as u64)
    {
        return Err(too_large());
    }
    let mut body = BytesMut::new();
    while let Some(chunk) = response.chunk().await.map_err(Error::network)? {
        if body.len() + chunk.len() > limit {
            return Err(too_large());
        }
        body.extend_from_slice(&chunk);
    }
    Ok(body.freeze())
}

/// Reads up to `limit` bytes for an error message, giving up on whatever it has when the
/// body itself fails to arrive.
async fn read_truncated(mut response: reqwest::Response, limit: usize) -> String {
    let mut body = BytesMut::new();
    while body.len() < limit {
        match response.chunk().await {
            Ok(Some(chunk)) => body.extend_from_slice(&chunk),
            Ok(None) | Err(_) => break,
        }
    }
    body.truncate(limit);
    String::from_utf8_lossy(&body).into_owned()
}

fn expires_at(seconds: u64) -> Option<DateTime<Utc>> {
    let lifetime = TimeDelta::try_seconds(seconds as i64)?;
    Utc::now().checked_add_signed(lifetime)
}

fn token_error(status: StatusCode, body: &[u8]) -> Error {
    match serde_json::from_slice::<TokenErrorResponse>(body) {
        Ok(response) if !response.error.is_empty() => {
            let description = response.error_description.unwrap_or_default();
            if description.is_empty() {
                Error::auth(format!("token error: {}", response.error)).with_status(status.as_u16())
            } else {
                let description = truncate(&description, MAX_ERROR_MESSAGE_BYTES);
                Error::auth(format!("token error: {}", response.error))
                    .with_hint(description)
                    .with_status(status.as_u16())
            }
        }
        _ => {
            let body = truncate(&String::from_utf8_lossy(body), MAX_ERROR_MESSAGE_BYTES);
            Error::auth(format!("token request failed with status {status}"))
                .with_hint(body)
                .with_status(status.as_u16())
        }
    }
}

#[derive(Deserialize)]
struct TokenErrorResponse {
    #[serde(default)]
    error: String,
    #[serde(default)]
    error_description: Option<String>,
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use wiremock::matchers::{body_string_contains, header, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use super::*;

    #[test]
    fn pkce_challenge_is_the_digest_of_the_verifier() {
        let pkce = generate_pkce();

        assert_eq!(
            32,
            URL_SAFE_NO_PAD
                .decode(pkce.verifier.expose())
                .unwrap()
                .len()
        );
        assert_eq!(
            URL_SAFE_NO_PAD.encode(Sha256::digest(pkce.verifier.expose().as_bytes())),
            pkce.challenge
        );
        assert_ne!(pkce.verifier, generate_pkce().verifier);
        assert_eq!("[REDACTED]", format!("{:?}", pkce.verifier));
    }

    #[test]
    fn state_is_sixteen_random_bytes() {
        let state = generate_state();

        assert_eq!(16, URL_SAFE_NO_PAD.decode(&state).unwrap().len());
        assert_ne!(state, generate_state());
    }

    #[test]
    fn authorization_url_carries_the_challenge_and_state() {
        let pkce = generate_pkce();
        let url = authorization_url(
            &metadata("https://auth.example.com"),
            "client-1",
            "http://127.0.0.1:9000/callback",
            Some("read write"),
            "state-1",
            &pkce,
        )
        .unwrap();

        let query: Vec<(String, String)> = url
            .query_pairs()
            .map(|(name, value)| (name.into_owned(), value.into_owned()))
            .collect();
        assert_eq!("https", url.scheme());
        assert_eq!("/authorize", url.path());
        assert_eq!(
            vec![
                ("response_type".to_string(), "code".to_string()),
                ("client_id".to_string(), "client-1".to_string()),
                (
                    "redirect_uri".to_string(),
                    "http://127.0.0.1:9000/callback".to_string()
                ),
                ("scope".to_string(), "read write".to_string()),
                ("state".to_string(), "state-1".to_string()),
                ("code_challenge".to_string(), pkce.challenge.clone()),
                ("code_challenge_method".to_string(), "S256".to_string()),
            ],
            query
        );
    }

    #[test]
    fn authorization_url_leaves_out_an_absent_scope() {
        let url = authorization_url(
            &metadata("https://auth.example.com"),
            "client-1",
            "http://127.0.0.1:9000/callback",
            None,
            "state-1",
            &generate_pkce(),
        )
        .unwrap();

        assert!(!url.query().unwrap().contains("scope"));
    }

    #[tokio::test]
    async fn discover_reads_the_well_known_document() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/.well-known/oauth-authorization-server"))
            .and(header("accept", "application/json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "issuer": "https://auth.example.com",
                "authorization_endpoint": "https://auth.example.com/authorize",
                "token_endpoint": "https://auth.example.com/token",
                "scopes_supported": [ "read", "write" ]
            })))
            .mount(&server)
            .await;

        let metadata = OAuthClient::new(reqwest::Client::new())
            .discover(&server.uri())
            .await
            .unwrap();

        assert_eq!("https://auth.example.com/token", metadata.token_endpoint);
        assert_eq!(
            Some(vec!["read".to_string(), "write".to_string()]),
            metadata.scopes_supported
        );
        assert_eq!(None, metadata.registration_endpoint);
    }

    #[tokio::test]
    async fn discover_reports_the_failing_status() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .respond_with(ResponseTemplate::new(404).set_body_string("no such server"))
            .mount(&server)
            .await;

        let error = OAuthClient::new(reqwest::Client::new())
            .discover(&server.uri())
            .await
            .unwrap_err();

        assert_eq!(Some(404), error.http_status());
        assert_eq!(Some("no such server"), error.hint());
    }

    #[tokio::test]
    async fn exchange_trades_a_code_for_a_token() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/token"))
            .and(header("content-type", FORM_CONTENT_TYPE))
            .and(body_string_contains("grant_type=authorization_code"))
            .and(body_string_contains("code_verifier=verifier-1"))
            .and(body_string_contains("code=code-1"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "access_token": "access-1",
                "refresh_token": "refresh-1",
                "token_type": "Bearer",
                "expires_in": 3600,
                "scope": "read"
            })))
            .mount(&server)
            .await;

        let request = ExchangeRequest {
            token_endpoint: format!("{}/token", server.uri()),
            code: "code-1".into(),
            redirect_uri: "http://127.0.0.1:9000/callback".to_string(),
            client_id: "client-1".to_string(),
            client_secret: None,
            code_verifier: "verifier-1".into(),
        };
        let token = OAuthClient::new(reqwest::Client::new())
            .exchange(&request)
            .await
            .unwrap();

        assert_eq!("access-1", token.access_token.expose());
        assert_eq!(
            Some("refresh-1"),
            token.refresh_token.as_ref().map(SensitiveString::expose)
        );
        assert_eq!(Some(3600), token.expires_in);
        assert!(token.expires_at.unwrap() > Utc::now());
    }

    #[tokio::test]
    async fn exchange_reports_the_server_error() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(400).set_body_json(json!({
                "error": "invalid_grant",
                "error_description": "The authorization code has expired"
            })))
            .mount(&server)
            .await;

        let request = ExchangeRequest {
            token_endpoint: format!("{}/token", server.uri()),
            code: "code-1".into(),
            redirect_uri: "http://127.0.0.1:9000/callback".to_string(),
            client_id: "client-1".to_string(),
            client_secret: None,
            code_verifier: "verifier-1".into(),
        };
        let error = OAuthClient::new(reqwest::Client::new())
            .exchange(&request)
            .await
            .unwrap_err();

        assert_eq!(crate::error::ErrorCode::Auth, error.code());
        assert_eq!("token error: invalid_grant", error.message());
        assert_eq!(Some("The authorization code has expired"), error.hint());
        assert_eq!(Some(400), error.http_status());
    }

    #[tokio::test]
    async fn exchange_wants_its_required_fields() {
        let error = OAuthClient::new(reqwest::Client::new())
            .exchange(&ExchangeRequest::default())
            .await
            .unwrap_err();

        assert_eq!(crate::error::ErrorCode::Usage, error.code());
        assert_eq!("token endpoint is required", error.message());
    }

    #[tokio::test]
    async fn refresh_trades_a_refresh_token_for_a_token() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/token"))
            .and(body_string_contains("grant_type=refresh_token"))
            .and(body_string_contains("refresh_token=refresh-1"))
            .and(body_string_contains("client_id=client-1"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "access_token": "access-2",
                "token_type": "Bearer",
                "expires_in": 900
            })))
            .mount(&server)
            .await;

        let request = RefreshRequest {
            token_endpoint: format!("{}/token", server.uri()),
            refresh_token: "refresh-1".into(),
            client_id: "client-1".to_string(),
            client_secret: None,
        };
        let token = OAuthClient::new(reqwest::Client::new())
            .refresh(&request)
            .await
            .unwrap();

        assert_eq!("access-2", token.access_token.expose());
        assert_eq!(None, token.refresh_token);
        assert!(token.expires_at.is_some());
    }

    #[tokio::test]
    async fn refresh_reports_an_unparsable_error_body() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(503).set_body_string("upstream is down"))
            .mount(&server)
            .await;

        let request = RefreshRequest {
            token_endpoint: format!("{}/token", server.uri()),
            refresh_token: "refresh-1".into(),
            client_id: "client-1".to_string(),
            client_secret: None,
        };
        let error = OAuthClient::new(reqwest::Client::new())
            .refresh(&request)
            .await
            .unwrap_err();

        assert_eq!(
            "token request failed with status 503 Service Unavailable",
            error.message()
        );
        assert_eq!(Some("upstream is down"), error.hint());
    }

    #[tokio::test]
    async fn a_plain_http_token_endpoint_is_refused() {
        let request = RefreshRequest {
            token_endpoint: "http://auth.example.com/token".to_string(),
            refresh_token: "refresh-1".into(),
            client_id: "client-1".to_string(),
            client_secret: None,
        };
        let error = OAuthClient::new(reqwest::Client::new())
            .refresh(&request)
            .await
            .unwrap_err();

        assert_eq!(crate::error::ErrorCode::Usage, error.code());
    }

    fn metadata(issuer: &str) -> ServerMetadata {
        ServerMetadata {
            issuer: issuer.to_string(),
            authorization_endpoint: format!("{issuer}/authorize"),
            token_endpoint: format!("{issuer}/token"),
            registration_endpoint: None,
            scopes_supported: None,
        }
    }
}
