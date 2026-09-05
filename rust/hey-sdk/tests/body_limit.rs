mod support;

use std::time::{Duration, Instant};

use chrono::Utc;
use hey_sdk::{Client, Config, ErrorCode, StaticTokenProvider};
use serde_json::json;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{TOKEN, builder, client};

#[tokio::test]
async fn a_body_at_the_cap_reads_whole_and_one_declared_past_it_never_starts() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/at-the-cap"))
        .respond_with(ResponseTemplate::new(200).set_body_string("x".repeat(64)))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/past-the-cap"))
        .respond_with(ResponseTemplate::new(200).set_body_string("x".repeat(65)))
        .mount(&server)
        .await;

    let client = builder(&server)
        .max_response_body_bytes(64)
        .build()
        .unwrap();

    assert_eq!(client.get("/at-the-cap").await.unwrap().body.len(), 64);
    let refused = client.get("/past-the-cap").await.unwrap_err();
    assert!(refused.is_response_too_large());
    assert_eq!(refused.code(), ErrorCode::Api);
    assert_eq!(
        refused.message(),
        "GET /past-the-cap: response body exceeds 64 bytes"
    );
}

/// An answer that declares no length is refused on the first byte past the cap instead.
/// The server here answers once, so the refusal standing rather than a connection failure
/// is also what says the request was not sent again.
#[tokio::test]
async fn a_streamed_body_is_refused_the_moment_it_passes_the_cap() {
    let address = serve_one_chunked_answer(vec![b'x'; 4096]).await;
    let client = Client::builder(Config::default().with_base_url(address))
        .token_provider(StaticTokenProvider::new(TOKEN))
        .max_response_body_bytes(1024)
        .base_delay(Duration::from_millis(1))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap();

    let refused = client.get("/items").await.unwrap_err();

    assert!(refused.is_response_too_large());
    assert_eq!(
        refused.message(),
        "GET /items: response body exceeds 1024 bytes"
    );
}

/// What the request asked for decides how much may come back, not what the server labels
/// the answer: a document the SDK parses is held to the configured cap, and a blob or an
/// export to the far larger fixed one.
#[tokio::test]
async fn what_was_asked_for_decides_how_much_may_come_back() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/big"))
        .respond_with(ResponseTemplate::new(200).set_body_string("%".repeat(4096)))
        .mount(&server)
        .await;

    let client = builder(&server)
        .max_response_body_bytes(1024)
        .build()
        .unwrap();

    assert_eq!(client.get_blob("/big").await.unwrap().body.len(), 4096);
    assert_eq!(client.get_csv("/big").await.unwrap().body.len(), 4096);
    assert!(
        client
            .get_html("/big")
            .await
            .unwrap_err()
            .is_response_too_large()
    );
    assert!(
        client
            .get("/big")
            .await
            .unwrap_err()
            .is_response_too_large()
    );
}

/// The status is what matters about a failure, and a body too long to read is no reason to
/// lose it.
#[tokio::test]
async fn an_oversized_error_body_still_reports_the_status_it_came_with() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/denied"))
        .respond_with(
            ResponseTemplate::new(401)
                .insert_header("X-Request-Id", "req-7f3a")
                .set_body_string("e".repeat(4096)),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/broken"))
        .respond_with(ResponseTemplate::new(500).set_body_string("e".repeat(4096)))
        .mount(&server)
        .await;

    let client = builder(&server)
        .max_response_body_bytes(1024)
        .max_retries(0)
        .build()
        .unwrap();
    let denied = client.get("/denied").await.unwrap_err();
    let broken = client.get("/broken").await.unwrap_err();

    assert_eq!(denied.code(), ErrorCode::Auth);
    assert_eq!(denied.http_status(), Some(401));
    assert_eq!(denied.request_id(), Some("req-7f3a"));
    assert!(denied.is_response_too_large());
    assert_eq!(
        denied.hint(),
        Some("GET /denied: response body exceeds 1024 bytes")
    );

    assert_eq!(broken.code(), ErrorCode::Api);
    assert_eq!(broken.http_status(), Some(500));
    assert!(broken.is_retryable());
    assert!(broken.is_response_too_large());
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_cap_of_zero_asks_for_the_default_rather_than_for_nothing() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/big"))
        .respond_with(ResponseTemplate::new(200).set_body_string("%".repeat(4096)))
        .mount(&server)
        .await;

    let client = builder(&server).max_response_body_bytes(0).build().unwrap();

    assert_eq!(client.get("/big").await.unwrap().body.len(), 4096);
}

/// `Retry-After` carries either a count of seconds or the HTTP-date the wait is over.
#[tokio::test]
async fn a_retry_after_date_is_waited_out_as_the_seconds_it_leaves() {
    let server = MockServer::start().await;
    let in_two_seconds = (Utc::now() + chrono::Duration::seconds(2))
        .format("%a, %d %b %Y %H:%M:%S GMT")
        .to_string();
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(
            ResponseTemplate::new(429).insert_header("Retry-After", in_two_seconds.as_str()),
        )
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/items"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([])))
        .mount(&server)
        .await;

    let started = Instant::now();
    client(&server).get("/items").await.unwrap();
    let waited = started.elapsed();

    assert_eq!(server.received_requests().await.unwrap().len(), 2);
    assert!(
        waited >= Duration::from_secs(1),
        "resent after only {waited:?}, so the date was not read as a wait"
    );
}

/// One answer with a chunked body and no `Content-Length`, which is what a streamed answer
/// looks like on the wire; wiremock always declares a length.
async fn serve_one_chunked_answer(body: Vec<u8>) -> String {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.unwrap();
        if stream.read(&mut [0; 1024]).await.unwrap_or(0) == 0 {
            return;
        }
        let head = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n";
        if stream.write_all(head).await.is_err() {
            return;
        }
        for chunk in body.chunks(64) {
            let framed = [format!("{:x}\r\n", chunk.len()).as_bytes(), chunk, b"\r\n"].concat();
            if stream.write_all(&framed).await.is_err() {
                return;
            }
        }
        let _ = stream.write_all(b"0\r\n\r\n").await;
    });
    format!("http://{address}")
}
