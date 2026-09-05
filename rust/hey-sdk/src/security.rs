use reqwest::header::{HeaderMap, HeaderValue};
use url::Url;

use crate::error::Error;

const SENSITIVE_HEADERS: &[&str] = &["authorization", "cookie", "set-cookie", "x-csrf-token"];

/// Refuses an endpoint that would carry credentials over plain HTTP, unless it is on this
/// machine.
pub fn require_secure_endpoint(url: &Url) -> Result<(), Error> {
    if url.scheme() == "https" || (url.scheme() == "http" && is_localhost(url)) {
        Ok(())
    } else {
        Err(Error::usage(format!("{url} must use HTTPS")))
    }
}

pub fn is_localhost(url: &Url) -> bool {
    match url.host_str() {
        Some(host) => {
            let host = host
                .trim_start_matches('[')
                .trim_end_matches(']')
                .to_ascii_lowercase();
            host == "localhost"
                || host == "127.0.0.1"
                || host == "::1"
                || host.ends_with(".localhost")
        }
        None => false,
    }
}

/// Whether two URLs share a scheme, host and port, with default ports treated as equal to
/// their explicit form.
pub fn is_same_origin(a: &Url, b: &Url) -> bool {
    a.scheme().eq_ignore_ascii_case(b.scheme())
        && a.host_str()
            .unwrap_or_default()
            .eq_ignore_ascii_case(b.host_str().unwrap_or_default())
        && a.port_or_known_default() == b.port_or_known_default()
}

/// A copy of the headers with credentials replaced, for logging.
pub fn redact_headers(headers: &HeaderMap) -> HeaderMap {
    let mut redacted = headers.clone();
    for name in SENSITIVE_HEADERS {
        if redacted.contains_key(*name) {
            redacted.insert(*name, HeaderValue::from_static("[REDACTED]"));
        }
    }
    redacted
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn localhost_may_use_plain_http() {
        assert!(require_secure_endpoint(&Url::parse("http://localhost:3000").unwrap()).is_ok());
        assert!(require_secure_endpoint(&Url::parse("http://127.0.0.1:8080/x").unwrap()).is_ok());
        assert!(require_secure_endpoint(&Url::parse("http://app.localhost").unwrap()).is_ok());
        assert!(require_secure_endpoint(&Url::parse("https://app.hey.com").unwrap()).is_ok());
        assert!(require_secure_endpoint(&Url::parse("http://evil.example.com").unwrap()).is_err());
    }

    #[test]
    fn same_origin_ignores_default_ports_and_case() {
        let a = Url::parse("https://App.HEY.com/boxes").unwrap();
        assert!(is_same_origin(
            &a,
            &Url::parse("HTTPS://app.hey.com:443/other").unwrap()
        ));
        assert!(!is_same_origin(
            &a,
            &Url::parse("http://app.hey.com/other").unwrap()
        ));
        assert!(!is_same_origin(
            &a,
            &Url::parse("https://evil.example.com/boxes").unwrap()
        ));
    }

    #[test]
    fn credentials_are_redacted() {
        let mut headers = HeaderMap::new();
        headers.insert("Authorization", HeaderValue::from_static("Bearer secret"));
        headers.insert("Accept", HeaderValue::from_static("application/json"));
        let redacted = redact_headers(&headers);
        assert_eq!(redacted["authorization"], "[REDACTED]");
        assert_eq!(redacted["accept"], "application/json");
    }
}
