#![allow(dead_code)]

use std::sync::Mutex;
use std::time::Duration;

use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use hey_sdk::{Client, ClientBuilder, Config, Error, StaticTokenProvider};
use wiremock::MockServer;

pub const TOKEN: &str = "test-token";

/// A client pointed at the mock server. The backoff is wound right down so a test that
/// exercises retries still runs in milliseconds, and the jitter is off so it is repeatable.
pub fn builder(server: &MockServer) -> ClientBuilder {
    Client::builder(Config::default().with_base_url(server.uri()))
        .token_provider(StaticTokenProvider::new(TOKEN))
        .base_delay(Duration::from_millis(20))
        .max_delay(Duration::from_millis(20))
        .max_jitter(Duration::ZERO)
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
