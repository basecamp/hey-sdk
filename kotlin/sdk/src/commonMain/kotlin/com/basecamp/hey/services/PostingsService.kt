package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Route
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.boxes
import com.basecamp.hey.json
import com.basecamp.hey.generated.models.MarkPostingsRequestContent
import com.basecamp.hey.generated.models.MovePostingsRequestContent
import com.basecamp.hey.generated.models.TrashPostingsRequestContent
import com.basecamp.hey.generated.models.GetBoxPostingChangesResponseContent
import com.basecamp.hey.generated.models.DeletedPosting
import com.basecamp.hey.generated.models.Posting
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
        val operation = client.operation(route, emptyList())
        postingIds.singleOrNull()?.let { operation.resourceId(it) }
        operation.json(body)
        client.sendUnit(operation)
    }

    /** Marks postings seen. The generated [markSeen] takes the request body. */
    suspend fun markPostingsSeen(postingIds: List<Long>) =
        bulk(Routes.MARK_POSTINGS_SEEN, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Marks postings unseen. */
    suspend fun markPostingsUnseen(postingIds: List<Long>) =
        bulk(Routes.MARK_POSTINGS_UNSEEN, postingIds, MarkPostingsRequestContent(selection(postingIds)))

    /** Trashes postings. */
    suspend fun trashPostings(postingIds: List<Long>) =
        bulk(Routes.TRASH_POSTINGS, postingIds, TrashPostingsRequestContent(postingIds = selection(postingIds)))

    /** Mutes postings. */
    suspend fun mutePostings(postingIds: List<Long>) =
        bulk(Routes.MUTE_POSTINGS, postingIds, MarkPostingsRequestContent(selection(postingIds)))

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
        return PostingChanges(
            added = page.value.added.orEmpty(),
            updated = page.value.updated.orEmpty(),
            deleted = page.value.deleted.orEmpty(),
            nextPage = page.nextPage,
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
    /** The page within this increment to read next, while there is one. */
    val nextPage: String? = null,
    /** Where to resume once this increment is read, when HEY named one. */
    val nextCursor: PostingChangesCursor? = null,
    /** Whether HEY refused the cursor and the box has to be read in full. */
    val fullSyncRequired: Boolean = false,
)
