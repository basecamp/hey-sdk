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
import com.basecamp.hey.generated.services.GetBoxPostingChangesOptions
import com.basecamp.hey.Page
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
     * Reads a box's change feed from a cursor, as the generated [getBoxChanges] does but
     * without the response cache: a cursor URL never repeats, so a cached answer would never
     * be revalidated and a long-running watch would grow the cache one dead entry per read.
     * The page's `nextCursor` is where to poll from next once its pages run out.
     */
    suspend fun changes(boxId: Long, since: String, options: GetBoxPostingChangesOptions? = null): Page<GetBoxPostingChangesResponseContent> {
        if (since.isEmpty()) throw HeyException.Usage("a change feed is read from a cursor: since is required")
        val operation = client.operation(Routes.GET_BOX_POSTING_CHANGES, listOf(boxId))
        operation.resourceId(boxId)
        operation.query("since", since)
        operation.queryOptional("v", options?.v)
        operation.queryOptional("page", options?.page)
        operation.queryOptional("per_page", options?.perPage)
        operation.noCache()
        return client.sendPage(operation)
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
