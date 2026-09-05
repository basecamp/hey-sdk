mod support;

use async_trait::async_trait;
use hey_sdk::cache::InMemoryCache;
use hey_sdk::{AuthStrategy, Client, Config, Error, ErrorCode};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

#[tokio::test]
async fn every_verb_reaches_the_path_it_was_given() {
    let server = MockServer::start().await;
    for verb in ["GET", "POST", "PUT", "PATCH", "DELETE"] {
        Mock::given(method(verb))
            .and(path("/custom/thing"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "ok": true })))
            .mount(&server)
            .await;
    }

    let client = client(&server);
    let body = json!({ "name": "Widget" });
    client.get("/custom/thing").await.unwrap();
    client.post("/custom/thing", &body).await.unwrap();
    client.put("/custom/thing", &body).await.unwrap();
    client.patch("/custom/thing", &body).await.unwrap();
    client.delete("/custom/thing").await.unwrap();

    let requests = server.received_requests().await.unwrap();
    let sent: Vec<(&str, &str)> = requests
        .iter()
        .map(|request| (request.method.as_str(), request.url.path()))
        .collect();
    assert_eq!(
        sent,
        [
            ("GET", "/custom/thing"),
            ("POST", "/custom/thing"),
            ("PUT", "/custom/thing"),
            ("PATCH", "/custom/thing"),
            ("DELETE", "/custom/thing")
        ]
    );
}

/// A modelled read has the `.json` Smithy could not spell put back on; a path the caller
/// wrote goes out as they wrote it.
#[tokio::test]
async fn a_raw_path_keeps_the_shape_it_was_written_in() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/123"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "id": 123 })))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes/123.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 123, "kind": "imbox", "name": "Imbox" })),
        )
        .mount(&server)
        .await;

    let client = client(&server);
    client.get("/boxes/123").await.unwrap();
    client.boxes().get(123, &Default::default()).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    let paths: Vec<&str> = requests.iter().map(|request| request.url.path()).collect();
    assert_eq!(paths, ["/boxes/123", "/boxes/123.json"]);
}

#[tokio::test]
async fn each_verb_says_what_kind_of_answer_it_wants() {
    let server = MockServer::start().await;
    Mock::given(path("/thing"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .mount(&server)
        .await;

    let client = client(&server);
    let body = json!({});
    client.get("/thing").await.unwrap();
    client.get_html("/thing").await.unwrap();
    client.get_csv("/thing").await.unwrap();
    client.get_blob("/thing").await.unwrap();
    client.post_mutation("/thing", &body).await.unwrap();
    client.patch_mutation("/thing", &body).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    let accepts: Vec<&str> = requests
        .iter()
        .map(|request| request.headers["accept"].to_str().unwrap())
        .collect();
    assert_eq!(
        accepts,
        [
            "application/json",
            "text/html",
            "text/csv",
            "*/*",
            "*/*",
            "*/*"
        ]
    );
}

/// A read is safe to send again, and so is replacing or removing a record. Creating one or
/// changing part of one is not: the first attempt may have gone through.
#[tokio::test]
async fn a_put_and_a_delete_are_resent_where_a_post_and_a_patch_are_not() {
    let server = MockServer::start().await;
    for verb in ["PUT", "DELETE", "POST", "PATCH"] {
        Mock::given(method(verb))
            .and(path("/custom"))
            .respond_with(ResponseTemplate::new(503))
            .mount(&server)
            .await;
    }

    let client = builder(&server).max_retries(2).build().unwrap();
    let body = json!({});
    client.put("/custom", &body).await.unwrap_err();
    client.delete("/custom").await.unwrap_err();
    client.post("/custom", &body).await.unwrap_err();
    client.patch("/custom", &body).await.unwrap_err();

    let requests = server.received_requests().await.unwrap();
    let attempts: Vec<&str> = requests
        .iter()
        .map(|request| request.method.as_str())
        .collect();
    assert_eq!(
        attempts,
        [
            "PUT", "PUT", "PUT", "DELETE", "DELETE", "DELETE", "POST", "PATCH"
        ]
    );
}

#[tokio::test]
async fn an_absolute_url_is_sent_as_it_stands_and_plain_http_stays_home() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;

    let client = client(&server);
    let here = format!("{}/items?page=2", server.uri());
    client.get(&here).await.unwrap();
    let refused = client
        .get("http://elsewhere.example.com/items")
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert_eq!(
        refused.message(),
        "URL must use HTTPS, got: http://elsewhere.example.com/items"
    );
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].url.query(), Some("page=2"));
}

#[tokio::test]
async fn a_blob_is_read_whole_and_never_revalidated() {
    let server = MockServer::start().await;
    let png: &[u8] = &[
        0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff, 0xfe,
    ];
    Mock::given(method("GET"))
        .and(path("/rails/blobs/abc/image.png"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"abc\"")
                .insert_header("Content-Type", "image/png")
                .set_body_bytes(png),
        )
        .mount(&server)
        .await;

    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    for _ in 0..2 {
        let blob = client.get_blob("/rails/blobs/abc/image.png").await.unwrap();
        assert_eq!(blob.body.as_ref(), png);
        assert!(!blob.from_cache);
    }

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    for request in &requests {
        assert_eq!(request.headers["accept"], "*/*");
        assert!(!request.headers.contains_key("if-none-match"));
    }
}

#[tokio::test]
async fn a_blob_off_the_hey_origin_is_refused_before_anything_is_sent() {
    let server = MockServer::start().await;

    let refused = client(&server)
        .get_blob("https://files.example.org/report.pdf")
        .await
        .unwrap_err();

    assert_eq!(refused.code(), ErrorCode::Usage);
    assert_eq!(refused.message(), "a blob URL must start on the HEY origin");
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn a_download_writes_the_file_out_and_hands_back_its_headers() {
    let server = MockServer::start().await;
    let attachment = b"streamed attachment".to_vec();
    Mock::given(method("GET"))
        .and(path("/rails/blobs/abc/file.bin"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Content-Type", "application/octet-stream")
                .set_body_bytes(attachment.clone()),
        )
        .mount(&server)
        .await;

    let mut destination = Vec::new();
    let (written, headers) = client(&server)
        .download_blob("/rails/blobs/abc/file.bin", &mut destination)
        .await
        .unwrap();

    assert_eq!(written, attachment.len() as u64);
    assert_eq!(destination, attachment);
    assert_eq!(headers["content-type"], "application/octet-stream");
    assert_eq!(
        server.received_requests().await.unwrap()[0].headers["accept"],
        "*/*"
    );
}

/// The cache is partitioned by the identity that filled it, so a request that carries no
/// credentials has nothing to file its answer under and is not cached at all.
#[tokio::test]
async fn nothing_is_cached_for_a_request_that_carries_no_credentials() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([])),
        )
        .mount(&server)
        .await;

    let client = Client::builder(Config::default().with_base_url(server.uri()))
        .auth_strategy(Anonymous)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    client.get("/items").await.unwrap();
    let second = client.get("/items").await.unwrap();

    assert!(!second.from_cache);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    for request in &requests {
        assert!(!request.headers.contains_key("authorization"));
        assert!(!request.headers.contains_key("if-none-match"));
    }
}

/// A strategy that signs nothing, for the deployments that authenticate somewhere else
/// entirely — a proxy, a mesh, a session cookie the transport already holds.
struct Anonymous;

#[async_trait]
impl AuthStrategy for Anonymous {
    async fn authenticate(&self, _request: &mut reqwest::Request) -> Result<(), Error> {
        Ok(())
    }
}
