//! What the types promise at compile time: a client can be shared across tasks and
//! threads, an error can cross them, and the futures the client hands back can be spawned.
//! The future checks are never run — they only have to compile.

use hey_sdk::http::Method;
use hey_sdk::models::CreateMessageRequestContent;
use hey_sdk::observability::Hooks;
use hey_sdk::services::boxes::GetBoxParams;
use hey_sdk::{
    AuthStrategy, Client, ClientBuilder, Error, HttpClient, Page, Response, TokenProvider,
};
use serde_json::Value;

fn is_send_sync<T: Send + Sync>() {}
fn is_clone<T: Clone>() {}
fn is_send_future<F: Future + Send>(_: F) {}

#[test]
fn the_client_and_its_error_can_be_shared_and_sent() {
    is_send_sync::<Client>();
    is_clone::<Client>();
    is_send_sync::<ClientBuilder>();
    is_send_sync::<Error>();
    is_send_sync::<Response>();
    is_send_sync::<Page<Vec<Value>>>();
    is_send_sync::<Box<dyn Hooks>>();
    is_send_sync::<Box<dyn TokenProvider>>();
    is_send_sync::<Box<dyn AuthStrategy>>();
    is_send_sync::<Box<dyn HttpClient>>();
}

#[allow(dead_code)]
fn every_kind_of_call_is_a_future_that_can_be_spawned(client: &Client, page: Page<Vec<Value>>) {
    is_send_future(client.boxes().list());
    is_send_future(client.boxes().get(1, &GetBoxParams::default()));
    is_send_future(
        client
            .messages()
            .create(&CreateMessageRequestContent::default()),
    );
    is_send_future(client.get("/anything"));
    is_send_future(client.get_all("/anything"));
    is_send_future(client.execute(client.request(Method::GET, "/anything")));
    is_send_future(client.for_account(1));
    is_send_future(client.next_page(&page));
    is_send_future(client.each_page(page, |_| true));
}
