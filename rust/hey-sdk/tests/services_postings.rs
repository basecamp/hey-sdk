mod support;

use std::sync::{Arc, Mutex};

use hey_sdk::observability::{Hooks, OperationInfo, OperationState};
use hey_sdk::services::{BubbleUpSlot, PostingChangesCursor};
use hey_sdk::{Date, ErrorCode};
use serde_json::{Value, json};
use wiremock::matchers::{method, path, query_param, query_param_is_missing};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{Outcomes, builder, client};

const CURSOR_URL: &str =
    "https://app.hey.com/boxes/24088/postings/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=2";

#[tokio::test]
async fn seen_and_unseen_carry_the_selection_in_the_body() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/seen.json").await;
    accepted(&server, "POST", "/postings/unseen.json").await;
    let postings = client(&server);

    postings
        .postings()
        .mark_postings_seen(&[1, 2])
        .await
        .unwrap();
    postings
        .postings()
        .mark_postings_unseen(&[3])
        .await
        .unwrap();

    let sent = sent(&server).await;
    assert_eq!(sent[0].0, "/postings/seen.json");
    assert_eq!(sent[0].1["posting_ids"], json!([1, 2]));
    assert_eq!(sent[1].0, "/postings/unseen.json");
    assert_eq!(sent[1].1["posting_ids"], json!([3]));
}

#[tokio::test]
async fn spam_mute_and_bubbling_up_now_carry_the_selection_in_the_body() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/spam.json").await;
    accepted(&server, "POST", "/postings/mutings.json").await;
    accepted(&server, "POST", "/postings/bulk_bubble_up_now.json").await;
    let postings = client(&server);

    postings.postings().mark_postings_spam(&[1]).await.unwrap();
    postings.postings().mute_postings(&[8, 9]).await.unwrap();
    postings
        .postings()
        .bubble_up_postings_now(&[11])
        .await
        .unwrap();

    let sent = sent(&server).await;
    assert_eq!(sent[0].0, "/postings/spam.json");
    assert_eq!(sent[0].1["posting_ids"], json!([1]));
    assert_eq!(sent[1].0, "/postings/mutings.json");
    assert_eq!(sent[1].1["posting_ids"], json!([8, 9]));
    assert_eq!(sent[2].0, "/postings/bulk_bubble_up_now.json");
    assert_eq!(sent[2].1["posting_ids"], json!([11]));
}

#[tokio::test]
async fn the_endpoints_without_a_body_carry_the_selection_comma_joined_in_the_query() {
    let server = MockServer::start().await;
    accepted(&server, "DELETE", "/postings/mutings.json").await;
    accepted(&server, "DELETE", "/postings/box_groups.json").await;
    accepted(&server, "DELETE", "/postings/bubble_up.json").await;
    let postings = client(&server);

    postings.postings().unmute_postings(&[8, 9]).await.unwrap();
    postings
        .postings()
        .remove_postings_from_box_group(&[8, 9])
        .await
        .unwrap();
    postings
        .postings()
        .cancel_postings_bubble_up(&[11, 12])
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("posting_ids=8%2C9"));
    assert_eq!(requests[1].url.query(), Some("posting_ids=8%2C9"));
    assert_eq!(requests[2].url.query(), Some("posting_ids=11%2C12"));
}

#[tokio::test]
async fn a_move_names_the_box_it_moves_to() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/moves.json").await;

    client(&server)
        .postings()
        .move_to_box(999, &[1, 2])
        .await
        .unwrap();

    let sent = sent(&server).await;
    assert_eq!(sent[0].0, "/postings/moves.json");
    assert_eq!(sent[0].1, json!({ "posting_ids": [1, 2], "box_id": 999 }));
}

#[tokio::test]
async fn a_move_by_kind_resolves_the_box_against_the_index() {
    let server = MockServer::start().await;
    mock_boxes(&server).await;
    accepted(&server, "POST", "/postings/moves.json").await;
    let postings = client(&server);

    postings.postings().move_to_imbox(&[5, 6]).await.unwrap();
    postings.postings().move_to_feed(&[5]).await.unwrap();
    postings.postings().move_to_set_aside(&[5]).await.unwrap();
    postings.postings().move_to_reply_later(&[5]).await.unwrap();
    postings.postings().move_to_paper_trail(&[5]).await.unwrap();

    let moves = sent(&server).await;
    let box_ids: Vec<&Value> = moves.iter().map(|(_, body)| &body["box_id"]).collect();
    assert_eq!(
        box_ids,
        vec![
            &json!(24088),
            &json!(24089),
            &json!(24090),
            &json!(24091),
            &json!(24092)
        ]
    );
    assert_eq!(moves[0].1["posting_ids"], json!([5, 6]));
}

/// The client reads the box index once and answers every kind from that reading, whichever
/// kind is asked for and however many times.
#[tokio::test]
async fn moving_by_kind_reads_the_box_index_once_for_the_life_of_the_client() {
    let server = MockServer::start().await;
    mock_boxes(&server).await;
    accepted(&server, "POST", "/postings/moves.json").await;
    let postings = client(&server);

    postings.postings().move_to_feed(&[1]).await.unwrap();
    postings.postings().move_to_feed(&[2]).await.unwrap();
    postings.postings().move_to_imbox(&[3]).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    let paths: Vec<&str> = requests.iter().map(|request| request.url.path()).collect();
    assert_eq!(
        paths,
        vec![
            "/boxes.json",
            "/postings/moves.json",
            "/postings/moves.json",
            "/postings/moves.json"
        ]
    );
}

#[tokio::test]
async fn a_kind_the_account_has_no_box_for_is_refused_after_the_index_is_read() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 24088, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let error = client(&server)
        .postings()
        .move_to_paper_trail(&[5])
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Api);
    assert_eq!(error.message(), r#"no box of kind "trailbox""#);
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn trashing_removes_only_your_own_access_unless_it_is_for_everyone() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/trash.json").await;
    let postings = client(&server);

    postings.postings().move_to_trash(&[7]).await.unwrap();
    postings.postings().trash_for_everyone(&[7]).await.unwrap();

    let sent = sent(&server).await;
    assert_eq!(sent[0].1, json!({ "posting_ids": [7] }));
    assert_eq!(
        sent[1].1,
        json!({ "posting_ids": [7], "remove_access": "false" })
    );
}

#[tokio::test]
async fn filing_names_the_folder_and_creating_one_names_it_by_name() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/filings.json").await;
    accepted(&server, "POST", "/postings/folders.json").await;
    let postings = client(&server);

    postings
        .postings()
        .file_postings(12, &[1, 2])
        .await
        .unwrap();
    postings
        .postings()
        .create_folder_for_postings("Receipts", &[1, 2])
        .await
        .unwrap();

    let sent = sent(&server).await;
    assert_eq!(sent[0].1, json!({ "posting_ids": [1, 2], "folder_id": 12 }));
    assert_eq!(
        sent[1].1,
        json!({ "posting_ids": [1, 2], "folder": { "name": "Receipts" } })
    );
}

/// Zero is not "every folder" to HEY, it is a folder that is not there, so unfiling from
/// every folder leaves the parameter out rather than sending it.
#[tokio::test]
async fn unfiling_from_every_folder_leaves_the_folder_out_of_the_query() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/postings/filings.json"))
        .and(query_param("folder_id", "12"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    Mock::given(method("DELETE"))
        .and(path("/postings/filings.json"))
        .and(query_param_is_missing("folder_id"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let postings = client(&server);

    postings
        .postings()
        .unfile_postings(12, &[1, 2])
        .await
        .unwrap();
    postings
        .postings()
        .unfile_postings(0, &[1, 2])
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.query(),
        Some("posting_ids=1%2C2&folder_id=12")
    );
    assert_eq!(requests[1].url.query(), Some("posting_ids=1%2C2"));
}

#[tokio::test]
async fn adding_to_a_set_aside_group_names_the_box_and_the_group() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/box_groups.json").await;

    client(&server)
        .postings()
        .add_postings_to_box_group(5, 11, &[1, 2])
        .await
        .unwrap();

    let sent = sent(&server).await;
    assert_eq!(
        sent[0].1,
        json!({ "posting_ids": [1, 2], "box_id": 5, "box_group_id": 11 })
    );
}

#[tokio::test]
async fn a_named_bubble_up_slot_carries_no_date_and_a_custom_one_carries_the_day() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/bubble_up.json").await;
    let postings = client(&server);
    let slots = [
        (BubbleUpSlot::LaterToday, "today"),
        (BubbleUpSlot::Tomorrow, "tomorrow"),
        (BubbleUpSlot::ThisWeekend, "weekend"),
        (BubbleUpSlot::NextWeek, "next_week"),
    ];

    for (slot, _) in slots {
        postings
            .postings()
            .schedule_postings_bubble_up(slot, &[11])
            .await
            .unwrap();
    }
    postings
        .postings()
        .schedule_postings_bubble_up(
            BubbleUpSlot::Custom(Date::new(2026, 9, 4).unwrap()),
            &[11, 12],
        )
        .await
        .unwrap();

    let sent = sent(&server).await;
    for (index, (_, name)) in slots.iter().enumerate() {
        assert_eq!(sent[index].1, json!({ "posting_ids": [11], "slot": name }));
    }
    assert_eq!(
        sent[4].1,
        json!({ "posting_ids": [11, 12], "slot": "custom", "date": "2026-09-04" })
    );
}

#[tokio::test]
async fn an_empty_selection_is_refused_before_anything_is_sent() {
    let server = MockServer::start().await;
    let postings = client(&server);

    for error in [
        postings.postings().mark_postings_seen(&[]).await,
        postings.postings().move_to_box(999, &[]).await,
        postings.postings().move_to_imbox(&[]).await,
        postings.postings().move_to_trash(&[]).await,
        postings.postings().unmute_postings(&[]).await,
        postings.postings().file_postings(12, &[]).await,
        postings.postings().unfile_postings(0, &[]).await,
        postings
            .postings()
            .create_folder_for_postings("R", &[])
            .await,
        postings
            .postings()
            .schedule_postings_bubble_up(BubbleUpSlot::Tomorrow, &[])
            .await,
    ] {
        let error = error.unwrap_err();
        assert_eq!(error.code(), ErrorCode::Usage);
        assert_eq!(error.message(), "at least one posting ID is required");
    }
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn a_selection_of_one_names_the_posting_it_acts_on() {
    let server = MockServer::start().await;
    accepted(&server, "POST", "/postings/seen.json").await;
    let announced = Arc::new(Announced::default());
    let postings = builder(&server)
        .hooks(Arc::clone(&announced))
        .build()
        .unwrap();

    postings.postings().mark_postings_seen(&[42]).await.unwrap();
    postings
        .postings()
        .mark_postings_seen(&[42, 43])
        .await
        .unwrap();

    assert_eq!(
        *announced.operations.lock().unwrap(),
        vec![
            ("MarkPostingsSeen".to_string(), Some(42)),
            ("MarkPostingsSeen".to_string(), None)
        ]
    );
}

#[test]
fn a_cursor_is_read_out_of_a_changes_url() {
    let cursor = PostingChangesCursor::from_url(&format!("{CURSOR_URL}&page=3")).unwrap();

    assert_eq!(cursor.since, "2026-08-18T09:00:00.000Z");
    assert_eq!(cursor.version.as_deref(), Some("2"));
    assert_eq!(cursor.page.as_deref(), Some("3"));
    assert_eq!(cursor.per_page, None);
}

#[tokio::test]
async fn a_changes_read_starts_from_the_cursor_and_answers_the_one_to_resume_from() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</boxes/24088/postings/changes.json?since=2026-08-18T09%3A14%3A22.031Z&v=2>; rel="next""#,
                )
                .set_body_json(json!({
                    "added": [{ "id": 9001, "kind": "topic", "box_id": 24088 }],
                    "updated": [{ "id": 9002, "kind": "topic", "box_id": 24088, "seen": true }],
                    "deleted": [{ "id": 9003, "box_id": 24088, "deleted_at": "2026-08-18T09:14:00.000Z" }]
                })),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .postings()
        .changes(24088, &cursor())
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.query(),
        Some("since=2026-08-18T09%3A00%3A00.000Z&v=2")
    );
    assert_eq!(changes.added.len(), 1);
    assert_eq!(changes.added[0].id, 9001);
    assert_eq!(changes.updated[0].seen, Some(true));
    assert_eq!(changes.deleted[0].id, 9003);
    assert_eq!(changes.next_page, None);
    assert_eq!(
        changes.next_cursor.unwrap().since,
        "2026-08-18T09:14:22.031Z"
    );
}

#[tokio::test]
async fn a_link_that_names_a_page_is_the_next_page_of_this_increment() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</boxes/24088/postings/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=2&page=2>; rel="next""#,
                )
                .set_body_json(json!({})),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .postings()
        .changes(24088, &cursor())
        .await
        .unwrap();

    assert_eq!(changes.next_page.unwrap().page.as_deref(), Some("2"));
    assert_eq!(changes.next_cursor, None);
}

#[tokio::test]
async fn a_changes_link_to_another_origin_is_refused_rather_than_followed() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"<https://evil.example.com/boxes/24088/postings/changes.json?since=2026-08-18T10%3A00%3A00.000Z>; rel="next""#,
                )
                .set_body_json(json!({})),
        )
        .mount(&server)
        .await;

    let error = client(&server)
        .postings()
        .changes(24088, &cursor())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert!(error.message().starts_with(
        "changes Link header points to a different origin: https://evil.example.com/"
    ));
}

#[tokio::test]
async fn a_cursor_without_a_since_is_refused_before_anything_is_sent() {
    let server = MockServer::start().await;

    let error = client(&server)
        .postings()
        .changes(24088, &PostingChangesCursor::default())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert_eq!(
        error.message(),
        "a since cursor is required — start from the box's posting_changes_url"
    );
    assert!(server.received_requests().await.unwrap().is_empty());
}

#[tokio::test]
async fn a_cursor_the_feed_has_left_behind_asks_for_a_full_sync_rather_than_failing() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .respond_with(ResponseTemplate::new(409))
        .mount(&server)
        .await;
    let outcomes = Arc::new(Outcomes::default());
    let postings = builder(&server).hooks(outcomes.clone()).build().unwrap();

    let changes = postings
        .postings()
        .all_changes(24088, &cursor())
        .await
        .unwrap();

    assert!(changes.full_sync_required);
    assert!(changes.added.is_empty());
    // The caller is told to sync in full; the hooks are told what HEY actually answered.
    assert_eq!(outcomes.statuses(), [Some(409)]);
}

#[tokio::test]
async fn all_changes_follows_the_pages_of_an_increment_and_keeps_the_last_cursor() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .and(query_param_is_missing("page"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</boxes/24088/postings/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=2&page=2>; rel="next""#,
                )
                .set_body_json(json!({ "added": [{ "id": 9001, "kind": "topic" }] })),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .and(query_param("page", "2"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</boxes/24088/postings/changes.json?since=2026-08-18T09%3A30%3A00.000Z&v=2>; rel="next""#,
                )
                .set_body_json(json!({ "added": [{ "id": 9002, "kind": "topic" }] })),
        )
        .mount(&server)
        .await;

    let changes = client(&server)
        .postings()
        .all_changes(24088, &cursor())
        .await
        .unwrap();

    assert_eq!(server.received_requests().await.unwrap().len(), 2);
    let added: Vec<i64> = changes.added.iter().map(|posting| posting.id).collect();
    assert_eq!(added, vec![9001, 9002]);
    assert_eq!(
        changes.next_cursor.unwrap().since,
        "2026-08-18T09:30:00.000Z"
    );
}

#[tokio::test]
async fn all_changes_stops_at_the_client_page_limit() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header(
                    "Link",
                    r#"</boxes/24088/postings/changes.json?since=2026-08-18T09%3A00%3A00.000Z&v=2&page=9>; rel="next""#,
                )
                .set_body_json(json!({ "added": [{ "id": 9001, "kind": "topic" }] })),
        )
        .mount(&server)
        .await;
    let capped = builder(&server).max_pages(2).build().unwrap();

    let changes = capped
        .postings()
        .all_changes(24088, &cursor())
        .await
        .unwrap();

    assert_eq!(server.received_requests().await.unwrap().len(), 2);
    assert_eq!(changes.added.len(), 2);
}

#[tokio::test]
async fn nothing_new_leaves_the_caller_the_cursor_it_already_holds() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes/24088/postings/changes.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
        .mount(&server)
        .await;

    let changes = client(&server)
        .postings()
        .all_changes(24088, &cursor())
        .await
        .unwrap();

    assert_eq!(changes, Default::default());
}

fn cursor() -> PostingChangesCursor {
    PostingChangesCursor::from_url(CURSOR_URL).unwrap()
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

async fn accepted(server: &MockServer, verb: &str, route: &str) {
    Mock::given(method(verb))
        .and(path(route))
        .respond_with(ResponseTemplate::new(204))
        .mount(server)
        .await;
}

/// The path and JSON body of every request that carried one, in the order they were sent.
async fn sent(server: &MockServer) -> Vec<(String, Value)> {
    server
        .received_requests()
        .await
        .unwrap()
        .iter()
        .filter(|request| !request.body.is_empty())
        .map(|request| {
            (
                request.url.path().to_string(),
                serde_json::from_slice(&request.body).unwrap(),
            )
        })
        .collect()
}

/// What each operation announced itself as, and the record it named.
#[derive(Default)]
struct Announced {
    operations: Mutex<Vec<(String, Option<i64>)>>,
}

impl Hooks for Announced {
    fn on_operation_start(&self, op: &OperationInfo) -> OperationState {
        self.operations
            .lock()
            .unwrap()
            .push((op.operation.to_string(), op.resource_id));
        None
    }
}
