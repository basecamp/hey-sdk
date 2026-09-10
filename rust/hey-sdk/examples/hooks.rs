//! Hearing what the client does: every operation, every request, every resend.
//!
//! ```sh
//! HEY_TOKEN=... cargo run --example hooks
//! ```

use std::time::Duration;

use hey_sdk::observability::{Hooks, OperationInfo, OperationState, RequestInfo, RequestResult};
use hey_sdk::{Client, Config, Error, StaticTokenProvider};

/// Prints each step to stderr. Every callback has a do-nothing default, so an
/// implementation says only what it cares about; several go on as one with `ChainHooks`.
struct Log;

impl Hooks for Log {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        eprintln!("-> {}.{}", op.service, op.operation);
        None
    }

    fn on_operation_end(
        &self,
        op: &OperationInfo,
        _state: OperationState,
        outcome: Result<(), &Error>,
        duration: Duration,
    ) {
        match outcome {
            Ok(()) => eprintln!("<- {}.{} ok in {duration:?}", op.service, op.operation),
            Err(error) => eprintln!("<- {}.{} {error} in {duration:?}", op.service, op.operation),
        }
    }

    fn on_request_end(&self, info: &RequestInfo, result: &RequestResult<'_>) {
        // The path, not the whole URL: a search's query string carries what was searched
        // for, and a log is the wrong place for it.
        eprintln!(
            "   {} {} attempt {} -> {:?} in {:?}",
            info.method,
            info.url.path(),
            info.attempt,
            result.status,
            result.duration
        );
    }

    fn on_retry(&self, info: &RequestInfo, next_attempt: u32, cause: &Error) {
        eprintln!(
            "   resending {} {} as attempt {next_attempt}: {cause}",
            info.method,
            info.url.path()
        );
    }
}

#[tokio::main]
async fn main() -> Result<(), Error> {
    let token = std::env::var("HEY_TOKEN")
        .map_err(|_| Error::usage("set HEY_TOKEN to a HEY access token"))?;
    let client = Client::builder(Config::default())
        .token_provider(StaticTokenProvider::new(token))
        .hooks(Log)
        .build()?;

    let boxes = client.boxes().list().await?;
    println!("{} boxes", boxes.len());
    Ok(())
}
