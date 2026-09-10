//! What a 401 costs when many calls earn one at once. The transport here is the test's
//! own: reqwest's pool holds later requests to a new host until the first connection has
//! settled its protocol, which would sign them after the refresh and prove nothing.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use async_trait::async_trait;
use bytes::Bytes;
use hey_sdk::http::{Body, HttpClient, Request, Response, StatusCode};
use hey_sdk::{Client, Config, Error, ErrorCode, TokenProvider};

/// A provider whose refresh takes a moment and hands out a new token each time, counting
/// how often it was asked. `refreshable` off makes every refresh fail.
struct Rotating {
    token: Mutex<String>,
    refreshes: AtomicUsize,
    refreshable: bool,
}

impl Rotating {
    fn new(refreshable: bool) -> Arc<Rotating> {
        Arc::new(Rotating {
            token: Mutex::new("token-0".to_string()),
            refreshes: AtomicUsize::new(0),
            refreshable,
        })
    }

    fn refreshes(&self) -> usize {
        self.refreshes.load(Ordering::SeqCst)
    }
}

#[async_trait]
impl TokenProvider for Rotating {
    async fn access_token(&self) -> Result<String, Error> {
        Ok(self.token.lock().unwrap().clone())
    }

    async fn refresh(&self) -> bool {
        let count = self.refreshes.fetch_add(1, Ordering::SeqCst) + 1;
        tokio::time::sleep(Duration::from_millis(50)).await;
        if self.refreshable {
            *self.token.lock().unwrap() = format!("token-{count}");
        }
        self.refreshable
    }
}

/// A server that answers `token-0` with a 401 and anything else with an empty list. It
/// holds every stale request at a barrier sized for the case, so the 401s come back
/// together whatever the scheduler did with the tasks.
struct Stale {
    tokens: Mutex<Vec<String>>,
    arrived: tokio::sync::Barrier,
}

impl Stale {
    fn new(expected: usize) -> Arc<Stale> {
        Arc::new(Stale {
            tokens: Mutex::new(Vec::new()),
            arrived: tokio::sync::Barrier::new(expected),
        })
    }

    fn tokens(&self) -> Vec<String> {
        self.tokens.lock().unwrap().clone()
    }
}

/// The transport handle the client takes; the test keeps the [`Stale`] behind it.
#[derive(Clone)]
struct Transport(Arc<Stale>);

#[async_trait]
impl HttpClient for Transport {
    async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, Error> {
        let token = request.headers()["authorization"]
            .to_str()
            .unwrap()
            .trim_start_matches("Bearer ")
            .to_string();
        self.0.tokens.lock().unwrap().push(token.clone());
        let mut response = Response::new(Body::from("[]"));
        if token == "token-0" {
            self.0.arrived.wait().await;
            *response.status_mut() = StatusCode::UNAUTHORIZED;
        }
        Ok(response)
    }
}

fn client(server: Arc<Stale>, provider: Arc<Rotating>) -> Client {
    Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider)
        .http_client(Transport(server))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap()
}

/// Ten calls sent on the same stale token are all in flight when the 401s come back; the
/// credentials are refreshed once, and every call is resent on the refreshed ones.
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn concurrent_401s_share_one_refresh() {
    let server = Stale::new(10);
    let provider = Rotating::new(true);
    let client = client(server.clone(), provider.clone());

    let calls: Vec<_> = (0..10)
        .map(|_| {
            let client = client.clone();
            tokio::spawn(async move { client.boxes().list().await })
        })
        .collect();
    for call in calls {
        call.await.unwrap().unwrap();
    }

    assert_eq!(provider.refreshes(), 1);
    let tokens = server.tokens();
    assert_eq!(tokens.len(), 20, "{tokens:?}");
    assert_eq!(
        tokens.iter().filter(|token| *token == "token-0").count(),
        10
    );
    assert_eq!(
        tokens.iter().filter(|token| *token == "token-1").count(),
        10
    );
}

/// A rotating refresh token is only good once: a call whose 401 comes back after another
/// call's refresh has already completed is resent on the new credentials rather than
/// refreshing again, which would burn the token the first refresh just issued.
#[tokio::test]
async fn a_401_that_arrives_after_the_refresh_does_not_refresh_again() {
    let server = Stale::new(2);
    let provider = Rotating::new(true);
    let client = client(server.clone(), provider.clone());

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), async {
        // Signed on token-0 alongside the first call, answered with it, then held past
        // the first call's refresh before its own 401 is acted on.
        let late = boxes.list();
        tokio::time::sleep(Duration::from_millis(200)).await;
        late.await
    });
    first.unwrap();
    second.unwrap();

    assert_eq!(provider.refreshes(), 1);
    assert_eq!(server.tokens().len(), 4);
}

/// A caller whose limit runs out during the refresh leaves the refresh to finish: the
/// provider keeps the token it was handed, and the next call is resent on it rather than
/// refreshing over the top of a refresh the token endpoint already honoured.
#[tokio::test]
async fn a_caller_cut_off_during_the_refresh_does_not_abandon_it() {
    let server = Stale::new(1);
    let provider = Rotating::new(true);
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(Transport(server.clone()))
        .max_jitter(Duration::ZERO)
        .operation_timeout(Duration::from_millis(20))
        .build()
        .unwrap();

    let error = client.boxes().list().await.unwrap_err();
    assert!(error.message().contains("timed out"), "{error}");

    tokio::time::sleep(Duration::from_millis(100)).await;
    assert_eq!(provider.refreshes(), 1);
    assert_eq!(
        provider.access_token().await.unwrap(),
        "token-1",
        "the refresh finished without the caller"
    );

    let unhurried = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(Transport(server.clone()))
        .build()
        .unwrap();
    unhurried.boxes().list().await.unwrap();
    assert_eq!(provider.refreshes(), 1);
}

/// A refresh that fails leaves the 401 standing for every caller, as the authentication
/// error it is, and the next 401 asks again rather than trusting the failure.
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn a_failed_refresh_leaves_every_caller_with_the_401() {
    let server = Stale::new(4);
    let provider = Rotating::new(false);
    let client = client(server.clone(), provider.clone());

    let calls: Vec<_> = (0..4)
        .map(|_| {
            let client = client.clone();
            tokio::spawn(async move { client.boxes().list().await })
        })
        .collect();
    for call in calls {
        let error = call.await.unwrap().unwrap_err();
        assert_eq!(error.code(), ErrorCode::Auth);
        assert_eq!(error.http_status(), Some(401));
    }

    assert_eq!(provider.refreshes(), 4);
    assert_eq!(server.tokens().len(), 4);
}

/// Callers whose limits run out while an earlier refresh holds the turn do not each have
/// a refresh started for them once it fails: a waiter whose caller is gone stands down.
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn a_waiter_whose_caller_is_gone_does_not_refresh_for_nobody() {
    let server = Stale::new(3);
    let provider = Rotating::new(false);
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(Transport(server.clone()))
        .max_jitter(Duration::ZERO)
        .operation_timeout(Duration::from_millis(20))
        .build()
        .unwrap();

    let calls: Vec<_> = (0..3)
        .map(|_| {
            let client = client.clone();
            tokio::spawn(async move { client.boxes().list().await })
        })
        .collect();
    for call in calls {
        call.await.unwrap().unwrap_err();
    }
    tokio::time::sleep(Duration::from_millis(300)).await;

    assert_eq!(provider.refreshes(), 1);
}
