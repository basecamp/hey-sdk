mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::{ClearanceStatus, ScreenOptions};
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

// The apps sync this read for the Screener badge, so the count must not drag the queue
// along with it.
#[tokio::test]
async fn the_pending_count_asks_for_the_count_alone() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/clearances.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(
            json!({ "pending_clearances_count": 3, "signed_stream_name": "eyJpZGVudGl0eSI6MX0--sig" }),
        ))
        .mount(&server)
        .await;
    let client = client(&server);

    let count = client.clearances().pending_count().await.unwrap();
    let summary = client.clearances().summary().await.unwrap();

    assert_eq!(count, 3);
    assert_eq!(
        summary.signed_stream_name.as_deref(),
        Some("eyJpZGVudGl0eSI6MX0--sig")
    );
    assert!(summary.clearances.is_none());
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), None);
    assert_eq!(requests[1].url.query(), None);
}

#[tokio::test]
async fn the_queue_is_asked_for_by_name_and_walked_by_its_cursor() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/clearances.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    "<https://app.hey.com/clearances.json?page=eyJwYWdlIjozfQ>; rel=\"next\"",
                )
                .set_body_json(json!({
                    "pending_clearances_count": 24,
                    "clearances": [{
                        "id": 91,
                        "status": "pending",
                        "petitioner": { "id": 51, "name": "Hollis Heimboch" }
                    }]
                })),
        )
        .mount(&server)
        .await;

    let page = client(&server)
        .clearances()
        .pending_page(Some("eyJwYWdlIjoyfQ"))
        .await
        .unwrap();

    assert_eq!(page.pending_clearances_count, Some(24));
    let clearances = page.clearances.as_ref().unwrap();
    assert_eq!(clearances[0].id, 91);
    assert_eq!(
        clearances[0].petitioner.as_ref().unwrap().name.as_deref(),
        Some("Hollis Heimboch")
    );
    assert_eq!(page.next_page(), Some("eyJwYWdlIjozfQ"));
    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.query(),
        Some("include_clearances=true&page=eyJwYWdlIjoyfQ")
    );
}

#[tokio::test]
async fn screening_sends_only_the_options_that_were_asked_for() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/clearances/91.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "id": 91, "status": "approved" })),
        )
        .mount(&server)
        .await;
    let options = ScreenOptions {
        designation_box_id: Some(7),
        mark_topics_as_seen: true,
        spam: false,
    };

    let clearance = client(&server)
        .clearances()
        .screen(91, ClearanceStatus::Approved, &options)
        .await
        .unwrap();

    assert_eq!(clearance.status.as_deref(), Some("approved"));
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "status": "approved", "designation_box_id": 7, "mark_topics_as_seen": true })
    );
}

#[tokio::test]
async fn screening_many_senders_names_them_in_one_list() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/clearances/bulk.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "clearances": [{ "id": 91, "status": "denied" }, { "id": 92, "status": "denied" }]
        })))
        .mount(&server)
        .await;

    let clearances = client(&server)
        .clearances()
        .screen_many(&[91, 92], ClearanceStatus::Denied, true)
        .await
        .unwrap();

    assert_eq!(clearances.len(), 2);
    assert_eq!(clearances[1].id, 92);
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "ids": "91,92", "status": "denied", "spam": true })
    );
}

#[tokio::test]
async fn screening_nobody_is_refused_before_the_round_trip() {
    let server = MockServer::start().await;

    let error = client(&server)
        .clearances()
        .screen_many(&[], ClearanceStatus::Denied, false)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert_eq!(error.message(), "at least one clearance is required");
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn the_decided_list_reads_from_the_identitys_own_clearances() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/my/clearances.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "clearances": [
                { "id": 91, "status": "approved", "petitioner": { "id": 51, "name": "Glenn" } },
                { "id": 92, "status": "denied", "petitioner": { "id": 52, "name": "Spammer" } }
            ]
        })))
        .mount(&server)
        .await;

    let clearances = client(&server).clearances().screened(None).await.unwrap();

    assert_eq!(clearances.len(), 2);
    assert_eq!(clearances[0].status.as_deref(), Some("approved"));
    assert_eq!(clearances[1].status.as_deref(), Some("denied"));
    assert_eq!(
        server.received_requests().await.unwrap()[0].url.query(),
        None
    );
}

// HEY takes the status flat here, not nested under a clearance key like its own form.
#[tokio::test]
async fn rescreening_changes_a_decision_already_taken() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/my/clearances/91.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "id": 91, "status": "denied" })),
        )
        .mount(&server)
        .await;

    let clearance = client(&server)
        .clearances()
        .rescreen(91, ClearanceStatus::Denied)
        .await
        .unwrap();

    assert_eq!(clearance.status.as_deref(), Some("denied"));
    assert_eq!(sent_json(&server, 0).await, json!({ "status": "denied" }));
}

#[tokio::test]
async fn a_decision_reads_back_from_its_name() {
    assert_eq!(
        "approved".parse::<ClearanceStatus>().unwrap(),
        ClearanceStatus::Approved
    );
    assert_eq!(
        "denied".parse::<ClearanceStatus>().unwrap(),
        ClearanceStatus::Denied
    );

    let error = "maybe".parse::<ClearanceStatus>().unwrap_err();

    assert_eq!(error.code(), ErrorCode::Validation);
    assert_eq!(
        error.message(),
        "clearance status must be \"approved\" or \"denied\", got \"maybe\""
    );
}

async fn sent_json(server: &MockServer, index: usize) -> Value {
    let requests = server.received_requests().await.unwrap();
    serde_json::from_slice(&requests[index].body).unwrap()
}
