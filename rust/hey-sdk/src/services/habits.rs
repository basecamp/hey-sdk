//! Starting and editing a habit.
//!
//! Both writes answer the habit as a recording: HEY renders it as JSON on create and on
//! update. There is no redirect fallback, so a caller never sees a "success" that wrote
//! nothing.

use crate::error::Error;
use crate::generated::types::{HabitPayload, HabitRequestContent, Recording};

pub use crate::generated::services::habits::*;

/// A habit, as its writes take it.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct HabitParams {
    pub name: String,
    pub icon: String,
    pub color: String,
    /// The days of the week the habit runs on, 0 for Sunday through 6 for Saturday.
    pub days: Vec<i32>,
}

impl<'a> Habits<'a> {
    /// Starts a new habit and answers it as a recording.
    pub async fn create_habit(&self, params: &HabitParams) -> Result<Recording, Error> {
        self.create(&habit_body(params)).await
    }

    /// Edits a habit and answers it as a recording. `habit_id` is the recording's id, and
    /// fields left empty are kept.
    pub async fn update_habit(
        &self,
        habit_id: i64,
        params: &HabitParams,
    ) -> Result<Recording, Error> {
        self.update(habit_id, &habit_body(params)).await
    }
}

fn habit_body(params: &HabitParams) -> HabitRequestContent {
    HabitRequestContent {
        calendar_habit: HabitPayload {
            name: omit_empty(&params.name),
            icon: omit_empty(&params.icon),
            color: omit_empty(&params.color),
            days: if params.days.is_empty() {
                None
            } else {
                Some(params.days.clone())
            },
        },
    }
}

fn omit_empty(value: &str) -> Option<String> {
    if value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}
