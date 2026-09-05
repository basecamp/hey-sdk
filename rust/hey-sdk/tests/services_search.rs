mod support;

use std::collections::HashMap;

use hey_sdk::services::SearchParams;
use serde_json::json;
use wiremock::matchers::{method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_search_sends_the_refinements_it_was_given_and_no_others() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/advanced_search.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "matches": [{
                "topic": { "id": 331, "name": "Kitchen remodel" },
                "posting_id": 4471829,
                "entries": [{ "id": 5512, "summary": "The cabinets arrive on Tuesday", "kind": "message" }]
            }]
        })))
        .mount(&server)
        .await;
    let params = SearchParams {
        query: "cabinets".to_string(),
        page: 2,
        from: "Jane".to_string(),
        r#in: "imbox".to_string(),
        ..SearchParams::default()
    };

    let result = client(&server).search().search(&params).await.unwrap();

    assert_eq!(result.matches.len(), 1);
    let matched = &result.matches[0];
    assert_eq!(matched.topic.id, 331);
    assert_eq!(matched.topic.name.as_deref(), Some("Kitchen remodel"));
    assert_eq!(matched.posting_id, Some(4471829));
    let entries = matched.entries.as_ref().unwrap();
    assert_eq!(
        entries[0].summary.as_deref(),
        Some("The cabinets arrive on Tuesday")
    );
    assert_eq!(
        sent_query(&server).await,
        HashMap::from([
            ("q".to_string(), "cabinets".to_string()),
            ("page".to_string(), "2".to_string()),
            ("refine[from]".to_string(), "Jane".to_string()),
            ("refine[in]".to_string(), "imbox".to_string()),
        ])
    );
}

#[tokio::test]
async fn the_first_page_is_asked_for_without_a_page_at_all() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/advanced_search.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "matches": [] })))
        .mount(&server)
        .await;
    let client = client(&server);

    for page in [0, 1] {
        let params = SearchParams {
            query: "cabinets".to_string(),
            page,
            ..SearchParams::default()
        };
        client.search().search(&params).await.unwrap();
    }

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("q=cabinets"));
    assert_eq!(requests[1].url.query(), Some("q=cabinets"));
}

#[tokio::test]
async fn search_numbers_the_page_that_comes_next_and_the_last_one_names_none() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/advanced_search.json"))
        .and(query_param("page", "3"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "matches": [
            { "topic": { "id": 331, "name": "Kitchen remodel" }, "posting_id": 4471829 }
        ] })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/advanced_search.json"))
        .and(query_param("page", "2"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</advanced_search.json?q=cabinets&page=3>; rel="next""#,
                )
                .set_body_json(json!({ "matches": [
                    { "topic": { "id": 332, "name": "Cabinet estimate" }, "posting_id": 4471830 }
                ] })),
        )
        .mount(&server)
        .await;
    let client = client(&server);

    let second = client
        .search()
        .search_page(&page_of("cabinets", 2))
        .await
        .unwrap();
    let last = client
        .search()
        .search_page(&page_of("cabinets", 3))
        .await
        .unwrap();

    assert_eq!(second.next_page, Some(3));
    assert_eq!(second.result.matches[0].topic.id, 332);
    assert_eq!(last.next_page, None);
    assert_eq!(last.result.matches.len(), 1);
}

fn page_of(query: &str, page: u32) -> SearchParams {
    SearchParams {
        query: query.to_string(),
        page,
        ..SearchParams::default()
    }
}

async fn sent_query(server: &MockServer) -> HashMap<String, String> {
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    requests[0]
        .url
        .query_pairs()
        .map(|(name, value)| (name.into_owned(), value.into_owned()))
        .collect()
}
