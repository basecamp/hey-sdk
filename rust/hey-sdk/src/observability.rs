//! What the SDK tells an application about the calls it makes: which operation is running,
//! which HTTP requests it takes, and what each one answered.

use std::any::Any;
use std::borrow::Cow;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use reqwest::{Method, StatusCode};
use url::Url;

use crate::error::Error;

/// The semantic identity of a call: the service and operation as the model names them,
/// the kind of record they touch, and whether they change anything.
///
/// This is what the call *means*, not what goes over the wire. A hand-written wrapper is
/// free to report itself as something other than the route it sends — HEY stops a time
/// track by updating it, and a wrapper that does so says `StopTimeTrack`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OperationInfo {
    /// The service handle the call came from: `Boxes`, `TimeTracks`.
    pub service: Cow<'static, str>,
    /// The operation as the model names it: `ListBoxes`, `MovePostings`.
    pub operation: Cow<'static, str>,
    /// The kind of record the call acts on, snake_cased: `box`, `time_track`.
    pub resource_type: Cow<'static, str>,
    pub is_mutation: bool,
    /// The record the path names, when it names one.
    pub resource_id: Option<i64>,
}

/// One HTTP request the SDK is about to make, or has just made. `attempt` counts from 1
/// across the whole operation, the resend after a credential refresh included.
#[derive(Debug, Clone)]
pub struct RequestInfo {
    pub method: Method,
    pub url: Url,
    pub attempt: u32,
}

/// How one HTTP request turned out.
#[derive(Debug)]
pub struct RequestResult<'a> {
    /// What HEY answered, or `None` when nothing came back at all.
    pub status: Option<StatusCode>,
    /// How long HEY took to answer, before the body was read.
    pub duration: Duration,
    /// What this request failed with, whether or not the SDK went on to resend it.
    pub error: Option<&'a Error>,
    /// The body came out of the response cache: HEY answered 304 and the SDK read the
    /// entry it was holding.
    pub from_cache: bool,
    /// The SDK would resend this, given an attempt to spare.
    pub retryable: bool,
    /// The seconds `Retry-After` named, on the statuses that carry one.
    pub retry_after: Option<u64>,
}

/// Whatever [`Hooks::on_operation_start`] hands the matching [`Hooks::on_operation_end`].
pub type OperationState = Option<Box<dyn Any + Send>>;

/// The callbacks the SDK makes as it works. Every one does nothing by default, so an
/// implementation says only what it cares about.
///
/// [`Hooks::on_operation_start`] can hand back a value — a span, a timer, a correlation
/// id — which the SDK carries through the operation and gives back to
/// [`Hooks::on_operation_end`]. It arrives boxed as [`Any`], so take it back with
/// `downcast`:
///
/// ```
/// use std::time::{Duration, Instant};
///
/// use hey_sdk::Error;
/// use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
///
/// struct Timing;
///
/// impl Hooks for Timing {
///     fn on_operation_start(&self, _op: &OperationInfo) -> OperationState {
///         Some(Box::new(Instant::now()))
///     }
///
///     fn on_operation_end(
///         &self,
///         op: &OperationInfo,
///         state: OperationState,
///         _outcome: Result<(), &Error>,
///         _duration: Duration,
///     ) {
///         if let Some(started) = state.and_then(|state| state.downcast::<Instant>().ok()) {
///             println!("{} took {:?}", op.operation, started.elapsed());
///         }
///     }
/// }
/// ```
#[async_trait]
pub trait Hooks: Send + Sync {
    /// Asked before anything is sent. An `Err` abandons the operation and becomes its
    /// answer, so a policy can refuse a call before it reaches HEY.
    ///
    /// It is the one callback that may wait: a policy that admits calls a few at a time can
    /// hold the caller until there is room rather than turning it away — which is how
    /// [`BulkheadConfig::max_wait`](crate::resilience::BulkheadConfig::max_wait) is spent.
    /// The rest are told what happened and are not asked to decide anything, so they stay
    /// synchronous.
    async fn on_operation_gate(&self, _op: &OperationInfo) -> Result<(), Error> {
        Ok(())
    }

    fn on_operation_start(&self, _op: &OperationInfo) -> OperationState {
        None
    }

    /// Told how the operation ended and how long the whole of it took, requests, waits
    /// and all.
    fn on_operation_end(
        &self,
        _op: &OperationInfo,
        _state: OperationState,
        _outcome: Result<(), &Error>,
        _duration: Duration,
    ) {
    }

    fn on_request_start(&self, _info: &RequestInfo) {}

    fn on_request_end(&self, _info: &RequestInfo, _result: &RequestResult<'_>) {}

    /// Told about a resend before it is made: the attempt that failed in `info`, the one
    /// about to be made as `next_attempt`, and what prompted it.
    fn on_retry(&self, _info: &RequestInfo, _next_attempt: u32, _cause: &Error) {}

    /// Answers `true` when this implementation does nothing, so [`ChainHooks`] can leave
    /// it out.
    fn is_noop(&self) -> bool {
        false
    }
}

#[async_trait]
impl<H: Hooks + ?Sized> Hooks for Arc<H> {
    async fn on_operation_gate(&self, op: &OperationInfo) -> Result<(), Error> {
        (**self).on_operation_gate(op).await
    }

    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        (**self).on_operation_start(op)
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        state: OperationState,
        outcome: Result<(), &Error>,
        duration: Duration,
    ) {
        (**self).on_operation_end(op, state, outcome, duration);
    }

    fn on_request_start(&self, info: &RequestInfo) {
        (**self).on_request_start(info);
    }

    fn on_request_end(&self, info: &RequestInfo, result: &RequestResult<'_>) {
        (**self).on_request_end(info, result);
    }

    fn on_retry(&self, info: &RequestInfo, next_attempt: u32, cause: &Error) {
        (**self).on_retry(info, next_attempt, cause);
    }

    fn is_noop(&self) -> bool {
        (**self).is_noop()
    }
}

/// Hooks that do nothing. What a client runs with until it is given others, and what a
/// chain of nothing comes to.
#[derive(Debug, Clone, Copy, Default)]
pub struct NoopHooks;

impl Hooks for NoopHooks {
    fn is_noop(&self) -> bool {
        true
    }
}

/// Several [`Hooks`] as one. Gates, starts and retries run in order; ends run in reverse,
/// so a hook that wraps the ones after it closes last, the way nested spans do.
pub struct ChainHooks {
    hooks: Vec<Arc<dyn Hooks>>,
}

impl ChainHooks {
    /// Chains what is left once the no-ops are dropped. Nothing left is a [`NoopHooks`],
    /// and one left is that hook itself — a chain of one is just the hook, which is why
    /// this answers some [`Hooks`] rather than always a `ChainHooks`.
    pub fn of(hooks: Vec<Arc<dyn Hooks>>) -> Arc<dyn Hooks> {
        let mut installed: Vec<Arc<dyn Hooks>> =
            hooks.into_iter().filter(|hook| !hook.is_noop()).collect();
        if installed.is_empty() {
            Arc::new(NoopHooks)
        } else if installed.len() == 1 {
            installed.remove(0)
        } else {
            Arc::new(ChainHooks { hooks: installed })
        }
    }
}

#[async_trait]
impl Hooks for ChainHooks {
    /// Asks every member in order and answers the first refusal. Go asks only the first
    /// member that implements its separate gating interface; here gating is part of
    /// [`Hooks`] itself, so the chain stops at whichever member refuses.
    async fn on_operation_gate(&self, op: &OperationInfo) -> Result<(), Error> {
        for hook in &self.hooks {
            hook.on_operation_gate(op).await?;
        }
        Ok(())
    }

    /// Keeps each member's own state, so [`ChainHooks::on_operation_end`] can hand every
    /// one of them back what it made.
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        let states: Vec<OperationState> = self
            .hooks
            .iter()
            .map(|hook| hook.on_operation_start(op))
            .collect();
        Some(Box::new(states))
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        state: OperationState,
        outcome: Result<(), &Error>,
        duration: Duration,
    ) {
        let mut states = member_states(state);
        for hook in self.hooks.iter().rev() {
            hook.on_operation_end(op, states.pop().flatten(), outcome, duration);
        }
    }

    fn on_request_start(&self, info: &RequestInfo) {
        for hook in &self.hooks {
            hook.on_request_start(info);
        }
    }

    fn on_request_end(&self, info: &RequestInfo, result: &RequestResult<'_>) {
        for hook in self.hooks.iter().rev() {
            hook.on_request_end(info, result);
        }
    }

    fn on_retry(&self, info: &RequestInfo, next_attempt: u32, cause: &Error) {
        for hook in &self.hooks {
            hook.on_retry(info, next_attempt, cause);
        }
    }
}

fn member_states(state: OperationState) -> Vec<OperationState> {
    match state.and_then(|state| state.downcast::<Vec<OperationState>>().ok()) {
        Some(states) => *states,
        None => Vec::new(),
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use super::*;
    use crate::ErrorCode;

    #[tokio::test]
    async fn every_noop_callback_is_safe_to_call() {
        let hooks = NoopHooks;
        let op = operation_info();
        let info = request_info();

        hooks.on_operation_gate(&op).await.unwrap();
        let state = hooks.on_operation_start(&op);
        assert!(state.is_none());
        hooks.on_request_start(&info);
        hooks.on_request_end(&info, &request_result());
        hooks.on_retry(&info, 2, &Error::usage("nothing"));
        hooks.on_operation_end(&op, state, Ok(()), Duration::from_secs(1));

        assert!(hooks.is_noop());
    }

    #[test]
    fn a_chain_runs_forwards_and_unwinds_backwards() {
        let log = Log::new();
        let chain = ChainHooks::of(vec![log.recorder("first"), log.recorder("second")]);
        let op = operation_info();

        let state = chain.on_operation_start(&op);
        chain.on_request_start(&request_info());
        chain.on_request_end(&request_info(), &request_result());
        chain.on_retry(&request_info(), 2, &Error::usage("nothing"));
        chain.on_operation_end(&op, state, Ok(()), Duration::from_secs(1));

        assert_eq!(
            log.entries(),
            [
                "first: start Svc.Do",
                "second: start Svc.Do",
                "first: request start 1",
                "second: request start 1",
                "second: request end 200",
                "first: request end 200",
                "first: retry 2",
                "second: retry 2",
                "second: end Svc.Do carrying second",
                "first: end Svc.Do carrying first",
            ]
        );
    }

    #[test]
    fn a_chain_of_one_is_that_hook() {
        let log = Log::new();
        let recorder = log.recorder("only");

        let chain = ChainHooks::of(vec![recorder.clone(), Arc::new(NoopHooks)]);

        assert!(Arc::ptr_eq(&chain, &recorder));
    }

    #[test]
    fn a_chain_of_nothing_but_noops_is_a_noop() {
        assert!(ChainHooks::of(vec![Arc::new(NoopHooks), Arc::new(NoopHooks)]).is_noop());
        assert!(ChainHooks::of(Vec::new()).is_noop());
    }

    #[tokio::test]
    async fn a_chain_answers_the_first_refusal() {
        let log = Log::new();
        let chain = ChainHooks::of(vec![
            log.recorder("first"),
            Arc::new(Refusing),
            log.recorder("third"),
        ]);

        let refused = chain
            .on_operation_gate(&operation_info())
            .await
            .unwrap_err();

        assert_eq!(refused.code(), ErrorCode::Usage);
        assert_eq!(refused.message(), "blocked");
        assert_eq!(log.entries(), ["first: gate Svc.Do"]);
    }

    /// One shared transcript, so a chain's members are seen in the order they were asked.
    struct Log {
        entries: Arc<Mutex<Vec<String>>>,
    }

    impl Log {
        fn new() -> Log {
            Log {
                entries: Arc::new(Mutex::new(Vec::new())),
            }
        }

        fn recorder(&self, name: &'static str) -> Arc<dyn Hooks> {
            Arc::new(Recorder {
                name,
                entries: self.entries.clone(),
            })
        }

        fn entries(&self) -> Vec<String> {
            self.entries.lock().unwrap().clone()
        }
    }

    struct Recorder {
        name: &'static str,
        entries: Arc<Mutex<Vec<String>>>,
    }

    impl Recorder {
        fn record(&self, event: String) {
            self.entries
                .lock()
                .unwrap()
                .push(format!("{}: {event}", self.name));
        }
    }

    #[async_trait]
    impl Hooks for Recorder {
        async fn on_operation_gate(&self, op: &OperationInfo) -> Result<(), Error> {
            self.record(format!("gate {}.{}", op.service, op.operation));
            Ok(())
        }

        fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
            self.record(format!("start {}.{}", op.service, op.operation));
            Some(Box::new(self.name.to_string()))
        }

        fn on_operation_end(
            &self,
            op: &OperationInfo,
            state: OperationState,
            _outcome: Result<(), &Error>,
            _duration: Duration,
        ) {
            let carried = match state.and_then(|state| state.downcast::<String>().ok()) {
                Some(name) => *name,
                None => "nothing".to_string(),
            };
            self.record(format!(
                "end {}.{} carrying {carried}",
                op.service, op.operation
            ));
        }

        fn on_request_start(&self, info: &RequestInfo) {
            self.record(format!("request start {}", info.attempt));
        }

        fn on_request_end(&self, _info: &RequestInfo, result: &RequestResult<'_>) {
            self.record(format!("request end {}", result.status.unwrap().as_u16()));
        }

        fn on_retry(&self, _info: &RequestInfo, next_attempt: u32, _cause: &Error) {
            self.record(format!("retry {next_attempt}"));
        }
    }

    struct Refusing;

    #[async_trait]
    impl Hooks for Refusing {
        async fn on_operation_gate(&self, _op: &OperationInfo) -> Result<(), Error> {
            Err(Error::usage("blocked"))
        }
    }

    fn operation_info() -> OperationInfo {
        OperationInfo {
            service: Cow::Borrowed("Svc"),
            operation: Cow::Borrowed("Do"),
            resource_type: Cow::Borrowed("thing"),
            is_mutation: false,
            resource_id: None,
        }
    }

    fn request_info() -> RequestInfo {
        RequestInfo {
            method: Method::GET,
            url: Url::parse("https://app.hey.example/boxes.json").unwrap(),
            attempt: 1,
        }
    }

    fn request_result() -> RequestResult<'static> {
        RequestResult {
            status: Some(StatusCode::OK),
            duration: Duration::from_millis(3),
            error: None,
            from_cache: false,
            retryable: false,
            retry_after: None,
        }
    }
}
