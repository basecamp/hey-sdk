use std::fmt::Display;

use percent_encoding::{AsciiSet, NON_ALPHANUMERIC, utf8_percent_encode};
use reqwest::Method;

/// One API operation: its method, its path template and the behaviour the Smithy model
/// attaches to it. Every route the SDK knows lives in [`crate::routes`].
#[derive(Debug)]
pub struct Route {
    pub id: &'static str,
    /// The service handle whose method sends this route, as every HEY SDK names it:
    /// `Boxes`, `TimeTracks`.
    pub service: &'static str,
    pub method: Method,
    /// The path as HEY serves it, `{param}` placeholders included.
    pub path: &'static str,
    /// The path without a `.json` suffix, for recognizing pasted URLs.
    pub pattern: &'static str,
    pub resource: &'static str,
    /// The kind of record the route acts on, snake_cased: `box`, `box_group`.
    pub resource_type: &'static str,
    pub params: &'static [RouteParam],
    pub idempotent: bool,
    /// The route only reads; nothing it does changes anything.
    pub readonly: bool,
    pub empty_on: &'static [u16],
    pub pagination: Pagination,
    pub retry: Retry,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct RouteParam {
    pub name: &'static str,
    pub role: ParamRole,
    pub kind: ParamKind,
}

/// Where a path parameter sits: the last segment names the record itself, anything
/// before it names a parent.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ParamRole {
    Parent,
    Recording,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ParamKind {
    String,
    Bool,
    Int32,
    Int64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Pagination {
    None,
    Link,
    Window,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Retry {
    pub max: u32,
    pub base_delay_ms: u64,
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
