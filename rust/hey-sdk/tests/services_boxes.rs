mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::BoxKind;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[test]
fn a_box_kind_reads_back_from_the_name_hey_gives_it() {
    assert_eq!("asidebox".parse::<BoxKind>().unwrap(), BoxKind::SetAside);
    assert_eq!(BoxKind::SetAside.as_str(), "asidebox");
    assert_eq!(BoxKind::BubbleUp.to_string(), "bubblebox");

    let error = "nope".parse::<BoxKind>().unwrap_err();
    assert_eq!(error.code(), ErrorCode::Usage);
}

#[tokio::test]
async fn the_box_index_maps_every_kind_to_its_id() {
    let server = MockServer::start().await;
    mock_boxes(&server).await;

    let kinds = client(&server).boxes().kinds().await.unwrap();

    assert_eq!(kinds.id(BoxKind::Imbox).unwrap(), 24088);
    assert_eq!(kinds.id(BoxKind::Feed).unwrap(), 24089);
    assert_eq!(kinds.id(BoxKind::SetAside).unwrap(), 24090);
    assert_eq!(kinds.id(BoxKind::ReplyLater).unwrap(), 24091);
    assert_eq!(kinds.id(BoxKind::PaperTrail).unwrap(), 24092);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

/// The client reads the index once and answers every kind from that reading. `kinds` is the
/// read itself, so it still goes to HEY.
#[tokio::test]
async fn the_client_reads_the_index_once_and_answers_every_kind_from_it() {
    let server = MockServer::start().await;
    mock_boxes(&server).await;
    let boxes = client(&server);

    assert_eq!(
        boxes.boxes().id_by_kind(BoxKind::Imbox).await.unwrap(),
        24088
    );
    assert_eq!(
        boxes.boxes().id_by_kind(BoxKind::Feed).await.unwrap(),
        24089
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 1);

    boxes.boxes().kinds().await.unwrap();

    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn a_kind_the_account_has_no_box_for_is_an_api_error() {
    let server = MockServer::start().await;
    mock_boxes(&server).await;
    let boxes = client(&server);

    let error = boxes
        .boxes()
        .id_by_kind(BoxKind::BubbleUp)
        .await
        .unwrap_err();
    boxes
        .boxes()
        .id_by_kind(BoxKind::BubbleUp)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Api);
    assert_eq!(error.message(), r#"no box of kind "bubblebox""#);
    // The index was read to find that out, and asking again does not read it afresh.
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn a_set_aside_group_gathers_the_postings_it_is_made_from() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/boxes/5/groups.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "id": 11 })))
        .mount(&server)
        .await;

    let group = client(&server)
        .boxes()
        .create_box_group(5, &[1, 2])
        .await
        .unwrap();

    assert_eq!(group.id, 11);
    let requests = server.received_requests().await.unwrap();
    let body: Value = serde_json::from_slice(&requests[0].body).unwrap();
    assert_eq!(body, json!({ "posting_ids": [1, 2] }));
}

async fn mock_boxes(server: &MockServer) {
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([
            { "id": 24088, "kind": "imbox", "name": "Imbox" },
            { "id": 24089, "kind": "feedbox", "name": "The Feed" },
            { "id": 24090, "kind": "asidebox", "name": "Set Aside" },
            { "id": 24091, "kind": "laterbox", "name": "Reply Later" },
            { "id": 24092, "kind": "trailbox", "name": "Paper Trail" }
        ])))
        .mount(server)
        .await;
}
