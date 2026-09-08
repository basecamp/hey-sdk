use std::collections::BTreeMap;

use hey_sdk::DateTime;
use serde::Deserialize;
use serde_json::{Map, Value};

pub type Params = Map<String, Value>;

/// One conformance case, as `conformance/tests/*.json` writes it. Keys the runner does
/// not read are ignored.
#[derive(Debug, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct TestCase {
    pub name: String,
    pub description: String,
    pub operation: String,
    pub method: String,
    pub path: String,
    pub path_params: Params,
    pub query_params: Params,
    pub request_body: Params,
    pub mock_responses: Vec<MockResponse>,
    pub assertions: Vec<Assertion>,
    pub tags: Vec<String>,
    pub config_overrides: ConfigOverrides,
    /// Invokes the operation this many times against one client, for behavior that only
    /// shows across calls — a cached read revalidating, say. Zero means once.
    pub repeat_operation: u32,
}

#[derive(Debug, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ConfigOverrides {
    pub base_url: Option<String>,
    pub client_layer: Option<String>,
    pub cache_enabled: bool,
    pub refreshable_credentials: bool,
    pub account_id: Option<i64>,
}

#[derive(Debug, Default, Clone, Deserialize)]
#[serde(default)]
pub struct MockResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Option<Value>,
    pub delay: u64,
}

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub struct Assertion {
    #[serde(rename = "type")]
    pub kind: String,
    pub expected: Value,
    pub min: f64,
    pub max: f64,
    pub path: String,
}

impl TestCase {
    pub fn is_hey_layer(&self) -> bool {
        self.config_overrides.client_layer.as_deref() == Some("hey")
    }

    pub fn runs(&self) -> u32 {
        self.repeat_operation.max(1)
    }

    /// Whether the case looks at what the SDK does with the `Link` header. Following it
    /// otherwise would ask the mock server for a response it does not have.
    pub fn follows_next_page(&self) -> bool {
        self.assertions
            .iter()
            .any(|assertion| assertion.kind == "urlOrigin")
    }
}

pub fn int64_param(params: &Params, key: &str) -> i64 {
    params.get(key).and_then(Value::as_i64).unwrap_or_default()
}

pub fn int32_param(params: &Params, key: &str) -> i32 {
    i32::try_from(int64_param(params, key)).unwrap_or_default()
}

pub fn int64_list_param(params: &Params, key: &str) -> Vec<i64> {
    value_list(params, key)
        .iter()
        .filter_map(Value::as_i64)
        .collect()
}

pub fn int32_list_param(params: &Params, key: &str) -> Option<Vec<i32>> {
    let values = params.get(key)?.as_array()?;
    Some(
        values
            .iter()
            .filter_map(Value::as_i64)
            .filter_map(|value| i32::try_from(value).ok())
            .collect(),
    )
}

pub fn string_param(params: &Params, key: &str) -> String {
    params
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

pub fn string_list_param(params: &Params, key: &str) -> Vec<String> {
    value_list(params, key)
        .iter()
        .filter_map(Value::as_str)
        .map(str::to_string)
        .collect()
}

pub fn optional_string_list_param(params: &Params, key: &str) -> Option<Vec<String>> {
    let values = params.get(key)?.as_array()?;
    Some(
        values
            .iter()
            .filter_map(Value::as_str)
            .map(str::to_string)
            .collect(),
    )
}

/// The value when the key carries a string, as the Go runner's `getStringPtrParam` reads it.
pub fn string_ptr_param(params: &Params, key: &str) -> Option<String> {
    params.get(key).and_then(Value::as_str).map(str::to_string)
}

pub fn bool_ptr_param(params: &Params, key: &str) -> Option<bool> {
    params.get(key).and_then(Value::as_bool)
}

/// The value when it is a non-empty string, for the wire fields an empty string omits.
pub fn non_empty_string_param(params: &Params, key: &str) -> Option<String> {
    string_ptr_param(params, key).filter(|value| !value.is_empty())
}

/// The value when the key is there at all, for the optional parameters a case sends by
/// naming them and omits by leaving them out.
pub fn gated_string_param(params: &Params, key: &str) -> Option<String> {
    if params.contains_key(key) {
        Some(string_param(params, key))
    } else {
        None
    }
}

pub fn gated_int64_param(params: &Params, key: &str) -> Option<i64> {
    if params.contains_key(key) {
        Some(int64_param(params, key))
    } else {
        None
    }
}

pub fn gated_int32_param(params: &Params, key: &str) -> Option<i32> {
    if params.contains_key(key) {
        Some(int32_param(params, key))
    } else {
        None
    }
}

pub fn date_time_param(params: &Params, key: &str) -> DateTime {
    serde_json::from_value(Value::String(string_param(params, key))).unwrap_or_default()
}

fn value_list<'a>(params: &'a Params, key: &str) -> &'a [Value] {
    params
        .get(key)
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or_default()
}
