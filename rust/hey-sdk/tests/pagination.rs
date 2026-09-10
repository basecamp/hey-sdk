//! `Page` and `next_page`: the `Link` cursor, `X-Total-Count`, and the origin a cursor may not leave.

mod support;

use hey_sdk::ErrorCode;
use serde_json::json;
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

#[tokio::test]
async fn a_page_carries_the_total_and_the_cursor_for_the_next_one() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=cursor-2>; rel="next""#)
                .insert_header("X-Total-Count", "75")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let page = client(&server).boxes().list().await.unwrap();

    assert_eq!(page.total_count(), Some(75));
    assert_eq!(page.next_page(), Some("cursor-2"));
    assert!(page.has_next());
    assert_eq!(page.next_url().unwrap().path(), "/boxes.json");
    assert_eq!(page.len(), 1);
}

#[tokio::test]
async fn an_absolute_link_names_the_same_next_page() {
    let server = MockServer::start().await;
    let link = format!(r#"<{}/boxes.json?page=cursor-2>; rel="next""#, server.uri());
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", link.as_str())
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;

    let page = client(&server).boxes().list().await.unwrap();

    assert_eq!(page.next_page(), Some("cursor-2"));
    assert_eq!(
        page.next_url().unwrap().as_str(),
        format!("{}/boxes.json?page=cursor-2", server.uri())
    );
    assert_eq!(page.total_count(), None);
}

#[tokio::test]
async fn the_next_page_is_followed_and_the_last_one_ends_the_list() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("page", "cursor-2"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 8, "kind": "feedbox", "name": "The Feed" }])),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param_is_missing("page"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=cursor-2>; rel="next""#)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = client(&server);
    let first = client.boxes().list().await.unwrap();
    let second = client.next_page(&first).await.unwrap().unwrap();

    assert_eq!(first[0].name, "Imbox");
    assert_eq!(second[0].name, "The Feed");
    assert!(client.next_page(&second).await.unwrap().is_none());
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_link_that_leaves_the_hey_origin_is_refused() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"<https://evil.example.com/boxes.json?page=2>; rel="next""#,
                )
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;
    let downgrade = format!(
        r#"<{}/contacts.json?page=2>; rel="next""#,
        server.uri().replace("http://", "https://")
    );
    Mock::given(method("GET"))
        .and(path("/contacts.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", downgrade.as_str())
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;

    let client = client(&server);
    let boxes = client.boxes().list().await.unwrap();
    let contacts = client
        .contacts()
        .list(&hey_sdk::services::contacts::ListContactsParams::default())
        .await
        .unwrap();
    let elsewhere = client.next_page(&boxes).await.unwrap_err();
    let other_scheme = client.next_page(&contacts).await.unwrap_err();

    assert_eq!(elsewhere.code(), ErrorCode::Usage);
    assert_eq!(
        elsewhere.message(),
        "pagination Link header points to a different origin: https://evil.example.com/boxes.json?page=2"
    );
    assert_eq!(other_scheme.code(), ErrorCode::Usage);
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

/// The pages visited stand; the walk says it was cut off rather than ending as if the
/// last page visited were the last page there was.
#[tokio::test]
async fn reading_every_page_stops_at_the_page_limit_and_says_so() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=more>; rel="next""#)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = builder(&server).max_pages(3).build().unwrap();
    let first = client.boxes().list().await.unwrap();
    let mut visited = 0;
    let error = client
        .each_page(first, |page| {
            visited += page.len();
            true
        })
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert!(error.message().contains("page limit of 3"), "{error}");
    assert_eq!(visited, 3);
    assert_eq!(server.received_requests().await.unwrap().len(), 3);
}

/// A visitor that stops the walk stops it: no page after the one it declined is read,
/// and reaching the limit while stopping is not a truncation.
#[tokio::test]
async fn a_visitor_that_stops_is_not_cut_off() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=more>; rel="next""#)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = builder(&server).max_pages(2).build().unwrap();
    let first = client.boxes().list().await.unwrap();
    let mut visited = 0;
    client
        .each_page(first, |_| {
            visited += 1;
            visited < 2
        })
        .await
        .unwrap();

    assert_eq!(visited, 2);
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}
