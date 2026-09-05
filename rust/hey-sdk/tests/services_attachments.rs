mod support;

use hey_sdk::ErrorCode;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

const CONTENTS: &[u8] = b"quarterly report contents";

/// The base64 MD5 of [`CONTENTS`], which is the integrity check Active Storage insists on.
const CHECKSUM: &str = "MQHgXimkWeoCLgITDhwBfg==";

#[tokio::test]
async fn an_upload_reserves_a_blob_and_puts_the_bytes_where_hey_said() {
    let storage = MockServer::start().await;
    mock_storage(&storage, ResponseTemplate::new(204)).await;
    let server = MockServer::start().await;
    mock_direct_upload(
        &server,
        ResponseTemplate::new(200).set_body_json(direct_upload(&storage.uri())),
    )
    .await;

    let upload = client(&server)
        .attachments()
        .upload("quarterly-report.pdf", Some("application/pdf"), CONTENTS)
        .await
        .unwrap();

    assert_eq!(upload.signed_id, "signed-123");
    assert_eq!(upload.attachable_sgid, "sgid-456");
    assert_eq!(
        reserved_blob(&server).await,
        json!({
            "filename": "quarterly-report.pdf",
            "byte_size": CONTENTS.len(),
            "checksum": CHECKSUM,
            "content_type": "application/pdf"
        })
    );

    let stored = storage.received_requests().await.unwrap();
    assert_eq!(stored.len(), 1);
    assert_eq!(stored[0].body, CONTENTS);
    assert_eq!(stored[0].headers["content-type"], "application/pdf");
    assert_eq!(stored[0].headers["content-md5"], CHECKSUM);
    assert_eq!(
        stored[0].headers["content-length"],
        CONTENTS.len().to_string().as_str()
    );
}

/// The HEY credentials belong on the HEY origin. The storage URL authenticates itself, and
/// even an `Authorization` HEY echoed back in the upload headers is dropped.
#[tokio::test]
async fn the_upload_carries_no_credentials_to_the_storage_service() {
    let storage = MockServer::start().await;
    mock_storage(&storage, ResponseTemplate::new(204)).await;
    let server = MockServer::start().await;
    let mut answer = direct_upload(&storage.uri());
    answer["direct_upload"]["headers"]["Authorization"] = json!("Bearer must-not-leak");
    mock_direct_upload(&server, ResponseTemplate::new(200).set_body_json(answer)).await;

    client(&server)
        .attachments()
        .upload("quarterly-report.pdf", Some("application/pdf"), CONTENTS)
        .await
        .unwrap();

    let stored = storage.received_requests().await.unwrap();
    assert!(!stored[0].headers.contains_key("authorization"));
}

#[tokio::test]
async fn an_attachment_with_no_content_type_is_a_stream_of_bytes() {
    let storage = MockServer::start().await;
    mock_storage(&storage, ResponseTemplate::new(204)).await;
    let server = MockServer::start().await;
    mock_direct_upload(
        &server,
        ResponseTemplate::new(200).set_body_json(direct_upload(&storage.uri())),
    )
    .await;

    client(&server)
        .attachments()
        .upload("notes", None, b"".as_slice())
        .await
        .unwrap();

    assert_eq!(
        reserved_blob(&server).await,
        json!({
            "filename": "notes",
            "byte_size": 0,
            "checksum": "1B2M2Y8AsgTpgAmY7PhCfg==",
            "content_type": "application/octet-stream"
        })
    );
}

#[tokio::test]
async fn an_attachment_needs_a_filename() {
    let server = MockServer::start().await;

    let refused = client(&server)
        .attachments()
        .upload("", Some("application/pdf"), CONTENTS)
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert_eq!(refused.message(), "an attachment needs a filename");
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn a_reservation_that_names_no_upload_is_refused() {
    let server = MockServer::start().await;
    mock_direct_upload(
        &server,
        ResponseTemplate::new(200).set_body_json(json!({
            "signed_id": "signed-123",
            "attachable_sgid": "",
            "direct_upload": { "url": "https://uploads.example.com/blob", "headers": {} }
        })),
    )
    .await;

    let refused = client(&server)
        .attachments()
        .upload("report.pdf", Some("application/pdf"), b"report".as_slice())
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Api);
    assert_eq!(
        refused.message(),
        "HEY returned an empty attachment upload response"
    );
}

/// The bytes would carry the storage service's own credentials, so a plain-HTTP target is
/// refused before anything is sent.
#[tokio::test]
async fn an_insecure_storage_target_is_refused() {
    let server = MockServer::start().await;
    mock_direct_upload(
        &server,
        ResponseTemplate::new(200).set_body_json(json!({
            "signed_id": "signed-123",
            "attachable_sgid": "sgid-456",
            "direct_upload": { "url": "http://uploads.example.com/blob", "headers": {} }
        })),
    )
    .await;

    let refused = client(&server)
        .attachments()
        .upload("report.pdf", Some("application/pdf"), b"report".as_slice())
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert!(
        refused
            .message()
            .starts_with("unsafe attachment upload target"),
        "{}",
        refused.message()
    );
}

#[tokio::test]
async fn a_storage_service_that_refuses_the_bytes_reports_its_status() {
    let storage = MockServer::start().await;
    mock_storage(&storage, ResponseTemplate::new(403)).await;
    let server = MockServer::start().await;
    mock_direct_upload(
        &server,
        ResponseTemplate::new(200).set_body_json(direct_upload(&storage.uri())),
    )
    .await;

    let refused = client(&server)
        .attachments()
        .upload("quarterly-report.pdf", Some("application/pdf"), CONTENTS)
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Forbidden);
    assert_eq!(refused.http_status(), Some(403));
}

/// The bytes go out on the client's own HTTP client, so whatever the caller configured it
/// with — a proxy, a certificate, the headers below standing in for both — reaches the
/// storage service too. Only the HEY credentials are held back.
#[tokio::test]
async fn the_bytes_go_out_on_the_clients_own_http_client() {
    let storage = MockServer::start().await;
    mock_storage(&storage, ResponseTemplate::new(204)).await;
    let server = MockServer::start().await;
    mock_direct_upload(
        &server,
        ResponseTemplate::new(200).set_body_json(direct_upload(&storage.uri())),
    )
    .await;
    let mut configured = reqwest::header::HeaderMap::new();
    configured.insert("x-through-the-proxy", "yes".parse().unwrap());

    builder(&server)
        .http_client(
            reqwest::Client::builder()
                .default_headers(configured)
                .build()
                .unwrap(),
        )
        .build()
        .unwrap()
        .attachments()
        .upload("quarterly-report.pdf", Some("application/pdf"), CONTENTS)
        .await
        .unwrap();

    let stored = storage.received_requests().await.unwrap();
    assert_eq!(stored[0].headers["x-through-the-proxy"], "yes");
    assert!(!stored[0].headers.contains_key("authorization"));
}

async fn mock_direct_upload(server: &MockServer, answer: ResponseTemplate) {
    Mock::given(method("POST"))
        .and(path("/rails/active_storage/direct_uploads.json"))
        .respond_with(answer)
        .mount(server)
        .await;
}

async fn mock_storage(storage: &MockServer, answer: ResponseTemplate) {
    Mock::given(method("PUT"))
        .and(path("/blob"))
        .respond_with(answer)
        .mount(storage)
        .await;
}

/// The reservation HEY answers with, pointing at the storage server this test runs. The URL
/// is plain HTTP on localhost, which is the one place the SDK lets credentials travel
/// unencrypted.
fn direct_upload(storage_uri: &str) -> Value {
    json!({
        "signed_id": "signed-123",
        "attachable_sgid": "sgid-456",
        "direct_upload": {
            "url": format!("{storage_uri}/blob"),
            "headers": { "Content-Type": "application/pdf", "Content-MD5": CHECKSUM }
        }
    })
}

async fn reserved_blob(server: &MockServer) -> Value {
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    let body: Value = serde_json::from_slice(&requests[0].body).unwrap();
    body["blob"].clone()
}
