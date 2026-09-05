use hey_sdk::error::{
    EXIT_AMBIGUOUS, EXIT_API, EXIT_AUTH, EXIT_FORBIDDEN, EXIT_NETWORK, EXIT_NOT_FOUND,
    EXIT_RATE_LIMIT, EXIT_USAGE, EXIT_VALIDATION, MAX_ERROR_BODY_BYTES, MAX_ERROR_MESSAGE_BYTES,
};
use hey_sdk::{Error, ErrorCode};
use reqwest::header::HeaderMap;
use reqwest::{Method, StatusCode};
use serde_json::{Value, json};

#[test]
fn an_error_reads_as_its_message_and_then_its_hint() {
    let bare = Error::api(500, "something broke");
    assert_eq!("something broke", bare.to_string());

    let hinted = bare.with_hint("check the logs");
    assert_eq!("something broke: check the logs", hinted.to_string());
}

#[test]
fn the_cause_stays_reachable() {
    let cause = std::io::Error::other("root cause");
    let error = Error::network(cause);

    assert_eq!(ErrorCode::Network, error.code());
    assert_eq!(
        "root cause",
        std::error::Error::source(&error).unwrap().to_string()
    );
}

#[test]
fn every_code_carries_the_exit_status_the_cli_leaves_with() {
    let expected = [
        (ErrorCode::Usage, EXIT_USAGE),
        (ErrorCode::NotFound, EXIT_NOT_FOUND),
        (ErrorCode::Auth, EXIT_AUTH),
        (ErrorCode::Forbidden, EXIT_FORBIDDEN),
        (ErrorCode::RateLimit, EXIT_RATE_LIMIT),
        (ErrorCode::Network, EXIT_NETWORK),
        (ErrorCode::Api, EXIT_API),
        (ErrorCode::Validation, EXIT_VALIDATION),
        (ErrorCode::Ambiguous, EXIT_AMBIGUOUS),
        (ErrorCode::Conflict, EXIT_VALIDATION),
        (ErrorCode::CircuitOpen, EXIT_API),
        (ErrorCode::BulkheadFull, EXIT_API),
    ];

    for (code, exit_code) in expected {
        assert_eq!(exit_code, code.exit_code(), "{code}");
        assert_eq!(exit_code, Error::new(code, "x").exit_code(), "{code}");
    }
}

#[test]
fn the_codes_are_the_strings_the_other_sdks_use() {
    let expected = [
        (ErrorCode::Usage, "usage"),
        (ErrorCode::NotFound, "not_found"),
        (ErrorCode::Auth, "auth_required"),
        (ErrorCode::Forbidden, "forbidden"),
        (ErrorCode::RateLimit, "rate_limit"),
        (ErrorCode::Network, "network"),
        (ErrorCode::Api, "api_error"),
        (ErrorCode::Validation, "validation"),
        (ErrorCode::Ambiguous, "ambiguous"),
        (ErrorCode::Conflict, "conflict"),
        (ErrorCode::CircuitOpen, "circuit_open"),
        (ErrorCode::BulkheadFull, "bulkhead_full"),
    ];

    for (code, name) in expected {
        assert_eq!(name, code.as_str());
        assert_eq!(name, code.to_string());
    }
}

#[test]
fn is_code_answers_for_the_code_it_was_built_with() {
    let error = Error::usage("bad arg");

    assert!(error.is_code(ErrorCode::Usage));
    assert!(!error.is_code(ErrorCode::Api));
}

#[test]
fn a_usage_error_carries_the_message_and_the_hint() {
    let bare = Error::usage("bad arg");
    assert_eq!(ErrorCode::Usage, bare.code());
    assert_eq!("bad arg", bare.message());
    assert_eq!(None, bare.hint());

    let hinted = Error::usage_with_hint("bad arg", "try --help");
    assert_eq!("bad arg", hinted.message());
    assert_eq!(Some("try --help"), hinted.hint());
}

#[test]
fn a_not_found_error_names_the_resource_and_the_identifier() {
    let bare = Error::not_found("Topic", 42);
    assert_eq!(ErrorCode::NotFound, bare.code());
    assert_eq!("Topic not found: 42", bare.message());
    assert_eq!(Some(404), bare.http_status());

    let hinted = Error::not_found_with_hint("Topic", 42, "check the ID");
    assert_eq!("Topic not found: 42", hinted.message());
    assert_eq!(Some("check the ID"), hinted.hint());
}

#[test]
fn an_auth_error_is_an_unauthorized_one() {
    let error = Error::auth("not logged in");

    assert_eq!(ErrorCode::Auth, error.code());
    assert_eq!(Some(401), error.http_status());
    assert!(!error.is_retryable());
}

#[test]
fn a_forbidden_error_is_a_403_and_a_scope_one_says_what_to_do() {
    let plain = Error::forbidden("nope");
    assert_eq!(ErrorCode::Forbidden, plain.code());
    assert_eq!(Some(403), plain.http_status());
    assert_eq!(None, plain.hint());

    let scope = Error::forbidden_scope();
    assert_eq!(ErrorCode::Forbidden, scope.code());
    assert_eq!("Access denied: insufficient scope", scope.message());
    assert_eq!(Some("Re-authenticate with full scope"), scope.hint());
    assert_eq!(Some(403), scope.http_status());
}

#[test]
fn a_rate_limit_error_says_how_long_to_wait_when_the_server_said() {
    let vague = Error::rate_limit(None);
    assert_eq!(ErrorCode::RateLimit, vague.code());
    assert_eq!("Rate limited", vague.message());
    assert_eq!(Some("Try again later"), vague.hint());
    assert_eq!(Some(429), vague.http_status());
    assert!(vague.is_retryable());

    assert_eq!(
        Some("Try again in 30 seconds"),
        Error::rate_limit(Some(30)).hint()
    );
    assert_eq!(Some("Try again later"), Error::rate_limit(Some(0)).hint());
}

#[test]
fn a_network_error_is_retryable_and_repeats_the_cause_as_its_hint() {
    let error = Error::network(std::io::Error::other("connection refused"));

    assert_eq!(ErrorCode::Network, error.code());
    assert_eq!("Network error", error.message());
    assert_eq!(Some("connection refused"), error.hint());
    assert!(error.is_retryable());
}

#[test]
fn an_api_error_keeps_the_status_it_was_given() {
    let error = Error::api(500, "server error");

    assert_eq!(ErrorCode::Api, error.code());
    assert_eq!(Some(500), error.http_status());
}

#[test]
fn a_conflict_is_a_409_that_leaves_with_the_validation_status() {
    let error = Error::conflict("already started");

    assert_eq!(ErrorCode::Conflict, error.code());
    assert_eq!("already started", error.message());
    assert_eq!(Some(409), error.http_status());
    assert_eq!(EXIT_VALIDATION, error.exit_code());
}

#[test]
fn a_validation_error_joins_the_models_own_messages() {
    let reported = Error::validation(&["Title can't be blank".to_string(), "Too late".to_string()]);
    assert_eq!("Title can't be blank; Too late", reported.message());
    assert_eq!(Some(422), reported.http_status());

    assert_eq!("validation error", Error::validation(&[]).message());
}

#[test]
fn an_ambiguous_error_names_up_to_five_matches() {
    let two = Error::ambiguous("contact", &["Alice".to_string(), "Bob".to_string()]);
    assert_eq!(ErrorCode::Ambiguous, two.code());
    assert_eq!("Ambiguous contact", two.message());
    assert_eq!(Some("Did you mean: Alice, Bob"), two.hint());

    let five: Vec<String> = (1..=5).map(|number| format!("Contact {number}")).collect();
    assert_eq!(
        Some("Did you mean: Contact 1, Contact 2, Contact 3, Contact 4, Contact 5"),
        Error::ambiguous("contact", &five).hint()
    );

    let six: Vec<String> = (1..=6).map(|number| format!("Contact {number}")).collect();
    assert_eq!(
        Some("Be more specific"),
        Error::ambiguous("contact", &six).hint()
    );
    assert_eq!(
        Some("Be more specific"),
        Error::ambiguous("contact", &[]).hint()
    );
}

#[test]
fn any_other_error_wraps_as_an_api_error_describing_itself() {
    let error = Error::from_std(std::io::Error::other("something"));

    assert_eq!(ErrorCode::Api, error.code());
    assert_eq!("something", error.message());
    assert_eq!(
        "something",
        std::error::Error::source(&error).unwrap().to_string()
    );
}

#[test]
fn a_401_reads_as_authentication_required() {
    let error = respond(401, &Method::GET, &[], b"");

    assert_eq!(ErrorCode::Auth, error.code());
    assert_eq!("authentication required", error.message());
    assert_eq!(Some(401), error.http_status());
}

#[test]
fn a_403_blames_the_scope_only_when_the_request_was_writing() {
    let read = respond(403, &Method::GET, &[], b"");
    assert_eq!(ErrorCode::Forbidden, read.code());
    assert_eq!("access denied", read.message());

    let write = respond(403, &Method::POST, &[], b"");
    assert_eq!(ErrorCode::Forbidden, write.code());
    assert_eq!("Access denied: insufficient scope", write.message());
    assert_eq!(Some("Re-authenticate with full scope"), write.hint());
}

#[test]
fn a_404_reads_as_a_missing_resource() {
    let error = respond(404, &Method::GET, &[], b"");

    assert_eq!(ErrorCode::NotFound, error.code());
    assert_eq!("resource not found", error.message());
    assert_eq!(Some(404), error.http_status());
}

#[test]
fn a_422_reads_as_a_validation_error() {
    let error = respond(422, &Method::POST, &[], b"");

    assert_eq!(ErrorCode::Validation, error.code());
    assert_eq!("validation error", error.message());
    assert_eq!(Some(422), error.http_status());
    assert!(!error.is_retryable());
}

#[test]
fn a_429_is_retryable_and_repeats_what_retry_after_asked_for() {
    let error = respond(429, &Method::GET, &[("retry-after", "30")], b"");

    assert_eq!(ErrorCode::RateLimit, error.code());
    assert_eq!("rate limited - try again later", error.message());
    assert_eq!(Some("Try again in 30 seconds"), error.hint());
    assert!(error.is_retryable());

    assert_eq!(
        Some("Try again later"),
        respond(429, &Method::GET, &[], b"").hint()
    );
}

#[test]
fn a_server_error_is_retryable_and_a_client_one_is_not() {
    let server = respond(503, &Method::GET, &[], b"");
    assert_eq!(ErrorCode::Api, server.code());
    assert_eq!(Some(503), server.http_status());
    assert!(server.is_retryable());

    let client = respond(418, &Method::GET, &[], b"");
    assert_eq!(ErrorCode::Api, client.code());
    assert!(!client.is_retryable());
}

#[test]
fn the_request_id_is_kept_so_support_can_find_the_request() {
    let error = respond(500, &Method::GET, &[("x-request-id", "abc-123")], b"");

    assert_eq!(Some("abc-123"), error.request_id());
    assert_eq!(None, respond(500, &Method::GET, &[], b"").request_id());
}

#[test]
fn the_servers_own_message_becomes_the_hint_when_nothing_else_did() {
    let explained = respond(404, &Method::GET, &[], br#"{"message":"No such box"}"#);
    assert_eq!(Some("No such box"), explained.hint());

    let scope = respond(403, &Method::POST, &[], br#"{"error":"read-only token"}"#);
    assert_eq!(Some("Re-authenticate with full scope"), scope.hint());
}

#[test]
fn the_body_stays_on_the_error_for_a_caller_who_needs_more_than_the_hint() {
    let refused = respond(
        409,
        &Method::POST,
        &[],
        br#"{"contact_id":9,"conflicting_contact_ids":[4,5]}"#,
    );

    assert_eq!(
        refused.body(),
        Some(&br#"{"contact_id":9,"conflicting_contact_ids":[4,5]}"#[..])
    );
    let clash: Value = refused.body_json().unwrap();
    assert_eq!(clash["conflicting_contact_ids"], json!([4, 5]));

    let bodiless = respond(500, &Method::GET, &[], b"");
    assert_eq!(bodiless.body(), None);
    assert_eq!(bodiless.body_json::<Value>(), None);
    assert_eq!(Error::usage("nothing was sent").body(), None);

    let prose = respond(500, &Method::GET, &[], b"not JSON at all");
    assert_eq!(prose.body(), Some(&b"not JSON at all"[..]));
    assert_eq!(prose.body_json::<Value>(), None);
}

/// An error is diagnostic, not a copy of the answer: a server that says a megabyte about a
/// refusal has said everything useful long before the cap, and the hint is a line of prose.
#[test]
fn a_failure_body_and_its_message_are_both_capped() {
    let long = "x".repeat(MAX_ERROR_BODY_BYTES + 4096);
    let sprawling = respond(500, &Method::GET, &[], long.as_bytes());
    assert_eq!(MAX_ERROR_BODY_BYTES, sprawling.body().unwrap().len());

    let message = "y".repeat(MAX_ERROR_MESSAGE_BYTES + 100);
    let wordy = respond(
        422,
        &Method::POST,
        &[],
        json!({ "message": message }).to_string().as_bytes(),
    );
    let hint = wordy.hint().unwrap();
    assert_eq!(MAX_ERROR_MESSAGE_BYTES, hint.chars().count());
    assert!(hint.ends_with("..."), "{hint}");

    let brief = respond(
        422,
        &Method::POST,
        &[],
        br#"{"message":"Title can't be blank"}"#,
    );
    assert_eq!(Some("Title can't be blank"), brief.hint());
}

fn respond(status: u16, method: &Method, headers: &[(&str, &str)], body: &[u8]) -> Error {
    let mut map = HeaderMap::new();
    for (name, value) in headers {
        map.insert(
            reqwest::header::HeaderName::from_bytes(name.as_bytes()).unwrap(),
            value.parse().unwrap(),
        );
    }
    Error::from_response(StatusCode::from_u16(status).unwrap(), method, &map, body)
}
