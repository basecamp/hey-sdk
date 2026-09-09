mod support;

use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

/// HEY files a designation under the box that holds it, so this posts to the box's path.
#[tokio::test]
async fn designating_a_contact_posts_it_to_the_box() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/boxes/5/designations.json"))
        .respond_with(ResponseTemplate::new(201))
        .mount(&server)
        .await;

    client(&server)
        .designations()
        .create_box_designation(5, 91824)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/boxes/5/designations.json");
    let body: Value = serde_json::from_slice(&requests[0].body).unwrap();
    assert_eq!(body, json!({ "contact_id": 91824 }));
}

#[tokio::test]
async fn removing_a_designation_names_the_designation_rather_than_the_contact() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/boxes/5/designations/44.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    client(&server).designations().delete(5, 44).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/boxes/5/designations/44.json");
}
