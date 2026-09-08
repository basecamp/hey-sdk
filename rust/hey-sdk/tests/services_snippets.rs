mod support;

use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn saving_a_snippet_carries_its_name_and_content() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/snippets"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/snippets"))
        .mount(&server)
        .await;

    client(&server)
        .snippets()
        .create("Scheduling reply", "<div>Does Tuesday work?</div>")
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/snippets");
    assert_eq!(
        form(&requests[0].body),
        [
            ("snippet[name]".to_string(), "Scheduling reply".to_string()),
            (
                "snippet[content]".to_string(),
                "<div>Does Tuesday work?</div>".to_string()
            ),
        ]
    );
}

/// An empty field is left out of the form, so HEY leaves what it names alone.
#[tokio::test]
async fn a_revision_that_names_only_the_name_sends_only_the_name() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/snippets/44"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/snippets"))
        .mount(&server)
        .await;

    client(&server)
        .snippets()
        .update(44, "Scheduling", "")
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/snippets/44");
    assert_eq!(
        form(&requests[0].body),
        [("snippet[name]".to_string(), "Scheduling".to_string())]
    );
}

#[tokio::test]
async fn throwing_a_snippet_away_deletes_it_by_id() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/snippets/44"))
        .respond_with(ResponseTemplate::new(303).insert_header("Location", "/snippets"))
        .mount(&server)
        .await;

    client(&server).snippets().delete(44).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/snippets/44");
    assert!(requests[0].body.is_empty());
}

fn form(body: &[u8]) -> Vec<(String, String)> {
    url::form_urlencoded::parse(body).into_owned().collect()
}
