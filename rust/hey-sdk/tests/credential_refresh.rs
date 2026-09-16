//! What a 401 costs when many calls earn one at once. The transport here is the test's
//! own: reqwest's pool holds later requests to a new host until the first connection has
//! settled its protocol, which would sign them after the refresh and prove nothing.

use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use async_trait::async_trait;
use bytes::Bytes;
use hey_sdk::http::{Body, HttpClient, Request, Response, StatusCode};
use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use hey_sdk::{
    AuthStrategy, BearerAuth, Client, Config, Error, ErrorCode, StaticTokenProvider, TokenProvider,
};
use tokio::sync::Notify;

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
/// error it is, and costs the one call to the provider: the other callers were signed
/// with the very credentials it failed to renew, so its answer is theirs.
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

    assert_eq!(provider.refreshes(), 1);
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

/// A gate the test opens once, that anyone may wait at before or after it opens.
#[derive(Default)]
struct Gate {
    open: AtomicBool,
    opened: Notify,
}

impl Gate {
    fn open(&self) {
        self.open.store(true, Ordering::SeqCst);
        self.opened.notify_waiters();
    }

    async fn wait(&self) {
        // Registered before the check, so an opening between the two is not missed.
        let opened = self.opened.notified();
        if !self.open.load(Ordering::SeqCst) {
            opened.await;
        }
    }
}

/// A provider whose token cannot be renewed, counting how often it was asked to. It says
/// when a refresh has started and answers only once the test opens `release`, so a 401
/// can be arranged to come back while the refresh is still running.
struct Refusing {
    refreshes: AtomicUsize,
    started: Arc<Gate>,
    release: Arc<Gate>,
}

impl Refusing {
    fn new() -> Arc<Refusing> {
        Arc::new(Refusing {
            refreshes: AtomicUsize::new(0),
            started: Arc::default(),
            release: Arc::default(),
        })
    }

    fn refreshes(&self) -> usize {
        self.refreshes.load(Ordering::SeqCst)
    }
}

#[async_trait]
impl TokenProvider for Refusing {
    async fn access_token(&self) -> Result<String, Error> {
        Ok("token-0".to_string())
    }

    async fn refresh(&self) -> bool {
        self.refreshes.fetch_add(1, Ordering::SeqCst);
        self.started.open();
        self.release.wait().await;
        false
    }
}

/// A server that answers `token-0` with a 401 and anything else with an empty list, and
/// lets the test say when the 401s come back: the first `together` stale requests wait at
/// a barrier, so all of them are signed before any is answered, and the `held` arrival
/// waits on past it until its gate opens.
struct Sequenced {
    tokens: Mutex<Vec<String>>,
    arrivals: AtomicUsize,
    together: usize,
    arrived: tokio::sync::Barrier,
    held: usize,
    gate: Arc<Gate>,
}

impl Sequenced {
    fn new(together: usize, held: usize, gate: Arc<Gate>) -> Arc<Sequenced> {
        Arc::new(Sequenced {
            tokens: Mutex::new(Vec::new()),
            arrivals: AtomicUsize::new(0),
            together,
            arrived: tokio::sync::Barrier::new(together),
            held,
            gate,
        })
    }

    fn tokens(&self) -> Vec<String> {
        self.tokens.lock().unwrap().clone()
    }
}

#[derive(Clone)]
struct SequencedTransport(Arc<Sequenced>);

#[async_trait]
impl HttpClient for SequencedTransport {
    async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, Error> {
        let token = request.headers()["authorization"]
            .to_str()
            .unwrap()
            .trim_start_matches("Bearer ")
            .to_string();
        self.0.tokens.lock().unwrap().push(token.clone());
        let mut response = Response::new(Body::from("[]"));
        if token == "token-0" {
            let arrival = self.0.arrivals.fetch_add(1, Ordering::SeqCst) + 1;
            if arrival <= self.0.together {
                self.0.arrived.wait().await;
            }
            if arrival == self.0.held {
                self.0.gate.wait().await;
            }
            *response.status_mut() = StatusCode::UNAUTHORIZED;
        }
        Ok(response)
    }
}

/// Opens its gate when an operation ends, whichever way it ended.
struct Ended(Arc<Gate>);

impl Hooks for Ended {
    fn on_operation_end(
        &self,
        _op: &OperationInfo,
        _state: OperationState,
        _outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
        self.0.open();
    }
}

/// A refresh that fails is one refresh: every request signed with the credentials it
/// could not renew gets the 401 rather than asking the token endpoint again for the same
/// credentials during the same outage. A request signed after the failure asks again.
#[tokio::test]
async fn a_failed_refresh_is_shared_by_every_request_signed_with_the_credentials_it_was_for() {
    // Both requests go out before either is answered, so both are signed with the
    // credentials the one refresh fails to renew; the second is answered only once the
    // first has failed, so its 401 finds the failure already recorded.
    let first_failed = Arc::new(Gate::default());
    let server = Sequenced::new(2, 2, first_failed.clone());
    let provider = Refusing::new();
    provider.release.open();
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(SequencedTransport(server.clone()))
        .hooks(Ended(first_failed))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());
    for outcome in [first, second] {
        let error = outcome.unwrap_err();
        assert_eq!(error.code(), ErrorCode::Auth);
        assert_eq!(error.http_status(), Some(401));
    }
    assert_eq!(
        provider.refreshes(),
        1,
        "one refresh for the one set of credentials"
    );
    assert_eq!(server.tokens().len(), 2, "and no resend");

    // A request signed after the failure earns a refresh of its own: its 401 is news.
    let error = boxes.list().await.unwrap_err();
    assert_eq!(error.code(), ErrorCode::Auth);
    assert_eq!(error.http_status(), Some(401));
    assert_eq!(provider.refreshes(), 2);
    assert_eq!(server.tokens().len(), 3);
}

/// A request whose 401 comes back while the failing refresh is still running waits its
/// turn behind it, and takes the failure as its answer too.
#[tokio::test(start_paused = true)]
async fn a_request_whose_401_arrives_during_the_failing_refresh_shares_its_failure() {
    // Both go out before either is answered; the second is answered once the refresh the
    // first earned is running, and the refresh is let go only after that.
    let provider = Refusing::new();
    let server = Sequenced::new(2, 2, provider.started.clone());
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(SequencedTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let calls: Vec<_> = (0..2)
        .map(|_| {
            let client = client.clone();
            tokio::spawn(async move { client.boxes().list().await })
        })
        .collect();
    provider.started.wait().await;
    // Time is paused, so the sleep goes by only once every other task is blocked: the
    // second 401 has been acted on, and its refresh is queued behind the running one.
    tokio::time::sleep(Duration::from_millis(1)).await;
    provider.release.open();
    for call in calls {
        let error = call.await.unwrap().unwrap_err();
        assert_eq!(error.code(), ErrorCode::Auth);
        assert_eq!(error.http_status(), Some(401));
    }

    assert_eq!(provider.refreshes(), 1);
    assert_eq!(server.tokens().len(), 2);
}

/// A provider that renews ahead of expiry, as OAuth libraries do: it hands out `t0` once
/// and `t1` from then on without being asked to refresh, and `t2` once it has been. The
/// first token can be made slow to arrive, so that a signer asking for it after another
/// would — unserialised — be recorded after it.
struct Renewing {
    signings: AtomicUsize,
    refreshes: AtomicUsize,
    first_takes: Duration,
}

impl Renewing {
    fn new() -> Arc<Renewing> {
        Renewing::slow_to_start(Duration::ZERO)
    }

    fn slow_to_start(first_takes: Duration) -> Arc<Renewing> {
        Arc::new(Renewing {
            signings: AtomicUsize::new(0),
            refreshes: AtomicUsize::new(0),
            first_takes,
        })
    }

    fn refreshes(&self) -> usize {
        self.refreshes.load(Ordering::SeqCst)
    }
}

#[async_trait]
impl TokenProvider for Renewing {
    async fn access_token(&self) -> Result<String, Error> {
        let signing = self.signings.fetch_add(1, Ordering::SeqCst) + 1;
        if signing == 1 {
            tokio::time::sleep(self.first_takes).await;
            Ok("t0".to_string())
        } else if self.refreshes() == 0 {
            Ok("t1".to_string())
        } else {
            Ok("t2".to_string())
        }
    }

    async fn refresh(&self) -> bool {
        self.refreshes.fetch_add(1, Ordering::SeqCst);
        true
    }
}

/// A server that answers 401 to whichever credentials the test says it rejects now and an
/// empty list to any other, holding the first `together` arrivals at a barrier so every
/// one of them is signed before any is answered.
struct Rejecting {
    credentials: Mutex<Vec<String>>,
    rejects: Mutex<Vec<String>>,
    arrivals: AtomicUsize,
    together: usize,
    arrived: tokio::sync::Barrier,
}

impl Rejecting {
    fn new(together: usize, rejects: &[&str]) -> Arc<Rejecting> {
        Arc::new(Rejecting {
            credentials: Mutex::new(Vec::new()),
            rejects: Mutex::new(rejects.iter().map(ToString::to_string).collect()),
            arrivals: AtomicUsize::new(0),
            together,
            arrived: tokio::sync::Barrier::new(together.max(1)),
        })
    }

    /// The credentials, as the `Authorization` values, every request so far went out with.
    fn credentials(&self) -> Vec<String> {
        self.credentials.lock().unwrap().clone()
    }

    fn reject(&self, tokens: &[&str]) {
        *self.rejects.lock().unwrap() = tokens.iter().map(ToString::to_string).collect();
    }
}

#[derive(Clone)]
struct RejectingTransport(Arc<Rejecting>);

#[async_trait]
impl HttpClient for RejectingTransport {
    async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, Error> {
        let credential = request.headers()["authorization"]
            .to_str()
            .unwrap()
            .to_string();
        self.0.credentials.lock().unwrap().push(credential.clone());
        let token = credential.trim_start_matches("Bearer ").to_string();
        if self.0.arrivals.fetch_add(1, Ordering::SeqCst) < self.0.together {
            self.0.arrived.wait().await;
        }
        let mut response = Response::new(Body::from("[]"));
        if self.0.rejects.lock().unwrap().contains(&token) {
            *response.status_mut() = StatusCode::UNAUTHORIZED;
        }
        Ok(response)
    }
}

/// A provider that renews ahead of expiry hands `access_token` a new token without being
/// asked to refresh. A 401 on the old token, arriving after the new one has signed a
/// request, is resent with the new one; refreshing would spend the refresh token the
/// provider was just issued. A 401 on the new token itself is refreshed once, as ever.
#[tokio::test]
async fn a_token_the_provider_renews_on_its_own_is_a_renewal_and_a_401_on_the_old_one_is_resent() {
    // The first two requests are signed t0 and t1 in turn, and neither is answered until
    // both are at the server; the t0 one is answered 401, the t1 one 200.
    let server = Rejecting::new(2, &["t0"]);
    let provider = Renewing::new();
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(RejectingTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());
    first.unwrap();
    second.unwrap();
    assert_eq!(
        provider.refreshes(),
        0,
        "the 401 on t0 was answered by t1, which the provider had already handed over"
    );
    let credentials = server.credentials();
    let mut signed_together = credentials[..2].to_vec();
    signed_together.sort();
    assert_eq!(
        signed_together,
        ["Bearer t0", "Bearer t1"],
        "{credentials:?}"
    );
    assert_eq!(credentials[2..], ["Bearer t1"], "{credentials:?}");

    // A 401 on t1 itself is news: the provider would still sign with it, so it is refreshed.
    server.reject(&["t1"]);
    boxes.list().await.unwrap();
    assert_eq!(
        provider.refreshes(),
        1,
        "a 401 on t1 itself is refreshed, once"
    );
    assert_eq!(server.credentials()[3..], ["Bearer t1", "Bearer t2"]);
}

/// The renewal need not have signed anything yet: before the provider is asked to
/// refresh, it is asked what it would sign with now, and a token other than the rejected
/// one is a renewal already made. The request is resent with it and the refresh is not
/// spent. A 401 on the token the provider would still sign with is refreshed.
#[tokio::test]
async fn a_provider_asked_what_it_would_sign_with_is_not_asked_to_refresh_when_it_has_renewed() {
    let server = Rejecting::new(0, &["t0"]);
    let provider = Renewing::new();
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(RejectingTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    client.boxes().list().await.unwrap();
    assert_eq!(
        provider.refreshes(),
        0,
        "the provider had renewed on its own by the time the 401 came back"
    );
    assert_eq!(server.credentials(), ["Bearer t0", "Bearer t1"]);

    server.reject(&["t1"]);
    client.boxes().list().await.unwrap();
    assert_eq!(
        provider.refreshes(),
        1,
        "a 401 on the token the provider would still sign with is refreshed"
    );
    assert_eq!(
        server.credentials(),
        ["Bearer t0", "Bearer t1", "Bearer t1", "Bearer t2"]
    );
}

/// Two requests are signed at once, and the provider is slow to hand the first its t0 and
/// quick to hand the second its t1: recorded in the order the signings ended, t1 would be
/// the earlier token and t0 the renewal, and a genuine 401 on t1 would be taken for one
/// already answered and merely resent. Signings are serialised, so the counts record the
/// order the provider issued in, and the 401 on t1 is refreshed, once.
///
/// With the serialisation this passes whatever the scheduler does. Without it, it fails
/// only when the second signer reaches the provider inside the first one's delay, which the
/// delay makes all but certain but cannot promise: whether a signer is waiting on the lock
/// is not something a test outside the client can see.
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn signings_are_recorded_in_the_order_the_provider_issued_in() {
    let server = Rejecting::new(0, &["t1"]);
    let provider = Renewing::slow_to_start(Duration::from_millis(100));
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(RejectingTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let calls: Vec<_> = (0..2)
        .map(|_| {
            let client = client.clone();
            tokio::spawn(async move { client.boxes().list().await })
        })
        .collect();
    for call in calls {
        call.await.unwrap().unwrap();
    }

    assert_eq!(
        provider.refreshes(),
        1,
        "the 401 on t1, the token the provider issued last, was refreshed"
    );
    // The signings are serialised; the sends after them are not, so which of t0 and t1
    // reaches the server first is the scheduler's to decide.
    let mut credentials = server.credentials();
    credentials.sort();
    assert_eq!(
        credentials,
        ["Bearer t0", "Bearer t1", "Bearer t2"],
        "each token went out once, and t1 was refreshed to t2 rather than resent"
    );
}

/// A strategy of the caller's that signs every request differently, counting how often
/// it was asked to refresh and signing acceptably from then on. The test keeps the
/// counters behind the handle the client takes.
#[derive(Default)]
struct Distinct {
    signings: AtomicUsize,
    refreshes: AtomicUsize,
}

struct DistinctAuth(Arc<Distinct>);

#[async_trait]
impl AuthStrategy for DistinctAuth {
    async fn authenticate(&self, request: &mut Request<Bytes>) -> Result<(), Error> {
        let signing = self.0.signings.fetch_add(1, Ordering::SeqCst) + 1;
        let prefix = if self.0.refreshes.load(Ordering::SeqCst) == 0 {
            "signature"
        } else {
            "renewed"
        };
        let value = format!("Bearer {prefix}-{signing}").parse().unwrap();
        request.headers_mut().insert("authorization", value);
        Ok(())
    }

    async fn refresh(&self) -> bool {
        self.0.refreshes.fetch_add(1, Ordering::SeqCst);
        true
    }
}

/// Only the SDK's own bearer strategy is read for a renewal: a strategy of the caller's
/// may legitimately sign every request differently, and a 401 on one of its requests
/// asks it to refresh, once, however many other requests it has signed since.
#[tokio::test]
async fn a_strategy_of_the_callers_that_signs_every_request_differently_is_not_taken_for_renewing()
{
    // Both requests are at the server before either is answered; the first is rejected,
    // the second, signed differently, is not.
    let server = Rejecting::new(2, &["signature-1"]);
    let strategy = Arc::new(Distinct::default());
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .auth_strategy(DistinctAuth(strategy.clone()))
        .http_client(RejectingTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());
    first.unwrap();
    second.unwrap();

    assert_eq!(
        strategy.refreshes.load(Ordering::SeqCst),
        1,
        "the differing signature was not taken for a renewal: the 401 was refreshed"
    );
    assert_eq!(
        server.credentials(),
        [
            "Bearer signature-1",
            "Bearer signature-2",
            "Bearer renewed-3"
        ]
    );
}

/// A provider that cannot hand over any token once it has handed over its first, as one
/// whose own renewal inside `access_token` has failed would, counting how often it is asked
/// to refresh.
#[derive(Default)]
struct Failing {
    signings: AtomicUsize,
    refreshes: AtomicUsize,
}

#[async_trait]
impl TokenProvider for Failing {
    async fn access_token(&self) -> Result<String, Error> {
        if self.signings.fetch_add(1, Ordering::SeqCst) == 0 {
            Ok("t0".to_string())
        } else {
            Err(Error::auth("the token could not be renewed"))
        }
    }

    async fn refresh(&self) -> bool {
        self.refreshes.fetch_add(1, Ordering::SeqCst);
        true
    }
}

/// Before a refresh is spent, the provider is asked what it would sign with. A provider that
/// cannot hand over a token at all is the refresh failing: the request gets its 401, and the
/// provider is not asked to refresh on top of the renewal it has just failed to make.
#[tokio::test]
async fn a_provider_that_cannot_hand_over_a_token_fails_the_refresh_without_refreshing() {
    let server = Rejecting::new(0, &["t0"]);
    let provider = Arc::new(Failing::default());
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .token_provider(provider.clone())
        .http_client(RejectingTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let error = client.boxes().list().await.unwrap_err();
    assert_eq!(error.code(), ErrorCode::Auth, "{error}");
    assert_eq!(
        provider.refreshes.load(Ordering::SeqCst),
        0,
        "the failed renewal is the refresh's answer"
    );
    assert_eq!(server.credentials(), ["Bearer t0"], "and nothing is resent");
}

/// A strategy of the caller's that signs through the SDK's own bearer strategy and then
/// rewrites the header with a per-request signature, counting how often it is asked to
/// refresh.
struct Wrapping {
    inner: BearerAuth<StaticTokenProvider>,
    signings: AtomicUsize,
    refreshes: AtomicUsize,
}

struct WrappingAuth(Arc<Wrapping>);

#[async_trait]
impl AuthStrategy for WrappingAuth {
    async fn authenticate(&self, request: &mut Request<Bytes>) -> Result<(), Error> {
        self.0.inner.authenticate(request).await?;
        let signing = self.0.signings.fetch_add(1, Ordering::SeqCst) + 1;
        let signed = format!("Bearer signed-{signing}").parse().unwrap();
        request.headers_mut().insert("authorization", signed);
        Ok(())
    }

    async fn refresh(&self) -> bool {
        self.0.refreshes.fetch_add(1, Ordering::SeqCst);
        true
    }
}

/// Signing through `BearerAuth` does not make a caller's strategy the SDK's own: its
/// differing headers are not taken for renewals, and a 401 on one of its requests asks it
/// to refresh, once, however many other requests it signed since.
#[tokio::test]
async fn a_strategy_of_the_callers_that_signs_through_bearer_auth_is_not_taken_for_renewing() {
    let server = Rejecting::new(2, &["signed-1"]);
    let strategy = Arc::new(Wrapping {
        inner: BearerAuth::new(StaticTokenProvider::new("static")),
        signings: AtomicUsize::new(0),
        refreshes: AtomicUsize::new(0),
    });
    let client = Client::builder(Config::default().with_base_url("https://hey.test"))
        .auth_strategy(WrappingAuth(strategy.clone()))
        .http_client(RejectingTransport(server.clone()))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let boxes = client.boxes();
    let (first, second) = tokio::join!(boxes.list(), boxes.list());
    first.unwrap();
    second.unwrap();

    assert_eq!(
        strategy.refreshes.load(Ordering::SeqCst),
        1,
        "the second request's differing signature was not taken for a renewal"
    );
    assert_eq!(
        server.credentials(),
        ["Bearer signed-1", "Bearer signed-2", "Bearer signed-3"]
    );
}
