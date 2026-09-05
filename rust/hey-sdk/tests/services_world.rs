mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::WORLD_ADDRESS;
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn publishing_addresses_the_world_and_answers_the_post_token() {
    let server = MockServer::start().await;
    mock_identity(&server).await;
    mock_publish(&server, "/world/posts/a1b2c3d4").await;

    let token = client(&server)
        .world()
        .publish("On writing less", "<div>Fewer words, more meaning.</div>")
        .await
        .unwrap();

    assert_eq!(token, "a1b2c3d4");
    assert_eq!(
        sent_form(&server).await,
        [
            "acting_sender_id=42".to_string(),
            "message[subject]=On writing less".to_string(),
            "message[content]=<div>Fewer words, more meaning.</div>".to_string(),
            format!("entry[addressed][directly]={WORLD_ADDRESS}"),
            "entry[status]=active".to_string(),
        ]
    );
}

/// The message goes out either way, so a landing that is not a post is reported as what it
/// is rather than as a failure to send.
#[tokio::test]
async fn a_message_that_did_not_become_a_post_says_where_it_landed() {
    let server = MockServer::start().await;
    mock_identity(&server).await;
    mock_publish(&server, "/topics/4471829").await;

    let refused = client(&server)
        .world()
        .publish("On writing less", "<div>Fewer words.</div>")
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Api);
    assert_eq!(
        refused.message(),
        "the message was sent but did not become a HEY World post (landed on \"/topics/4471829\")"
    );
}

#[tokio::test]
async fn an_edit_leaves_out_what_it_was_not_given() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/world/posts/a1b2c3d4"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", "/world/posts/a1b2c3d4"))
        .mount(&server)
        .await;

    client(&server)
        .world()
        .update_post("a1b2c3d4", "On writing less, revisited", "")
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        ["world_post[subject]=On writing less, revisited"]
    );
}

#[tokio::test]
async fn deleting_a_post_names_it_by_its_token() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/world/posts/a1b2c3d4"))
        .respond_with(
            ResponseTemplate::new(303).insert_header("Location", "/world/lists/david@example.com"),
        )
        .mount(&server)
        .await;

    client(&server)
        .world()
        .delete_post("a1b2c3d4")
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert!(requests[0].body.is_empty());
}

#[tokio::test]
async fn subscribers_export_as_the_csv_hey_streamed() {
    let server = MockServer::start().await;
    let csv = "email_address,subscribed_at\njane.dawson@example.com,2026-04-06 09:00:00 UTC\n";
    Mock::given(method("GET"))
        .and(path("/world/lists/david%40example.com/export.csv"))
        .respond_with(ResponseTemplate::new(200).set_body_string(csv))
        .mount(&server)
        .await;

    let exported = client(&server)
        .world()
        .export_subscribers("david@example.com")
        .await
        .unwrap();

    assert_eq!(String::from_utf8(exported.to_vec()).unwrap(), csv);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].headers["accept"], "text/csv");
}

#[tokio::test]
async fn an_import_uploads_the_csv_as_the_part_hey_reads() {
    let server = MockServer::start().await;
    mock_import(&server).await;

    client(&server)
        .world()
        .import_subscribers(
            "david@example.com",
            "subscribers",
            b"email_address\njane.dawson@example.com\n",
        )
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    let content_type = requests[0].headers["content-type"].to_str().unwrap();
    let boundary = content_type
        .strip_prefix("multipart/form-data; boundary=")
        .unwrap();
    let body = String::from_utf8(requests[0].body.clone()).unwrap();
    assert_eq!(
        body,
        format!(
            "--{boundary}\r\nContent-Disposition: form-data; name=\"world_list_import[source]\"; filename=\"subscribers.csv\"\r\nContent-Type: application/octet-stream\r\n\r\nemail_address\njane.dawson@example.com\n\r\n--{boundary}--\r\n"
        )
    );
}

/// A name is the caller's, an extension is HEY's business: the import is read by its `.csv`
/// suffix, so an unnamed file becomes `subscribers.csv` and a named one keeps its name.
#[tokio::test]
async fn an_import_without_a_csv_name_gets_one() {
    let server = MockServer::start().await;
    mock_import(&server).await;
    let client = client(&server);

    client
        .world()
        .import_subscribers("david@example.com", "", b"email_address\n")
        .await
        .unwrap();
    client
        .world()
        .import_subscribers("david@example.com", "readers.csv", b"email_address\n")
        .await
        .unwrap();

    assert_eq!(
        uploaded_filenames(&server).await,
        ["subscribers.csv", "readers.csv"]
    );
}

async fn mock_identity(server: &MockServer) {
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 1, "senders": [{ "id": 42, "default": true }] })),
        )
        .mount(server)
        .await;
}

async fn mock_publish(server: &MockServer, location: &str) {
    Mock::given(method("POST"))
        .and(path("/messages"))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", location))
        .mount(server)
        .await;
}

async fn mock_import(server: &MockServer) {
    Mock::given(method("POST"))
        .and(path("/world/lists/david%40example.com/imports"))
        .respond_with(
            ResponseTemplate::new(302)
                .insert_header("Location", "/world/lists/david@example.com/imports/1"),
        )
        .mount(server)
        .await;
}

/// The form the publish carried. The identity read comes first, so the message is the
/// request after it.
async fn sent_form(server: &MockServer) -> Vec<String> {
    let requests = server.received_requests().await.unwrap();
    let message = requests.last().unwrap();
    url::form_urlencoded::parse(&message.body)
        .map(|(name, value)| format!("{name}={value}"))
        .collect()
}

async fn uploaded_filenames(server: &MockServer) -> Vec<String> {
    server
        .received_requests()
        .await
        .unwrap()
        .iter()
        .map(|request| {
            let body = String::from_utf8(request.body.clone()).unwrap();
            let (_, named) = body.split_once("filename=\"").unwrap();
            let (filename, _) = named.split_once('"').unwrap();
            filename.to_string()
        })
        .collect()
}
