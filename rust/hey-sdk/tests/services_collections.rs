mod support;

use hey_sdk::ErrorCode;
use hey_sdk::models::{CollectionPayload, UpdateCollectionRequestContent};
use hey_sdk::services::collections::GetCollectionParams;
use serde_json::{Value, json};
use wiremock::matchers::{method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn the_collections_index_names_what_is_in_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/collections.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 3, "name": "Kitchen remodel" }])),
        )
        .mount(&server)
        .await;

    let collections = client(&server).collections().list().await.unwrap();

    assert_eq!(collections.len(), 1);
    assert_eq!(collections[0].name.as_deref(), Some("Kitchen remodel"));
}

#[tokio::test]
async fn a_collection_page_carries_its_threads_the_total_and_the_next_cursor() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/collections/33.json"))
        .and(query_param("page", "current-cursor"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("X-Total-Count", "42")
                .insert_header(
                    "Link",
                    r#"</collections/33.json?page=next-cursor>; rel="next""#,
                )
                .set_body_json(json!({
                    "id": 33,
                    "name": "Kitchen remodel",
                    "postings": [{ "id": 91, "kind": "topic", "seen": true, "name": "Contractor quotes" }]
                })),
        )
        .mount(&server)
        .await;
    let params = GetCollectionParams {
        page: Some("current-cursor".to_string()),
    };

    let page = client(&server)
        .collections()
        .get(33, &params)
        .await
        .unwrap();

    assert_eq!(page.id, 33);
    let postings = page.postings.as_ref().unwrap();
    assert_eq!(postings[0].id, 91);
    assert_eq!(postings[0].seen, Some(true));
    assert_eq!(page.total_count(), Some(42));
    assert_eq!(page.next_page(), Some("next-cursor"));
}

#[tokio::test]
async fn a_collection_that_is_not_there_is_a_not_found() {
    let server = MockServer::start().await;

    let error = client(&server)
        .collections()
        .get(33, &GetCollectionParams::default())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
}

/// The payload leaves out what the caller said nothing about, so a rename keeps whatever
/// summary the collection already has.
#[tokio::test]
async fn a_rename_leaves_the_summary_alone() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/collections/3.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let update = UpdateCollectionRequestContent {
        collection: CollectionPayload {
            name: Some("Kitchen remodel 2026".to_string()),
            summary: None,
        },
    };

    client(&server)
        .collections()
        .update(3, &update)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    let body: Value = serde_json::from_slice(&requests[0].body).unwrap();
    assert_eq!(
        body,
        json!({ "collection": { "name": "Kitchen remodel 2026" } })
    );
}
