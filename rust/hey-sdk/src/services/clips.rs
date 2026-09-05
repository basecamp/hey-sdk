//! Saving and throwing away clips, on top of the generated clip route.
//!
//! Clips have no JSON surface for writes: a write answers with a Turbo Stream rather than a
//! record, so both of them are browser form posts.

use reqwest::Method;

use crate::error::Error;
use crate::services::write_info;

pub use crate::generated::services::clips::*;

impl<'a> Clips<'a> {
    /// Clips a piece of an entry, so it can be found again without the thread.
    pub async fn create(&self, entry_id: i64, content: &str) -> Result<(), Error> {
        let entry = entry_id.to_string();
        let mut operation = self.client().form(Method::POST, "/clips")?;
        operation.info(write_info("Clips", "CreateClip", "clip", Some(entry_id)));
        operation.form(&[
            ("clip[entry_id]", entry.as_str()),
            ("clip[content]", content),
        ]);
        self.client().send_unit(operation).await
    }

    /// Throws a clip away.
    pub async fn delete(&self, clip_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::DELETE, &format!("/clips/{clip_id}"))?;
        operation.info(write_info("Clips", "DeleteClip", "clip", Some(clip_id)));
        self.client().send_unit(operation).await
    }
}
