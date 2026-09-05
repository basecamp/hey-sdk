//! Turning a thread into a public web page, on top of the generated publication route.
//!
//! Publishing has no JSON surface: both writes are browser form posts that redirect, and
//! the public link only appears once the publication is read back.

use reqwest::Method;

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::TopicPublication;
use crate::services::write_info;

pub use crate::generated::services::publications::*;

impl<'a> Publications<'a> {
    /// Publishes a thread and answers its public link.
    ///
    /// The redirect lands on the sharing panel rather than carrying the link, so the
    /// publication is read back. That read is a [quiet](crate::Operation::quiet) one, so the
    /// hooks hear `Publications.CreateTopicPublication` once and see both requests under it,
    /// as they do in Go. The operation's own end still lands after the first of the two, the
    /// SDK having no seam for wrapping a block of them.
    ///
    /// HEY answers a forbidden error on accounts that are not eligible to publish.
    pub async fn publish(&self, topic_id: i64) -> Result<TopicPublication, Error> {
        let mut operation = self
            .client()
            .form(Method::POST, &format!("/topics/{topic_id}/publication"))?;
        operation.info(write_info(
            "Publications",
            "CreateTopicPublication",
            "publication",
            Some(topic_id),
        ));
        operation.form(&[]);
        self.client().send_unit(operation).await?;

        let mut read = self
            .client()
            .operation(&routes::GET_TOPIC_PUBLICATION, &[&topic_id]);
        read.quiet();
        self.client().send(read).await
    }

    /// Unpublishes a thread, breaking its public link.
    pub async fn unpublish(&self, topic_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::DELETE, &format!("/topics/{topic_id}/publication"))?;
        operation.info(write_info(
            "Publications",
            "DeleteTopicPublication",
            "publication",
            Some(topic_id),
        ));
        self.client().send_unit(operation).await
    }
}
