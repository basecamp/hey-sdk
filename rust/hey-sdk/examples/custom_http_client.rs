//! A transport of the caller's own.
//!
//! Everything the SDK sends goes out through one `HttpClient`. This example replaces the
//! shipped one with a transport that answers from a table, so it runs without the network
//! and without the `reqwest` feature:
//!
//! ```sh
//! cargo run --example custom_http_client --no-default-features
//! ```
//!
//! A real one hands the request to the platform's own HTTP stack and answers with its
//! status, headers and body. Two rules: never follow a redirect (the SDK does that itself,
//! and reads the `Location` a form request was made for), and enforce the timeout, since
//! the SDK cannot interrupt a transport it does not know.

use async_trait::async_trait;
use bytes::Bytes;
use hey_sdk::http::{Body, HttpClient, Request, Response, StatusCode, header};
use hey_sdk::{Client, Config, Error, StaticTokenProvider};

/// Answers every request with the same page of boxes.
struct Canned;

#[async_trait]
impl HttpClient for Canned {
    async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, Error> {
        eprintln!("{} {}", request.method(), request.uri());
        let body = r#"[{"id": 1, "kind": "imbox", "name": "Imbox"},
                       {"id": 2, "kind": "feed", "name": "The Feed"}]"#;
        Response::builder()
            .status(StatusCode::OK)
            .header(header::CONTENT_TYPE, "application/json")
            .header("X-Request-Id", "canned")
            .body(Body::from(Bytes::from_static(body.as_bytes())))
            .map_err(Error::network)
    }
}

#[tokio::main]
async fn main() -> Result<(), Error> {
    let client = Client::builder(Config::default())
        .token_provider(StaticTokenProvider::new("not-a-real-token"))
        .http_client(Canned)
        .build()?;

    for mailbox in client.boxes().list().await?.iter() {
        println!("{:>4}  {:<8} {}", mailbox.id, mailbox.kind, mailbox.name);
    }
    Ok(())
}
