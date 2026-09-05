use std::time::Duration;

use hey_sdk::pagination::next_link;
use hey_sdk::routes;
use hey_sdk::{Error, ErrorCode};
use serde_json::{Map, Value};
use url::Url;

use crate::fixtures::{Assertion, TestCase};
use crate::operations::Outcome;
use crate::server::Recorded;

/// One case after it ran: what the SDK answered and what the mock server saw.
pub struct Run<'a> {
    pub case: &'a TestCase,
    pub outcome: &'a Result<Outcome, Error>,
    pub recorded: &'a Recorded,
    pub base_url: &'a str,
}

pub fn check_all(run: &Run) -> Result<(), String> {
    for assertion in &run.case.assertions {
        check(run, assertion)?;
    }
    Ok(())
}

fn check(run: &Run, assertion: &Assertion) -> Result<(), String> {
    match assertion.kind.as_str() {
        "requestCount" => check_request_count(run, assertion),
        "delayBetweenRequests" => check_delay_between_requests(run, assertion),
        "noError" => check_no_error(run),
        "errorCode" => check_error_code(run, assertion),
        "errorField" => check_error_field(run, assertion),
        "statusCode" => check_status_code(run, assertion),
        "requestPath" => check_request_path(run, assertion, First),
        "lastRequestPath" => check_request_path(run, assertion, Last),
        "requestMethod" => check_request_method(run, assertion),
        "requestQuery" => check_request_query(run, assertion, First),
        "lastRequestQuery" => check_request_query(run, assertion, Last),
        "requestBody" => check_request_body(run, assertion),
        "requestForm" => check_request_form(run, assertion, First),
        "lastRequestForm" => check_request_form(run, assertion, Last),
        "headerPresent" => check_header_present(run, assertion),
        "lastRequestHeader" => check_last_request_header(run, assertion),
        "responseMeta" => check_response_meta(run, assertion),
        "urlOrigin" => check_url_origin(run, assertion),
        "responseBody" => check_response_body(run, assertion),
        kind => Err(format!("Unknown assertion type: {kind}")),
    }
}

fn check_request_count(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let expected = expected_int(assertion, "requestCount")?;
    if run.recorded.count as i64 == expected {
        Ok(())
    } else {
        Err(format!(
            "Expected {expected} requests, got {}",
            run.recorded.count
        ))
    }
}

fn check_delay_between_requests(run: &Run, assertion: &Assertion) -> Result<(), String> {
    if run.recorded.times.len() < 2 {
        Ok(())
    } else {
        let delay = run.recorded.times[1].duration_since(run.recorded.times[0]);
        let minimum = Duration::from_millis(assertion.min as u64);
        if delay >= minimum {
            Ok(())
        } else {
            Err(format!("Expected delay >= {minimum:?}, got {delay:?}"))
        }
    }
}

fn check_no_error(run: &Run) -> Result<(), String> {
    match run.outcome {
        Ok(_) => Ok(()),
        Err(error) if empty_statuses(&run.case.operation).contains(&last_status(run)) => {
            Err(format!(
                "Expected no error, got: {error} (the route treats {} as empty for {})",
                last_status(run),
                run.case.operation
            ))
        }
        Err(error) => Err(format!("Expected no error, got: {error}")),
    }
}

/// The statuses the model has the operation treat as "nothing there" rather than an error.
fn empty_statuses(operation: &str) -> &'static [u16] {
    routes::ROUTES
        .iter()
        .find(|route| route.id == operation)
        .map(|route| route.empty_on)
        .unwrap_or_default()
}

fn check_error_code(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let expected = expected_string(assertion, "errorCode")?;
    match run.outcome {
        Ok(_) => Err(format!(
            "Expected error code {expected:?}, but got no error"
        )),
        Err(error) if error.code().as_str() == expected => Ok(()),
        Err(error) => Err(format!(
            "Expected error code {expected:?}, got {:?}",
            error.code().as_str()
        )),
    }
}

fn check_error_field(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let Err(error) = run.outcome else {
        return Err(format!(
            "Expected error field {:?}, but got no error",
            assertion.path
        ));
    };
    match assertion.path.as_str() {
        "httpStatus" => {
            let expected = expected_int(assertion, "errorField.httpStatus")?;
            let actual = error.http_status().unwrap_or_default();
            if i64::from(actual) == expected {
                Ok(())
            } else {
                Err(format!(
                    "Expected error httpStatus {expected}, got {actual}"
                ))
            }
        }
        "retryable" => {
            let expected = expected_bool(assertion, "errorField.retryable")?;
            if error.is_retryable() == expected {
                Ok(())
            } else {
                Err(format!(
                    "Expected error retryable={expected}, got {}",
                    error.is_retryable()
                ))
            }
        }
        "requestId" => {
            let expected = expected_string(assertion, "errorField.requestId")?;
            let actual = error.request_id().unwrap_or_default();
            if actual == expected {
                Ok(())
            } else {
                Err(format!(
                    "Expected error requestId {expected:?}, got {actual:?}"
                ))
            }
        }
        field => Err(format!("Unknown error field: {field}")),
    }
}

/// The status the case is about. A failure has to carry it on the error itself — that is
/// the whole point of the assertion, and reading it off the server instead would pass a
/// case where the SDK dropped the status on the floor. Only a success has no error to read
/// it from, and there the status the server served is the answer.
fn check_status_code(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let expected = expected_int(assertion, "statusCode")?;
    let actual = match run.outcome {
        Err(error) => match error.http_status().filter(|status| *status > 0) {
            Some(status) => status,
            None => {
                return Err(format!(
                    "Expected status code {expected}, but the SDK error carries no HTTP status: {error}"
                ));
            }
        },
        Ok(_) => last_status(run),
    };
    if i64::from(actual) == expected {
        Ok(())
    } else {
        Err(format!("Expected status code {expected}, got {actual}"))
    }
}

fn last_status(run: &Run) -> u16 {
    run.recorded.statuses.last().copied().unwrap_or_default()
}

fn check_request_path(run: &Run, assertion: &Assertion, which: Which) -> Result<(), String> {
    let expected = with_json_extension(expected_string(assertion, "requestPath")?);
    let actual = which.pick(&run.recorded.paths).ok_or_else(no_requests)?;
    if *actual == expected {
        Ok(())
    } else {
        Err(format!(
            "Expected {} request path {expected:?}, got {actual:?}",
            which.label()
        ))
    }
}

/// HEY answers JSON to paths ending in `.json`, and this SDK always puts the extension back
/// on a path — modelled or raw — whose last segment has none. So the expected path is
/// normalised the same way and matched exactly: one answer rather than two.
///
/// The fixtures are shared with the Go runner, which drives the raw generated client and
/// therefore sees the bare paths; this one drives the full client, which is why the
/// normalisation is here and not in the fixtures.
fn with_json_extension(path: &str) -> String {
    let last_segment = path.rsplit('/').next().unwrap_or_default();
    if path.is_empty() || path.ends_with('/') || last_segment.contains('.') {
        path.to_string()
    } else {
        format!("{path}.json")
    }
}

fn check_request_method(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let expected = expected_string(assertion, "requestMethod")?;
    let actual = run.recorded.methods.first().ok_or_else(no_requests)?;
    if actual == expected {
        Ok(())
    } else {
        Err(format!(
            "Expected request method {expected:?}, got {actual:?}"
        ))
    }
}

fn check_request_query(run: &Run, assertion: &Assertion, which: Which) -> Result<(), String> {
    let expected = expected_object(assertion, &assertion.kind)?;
    let query = which.pick(&run.recorded.queries).ok_or_else(no_requests)?;
    for (name, want) in expected {
        let got = query
            .iter()
            .find(|(key, _)| key == name)
            .map(|(_, value)| value.as_str());
        match (want, got) {
            (Value::Null, None) => {}
            (Value::Null, Some(value)) => {
                return Err(format!(
                    "Expected {} query param {name:?} to be absent, got {value:?}",
                    which.label()
                ));
            }
            (_, None) => {
                return Err(format!(
                    "Expected {} query param {name}={}, got \"\"",
                    which.label(),
                    display(want)
                ));
            }
            (_, Some(value)) if value == display(want) => {}
            (_, Some(value)) => {
                return Err(format!(
                    "Expected {} query param {name}={}, got {value:?}",
                    which.label(),
                    display(want)
                ));
            }
        }
    }
    Ok(())
}

fn check_request_body(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let expected = expected_object(assertion, "requestBody")?;
    let raw = run.recorded.bodies.first().ok_or_else(no_requests)?;
    let body = if raw.is_empty() {
        Value::Null
    } else {
        serde_json::from_slice(raw)
            .map_err(|error| format!("requestBody: request body is not JSON: {error}"))?
    };
    for (path, want) in expected {
        match (want, lookup(&body, path)) {
            (Value::Null, None) => {}
            (Value::Null, Some(got)) => {
                return Err(format!(
                    "Expected body key {path:?} to be absent, got {}",
                    display(got)
                ));
            }
            (_, None) => {
                return Err(format!(
                    "Expected body key {path:?} = {}, but it is absent",
                    display(want)
                ));
            }
            (_, Some(got)) if values_match(want, got) => {}
            (_, Some(got)) => {
                return Err(format!(
                    "Expected body key {path:?} = {}, got {}",
                    display(want),
                    display(got)
                ));
            }
        }
    }
    Ok(())
}

fn check_request_form(run: &Run, assertion: &Assertion, which: Which) -> Result<(), String> {
    let expected = expected_object(assertion, &assertion.kind)?;
    let raw = which.pick(&run.recorded.bodies).ok_or_else(no_requests)?;
    let fields: Vec<(String, String)> = url::form_urlencoded::parse(raw)
        .map(|(name, value)| (name.into_owned(), value.into_owned()))
        .collect();
    for (name, want) in expected {
        let got = fields
            .iter()
            .find(|(field, _)| field == name)
            .map(|(_, value)| value.as_str());
        match (want, got) {
            (Value::Null, None) => {}
            (Value::Null, Some(value)) => {
                return Err(format!(
                    "{}: expected form field {name:?} to be absent, got {value:?}",
                    assertion.kind
                ));
            }
            (_, None) => {
                return Err(format!(
                    "{}: expected form field {name:?} = {}, but it is absent",
                    assertion.kind,
                    display(want)
                ));
            }
            (_, Some(value)) if value == display(want) => {}
            (_, Some(value)) => {
                return Err(format!(
                    "{}: expected form field {name:?} = {}, got {value:?}",
                    assertion.kind,
                    display(want)
                ));
            }
        }
    }
    Ok(())
}

fn check_header_present(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let name = &assertion.path;
    let headers = run.recorded.headers.first().ok_or_else(|| {
        format!("Expected request with header {name:?}, but no requests were recorded")
    })?;
    if header(headers, name).is_empty() {
        Err(format!(
            "Expected header {name:?} to be present, but it was not"
        ))
    } else {
        Ok(())
    }
}

fn check_last_request_header(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let name = &assertion.path;
    let expected = expected_string(assertion, "lastRequestHeader")?;
    let headers = run.recorded.headers.last().ok_or_else(|| {
        format!("Expected request with header {name:?}, but no requests were recorded")
    })?;
    let actual = header(headers, name);
    if actual == expected {
        Ok(())
    } else {
        Err(format!(
            "Expected last request header {name:?} = {expected:?}, got {actual:?}"
        ))
    }
}

fn header<'a>(headers: &'a axum::http::HeaderMap, name: &str) -> &'a str {
    headers
        .get(name)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
}

fn check_response_meta(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let Ok(Outcome::Page {
        next_page,
        total_count,
        ..
    }) = run.outcome
    else {
        return Err(format!(
            "Expected a paginated result to read responseMeta.{} from",
            assertion.path
        ));
    };
    match assertion.path.as_str() {
        "totalCount" => {
            let expected = expected_int(assertion, "responseMeta.totalCount")?;
            match total_count {
                None => Err("X-Total-Count header not present in response".to_string()),
                Some(actual) if *actual as i64 == expected => Ok(()),
                Some(actual) => Err(format!("Expected X-Total-Count={expected}, got {actual}")),
            }
        }
        "nextPage" => {
            let expected = expected_string(assertion, "responseMeta.nextPage")?;
            match next_page {
                None => Err("Link header does not contain a valid next URL".to_string()),
                Some(actual) if actual == expected => Ok(()),
                Some(actual) => Err(format!("Expected next page {expected:?}, got {actual:?}")),
            }
        }
        path => Err(format!("Unknown responseMeta path: {path}")),
    }
}

fn check_url_origin(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let expected = expected_string(assertion, "urlOrigin")?;
    if expected != "rejected" {
        return Err(format!(
            "urlOrigin: unsupported expected value {expected:?} (only \"rejected\" is supported)"
        ));
    }
    let link = run
        .recorded
        .links
        .last()
        .cloned()
        .flatten()
        .unwrap_or_default();
    if link.is_empty() {
        return Err("No Link header in response to validate origin".to_string());
    }
    let target =
        next_link(&link).ok_or_else(|| format!("No next URL found in Link header: {link}"))?;
    let server =
        Url::parse(run.base_url).map_err(|error| format!("Failed to parse server URL: {error}"))?;
    match Url::parse(&target) {
        Err(_) => Err(format!(
            "Expected cross-origin Link URL for rejection test, but got relative URL: {target}"
        )),
        Ok(next) if same_origin(&next, &server) => Err(format!(
            "Expected cross-origin Link URL for rejection test, but {target} has same origin as server"
        )),
        Ok(_) => check_next_page_refused(run),
    }
}

fn same_origin(a: &Url, b: &Url) -> bool {
    a.scheme().eq_ignore_ascii_case(b.scheme())
        && a.host_str()
            .unwrap_or_default()
            .eq_ignore_ascii_case(b.host_str().unwrap_or_default())
        && a.port_or_known_default() == b.port_or_known_default()
}

fn check_next_page_refused(run: &Run) -> Result<(), String> {
    match run.outcome {
        Err(error) => Err(format!(
            "Expected a paginated result to read the next page from, got: {error}"
        )),
        Ok(Outcome::Page {
            next_url_check: Some(Err(error)),
            ..
        }) if error.code() == ErrorCode::Usage => Ok(()),
        Ok(Outcome::Page {
            next_url_check: Some(Err(error)),
            ..
        }) => Err(format!(
            "Expected the cross-origin next page to be refused as a usage error, got {}",
            error.code()
        )),
        Ok(Outcome::Page {
            next_url_check: Some(Ok(())),
            ..
        }) => Err(
            "Expected the cross-origin next page to be refused, but the SDK followed it"
                .to_string(),
        ),
        Ok(_) => Err("Expected a paginated result to read the next page from".to_string()),
    }
}

fn check_response_body(run: &Run, assertion: &Assertion) -> Result<(), String> {
    let path = &assertion.path;
    let body = match run.outcome {
        Err(error) => return Err(format!("Expected responseBody.{path}, got: {error}")),
        Ok(outcome) => outcome.body().ok_or_else(|| {
            format!("Expected responseBody.{path}, but no response body captured")
        })?,
    };
    let actual = lookup(body, path)
        .ok_or_else(|| format!("Expected responseBody.{path}, but field not present"))?;
    if values_match(&assertion.expected, actual) {
        Ok(())
    } else {
        Err(format!(
            "Expected responseBody.{path} = {}, got {}",
            display(&assertion.expected),
            display(actual)
        ))
    }
}

/// Which recorded request an assertion reads: the first or the last.
#[derive(Clone, Copy)]
enum Which {
    First,
    Last,
}

use Which::{First, Last};

impl Which {
    fn pick<'a, T>(&self, values: &'a [T]) -> Option<&'a T> {
        match self {
            First => values.first(),
            Last => values.last(),
        }
    }

    fn label(&self) -> &'static str {
        match self {
            First => "first",
            Last => "last",
        }
    }
}

fn no_requests() -> String {
    "Expected a request, but none were recorded".to_string()
}

fn expected_int(assertion: &Assertion, label: &str) -> Result<i64, String> {
    assertion.expected.as_i64().ok_or_else(|| {
        format!(
            "{label}: expected an integer, got {}",
            display(&assertion.expected)
        )
    })
}

fn expected_bool(assertion: &Assertion, label: &str) -> Result<bool, String> {
    assertion.expected.as_bool().ok_or_else(|| {
        format!(
            "{label}: expected a bool, got {}",
            display(&assertion.expected)
        )
    })
}

fn expected_string<'a>(assertion: &'a Assertion, label: &str) -> Result<&'a str, String> {
    assertion.expected.as_str().ok_or_else(|| {
        format!(
            "{label}: expected a string, got {}",
            display(&assertion.expected)
        )
    })
}

fn expected_object<'a>(
    assertion: &'a Assertion,
    label: &str,
) -> Result<&'a Map<String, Value>, String> {
    assertion.expected.as_object().ok_or_else(|| {
        format!(
            "{label}: expected an object, got {}",
            display(&assertion.expected)
        )
    })
}

/// Walks a decoded JSON value by a dot-separated path, reading integer segments as array
/// indexes.
fn lookup<'a>(value: &'a Value, path: &str) -> Option<&'a Value> {
    let mut current = value;
    for segment in path.split('.') {
        current = match current {
            Value::Object(fields) => fields.get(segment)?,
            Value::Array(items) => items.get(segment.parse::<usize>().ok()?)?,
            _ => return None,
        };
    }
    Some(current)
}

fn values_match(expected: &Value, actual: &Value) -> bool {
    if let (Some(expected), Some(actual)) = (expected.as_i64(), actual.as_i64()) {
        expected == actual
    } else if let (Some(expected), Some(actual)) = (expected.as_f64(), actual.as_f64()) {
        expected == actual
    } else if let (Some(expected), Some(actual)) = (expected.as_bool(), actual.as_bool()) {
        expected == actual
    } else {
        display(expected) == display(actual)
    }
}

fn display(value: &Value) -> String {
    match value {
        Value::String(text) => text.clone(),
        other => other.to_string(),
    }
}
