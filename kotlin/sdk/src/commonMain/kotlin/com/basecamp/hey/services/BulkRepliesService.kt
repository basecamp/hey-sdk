package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.generated.models.BulkReplyMessagePayload
import com.basecamp.hey.generated.models.BulkReplyRequestContent
import com.basecamp.hey.generated.models.CreateBulkReplyResponseContent
import com.basecamp.hey.generated.models.NewBulkReplyResponseContent
import com.basecamp.hey.parseAbsoluteUrl
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.BulkRepliesService as GeneratedBulkRepliesService

/**
 * Bulk replies service — one reply to many threads, and calling a delayed one back — on top
 * of the generated surface (`newBulkReply`, `create`).
 */
class BulkRepliesService(client: HeyClient) : GeneratedBulkRepliesService(client) {
    /**
     * Works out which entries a bulk reply would answer, and how it starts. HEY replies to
     * the last replyable entry of each thread and skips threads it has no reply address for,
     * so the postings you hold are not the entries the reply goes to. Send the entries this
     * answers — or a subset of them — to [send]. The generated [newBulkReply] takes the ids
     * already comma-joined.
     */
    suspend fun draft(postingIds: List<Long>): NewBulkReplyResponseContent {
        if (postingIds.isEmpty()) throw HeyException.Usage("at least one posting is required")
        return newBulkReply(postingIds.joinToString(","))
    }

    /**
     * Replies to every entry with the same content, and answers what was sent. Delivery is
     * queued: while the sender has undo enabled the send is held open, and the answer says
     * so — `delayed` is true and `undoSendUrl` is where to call it back with [undo]. The
     * generated [create] takes the same request as a body.
     */
    suspend fun send(entryIds: List<Long>, content: String): CreateBulkReplyResponseContent {
        if (entryIds.isEmpty()) throw HeyException.Usage("at least one entry is required")
        return create(BulkReplyRequestContent(entryIds = entryIds, message = BulkReplyMessagePayload(content = content)))
    }

    /**
     * Calls a delayed bulk reply back before it goes out. HEY answers this one with a
     * redirect rather than JSON — the same answer its own apps read. Once the replies have
     * gone out there is nothing left to call back, and HEY refuses; the SDK does not check
     * the delivery first.
     */
    suspend fun undo(bulkReplyId: Long) {
        val operation = client.form(Method.POST, "/bulk_replies/$bulkReplyId/undo_send")
        operation.info(writeInfo("BulkReplies", "UndoBulkReplySend", "bulk_reply", bulkReplyId))
        operation.form(emptyList())
        client.sendUnit(operation)
    }
}

/**
 * The bulk reply id in a delivery's `undo_send_url`, for a caller holding the URL rather
 * than the delivery. Only that URL is read: anything else the caller might be holding — the
 * bulk reply itself, some other action on it — is refused rather than guessed at.
 */
fun undoSendId(undoSendUrl: String): Long {
    val path = parseAbsoluteUrl(undoSendUrl)?.encodedPath ?: undoSendUrl.substringBefore('?').substringBefore('#')
    val segments = path.trim('/').split('/')
    if (segments.size != 3 || segments[0] != "bulk_replies" || segments[2] != "undo_send") throw notAnUndoUrl(undoSendUrl)
    return segments[1].toLongOrNull() ?: throw notAnUndoUrl(undoSendUrl)
}

private fun notAnUndoUrl(undoSendUrl: String): HeyException = HeyException.Usage("not a bulk reply undo URL: $undoSendUrl")
