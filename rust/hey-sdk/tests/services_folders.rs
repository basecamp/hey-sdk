mod support;

use hey_sdk::services::folders::GetFolderParams;
use serde_json::json;
use wiremock::matchers::{method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_folder_page_carries_its_postings_the_total_and_the_next_cursor() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/folders/12.json"))
        .and(query_param("page", "current-cursor"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("X-Total-Count", "42")
                .insert_header("Link", r#"</folders/12.json?page=next-cursor>; rel="next""#)
                .set_body_json(json!({
                    "id": 12,
                    "name": "Receipts",
                    "postings": [{ "id": 1, "kind": "topic" }]
                })),
        )
        .mount(&server)
        .await;
    let params = GetFolderParams {
        page: Some("current-cursor".to_string()),
    };

    let page = client(&server).folders().get(12, &params).await.unwrap();

    assert_eq!(page.name.as_deref(), Some("Receipts"));
    assert_eq!(page.postings.as_ref().unwrap().len(), 1);
    assert_eq!(page.total_count(), Some(42));
    assert_eq!(page.next_page(), Some("next-cursor"));
}

/// The last page carries no Link header, which is how a caller walking a folder is told to
/// stop asking for more.
#[tokio::test]
async fn the_last_page_of_a_folder_names_no_next_one() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/folders/12.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 12, "name": "Receipts", "postings": [] })),
        )
        .mount(&server)
        .await;

    let page = client(&server)
        .folders()
        .get(12, &GetFolderParams::default())
        .await
        .unwrap();

    assert_eq!(page.next_page(), None);
    assert!(!page.has_next());
}
