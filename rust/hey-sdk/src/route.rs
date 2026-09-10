use std::fmt::Display;

use crate::http::Method;
use percent_encoding::{AsciiSet, NON_ALPHANUMERIC, utf8_percent_encode};

/// One API operation: its method, its path template and the behaviour the Smithy model
/// attaches to it. Every route the SDK knows lives in [`crate::routes`].
#[derive(Debug)]
pub struct Route {
    /// The operation as the model names it: `ListBoxes`, `GetTopic`.
    pub id: &'static str,
    /// The service handle whose method sends this route, as every HEY SDK names it:
    /// `Boxes`, `TimeTracks`.
    pub service: &'static str,
    /// The HTTP method the route is sent with.
    pub method: Method,
    /// The path as HEY serves it, `{param}` placeholders included.
    pub path: &'static str,
    /// The path without a `.json` suffix, for recognizing pasted URLs.
    pub pattern: &'static str,
    /// The part of HEY the route belongs to, as the model titles it: `Boxes`, `Calendar Time
    /// Tracks`.
    pub resource: &'static str,
    /// The kind of record the route acts on, in `snake_case`: `box`, `box_group`.
    pub resource_type: &'static str,
    /// The path parameters, in the order they appear in [`Route::path`].
    pub params: &'static [RouteParam],
    /// The route may be sent again after a failure without doing its work twice.
    pub idempotent: bool,
    /// The route only reads; nothing it does changes anything.
    pub readonly: bool,
    /// The route answers a page as HTML rather than a JSON document, so it is asked for as
    /// written — no `.json` suffix — with `Accept: text/html`.
    pub html: bool,
    /// The statuses that mean HEY has nothing for this route rather than that it failed — a
    /// 404 for a record that may simply not be there. Such an answer is empty, not an error.
    pub empty_on: &'static [u16],
    /// How the route pages, when it does.
    pub pagination: Pagination,
    /// The retry policy the model attaches to the route.
    pub retry: Retry,
}

/// One `{param}` placeholder in a route's path.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct RouteParam {
    /// The placeholder's name as it appears in the path: `boxId`.
    pub name: &'static str,
    /// Whether the parameter names the record itself or a parent of it.
    pub role: ParamRole,
    /// The type the model gives the value.
    pub kind: ParamKind,
}

/// Where a path parameter sits: the last segment names the record itself, anything
/// before it names a parent.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ParamRole {
    /// Names a record the one the route acts on belongs to: the box a group is in.
    Parent,
    /// Names the record the route acts on.
    Recording,
}

/// The type the model gives a path parameter's value.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ParamKind {
    /// Any text, such as a slug or a token.
    String,
    /// `true` or `false`.
    Bool,
    /// A 32-bit integer.
    Int32,
    /// A 64-bit integer, which is what a HEY record id is.
    Int64,
}

/// How a route pages its answer.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Pagination {
    /// The whole answer comes at once.
    None,
    /// HEY names the next page in a `Link` header; see [`crate::Page`].
    Link,
    /// The read covers a window of dates, and the caller moves the window to read on.
    Window,
}

/// The retry policy the model attaches to a route: how many attempts, how long the first
/// wait is, and which statuses are worth another try.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Retry {
    /// The most attempts the route is given, the first one included.
    pub max: u32,
    /// The wait before the second attempt, in milliseconds; later waits grow from it.
    pub base_delay_ms: u64,
    /// The statuses that are worth another attempt.
    pub retry_on: &'static [u16],
}

const PATH_SEGMENT: &AsciiSet = &NON_ALPHANUMERIC
    .remove(b'-')
    .remove(b'_')
    .remove(b'.')
    .remove(b'~');

impl Route {
    /// Substitutes the path parameters, in order, percent-encoding each value.
    ///
    /// # Panics
    ///
    /// When `values` is not exactly as long as [`Route::params`]. A short list would leave
    /// a `{param}` in the path and send it to HEY as written, which is worse than stopping;
    /// every generated caller passes the right count, so reaching this means the call was
    /// built by hand and built wrong.
    pub fn fill(&self, values: &[&dyn Display]) -> String {
        assert_eq!(
            values.len(),
            self.params.len(),
            "{} takes {} path parameters",
            self.id,
            self.params.len()
        );
        let mut path = self.path.to_string();
        for (param, value) in self.params.iter().zip(values) {
            let encoded = utf8_percent_encode(&value.to_string(), PATH_SEGMENT).to_string();
            path = path.replace(&format!("{{{}}}", param.name), &encoded);
        }
        path
    }

    /// Matches a path against the route's pattern and answers the captured parameters.
    pub fn recognize(&self, path: &str) -> Option<Vec<(&'static str, String)>> {
        let pattern_segments: Vec<&str> = self.pattern.split('/').collect();
        let path_segments: Vec<&str> = path.split('/').collect();
        if pattern_segments.len() != path_segments.len() {
            return None;
        }
        let mut params = Vec::new();
        for (pattern, actual) in pattern_segments.iter().zip(&path_segments) {
            if let Some(name) = pattern
                .strip_prefix('{')
                .and_then(|rest| rest.strip_suffix('}'))
            {
                if actual.is_empty() {
                    return None;
                }
                let param = self.params.iter().find(|param| param.name == name)?;
                params.push((param.name, actual.to_string()));
            } else if pattern != actual {
                return None;
            }
        }
        Some(params)
    }
}
