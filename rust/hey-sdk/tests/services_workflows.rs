//! Wire-level tests for the `workflows` service: what each call sends and how it reads what HEY answers, against literal bodies.

mod support;

use std::sync::Arc;

use hey_sdk::ErrorCode;
use hey_sdk::services::{WorkflowStageTopic, WorkflowStageView};
use serde_json::json;
use wiremock::matchers::{header, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{Operations, builder, client};

/// The autocomplete endpoint answers bare rows, and pads them with entries it has nothing
/// to say about: a row with no name, or whose first column is no id, names no workflow.
#[tokio::test]
async fn the_workflow_list_reads_the_rows_the_autocomplete_endpoint_answers() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/autocompletable/accounts/77/workflows"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!([
            ["8801", "Hiring", "Example Co"],
            ["8802", "Sales pipeline"],
            ["not-an-id", "Ignored", "Example Co"],
            ["8804"],
            []
        ])))
        .mount(&server)
        .await;

    let workflows = client(&server).workflows().list(77).await.unwrap();

    let listed: Vec<(i64, &str, &str)> = workflows
        .iter()
        .map(|workflow| {
            (
                workflow.id,
                workflow.name.as_str(),
                workflow.account_name.as_str(),
            )
        })
        .collect();
    assert_eq!(
        listed,
        [(8801, "Hiring", "Example Co"), (8802, "Sales pipeline", "")]
    );
    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.path(),
        "/autocompletable/accounts/77/workflows"
    );
}

#[tokio::test]
async fn a_workflows_stages_come_off_the_workflow_itself() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/workflows/8801.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 8801,
            "name": "Hiring",
            "stages": [{ "id": 5512, "name": "Applied" }, { "id": 5513, "name": "Interviewing" }]
        })))
        .mount(&server)
        .await;

    let stages = client(&server).workflows().stages(8801).await.unwrap();

    assert_eq!(stages.len(), 2);
    assert_eq!(stages[0].id, 5512);
    assert_eq!(stages[0].name.as_deref(), Some("Applied"));
    assert_eq!(stages[1].name.as_deref(), Some("Interviewing"));
}

#[tokio::test]
async fn a_workflow_is_made_on_the_account_it_names() {
    let server = MockServer::start().await;
    mock_form(&server, "POST", "/workflows").await;

    client(&server)
        .workflows()
        .create("Hiring", Some(77))
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/workflows");
    assert_eq!(
        form(&requests[0].body),
        [
            ("workflow[name]".to_string(), "Hiring".to_string()),
            ("account_id".to_string(), "77".to_string()),
        ]
    );
}

#[tokio::test]
async fn a_workflow_made_without_an_account_leaves_hey_to_pick_one() {
    let server = MockServer::start().await;
    mock_form(&server, "POST", "/workflows").await;
    let client = client(&server);

    client.workflows().create("Hiring", None).await.unwrap();
    client.workflows().create("Hiring", Some(0)).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    for request in &requests {
        assert_eq!(
            form(&request.body),
            [("workflow[name]".to_string(), "Hiring".to_string())]
        );
    }
}

#[tokio::test]
async fn renaming_and_throwing_away_a_workflow() {
    let server = MockServer::start().await;
    mock_form(&server, "PATCH", "/workflows/8801").await;
    mock_form(&server, "DELETE", "/workflows/8801").await;
    let client = client(&server);

    client.workflows().update(8801, "Recruiting").await.unwrap();
    client.workflows().delete(8801).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/workflows/8801");
    assert_eq!(
        form(&requests[0].body),
        [("workflow[name]".to_string(), "Recruiting".to_string())]
    );
    assert_eq!(requests[1].url.path(), "/workflows/8801");
    assert!(requests[1].body.is_empty());
}

/// A new column is posted with nothing in it — HEY names it "Untitled" — so the request
/// still carries a form body, empty.
#[tokio::test]
async fn a_new_stage_is_posted_with_an_empty_form() {
    let server = MockServer::start().await;
    mock_form(&server, "POST", "/workflows/8801/stages").await;

    client(&server)
        .workflows()
        .create_stage(8801)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/workflows/8801/stages");
    assert_eq!(
        requests[0].headers["content-type"],
        "application/x-www-form-urlencoded"
    );
    assert!(requests[0].body.is_empty());
}

#[tokio::test]
async fn renaming_and_removing_a_stage() {
    let server = MockServer::start().await;
    mock_form(&server, "PATCH", "/workflows/8801/stages/5512").await;
    mock_form(&server, "DELETE", "/workflows/8801/stages/5512").await;
    let client = client(&server);

    client
        .workflows()
        .update_stage(8801, 5512, "Applied")
        .await
        .unwrap();
    client.workflows().delete_stage(8801, 5512).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/workflows/8801/stages/5512");
    assert_eq!(
        form(&requests[0].body),
        [("workflow_stage[name]".to_string(), "Applied".to_string())]
    );
    assert!(requests[1].body.is_empty());
}

/// HEY files the topic before it selects the stage, so staging is two requests: the
/// membership, then the move.
#[tokio::test]
async fn staging_a_topic_files_it_then_moves_it_to_the_stage_asked_for() {
    let server = MockServer::start().await;
    mock_staging(&server, "POST").await;
    mock_staging(&server, "PATCH").await;
    let operations = Arc::new(Operations::default());

    builder(&server)
        .hooks(operations.clone())
        .build()
        .unwrap()
        .workflows()
        .stage_topic(4_471_829, 8801, 5512)
        .await
        .unwrap();

    // The stage selection is quiet, so this is one operation however many requests it takes.
    assert_eq!(operations.started(), ["Workflows.CreateWorkflowStaging"]);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert_eq!(requests[0].method.as_str(), "POST");
    assert_eq!(
        requests[0].url.path(),
        "/topics/4471829/workflows/8801/stagings"
    );
    assert_eq!(requests[0].headers["accept"], "*/*");
    assert!(requests[0].body.is_empty());
    assert_eq!(requests[1].method.as_str(), "PATCH");
    assert_eq!(
        requests[1].url.path(),
        "/topics/4471829/workflows/8801/stagings"
    );
    assert_eq!(
        form(&requests[1].body),
        [(
            "workflow_staging[workflow_stage_id]".to_string(),
            "5512".to_string()
        )]
    );
}

/// The stage selection is a second request, so its refusal surfaces — and the topic is left
/// in the workflow's first stage.
#[tokio::test]
async fn a_stage_that_will_not_take_the_topic_surfaces_after_the_topic_is_filed() {
    let server = MockServer::start().await;
    mock_staging(&server, "POST").await;
    Mock::given(method("PATCH"))
        .and(path("/topics/4471829/workflows/8801/stagings"))
        .respond_with(ResponseTemplate::new(422))
        .mount(&server)
        .await;

    let error = client(&server)
        .workflows()
        .stage_topic(4_471_829, 8801, 9999)
        .await
        .unwrap_err();

    assert_eq!(error.http_status(), Some(422));
    assert_eq!(server.received_requests().await.unwrap().len(), 2);
}

#[tokio::test]
async fn moving_a_staged_topic_sends_the_stage_as_a_form() {
    let server = MockServer::start().await;
    mock_staging(&server, "PATCH").await;

    client(&server)
        .workflows()
        .move_topic_to_stage(4_471_829, 8801, 5513)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.path(),
        "/topics/4471829/workflows/8801/stagings"
    );
    assert_eq!(requests[0].headers["accept"], "*/*");
    assert_eq!(
        form(&requests[0].body),
        [(
            "workflow_staging[workflow_stage_id]".to_string(),
            "5513".to_string()
        )]
    );
}

#[tokio::test]
async fn unstaging_a_topic_takes_it_off_the_workflow() {
    let server = MockServer::start().await;
    mock_form(&server, "DELETE", "/topics/4471829/workflows/8801/stagings").await;

    client(&server)
        .workflows()
        .unstage_topic(4_471_829, 8801)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.path(),
        "/topics/4471829/workflows/8801/stagings"
    );
    assert!(requests[0].body.is_empty());
}

async fn mock_form(server: &MockServer, verb: &str, at: &str) {
    Mock::given(method(verb))
        .and(path(at.to_string()))
        .respond_with(ResponseTemplate::new(302).insert_header("Location", at))
        .mount(server)
        .await;
}

/// The two staging requests are modelled routes rather than form endpoints, so HEY answers
/// them with a status rather than a redirect.
async fn mock_staging(server: &MockServer, verb: &str) {
    Mock::given(method(verb))
        .and(path("/topics/4471829/workflows/8801/stagings"))
        .respond_with(ResponseTemplate::new(204))
        .mount(server)
        .await;
}

fn form(body: &[u8]) -> Vec<(String, String)> {
    url::form_urlencoded::parse(body).into_owned().collect()
}

/// The stage page as HEY renders it, read the way Go's `GetStage` reads it: the stage's
/// name without its screen-reader suffix, a card with its subject and count, a sparse
/// card with neither, and two cards whose ids will not parse left out.
#[tokio::test]
async fn a_stage_is_read_out_of_the_page_hey_serves_for_it() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/workflows/8801/stages/5512"))
        .and(header("accept", "text/html"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/html; charset=utf-8")
                .set_body_string(
                    r#"<section id="container_workflow_stage_5512"><h2>Applied<span class="u-for-screen-reader"> stage</span></h2><div class="workflow__card" id="topic_4471829" data-identifier="91"><h3>Application<span class="u-for-screen-reader">Thread: Application</span></h3><p class="card__detail"><span class="u-for-screen-reader">,</span>3 emails</p></div><div class="workflow__card" id="topic_4471830" data-identifier="92"></div><div id="topic_not-a-number" data-identifier="93"></div><div id="topic_4471831" data-identifier="not-a-number"></div></section>"#,
                ),
        )
        .mount(&server)
        .await;

    let stage = client(&server).workflows().stage(8801, 5512).await.unwrap();

    assert_eq!(stage.id, 5512);
    assert_eq!(stage.name, "Applied");
    assert_eq!(
        stage.topics,
        [
            WorkflowStageTopic {
                staging_id: 91,
                topic_id: 4_471_829,
                subject: "Application".to_string(),
                entry_count: 3,
            },
            WorkflowStageTopic {
                staging_id: 92,
                topic_id: 4_471_830,
                subject: String::new(),
                entry_count: 0,
            },
        ]
    );
}

#[tokio::test]
async fn a_page_without_the_stage_is_not_found() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/workflows/8801/stages/5512"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/html")
                .set_body_string(r#"<section id="container_workflow_stage_9999"></section>"#),
        )
        .mount(&server)
        .await;

    let error = client(&server)
        .workflows()
        .stage(8801, 5512)
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::NotFound);
    assert!(error.message().contains("5512"), "{error}");
}

/// A card whose detail line starts with something other than a count is left out, as Go
/// leaves it out; a card inside a card is that card's content, not a card of its own; a
/// card is found by a detail class that merely mentions `card__detail`, and at the element
/// itself; what wraps the stage is not a card; and the count is read as Go reads it.
#[test]
fn the_parser_keeps_to_the_cards_go_keeps_to() {
    let page = r#"<html><body><nav>Boards</nav><div id="topic_shell">
      <section id="container_workflow_stage_7"><h2>  In  <b>review</b> </h2>
        <div id="topic_1" data-identifier="10"><h3>One</h3><p class="card__detail">1 email</p>
          <div id="topic_2" data-identifier="20"><h3>Nested</h3></div></div>
        <div id="topic_3" data-identifier="30"><h3>Three</h3><p class="card__detail">many emails</p></div>
        <div id="topic_4" data-identifier="40"><h3>Four</h3><p class="card__detail"></p></div>
        <div id="topic_0" data-identifier="50"><h3>Zero</h3></div>
        <div id="topic_5" data-identifier="0"><h3>Unstaged</h3></div>
        <div id="topic_6" data-identifier="60"><h3>Six</h3><p class="card__detail--compact">6 emails</p></div>
        <h3 id="topic_7" data-identifier="70">Seven</h3>
        <div id="topic_8" data-identifier="80"><h3>Eight</h3><p class="card__detail">-1 emails</p></div>
        <div id="topic_9" data-identifier="90"><h3><span class="sr-only&#160;x">Hidden</span>Nine</h3><p class="card__detail">-0 emails</p></div>
      </section></div></body></html>"#;

    let stage = WorkflowStageView::parse(page, 7).unwrap();

    assert_eq!(stage.name, "In review");
    let topics: Vec<(i64, i64, &str, u64)> = stage
        .topics
        .iter()
        .map(|topic| {
            (
                topic.topic_id,
                topic.staging_id,
                topic.subject.as_str(),
                topic.entry_count,
            )
        })
        .collect();
    assert_eq!(
        topics,
        [
            (1, 10, "One", 1),
            (6, 60, "Six", 6),
            (7, 70, "Seven", 0),
            (9, 90, "Nine", 0)
        ]
    );
}

/// A stage element that is itself the heading names the stage, as Go's inclusive search
/// finds it, and a page nested past any sane depth is read without recursing into it.
#[test]
fn the_parser_reads_the_element_itself_and_survives_deep_nesting() {
    let page = r#"<h2 id="container_workflow_stage_3">Ready</h2>"#;
    assert_eq!(WorkflowStageView::parse(page, 3).unwrap().name, "Ready");

    let deep = format!(
        r#"<section id="container_workflow_stage_4"><h2>{}Deep{}</h2></section>"#,
        "<span>".repeat(200_000),
        "</span>".repeat(200_000)
    );
    assert_eq!(WorkflowStageView::parse(&deep, 4).unwrap().name, "Deep");
}
