package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.boxes
import com.basecamp.hey.generated.models.MarkPostingsRequestContent
import com.basecamp.hey.generated.models.MovePostingsRequestContent
import com.basecamp.hey.generated.models.TrashPostingsRequestContent
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

    /** Marks postings seen. The generated [markSeen] takes the request body. */
    suspend fun markPostingsSeen(postingIds: List<Long>) = markSeen(MarkPostingsRequestContent(selection(postingIds)))

    /** Marks postings unseen. */
    suspend fun markPostingsUnseen(postingIds: List<Long>) = markUnseen(MarkPostingsRequestContent(selection(postingIds)))

    /** Trashes postings. */
    suspend fun trashPostings(postingIds: List<Long>) = trash(TrashPostingsRequestContent(postingIds = selection(postingIds)))

    /** Mutes postings. */
    suspend fun mutePostings(postingIds: List<Long>) = mute(MarkPostingsRequestContent(selection(postingIds)))

    /** Moves postings to a box. */
    suspend fun moveToBox(boxId: Long, postingIds: List<Long>) =
        movePostings(MovePostingsRequestContent(postingIds = selection(postingIds), boxId = boxId))

    /** Moves postings to the box of a kind, resolving the box index once per client. */
    suspend fun moveTo(kind: BoxKind, postingIds: List<Long>) = moveToBox(client.boxes.idByKind(kind), selection(postingIds))

    /** Moves postings to Set Aside. */
    suspend fun moveToSetAside(postingIds: List<Long>) = moveTo(BoxKind.SET_ASIDE, postingIds)

    /** Moves postings to Reply Later. */
    suspend fun moveToReplyLater(postingIds: List<Long>) = moveTo(BoxKind.REPLY_LATER, postingIds)

    /** Moves postings to the Imbox. */
    suspend fun moveToImbox(postingIds: List<Long>) = moveTo(BoxKind.IMBOX, postingIds)
}
