//! Delivering messages and keeping drafts, on top of the generated message routes.

use crate::client::Response;
use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::{
    CreateMessageRequestContent, MessageAddressed, MessageEntryPayload, MessagePayload, SentMessage,
};

pub use crate::generated::services::messages::*;

/// What `entry.status` carries to keep an entry a draft. Any other value — or omitting
/// the status — has HEY deliver it.
const DRAFTED: &str = "drafted";

/// A message to deliver: the subject, the Trix HTML body and the recipients per kind.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct MessageContent {
    /// The subject line.
    pub subject: String,
    /// The body, as Trix HTML.
    pub content: String,
    /// The addresses the message goes to.
    pub to: Vec<String>,
    /// The addresses copied on it.
    pub cc: Vec<String>,
    /// The addresses copied on it without the others seeing.
    pub bcc: Vec<String>,
    /// The identity the message goes out as. `None` resolves the client's default sender.
    pub acting_sender_id: Option<i64>,
}

/// When a draft goes out, read in the identity's time zone. HEY schedules to the hour.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct DeliverySchedule {
    /// `YYYY-MM-DD`, "today" or "tomorrow".
    pub date: String,
    /// 0 through 23.
    pub hour: u8,
}

/// The whole of what a draft carries. HEY revises a draft from the whole of it, so a
/// caller edits by reading the draft, changing fields and sending everything back: empty
/// recipients remove them, and a `None` schedule clears one already set.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct DraftContent {
    /// The subject line.
    pub subject: String,
    /// The body, as Trix HTML.
    pub content: String,
    /// The addresses the draft goes to.
    pub to: Vec<String>,
    /// The addresses copied on it.
    pub cc: Vec<String>,
    /// The addresses copied on it without the others seeing.
    pub bcc: Vec<String>,
    /// The identity the draft is saved — and ultimately delivered — as. `None` resolves
    /// the client's default sender. A caller who chose an alternate identity carries it
    /// on every revision, since a revision that leaves it out hands the draft back to
    /// the default one.
    pub acting_sender_id: Option<i64>,
    /// When HEY delivers the draft on its own. `None` is a draft that waits to be sent.
    pub schedule: Option<DeliverySchedule>,
}

impl Messages<'_> {
    /// Delivers a new message through HEY's undo-delay window, and answers what HEY said
    /// about it: the entry that went out, the thread it started, its subject and whether
    /// Undo Send is holding it back. An answer that cannot be read costs the ids rather than
    /// the send. Delivery needs somebody to deliver to, so at least one recipient is required.
    pub async fn send(&self, message: &MessageContent) -> Result<SentMessage, Error> {
        if !has_recipients(&message.to, &message.cc, &message.bcc) {
            return Err(Error::usage(
                "a message needs at least one recipient (to, cc or bcc)",
            ));
        }

        let body = CreateMessageRequestContent {
            acting_sender_id: self.sender_for(message.acting_sender_id).await?,
            message: MessagePayload {
                subject: message.subject.clone(),
                content: message.content.clone(),
            },
            entry: Some(delivered_entry(&message.to, &message.cc, &message.bcc)),
        };
        let mut operation = self.client().operation(&routes::CREATE_MESSAGE, &[]);
        operation.json(&body)?;
        let response = self.client().execute(operation).await?;
        Ok(sent_message(&response))
    }

    /// Saves a new message as a draft instead of delivering it, and answers the draft's
    /// entry id — the id [`Messages::get_edit`], [`Messages::update_draft`],
    /// [`Messages::send_draft`] and `Entries::delete_draft` take. A draft needs no
    /// recipients; whatever it carries is kept for the send.
    pub async fn create_draft(&self, draft: &DraftContent) -> Result<i64, Error> {
        let body = self.drafted_request(draft).await?;
        let mut operation = self.client().operation(&routes::CREATE_MESSAGE, &[]);
        operation.json(&body)?;
        let response = self.client().execute(operation).await?;
        entry_id_from_location(&response)
    }

    /// Revises a draft in place from the whole of `draft`. A trashed draft is silently
    /// restored by the revision.
    pub async fn update_draft(&self, entry_id: i64, draft: &DraftContent) -> Result<(), Error> {
        let body = self.drafted_request(draft).await?;
        let mut operation = self
            .client()
            .operation(&routes::UPDATE_MESSAGE, &[&entry_id]);
        operation.json(&body)?;
        self.client().send_unit(operation).await
    }

    /// Delivers a draft through HEY's undo-delay window. The revision and the delivery are
    /// one request, so the draft's final state rides along: subject, body and recipients
    /// are replaced with what is sent, exactly as [`Messages::update_draft`] replaces them.
    ///
    /// The request is never retried, despite the PUT: it triggers a delivery, and a resend
    /// after an ambiguous first attempt could send the message twice. Reading the draft, or
    /// the thread, is the caller's way out of an ambiguous failure.
    ///
    /// It answers what HEY said about the delivery, as [`Messages::send`] does. The entry
    /// that went out is normally the draft itself; a draft that breaks out into a thread of
    /// its own on a Domains account goes out as a new entry, and `id` names that one.
    pub async fn send_draft(
        &self,
        entry_id: i64,
        draft: &DraftContent,
    ) -> Result<SentMessage, Error> {
        if !has_recipients(&draft.to, &draft.cc, &draft.bcc) {
            return Err(Error::usage(
                "sending a draft needs at least one recipient (to, cc or bcc)",
            ));
        }

        let body = CreateMessageRequestContent {
            acting_sender_id: self.sender_for(draft.acting_sender_id).await?,
            message: MessagePayload {
                subject: draft.subject.clone(),
                content: draft.content.clone(),
            },
            entry: Some(delivered_entry(&draft.to, &draft.cc, &draft.bcc)),
        };
        let mut operation = self
            .client()
            .operation(&routes::UPDATE_MESSAGE, &[&entry_id]);
        operation.json(&body)?;
        operation.idempotent(false);
        let response = self.client().execute(operation).await?;
        Ok(sent_message(&response))
    }

    async fn sender_for(&self, chosen: Option<i64>) -> Result<i64, Error> {
        match chosen {
            Some(id) => Ok(id),
            None => self.client().default_sender_id().await,
        }
    }

    async fn drafted_request(
        &self,
        draft: &DraftContent,
    ) -> Result<CreateMessageRequestContent, Error> {
        let mut entry = drafted_entry(&draft.to, &draft.cc, &draft.bcc);
        if let Some(schedule) = &draft.schedule {
            entry.scheduled_delivery = Some("true".to_string());
            entry.scheduled_delivery_at_date = Some(schedule.date.clone());
            entry.scheduled_delivery_at_hour = Some(schedule.hour.to_string());
        }
        Ok(CreateMessageRequestContent {
            acting_sender_id: self.sender_for(draft.acting_sender_id).await?,
            message: MessagePayload {
                subject: draft.subject.clone(),
                content: draft.content.clone(),
            },
            entry: Some(entry),
        })
    }
}

pub(crate) fn has_recipients(to: &[String], cc: &[String], bcc: &[String]) -> bool {
    !to.is_empty() || !cc.is_empty() || !bcc.is_empty()
}

/// The entry of a message HEY delivers, whose recipient kinds are the ones that name
/// somebody.
pub(crate) fn delivered_entry(to: &[String], cc: &[String], bcc: &[String]) -> MessageEntryPayload {
    let addressed = MessageAddressed {
        directly: recipients(to),
        copied: recipients(cc),
        blindcopied: recipients(bcc),
    };
    MessageEntryPayload {
        addressed: Some(addressed),
        ..MessageEntryPayload::default()
    }
}

fn recipients(addresses: &[String]) -> Option<Vec<String>> {
    if addresses.is_empty() {
        None
    } else {
        Some(addresses.to_vec())
    }
}

/// The entry of a message HEY keeps as a draft. Every recipient kind is present, empty
/// ones included: a draft addressed to nobody yet is the normal case, and on a revision
/// an empty list is how recipients are removed.
pub(crate) fn drafted_entry(to: &[String], cc: &[String], bcc: &[String]) -> MessageEntryPayload {
    let addressed = MessageAddressed {
        directly: Some(to.to_vec()),
        copied: Some(cc.to_vec()),
        blindcopied: Some(bcc.to_vec()),
    };
    MessageEntryPayload {
        addressed: Some(addressed),
        status: Some(DRAFTED.to_string()),
        ..MessageEntryPayload::default()
    }
}

/// What HEY answered for a message it has just delivered — a new message, a reply or a
/// sent draft. By the time it is read the message has gone out, so an answer that cannot
/// be read costs the ids rather than the send: a caller told the send failed would send it
/// again. A HEY that predates the ids answers `{}` — or, while Undo Send holds the delivery
/// back, only the undo members, and `delayed` is read from `undo_action` there, since an
/// undo is only offered while the delivery is delayed.
pub(crate) fn sent_message(response: &Response) -> SentMessage {
    let mut sent: SentMessage = response.json().unwrap_or_default();
    if sent.delayed.is_none() && sent.undo_action.is_some() {
        sent.delayed = Some(true);
    }
    sent
}

/// The entry id out of the `Location` a draft save answers with: the save serves no body,
/// so the header is the only place the id is named.
pub(crate) fn entry_id_from_location(response: &Response) -> Result<i64, Error> {
    let status = response.status.as_u16();
    let location = response.header("location").ok_or_else(|| {
        Error::api(
            status,
            "draft saved but the response named no Location; cannot report the draft's id",
        )
    })?;
    let url = response.url.join(location).map_err(|error| {
        Error::api(
            status,
            format!("draft saved but its Location {location:?} is unreadable: {error}"),
        )
    })?;
    match url.path().rsplit('/').next().unwrap_or_default().parse() {
        Ok(entry_id) if entry_id > 0 => Ok(entry_id),
        _ => Err(Error::api(
            status,
            format!("draft saved but its Location {location:?} names no entry id"),
        )),
    }
}

// Wire tests on the shipped HTTP client; without the `reqwest` feature there is none.
#[cfg(all(test, feature = "reqwest"))]
mod tests {
    use std::time::Duration;

    use serde_json::{Value, json};
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use super::*;
    use crate::auth::StaticTokenProvider;
    use crate::client::Client;
    use crate::config::Config;
    use crate::error::ErrorCode;

    #[tokio::test]
    async fn send_refuses_a_message_addressed_to_nobody() {
        let server = MockServer::start().await;
        let message = MessageContent {
            subject: "Hello".to_string(),
            content: "Body".to_string(),
            ..MessageContent::default()
        };

        let error = client(&server).messages().send(&message).await.unwrap_err();

        assert_eq!(error.code(), ErrorCode::Usage);
        assert!(server.received_requests().await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn send_resolves_the_identity_default_sender() {
        let server = MockServer::start().await;
        let identity = json!({ "id": 1, "senders": [{ "id": 41 }, { "id": 42, "default": true }] });
        Mock::given(method("GET"))
            .and(path("/identity.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(identity))
            .mount(&server)
            .await;
        Mock::given(method("POST"))
            .and(path("/messages.json"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;
        let message = MessageContent {
            subject: "Hello".to_string(),
            content: "Body".to_string(),
            to: vec!["someone@example.com".to_string()],
            ..MessageContent::default()
        };

        client(&server).messages().send(&message).await.unwrap();

        let requests = server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 2);
        assert_eq!(requests[0].url.path(), "/identity.json");
        let body: Value = serde_json::from_slice(&requests[1].body).unwrap();
        assert_eq!(body["acting_sender_id"], 42);
        assert_eq!(
            body["message"],
            json!({ "subject": "Hello", "content": "Body" })
        );
        assert_eq!(
            body["entry"],
            json!({ "addressed": { "directly": ["someone@example.com"] } })
        );
    }

    #[tokio::test]
    async fn create_draft_saves_it_drafted_and_answers_the_entry_id() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/messages.json"))
            .respond_with(
                ResponseTemplate::new(204)
                    .insert_header("Location", "https://app.hey.com/messages/777"),
            )
            .mount(&server)
            .await;
        let draft = DraftContent {
            subject: "From the support address".to_string(),
            content: "Draft body".to_string(),
            acting_sender_id: Some(314),
            schedule: Some(DeliverySchedule {
                date: "2026-09-02".to_string(),
                hour: 0,
            }),
            ..DraftContent::default()
        };

        let entry_id = client(&server)
            .messages()
            .create_draft(&draft)
            .await
            .unwrap();

        assert_eq!(entry_id, 777);
        let body = sent_json(&server).await;
        assert_eq!(body["acting_sender_id"], 314);
        assert_eq!(
            body["entry"],
            json!({
                "addressed": { "directly": [], "copied": [], "blindcopied": [] },
                "status": "drafted",
                "scheduled_delivery": "true",
                "scheduled_delivery_at_date": "2026-09-02",
                "scheduled_delivery_at_hour": "0"
            })
        );
    }

    #[tokio::test]
    async fn update_draft_says_the_acting_sender_back() {
        let server = MockServer::start().await;
        Mock::given(method("PUT"))
            .and(path("/messages/777.json"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;
        let draft = DraftContent {
            subject: "From the support address (v2)".to_string(),
            content: "Rewritten body".to_string(),
            to: vec!["someone@example.com".to_string()],
            acting_sender_id: Some(314),
            ..DraftContent::default()
        };

        client(&server)
            .messages()
            .update_draft(777, &draft)
            .await
            .unwrap();

        let body = sent_json(&server).await;
        assert_eq!(body["acting_sender_id"], 314);
        assert_eq!(body["entry"]["status"], "drafted");
        assert_eq!(
            body["entry"]["addressed"],
            json!({ "directly": ["someone@example.com"], "copied": [], "blindcopied": [] })
        );
        assert!(body["entry"]["scheduled_delivery"].is_null());
    }

    #[tokio::test]
    async fn send_draft_delivers_it_by_leaving_the_status_off() {
        let server = MockServer::start().await;
        Mock::given(method("PUT"))
            .and(path("/messages/777.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "id": 777 })))
            .mount(&server)
            .await;

        client(&server)
            .messages()
            .send_draft(777, &deliverable_draft())
            .await
            .unwrap();

        let body = sent_json(&server).await;
        assert_eq!(body["acting_sender_id"], 314);
        assert!(body["entry"]["status"].is_null());
        assert_eq!(
            body["entry"]["addressed"],
            json!({ "directly": ["someone@example.com"] })
        );
    }

    #[tokio::test]
    async fn send_draft_refuses_a_draft_addressed_to_nobody() {
        let server = MockServer::start().await;
        let draft = DraftContent {
            to: Vec::new(),
            ..deliverable_draft()
        };

        let error = client(&server)
            .messages()
            .send_draft(777, &draft)
            .await
            .unwrap_err();

        assert_eq!(error.code(), ErrorCode::Usage);
        assert!(server.received_requests().await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn send_draft_is_never_resent() {
        let server = MockServer::start().await;
        Mock::given(method("PUT"))
            .and(path("/messages/777.json"))
            .respond_with(ResponseTemplate::new(503))
            .mount(&server)
            .await;
        let client = Client::builder(Config::default().with_base_url(server.uri()))
            .token_provider(StaticTokenProvider::new("t"))
            .base_delay(Duration::from_millis(1))
            .build()
            .unwrap();

        let error = client
            .messages()
            .send_draft(777, &deliverable_draft())
            .await
            .unwrap_err();

        assert_eq!(error.http_status(), Some(503));
        assert_eq!(server.received_requests().await.unwrap().len(), 1);
    }

    #[tokio::test]
    async fn a_draft_save_without_a_location_is_an_error() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/messages.json"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;

        let draft = DraftContent {
            acting_sender_id: Some(314),
            ..DraftContent::default()
        };

        let error = client(&server)
            .messages()
            .create_draft(&draft)
            .await
            .unwrap_err();

        assert_eq!(error.code(), ErrorCode::Api);
    }

    #[tokio::test]
    async fn send_answers_the_entry_hey_delivered() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/messages.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "id": 2201, "topic_id": 880, "subject": "Lunch on Friday", "delayed": false
            })))
            .mount(&server)
            .await;

        let sent = client(&server)
            .messages()
            .send(&deliverable_message())
            .await
            .unwrap();

        assert_eq!(sent.id, Some(2201));
        assert_eq!(sent.topic_id, Some(880));
        assert_eq!(sent.subject.as_deref(), Some("Lunch on Friday"));
        assert_eq!(sent.delayed, Some(false));
        assert_eq!(sent.undo_action, None);
    }

    #[tokio::test]
    async fn send_draft_answers_an_undoable_delivery() {
        let server = MockServer::start().await;
        Mock::given(method("PUT"))
            .and(path("/messages/777.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "id": 777, "topic_id": 880, "subject": "From the support address", "delayed": true,
                "notice": "Message sent",
                "undo_action": "https://app.hey.com/topics/880/undo_send",
                "undo_timeout": 12
            })))
            .mount(&server)
            .await;

        let sent = client(&server)
            .messages()
            .send_draft(777, &deliverable_draft())
            .await
            .unwrap();

        assert_eq!(sent.id, Some(777));
        assert_eq!(sent.topic_id, Some(880));
        assert_eq!(sent.delayed, Some(true));
        assert_eq!(
            sent.undo_action.as_deref(),
            Some("https://app.hey.com/topics/880/undo_send")
        );
        assert_eq!(sent.undo_timeout, Some(12));
    }

    #[tokio::test]
    async fn send_answers_without_ids_from_a_hey_that_serves_none() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/messages.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({})))
            .mount(&server)
            .await;

        let sent = client(&server)
            .messages()
            .send(&deliverable_message())
            .await
            .unwrap();

        assert_eq!(sent, SentMessage::default());
    }

    #[tokio::test]
    async fn send_reads_the_delay_from_the_undo_of_a_hey_that_serves_no_ids() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/messages.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "notice": "Message sent",
                "undo_action": "https://app.hey.com/topics/880/undo_send",
                "undo_timeout": 12
            })))
            .mount(&server)
            .await;

        let sent = client(&server)
            .messages()
            .send(&deliverable_message())
            .await
            .unwrap();

        assert_eq!(sent.id, None);
        assert_eq!(sent.topic_id, None);
        assert_eq!(sent.delayed, Some(true));
    }

    // The message has gone out by the time its answer is read, so an answer the SDK cannot
    // read must not turn into an error: a caller told the send failed would send it again.
    #[tokio::test]
    async fn a_delivered_message_is_not_an_error_over_an_unreadable_answer() {
        for answer in [
            ResponseTemplate::new(204),
            ResponseTemplate::new(200).set_body_string("<html>Message sent</html>"),
            ResponseTemplate::new(200).set_body_json(json!({ "id": "2201" })),
        ] {
            let server = MockServer::start().await;
            Mock::given(method("POST"))
                .and(path("/messages.json"))
                .respond_with(answer)
                .mount(&server)
                .await;

            let sent = client(&server)
                .messages()
                .send(&deliverable_message())
                .await
                .unwrap();

            assert_eq!(sent, SentMessage::default());
        }
    }

    fn deliverable_message() -> MessageContent {
        MessageContent {
            subject: "Lunch on Friday".to_string(),
            content: "Are you free at noon?".to_string(),
            to: vec!["someone@example.com".to_string()],
            acting_sender_id: Some(314),
            ..MessageContent::default()
        }
    }

    fn deliverable_draft() -> DraftContent {
        DraftContent {
            subject: "From the support address".to_string(),
            content: "Final body".to_string(),
            to: vec!["someone@example.com".to_string()],
            acting_sender_id: Some(314),
            ..DraftContent::default()
        }
    }

    fn client(server: &MockServer) -> Client {
        Client::builder(Config::default().with_base_url(server.uri()))
            .token_provider(StaticTokenProvider::new("t"))
            .max_retries(0)
            .build()
            .unwrap()
    }

    async fn sent_json(server: &MockServer) -> Value {
        let requests = server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 1);
        serde_json::from_slice(&requests[0].body).unwrap()
    }
}
