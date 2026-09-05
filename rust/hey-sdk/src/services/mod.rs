//! The service handles a [`crate::Client`] hands out, one per resource. Their methods are
//! generated from the model; the modules here add the hand-written conveniences on top and
//! re-export the generated service they extend.
//!
//! A hand-written method keeps its plain name where the generated service leaves it free.
//! Where the generated method already holds that name, the hand-written one takes the name
//! the model gives the operation it sends: `MarkPostingsSeen` is
//! [`postings::Postings::mark_postings_seen`], which takes the posting ids, alongside the
//! generated `mark_seen`, which takes the request body.
//!
//! A convenience that changes the shape of the call — different arguments, a different
//! result, or a refusal the generated method leaves to HEY — may take a descriptive name
//! instead, since the two are not the same call under one name.
//! [`time_tracks::TimeTracks::start_tracking`] answers a running track's 409 as a conflict
//! the caller can branch on; [`calendars::Calendars::toggle_selection`] answers the
//! selection rather than the payload holding it; [`messages::Messages::send`] takes a
//! message rather than a request body; [`identity::Identity::set_first_week_day`] and
//! [`identity::Identity::set_time_format`] take and answer the setting itself.
//!
//! The parts of HEY with no JSON surface go out as browser forms, built with
//! [`crate::Client::form`] and sent with [`crate::Client::send_form`]. The model describes
//! none of those paths, so such a call says what it means with [`write_info`].

pub mod attachments;
pub mod boxes;
pub mod bulk_replies;
pub mod calendar_changes;
pub mod calendar_events;
pub mod calendar_periods;
pub mod calendar_todos;
pub mod calendars;
pub mod clearances;
pub mod clips;
pub mod collections;
pub mod contacts;
pub mod designations;
pub mod entries;
pub mod extenzions;
pub mod habits;
pub mod identity;
pub mod journal;
pub mod messages;
pub mod postings;
pub mod publications;
pub mod search;
pub mod snippets;
pub mod stickies;
pub mod time_tracks;
pub mod topics;
pub mod workflows;
pub mod world;

use std::borrow::Cow;

use crate::observability::OperationInfo;

pub use crate::generated::services::*;
pub use boxes::{BoxKind, BoxKinds};
pub use bulk_replies::undo_send_id;
pub use calendar_changes::{
    CalendarChanges, CalendarChangesCursor, DeletedCalendar, DeletedRecording, RecordingChanges,
};
pub use calendar_events::{
    CalendarEventUpdate, Countdown, CountdownUnit, CreateCalendarEventParams, EventContent,
    OccurrenceId, OccurrenceScope, Repeat, RepeatFrequency, RepeatUntil, UpdateCalendarEventParams,
    UpdateOccurrenceParams,
};
pub use calendar_todos::TodoChanges;
pub use calendars::{CalendarList, ListedCalendar};
pub use clearances::{ClearanceStatus, ScreenOptions};
pub use collections::{CreateCollectionParams, UpdateCollectionParams};
pub use contacts::{ContactConflict, ContactParams};
pub use entries::ReplyContent;
pub use extenzions::{CreateExtenzionParams, Extenzion, UpdateExtenzionParams};
pub use habits::HabitParams;
pub use identity::TimeFormat;
pub use messages::{DeliverySchedule, DraftContent, MessageContent};
pub use postings::{BubbleUpSlot, PostingChanges, PostingChangesCursor};
pub use search::{SearchParams, SearchResults};
pub use stickies::{MAX_STICKIES_LIMIT, MAX_STICKY_POSITION, StickySize};
pub use workflows::WorkflowSummary;
pub use world::WORLD_ADDRESS;

/// What a write to a path outside the model announces itself as. Build the request itself
/// with [`crate::Client::form`] and hand it one of these with
/// [`crate::Operation::info`].
///
/// ```no_run
/// # use hey_sdk::services::write_info;
/// # use reqwest::Method;
/// # async fn rename(client: &hey_sdk::Client, workflow_id: i64) -> Result<(), hey_sdk::Error> {
/// let mut operation = client.form(Method::PATCH, &format!("/workflows/{workflow_id}"))?;
/// operation.info(write_info(
///     "Workflows",
///     "UpdateWorkflow",
///     "workflow",
///     Some(workflow_id),
/// ));
/// operation.form(&[("workflow[name]", "Launch")]);
/// client.send_form(operation).await?;
/// # Ok(())
/// # }
/// ```
pub fn write_info(
    service: &'static str,
    operation: &'static str,
    resource_type: &'static str,
    resource_id: Option<i64>,
) -> OperationInfo {
    OperationInfo {
        service: Cow::Borrowed(service),
        operation: Cow::Borrowed(operation),
        resource_type: Cow::Borrowed(resource_type),
        is_mutation: true,
        resource_id,
    }
}
