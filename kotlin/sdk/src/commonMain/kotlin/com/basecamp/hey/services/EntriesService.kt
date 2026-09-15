package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.CreateReplyRequestContent
import com.basecamp.hey.generated.models.ReplyMessagePayload
import com.basecamp.hey.json
import com.basecamp.hey.generated.services.EntriesService as GeneratedEntriesService

/** A reply, as the reply prefill (`EntriesService.newReply`) hands its parts over. */
data class ReplyContent(
    /**
     * The identity the reply goes out as. HEY resolves a reply's sender from the thread and
     * hands it back as the prefill's sender; pass that id. Zero falls back to the client's
     * default sender, which on a shared address is the wrong identity.
     */
    val actingSenderId: Long = 0,
    /** HEY derives no subject for a reply, so pass the prefill's "Re: …". Empty leaves it off the wire. */
    val subject: String = "",
    /** The reply body alone: HEY appends the quoted original at delivery. */
    val content: String = "",
    val to: List<String> = emptyList(),
    val cc: List<String> = emptyList(),
    val bcc: List<String> = emptyList(),
)

/** Entries service with reply conveniences on top of the generated surface (`createReply`, `newReply`, `deleteDraft`, ...). */
class EntriesService(client: HeyClient) : GeneratedEntriesService(client) {
    /**
     * Delivers a reply to an entry. HEY does not reply-all on the caller's behalf, and saves
     * an unaddressed reply as a draft rather than delivering it, so the recipients are
     * required.
     */
    suspend fun reply(entryId: Long, reply: ReplyContent) {
        if (!hasRecipients(reply.to, reply.cc, reply.bcc)) {
            throw HeyException.Usage("a reply needs at least one recipient (to, cc or bcc); HEY saves an unaddressed reply as a draft")
        }
        val body = CreateReplyRequestContent(
            actingSenderId = senderFor(reply),
            message = replyPayload(reply),
            entry = deliveredEntry(reply.to, reply.cc, reply.bcc),
        )
        val operation = client.operation(Routes.CREATE_REPLY, listOf(entryId))
        operation.resourceId(entryId)
        operation.json(body)
        client.sendUnit(operation)
    }

    /** Saves a reply as a draft instead of delivering it, and answers the draft's entry id. It needs no recipients. */
    suspend fun replyDraft(entryId: Long, reply: ReplyContent): Long {
        val body = CreateReplyRequestContent(
            actingSenderId = senderFor(reply),
            message = replyPayload(reply),
            entry = draftedEntry(reply.to, reply.cc, reply.bcc),
        )
        val operation = client.operation(Routes.CREATE_REPLY, listOf(entryId))
        operation.resourceId(entryId)
        operation.json(body)
        return client.execute(operation) { entryIdFromLocation(it) }
    }

    private suspend fun senderFor(reply: ReplyContent): Long =
        if (reply.actingSenderId == 0L) client.defaultSenderId() else reply.actingSenderId

    private fun replyPayload(reply: ReplyContent): ReplyMessagePayload =
        ReplyMessagePayload(subject = reply.subject.ifEmpty { null }, content = reply.content)
}
