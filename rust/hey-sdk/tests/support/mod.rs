#![allow(dead_code)]

use std::sync::Mutex;
use std::time::Duration;

use hey_sdk::http::HttpClient;
use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use hey_sdk::{Client, ClientBuilder, Config, Error, StaticTokenProvider};
use wiremock::MockServer;

pub const TOKEN: &str = "test-token";

/// A client pointed at the mock server. The backoff is wound right down so a test that
/// exercises retries still runs in milliseconds, and the jitter is off so it is repeatable.
pub fn builder(server: &MockServer) -> ClientBuilder {
    Client::builder(Config::default().with_base_url(server.uri()))
        .token_provider(StaticTokenProvider::new(TOKEN))
        .http_client(http_client())
        .base_delay(Duration::from_millis(20))
        .max_delay(Duration::from_millis(20))
        .max_jitter(Duration::ZERO)
}

/// The transport the tests send on. With the `reqwest` feature it is the client the crate
/// ships, so the tests see what a caller gets by default. Without it there is none, and the
/// tests run instead through [`transport::DevTransport`], a transport of the caller's own
/// over the dev-dependency: the same suite then proves the feature's promise, that nothing
/// above the [`HttpClient`] seam needs the shipped client.
#[cfg(feature = "reqwest")]
pub fn http_client() -> impl HttpClient + 'static {
    hey_sdk::http::ReqwestClient::default()
}

#[cfg(not(feature = "reqwest"))]
pub fn http_client() -> impl HttpClient + 'static {
    transport::DevTransport::default()
}

#[cfg(not(feature = "reqwest"))]
mod transport {
    use async_trait::async_trait;
    use bytes::Bytes;
    use futures_util::stream;
    use hey_sdk::Error;
    use hey_sdk::http::{Body, HttpClient, Request, Response};
    use reqwest::redirect::Policy;

    /// A transport over the `reqwest` dev-dependency alone. It does what the trait asks and
    /// nothing else: no redirects, which the SDK follows for itself, the body handed over
    /// as it arrives, and a deadline of its own, since the trait leaves timeouts to the
    /// transport and a stalled mock would otherwise hang the test.
    pub struct DevTransport {
        http: reqwest::Client,
    }

    impl Default for DevTransport {
        fn default() -> DevTransport {
            let http = reqwest::Client::builder()
                .redirect(Policy::none())
                .timeout(std::time::Duration::from_secs(30))
                .build()
                .expect("reqwest builds a client from its defaults");
            DevTransport { http }
        }
    }

    #[async_trait]
    impl HttpClient for DevTransport {
        async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, Error> {
            let request = reqwest::Request::try_from(request).map_err(Error::network)?;
            let answered = self.http.execute(request).await.map_err(Error::network)?;

            let status = answered.status();
            let version = answered.version();
            let headers = answered.headers().clone();
            let content_length = answered.content_length();
            let chunks = stream::try_unfold(answered, |mut answered| async move {
                match answered.chunk().await {
                    Ok(Some(chunk)) => Ok(Some((chunk, answered))),
                    Ok(None) => Ok(None),
                    Err(error) => Err(Error::network(error)),
                }
            });

            let mut response = Response::new(Body::from_stream(chunks, content_length));
            *response.status_mut() = status;
            *response.version_mut() = version;
            *response.headers_mut() = headers;
            Ok(response)
        }
    }
}

pub fn client(server: &MockServer) -> Client {
    builder(server).build().unwrap()
}

/// What each operation announced itself as, in the order they started. For the calls that
/// take more than one request: this says how many operations the hooks were told that was.
#[derive(Default)]
pub struct Operations {
    started: Mutex<Vec<String>>,
}

impl Operations {
    pub fn started(&self) -> Vec<String> {
        self.started.lock().unwrap().clone()
    }
}

impl Hooks for Operations {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.started
            .lock()
            .unwrap()
            .push(format!("{}.{}", op.service, op.operation));
        None
    }
}

/// How each operation ended, as its status: `None` where it succeeded. For the reads whose
/// answer to the caller is not the answer the hooks are told about.
#[derive(Default)]
pub struct Outcomes {
    statuses: Mutex<Vec<Option<u16>>>,
}

impl Outcomes {
    pub fn statuses(&self) -> Vec<Option<u16>> {
        self.statuses.lock().unwrap().clone()
    }
}

impl Hooks for Outcomes {
    fn on_operation_end(
        &self,
        _op: &OperationInfo,
        _state: OperationState,
        outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        self.statuses
            .lock()
            .unwrap()
            .push(outcome.err().and_then(Error::http_status));
    }
}
