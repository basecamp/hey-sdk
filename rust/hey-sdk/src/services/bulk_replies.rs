//! Sending one reply to many threads, and calling a delayed one back, on top of the
//! generated bulk reply routes.

use reqwest::Method;
use url::Url;

use crate::error::Error;
use crate::generated::types::{
    BulkReplyMessagePayload, BulkReplyRequestContent, CreateBulkReplyResponseContent,
    NewBulkReplyResponseContent,
};
use crate::services::write_info;

pub use crate::generated::services::bulk_replies::*;

impl<'a> BulkReplies<'a> {
    /// Works out which entries a bulk reply would answer, and how it starts.
    ///
    /// HEY replies to the last replyable entry of each thread and skips threads it has no
    /// reply address for, so the postings you hold are not the entries the reply goes to.
    /// Send the entries this answers — or a subset of them — to [`BulkReplies::send`].
    ///
    /// The generated [`BulkReplies::new_bulk_reply`] takes the ids already comma-joined.
    pub async fn draft(&self, posting_ids: &[i64]) -> Result<NewBulkReplyResponseContent, Error> {
        if posting_ids.is_empty() {
            return Err(Error::usage("at least one posting is required"));
        }
        self.new_bulk_reply(&join_ids(posting_ids)).await
    }

    /// Replies to every entry with the same content, and answers what was sent.
    ///
    /// Delivery is queued. While the sender has undo enabled the send is held open, and the
    /// answer says so: `delayed` is true and `undo_send_url` is where to call it back with
    /// [`BulkReplies::undo`].
    ///
    /// The generated [`BulkReplies::create`] takes the same request as a body.
    pub async fn send(
        &self,
        entry_ids: &[i64],
        content: &str,
    ) -> Result<CreateBulkReplyResponseContent, Error> {
        if entry_ids.is_empty() {
            return Err(Error::usage("at least one entry is required"));
        }
        let body = BulkReplyRequestContent {
            entry_ids: entry_ids.to_vec(),
            message: BulkReplyMessagePayload {
                content: content.to_string(),
            },
        };
        self.create(&body).await
    }

    /// Calls a delayed bulk reply back before it goes out.
    ///
    /// HEY answers this one with a redirect rather than JSON — the same answer its own apps
    /// read. Once the replies have gone out there is nothing left to call back, and HEY
    /// refuses; the SDK does not check the delivery first.
    pub async fn undo(&self, bulk_reply_id: i64) -> Result<(), Error> {
        let mut operation = self.client().form(
            Method::POST,
            &format!("/bulk_replies/{bulk_reply_id}/undo_send"),
        )?;
        operation.info(write_info(
            "BulkReplies",
            "UndoBulkReplySend",
            "bulk_reply",
            Some(bulk_reply_id),
        ));
        operation.form(&[]);
        self.client().send_unit(operation).await
    }
}

/// The bulk reply id in a delivery's `undo_send_url`, for a caller holding the URL rather
/// than the delivery. Only that URL is read: anything else the caller might be holding —
/// the bulk reply itself, some other action on it — is refused rather than guessed at.
pub fn undo_send_id(undo_send_url: &str) -> Result<i64, Error> {
    let path = match Url::parse(undo_send_url) {
        Ok(url) => url.path().to_string(),
        Err(url::ParseError::RelativeUrlWithoutBase) => undo_send_url
            .split(['?', '#'])
            .next()
            .unwrap_or_default()
            .to_string(),
        Err(_) => String::new(),
    };

    let segments: Vec<&str> = path.trim_matches('/').split('/').collect();
    match segments.as_slice() {
        ["bulk_replies", id, "undo_send"] => id.parse().map_err(|_| not_an_undo_url(undo_send_url)),
        _ => Err(not_an_undo_url(undo_send_url)),
    }
}

fn not_an_undo_url(undo_send_url: &str) -> Error {
    Error::usage(format!("not a bulk reply undo URL: {undo_send_url}"))
}

fn join_ids(posting_ids: &[i64]) -> String {
    posting_ids
        .iter()
        .map(i64::to_string)
        .collect::<Vec<_>>()
        .join(",")
}
