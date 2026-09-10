mod support;

use std::time::{Duration, Instant};

use chrono::Utc;
use hey_sdk::http::Method;
use hey_sdk::routes::{self, Pagination, Retry, Route};
use hey_sdk::{Client, Config, StaticTokenProvider};
use serde_json::{Value, json};
use wiremock::matchers::{method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{builder, client};

/// A route of the test's own, with whatever policy the case needs: the model only ever
/// says `max: 3` or `max: 2` on `[429, 503]`, and the client has to be right about the
/// rest of what a policy can say.
const fn route(retry: Retry) -> Route {
    Route {
        id: "ReadThing",
        service: "Things",
        method: Method::GET,
        path: "/things",
        pattern: "/things",
        resource: "Things",
        resource_type: "thing",
        params: &[],
        idempotent: true,
        readonly: true,
        html: false,
        empty_on: &[],
        pagination: Pagination::None,
        retry,
    }
}

static ONLY_ON_503: Route = route(Retry {
    max: 3,
    base_delay_ms: 10,
    retry_on: &[503],
});

static NO_POLICY: Route = route(Retry {
    max: 0,
    base_delay_ms: 1000,
    retry_on: &[],
});

static SLOW_TO_RESEND: Route = route(Retry {
    max: 2,
    base_delay_ms: 300,
    retry_on: &[503],
});

async fn always(server: &MockServer, verb: &str, route: &str, response: ResponseTemplate) {
    Mock::given(method(verb))
        .and(path(route))
        .respond_with(response)
        .mount(server)
        .await;
}

async fn first(server: &MockServer, route: &str, times: u64, response: ResponseTemplate) {
    Mock::given(method("GET"))
        .and(path(route))
        .respond_with(response)
        .up_to_n_times(times)
        .mount(server)
        .await;
}

async fn requests(server: &MockServer) -> usize {
    server.received_requests().await.unwrap().len()
}

/// `ListBoxes` is modelled with `max: 3`: three sends in all, whatever the client would
/// allow beyond that.
#[tokio::test]
async fn a_route_is_sent_as_many_times_as_its_policy_allows() {
    let server = MockServer::start().await;
    always(&server, "GET", "/boxes.json", ResponseTemplate::new(503)).await;

    let error = builder(&server)
        .max_retries(10)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(503));
    assert_eq!(requests(&server).await, 3);
}

#[tokio::test]
async fn the_client_setting_only_lowers_what_the_policy_allows() {
    let server = MockServer::start().await;
    always(&server, "GET", "/boxes.json", ResponseTemplate::new(503)).await;

    builder(&server)
        .max_retries(1)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();
    assert_eq!(requests(&server).await, 2);

    server.reset().await;
    always(&server, "GET", "/boxes.json", ResponseTemplate::new(503)).await;
    builder(&server)
        .max_retries(0)
        .build()
        .unwrap()
        .boxes()
        .list()
        .await
        .unwrap_err();
    assert_eq!(requests(&server).await, 1);
}

/// `DeleteBoxDesignation` is modelled with `max: 2`, one fewer than a read.
#[tokio::test]
async fn each_route_brings_its_own_count() {
    let server = MockServer::start().await;
    always(
        &server,
        "DELETE",
        "/boxes/12345/designations/67890.json",
        ResponseTemplate::new(503),
    )
    .await;

    client(&server)
        .designations()
        .delete(12345, 67890)
        .await
        .unwrap_err();

    assert_eq!(requests(&server).await, 2);
}

/// The model names 429 and 503. A 500 is not on the list, so it is the answer.
#[tokio::test]
async fn a_status_the_policy_does_not_name_is_not_resent() {
    let server = MockServer::start().await;
    always(&server, "GET", "/boxes.json", ResponseTemplate::new(500)).await;

    let error = client(&server).boxes().list().await.unwrap_err();

    assert_eq!(error.http_status(), Some(500));
    assert_eq!(requests(&server).await, 1);
}

/// The same 500 on a path the caller wrote is resent: no policy covers it, so the
/// client's own list of transient statuses does.
#[tokio::test]
async fn a_path_the_caller_wrote_runs_on_the_client_settings() {
    let server = MockServer::start().await;
    always(&server, "GET", "/custom", ResponseTemplate::new(500)).await;

    builder(&server)
        .max_retries(1)
        .build()
        .unwrap()
        .get("/custom")
        .await
        .unwrap_err();

    assert_eq!(requests(&server).await, 2);
}

#[tokio::test]
async fn a_rate_limit_the_policy_does_not_name_is_not_waited_out() {
    let server = MockServer::start().await;
    always(
        &server,
        "GET",
        "/things.json",
        ResponseTemplate::new(429).insert_header("Retry-After", "5"),
    )
    .await;
    let client = client(&server);

    let started = Instant::now();
    let error = client
        .send_unit(client.operation(&ONLY_ON_503, &[]))
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(429));
    assert_eq!(requests(&server).await, 1);
    assert!(started.elapsed() < Duration::from_secs(1));
}

#[tokio::test]
async fn a_route_the_model_gives_no_policy_is_sent_once() {
    let server = MockServer::start().await;
    always(&server, "GET", "/things.json", ResponseTemplate::new(503)).await;
    let client = builder(&server).max_retries(5).build().unwrap();

    client
        .send_unit(client.operation(&NO_POLICY, &[]))
        .await
        .unwrap_err();

    assert_eq!(requests(&server).await, 1);
}

#[tokio::test]
async fn a_mutation_is_sent_once_whatever_its_policy_says() {
    let server = MockServer::start().await;
    always(&server, "GET", "/things.json", ResponseTemplate::new(503)).await;
    let client = client(&server);

    let mut operation = client.operation(&ONLY_ON_503, &[]);
    operation.idempotent(false);
    client.send_unit(operation).await.unwrap_err();

    assert_eq!(requests(&server).await, 1);
}

fn unpaced(server: &MockServer) -> hey_sdk::ClientBuilder {
    Client::builder(Config::default().with_base_url(server.uri()))
        .token_provider(StaticTokenProvider::new("t"))
        .max_jitter(Duration::ZERO)
}

/// The first wait is the policy's, unless the client asked for a longer one.
#[tokio::test]
async fn the_first_wait_is_the_policy_s_and_the_client_can_only_lengthen_it() {
    let server = MockServer::start().await;
    first(&server, "/things.json", 1, ResponseTemplate::new(503)).await;
    always(&server, "GET", "/things.json", ResponseTemplate::new(200)).await;
    let client = unpaced(&server).build().unwrap();

    let started = Instant::now();
    client
        .send_unit(client.operation(&SLOW_TO_RESEND, &[]))
        .await
        .unwrap();
    let waited = started.elapsed();

    assert_eq!(requests(&server).await, 2);
    assert!(
        (Duration::from_millis(300)..Duration::from_millis(900)).contains(&waited),
        "waited {waited:?}"
    );

    server.reset().await;
    first(&server, "/things.json", 1, ResponseTemplate::new(503)).await;
    always(&server, "GET", "/things.json", ResponseTemplate::new(200)).await;
    let client = unpaced(&server)
        .base_delay(Duration::from_millis(600))
        .build()
        .unwrap();

    let started = Instant::now();
    client
        .send_unit(client.operation(&SLOW_TO_RESEND, &[]))
        .await
        .unwrap();
    let waited = started.elapsed();

    assert!(waited >= Duration::from_millis(600), "waited {waited:?}");
}

/// A shorter wait than the policy's is not a floor the client can set, but the ceiling
/// still holds: a test suite that winds the backoff down keeps its speed.
#[tokio::test]
async fn the_longest_wait_holds_the_policy_s_wait_down() {
    let server = MockServer::start().await;
    first(&server, "/things.json", 1, ResponseTemplate::new(503)).await;
    always(&server, "GET", "/things.json", ResponseTemplate::new(200)).await;
    let client = unpaced(&server)
        .max_delay(Duration::from_millis(20))
        .build()
        .unwrap();

    let started = Instant::now();
    client
        .send_unit(client.operation(&SLOW_TO_RESEND, &[]))
        .await
        .unwrap();

    assert!(started.elapsed() < Duration::from_millis(250));
}

#[tokio::test]
async fn a_rate_limit_the_policy_names_is_waited_out_as_the_date_asks() {
    let server = MockServer::start().await;
    let in_two_seconds = (Utc::now() + chrono::Duration::seconds(2))
        .format("%a, %d %b %Y %H:%M:%S GMT")
        .to_string();
    first(
        &server,
        "/boxes.json",
        1,
        ResponseTemplate::new(429).insert_header("Retry-After", in_two_seconds.as_str()),
    )
    .await;
    always(
        &server,
        "GET",
        "/boxes.json",
        ResponseTemplate::new(200).set_body_json(json!([])),
    )
    .await;

    let started = Instant::now();
    client(&server).boxes().list().await.unwrap();
    let waited = started.elapsed();

    assert_eq!(requests(&server).await, 2);
    assert!(
        waited >= Duration::from_secs(1),
        "resent after only {waited:?}"
    );
}

/// The pages after the first are read under the policy of the route the first came from,
/// not as anonymous requests with the client's defaults.
#[tokio::test]
async fn a_later_page_is_resent_under_the_first_page_s_policy() {
    let server = MockServer::start().await;
    let boxes: Value = json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }]);
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("page", "2"))
        .respond_with(ResponseTemplate::new(503))
        .up_to_n_times(2)
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("page", "2"))
        .respond_with(ResponseTemplate::new(200).set_body_json(&boxes))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=2>; rel="next""#)
                .set_body_json(&boxes),
        )
        .mount(&server)
        .await;

    let client = client(&server);
    let page = client.boxes().list().await.unwrap();
    let next = client.next_page(&page).await.unwrap().unwrap();
    assert_eq!(next.len(), 1);
    assert_eq!(requests(&server).await, 4);

    server.reset().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("page", "2"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("Link", r#"</boxes.json?page=2>; rel="next""#)
                .set_body_json(&boxes),
        )
        .mount(&server)
        .await;
    let client = builder(&server).max_retries(1).build().unwrap();
    let page = client.boxes().list().await.unwrap();
    let error = client.next_page(&page).await.unwrap_err();

    assert_eq!(error.http_status(), Some(503));
    assert_eq!(requests(&server).await, 3);
}

/// The policy on the wire is the model's, read off the route table rather than written
/// into the client, so regenerating from a changed model is what changes behaviour.
#[test]
fn the_policy_on_the_wire_is_the_route_table_s() {
    assert_eq!(routes::LIST_BOXES.retry.max, 3);
    assert_eq!(routes::DELETE_BOX_DESIGNATION.retry.max, 2);
    assert_eq!(routes::LIST_BOXES.retry.retry_on, &[429, 503]);
    assert_eq!(routes::CREATE_BULK_REPLY.retry.max, 0);
}

/// The ceiling is a ceiling: a floor set above it does not lift it, and the jitter goes
/// under it too.
#[tokio::test]
async fn the_longest_wait_holds_the_client_s_own_floor_and_jitter_down() {
    let server = MockServer::start().await;
    first(&server, "/boxes.json", 2, ResponseTemplate::new(503)).await;
    always(
        &server,
        "GET",
        "/boxes.json",
        ResponseTemplate::new(200).set_body_json(json!([])),
    )
    .await;
    let client = Client::builder(Config::default().with_base_url(server.uri()))
        .token_provider(StaticTokenProvider::new("t"))
        .base_delay(Duration::from_millis(600))
        .max_jitter(Duration::from_millis(600))
        .max_delay(Duration::from_millis(20))
        .build()
        .unwrap();

    let started = Instant::now();
    client.boxes().list().await.unwrap();

    assert_eq!(requests(&server).await, 3);
    assert!(started.elapsed() < Duration::from_millis(300));
}

/// The wait HEY asked for is not shortened by the ceiling: resending sooner only earns
/// another refusal.
#[tokio::test]
async fn the_wait_hey_asked_for_is_not_held_down() {
    let server = MockServer::start().await;
    first(
        &server,
        "/boxes.json",
        1,
        ResponseTemplate::new(429).insert_header("Retry-After", "1"),
    )
    .await;
    always(
        &server,
        "GET",
        "/boxes.json",
        ResponseTemplate::new(200).set_body_json(json!([])),
    )
    .await;

    let started = Instant::now();
    client(&server).boxes().list().await.unwrap();

    assert!(started.elapsed() >= Duration::from_secs(1));
}
