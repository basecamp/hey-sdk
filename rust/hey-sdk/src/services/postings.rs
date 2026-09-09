//! Acting on a selection of postings — seen, moved, trashed, muted, filed, bubbled up —
//! and following a box's changes feed.
//!
//! Every HEY posting endpoint is a bulk one, so the methods here take the ids of the
//! postings to act on. An empty selection is refused before anything is sent.

use url::Url;

use crate::client::Response;
use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::{
    AddPostingsToBoxGroupRequestContent, CreateFolderForPostingsRequestContent, DeletedPosting,
    FilePostingsRequestContent, FolderPayload, GetBoxPostingChangesResponseContent,
    MarkPostingsRequestContent, MovePostingsRequestContent, Posting,
    SchedulePostingsBubbleUpRequestContent, TrashPostingsRequestContent,
};
use crate::operation::Operation;
use crate::pagination::next_link;
use crate::route::Route;
use crate::security::is_same_origin;
use crate::services::boxes::BoxKind;
use crate::types::Date;

pub use crate::generated::services::postings::*;

/// The status HEY answers when the cursor is too far behind for an increment to carry the
/// difference: read the box in full instead.
const TOO_FAR_BEHIND: u16 = 409;

/// When a posting bubbles back up.
///
/// HEY resurfaces a posting at its morning hour of the day the slot names —
/// [`BubbleUpSlot::LaterToday`] at its evening hour of the current day instead — and reads
/// both hours in UTC, like every hour it takes out of a JSON request.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BubbleUpSlot {
    LaterToday,
    Tomorrow,
    /// Saturday.
    ThisWeekend,
    /// Monday.
    NextWeek,
    /// A day of the caller's choosing. HEY does not refuse one that has already passed —
    /// those postings bubble up on the next run of its scheduler.
    Custom(Date),
}

impl BubbleUpSlot {
    pub fn as_str(&self) -> &'static str {
        match self {
            BubbleUpSlot::LaterToday => "today",
            BubbleUpSlot::Tomorrow => "tomorrow",
            BubbleUpSlot::ThisWeekend => "weekend",
            BubbleUpSlot::NextWeek => "next_week",
            BubbleUpSlot::Custom(_) => "custom",
        }
    }

    fn date(&self) -> Option<String> {
        match self {
            BubbleUpSlot::Custom(date) => Some(date.to_string()),
            _ => None,
        }
    }
}

/// Where a read of a box's changes feed starts.
///
/// `since` is an ISO 8601 timestamp with milliseconds and is exclusive; `version` is the
/// contract version the caller speaks. A box's `posting_changes_url` carries the pair to
/// begin with — read it with [`PostingChangesCursor::from_url`] rather than picking the
/// query apart.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct PostingChangesCursor {
    pub since: String,
    pub version: Option<String>,
    pub page: Option<String>,
    pub per_page: Option<String>,
}

impl PostingChangesCursor {
    /// Reads a cursor out of a changes URL HEY issued, either a box's `posting_changes_url`
    /// or a `Link` header the feed answered with.
    pub fn from_url(changes_url: &str) -> Result<PostingChangesCursor, Error> {
        let url = Url::parse(changes_url).map_err(|error| {
            Error::usage(format!(
                "failed to read changes URL {changes_url:?}: {error}"
            ))
        })?;
        let mut cursor = PostingChangesCursor::default();
        for (name, value) in url.query_pairs() {
            match name.as_ref() {
                "since" => cursor.since = value.into_owned(),
                "v" => cursor.version = Some(value.into_owned()),
                "page" => cursor.page = Some(value.into_owned()),
                "per_page" => cursor.per_page = Some(value.into_owned()),
                _ => {}
            }
        }
        Ok(cursor)
    }
}

/// Everything that happened to a box's postings since a cursor.
///
/// `next_page` is set while this increment has more pages to read now. `next_cursor` is set
/// on the last page and is where the next read resumes; it is `None` when nothing changed,
/// in which case the cursor that produced this page still stands. `full_sync_required` is
/// set when the cursor is too far behind for an increment to carry the difference, and the
/// box has to be read in full instead.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct PostingChanges {
    pub added: Vec<Posting>,
    pub updated: Vec<Posting>,
    pub deleted: Vec<DeletedPosting>,
    pub next_page: Option<PostingChangesCursor>,
    pub next_cursor: Option<PostingChangesCursor>,
    pub full_sync_required: bool,
}

impl<'a> Postings<'a> {
    /// Marks a selection of postings as seen.
    pub async fn mark_postings_seen(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.mark(&routes::MARK_POSTINGS_SEEN, posting_ids).await
    }

    /// Marks a selection of postings as unseen.
    pub async fn mark_postings_unseen(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.mark(&routes::MARK_POSTINGS_UNSEEN, posting_ids).await
    }

    /// Moves a selection of postings to a box. Box ids come from [`Boxes::list`].
    ///
    /// [`Boxes::list`]: crate::services::boxes::Boxes::list
    pub async fn move_to_box(&self, box_id: i64, posting_ids: &[i64]) -> Result<(), Error> {
        let mut operation = self.selection(&routes::MOVE_POSTINGS, posting_ids)?;
        operation.json(&MovePostingsRequestContent {
            posting_ids: posting_ids.to_vec(),
            box_id,
        })?;
        self.client().send_unit(operation).await
    }

    /// Moves a selection of postings to the box of a kind, resolving the kind through
    /// [`Boxes::id_by_kind`], which answers from the one reading of the box index the
    /// client keeps. Only the first move by kind costs that read.
    ///
    /// [`Boxes::id_by_kind`]: crate::services::boxes::Boxes::id_by_kind
    pub async fn move_to_kind(&self, kind: BoxKind, posting_ids: &[i64]) -> Result<(), Error> {
        require_ids(posting_ids)?;
        let box_id = self.client().boxes().id_by_kind(kind).await?;
        self.move_to_box(box_id, posting_ids).await
    }

    /// Moves a selection of postings to the Imbox.
    pub async fn move_to_imbox(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.move_to_kind(BoxKind::Imbox, posting_ids).await
    }

    /// Moves a selection of postings to The Feed.
    pub async fn move_to_feed(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.move_to_kind(BoxKind::Feed, posting_ids).await
    }

    /// Moves a selection of postings to Set Aside.
    pub async fn move_to_set_aside(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.move_to_kind(BoxKind::SetAside, posting_ids).await
    }

    /// Moves a selection of postings to Reply Later.
    pub async fn move_to_reply_later(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.move_to_kind(BoxKind::ReplyLater, posting_ids).await
    }

    /// Moves a selection of postings to the Paper Trail.
    pub async fn move_to_paper_trail(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.move_to_kind(BoxKind::PaperTrail, posting_ids).await
    }

    /// Moves a selection of postings to the trash. On a shared topic HEY removes your own
    /// access rather than trashing the thread for everybody on it.
    pub async fn move_to_trash(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.trash_selection(None, posting_ids).await
    }

    /// Moves a selection of postings to the trash, and trashes a shared topic for everybody
    /// on it rather than only dropping your own access.
    pub async fn trash_for_everyone(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.trash_selection(Some("false"), posting_ids).await
    }

    /// Mutes a selection of postings, so their threads stop notifying.
    pub async fn mute_postings(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.mark(&routes::MUTE_POSTINGS, posting_ids).await
    }

    /// Unmutes a selection of postings.
    pub async fn unmute_postings(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.by_ids(&routes::UNMUTE_POSTINGS, posting_ids).await
    }

    /// Marks a selection of postings as spam. Past ten postings HEY hands the work to a
    /// background job, so the call comes back before the postings have moved.
    pub async fn mark_postings_spam(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.mark(&routes::MARK_POSTINGS_SPAM, posting_ids).await
    }

    /// Files a selection of postings into an existing Set Aside group.
    pub async fn add_postings_to_box_group(
        &self,
        box_id: i64,
        box_group_id: i64,
        posting_ids: &[i64],
    ) -> Result<(), Error> {
        let mut operation = self.selection(&routes::ADD_POSTINGS_TO_BOX_GROUP, posting_ids)?;
        operation.json(&AddPostingsToBoxGroupRequestContent {
            posting_ids: posting_ids.to_vec(),
            box_id,
            box_group_id,
        })?;
        self.client().send_unit(operation).await
    }

    /// Takes a selection of postings out of whatever Set Aside group they are in.
    pub async fn remove_postings_from_box_group(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.by_ids(&routes::REMOVE_POSTINGS_FROM_BOX_GROUP, posting_ids)
            .await
    }

    /// Labels a selection of postings with an existing folder.
    pub async fn file_postings(&self, folder_id: i64, posting_ids: &[i64]) -> Result<(), Error> {
        let mut operation = self.selection(&routes::FILE_POSTINGS, posting_ids)?;
        operation.json(&FilePostingsRequestContent {
            posting_ids: posting_ids.to_vec(),
            folder_id,
        })?;
        self.client().send_unit(operation).await
    }

    /// Takes a label off a selection of postings, or every label when `folder_id` is 0.
    ///
    /// Zero is not "every folder" to HEY, it is a folder that does not exist, so it is left
    /// out of the request rather than sent.
    pub async fn unfile_postings(&self, folder_id: i64, posting_ids: &[i64]) -> Result<(), Error> {
        let mut operation = self.selection(&routes::UNFILE_POSTINGS, posting_ids)?;
        operation.query("posting_ids", join_ids(posting_ids));
        if folder_id != 0 {
            operation.query("folder_id", folder_id);
        }
        self.client().send_unit(operation).await
    }

    /// Creates a folder and files a selection of postings into it. HEY serves no JSON
    /// endpoint for creating a folder on its own.
    pub async fn create_folder_for_postings(
        &self,
        name: &str,
        posting_ids: &[i64],
    ) -> Result<(), Error> {
        let mut operation = self.selection(&routes::CREATE_FOLDER_FOR_POSTINGS, posting_ids)?;
        operation.json(&CreateFolderForPostingsRequestContent {
            posting_ids: posting_ids.to_vec(),
            folder: FolderPayload {
                name: name.to_string(),
                status: None,
            },
        })?;
        self.client().send_unit(operation).await
    }

    /// Bubbles a selection of postings up right away.
    pub async fn bubble_up_postings_now(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.mark(&routes::BUBBLE_UP_POSTINGS_NOW, posting_ids)
            .await
    }

    /// Schedules a selection of postings to bubble back up at a slot.
    pub async fn schedule_postings_bubble_up(
        &self,
        slot: BubbleUpSlot,
        posting_ids: &[i64],
    ) -> Result<(), Error> {
        let mut operation = self.selection(&routes::SCHEDULE_POSTINGS_BUBBLE_UP, posting_ids)?;
        operation.json(&SchedulePostingsBubbleUpRequestContent {
            posting_ids: posting_ids.to_vec(),
            slot: slot.as_str().to_string(),
            date: slot.date(),
        })?;
        self.client().send_unit(operation).await
    }

    /// Drops the scheduled bubble up on a selection of postings.
    pub async fn cancel_postings_bubble_up(&self, posting_ids: &[i64]) -> Result<(), Error> {
        self.by_ids(&routes::CANCEL_POSTINGS_BUBBLE_UP, posting_ids)
            .await
    }

    /// Reads a box's changes feed from a cursor to the end of the increment, following the
    /// pages HEY names.
    ///
    /// A full sync comes back as soon as HEY asks for one, with whatever was read before it
    /// dropped: the box has to be read in full anyway. Reading stops at the client's page
    /// limit, and the answer then carries the cursor of the last page read rather than the
    /// end of the feed.
    pub async fn all_changes(
        &self,
        box_id: i64,
        cursor: &PostingChangesCursor,
    ) -> Result<PostingChanges, Error> {
        let mut all = PostingChanges::default();
        let mut cursor = cursor.clone();
        for _ in 0..self.client().max_pages() {
            let mut changes = self.changes(box_id, &cursor).await?;
            if changes.full_sync_required {
                return Ok(changes);
            }
            all.added.append(&mut changes.added);
            all.updated.append(&mut changes.updated);
            all.deleted.append(&mut changes.deleted);
            all.next_cursor = changes.next_cursor;
            match changes.next_page {
                Some(next) => cursor = next,
                None => return Ok(all),
            }
        }
        tracing::warn!(
            max_pages = self.client().max_pages(),
            "posting changes pagination capped"
        );
        Ok(all)
    }

    /// Reads one page of what changed among a box's postings since a cursor.
    ///
    /// This is the incremental sync feed the mail clients follow rather than re-reading a
    /// box. HEY answers 409 when the cursor is too far behind for an increment to carry the
    /// difference, which comes back as a `full_sync_required` answer rather than a failure:
    /// read the box in full instead. The hooks still see the 409 for what it was.
    pub async fn changes(
        &self,
        box_id: i64,
        cursor: &PostingChangesCursor,
    ) -> Result<PostingChanges, Error> {
        if cursor.since.is_empty() {
            return Err(Error::usage(
                "a since cursor is required — start from the box's posting_changes_url",
            ));
        }

        let mut operation = self
            .client()
            .operation(&routes::GET_BOX_POSTING_CHANGES, &[&box_id]);
        operation.resource_id(box_id);
        operation.query("since", &cursor.since);
        operation.query_optional("v", cursor.version.as_ref());
        operation.query_optional("page", cursor.page.as_ref());
        operation.query_optional("per_page", cursor.per_page.as_ref());
        // A cursor URL never repeats, so a cached answer would never be revalidated and a
        // long-running watch would grow the cache one dead entry per read.
        operation.no_cache();

        let response = match self.client().execute(operation).await {
            Ok(response) => response,
            Err(error) if error.http_status() == Some(TOO_FAR_BEHIND) => {
                return Ok(PostingChanges {
                    full_sync_required: true,
                    ..PostingChanges::default()
                });
            }
            Err(error) => return Err(error),
        };

        let body: GetBoxPostingChangesResponseContent = response.json()?;
        let mut changes = PostingChanges {
            added: body.added.unwrap_or_default(),
            updated: body.updated.unwrap_or_default(),
            deleted: body.deleted.unwrap_or_default(),
            ..PostingChanges::default()
        };
        if let Some(next) = self.next_cursor(&response)? {
            // The feed names a page while an increment has more of them to read, and a
            // fresh since cursor on the last one.
            if next.page.is_some() {
                changes.next_page = Some(next);
            } else {
                changes.next_cursor = Some(next);
            }
        }
        Ok(changes)
    }

    async fn mark(&self, route: &'static Route, posting_ids: &[i64]) -> Result<(), Error> {
        let mut operation = self.selection(route, posting_ids)?;
        operation.json(&MarkPostingsRequestContent {
            posting_ids: posting_ids.to_vec(),
        })?;
        self.client().send_unit(operation).await
    }

    /// Sends the selection in the query, comma-joined, for the endpoints whose method
    /// carries no body.
    async fn by_ids(&self, route: &'static Route, posting_ids: &[i64]) -> Result<(), Error> {
        let mut operation = self.selection(route, posting_ids)?;
        operation.query("posting_ids", join_ids(posting_ids));
        self.client().send_unit(operation).await
    }

    /// A `remove_access` of `None` is left out of the request, which HEY reads as removing
    /// only your own access from a shared topic.
    async fn trash_selection(
        &self,
        remove_access: Option<&str>,
        posting_ids: &[i64],
    ) -> Result<(), Error> {
        let mut operation = self.selection(&routes::TRASH_POSTINGS, posting_ids)?;
        operation.json(&TrashPostingsRequestContent {
            posting_ids: posting_ids.to_vec(),
            remove_access: remove_access.map(str::to_string),
        })?;
        self.client().send_unit(operation).await
    }

    /// The operation for a bulk endpoint: an empty selection is refused before anything is
    /// sent, and a selection of one names the posting it acts on.
    fn selection(&self, route: &'static Route, posting_ids: &[i64]) -> Result<Operation, Error> {
        require_ids(posting_ids)?;
        let mut operation = self.client().operation(route, &[]);
        if let [posting_id] = posting_ids {
            operation.resource_id(*posting_id);
        }
        Ok(operation)
    }

    fn next_cursor(&self, response: &Response) -> Result<Option<PostingChangesCursor>, Error> {
        match response.header("link").and_then(next_link) {
            None => Ok(None),
            Some(target) => {
                let next = response.url.join(&target)?;
                if is_same_origin(&next, self.client().base_url()) {
                    PostingChangesCursor::from_url(next.as_str()).map(Some)
                } else {
                    Err(Error::usage(format!(
                        "changes Link header points to a different origin: {next}"
                    )))
                }
            }
        }
    }
}

fn require_ids(posting_ids: &[i64]) -> Result<(), Error> {
    if posting_ids.is_empty() {
        Err(Error::usage("at least one posting ID is required"))
    } else {
        Ok(())
    }
}

fn join_ids(posting_ids: &[i64]) -> String {
    posting_ids
        .iter()
        .map(i64::to_string)
        .collect::<Vec<_>>()
        .join(",")
}
