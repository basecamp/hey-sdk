mod support;

use std::sync::Mutex;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use hey_sdk::models::{
    ContactPayload, CreateContactRequestContent, CreateMessageRequestContent, MessagePayload,
};
use hey_sdk::services::clearances::GetClearancesParams;
use hey_sdk::services::search::AdvancedSearchParams;
use hey_sdk::version::default_user_agent;
use hey_sdk::{
    API_VERSION, Client, Config, Error, ErrorCode, StaticTokenProvider, TokenProvider, VERSION,
};
use reqwest::Method;
use serde_json::{Value, json};
use wiremock::matchers::{header, method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{TOKEN, builder, client};

#[tokio::test]
async fn every_request_carries_the_token_and_names_the_sdk() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;

    client(&server).boxes().list().await.unwrap();

    assert_eq!(
        default_user_agent(),
        format!("hey-sdk-rust/{VERSION} (api:{API_VERSION})")
    );
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(
        requests[0].headers["authorization"],
        format!("Bearer {TOKEN}")
    );
    assert_eq!(requests[0].headers["accept"], "application/json");
    assert_eq!(requests[0].headers["user-agent"], default_user_agent());
    assert!(!requests[0].headers.contains_key("content-type"));
}

#[tokio::test]
async fn a_json_body_goes_out_as_json() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/messages.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    client(&server).messages().create(&message()).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].headers["content-type"], "application/json");
    assert_eq!(
        requests[0].body_json::<Value>().unwrap(),
        json!({
            "acting_sender_id": 314,
            "message": { "subject": "Quarterly planning", "content": "<div>Agenda.</div>" }
        })
    );
}

#[tokio::test]
async fn every_path_is_read_as_json() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/123.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 123, "kind": "imbox", "name": "Imbox" })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/calendar/days/2026-03-04/journal_entry.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 55, "type": "Calendar::JournalEntry" })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/custom.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .mount(&server)
        .await;

    let client = client(&server);
    assert_eq!(
        client
            .boxes()
            .get(123, &Default::default())
            .await
            .unwrap()
            .name,
        "Imbox"
    );
    assert!(client.boxes().list().await.unwrap().is_empty());
    assert_eq!(
        client.journal().get_entry("2026-03-04").await.unwrap().id,
        55
    );
    client
        .send_unit(client.request(Method::GET, "/custom"))
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    let paths: Vec<&str> = requests.iter().map(|request| request.url.path()).collect();
    assert_eq!(
        paths,
        [
            "/boxes/123.json",
            "/boxes.json",
            "/calendar/days/2026-03-04/journal_entry.json",
            "/custom.json"
        ]
    );
}

#[tokio::test]
async fn optional_query_parameters_are_left_out() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/time_tracks.json"))
        .and(query_param_is_missing("page"))
        .and(query_param_is_missing("category_id"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "time_tracks": [{ "id": 701, "type": "Calendar::TimeTrack" }],
            "categories": [{ "id": 31, "title": "Client work" }]
        })))
        .mount(&server)
        .await;

    let tracked = client(&server)
        .time_tracks()
        .list(&Default::default())
        .await
        .unwrap();

    assert_eq!(tracked.time_tracks.as_ref().unwrap()[0].id, 701);
    assert_eq!(
        tracked.categories.as_ref().unwrap()[0].title.as_deref(),
        Some("Client work")
    );
    assert_eq!(
        server.received_requests().await.unwrap()[0].url.query(),
        None
    );
}

#[tokio::test]
async fn query_parameters_reach_the_server_the_way_hey_reads_them() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/postings/mutings.json"))
        .and(query_param("posting_ids", "1,2"))
        .respond_with(ResponseTemplate::new(201))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/clearances.json"))
        .and(query_param("include_clearances", "true"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "pending_clearances_count": 3 })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/advanced_search.json"))
        .and(query_param("refine[from]", "jane@example.com"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "matches": [] })))
        .mount(&server)
        .await;

    let client = client(&server);
    client.postings().unmute("1,2").await.unwrap();
    let screener = client
        .clearances()
        .get(&GetClearancesParams {
            include_clearances: Some(true),
            ..Default::default()
        })
        .await
        .unwrap();
    let found = client
        .search()
        .advanced(&AdvancedSearchParams {
            refine_from: Some("jane@example.com".to_string()),
            ..Default::default()
        })
        .await
        .unwrap();

    assert_eq!(screener.pending_clearances_count, Some(3));
    assert!(found.matches.is_empty());
    let queries: Vec<String> = server
        .received_requests()
        .await
        .unwrap()
        .iter()
        .map(|request| {
            request
                .url
                .query_pairs()
                .map(|(name, value)| format!("{name}={value}"))
                .collect()
        })
        .collect();
    assert_eq!(
        queries,
        [
            "posting_ids=1,2",
            "include_clearances=true",
            "refine[from]=jane@example.com"
        ]
    );
}

#[tokio::test]
async fn a_read_is_resent_until_the_server_answers() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(503))
        .up_to_n_times(2)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let boxes = client(&server).boxes().list().await.unwrap();

    assert_eq!(boxes.len(), 1);
    assert_eq!(boxes[0].name, "Imbox");
    assert_eq!(server.received_requests().await.unwrap().len(), 3);
}

#[tokio::test]
async fn a_rate_limited_read_waits_as_long_as_the_server_asks() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(429).insert_header("Retry-After", "1"))
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;

    let started = Instant::now();
    client(&server).boxes().list().await.unwrap();
    let waited = started.elapsed();

    assert_eq!(server.received_requests().await.unwrap().len(), 2);
    assert!(
        waited >= Duration::from_secs(1),
        "resent after only {waited:?}"
    );
}

#[tokio::test]
async fn a_mutation_is_never_resent() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/messages.json"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;

    let error = client(&server)
        .messages()
        .create(&message())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Api);
    assert_eq!(error.http_status(), Some(503));
    assert!(error.is_retryable());
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn the_model_decides_what_is_resent_not_the_method() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/calendar/todos/12345/completions.json"))
        .respond_with(ResponseTemplate::new(503))
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("POST"))
        .and(path("/calendar/todos/12345/completions.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 12345, "type": "Calendar::Todo" })),
        )
        .mount(&server)
        .await;
    Mock::given(method("PUT"))
        .and(path("/messages/456.json"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;

    let client = client(&server);
    let completion = client.calendar_todos().complete(12345).await.unwrap();
    let error = client.messages().update(456, &message()).await.unwrap_err();

    assert_eq!(completion.id, 12345);
    assert_eq!(error.http_status(), Some(503));
    let requests = server.received_requests().await.unwrap();
    let paths: Vec<&str> = requests.iter().map(|request| request.url.path()).collect();
    assert_eq!(
        paths,
        [
            "/calendar/todos/12345/completions.json",
            "/calendar/todos/12345/completions.json",
            "/messages/456.json"
        ]
    );
}

#[tokio::test]
async fn an_answer_that_will_not_change_is_surfaced_at_once() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/99999.json"))
        .respond_with(ResponseTemplate::new(404))
        .mount(&server)
        .await;

    let error = client(&server)
        .boxes()
        .get(99999, &Default::default())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
    assert_eq!(error.http_status(), Some(404));
    assert!(!error.is_retryable());
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn resending_stops_at_the_retry_limit() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(429).insert_header("Retry-After", "0"))
        .mount(&server)
        .await;

    let error = builder(&server)
        .max_retries(3)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::RateLimit);
    assert_eq!(error.http_status(), Some(429));
    assert!(error.is_retryable());
    assert_eq!(server.received_requests().await.unwrap().len(), 4);
}

#[tokio::test]
async fn a_refreshed_credential_is_sent_once_more() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("authorization", "Bearer stale-token"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("authorization", "Bearer fresh-token"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;

    let client = builder(&server)
        .token_provider(RefreshingToken::new())
        .build()
        .unwrap();
    assert!(client.boxes().list().await.unwrap().is_empty());

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert_eq!(requests[0].headers["authorization"], "Bearer stale-token");
    assert_eq!(requests[1].headers["authorization"], "Bearer fresh-token");
}

#[tokio::test]
async fn a_401_that_outlives_the_refresh_is_surfaced() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;

    let client = builder(&server)
        .token_provider(RefreshingToken::new())
        .build()
        .unwrap();
    let error = client.boxes().list().await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Auth);
    assert_eq!(error.http_status(), Some(401));
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_401_stands_when_nothing_can_refresh_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;

    let error = client(&server).boxes().list().await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Auth);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn a_refreshed_mutation_resends_the_same_body() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/contacts.json"))
        .and(header("authorization", "Bearer stale-token"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&server)
        .await;
    Mock::given(method("POST"))
        .and(path("/contacts.json"))
        .and(header("authorization", "Bearer fresh-token"))
        .respond_with(
            ResponseTemplate::new(201).set_body_json(json!({ "id": 91824, "name": "Jane Dawson" })),
        )
        .mount(&server)
        .await;

    let body = CreateContactRequestContent {
        acting_user_id: None,
        contact: ContactPayload {
            name: "Jane Dawson".to_string(),
            email_address: Some("jane@example.com".into()),
            alias_email_addresses: None,
        },
    };
    let client = builder(&server)
        .token_provider(RefreshingToken::new())
        .build()
        .unwrap();
    let contact = client.contacts().create(&body).await.unwrap();

    assert_eq!(contact.id, 91824);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert_eq!(requests[0].body, requests[1].body);
    assert_eq!(
        requests[1].body_json::<Value>().unwrap(),
        json!({ "contact": { "name": "Jane Dawson", "email_address": "jane@example.com" } })
    );
}

#[tokio::test]
async fn http_statuses_map_onto_the_sdk_error_vocabulary() {
    let server = MockServer::start().await;
    for status in [401, 404, 409, 422, 429, 500] {
        Mock::given(method("GET"))
            .and(path(format!("/status/{status}.json")))
            .respond_with(ResponseTemplate::new(status))
            .mount(&server)
            .await;
    }
    let client = builder(&server).max_retries(0).build().unwrap();

    let unauthorized = failure(&client, 401).await;
    assert_eq!(unauthorized.code(), ErrorCode::Auth);
    assert_eq!(unauthorized.http_status(), Some(401));
    assert!(!unauthorized.is_retryable());

    let missing = failure(&client, 404).await;
    assert_eq!(missing.code(), ErrorCode::NotFound);
    assert_eq!(missing.http_status(), Some(404));
    assert!(!missing.is_retryable());

    let conflicted = failure(&client, 409).await;
    assert_eq!(conflicted.code(), ErrorCode::Api);
    assert_eq!(conflicted.http_status(), Some(409));
    assert!(!conflicted.is_retryable());

    let invalid = failure(&client, 422).await;
    assert_eq!(invalid.code(), ErrorCode::Validation);
    assert_eq!(invalid.http_status(), Some(422));
    assert!(!invalid.is_retryable());

    let limited = failure(&client, 429).await;
    assert_eq!(limited.code(), ErrorCode::RateLimit);
    assert_eq!(limited.http_status(), Some(429));
    assert!(limited.is_retryable());

    let broken = failure(&client, 500).await;
    assert_eq!(broken.code(), ErrorCode::Api);
    assert_eq!(broken.http_status(), Some(500));
    assert!(broken.is_retryable());
}

#[tokio::test]
async fn a_forbidden_read_and_a_forbidden_write_give_different_advice() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(403))
        .mount(&server)
        .await;
    Mock::given(method("POST"))
        .and(path("/messages.json"))
        .respond_with(ResponseTemplate::new(403))
        .mount(&server)
        .await;

    let client = client(&server);
    let read = client.boxes().list().await.unwrap_err();
    let write = client.messages().create(&message()).await.unwrap_err();

    assert_eq!(read.code(), ErrorCode::Forbidden);
    assert_eq!(read.message(), "access denied");
    assert_eq!(read.hint(), None);
    assert_eq!(write.code(), ErrorCode::Forbidden);
    assert_eq!(write.message(), "Access denied: insufficient scope");
    assert_eq!(write.hint(), Some("Re-authenticate with full scope"));
}

#[tokio::test]
async fn an_error_carries_the_request_id_and_what_the_server_said() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/99999.json"))
        .respond_with(
            ResponseTemplate::new(404)
                .insert_header("X-Request-Id", "req-unique-abc")
                .set_body_json(json!({ "message": "No such box" })),
        )
        .mount(&server)
        .await;

    let error = client(&server)
        .boxes()
        .get(99999, &Default::default())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
    assert_eq!(error.request_id(), Some("req-unique-abc"));
    assert_eq!(error.hint(), Some("No such box"));
    assert_eq!(error.to_string(), "resource not found: No such box");
}

#[tokio::test]
async fn an_ongoing_time_track_that_is_not_there_is_not_an_error() {
    let idle = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/ongoing_time_track.json"))
        .respond_with(ResponseTemplate::new(404).set_body_json(json!({ "error": "Not found" })))
        .mount(&idle)
        .await;
    let running = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/ongoing_time_track.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 123, "type": "Calendar::TimeTrack" })),
        )
        .mount(&running)
        .await;

    assert_eq!(
        client(&idle).time_tracks().get_ongoing().await.unwrap(),
        None
    );
    assert_eq!(
        client(&running)
            .time_tracks()
            .get_ongoing()
            .await
            .unwrap()
            .unwrap()
            .id,
        123
    );
    assert_eq!(idle.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn only_a_404_means_nothing_is_ongoing() {
    let unauthorized = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/ongoing_time_track.json"))
        .respond_with(ResponseTemplate::new(401))
        .mount(&unauthorized)
        .await;
    let broken = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/ongoing_time_track.json"))
        .respond_with(ResponseTemplate::new(500))
        .mount(&broken)
        .await;

    let denied = client(&unauthorized)
        .time_tracks()
        .get_ongoing()
        .await
        .unwrap_err();
    let failed = builder(&broken)
        .max_retries(0)
        .build()
        .unwrap()
        .time_tracks()
        .get_ongoing()
        .await
        .unwrap_err();

    assert_eq!(denied.code(), ErrorCode::Auth);
    assert_eq!(denied.http_status(), Some(401));
    assert_eq!(failed.code(), ErrorCode::Api);
    assert_eq!(failed.http_status(), Some(500));
}

#[tokio::test]
async fn a_response_body_over_the_limit_is_refused() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_string("x".repeat(200)))
        .mount(&server)
        .await;

    let error = builder(&server)
        .max_response_body_bytes(64)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Api);
    assert_eq!(
        error.message(),
        "GET /boxes.json: response body exceeds 64 bytes"
    );
}

#[tokio::test]
async fn an_id_beyond_double_precision_survives_the_round_trip() {
    let server = MockServer::start().await;
    let body = r#"{"id":9007199254740993,"kind":"imbox","name":"Large ID Box"}"#;
    Mock::given(method("GET"))
        .and(path("/boxes/9007199254740993.json"))
        .respond_with(ResponseTemplate::new(200).set_body_string(body))
        .mount(&server)
        .await;

    let mailbox = client(&server)
        .boxes()
        .get(9007199254740993, &Default::default())
        .await
        .unwrap();

    assert_eq!(mailbox.id, 9007199254740993);
    assert_eq!(serde_json::to_string(mailbox.value()).unwrap(), body);
}

#[test]
fn the_builder_refuses_a_client_it_could_not_use_safely() {
    let insecure = Client::builder(Config::default().with_base_url("http://evil.example.com"))
        .token_provider(StaticTokenProvider::new(TOKEN))
        .build()
        .err()
        .unwrap();
    assert_eq!(insecure.code(), ErrorCode::Usage);
    assert_eq!(
        insecure.message(),
        "http://evil.example.com/ must use HTTPS"
    );

    let local = Client::builder(Config::default().with_base_url("http://127.0.0.1:1"))
        .token_provider(StaticTokenProvider::new(TOKEN))
        .build()
        .unwrap();
    assert_eq!(local.base_url().as_str(), "http://127.0.0.1:1/");

    let anonymous = Client::builder(Config::default()).build().err().unwrap();
    assert_eq!(anonymous.code(), ErrorCode::Usage);
    assert_eq!(
        anonymous.message(),
        "a token provider or auth strategy is required"
    );
}

fn message() -> CreateMessageRequestContent {
    CreateMessageRequestContent {
        acting_sender_id: 314,
        message: MessagePayload {
            subject: "Quarterly planning".to_string(),
            content: "<div>Agenda.</div>".to_string(),
        },
        entry: None,
    }
}

async fn failure(client: &Client, status: u16) -> Error {
    client
        .send_unit(client.request(Method::GET, format!("/status/{status}")))
        .await
        .unwrap_err()
}

/// A provider whose credentials can be renewed: the first 401 swaps the stale token for a
/// fresh one, which is what the resent request goes out with.
struct RefreshingToken {
    token: Mutex<String>,
}

impl RefreshingToken {
    fn new() -> RefreshingToken {
        RefreshingToken {
            token: Mutex::new("stale-token".to_string()),
        }
    }
}

#[async_trait]
impl TokenProvider for RefreshingToken {
    async fn access_token(&self) -> Result<String, Error> {
        Ok(self.token.lock().unwrap().clone())
    }

    async fn refresh(&self) -> bool {
        *self.token.lock().unwrap() = "fresh-token".to_string();
        true
    }
}
