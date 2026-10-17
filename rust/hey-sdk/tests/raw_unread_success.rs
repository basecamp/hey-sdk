//! A write to a path the model does not cover has taken effect once HEY answers a success,
//! so a body lost after that answer is the success with the loss attached, never an error a
//! caller would answer by sending the write again.

mod support;

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

use hey_sdk::{Client, Config, ErrorCode, StaticTokenProvider};
use serde_json::{Value, json};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{TOKEN, builder};

/// A server that answers every request 200, promises a 1000-byte body, sends the start of
/// one and then closes the connection, as a proxy that drops a connection mid-answer does.
/// It counts the requests it sees.
async fn serve_broken_successes() -> (String, Arc<AtomicUsize>) {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let requests = Arc::new(AtomicUsize::new(0));
    let seen = Arc::clone(&requests);
    tokio::spawn(async move {
        loop {
            let Ok((mut stream, _)) = listener.accept().await else {
                return;
            };
            let seen = Arc::clone(&seen);
            tokio::spawn(async move {
                if stream.read(&mut [0; 8192]).await.unwrap_or(0) == 0 {
                    return;
                }
                seen.fetch_add(1, Ordering::SeqCst);
                let answer = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{\"id\":2201,\"topic_id\":";
                let _ = stream.write_all(answer).await;
                let _ = stream.flush().await;
                // Dropping the stream closes the connection 978 bytes short.
            });
        }
    });
    (format!("http://{address}"), requests)
}

fn client_for(address: String) -> Client {
    Client::builder(Config::default().with_base_url(address))
        .token_provider(StaticTokenProvider::new(TOKEN))
        .http_client(support::http_client())
        .max_retries(2)
        .base_delay(Duration::from_millis(1))
        .max_jitter(Duration::ZERO)
        .build()
        .unwrap()
}

#[tokio::test]
async fn a_raw_post_and_put_whose_success_body_breaks_answer_the_success() {
    let body = json!({ "subject": "Lunch on Friday" });
    for verb in ["POST", "PUT"] {
        let (address, requests) = serve_broken_successes().await;
        let client = client_for(address);

        let response = match verb {
            "POST" => client.post("/messages", &body).await,
            _ => client.put("/messages/1", &body).await,
        }
        .unwrap_or_else(|error| panic!("{verb} answered 200 must not fail over its body: {error}"));

        assert_eq!(response.status.as_u16(), 200, "{verb}");
        assert!(response.body.is_empty(), "{verb} kept a partial body");
        let unread = response
            .body_error
            .as_ref()
            .expect("the read failure is attached");
        assert_eq!(unread.code(), ErrorCode::Network, "{verb}");
        assert_eq!(
            requests.load(Ordering::SeqCst),
            1,
            "{verb} that took effect was sent again"
        );

        let decoded = response.json::<Value>().unwrap_err();
        assert_eq!(decoded.code(), ErrorCode::Network, "{verb}");
        assert_eq!(decoded.http_status(), Some(200), "{verb}");
    }
}

#[tokio::test]
async fn send_unit_answers_the_success_and_send_and_send_text_the_missing_body() {
    let (address, _) = serve_broken_successes().await;
    let client = client_for(address);
    let write = || {
        let mut operation = client.request(hey_sdk::http::Method::POST, "/messages");
        operation.json(&json!({ "subject": "Lunch" })).unwrap();
        operation
    };

    client.send_unit(write()).await.unwrap();
    assert_eq!(
        client.send::<Value>(write()).await.unwrap_err().code(),
        ErrorCode::Network
    );
    assert_eq!(
        client.send_text(write()).await.unwrap_err().code(),
        ErrorCode::Network
    );
}

/// A GET changes nothing, so a GET whose body breaks is the error it always was: the caller
/// can fetch again, and an empty success would read as an empty resource.
#[tokio::test]
async fn a_raw_get_whose_body_breaks_is_still_an_error() {
    let (address, _) = serve_broken_successes().await;
    let client = client_for(address);

    let error = client.get("/topics/1").await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Network);
}

#[tokio::test]
async fn a_raw_post_whose_success_body_runs_past_the_cap_answers_the_success() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/messages"))
        .respond_with(ResponseTemplate::new(201).set_body_string("x".repeat(65)))
        .mount(&server)
        .await;
    let client = builder(&server)
        .max_response_body_bytes(64)
        .build()
        .unwrap();

    let response = client.post("/messages", &json!({})).await.unwrap();

    assert_eq!(response.status.as_u16(), 201);
    let unread = response
        .body_error
        .as_ref()
        .expect("the refusal is attached");
    assert!(unread.is_response_too_large());
    let decoded = response.json::<Value>().unwrap_err();
    assert!(decoded.is_response_too_large());
    assert_eq!(decoded.http_status(), Some(201));
}
