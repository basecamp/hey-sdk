//! Trashing a topic, including the confirmation HEY asks for before it trashes a shared
//! one, and calling back a message Undo Send still holds.
//!
//! [`Topics::get_entries`] is paged by geared pagination, so its `page` is a cursor out of
//! the previous answer's `Link` header rather than an offset: a number is ignored and
//! answered with the first page. [`crate::Page::next_page`] carries the cursor to pass back.

use url::Url;

use crate::client::Response;
use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::MoveTopicRequestContent;

pub use crate::generated::services::topics::*;

impl Topics<'_> {
    /// Moves a topic to a box, by the box's id. The generated [`Topics::move_topic`] takes
    /// the request body; this takes the one thing it carries.
    pub async fn move_to_box(&self, topic_id: i64, box_id: i64) -> Result<(), Error> {
        self.move_topic(topic_id, &MoveTopicRequestContent { box_id })
            .await
    }

    /// Trashes a topic.
    ///
    /// HEY will not trash a shared topic without being asked twice: it answers the removal
    /// confirmation page instead, which comes back here as a usage error rather than as a
    /// trashing that quietly did nothing. Confirming trashes the topic and removes your
    /// access to it.
    ///
    /// The generated [`Topics::trash`] sends the same request from its parts, and follows
    /// that redirect rather than reading it.
    pub async fn trash_topic(&self, topic_id: i64, confirm_destroy: bool) -> Result<(), Error> {
        let mut operation = self.client().operation(&routes::TRASH_TOPIC, &[&topic_id]);
        operation.resource_id(topic_id);
        // An empty confirm_destroy reads as truthy on the server and skips the
        // confirmation, so it is sent only when it is asked for.
        if confirm_destroy {
            operation.query("confirm_destroy", 1);
        }
        operation.capture_redirects();

        let response = self.client().execute(operation).await?;
        if awaiting_confirmation(&response, topic_id) {
            Err(Error::usage_with_hint(
                format!("topic {topic_id} is shared; HEY wants confirmation before trashing it"),
                "Call trash_topic with confirm_destroy = true to trash it and remove your access",
            ))
        } else {
            Ok(())
        }
    }

    /// Calls back the last message the caller sent on a topic, while Undo Send still holds
    /// it, and answers the entry HEY called back: a draft again, unsent.
    ///
    /// HEY picks the message, the caller's last delayed entry on the topic, so compare the
    /// entry answered with the one meant: two messages sent to one thread within the delay
    /// are called back last first. A reply that broke out into a thread of its own is
    /// called back on that thread, the one the send's `topic_id` names.
    ///
    /// HEY answers with a redirect to the draft rather than JSON, the answer its own apps
    /// read, and the redirect is the only place it names the entry: this takes it as the
    /// answer rather than follow it, and reads the entry with [`undone_entry_id`]. The
    /// entry is `None` when the redirect names none. Once the delay has run out and the
    /// message has gone, or when nothing was held, HEY answers 400: an
    /// [`crate::ErrorCode::Api`] error whose `http_status` is 400, and nothing was changed.
    /// It is never sent twice.
    ///
    /// The generated [`Topics::undo_send`] sends the same request and follows the redirect,
    /// which says the undo went through and not which entry it was.
    pub async fn undo_topic_send(&self, topic_id: i64) -> Result<Option<i64>, Error> {
        let mut operation = self
            .client()
            .operation(&routes::UNDO_TOPIC_SEND, &[&topic_id]);
        operation.resource_id(topic_id);
        operation.capture_redirects();
        // The refusal is reworded inside the operation, so the hooks hear the error the
        // caller gets.
        operation.quiet();
        let info = operation.info.clone();
        self.client()
            .as_operation(
                &info,
                Box::pin(async {
                    match self.client().execute(operation).await {
                        Ok(response) => Ok(called_back(&response)),
                        Err(error) if error.http_status() == Some(400) => {
                            Err(error.reworded(NOTHING_TO_UNDO))
                        }
                        Err(error) => Err(error),
                    }
                }),
            )
            .await
    }
}

/// What a 400 from an undo means: there was no delayed entry of the caller's on the topic.
const NOTHING_TO_UNDO: &str = "nothing to undo: the message was already sent, or none was held";

/// The entry the redirect an undo answered with names, when it names one.
fn called_back(response: &Response) -> Option<i64> {
    if response.status.is_redirection() {
        response.header("location").and_then(undone_entry_id)
    } else {
        None
    }
}

/// The entry HEY called back, read out of the redirect an undo send answers with, for a
/// caller holding that `Location`: `/messages/{id}/edit` for a message on a thread of its
/// own, `/topics/{topicId}?expanded_draft={id}` for a reply. Anything else names no entry:
/// the thread alone, which is where HEY's pages send a browser with nothing to undo, is
/// not an undo.
pub fn undone_entry_id(location: &str) -> Option<i64> {
    // A Location is often a path rather than a whole URL, so it is read against a base
    // that stands for wherever it came from: only its path and query are looked at.
    let target = Url::parse("https://hey.invalid/")
        .ok()?
        .join(location)
        .ok()?;
    let segments: Vec<&str> = target.path().trim_matches('/').split('/').collect();
    let named = match segments.as_slice() {
        ["messages", id, "edit"] => Some((*id).to_string()),
        ["topics", _] => target
            .query_pairs()
            .find(|(name, _)| name == "expanded_draft")
            .map(|(_, value)| value.into_owned()),
        _ => None,
    }?;
    named.parse().ok().filter(|id| *id > 0)
}

/// Whether HEY answered by sending the caller to the topic's removal confirmation page,
/// which is how it says it will not trash a shared topic unasked. Any other redirect is HEY
/// sending the caller back to where the topic was, with the trashing done.
fn awaiting_confirmation(response: &Response, topic_id: i64) -> bool {
    match response.header("location") {
        Some(location) => response
            .url
            .join(location)
            .is_ok_and(|target| target.path() == format!("/topics/{topic_id}/removal/new")),
        None => false,
    }
}
