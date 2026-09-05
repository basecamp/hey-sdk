mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::BoxKind;
use serde_json::{Value, json};
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_scoped_client_checks_the_account_and_filters_what_it_reads() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(identity()))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("filtered_account_id", "42"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let scoped = client(&server).for_account(42).await.unwrap();
    let boxes = scoped.boxes().list().await.unwrap();

    assert_eq!(scoped.account_id(), Some(42));
    assert_eq!(boxes[0].name, "Imbox");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert_eq!(requests[0].url.path(), "/identity.json");
    assert_eq!(requests[1].url.path(), "/boxes.json");
    assert_eq!(requests[1].url.query(), Some("filtered_account_id=42"));
}

#[tokio::test]
async fn an_account_the_identity_cannot_reach_is_not_found() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 1,
            "accounts": [
                { "id": 42, "status": "active" },
                { "id": 7, "status": "inactive", "purpose": "personal" }
            ]
        })))
        .mount(&server)
        .await;

    let client = client(&server);
    let unknown = client.for_account(99).await.err().unwrap();
    let inactive = client.for_account(7).await.err().unwrap();

    assert_eq!(unknown.code(), ErrorCode::NotFound);
    assert_eq!(unknown.message(), "accessible account not found: 99");
    assert_eq!(inactive.code(), ErrorCode::NotFound);
    assert_eq!(inactive.http_status(), Some(404));
}

#[tokio::test]
async fn the_scoped_default_sender_comes_from_the_identity_already_read() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(identity()))
        .mount(&server)
        .await;

    let scoped = client(&server).for_account(42).await.unwrap();

    assert_eq!(scoped.default_sender_id().await.unwrap(), 314);
    assert_eq!(scoped.default_sender_id().await.unwrap(), 314);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn the_identitys_own_default_sender_is_read_once_and_remembered() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 1,
            "senders": [{ "id": 1, "default": false }, { "id": 2, "default": true }]
        })))
        .mount(&server)
        .await;

    let client = client(&server);

    assert_eq!(client.default_sender_id().await.unwrap(), 2);
    assert_eq!(client.default_sender_id().await.unwrap(), 2);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// Box ids belong to the account they were read under, so a derived client works the index
/// out again rather than borrowing what the client it came from resolved.
#[tokio::test]
async fn a_derived_client_resolves_box_kinds_against_its_own_account() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(identity()))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("filtered_account_id", "42"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param_is_missing("filtered_account_id"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 1, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let all_accounts = client(&server);
    let scoped = all_accounts.for_account(42).await.unwrap();

    assert_eq!(
        all_accounts
            .boxes()
            .id_by_kind(BoxKind::Imbox)
            .await
            .unwrap(),
        1
    );
    assert_eq!(scoped.boxes().id_by_kind(BoxKind::Imbox).await.unwrap(), 7);
}

fn identity() -> Value {
    json!({
        "id": 1,
        "accounts": [{ "id": 42, "status": "active" }],
        "senders": [{ "id": 314, "account_id": 42, "default": true }]
    })
}
