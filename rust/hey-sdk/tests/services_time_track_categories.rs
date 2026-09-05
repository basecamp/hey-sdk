mod support;

use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_category_is_created_by_its_title() {
    let server = MockServer::start().await;
    mock_form(&server, "POST", "/calendar/time_tracks/categories").await;

    client(&server)
        .time_tracks()
        .create_category("Client work")
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/calendar/time_tracks/categories");
    assert_eq!(
        form(&requests[0].body),
        [("category[title]".to_string(), "Client work".to_string())]
    );
}

#[tokio::test]
async fn renaming_a_category_patches_the_one_named() {
    let server = MockServer::start().await;
    mock_form(&server, "PATCH", "/calendar/time_tracks/categories/31").await;

    client(&server)
        .time_tracks()
        .update_category(31, "Client work 2026")
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.path(),
        "/calendar/time_tracks/categories/31"
    );
    assert_eq!(
        form(&requests[0].body),
        [(
            "category[title]".to_string(),
            "Client work 2026".to_string()
        )]
    );
}

#[tokio::test]
async fn removing_a_category_sends_no_body() {
    let server = MockServer::start().await;
    mock_form(&server, "DELETE", "/calendar/time_tracks/categories/31").await;

    client(&server)
        .time_tracks()
        .delete_category(31)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.path(),
        "/calendar/time_tracks/categories/31"
    );
    assert!(requests[0].body.is_empty());
}

#[tokio::test]
async fn the_export_comes_back_as_the_file_hey_streamed() {
    let csv = "Start,End,Duration,Category,Notes\n\
               2026-04-06 09:00,2026-04-06 11:00,2:00,Client work,Kitchen remodel call\n";
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/calendar/time_tracks/exports"))
        .and(header("accept", "text/csv"))
        .respond_with(ResponseTemplate::new(200).set_body_string(csv))
        .mount(&server)
        .await;

    let exported = client(&server).time_tracks().export().await.unwrap();

    assert_eq!(exported, csv.as_bytes());
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/calendar/time_tracks/exports");
}

async fn mock_form(server: &MockServer, verb: &str, at: &str) {
    Mock::given(method(verb))
        .and(path(at.to_string()))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/calendar/time_tracks"))
        .mount(server)
        .await;
}

fn form(body: &[u8]) -> Vec<(String, String)> {
    url::form_urlencoded::parse(body).into_owned().collect()
}
