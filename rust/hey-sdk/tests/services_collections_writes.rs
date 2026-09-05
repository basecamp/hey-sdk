mod support;

use hey_sdk::services::{CreateCollectionParams, UpdateCollectionParams};
use serde_json::json;
use wiremock::matchers::{method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_collection_carries_its_name_summary_and_account() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/collections"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/collections"))
        .mount(&server)
        .await;
    let params = CreateCollectionParams {
        name: "Kitchen remodel".to_string(),
        summary: Some("Quotes, drawings and the schedule".to_string()),
        account_id: Some(77),
    };

    client(&server).collections().create(&params).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/collections");
    assert_eq!(
        form(&requests[0].body),
        [
            (
                "collection[name]".to_string(),
                "Kitchen remodel".to_string()
            ),
            (
                "collection[summary]".to_string(),
                "Quotes, drawings and the schedule".to_string()
            ),
            ("account_id".to_string(), "77".to_string()),
        ]
    );
}

#[tokio::test]
async fn a_collection_with_nothing_but_a_name_sends_nothing_but_the_name() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/collections"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/collections"))
        .mount(&server)
        .await;
    let params = CreateCollectionParams {
        name: "Kitchen remodel".to_string(),
        ..CreateCollectionParams::default()
    };

    client(&server).collections().create(&params).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        form(&requests[0].body),
        [(
            "collection[name]".to_string(),
            "Kitchen remodel".to_string()
        )]
    );
}

/// A field left unset is left off the wire, so HEY leaves the one it does not hear about
/// alone rather than blanking it.
#[tokio::test]
async fn an_edit_sends_only_the_fields_it_names() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/collections/3.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let client = client(&server);

    client
        .collections()
        .update_collection(
            3,
            &UpdateCollectionParams {
                name: Some("Kitchen remodel".to_string()),
                summary: Some("Quotes and drawings".to_string()),
            },
        )
        .await
        .unwrap();
    client
        .collections()
        .update_collection(
            3,
            &UpdateCollectionParams {
                summary: Some(String::new()),
                ..UpdateCollectionParams::default()
            },
        )
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        serde_json::from_slice::<serde_json::Value>(&requests[0].body).unwrap(),
        json!({ "collection": { "name": "Kitchen remodel", "summary": "Quotes and drawings" } })
    );
    assert_eq!(
        serde_json::from_slice::<serde_json::Value>(&requests[1].body).unwrap(),
        json!({ "collection": {} })
    );
}

#[tokio::test]
async fn filing_a_topic_names_the_collection_in_the_query_and_posts_an_empty_form() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/topics/4471829/collecting"))
        .and(query_param("collection_id", "3"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/topics/4471829"))
        .mount(&server)
        .await;

    client(&server)
        .collections()
        .add_topic(4471829, 3)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/topics/4471829/collecting");
    assert_eq!(requests[0].url.query(), Some("collection_id=3"));
    assert_eq!(
        requests[0].headers["content-type"],
        "application/x-www-form-urlencoded"
    );
    assert!(requests[0].body.is_empty());
}

#[tokio::test]
async fn taking_a_topic_back_out_deletes_the_same_path() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/topics/4471829/collecting"))
        .and(query_param("collection_id", "3"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/topics/4471829"))
        .mount(&server)
        .await;

    client(&server)
        .collections()
        .remove_topic(4471829, 3)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/topics/4471829/collecting");
    assert_eq!(requests[0].url.query(), Some("collection_id=3"));
    assert!(requests[0].body.is_empty());
}

fn form(body: &[u8]) -> Vec<(String, String)> {
    url::form_urlencoded::parse(body).into_owned().collect()
}
