//! The journal entry a day holds, read and written as its content.
//!
//! A day has at most one entry. HEY answers it as a calendar recording carrying the full
//! text (`content`) and the rich-text HTML (`content_html`); a day with no entry answers
//! 204 and no body, which reads here as `None`.

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::{JournalEntryPayload, Recording, UpdateJournalEntryRequestContent};
use crate::operation::Operation;

pub use crate::generated::services::journal::*;

impl<'a> Journal<'a> {
    /// The rich-text HTML of the day's journal entry, falling back to its plain text, or
    /// `None` when the day has no entry. `day` is `YYYY-MM-DD`.
    ///
    /// The fallback covers an empty `content_html` as well as a missing one: HEY serves the
    /// key blank on an entry it has no rendered body for, and blank is not the entry.
    pub async fn get_content(&self, day: &str) -> Result<Option<String>, Error> {
        let mut operation = self.client().operation(&routes::GET_JOURNAL_ENTRY, &[&day]);
        operation.operation_name("GetJournalContent");
        let entry = self.recording(operation).await?;
        Ok(entry.and_then(|entry| {
            entry
                .content_html
                .filter(|html| !html.is_empty())
                .or(entry.content)
        }))
    }

    /// The day's journal entry, or `None` when it has none. The generated
    /// [`Journal::get_entry`] reads the same route but takes the empty answer for a day
    /// without an entry as a body it could not decode.
    pub async fn entry(&self, day: &str) -> Result<Option<Recording>, Error> {
        let operation = self.client().operation(&routes::GET_JOURNAL_ENTRY, &[&day]);
        self.recording(operation).await
    }

    /// Writes the day's journal entry, creating it if needed, and answers it as a
    /// recording. Empty content removes the entry, which HEY answers with nothing, so the
    /// result is `None`.
    pub async fn update_content(
        &self,
        day: &str,
        content: &str,
    ) -> Result<Option<Recording>, Error> {
        let body = UpdateJournalEntryRequestContent {
            calendar_journal_entry: JournalEntryPayload {
                content: content.to_string(),
            },
        };
        let mut operation = self
            .client()
            .operation(&routes::UPDATE_JOURNAL_ENTRY, &[&day]);
        operation.json(&body)?;
        self.recording(operation).await
    }

    async fn recording(&self, operation: Operation) -> Result<Option<Recording>, Error> {
        let response = self.client().execute(operation).await?;
        if response.body.is_empty() {
            Ok(None)
        } else {
            response.json().map(Some)
        }
    }
}
