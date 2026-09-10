//! The whole OAuth 2.0 PKCE flow against HEY, then a call with the token it produced, with
//! refresh wired in for when the token expires.
//!
//! ```sh
//! cargo run --example oauth_pkce
//! ```
//!
//! It opens a listener on the loopback interface for the redirect, prints the URL to
//! approve the client at, waits for the browser to come back with the code, and trades it
//! for tokens. HEY wants an `install_id` on the authorization request and on every token
//! request after it: a stable identifier the application mints once per installation and
//! keeps beside the tokens. This example keeps it for one run.

use std::sync::Mutex;

use async_trait::async_trait;
use hey_sdk::oauth::{self, ExchangeRequest, OAuthClient, RefreshRequest, ServerMetadata, Token};
use hey_sdk::{Client, Config, Error, TokenProvider};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::TcpListener;

#[tokio::main]
async fn main() -> Result<(), Error> {
    let config = Config::default().with_env();
    let oauth = OAuthClient::default();
    // HEY publishes no well-known document, so its endpoints are known rather than found;
    // `oauth.discover(..)` is for a server that does publish one.
    let metadata = ServerMetadata::for_hey(&config.base_url);

    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .map_err(Error::network)?;
    let port = listener.local_addr().map_err(Error::network)?.port();
    let redirect_uri = format!("http://127.0.0.1:{port}/callback");

    let pkce = oauth::generate_pkce();
    let state = oauth::generate_state();
    let install_id = oauth::generate_state();
    let url = oauth::authorization_url(
        &metadata,
        &config.oauth_client_id,
        &redirect_uri,
        None,
        &state,
        &pkce,
        &install_id,
    )?;
    println!("Open this in a browser and approve the client:\n\n  {url}\n");

    let (code, returned_state) = wait_for_redirect(&listener).await?;
    if returned_state != state {
        return Err(Error::usage(
            "the redirect carried a state this run did not send",
        ));
    }

    let token = oauth
        .exchange(&ExchangeRequest {
            token_endpoint: metadata.token_endpoint.clone(),
            code: code.into(),
            redirect_uri,
            client_id: config.oauth_client_id.clone(),
            client_secret: None,
            code_verifier: pkce.verifier,
            install_id: install_id.clone(),
        })
        .await?;
    println!(
        "Authorized; the access token expires at {}",
        token
            .expires_at
            .map(|at| at.to_rfc3339())
            .unwrap_or_else(|| "an unknown time".to_string())
    );

    // The client asks the provider for a token before each request, and asks it to refresh
    // once when HEY answers 401; the request is sent again when refresh answers `true`.
    let provider = Refreshing {
        oauth,
        metadata,
        client_id: config.oauth_client_id.clone(),
        install_id,
        token: Mutex::new(token),
    };
    let client = Client::new(config, provider)?;
    let me = client.identity().get().await?;
    println!("Signed in as {}", me.name.unwrap_or_default());
    Ok(())
}

/// Holds the tokens for this run and refreshes them when asked. An application keeps
/// `Token` (with its `expires_at`) and the `install_id` in its own store instead, and
/// answers `access_token` from there.
struct Refreshing {
    oauth: OAuthClient,
    metadata: ServerMetadata,
    client_id: String,
    install_id: String,
    token: Mutex<Token>,
}

#[async_trait]
impl TokenProvider for Refreshing {
    async fn access_token(&self) -> Result<String, Error> {
        Ok(self.lock().access_token.expose().to_string())
    }

    async fn refresh(&self) -> bool {
        let Some(refresh_token) = self.lock().refresh_token.clone() else {
            return false;
        };
        let request = RefreshRequest {
            token_endpoint: self.metadata.token_endpoint.clone(),
            refresh_token,
            client_id: self.client_id.clone(),
            client_secret: None,
            install_id: self.install_id.clone(),
        };
        match self.oauth.refresh(&request).await {
            Ok(mut renewed) => {
                // A refresh answer may leave the refresh token out, which means keep using
                // the one you have; only a new one replaces it.
                let mut held = self.lock();
                if renewed.refresh_token.is_none() {
                    renewed.refresh_token = held.refresh_token.take();
                }
                *held = renewed;
                true
            }
            Err(error) => {
                eprintln!("refresh failed: {error}");
                false
            }
        }
    }
}

impl Refreshing {
    fn lock(&self) -> std::sync::MutexGuard<'_, Token> {
        self.token
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
    }
}

/// Takes one HTTP request on the listener, answers it with a page to close, and picks the
/// `code` and `state` out of its query string.
async fn wait_for_redirect(listener: &TcpListener) -> Result<(String, String), Error> {
    let (mut socket, _) = listener.accept().await.map_err(Error::network)?;

    // The request line, then the headers up to the blank line, however TCP splits them;
    // 16 KiB is more than a redirect needs.
    let mut reader = BufReader::new((&mut socket).take(16 * 1024));
    let mut request_line = String::new();
    reader
        .read_line(&mut request_line)
        .await
        .map_err(Error::network)?;
    let mut header = String::new();
    loop {
        header.clear();
        let read = reader
            .read_line(&mut header)
            .await
            .map_err(Error::network)?;
        if read == 0 || header == "\r\n" || header == "\n" {
            break;
        }
    }
    let target = request_line
        .split_whitespace()
        .nth(1)
        .ok_or_else(|| Error::usage("the redirect was not an HTTP request"))?;
    let url = url::Url::parse(&format!("http://127.0.0.1{target}"))?;
    let mut code = None;
    let mut state = None;
    for (key, value) in url.query_pairs() {
        match key.as_ref() {
            "code" => code = Some(value.into_owned()),
            "state" => state = Some(value.into_owned()),
            _ => {}
        }
    }
    let page = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n\
                Authorized. You can close this tab.\n";
    socket
        .write_all(page.as_bytes())
        .await
        .map_err(Error::network)?;
    match (code, state) {
        (Some(code), Some(state)) => Ok((code, state)),
        _ => Err(Error::usage("the redirect carried no code and state")),
    }
}
