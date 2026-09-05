mod support;

use hey_sdk::ErrorCode;
use hey_sdk::services::{ClearanceStatus, ContactConflict, ContactParams};
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn a_scoped_client_files_a_new_contact_under_its_own_account() {
    let server = MockServer::start().await;
    mock_identity(&server).await;
    Mock::given(method("POST"))
        .and(path("/contacts.json"))
        .respond_with(ResponseTemplate::new(201).set_body_json(json!({ "id": 91824 })))
        .mount(&server)
        .await;
    let scoped = client(&server).for_account(42).await.unwrap();

    let contact = scoped.contacts().create_contact(&jane()).await.unwrap();

    assert_eq!(contact.id, 91824);
    let body = sent_json(&server, 1).await;
    assert_eq!(body["acting_user_id"], 4849);
    assert_eq!(
        body["contact"],
        json!({
            "name": "Jane Dawson",
            "email_address": "jane.dawson@example.com",
            "alias_email_addresses": ["j.dawson@example.org"]
        })
    );
}

#[tokio::test]
async fn a_scoped_client_refuses_a_contact_filed_under_another_account() {
    let server = MockServer::start().await;
    mock_identity(&server).await;
    let scoped = client(&server).for_account(42).await.unwrap();
    let params = ContactParams {
        account_user_id: Some(99),
        ..jane()
    };

    let error = scoped.contacts().create_contact(&params).await.unwrap_err();

    assert_eq!(error.code(), ErrorCode::Usage);
    assert_eq!(
        error.message(),
        "account user 99 does not belong to selected account 42"
    );
    assert_eq!(server.received_requests().await.unwrap().len(), 1);
}

#[tokio::test]
async fn an_unscoped_client_sends_only_the_account_it_was_given() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/contacts.json"))
        .respond_with(ResponseTemplate::new(201).set_body_json(json!({ "id": 91824 })))
        .mount(&server)
        .await;
    let client = client(&server);
    let chosen = ContactParams {
        account_user_id: Some(4849),
        ..jane()
    };

    client.contacts().create_contact(&jane()).await.unwrap();
    client.contacts().create_contact(&chosen).await.unwrap();

    assert!(sent_json(&server, 0).await["acting_user_id"].is_null());
    assert_eq!(sent_json(&server, 1).await["acting_user_id"], 4849);
}

#[tokio::test]
async fn a_clashing_create_names_the_contacts_to_merge_with() {
    let server = MockServer::start().await;
    mock_create_refusal(
        &server,
        409,
        json!({
            "errors": ["Some email addresses are already in use for other contacts"],
            "contact_id": 9,
            "conflicting_contact_ids": [4, 5]
        }),
    )
    .await;

    let error = client(&server)
        .contacts()
        .create_contact(&jane())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Conflict);
    assert_eq!(error.http_status(), Some(409));
    assert_eq!(
        error.message(),
        "Some email addresses are already in use for other contacts"
    );
    let conflict = ContactConflict::from_error(&error).unwrap();
    assert_eq!(conflict.contact_id, 9);
    assert_eq!(conflict.conflicting_contact_ids, vec![4, 5]);
}

#[tokio::test]
async fn a_clash_the_server_says_nothing_about_still_reads_as_one() {
    let server = MockServer::start().await;
    mock_create_refusal(&server, 409, json!({ "conflicting_contact_ids": [4] })).await;

    let error = client(&server)
        .contacts()
        .create_contact(&jane())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Conflict);
    assert_eq!(
        error.message(),
        "the contact conflicts with one that already exists"
    );
    assert_eq!(
        ContactConflict::from_error(&error)
            .unwrap()
            .conflicting_contact_ids,
        vec![4]
    );
}

#[tokio::test]
async fn a_contact_the_model_rejects_reads_the_models_own_words() {
    let server = MockServer::start().await;
    mock_create_refusal(
        &server,
        422,
        json!({ "errors": ["Email address is already in use", "Name can't be blank"] }),
    )
    .await;

    let error = client(&server)
        .contacts()
        .create_contact(&jane())
        .await
        .unwrap_err();

    assert_eq!(error.code(), ErrorCode::Validation);
    assert_eq!(error.http_status(), Some(422));
    assert_eq!(
        error.message(),
        "Email address is already in use; Name can't be blank"
    );
    assert!(ContactConflict::from_error(&error).is_none());
}

#[tokio::test]
async fn an_update_fills_in_the_fields_it_was_not_given_and_can_promote_an_alias() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/contacts/7.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 7,
            "name": "Jane Dawson",
            "email_address": "jane.dawson@example.com",
            "aliases": [{ "id": 8, "email_address": "j.dawson@example.org" }]
        })))
        .mount(&server)
        .await;
    Mock::given(method("PATCH"))
        .and(path("/contacts/7.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "id": 8 })))
        .mount(&server)
        .await;
    let params = ContactParams {
        email_address: "j.dawson@example.org".to_string(),
        ..ContactParams::default()
    };

    let contact = client(&server)
        .contacts()
        .update_contact(7, &params)
        .await
        .unwrap();

    assert_eq!(contact.id, 8);
    assert_eq!(
        sent_json(&server, 1).await["contact"],
        json!({
            "name": "Jane Dawson",
            "email_address": "j.dawson@example.org",
            "alias_email_addresses": ["j.dawson@example.org"]
        })
    );
}

#[tokio::test]
async fn a_note_is_written_whole_and_a_refused_one_reads_like_a_refused_contact() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/contacts/91824/note.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(
            json!({ "contact_id": 91824, "note": "Prefers a call", "note_html": "<div>Prefers a call</div>" }),
        ))
        .mount(&server)
        .await;
    Mock::given(method("PATCH"))
        .and(path("/contacts/91825/note.json"))
        .respond_with(
            ResponseTemplate::new(422).set_body_json(json!({ "errors": ["Note is too long"] })),
        )
        .mount(&server)
        .await;
    let client = client(&server);

    let note = client
        .contacts()
        .set_note(91824, "Prefers a call")
        .await
        .unwrap();
    let error = client
        .contacts()
        .set_note(91825, "Prefers a call")
        .await
        .unwrap_err();

    assert_eq!(note.note, "Prefers a call");
    assert_eq!(note.note_html, "<div>Prefers a call</div>");
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "contact": { "note": "Prefers a call" } })
    );
    assert_eq!(error.code(), ErrorCode::Validation);
    assert_eq!(error.message(), "Note is too long");
}

#[tokio::test]
async fn screening_a_contact_sends_the_decision_alone() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/contacts/91824/clearance.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    client(&server)
        .contacts()
        .screen(91824, ClearanceStatus::Denied)
        .await
        .unwrap();

    assert_eq!(sent_json(&server, 0).await, json!({ "status": "denied" }));
}

fn jane() -> ContactParams {
    ContactParams {
        name: "Jane Dawson".to_string(),
        email_address: "jane.dawson@example.com".to_string(),
        alias_email_addresses: Some(vec!["j.dawson@example.org".to_string()]),
        account_user_id: None,
    }
}

async fn mock_identity(server: &MockServer) {
    Mock::given(method("GET"))
        .and(path("/identity.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "id": 1,
            "accounts": [{ "id": 42, "status": "active" }],
            "all_users": [{ "id": 4849, "account_id": 42 }]
        })))
        .mount(server)
        .await;
}

async fn mock_create_refusal(server: &MockServer, status: u16, body: Value) {
    Mock::given(method("POST"))
        .and(path("/contacts.json"))
        .respond_with(ResponseTemplate::new(status).set_body_json(body))
        .mount(server)
        .await;
}

async fn sent_json(server: &MockServer, index: usize) -> Value {
    let requests = server.received_requests().await.unwrap();
    serde_json::from_slice(&requests[index].body).unwrap()
}
