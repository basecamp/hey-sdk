mod support;

use std::fs;

use bytes::Bytes;
use hey_sdk::cache::{CachedResponse, FileCache, InMemoryCache, ResponseCache};
use hey_sdk::models::{CreateMessageRequestContent, MessagePayload};
use serde_json::json;
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

/// The cache directory is the one hey-cli keeps its own state in, so clearing the cache
/// takes only what the cache put there.
#[test]
fn clearing_the_file_cache_leaves_the_rest_of_the_directory_alone() {
    let directory = std::env::temp_dir().join(format!("hey-sdk-cache-{}", rand::random::<u64>()));
    fs::create_dir_all(&directory).unwrap();
    fs::write(directory.join("credentials.json"), b"{}").unwrap();
    let cache = FileCache::new(&directory);
    cache.set(
        "abc",
        CachedResponse {
            etag: "\"v1\"".to_string(),
            body: Bytes::from_static(b"[]"),
        },
    );

    cache.clear();

    assert_eq!(None, cache.get("abc"));
    assert!(!directory.join("responses").exists());
    assert!(!directory.join("etags.json").exists());
    assert!(directory.join("credentials.json").exists());

    fs::remove_dir_all(&directory).unwrap();
}

#[tokio::test]
async fn a_second_read_revalidates_and_is_answered_from_the_cache() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1\""))
        .respond_with(ResponseTemplate::new(304).insert_header("ETag", "\"v1\""))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    let first = client.boxes().list().await.unwrap();
    let second = client.boxes().list().await.unwrap();

    assert_eq!(first[0].name, "Imbox");
    assert_eq!(second.len(), 1);
    assert_eq!(second[0].id, 7);
    assert_eq!(second[0].name, "Imbox");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert!(!requests[0].headers.contains_key("if-none-match"));
    assert_eq!(requests[1].headers["if-none-match"], "\"v1\"");
}

#[tokio::test]
async fn a_new_etag_replaces_what_the_cache_holds() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v2\""))
        .respond_with(ResponseTemplate::new(304).insert_header("ETag", "\"v2\""))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1\""))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v2\"")
                .set_body_json(json!([
                    { "id": 7, "kind": "imbox", "name": "Imbox" },
                    { "id": 8, "kind": "feedbox", "name": "The Feed" }
                ])),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    assert_eq!(client.boxes().list().await.unwrap().len(), 1);
    assert_eq!(client.boxes().list().await.unwrap().len(), 2);
    let third = client.boxes().list().await.unwrap();

    assert_eq!(third.len(), 2);
    assert_eq!(third[1].name, "The Feed");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 3);
    assert_eq!(requests[1].headers["if-none-match"], "\"v1\"");
    assert_eq!(requests[2].headers["if-none-match"], "\"v2\"");
}

#[tokio::test]
async fn a_client_without_a_cache_never_revalidates() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = client(&server);
    client.boxes().list().await.unwrap();
    client.boxes().list().await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert!(
        requests
            .iter()
            .all(|request| !request.headers.contains_key("if-none-match"))
    );
}

#[tokio::test]
async fn a_write_is_never_cached() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/messages.json"))
        .respond_with(ResponseTemplate::new(204).insert_header("ETag", "\"v1\""))
        .mount(&server)
        .await;

    let body = CreateMessageRequestContent {
        acting_sender_id: 314,
        message: MessagePayload {
            subject: "Quarterly planning".to_string(),
            content: "<div>Agenda.</div>".to_string(),
        },
        entry: None,
    };
    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    client.messages().create(&body).await.unwrap();
    client.messages().create(&body).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert!(
        requests
            .iter()
            .all(|request| !request.headers.contains_key("if-none-match"))
    );
}
