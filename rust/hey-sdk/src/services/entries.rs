//! Replying to an entry, delivered or kept as a draft.

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::{CreateReplyRequestContent, ReplyMessagePayload};
use crate::services::messages::{
    delivered_entry, drafted_entry, entry_id_from_location, has_recipients,
};

pub use crate::generated::services::entries::*;

/// A reply, as the reply prefill (`Entries::new_reply`) hands its parts over.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct ReplyContent {
    /// The identity the reply goes out as. HEY resolves a reply's sender from the thread
    /// — on a shared or alternate address that is not the account default — and hands it
    /// back as the prefill's sender; pass that id. Zero falls back to the client's
    /// default sender, which on such a thread is the wrong identity.
    pub acting_sender_id: i64,
    /// HEY derives no subject for a reply, so pass the prefill's "Re: …" — a draft saved
    /// without one reads "No subject". Empty leaves it off the wire.
    pub subject: String,
    /// The reply body alone: HEY appends the quoted original at delivery, so the
    /// prefill's quoted content must not be echoed back.
    pub content: String,
    pub to: Vec<String>,
    pub cc: Vec<String>,
    pub bcc: Vec<String>,
}

impl<'a> Entries<'a> {
    /// Delivers a reply to an entry. HEY does not reply-all on the caller's behalf, and
    /// saves an unaddressed reply as a draft rather than delivering it, so the thread's
    /// recipients — the prefill's — are required.
    pub async fn reply(&self, entry_id: i64, reply: &ReplyContent) -> Result<(), Error> {
        if !has_recipients(&reply.to, &reply.cc, &reply.bcc) {
            return Err(Error::usage(
                "a reply needs at least one recipient (to, cc or bcc); HEY saves an unaddressed reply as a draft",
            ));
        }

        let body = CreateReplyRequestContent {
            acting_sender_id: self.sender_for(reply).await?,
            message: reply_payload(reply),
            entry: Some(delivered_entry(&reply.to, &reply.cc, &reply.bcc)),
        };
        let mut operation = self.client().operation(&routes::CREATE_REPLY, &[&entry_id]);
        operation.json(&body)?;
        self.client().send_unit(operation).await
    }

    /// Saves a reply as a draft instead of delivering it, and answers the draft's entry
    /// id. Unlike [`Entries::reply`] it needs no recipients; whatever it carries is kept
    /// for the send.
    pub async fn reply_draft(&self, entry_id: i64, reply: &ReplyContent) -> Result<i64, Error> {
        let body = CreateReplyRequestContent {
            acting_sender_id: self.sender_for(reply).await?,
            message: reply_payload(reply),
            entry: Some(drafted_entry(&reply.to, &reply.cc, &reply.bcc)),
        };
        let mut operation = self.client().operation(&routes::CREATE_REPLY, &[&entry_id]);
        operation.json(&body)?;
        let response = self.client().execute(operation).await?;
        entry_id_from_location(&response)
    }

    async fn sender_for(&self, reply: &ReplyContent) -> Result<i64, Error> {
        if reply.acting_sender_id == 0 {
            self.client().default_sender_id().await
        } else {
            Ok(reply.acting_sender_id)
        }
    }
}

fn reply_payload(reply: &ReplyContent) -> ReplyMessagePayload {
    let subject = if reply.subject.is_empty() {
        None
    } else {
        Some(reply.subject.clone())
    };
    ReplyMessagePayload {
        subject,
        content: reply.content.clone(),
    }
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use super::*;
    use crate::auth::StaticTokenProvider;
    use crate::client::Client;
    use crate::config::Config;
    use crate::error::ErrorCode;

    #[tokio::test]
    async fn reply_sends_the_chosen_acting_sender_untouched() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/entries/456/replies.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "notice": "sent" })))
            .mount(&server)
            .await;

        client(&server)
            .entries()
            .reply(456, &prefilled_reply())
            .await
            .unwrap();

        let body = sent_json(&server).await;
        assert_eq!(body["acting_sender_id"], 314);
        assert_eq!(
            body["message"],
            json!({ "subject": "Re: From the support address", "content": "Reply text" })
        );
        assert_eq!(
            body["entry"],
            json!({ "addressed": { "directly": ["someone@example.com"] } })
        );
    }

    #[tokio::test]
    async fn reply_refuses_a_reply_addressed_to_nobody() {
        let server = MockServer::start().await;
        let reply = ReplyContent {
            to: Vec::new(),
            ..prefilled_reply()
        };

        let error = client(&server)
            .entries()
            .reply(456, &reply)
            .await
            .unwrap_err();

        assert_eq!(error.code(), ErrorCode::Usage);
        assert!(server.received_requests().await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn reply_draft_saves_it_drafted_and_answers_the_entry_id() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/entries/456/replies.json"))
            .respond_with(
                ResponseTemplate::new(204)
                    .insert_header("Location", "https://app.hey.com/messages/777"),
            )
            .mount(&server)
            .await;
        let reply = ReplyContent {
            to: Vec::new(),
            content: "Draft text".to_string(),
            ..prefilled_reply()
        };

        let entry_id = client(&server)
            .entries()
            .reply_draft(456, &reply)
            .await
            .unwrap();

        assert_eq!(entry_id, 777);
        let body = sent_json(&server).await;
        assert_eq!(body["acting_sender_id"], 314);
        assert_eq!(body["message"]["subject"], "Re: From the support address");
        assert_eq!(
            body["entry"],
            json!({
                "addressed": { "directly": [], "copied": [], "blindcopied": [] },
                "status": "drafted"
            })
        );
    }

    #[tokio::test]
    async fn a_reply_without_a_subject_leaves_it_off_the_wire() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/entries/456/replies.json"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "notice": "sent" })))
            .mount(&server)
            .await;
        let reply = ReplyContent {
            subject: String::new(),
            ..prefilled_reply()
        };

        client(&server).entries().reply(456, &reply).await.unwrap();

        let body = sent_json(&server).await;
        assert_eq!(body["message"], json!({ "content": "Reply text" }));
    }

    fn prefilled_reply() -> ReplyContent {
        ReplyContent {
            acting_sender_id: 314,
            subject: "Re: From the support address".to_string(),
            content: "Reply text".to_string(),
            to: vec!["someone@example.com".to_string()],
            ..ReplyContent::default()
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
