package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.redactLocation
import com.basecamp.hey.Response
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.CreateMessageRequestContent
import com.basecamp.hey.generated.models.MessageAddressed
import com.basecamp.hey.generated.models.MessageEntryPayload
import com.basecamp.hey.generated.models.MessagePayload
import com.basecamp.hey.json
import io.ktor.http.URLBuilder
import io.ktor.http.encodedPath
import io.ktor.http.takeFrom
import com.basecamp.hey.generated.services.MessagesService as GeneratedMessagesService

/** What `entry.status` carries to keep an entry a draft. Any other value, or omitting it, has HEY deliver it. */
private const val DRAFTED = "drafted"

/** A message to deliver: the subject, the Trix HTML body and the recipients per kind. */
data class MessageContent(
    /** The subject line. */
    val subject: String = "",
    /** The body, as Trix HTML. */
    val content: String = "",
    /** The addresses the message goes to. */
    val to: List<String> = emptyList(),
    /** The addresses copied on it. */
    val cc: List<String> = emptyList(),
    /** The addresses copied on it without the others seeing. */
    val bcc: List<String> = emptyList(),
    /** The identity the message goes out as. Null resolves the client's default sender. */
    val actingSenderId: Long? = null,
)

/** When a draft goes out, read in the identity's time zone. HEY schedules to the hour. */
data class DeliverySchedule(
    /** `YYYY-MM-DD`, "today" or "tomorrow". */
    val date: String,
    /** 0 through 23. */
    val hour: Int,
)

/**
 * The whole of what a draft carries. HEY revises a draft from the whole of it, so a caller
 * edits by reading the draft, changing fields and sending everything back: empty recipients
 * remove them, and a null schedule clears one already set.
 */
data class DraftContent(
    val subject: String = "",
    val content: String = "",
    val to: List<String> = emptyList(),
    val cc: List<String> = emptyList(),
    val bcc: List<String> = emptyList(),
    /**
     * The identity the draft is saved — and ultimately delivered — as. Null resolves the
     * client's default sender. A caller who chose an alternate identity carries it on every
     * revision, since a revision that leaves it out hands the draft back to the default one.
     */
    val actingSenderId: Long? = null,
    /** When HEY delivers the draft on its own. Null is a draft that waits to be sent. */
    val schedule: DeliverySchedule? = null,
)

/** Messages service with delivery and draft conveniences on top of the generated surface (`create`, `update`, `get`, `getEdit`). */
class MessagesService(client: HeyClient) : GeneratedMessagesService(client) {
    /**
     * Delivers a new message through HEY's undo-delay window. Delivery needs somebody to
     * deliver to, so at least one recipient is required.
     */
    suspend fun send(message: MessageContent) {
        if (!hasRecipients(message.to, message.cc, message.bcc)) {
            throw HeyException.Usage("a message needs at least one recipient (to, cc or bcc)")
        }
        val body = CreateMessageRequestContent(
            actingSenderId = message.actingSenderId ?: client.defaultSenderId(),
            message = MessagePayload(subject = message.subject, content = message.content),
            entry = deliveredEntry(message.to, message.cc, message.bcc),
        )
        val operation = client.operation(Routes.CREATE_MESSAGE, emptyList())
        operation.json(body)
        client.sendUnit(operation)
    }

    /**
     * Saves a new message as a draft instead of delivering it, and answers the draft's entry
     * id — the id [getEdit], [updateDraft], [sendDraft] and `EntriesService.deleteDraft`
     * take. A draft needs no recipients.
     */
    suspend fun createDraft(draft: DraftContent): Long {
        val operation = client.operation(Routes.CREATE_MESSAGE, emptyList())
        operation.json(draftedRequest(draft))
        return entryIdFromLocation(client.execute(operation))
    }

    /** Revises a draft in place from the whole of [draft]. A trashed draft is silently restored by the revision. */
    suspend fun updateDraft(entryId: Long, draft: DraftContent) {
        val operation = client.operation(Routes.UPDATE_MESSAGE, listOf(entryId))
        operation.resourceId(entryId)
        operation.json(draftedRequest(draft))
        client.sendUnit(operation)
    }

    /**
     * Delivers a draft through HEY's undo-delay window. The revision and the delivery are one
     * request, so the draft's final state rides along. It is never retried, despite the PUT:
     * it triggers a delivery, and a resend after an ambiguous first attempt could send twice.
     */
    suspend fun sendDraft(entryId: Long, draft: DraftContent) {
        if (!hasRecipients(draft.to, draft.cc, draft.bcc)) {
            throw HeyException.Usage("sending a draft needs at least one recipient (to, cc or bcc)")
        }
        val body = CreateMessageRequestContent(
            actingSenderId = draft.actingSenderId ?: client.defaultSenderId(),
            message = MessagePayload(subject = draft.subject, content = draft.content),
            entry = deliveredEntry(draft.to, draft.cc, draft.bcc),
        )
        val operation = client.operation(Routes.UPDATE_MESSAGE, listOf(entryId))
        operation.resourceId(entryId)
        operation.json(body)
        operation.idempotent(false)
        client.sendUnit(operation)
    }

    private suspend fun draftedRequest(draft: DraftContent): CreateMessageRequestContent {
        var entry = draftedEntry(draft.to, draft.cc, draft.bcc)
        draft.schedule?.let { schedule ->
            entry = entry.copy(
                scheduledDelivery = "true",
                scheduledDeliveryAtDate = schedule.date,
                scheduledDeliveryAtHour = schedule.hour.toString(),
            )
        }
        return CreateMessageRequestContent(
            actingSenderId = draft.actingSenderId ?: client.defaultSenderId(),
            message = MessagePayload(subject = draft.subject, content = draft.content),
            entry = entry,
        )
    }
}

internal fun hasRecipients(to: List<String>, cc: List<String>, bcc: List<String>): Boolean =
    to.isNotEmpty() || cc.isNotEmpty() || bcc.isNotEmpty()

/** The entry of a message HEY delivers, whose recipient kinds are the ones that name somebody. */
internal fun deliveredEntry(to: List<String>, cc: List<String>, bcc: List<String>): MessageEntryPayload =
    MessageEntryPayload(
        addressed = MessageAddressed(
            directly = to.ifEmpty { null },
            copied = cc.ifEmpty { null },
            blindcopied = bcc.ifEmpty { null },
        ),
    )

/**
 * The entry of a message HEY keeps as a draft. Every recipient kind is present, empty ones
 * included: a draft addressed to nobody yet is the normal case, and on a revision an empty
 * list is how recipients are removed.
 */
internal fun draftedEntry(to: List<String>, cc: List<String>, bcc: List<String>): MessageEntryPayload =
    MessageEntryPayload(
        addressed = MessageAddressed(directly = to, copied = cc, blindcopied = bcc),
        status = DRAFTED,
    )

/** The entry id out of the `Location` a draft save answers with: the save serves no body, so the header is the only place the id is named. */
internal fun entryIdFromLocation(response: Response): Long {
    val location = response.header("Location")
        ?: throw HeyException.Api("draft saved but the response named no Location; cannot report the draft's id", httpStatus = response.status, retryable = false)
    val path = runCatching { URLBuilder(response.url).takeFrom(location).build().encodedPath }.getOrNull()
        ?: throw HeyException.Api("draft saved but its Location \"${redactLocation(location)}\" is unreadable", httpStatus = response.status, retryable = false)
    val entryId = path.trimEnd('/').substringAfterLast('/').toLongOrNull()
    if (entryId == null || entryId <= 0) {
        throw HeyException.Api("draft saved but its Location \"${redactLocation(location)}\" names no entry id", httpStatus = response.status, retryable = false)
    }
    return entryId
}
