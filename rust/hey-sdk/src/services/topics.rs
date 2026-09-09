//! Trashing a topic, including the confirmation HEY asks for before it trashes a shared
//! one.
//!
//! [`Topics::get_entries`] is paged by geared pagination, so its `page` is a cursor out of
//! the previous answer's `Link` header rather than an offset: a number is ignored and
//! answered with the first page. [`crate::Page::next_page`] carries the cursor to pass back.

use crate::client::Response;
use crate::error::Error;
use crate::generated::routes;

pub use crate::generated::services::topics::*;

impl<'a> Topics<'a> {
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
