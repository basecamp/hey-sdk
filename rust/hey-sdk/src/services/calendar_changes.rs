//! Following the calendar changes feeds: what changed since a cursor, and where to resume.
//!
//! There are two of them and they speak the same cursor — the calendar-level feed behind a
//! [`CalendarList`](crate::services::CalendarList)'s `calendar_changes_url`, and each
//! calendar's own recording feed behind its
//! [`ListedCalendar`]'s `recording_changes_url`.
//!
//! Neither feed's answers are cached. A cursor URL never repeats, so a cached response would
//! never be revalidated, and a long-running watch would grow the cache by one dead entry per
//! read.

use std::borrow::Cow;
use std::collections::{BTreeMap, HashSet};

use reqwest::Method;
use serde::{Deserialize, Serialize};
use url::Url;

use crate::client::Response;
use crate::error::Error;
use crate::generated::services::calendars::Calendars;
use crate::generated::types::{Calendar, Recording};
use crate::observability::OperationInfo;
use crate::operation::Operation;
use crate::pagination::next_link;
use crate::security::is_same_origin;
use crate::services::calendars::ListedCalendar;
use crate::types::DateTime;

/// The recording feed answers 409 when the cursor is too far behind for an increment to
/// carry the difference, or speaks a version the feed no longer does. Both mean "read the
/// calendar in full", which comes back as an answer rather than a failure.
const TOO_FAR_BEHIND: u16 = 409;

/// Where a read of a changes feed starts. `since` is an ISO 8601 timestamp with
/// milliseconds and is exclusive; `version` is the contract version the caller speaks.
///
/// Build one with [`CalendarChangesCursor::from_url`] rather than by hand. The two
/// server-issued URLs differ — a recording changes URL carries `v=1`, which the recording
/// feed refuses to answer without, while a calendar changes URL carries no version at all —
/// so only the server knows which pair its feed wants.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CalendarChangesCursor {
    pub since: Option<String>,
    pub version: Option<String>,
    pub page: Option<String>,
    pub per_page: Option<String>,
}

impl CalendarChangesCursor {
    /// Reads a cursor out of a changes URL the server issued: a calendar list's
    /// `calendar_changes_url`, a listed calendar's `recording_changes_url`, or the `Link`
    /// header either feed answered with.
    pub fn from_url(changes_url: &str) -> Result<CalendarChangesCursor, Error> {
        let url = Url::parse(changes_url)
            .map_err(|error| Error::usage(format!("changes URL {changes_url}: {error}")))?;
        Ok(CalendarChangesCursor::from_parsed(&url))
    }

    fn from_parsed(url: &Url) -> CalendarChangesCursor {
        CalendarChangesCursor {
            since: parameter(url, "since"),
            version: parameter(url, "v"),
            page: parameter(url, "page"),
            per_page: parameter(url, "per_page"),
        }
    }

    /// Renders the cursor onto a request. The version is never invented here: a cursor read
    /// from a server-issued URL carries whichever version that feed speaks.
    fn apply(&self, operation: &mut Operation) {
        operation.query_optional("since", self.since.as_ref());
        operation.query_optional("v", self.version.as_ref());
        operation.query_optional("page", self.page.as_ref());
        operation.query_optional("per_page", self.per_page.as_ref());
    }
}

/// A calendar the changes feed reports gone.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct DeletedCalendar {
    #[serde(default)]
    pub id: i64,
    pub deleted_at: DateTime,
}

/// Everything that happened to the calendar list since a cursor. Added calendars arrive as
/// [`ListedCalendar`], so a new calendar comes with the changes URL and signed stream name
/// a live follower needs.
///
/// `next_page` is set while this increment has more pages to read now. `next_cursor` is set
/// on the last page and is where the next read should resume; it is `None` when nothing
/// changed, in which case the cursor that produced this page still stands. Unlike the
/// recording feed, this one never falls too far behind, so there is no full sync to ask for.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct CalendarChanges {
    pub added: Vec<ListedCalendar>,
    pub updated: Vec<Calendar>,
    pub deleted: Vec<DeletedCalendar>,
    pub next_page: Option<CalendarChangesCursor>,
    pub next_cursor: Option<CalendarChangesCursor>,
}

/// A recording the changes feed reports gone. `type` is the recordable type key the
/// recording was grouped under while it existed.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct DeletedRecording {
    #[serde(default)]
    pub id: i64,
    pub deleted_at: DateTime,
    #[serde(default)]
    pub r#type: String,
}

/// Everything that happened to a calendar's recordings since a cursor.
///
/// `added` and `updated` keep the wire's grouping by recordable type key —
/// `Calendar::Event`, `Calendar::Habit`, `Calendar::Habit::Completion`,
/// `Calendar::DayTitle`, `Calendar::DayBackground`, `Calendar::TimeTrack`,
/// `Calendar::Todo`, `Calendar::Countdown`, `Calendar::JournalEntry` — the server owns that
/// vocabulary. `deleted` is one deduplicated list instead: the wire groups deletions by
/// type key too, but repeats the whole deleted collection under every key it groups, so the
/// map shape carries nothing beyond each record's own `type`, which is authoritative.
///
/// `next_page` is set while this increment has more pages to read now. `next_cursor` is set
/// on the last page and is where the next read should resume; it is `None` when nothing
/// changed, in which case the cursor that produced this page still stands.
/// `full_sync_required` is set when the cursor is too far behind for an increment to carry
/// the difference — or speaks a version the feed no longer does — and the calendar has to be
/// read in full instead.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct RecordingChanges {
    pub added: BTreeMap<String, Vec<Recording>>,
    pub updated: BTreeMap<String, Vec<Recording>>,
    pub deleted: Vec<DeletedRecording>,
    pub next_page: Option<CalendarChangesCursor>,
    pub next_cursor: Option<CalendarChangesCursor>,
    pub full_sync_required: bool,
}

impl<'a> Calendars<'a> {
    /// Reads the calendar changes feed from a cursor to its end, following the pages the
    /// feed hands out up to the client's page limit.
    pub async fn all_calendar_changes(
        &self,
        cursor: &CalendarChangesCursor,
    ) -> Result<CalendarChanges, Error> {
        let mut all = CalendarChanges::default();
        let mut cursor = cursor.clone();

        for _ in 0..self.client().max_pages() {
            let mut changes = self.calendar_changes(&cursor).await?;
            all.added.append(&mut changes.added);
            all.updated.append(&mut changes.updated);
            all.deleted.append(&mut changes.deleted);
            all.next_cursor = changes.next_cursor;

            match changes.next_page {
                None => return Ok(all),
                Some(next) => cursor = next,
            }
        }

        tracing::warn!(
            max_pages = self.client().max_pages(),
            "calendar changes pagination capped"
        );
        Ok(all)
    }

    /// Reads one page of the calendar changes feed.
    pub async fn calendar_changes(
        &self,
        cursor: &CalendarChangesCursor,
    ) -> Result<CalendarChanges, Error> {
        if cursor.since.is_none() {
            return Err(Error::usage(
                "a since cursor is required — start from the list's calendar_changes_url",
            ));
        }

        let mut operation = self.client().request(Method::GET, "/calendar/changes");
        operation.info(changes_info("GetCalendarChanges", "calendar"));
        cursor.apply(&mut operation);
        operation.no_cache();

        let response = self.client().execute(operation).await?;
        let payload: CalendarChangesPayload = response.json()?;
        let (next_page, next_cursor) = next_cursors(&response, self.client().base_url())?;
        Ok(CalendarChanges {
            added: payload.added,
            updated: payload.updated,
            deleted: payload.deleted,
            next_page,
            next_cursor,
        })
    }

    /// Reads a calendar's recording changes feed from a cursor to its end, following the
    /// pages the feed hands out up to the client's page limit. A cursor the feed has left
    /// behind ends the walk on the spot with `full_sync_required`.
    pub async fn all_recording_changes(
        &self,
        calendar_id: i64,
        cursor: &CalendarChangesCursor,
    ) -> Result<RecordingChanges, Error> {
        let mut all = RecordingChanges::default();
        let mut cursor = cursor.clone();

        for _ in 0..self.client().max_pages() {
            let mut changes = self.recording_changes(calendar_id, &cursor).await?;
            if changes.full_sync_required {
                return Ok(changes);
            }

            merge_recordings(&mut all.added, changes.added);
            merge_recordings(&mut all.updated, changes.updated);
            all.deleted.append(&mut changes.deleted);
            all.next_cursor = changes.next_cursor;

            match changes.next_page {
                None => return Ok(all),
                Some(next) => cursor = next,
            }
        }

        tracing::warn!(
            max_pages = self.client().max_pages(),
            "recording changes pagination capped"
        );
        Ok(all)
    }

    /// Reads one page of a calendar's recording changes feed.
    pub async fn recording_changes(
        &self,
        calendar_id: i64,
        cursor: &CalendarChangesCursor,
    ) -> Result<RecordingChanges, Error> {
        if cursor.since.is_none() {
            return Err(Error::usage(
                "a since cursor is required — start from the calendar's recording_changes_url",
            ));
        }
        if cursor.version.is_none() {
            return Err(Error::usage(
                "a feed version is required — read the calendar's recording_changes_url with CalendarChangesCursor::from_url",
            ));
        }

        let mut operation = self.client().request(
            Method::GET,
            format!("/calendars/{calendar_id}/recording/changes"),
        );
        operation.info(changes_info("GetCalendarRecordingChanges", "recording"));
        operation.resource_id(calendar_id);
        cursor.apply(&mut operation);
        operation.no_cache();

        let response = match self.client().execute(operation).await {
            Ok(response) => response,
            Err(error) if error.http_status() == Some(TOO_FAR_BEHIND) => {
                return Ok(RecordingChanges {
                    full_sync_required: true,
                    ..RecordingChanges::default()
                });
            }
            Err(error) => return Err(error),
        };

        let payload: RecordingChangesPayload = response.json()?;
        let (next_page, next_cursor) = next_cursors(&response, self.client().base_url())?;
        Ok(RecordingChanges {
            added: payload.added,
            updated: payload.updated,
            deleted: flatten_deleted_recordings(payload.deleted),
            next_page,
            next_cursor,
            full_sync_required: false,
        })
    }
}

#[derive(Debug, Default, Deserialize)]
struct CalendarChangesPayload {
    #[serde(default)]
    added: Vec<ListedCalendar>,
    #[serde(default)]
    updated: Vec<Calendar>,
    #[serde(default)]
    deleted: Vec<DeletedCalendar>,
}

#[derive(Debug, Default, Deserialize)]
struct RecordingChangesPayload {
    #[serde(default)]
    added: BTreeMap<String, Vec<Recording>>,
    #[serde(default)]
    updated: BTreeMap<String, Vec<Recording>>,
    #[serde(default)]
    deleted: BTreeMap<String, Vec<DeletedRecording>>,
}

fn changes_info(operation: &'static str, resource_type: &'static str) -> OperationInfo {
    OperationInfo {
        service: Cow::Borrowed("Calendars"),
        operation: Cow::Borrowed(operation),
        resource_type: Cow::Borrowed(resource_type),
        is_mutation: false,
        resource_id: None,
    }
}

/// The cursors the feed's `Link` header names, the page one first: while an increment has
/// more pages the link carries a page cursor, and the last page carries a fresh `since`
/// cursor instead. Both are `None` when there is no link.
fn next_cursors(
    response: &Response,
    base_url: &Url,
) -> Result<(Option<CalendarChangesCursor>, Option<CalendarChangesCursor>), Error> {
    match link_cursor(response, base_url)? {
        None => Ok((None, None)),
        Some(cursor) if cursor.page.is_some() => Ok((Some(cursor), None)),
        Some(cursor) => Ok((None, Some(cursor))),
    }
}

fn link_cursor(
    response: &Response,
    base_url: &Url,
) -> Result<Option<CalendarChangesCursor>, Error> {
    match response.header("link").and_then(next_link) {
        None => Ok(None),
        Some(target) => {
            let next = response.url.join(&target)?;
            if is_same_origin(&next, base_url) {
                Ok(Some(CalendarChangesCursor::from_parsed(&next)))
            } else {
                Err(Error::usage(format!(
                    "changes Link header points to a different origin: {next}"
                )))
            }
        }
    }
}

fn merge_recordings(
    into: &mut BTreeMap<String, Vec<Recording>>,
    from: BTreeMap<String, Vec<Recording>>,
) {
    for (key, recordings) in from {
        into.entry(key).or_default().extend(recordings);
    }
}

/// Folds the wire's per-type deleted buckets into one list. The server repeats the whole
/// deleted collection under every type key it groups, so the same deletion arrives once per
/// key: the id dedupe drops the repeats, and each record's own `type` says what it was.
fn flatten_deleted_recordings(
    buckets: BTreeMap<String, Vec<DeletedRecording>>,
) -> Vec<DeletedRecording> {
    let mut seen = HashSet::new();
    buckets
        .into_values()
        .flatten()
        .filter(|record| seen.insert(record.id))
        .collect()
}

fn parameter(url: &Url, name: &str) -> Option<String> {
    url.query_pairs()
        .find(|(key, _)| key == name)
        .map(|(_, value)| value.into_owned())
        .filter(|value| !value.is_empty())
}
