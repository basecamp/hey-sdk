//! Wire-level tests for the `extenzions` service: what each call sends and how it reads what HEY answers, against literal bodies.

mod support;

use std::sync::Arc;

use hey_sdk::services::{CreateExtenzionParams, UpdateExtenzionParams};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::{Operations, builder, client};

/// The group leads with the "All Extensions" link, which names a domains page rather than a
/// contact and so is not an extenzion.
#[tokio::test]
async fn listing_reads_the_extensions_group_out_of_navigation() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/my/navigation.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "items": [
                { "title": "Imbox", "app_url": "https://app.hey.com/imbox" },
                { "title": "Extensions", "menu_items": [
                    { "title": "All Extensions", "app_url": "https://app.hey.com/accounts/1/domains/extenzions" },
                    { "title": "sales", "app_url": "https://app.hey.com/contacts/10" },
                    { "title": "support", "app_url": "https://app.hey.com/contacts/20" }
                ]}
            ]
        })))
        .mount(&server)
        .await;

    let extenzions = client(&server).extenzions().list().await.unwrap();

    let listed: Vec<(i64, &str, &str)> = extenzions
        .iter()
        .map(|extenzion| {
            (
                extenzion.id,
                extenzion.name.as_str(),
                extenzion.app_url.as_str(),
            )
        })
        .collect();
    assert_eq!(
        listed,
        [
            (10, "sales", "https://app.hey.com/contacts/10"),
            (20, "support", "https://app.hey.com/contacts/20"),
        ]
    );
}

/// The id is the contact's, out of `app_url` — the payload's own id belongs to the Extenzion
/// record, which no write endpoint takes.
#[tokio::test]
async fn creating_answers_the_extenzion_under_its_contact_id() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/accounts/1/domains/extenzions.json"))
        .respond_with(ResponseTemplate::new(201).set_body_json(
            json!({ "id": 55, "name": "sales", "app_url": "https://app.hey.com/contacts/10" }),
        ))
        .mount(&server)
        .await;

    let created = client(&server)
        .extenzions()
        .create(
            1,
            &CreateExtenzionParams {
                name: "sales".to_string(),
                members: vec!["jane.dawson@example.com".to_string()],
            },
        )
        .await
        .unwrap();

    let created = created.unwrap();
    assert_eq!(created.id, 10);
    assert_eq!(created.name, "sales");
    assert_eq!(created.app_url, "https://app.hey.com/contacts/10");
    assert_eq!(
        sent_form(&server).await,
        [
            "extenzion[name]=sales",
            "extenzion[members][]=jane.dawson@example.com",
        ]
    );
}

/// A server without the JSON create branch redirects to the extensions page, which names
/// nothing about the new extenzion.
#[tokio::test]
async fn a_create_that_only_redirects_hands_nothing_back() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/accounts/1/domains/extenzions.json"))
        .respond_with(
            ResponseTemplate::new(302).insert_header("Location", "/accounts/1/domains/extenzions"),
        )
        .mount(&server)
        .await;

    let created = client(&server)
        .extenzions()
        .create(
            1,
            &CreateExtenzionParams {
                name: "sales".to_string(),
                ..CreateExtenzionParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(created, None);
}

#[tokio::test]
async fn a_revision_names_only_what_it_was_asked_to_change() {
    let server = MockServer::start().await;
    mock_update(&server).await;

    let updated = client(&server)
        .extenzions()
        .update(
            1,
            10,
            &UpdateExtenzionParams {
                name: Some("support".to_string()),
                ..UpdateExtenzionParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(updated.unwrap().name, "support");
    assert_eq!(sent_form(&server).await, ["extenzion[name]=support"]);
}

/// A `None` membership leaves the roster alone; naming one replaces it wholesale, and an
/// empty one is how it is emptied.
#[tokio::test]
async fn a_revision_replaces_the_whole_membership_when_it_names_one() {
    let server = MockServer::start().await;
    mock_update(&server).await;

    client(&server)
        .extenzions()
        .update(
            1,
            10,
            &UpdateExtenzionParams {
                members: Some(vec![
                    "jane.dawson@example.com".to_string(),
                    "yusuf.demir@example.org".to_string(),
                ]),
                ..UpdateExtenzionParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "extenzion[members][]=jane.dawson@example.com",
            "extenzion[members][]=yusuf.demir@example.org",
        ]
    );
}

#[tokio::test]
async fn an_update_that_only_redirects_hands_nothing_back() {
    let server = MockServer::start().await;
    Mock::given(method("PATCH"))
        .and(path("/accounts/1/domains/extenzions/10.json"))
        .respond_with(
            ResponseTemplate::new(302).insert_header("Location", "/accounts/1/domains/extenzions"),
        )
        .mount(&server)
        .await;

    let updated = client(&server)
        .extenzions()
        .update(
            1,
            10,
            &UpdateExtenzionParams {
                name: Some("support".to_string()),
                ..UpdateExtenzionParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(updated, None);
}

/// Listing the extenzions is one operation of its own, not the identity read it is built
/// on: the hooks hear `Extenzions.ListExtenzions`.
#[tokio::test]
async fn listing_announces_itself_as_an_extenzion_read() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/my/navigation.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "items": [] })))
        .mount(&server)
        .await;
    let operations = Arc::new(Operations::default());
    let client = builder(&server).hooks(operations.clone()).build().unwrap();

    client.extenzions().list().await.unwrap();

    assert_eq!(
        ["Extenzions.ListExtenzions"],
        operations.started().as_slice()
    );
}

/// A contact URL the SDK cannot read is a failure, not another "All Extensions" link:
/// dropping it would hide an extenzion the caller would then never hear about.
#[tokio::test]
async fn a_contact_url_that_names_no_readable_id_is_refused_rather_than_skipped() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/my/navigation.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "items": [
                { "title": "Extensions", "menu_items": [
                    { "title": "sales", "app_url": "https://app.hey.com/contacts/99999999999999999999" }
                ]}
            ]
        })))
        .mount(&server)
        .await;

    let error = client(&server).extenzions().list().await.unwrap_err();

    assert!(error.message().contains("is not a number"), "{error}");
}

async fn mock_update(server: &MockServer) {
    Mock::given(method("PATCH"))
        .and(path("/accounts/1/domains/extenzions/10.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(
            json!({ "id": 55, "name": "support", "app_url": "https://app.hey.com/contacts/10" }),
        ))
        .mount(server)
        .await;
}

async fn sent_form(server: &MockServer) -> Vec<String> {
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    url::form_urlencoded::parse(&requests[0].body)
        .map(|(name, value)| format!("{name}={value}"))
        .collect()
}
