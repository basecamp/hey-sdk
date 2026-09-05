//! Talking to HEY's form-backed endpoints, and what they answer with. Several parts of HEY
//! have no JSON surface: a workflow is created by posting a form and reading the record back
//! out of the redirect the browser would have followed.
//!
//! [`Client::form`] builds the request such an endpoint expects and [`Client::send_form`]
//! sends it. The raw verbs — [`Client::post_form`] and its neighbours — are those two
//! together for the common shapes.

use reqwest::{Method, StatusCode};
use url::Url;

use crate::client::{Client, Response};
use crate::error::Error;
use crate::operation::Operation;

impl Client {
    /// A request to one of the endpoints HEY serves only as a browser form: the path as the
    /// caller wrote it, a browser's `Accept`, and the redirect taken for the answer rather
    /// than followed.
    ///
    /// It is not retried, whatever its method — a form post that may already have gone
    /// through is not one to repeat on a timeout or a 503. A 401 is the exception the whole
    /// client makes: credentials are refreshed and the request goes out once more, since a
    /// request HEY refused for want of a token never reached the write it would repeat.
    ///
    /// The model describes none of these paths, so say what the call means with
    /// [`Operation::info`] before sending it, or the hooks will only hear that something raw
    /// went out. [`crate::services::write_info`] builds that.
    ///
    /// A path that already is a URL is checked before it is taken: HTTPS goes anywhere,
    /// plain HTTP only back to the base URL's own host. That check is what this can fail on.
    pub fn form(&self, method: Method, path: &str) -> Result<Operation, Error> {
        let mut operation = self.raw(method, path)?;
        operation
            .form_representation()
            .capture_redirects()
            .idempotent(false);
        Ok(operation)
    }

    /// Sends a form request and reads the redirect it answered with.
    ///
    /// A failure keeps the code, hint and request id HEY answered with. Go flattens every
    /// status but 401 into a bare "Form request failed (HTTP 503)", which loses the request
    /// id support would look the call up by and tells a caller nothing it could act on; a
    /// 503 here still reads as the retryable API error it is.
    pub async fn send_form(&self, operation: Operation) -> Result<FormResponse, Error> {
        let response = self.execute(operation).await?;
        Ok(FormResponse::new(&response))
    }
}

/// The answer to a form or multipart request. A redirect is captured rather than followed,
/// so a 302 or 303 arrives here with its `Location` intact; an endpoint reached on a
/// `.json` path answers the record itself, which lands in `body` instead.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FormResponse {
    /// Where the redirect pointed, exactly as HEY wrote it — often a path rather than a
    /// whole URL.
    pub location: Option<String>,
    pub status: StatusCode,
    /// What the endpoint answered when it answered a document instead of a redirect.
    pub body: String,
}

impl FormResponse {
    pub(crate) fn new(response: &Response) -> FormResponse {
        let mut location = None;
        let mut body = String::new();
        if response.status.is_redirection() {
            location = response.header("location").map(String::from);
        } else {
            body = String::from_utf8_lossy(&response.body).into_owned();
        }
        FormResponse {
            location,
            status: response.status,
            body,
        }
    }

    /// The id of the record the redirect named: the rightmost path segment that reads as a
    /// number, so `/calendar/events/42` and `/calendar/events/42/edit` both answer 42.
    pub fn extract_id(&self) -> Result<i64, Error> {
        let location = self
            .location
            .as_deref()
            .filter(|location| !location.is_empty())
            .ok_or_else(|| Error::api(0, "no location header in response"))?;
        let path = match Url::parse(location) {
            Ok(url) => url.path().to_string(),
            Err(url::ParseError::RelativeUrlWithoutBase) => location
                .split(['?', '#'])
                .next()
                .unwrap_or_default()
                .to_string(),
            Err(error) => {
                return Err(Error::api(
                    0,
                    format!("failed to parse location URL: {error}"),
                ));
            }
        };
        path.trim_end_matches('/')
            .rsplit('/')
            .find_map(|segment| segment.parse().ok())
            .ok_or_else(|| Error::api(0, format!("no numeric ID found in location: {location}")))
    }
}
