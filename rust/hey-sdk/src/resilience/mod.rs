//! Three ways to stop a struggling HEY from taking the caller down with it: a circuit
//! breaker that gives up on an operation that keeps failing, a bulkhead that caps how many
//! calls of one kind run at once, and a rate limiter that keeps the caller inside a budget
//! of its own.
//!
//! Each layer is installed on the builder and keeps its counters per scope — the service
//! and operation as the model names them, `Boxes.ListBoxes` — so a failing search does not
//! shut down the mailbox reads next to it.
//!
//! A call meets them at the gate, before anything is sent. The bulkhead is the one that may
//! hold it there: a scope already running all it may keeps the caller waiting for up to
//! [`BulkheadConfig::max_wait`] and refuses only when no room comes free.
//!
//! ```no_run
//! use hey_sdk::resilience::{CircuitBreakerConfig, ResilienceConfig};
//! use hey_sdk::{Client, Config, StaticTokenProvider};
//!
//! # fn build() -> Result<Client, hey_sdk::Error> {
//! Client::builder(Config::default())
//!     .token_provider(StaticTokenProvider::new("token"))
//!     .circuit_breaker(CircuitBreakerConfig {
//!         failure_threshold: 3,
//!         ..CircuitBreakerConfig::default()
//!     })
//!     .build()
//! # }
//! ```
//!
//! Or all three at their defaults with [`ClientBuilder::resilience`] and
//! [`ResilienceConfig::default`].
//!
//! # Where this parts company with Go
//!
//! Go gates through a separate `GatingHooks` interface and its chain stops at the first
//! member that implements it, so installing two resilience layers there silently runs only
//! the outer one. Here gating is part of [`Hooks`] itself and each layer holds the hooks it
//! wrapped, so every installed layer is asked. Two layers means two gates, which is what
//! asking for both should mean.

mod bulkhead;
mod circuit_breaker;
mod rate_limit;

use std::collections::HashMap;
use std::fmt;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use async_trait::async_trait;

use crate::client::ClientBuilder;
use crate::error::{Error, ErrorCode};
use crate::observability::{Hooks, OperationInfo, OperationState, RequestInfo, RequestResult};

pub use bulkhead::{Bulkhead, BulkheadConfig, BulkheadPermit};
pub use circuit_breaker::{CircuitBreaker, CircuitBreakerConfig};
pub use rate_limit::{RateLimitConfig, RateLimiter};

/// Every resilience layer at once. A member left `None` is a layer left off;
/// [`ResilienceConfig::default`] turns all three on at their own defaults.
#[derive(Debug, Clone)]
pub struct ResilienceConfig {
    pub circuit_breaker: Option<CircuitBreakerConfig>,
    pub bulkhead: Option<BulkheadConfig>,
    pub rate_limit: Option<RateLimitConfig>,
}

impl Default for ResilienceConfig {
    fn default() -> ResilienceConfig {
        ResilienceConfig {
            circuit_breaker: Some(CircuitBreakerConfig::default()),
            bulkhead: Some(BulkheadConfig::default()),
            rate_limit: Some(RateLimitConfig::default()),
        }
    }
}

impl ResilienceConfig {
    /// No layer at all: what to start from when turning on one or two by hand, and what
    /// [`ClientBuilder::circuit_breaker`] and its neighbours build on.
    pub fn none() -> ResilienceConfig {
        ResilienceConfig {
            circuit_breaker: None,
            bulkhead: None,
            rate_limit: None,
        }
    }
}

impl ClientBuilder {
    /// Installs the layers the config asks for, keeping whatever hooks the builder already
    /// carries: they still hear about every operation and request.
    pub fn resilience(mut self, config: ResilienceConfig) -> ClientBuilder {
        self.hooks = Arc::new(ResilienceHooks::new(self.hooks.clone(), config));
        self
    }

    /// Installs the circuit breaker alone.
    pub fn circuit_breaker(self, config: CircuitBreakerConfig) -> ClientBuilder {
        self.resilience(ResilienceConfig {
            circuit_breaker: Some(config),
            ..ResilienceConfig::none()
        })
    }

    /// Installs the bulkhead alone.
    pub fn bulkhead(self, config: BulkheadConfig) -> ClientBuilder {
        self.resilience(ResilienceConfig {
            bulkhead: Some(config),
            ..ResilienceConfig::none()
        })
    }

    /// Installs the rate limiter alone.
    pub fn rate_limit(self, config: RateLimitConfig) -> ClientBuilder {
        self.resilience(ResilienceConfig {
            rate_limit: Some(config),
            ..ResilienceConfig::none()
        })
    }
}

/// Whether a failed operation counts against the scope's circuit breaker. A call the SDK
/// refused for itself does not — the breaker would be counting its own work — and neither
/// does anything HEY answered for itself, however unwelcome. What is left is HEY failing to
/// answer: a network error or a 5xx.
///
/// Go also trips on any error that is not its own `*Error`, since a stray error from
/// somewhere else says nothing about HEY's health. Every error here is [`Error`], so that
/// case has no counterpart; the nearest thing, an [`ErrorCode::Api`] carrying a 5xx, trips.
pub fn should_trip_circuit(error: &Error) -> bool {
    match error.code() {
        ErrorCode::CircuitOpen | ErrorCode::BulkheadFull | ErrorCode::RateLimit => false,
        ErrorCode::Network => true,
        _ => error.http_status().is_some_and(|status| status >= 500),
    }
}

/// Where a breaker or a limiter reads the time. Tests hand one they move themselves.
#[derive(Clone)]
pub struct Clock(Arc<dyn Fn() -> Instant + Send + Sync>);

impl Clock {
    pub fn new(now: impl Fn() -> Instant + Send + Sync + 'static) -> Clock {
        Clock(Arc::new(now))
    }

    pub fn now(&self) -> Instant {
        (self.0)()
    }
}

impl Default for Clock {
    fn default() -> Clock {
        Clock::new(Instant::now)
    }
}

impl fmt::Debug for Clock {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("Clock")
    }
}

/// The [`Hooks`] the layers run behind: they gate an operation before it is sent, hold the
/// bulkhead permit for as long as it runs, and tell the breaker how it went. The hooks the
/// builder already carried are wrapped rather than replaced, and hear everything they would
/// have heard on their own.
pub struct ResilienceHooks {
    inner: Arc<dyn Hooks>,
    circuit_breakers: Option<Registry<CircuitBreaker>>,
    bulkheads: Option<Registry<Bulkhead>>,
    rate_limiter: Option<RateLimiter>,
    /// Permits taken in [`Hooks::on_operation_gate`], waiting for the
    /// [`Hooks::on_operation_start`] that carries them into the operation. Go keys the same
    /// handoff by a counter it puts in the context; the SDK calls start straight after a
    /// gate it let through, with nothing awaited in between, so a permit is only ever
    /// waiting for the operation that took it. Permits from one scope are interchangeable,
    /// which is all the scope needs to key them by.
    pending: Mutex<HashMap<String, Vec<BulkheadPermit>>>,
}

impl ResilienceHooks {
    pub fn new(inner: Arc<dyn Hooks>, config: ResilienceConfig) -> ResilienceHooks {
        ResilienceHooks {
            inner,
            circuit_breakers: config
                .circuit_breaker
                .map(|config| Registry::new(move || CircuitBreaker::new(config.clone()))),
            bulkheads: config
                .bulkhead
                .map(|config| Registry::new(move || Bulkhead::new(config.clone()))),
            rate_limiter: config.rate_limit.map(RateLimiter::new),
            pending: Mutex::default(),
        }
    }

    /// The layers in the order a call meets them: the breaker says whether this operation is
    /// worth trying, the bulkhead makes room for it, and the limiter spends a token. A
    /// permit taken on the way is released by being dropped when a later layer refuses.
    ///
    /// Waiting for room is the bulkhead's own doing: [`Bulkhead::acquire`] holds the caller
    /// for up to [`BulkheadConfig::max_wait`] before giving up on the scope.
    async fn admit(&self, scope: &str) -> Result<Option<BulkheadPermit>, Error> {
        if let Some(breakers) = &self.circuit_breakers
            && !breakers.get(scope).allow()
        {
            return Err(Error::circuit_open());
        }

        let permit = match &self.bulkheads {
            Some(bulkheads) => Some(bulkheads.get(scope).acquire().await?),
            None => None,
        };

        if let Some(limiter) = &self.rate_limiter
            && !limiter.allow()
        {
            return Err(Error::rate_limited());
        }

        Ok(permit)
    }

    fn record(&self, scope: &str, outcome: Result<(), &Error>) {
        if let Some(breakers) = &self.circuit_breakers {
            let breaker = breakers.get(scope);
            match outcome {
                Ok(()) => breaker.record_success(),
                Err(error) if should_trip_circuit(error) => breaker.record_failure(),
                Err(_) => {}
            }
        }
    }
}

#[async_trait]
impl Hooks for ResilienceHooks {
    async fn on_operation_gate(&self, op: &OperationInfo) -> Result<(), Error> {
        let scope = scope_of(op);
        let permit = self.admit(&scope).await?;
        self.inner.on_operation_gate(op).await?;
        if let Some(permit) = permit {
            self.pending
                .lock()
                .unwrap()
                .entry(scope)
                .or_default()
                .push(permit);
        }
        Ok(())
    }

    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        let permit = self
            .pending
            .lock()
            .unwrap()
            .get_mut(&scope_of(op))
            .and_then(Vec::pop);
        Some(Box::new(Held {
            permit,
            inner: self.inner.on_operation_start(op),
        }))
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        state: OperationState,
        outcome: Result<(), &Error>,
        duration: Duration,
    ) {
        let held = match state.and_then(|state| state.downcast::<Held>().ok()) {
            Some(held) => *held,
            None => Held::default(),
        };
        drop(held.permit);
        self.record(&scope_of(op), outcome);
        self.inner
            .on_operation_end(op, held.inner, outcome, duration);
    }

    fn on_request_start(&self, info: &RequestInfo) {
        self.inner.on_request_start(info);
    }

    /// HEY asking for a wait is worth more than the limiter's own accounting, so a 429 arms
    /// the limiter for as long as it asked — a minute when it did not say. A 503 is only
    /// sometimes about load, so it arms the limiter only when it named a wait.
    fn on_request_end(&self, info: &RequestInfo, result: &RequestResult<'_>) {
        if let Some(limiter) = &self.rate_limiter {
            let asked = result.retry_after.filter(|seconds| *seconds > 0);
            match result.status.map(|status| status.as_u16()) {
                Some(429) => limiter.set_retry_after_in(Duration::from_secs(asked.unwrap_or(60))),
                Some(503) => {
                    if let Some(seconds) = asked {
                        limiter.set_retry_after_in(Duration::from_secs(seconds));
                    }
                }
                _ => {}
            }
        }
        self.inner.on_request_end(info, result);
    }

    fn on_retry(&self, info: &RequestInfo, next_attempt: u32, cause: &Error) {
        self.inner.on_retry(info, next_attempt, cause);
    }
}

/// What an operation carries from its start to its end: the bulkhead permit it holds, and
/// whatever the wrapped hooks made for themselves.
#[derive(Default)]
struct Held {
    permit: Option<BulkheadPermit>,
    inner: OperationState,
}

fn scope_of(op: &OperationInfo) -> String {
    format!("{}.{}", op.service, op.operation)
}

/// One breaker or bulkhead per scope, built on first sight and kept for the life of the
/// client.
struct Registry<T> {
    build: Box<dyn Fn() -> T + Send + Sync>,
    entries: Mutex<HashMap<String, Arc<T>>>,
}

impl<T> Registry<T> {
    fn new(build: impl Fn() -> T + Send + Sync + 'static) -> Registry<T> {
        Registry {
            build: Box::new(build),
            entries: Mutex::default(),
        }
    }

    fn get(&self, scope: &str) -> Arc<T> {
        self.entries
            .lock()
            .unwrap()
            .entry(scope.to_string())
            .or_insert_with(|| Arc::new((self.build)()))
            .clone()
    }
}

/// A clock a test moves itself, and the handle it moves it by.
#[cfg(test)]
pub(crate) fn test_clock() -> (Clock, Arc<Mutex<Instant>>) {
    let now = Arc::new(Mutex::new(Instant::now()));
    let reading = now.clone();
    (Clock::new(move || *reading.lock().unwrap()), now)
}

#[cfg(test)]
pub(crate) fn advance(clock: &Arc<Mutex<Instant>>, elapsed: Duration) {
    let mut now = clock.lock().unwrap();
    *now += elapsed;
}

#[cfg(test)]
mod tests {
    use std::borrow::Cow;

    use super::*;

    #[test]
    fn the_default_config_installs_every_layer() {
        let config = ResilienceConfig::default();

        assert!(config.circuit_breaker.is_some());
        assert!(config.bulkhead.is_some());
        assert!(config.rate_limit.is_some());
    }

    #[test]
    fn only_what_hey_failed_to_answer_trips_the_breaker() {
        let cases = [
            (Error::circuit_open(), false),
            (Error::bulkhead_full(), false),
            (Error::rate_limited(), false),
            (Error::rate_limit(Some(3)), false),
            (
                Error::network(std::io::Error::other("connection refused")),
                true,
            ),
            (Error::api(500, "boom"), true),
            (Error::api(503, "unavailable"), true),
            (Error::api(400, "bad request"), false),
            (Error::auth("authentication required"), false),
            (Error::usage("bad argument"), false),
            (Error::not_found("box", 1), false),
        ];

        for (error, expected) in cases {
            assert_eq!(expected, should_trip_circuit(&error), "{error}");
        }
    }

    #[test]
    fn a_registry_keeps_one_breaker_per_scope() {
        let registry = Registry::new(|| CircuitBreaker::new(CircuitBreakerConfig::default()));

        assert!(Arc::ptr_eq(
            &registry.get("scope1"),
            &registry.get("scope1")
        ));
        assert!(!Arc::ptr_eq(
            &registry.get("scope1"),
            &registry.get("scope2")
        ));
    }

    #[test]
    fn a_registry_keeps_one_bulkhead_per_scope() {
        let registry = Registry::new(|| Bulkhead::new(BulkheadConfig::default()));

        assert!(Arc::ptr_eq(
            &registry.get("scope1"),
            &registry.get("scope1")
        ));
        assert!(!Arc::ptr_eq(
            &registry.get("scope1"),
            &registry.get("scope2")
        ));
    }

    #[tokio::test]
    async fn a_breaker_only_config_refuses_once_the_scope_has_failed_enough() {
        let hooks = hooks(ResilienceConfig {
            circuit_breaker: Some(CircuitBreakerConfig {
                failure_threshold: 2,
                ..CircuitBreakerConfig::default()
            }),
            ..ResilienceConfig::none()
        });

        fail(&hooks, &operation("Boxes", "ListBoxes")).await;
        assert!(
            hooks
                .on_operation_gate(&operation("Boxes", "ListBoxes"))
                .await
                .is_ok()
        );
        fail(&hooks, &operation("Boxes", "ListBoxes")).await;

        let refused = hooks
            .on_operation_gate(&operation("Boxes", "ListBoxes"))
            .await
            .unwrap_err();
        assert_eq!(ErrorCode::CircuitOpen, refused.code());
        assert_eq!("circuit breaker is open", refused.message());
        assert!(
            hooks
                .on_operation_gate(&operation("Boxes", "GetBox"))
                .await
                .is_ok()
        );
    }

    #[tokio::test]
    async fn a_bulkhead_with_no_wait_refuses_a_scope_that_is_already_busy() {
        let hooks = hooks(ResilienceConfig {
            bulkhead: Some(BulkheadConfig {
                max_concurrent: 1,
                max_wait: Duration::ZERO,
            }),
            ..ResilienceConfig::none()
        });
        let op = operation("Boxes", "ListBoxes");

        hooks.on_operation_gate(&op).await.unwrap();
        let state = hooks.on_operation_start(&op);

        let refused = hooks.on_operation_gate(&op).await.unwrap_err();
        assert_eq!(ErrorCode::BulkheadFull, refused.code());
        assert_eq!("bulkhead is full", refused.message());
        assert!(
            hooks
                .on_operation_gate(&operation("Boxes", "GetBox"))
                .await
                .is_ok()
        );

        hooks.on_operation_end(&op, state, Ok(()), Duration::ZERO);
        assert!(hooks.on_operation_gate(&op).await.is_ok());
    }

    #[tokio::test]
    async fn a_limiter_only_config_refuses_once_the_budget_is_spent() {
        let hooks = hooks(ResilienceConfig {
            rate_limit: Some(RateLimitConfig {
                requests_per_second: 0.0001,
                burst_size: 1,
                ..RateLimitConfig::default()
            }),
            ..ResilienceConfig::none()
        });
        let op = operation("Boxes", "ListBoxes");

        hooks.on_operation_gate(&op).await.unwrap();

        let refused = hooks.on_operation_gate(&op).await.unwrap_err();
        assert_eq!(ErrorCode::RateLimit, refused.code());
        assert_eq!("rate limit exceeded", refused.message());
        assert_eq!(None, refused.http_status());
    }

    /// The limiter refusing has to give back the permit the bulkhead just handed out, or the
    /// scope would lose a slot to every refusal.
    #[tokio::test]
    async fn a_refused_call_gives_back_the_permit_it_took() {
        let hooks = hooks(ResilienceConfig {
            bulkhead: Some(BulkheadConfig {
                max_concurrent: 1,
                max_wait: Duration::ZERO,
            }),
            rate_limit: Some(RateLimitConfig {
                requests_per_second: 0.0001,
                burst_size: 1,
                ..RateLimitConfig::default()
            }),
            ..ResilienceConfig::none()
        });
        let op = operation("Boxes", "ListBoxes");

        hooks.on_operation_gate(&op).await.unwrap();
        let state = hooks.on_operation_start(&op);
        hooks.on_operation_end(&op, state, Ok(()), Duration::ZERO);

        assert_eq!(
            ErrorCode::RateLimit,
            hooks.on_operation_gate(&op).await.unwrap_err().code()
        );
        assert_eq!(
            1,
            hooks
                .bulkheads
                .as_ref()
                .unwrap()
                .get("Boxes.ListBoxes")
                .available()
        );
    }

    /// And so does a layer installed underneath this one refusing, which is how two
    /// resilience layers stack.
    #[tokio::test]
    async fn a_call_refused_below_gives_back_the_permit_too() {
        let hooks = ResilienceHooks::new(
            Arc::new(Refusing),
            ResilienceConfig {
                bulkhead: Some(BulkheadConfig {
                    max_concurrent: 1,
                    max_wait: Duration::ZERO,
                }),
                ..ResilienceConfig::none()
            },
        );
        let op = operation("Boxes", "ListBoxes");

        let refused = hooks.on_operation_gate(&op).await.unwrap_err();

        assert_eq!(ErrorCode::Usage, refused.code());
        assert_eq!(
            1,
            hooks
                .bulkheads
                .as_ref()
                .unwrap()
                .get("Boxes.ListBoxes")
                .available()
        );
    }

    #[tokio::test]
    async fn the_hooks_underneath_still_hear_everything() {
        let recorder = Arc::new(Recorder::default());
        let hooks = ResilienceHooks::new(recorder.clone(), ResilienceConfig::default());
        let op = operation("Boxes", "ListBoxes");

        hooks.on_operation_gate(&op).await.unwrap();
        let state = hooks.on_operation_start(&op);
        hooks.on_operation_end(&op, state, Ok(()), Duration::ZERO);

        assert_eq!(
            vec!["gate", "start", "end carrying its own"],
            recorder.entries()
        );
    }

    fn hooks(config: ResilienceConfig) -> ResilienceHooks {
        ResilienceHooks::new(Arc::new(crate::observability::NoopHooks), config)
    }

    async fn fail(hooks: &ResilienceHooks, op: &OperationInfo) {
        hooks.on_operation_gate(op).await.unwrap();
        let state = hooks.on_operation_start(op);
        hooks.on_operation_end(op, state, Err(&Error::api(500, "boom")), Duration::ZERO);
    }

    fn operation(service: &'static str, operation: &'static str) -> OperationInfo {
        OperationInfo {
            service: Cow::Borrowed(service),
            operation: Cow::Borrowed(operation),
            resource_type: Cow::Borrowed("box"),
            is_mutation: false,
            resource_id: None,
        }
    }

    #[derive(Default)]
    struct Recorder {
        entries: Mutex<Vec<String>>,
    }

    impl Recorder {
        fn entries(&self) -> Vec<String> {
            self.entries.lock().unwrap().clone()
        }

        fn record(&self, entry: &str) {
            self.entries.lock().unwrap().push(entry.to_string());
        }
    }

    #[async_trait]
    impl Hooks for Recorder {
        async fn on_operation_gate(&self, _op: &OperationInfo) -> Result<(), Error> {
            self.record("gate");
            Ok(())
        }

        fn on_operation_start(&self, _op: &OperationInfo) -> OperationState {
            self.record("start");
            Some(Box::new("its own"))
        }

        fn on_operation_end(
            &self,
            _op: &OperationInfo,
            state: OperationState,
            _outcome: Result<(), &Error>,
            _duration: Duration,
        ) {
            let carried = match state.and_then(|state| state.downcast::<&str>().ok()) {
                Some(carried) => *carried,
                None => "nothing",
            };
            self.record(&format!("end carrying {carried}"));
        }
    }

    /// A layer installed before the resilience ones that turns everything away.
    struct Refusing;

    #[async_trait]
    impl Hooks for Refusing {
        async fn on_operation_gate(&self, _op: &OperationInfo) -> Result<(), Error> {
            Err(Error::usage("blocked"))
        }
    }
}
