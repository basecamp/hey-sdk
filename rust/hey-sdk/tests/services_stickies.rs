mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::{MAX_STICKY_POSITION, StickySize};
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

// Sending limit=0 would be read as one sticky rather than "no limit", and anything above
// 100 is clamped by the server anyway.
#[tokio::test]
async fn a_limit_is_left_off_when_it_is_zero_and_clamped_when_it_is_too_big() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/stickies.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 1, "body": "Call the plumber", "size": "medium" }])),
        )
        .mount(&server)
        .await;
    let client = client(&server);

    let stickies = client.stickies().list_up_to(0).await.unwrap();
    client.stickies().list_up_to(250).await.unwrap();

    assert_eq!(stickies[0].body.as_deref(), Some("Call the plumber"));
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), None);
    assert_eq!(requests[1].url.query(), Some("limit=100"));
}

#[tokio::test]
async fn a_new_sticky_carries_its_size_and_an_edit_leaves_out_what_it_was_not_given() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/stickies.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 1, "body": "Call the plumber", "size": "large" })),
        )
        .mount(&server)
        .await;
    Mock::given(method("PATCH"))
        .and(path("/stickies/1.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 1, "body": "Call the electrician" })),
        )
        .mount(&server)
        .await;
    let client = client(&server);

    let written = client
        .stickies()
        .create_sticky("Call the plumber", Some(StickySize::Large))
        .await
        .unwrap();
    let edited = client
        .stickies()
        .update_sticky(1, "Call the electrician", None)
        .await
        .unwrap();

    assert_eq!(written.size.as_deref(), Some("large"));
    assert_eq!(edited.body.as_deref(), Some("Call the electrician"));
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "sticky": { "body": "Call the plumber", "size": "large" } })
    );
    assert_eq!(
        sent_json(&server, 1).await,
        json!({ "sticky": { "body": "Call the electrician" } })
    );
}

#[tokio::test]
async fn a_move_names_the_sticky_and_where_it_lands() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/stickies/moves.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    client(&server).stickies().move_to(1, 2).await.unwrap();

    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "id": 1, "position": 2 })
    );
}

#[tokio::test]
async fn a_position_off_the_board_is_refused_before_the_round_trip() {
    let server = MockServer::start().await;
    let client = client(&server);

    for position in [-1, MAX_STICKY_POSITION + 1] {
        let error = client.stickies().move_to(1, position).await.unwrap_err();

        assert_eq!(error.code(), ErrorCode::Usage);
        assert_eq!(
            error.message(),
            format!("sticky position must be between 0 and {MAX_STICKY_POSITION}, got {position}")
        );
    }
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn a_size_reads_back_from_its_name() {
    assert_eq!("small".parse::<StickySize>().unwrap(), StickySize::Small);
    assert_eq!("medium".parse::<StickySize>().unwrap(), StickySize::Medium);
    assert_eq!("large".parse::<StickySize>().unwrap(), StickySize::Large);
    assert!("enormous".parse::<StickySize>().is_err());
}

async fn sent_json(server: &MockServer, index: usize) -> Value {
    let requests = server.received_requests().await.unwrap();
    serde_json::from_slice(&requests[index].body).unwrap()
}
