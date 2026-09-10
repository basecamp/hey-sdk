//! The `tracing` the SDK emits, behind the `tracing` feature: one span per operation, a
//! child span per attempt, and the debug lines the retry loop and pagination write. Without
//! the feature every one of these is nothing, so the call sites read the same either way.
//!
//! A span is put on a future with `Instrument` and never entered across an `.await`: an
//! entered span is a thread-local, and a task that yields would leave it on whatever the
//! executor runs next. Nothing a caller passed to an operation is recorded — the span names
//! the operation, the attempt, the status and HEY's request id, and that is all.

use crate::http::StatusCode;
use crate::observability::OperationInfo;

#[cfg(feature = "tracing")]
macro_rules! debug {
    ($($arg:tt)*) => { tracing::debug!($($arg)*) };
}

#[cfg(not(feature = "tracing"))]
macro_rules! debug {
    ($($arg:tt)*) => {{}};
}

#[cfg(feature = "tracing")]
macro_rules! warning {
    ($($arg:tt)*) => { tracing::warn!($($arg)*) };
}

#[cfg(not(feature = "tracing"))]
macro_rules! warning {
    ($($arg:tt)*) => {{}};
}

pub(crate) use {debug, warning};

/// The span one operation runs inside, from the gate to the end hook. Its `http.status`
/// and `request_id` are empty until the answer the retry loop settled on is in hand.
#[derive(Clone)]
pub(crate) struct OperationSpan {
    #[cfg(feature = "tracing")]
    span: tracing::Span,
}

impl OperationSpan {
    pub(crate) fn new(info: &OperationInfo) -> OperationSpan {
        #[cfg(not(feature = "tracing"))]
        let _ = info;
        OperationSpan {
            #[cfg(feature = "tracing")]
            span: tracing::info_span!(
                "hey.operation",
                operation = %info.operation,
                service = %info.service,
                http.status = tracing::field::Empty,
                request_id = tracing::field::Empty,
            ),
        }
    }

    /// The span nothing is announced in: a quiet send, which is one request inside another
    /// operation, runs in that operation's span rather than one of its own.
    pub(crate) fn none() -> OperationSpan {
        OperationSpan {
            #[cfg(feature = "tracing")]
            span: tracing::Span::none(),
        }
    }

    #[cfg(feature = "tracing")]
    pub(crate) fn wrap<F: Future>(&self, work: F) -> tracing::instrument::Instrumented<F> {
        tracing::Instrument::instrument(work, self.span.clone())
    }

    #[cfg(not(feature = "tracing"))]
    pub(crate) fn wrap<F: Future>(&self, work: F) -> F {
        work
    }

    pub(crate) fn answered(&self, status: StatusCode, request_id: Option<&str>) {
        #[cfg(feature = "tracing")]
        {
            self.span.record("http.status", status.as_u16());
            if let Some(request_id) = request_id {
                self.span.record("request_id", request_id);
            }
        }
        #[cfg(not(feature = "tracing"))]
        {
            let _ = (status, request_id);
        }
    }
}

/// The span one send runs inside, a child of the operation's. Numbered the way the hooks
/// number attempts: from 1 across the whole operation, the resend after a credential
/// refresh included.
pub(crate) struct AttemptSpan {
    #[cfg(feature = "tracing")]
    span: tracing::Span,
}

impl AttemptSpan {
    pub(crate) fn new(attempt: u32) -> AttemptSpan {
        #[cfg(not(feature = "tracing"))]
        let _ = attempt;
        AttemptSpan {
            #[cfg(feature = "tracing")]
            span: tracing::debug_span!("hey.attempt", attempt, http.status = tracing::field::Empty),
        }
    }

    #[cfg(feature = "tracing")]
    pub(crate) fn wrap<F: Future>(&self, work: F) -> tracing::instrument::Instrumented<F> {
        tracing::Instrument::instrument(work, self.span.clone())
    }

    #[cfg(not(feature = "tracing"))]
    pub(crate) fn wrap<F: Future>(&self, work: F) -> F {
        work
    }

    pub(crate) fn answered(&self, status: StatusCode) {
        #[cfg(feature = "tracing")]
        self.span.record("http.status", status.as_u16());
        #[cfg(not(feature = "tracing"))]
        let _ = status;
    }
}
