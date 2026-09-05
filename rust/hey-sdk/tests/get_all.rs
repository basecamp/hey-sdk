mod support;

use std::sync::{Arc, Mutex};

use hey_sdk::ErrorCode;
use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use serde_json::json;
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

#[tokio::test]
async fn every_page_is_read_and_the_items_come_back_as_one_list() {
    let server = MockServer::start().await;
    page(&server, None, json!([{ "id": 1 }]), Some("/items?page=2")).await;
    page(
        &server,
        Some("2"),
        json!([{ "id": 2 }]),
        Some("/items?page=3"),
    )
    .await;
    page(&server, Some("3"), json!([{ "id": 3 }]), None).await;

    let items = client(&server).get_all("/items").await.unwrap();

    assert_eq!(
        items,
        [json!({ "id": 1 }), json!({ "id": 2 }), json!({ "id": 3 })]
    );
    let requests = server.received_requests().await.unwrap();
    let queries: Vec<Option<&str>> = requests.iter().map(|request| request.url.query()).collect();
    assert_eq!(queries, [None, Some("page=2"), Some("page=3")]);
}

#[tokio::test]
async fn a_limit_stops_the_walk_on_exactly_that_many_items() {
    let server = MockServer::start().await;
    page(
        &server,
        None,
        json!([{ "id": 1 }, { "id": 2 }]),
        Some("/items?page=2"),
    )
    .await;
    page(
        &server,
        Some("2"),
        json!([{ "id": 3 }, { "id": 4 }]),
        Some("/items?page=3"),
    )
    .await;

    let items = client(&server)
        .get_all_with_limit("/items", 3)
        .await
        .unwrap();

    assert_eq!(
        items,
        [json!({ "id": 1 }), json!({ "id": 2 }), json!({ "id": 3 })]
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_next_link_off_the_origin_is_refused_rather_than_followed() {
    let server = MockServer::start().await;
    page(
        &server,
        None,
        json!([{ "id": 1 }]),
        Some("https://evil.example.com/steal"),
    )
    .await;

    let refused = client(&server).get_all("/items").await.unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert_eq!(
        refused.message(),
        "pagination Link header points to a different origin: https://evil.example.com/steal"
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn the_walk_stops_at_the_page_limit_with_what_it_has() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</items?page=next>; rel="next""#)
                .set_body_json(json!([{ "id": 1 }])),
        )
        .mount(&server)
        .await;

    let items = builder(&server)
        .max_pages(3)
        .build()
        .unwrap()
        .get_all("/items")
        .await
        .unwrap();

    assert_eq!(items.len(), 3);
    assert_eq!(server.received_requests().await.unwrap().len(), 3);
}

/// Following on from a page already in hand reads only what comes after it, and counts the
/// items it was told that page held towards the limit.
#[tokio::test]
async fn following_on_from_a_page_already_read_leaves_that_page_out() {
    let server = MockServer::start().await;
    page(
        &server,
        None,
        json!([{ "id": 1 }, { "id": 2 }]),
        Some("/items?page=2"),
    )
    .await;
    page(
        &server,
        Some("2"),
        json!([{ "id": 3 }, { "id": 4 }]),
        Some("/items?page=3"),
    )
    .await;

    let client = client(&server);
    let first = client.get("/items").await.unwrap();
    let rest = client.follow_pagination(&first, 2, 3).await.unwrap();

    assert_eq!(rest, [json!({ "id": 3 })]);
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_first_page_that_already_met_the_limit_is_not_followed() {
    let server = MockServer::start().await;
    page(&server, None, json!([{ "id": 1 }]), Some("/items?page=2")).await;

    let client = client(&server);
    let first = client.get("/items").await.unwrap();
    let rest = client.follow_pagination(&first, 10, 10).await.unwrap();

    assert!(rest.is_empty());
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// Walking on from a page is part of the read that produced it, not a request of its own,
/// so every page an application sees comes from the operation it asked for.
#[tokio::test]
async fn a_walk_stays_the_operation_it_started_as() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("page", "2"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=2>; rel="next""#)
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;
    let announced = Announced::new();

    let client = builder(&server).hooks(announced.clone()).build().unwrap();
    let first = client.boxes().list().await.unwrap();
    client.next_page(&first).await.unwrap().unwrap();

    assert_eq!(announced.names(), ["Boxes.ListBoxes", "Boxes.ListBoxes"]);
}

/// Records what each operation announced itself as.
#[derive(Clone)]
struct Announced {
    names: Arc<Mutex<Vec<String>>>,
}

impl Announced {
    fn new() -> Announced {
        Announced {
            names: Arc::new(Mutex::new(Vec::new())),
        }
    }

    fn names(&self) -> Vec<String> {
        self.names.lock().unwrap().clone()
    }
}

impl Hooks for Announced {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.names
            .lock()
            .unwrap()
            .push(format!("{}.{}", op.service, op.operation));
        None
    }
}

async fn page(
    server: &MockServer,
    cursor: Option<&str>,
    items: serde_json::Value,
    next: Option<&str>,
) {
    let mut answer = ResponseTemplate::new(200).set_body_json(items);
    if let Some(next) = next {
        answer = answer.insert_header("Link", format!("<{next}>; rel=\"next\"").as_str());
    }
    let mounted = Mock::given(method("GET")).and(path("/items"));
    match cursor {
        Some(cursor) => mounted.and(query_param("page", cursor)),
        None => mounted.and(query_param_is_missing("page")),
    }
    .respond_with(answer)
    .mount(server)
    .await;
}
