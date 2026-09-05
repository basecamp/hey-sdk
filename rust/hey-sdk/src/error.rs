use std::fmt;

use reqwest::header::HeaderMap;
use reqwest::{Method, StatusCode};
use serde::de::DeserializeOwned;

/// The call succeeded.
pub const EXIT_OK: i32 = 0;
/// Invalid arguments or flags.
pub const EXIT_USAGE: i32 = 1;
/// Resource not found.
pub const EXIT_NOT_FOUND: i32 = 2;
/// Not authenticated.
pub const EXIT_AUTH: i32 = 3;
/// Access denied, usually a scope the token does not carry.
pub const EXIT_FORBIDDEN: i32 = 4;
/// Rate limited (429).
pub const EXIT_RATE_LIMIT: i32 = 5;
/// Connection, DNS or timeout failure.
pub const EXIT_NETWORK: i32 = 6;
/// The server returned an error.
pub const EXIT_API: i32 = 7;
/// A name matched more than one record.
pub const EXIT_AMBIGUOUS: i32 = 8;
/// The server rejected the contents of the request (422), or the request conflicts with
/// the state the server already holds (409).
pub const EXIT_VALIDATION: i32 = 9;

/// The most of a failure's body an error keeps, mirroring Go's `MaxErrorBodyBytes`. A body
/// past it is kept up to the bound and no further: an error is diagnostic, and a server
/// answering a refusal with a megabyte of anything has said everything useful long before.
pub const MAX_ERROR_BODY_BYTES: usize = 1 << 20;

/// The most of a server's own message an error's hint carries, mirroring Go's
/// `MaxErrorMessageBytes`. A longer one is cut and ends in `...`.
pub const MAX_ERROR_MESSAGE_BYTES: usize = 500;

/// Machine-readable error categories, shared with the other HEY SDKs.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum ErrorCode {
    Usage,
    NotFound,
    Auth,
    Forbidden,
    RateLimit,
    Network,
    Api,
    Validation,
    Ambiguous,
    Conflict,
    /// The scope's circuit breaker is open, so the SDK refused the call itself. See
    /// [`crate::resilience`].
    CircuitOpen,
    /// The scope already has as many calls in flight as its bulkhead allows.
    BulkheadFull,
}

impl ErrorCode {
    pub fn as_str(&self) -> &'static str {
        match self {
            ErrorCode::Usage => "usage",
            ErrorCode::NotFound => "not_found",
            ErrorCode::Auth => "auth_required",
            ErrorCode::Forbidden => "forbidden",
            ErrorCode::RateLimit => "rate_limit",
            ErrorCode::Network => "network",
            ErrorCode::Api => "api_error",
            ErrorCode::Validation => "validation",
            ErrorCode::Ambiguous => "ambiguous",
            ErrorCode::Conflict => "conflict",
            ErrorCode::CircuitOpen => "circuit_open",
            ErrorCode::BulkheadFull => "bulkhead_full",
        }
    }

    /// The process exit status a command should end with for this category. A conflict
    /// leaves with [`EXIT_VALIDATION`]: both mean the server refused what was asked of it.
    /// A call the SDK refused for itself leaves with [`EXIT_API`], which is where Go's
    /// `ExitCodeFor` sends the codes it does not name.
    pub fn exit_code(&self) -> i32 {
        match self {
            ErrorCode::Usage => EXIT_USAGE,
            ErrorCode::NotFound => EXIT_NOT_FOUND,
            ErrorCode::Auth => EXIT_AUTH,
            ErrorCode::Forbidden => EXIT_FORBIDDEN,
            ErrorCode::RateLimit => EXIT_RATE_LIMIT,
            ErrorCode::Network => EXIT_NETWORK,
            ErrorCode::Api => EXIT_API,
            ErrorCode::Validation => EXIT_VALIDATION,
            ErrorCode::Ambiguous => EXIT_AMBIGUOUS,
            ErrorCode::Conflict => EXIT_VALIDATION,
            ErrorCode::CircuitOpen | ErrorCode::BulkheadFull => EXIT_API,
        }
    }
}

impl fmt::Display for ErrorCode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

/// The error every SDK call can answer with.
#[derive(Debug)]
pub struct Error {
    code: ErrorCode,
    message: String,
    hint: Option<String>,
    http_status: Option<u16>,
    retryable: bool,
    request_id: Option<String>,
    source: Option<Box<dyn std::error::Error + Send + Sync>>,
    response_too_large: bool,
    /// What HEY answered the failure with, kept whole so a caller can read the model's own
    /// account of the refusal. See [`Error::body`]. A boxed slice rather than a
    /// [`bytes::Bytes`]: an error is never cloned, so there is nothing for the sharing to
    /// buy, and half the width keeps every `Result` in the crate under clippy's
    /// `result_large_err` bound.
    body: Option<Box<[u8]>>,
}

impl Error {
    pub fn new(code: ErrorCode, message: impl Into<String>) -> Error {
        Error {
            code,
            message: message.into(),
            hint: None,
            http_status: None,
            retryable: false,
            request_id: None,
            source: None,
            response_too_large: false,
            body: None,
        }
    }

    pub fn usage(message: impl Into<String>) -> Error {
        Error::new(ErrorCode::Usage, message)
    }

    pub fn usage_with_hint(message: impl Into<String>, hint: impl Into<String>) -> Error {
        Error::usage(message).with_hint(hint)
    }

    pub fn not_found(resource: &str, identifier: impl fmt::Display) -> Error {
        Error::new(
            ErrorCode::NotFound,
            format!("{resource} not found: {identifier}"),
        )
        .with_status(404)
    }

    pub fn not_found_with_hint(
        resource: &str,
        identifier: impl fmt::Display,
        hint: impl Into<String>,
    ) -> Error {
        Error::not_found(resource, identifier).with_hint(hint)
    }

    pub fn auth(message: impl Into<String>) -> Error {
        Error::new(ErrorCode::Auth, message).with_status(401)
    }

    pub fn forbidden(message: impl Into<String>) -> Error {
        Error::new(ErrorCode::Forbidden, message).with_status(403)
    }

    pub fn forbidden_scope() -> Error {
        Error::forbidden("Access denied: insufficient scope")
            .with_hint("Re-authenticate with full scope")
    }

    /// The rate-limit error a caller raises for itself, worded the way the other SDKs word
    /// theirs: "Rate limited". A 429 that came back from HEY reads "rate limited - try
    /// again later" instead — see [`Error::from_response`].
    pub fn rate_limit(retry_after: Option<u64>) -> Error {
        Error::new(ErrorCode::RateLimit, "Rate limited")
            .with_hint(retry_hint(retry_after))
            .with_status(429)
            .retryable()
    }

    /// The client's own rate limiter refused a call, so nothing was sent. It shares
    /// [`ErrorCode::RateLimit`] with a 429 from HEY and is told apart by carrying no HTTP
    /// status; Go keeps the two apart with a sentinel instead.
    pub fn rate_limited() -> Error {
        Error::new(ErrorCode::RateLimit, "rate limit exceeded")
    }

    /// The scope's circuit breaker is open, so the SDK refused the call itself.
    pub fn circuit_open() -> Error {
        Error::new(ErrorCode::CircuitOpen, "circuit breaker is open")
    }

    /// The caller dropped the future before it finished — a `tokio::time::timeout` that
    /// expired, a `select!` that took another branch. Nobody is waiting for this error: it
    /// is what the operation hooks are told the call ended as, so the bookkeeping every
    /// layer keeps per operation is closed out rather than left open.
    ///
    /// It is a [`ErrorCode::Network`] because that is what a call that never got an answer
    /// is — and it counts against the scope's circuit breaker like any other, since a call
    /// the caller had to give up waiting for says the same thing about HEY as one that
    /// timed out on its own. It is not retryable: there is nobody left to answer.
    pub fn cancelled() -> Error {
        Error::new(ErrorCode::Network, "operation cancelled")
    }

    /// The scope already has as many calls in flight as its bulkhead allows.
    pub fn bulkhead_full() -> Error {
        Error::new(ErrorCode::BulkheadFull, "bulkhead is full")
    }

    pub fn network(source: impl std::error::Error + Send + Sync + 'static) -> Error {
        Error::new(ErrorCode::Network, "Network error")
            .with_hint(source.to_string())
            .retryable()
            .with_source(source)
    }

    pub fn api(status: u16, message: impl Into<String>) -> Error {
        Error::new(ErrorCode::Api, message).with_status(status)
    }

    pub fn conflict(message: impl Into<String>) -> Error {
        Error::new(ErrorCode::Conflict, message).with_status(409)
    }

    /// A body the client refused to read, because reading it whole is what the caller
    /// would have gone on to do. It carries no HTTP status of its own: the answer never
    /// arrived in full, so there is nothing to report about it but the refusal.
    pub fn response_too_large(limit: usize, method: &Method, path: &str) -> Error {
        Error {
            response_too_large: true,
            ..Error::api(
                0,
                format!("{method} {path}: response body exceeds {limit} bytes"),
            )
        }
    }

    /// Puts a refusal behind the error a status maps to, so a body too large to read on a
    /// non-2xx answer still reports the status the answer carried.
    pub(crate) fn refusing(mut self, refusal: Error) -> Error {
        self.response_too_large = refusal.response_too_large;
        if self.hint.is_none() {
            self.hint = Some(refusal.to_string());
        }
        self.with_source(refusal)
    }

    /// The messages the model itself produced, joined. With none of them the error still
    /// says something: "validation error".
    pub fn validation(messages: &[String]) -> Error {
        let mut message = messages.join("; ");
        if message.is_empty() {
            message = "validation error".to_string();
        }
        Error::new(ErrorCode::Validation, message).with_status(422)
    }

    /// A name that matched more than one record. Up to five matches are named in the hint;
    /// beyond that the only useful advice is to narrow the search.
    ///
    /// Go renders the matches with `%v`, as `Did you mean: [Alice Bob]`. Here they read as
    /// a list — `Did you mean: Alice, Bob` — which is what a Rust caller printing the hint
    /// would expect.
    pub fn ambiguous(resource: &str, matches: &[String]) -> Error {
        let hint = match matches.len() {
            1..=5 => format!("Did you mean: {}", matches.join(", ")),
            _ => "Be more specific".to_string(),
        };
        Error::new(ErrorCode::Ambiguous, format!("Ambiguous {resource}")).with_hint(hint)
    }

    /// Wraps an error from outside the SDK as an API error that reads the way the original
    /// did, for the callers that have to answer with this type and nothing better fits.
    pub fn from_std(source: impl std::error::Error + Send + Sync + 'static) -> Error {
        Error::new(ErrorCode::Api, source.to_string()).with_source(source)
    }

    /// Maps a non-2xx response onto the SDK's error vocabulary. The hint carries whatever
    /// message the server put in the body, when it sent one, and the body itself is kept on
    /// the error for a caller that needs more of it than a hint — see [`Error::body`].
    pub fn from_response(
        status: StatusCode,
        method: &Method,
        headers: &HeaderMap,
        body: &[u8],
    ) -> Error {
        let error = match status.as_u16() {
            401 => Error::auth("authentication required"),
            403 if method != Method::GET => Error::forbidden_scope(),
            403 => Error::forbidden("access denied"),
            404 => Error::new(ErrorCode::NotFound, "resource not found").with_status(404),
            422 => Error::new(ErrorCode::Validation, "validation error").with_status(422),
            429 => Error::new(ErrorCode::RateLimit, "rate limited - try again later")
                .with_hint(retry_hint(retry_after_seconds(headers)))
                .with_status(429)
                .retryable(),
            code => {
                let error = Error::api(code, format!("API error: {status}"));
                if status.is_server_error() {
                    error.retryable()
                } else {
                    error
                }
            }
        };
        let error = match headers
            .get("x-request-id")
            .and_then(|value| value.to_str().ok())
        {
            Some(request_id) => error.with_request_id(request_id),
            None => error,
        };
        let mut error = match (error.hint.is_none(), server_message(body)) {
            (true, Some(message)) => error.with_hint(message),
            _ => error,
        };
        if !body.is_empty() {
            let kept = body.len().min(MAX_ERROR_BODY_BYTES);
            error.body = Some(Box::from(&body[..kept]));
        }
        error
    }

    pub fn with_hint(mut self, hint: impl Into<String>) -> Error {
        self.hint = Some(hint.into());
        self
    }

    pub fn with_status(mut self, status: u16) -> Error {
        self.http_status = Some(status);
        self
    }

    pub fn with_request_id(mut self, request_id: impl Into<String>) -> Error {
        self.request_id = Some(request_id.into());
        self
    }

    pub fn with_source(mut self, source: impl std::error::Error + Send + Sync + 'static) -> Error {
        self.source = Some(Box::new(source));
        self
    }

    pub fn retryable(mut self) -> Error {
        self.retryable = true;
        self
    }

    pub fn code(&self) -> ErrorCode {
        self.code
    }

    pub fn is_code(&self, code: ErrorCode) -> bool {
        self.code == code
    }

    /// The process exit status a command should end with for this error.
    pub fn exit_code(&self) -> i32 {
        self.code.exit_code()
    }

    pub fn message(&self) -> &str {
        &self.message
    }

    pub fn hint(&self) -> Option<&str> {
        self.hint.as_deref()
    }

    pub fn http_status(&self) -> Option<u16> {
        self.http_status
    }

    pub fn is_retryable(&self) -> bool {
        self.retryable
    }

    pub fn request_id(&self) -> Option<&str> {
        self.request_id.as_deref()
    }

    /// The answer was longer than the client will hold in memory, whether that refusal is
    /// the error itself or sits behind the status the answer carried.
    pub fn is_response_too_large(&self) -> bool {
        self.response_too_large
    }

    /// What HEY answered the failure with, up to [`MAX_ERROR_BODY_BYTES`]. Several
    /// endpoints describe a refusal in the body rather than in the status alone — the
    /// contacts a clashing write collided with, the fields a 422 objected to — and this is
    /// where that account is kept. It is `None` when the answer carried no body, when the
    /// SDK raised the error itself, and when the body was too long to read.
    pub fn body(&self) -> Option<&[u8]> {
        self.body.as_deref()
    }

    /// The failure body read as `T`, or `None` when there is no body or it does not read as
    /// one.
    ///
    /// ```no_run
    /// # use hey_sdk::Error;
    /// # use serde::Deserialize;
    /// #[derive(Deserialize)]
    /// struct Refusal {
    ///     errors: Vec<String>,
    /// }
    ///
    /// # fn report(error: &Error) {
    /// if let Some(refusal) = error.body_json::<Refusal>() {
    ///     println!("{}", refusal.errors.join("; "));
    /// }
    /// # }
    /// ```
    pub fn body_json<T: DeserializeOwned>(&self) -> Option<T> {
        serde_json::from_slice(self.body()?).ok()
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match &self.hint {
            Some(hint) => write!(f, "{}: {hint}", self.message),
            None => f.write_str(&self.message),
        }
    }
}

impl std::error::Error for Error {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        self.source
            .as_deref()
            .map(|source| source as &(dyn std::error::Error + 'static))
    }
}

impl From<reqwest::Error> for Error {
    fn from(error: reqwest::Error) -> Error {
        if error.is_decode() {
            Error::api(0, "unreadable response")
                .with_hint(error.to_string())
                .with_source(error)
        } else {
            Error::network(error)
        }
    }
}

impl From<serde_json::Error> for Error {
    fn from(error: serde_json::Error) -> Error {
        Error::api(0, "unexpected JSON")
            .with_hint(error.to_string())
            .with_source(error)
    }
}

impl From<url::ParseError> for Error {
    fn from(error: url::ParseError) -> Error {
        Error::usage(format!("invalid URL: {error}")).with_source(error)
    }
}

fn retry_hint(retry_after: Option<u64>) -> String {
    match retry_after {
        Some(seconds) if seconds > 0 => format!("Try again in {seconds} seconds"),
        _ => "Try again later".to_string(),
    }
}

/// The wait `Retry-After` asks for, in seconds. The header carries either a count of
/// seconds or the HTTP-date the wait is over, and a date already past asks for no wait at
/// all.
pub(crate) fn retry_after_seconds(headers: &HeaderMap) -> Option<u64> {
    let asked = headers.get("retry-after")?.to_str().ok()?.trim();
    match asked.parse::<i64>() {
        Ok(seconds) => u64::try_from(seconds).ok(),
        Err(_) => {
            let until = chrono::DateTime::parse_from_rfc2822(asked).ok()?;
            u64::try_from(
                until
                    .signed_duration_since(chrono::Utc::now())
                    .num_seconds(),
            )
            .ok()
        }
    }
}

fn server_message(body: &[u8]) -> Option<String> {
    let value: serde_json::Value = serde_json::from_slice(body).ok()?;
    let message = value
        .get("message")
        .or_else(|| value.get("error"))?
        .as_str()?;
    Some(truncate(message, MAX_ERROR_MESSAGE_BYTES))
}

/// Cuts a message to `limit` characters, saying so with a trailing `...`, the way Go's
/// `truncateString` does.
pub(crate) fn truncate(message: &str, limit: usize) -> String {
    if message.chars().count() <= limit {
        message.to_string()
    } else {
        let kept: String = message.chars().take(limit - 3).collect();
        format!("{kept}...")
    }
}
