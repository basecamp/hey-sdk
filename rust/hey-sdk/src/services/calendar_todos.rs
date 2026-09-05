//! Writing calendar todos with the day sent as a bare date.
//!
//! `starts_at` goes on the wire as `YYYY-MM-DD`, which HEY casts in the reader's time zone.
//! The generated [`CalendarTodoPayload`](crate::models::CalendarTodoPayload) types it as an
//! instant instead, and an instant at UTC midnight lands on the previous day once HEY casts
//! it — so these two send the body themselves rather than through the generated payload.

use serde_json::{Map, Value, json};

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::Recording;
use crate::types::Date;

pub use crate::generated::services::calendar_todos::*;

/// What an edit changes about a todo. A field left unset is left alone: HEY applies what it
/// is sent and keeps the rest, so a rename carries a title and says nothing about the day.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct TodoChanges {
    pub title: Option<String>,
    pub starts_at: Option<Date>,
    pub focused: Option<bool>,
}

impl<'a> CalendarTodos<'a> {
    /// Creates a todo, filed on a day. No day files it on today where this machine is.
    pub async fn create_todo(
        &self,
        title: &str,
        starts_at: Option<Date>,
    ) -> Result<Recording, Error> {
        let starts_at = starts_at.unwrap_or_else(Date::today);
        let body = json!({ "calendar_todo": { "title": title, "starts_at": starts_at } });

        let mut operation = self.client().operation(&routes::CREATE_CALENDAR_TODO, &[]);
        operation.json(&body)?;
        self.client().send(operation).await
    }

    /// Edits a todo. `todo_id` is the recording's id.
    ///
    /// Changing nothing is refused rather than sent: an empty payload asks HEY to do nothing
    /// and answers as though it had done something.
    pub async fn update_todo(
        &self,
        todo_id: i64,
        changes: &TodoChanges,
    ) -> Result<Recording, Error> {
        let fields = changed_fields(changes);
        if fields.is_empty() {
            return Err(Error::usage(format!(
                "update calendar todo {todo_id}: nothing to change"
            )));
        }

        let mut operation = self
            .client()
            .operation(&routes::UPDATE_CALENDAR_TODO, &[&todo_id]);
        operation.resource_id(todo_id);
        operation.json(&json!({ "calendar_todo": fields }))?;
        self.client().send(operation).await
    }
}

fn changed_fields(changes: &TodoChanges) -> Map<String, Value> {
    let mut fields = Map::new();
    // An empty title is no title: HEY refuses a todo without one, so a `Some("")` is left
    // out and reads as changing nothing rather than as clearing it.
    if let Some(title) = changes.title.as_deref().filter(|title| !title.is_empty()) {
        fields.insert("title".to_string(), json!(title));
    }
    if let Some(starts_at) = changes.starts_at {
        fields.insert("starts_at".to_string(), json!(starts_at));
    }
    if let Some(focused) = changes.focused {
        fields.insert("focused".to_string(), json!(focused));
    }
    fields
}
