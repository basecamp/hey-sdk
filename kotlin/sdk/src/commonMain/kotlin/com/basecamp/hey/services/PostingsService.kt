package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Operation
import com.basecamp.hey.Route
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.boxes
import com.basecamp.hey.json
import com.basecamp.hey.generated.models.AddPostingsToBoxGroupRequestContent
import com.basecamp.hey.generated.models.CreateFolderForPostingsRequestContent
import com.basecamp.hey.generated.models.FilePostingsRequestContent
import com.basecamp.hey.generated.models.FolderPayload
import com.basecamp.hey.generated.models.MarkPostingsRequestContent
import com.basecamp.hey.generated.models.MovePostingsRequestContent
import com.basecamp.hey.generated.models.SchedulePostingsBubbleUpRequestContent
import com.basecamp.hey.generated.models.TrashPostingsRequestContent
import com.basecamp.hey.generated.models.GetBoxPostingChangesResponseContent
import com.basecamp.hey.generated.models.DeletedPosting
import com.basecamp.hey.generated.models.Posting
import com.basecamp.hey.isSameOrigin
import com.basecamp.hey.parseAbsoluteUrl
import com.basecamp.hey.generated.services.PostingsService as GeneratedPostingsService

/**
 * Postings service with selection conveniences on top of the generated surface. Every HEY
 * posting endpoint is a bulk one, so the methods here take the ids of the postings to act
 * on; an empty selection is refused before anything is sent.
 */
class PostingsService(client: HeyClient) : GeneratedPostingsService(client) {
    private fun selection(postingIds: List<Long>): List<Long> {
        if (postingIds.isEmpty()) throw HeyException.Usage("at least one posting id is required")
        return postingIds
    }

    /**
     * Sends a bulk route over a selection: an empty one is refused before anything is sent,
     * and a selection of one names the posting it acts on, for the hooks, as Go's and Rust's do.
     */
    private suspend inline fun <reified T> bulk(route: Route, postingIds: List<Long>, body: T) {
        val operation = selection(route, postingIds)
        operation.json(body)
        client.sendUnit(operation)
    }

    /**
     * Sends the selection in the query, comma-joined, for the endpoints whose method carries
     * no body — which is what the generated forms of those operations take as a raw string.
     */
    private suspend fun byIds(route: Route, postingIds: List<Long>) {
        val operation = selection(route, postingIds)
        operation.query("posting_ids", joinIds(postingIds))
        client.sendUnit(operation)
    }

    /** The operation for a bulk route: an empty selection is refused before anything is sent, and a selection of one names the posting it acts on. */
    private fun selection(route: Route, postingIds: List<Long>): Operation {
        selection(postingIds)
        val operation = client.operation(route, emptyList())
        postingIds.singleOrNull()?.let { operation.resourceId(it) }
        return operation
    }

    /** Marks postings seen. The generated [markSeen] takes the request body. */
    suspend fun markPostingsSeen(postingIds: List<Long>) =
        bulk(Routes.MARK_POSTINGS_SEEN, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Marks postings unseen. */
    suspend fun markPostingsUnseen(postingIds: List<Long>) =
        bulk(Routes.MARK_POSTINGS_UNSEEN, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Trashes postings. On a shared topic HEY removes your own access rather than trashing the thread for everybody on it. */
    suspend fun trashPostings(postingIds: List<Long>) = trashSelection(null, postingIds)

    /** Moves postings to the trash: [trashPostings] under the name Go and Rust give it. */
    suspend fun moveToTrash(postingIds: List<Long>) = trashSelection(null, postingIds)

    /** Trashes postings, and trashes a shared topic for everybody on it rather than only dropping your own access. */
    suspend fun trashForEveryone(postingIds: List<Long>) = trashSelection("false", postingIds)

    /** A null `remove_access` is left out of the request, which HEY reads as removing only your own access from a shared topic. */
    private suspend fun trashSelection(removeAccess: String?, postingIds: List<Long>) =
        bulk(Routes.TRASH_POSTINGS, postingIds, TrashPostingsRequestContent(postingIds = selection(postingIds), removeAccess = removeAccess))

    /** Mutes postings, so their threads stop notifying. */
    suspend fun mutePostings(postingIds: List<Long>) =
        bulk(Routes.MUTE_POSTINGS, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Unmutes postings. */
    suspend fun unmutePostings(postingIds: List<Long>) = byIds(Routes.UNMUTE_POSTINGS, postingIds)

    /** Marks postings as spam. Past ten postings HEY hands the work to a background job, so the call comes back before the postings have moved. */
    suspend fun markPostingsSpam(postingIds: List<Long>) =
        bulk(Routes.MARK_POSTINGS_SPAM, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Files postings into an existing Set Aside group. */
    suspend fun addPostingsToBoxGroup(boxId: Long, boxGroupId: Long, postingIds: List<Long>) =
        bulk(
            Routes.ADD_POSTINGS_TO_BOX_GROUP,
            postingIds,
            AddPostingsToBoxGroupRequestContent(postingIds = selection(postingIds), boxId = boxId, boxGroupId = boxGroupId),
        )

    /** Takes postings out of whatever Set Aside group they are in. */
    suspend fun removePostingsFromBoxGroup(postingIds: List<Long>) = byIds(Routes.REMOVE_POSTINGS_FROM_BOX_GROUP, postingIds)

    /** Labels postings with an existing folder. */
    suspend fun filePostings(folderId: Long, postingIds: List<Long>) =
        bulk(Routes.FILE_POSTINGS, postingIds, FilePostingsRequestContent(postingIds = selection(postingIds), folderId = folderId))

    /**
     * Takes a label off postings, or every label when [folderId] is 0. Zero is not "every
     * folder" to HEY, it is a folder that does not exist, so it is left out of the request
     * rather than sent.
     */
    suspend fun unfilePostings(folderId: Long, postingIds: List<Long>) {
        val operation = selection(Routes.UNFILE_POSTINGS, postingIds)
        operation.query("posting_ids", joinIds(postingIds))
        if (folderId != 0L) operation.query("folder_id", folderId)
        client.sendUnit(operation)
    }

    /** Creates a folder and files postings into it. HEY serves no JSON endpoint for creating a folder on its own. */
    suspend fun createFolderForPostings(name: String, postingIds: List<Long>) =
        bulk(
            Routes.CREATE_FOLDER_FOR_POSTINGS,
            postingIds,
            CreateFolderForPostingsRequestContent(postingIds = selection(postingIds), folder = FolderPayload(name = name)),
        )

    /** Bubbles postings up right away. */
    suspend fun bubbleUpPostingsNow(postingIds: List<Long>) =
        bulk(Routes.BUBBLE_UP_POSTINGS_NOW, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Schedules postings to bubble back up at a slot. */
    suspend fun schedulePostingsBubbleUp(slot: BubbleUpSlot, postingIds: List<Long>) =
        bulk(
            Routes.SCHEDULE_POSTINGS_BUBBLE_UP,
            postingIds,
            SchedulePostingsBubbleUpRequestContent(postingIds = selection(postingIds), slot = slot.wire, date = slot.date),
        )

    /** Drops the scheduled bubble up on postings. */
    suspend fun cancelPostingsBubbleUp(postingIds: List<Long>) = byIds(Routes.CANCEL_POSTINGS_BUBBLE_UP, postingIds)

    /**
     * Reads a box's change feed from a cursor to the end of the increment, following the
     * pages HEY names, and answers them combined. A full sync comes back as soon as HEY asks
     * for one, with whatever was read before it dropped: the box has to be read in full
     * anyway. Reading stops at the client's page limit; the answer then carries the cursor of
     * the last page read rather than the end of the feed, and names the page it did not read
     * in [PostingChanges.nextPage], which a complete answer never does.
     */
    suspend fun allChanges(boxId: Long, cursor: PostingChangesCursor): PostingChanges {
        val added = mutableListOf<Posting>()
        val updated = mutableListOf<Posting>()
        val deleted = mutableListOf<DeletedPosting>()
        var nextCursor: PostingChangesCursor? = null
        var next = cursor
        repeat(client.config.maxPages) {
            val changes = changes(boxId, next)
            if (changes.fullSyncRequired) return changes
            added += changes.added
            updated += changes.updated
            deleted += changes.deleted
            nextCursor = changes.nextCursor
            next = changes.nextPage ?: return PostingChanges(added, updated, deleted, nextPage = null, nextCursor = nextCursor)
        }
        return PostingChanges(added, updated, deleted, nextPage = next, nextCursor = nextCursor)
    }

    /**
     * Reads everything that happened to a box's postings since a cursor, as Go's and Rust's
     * conveniences do: the generated [getBoxChanges] answers the page as HEY serves it, this
     * answers a [PostingChanges] with the cursor to resume from and the one answer the page
     * cannot carry — a 409, which is HEY saying the cursor is too old or speaks another
     * version, so the box has to be read in full. It goes without the response cache: a
     * cursor URL never repeats, so a cached answer would never be revalidated and a
     * long-running watch would grow the cache one dead entry per read.
     */
    suspend fun changes(boxId: Long, cursor: PostingChangesCursor): PostingChanges {
        if (cursor.since.isEmpty()) throw HeyException.Usage("a change feed is read from a cursor: since is required")
        val operation = client.operation(Routes.GET_BOX_POSTING_CHANGES, listOf(boxId))
        operation.resourceId(boxId)
        operation.query("since", cursor.since)
        operation.queryOptional("v", cursor.version)
        operation.queryOptional("page", cursor.page)
        operation.queryOptional("per_page", cursor.perPage)
        operation.noCache()
        val page = try {
            client.sendPage<GetBoxPostingChangesResponseContent>(operation)
        } catch (conflict: HeyException.Conflict) {
            return PostingChanges(fullSyncRequired = true)
        }
        // Both links are read whole: the page within an increment can move the since, the
        // version or the size along with the page, and the read that follows sends what HEY
        // issued, not a page number pinned to the cursor this read started from.
        val nextPage = page.nextUrl?.let { next ->
            if (!isSameOrigin(next, client.baseUrl)) {
                throw HeyException.Usage("pagination Link header points to a different origin: ${next.protocol.name}://${next.host}")
            }
            PostingChangesCursor.fromUrl(next.toString())
        }
        return PostingChanges(
            added = page.value.added.orEmpty(),
            updated = page.value.updated.orEmpty(),
            deleted = page.value.deleted.orEmpty(),
            nextPage = nextPage,
            nextCursor = page.nextCursor?.let { PostingChangesCursor.fromUrl(it.toString()) },
            fullSyncRequired = false,
        )
    }

    /** Moves postings to a box. */
    suspend fun moveToBox(boxId: Long, postingIds: List<Long>) =
        bulk(Routes.MOVE_POSTINGS, postingIds, MovePostingsRequestContent(postingIds = selection(postingIds), boxId = boxId))

    /** Moves postings to the box of a kind, resolving the box index once per client. An empty selection is refused before the index is read. */
    suspend fun moveTo(kind: BoxKind, postingIds: List<Long>) {
        val selected = selection(postingIds)
        moveToBox(client.boxes.idByKind(kind), selected)
    }

    /** Moves postings to Set Aside. */
    suspend fun moveToSetAside(postingIds: List<Long>) = moveTo(BoxKind.SET_ASIDE, postingIds)

    /** Moves postings to Reply Later. */
    suspend fun moveToReplyLater(postingIds: List<Long>) = moveTo(BoxKind.REPLY_LATER, postingIds)

    /** Moves postings to the Imbox. */
    suspend fun moveToImbox(postingIds: List<Long>) = moveTo(BoxKind.IMBOX, postingIds)

    /** Moves postings to The Feed. */
    suspend fun moveToFeed(postingIds: List<Long>) = moveTo(BoxKind.FEED, postingIds)

    /** Moves postings to the Paper Trail. */
    suspend fun moveToPaperTrail(postingIds: List<Long>) = moveTo(BoxKind.PAPER_TRAIL, postingIds)
}

private fun joinIds(postingIds: List<Long>): String = postingIds.joinToString(",")

/**
 * When a posting bubbles back up. HEY resurfaces a posting at its morning hour of the day
 * the slot names — [LaterToday] at its evening hour of the current day instead — and reads
 * both hours in UTC, like every hour it takes out of a JSON request.
 */
sealed class BubbleUpSlot(
    /** The slot as HEY's `slot` parameter names it. */
    val wire: String,
) {
    /** The day a [Custom] slot names, and nothing for the named ones: HEY works those out itself. */
    open val date: String? get() = null

    /** This evening. HEY's `today`. */
    data object LaterToday : BubbleUpSlot("today")

    /** Tomorrow morning. */
    data object Tomorrow : BubbleUpSlot("tomorrow")

    /** Saturday. */
    data object ThisWeekend : BubbleUpSlot("weekend")

    /** Monday. */
    data object NextWeek : BubbleUpSlot("next_week")

    /**
     * A day of the caller's choosing, as `YYYY-MM-DD`. HEY does not refuse one that has
     * already passed — those postings bubble up on the next run of its scheduler — but a
     * day the calendar does not have is refused here rather than sent.
     */
    data class Custom(override val date: String) : BubbleUpSlot("custom") {
        init {
            if (!OccurrenceId.isCalendarDate(date)) throw HeyException.Usage("bubble up date \"$date\" is not a YYYY-MM-DD the calendar has")
        }
    }
}

/**
 * Where a read of a box's change feed starts: the instant the changes come after, the
 * contract version the feed speaks (HEY's `v`), the page within an increment while it has
 * more than one, and the page size when the URL named one. Read out of a changes URL HEY
 * issued — a box's `posting_changes_url`, or the cursor a page hands back — with [fromUrl].
 */
data class PostingChangesCursor(
    val since: String,
    val version: String? = null,
    val page: String? = null,
    val perPage: String? = null,
) {
    companion object {
        /** Reads a cursor out of a changes URL HEY issued. */
        fun fromUrl(changesUrl: String): PostingChangesCursor {
            val url = parseAbsoluteUrl(changesUrl) ?: throw HeyException.Usage("changes URL is not an absolute URL")
            val parameters = url.parameters
            return PostingChangesCursor(
                since = parameters["since"].orEmpty(),
                version = parameters["v"],
                page = parameters["page"],
                perPage = parameters["per_page"],
            )
        }
    }
}

/**
 * Everything that happened to a box's postings since a cursor. [nextPage] is set while this
 * increment has more pages to read now; [nextCursor] is set on the last page and is where
 * the next read resumes, and null when nothing changed, in which case the cursor that
 * produced this page still stands. [fullSyncRequired] is HEY's 409: the cursor is too old
 * or speaks another version, nothing else here is set, and the box has to be read in full.
 */
data class PostingChanges(
    /** The postings that appeared in the box. */
    val added: List<Posting> = emptyList(),
    /** The postings that changed. */
    val updated: List<Posting> = emptyList(),
    /** The postings that left the box. */
    val deleted: List<DeletedPosting> = emptyList(),
    /** The page within this increment to read next, while there is one: a whole cursor, as HEY issued it. */
    val nextPage: PostingChangesCursor? = null,
    /** Where to resume once this increment is read, when HEY named one. */
    val nextCursor: PostingChangesCursor? = null,
    /** Whether HEY refused the cursor and the box has to be read in full. */
    val fullSyncRequired: Boolean = false,
)
