package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Response
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.MoveTopicRequestContent
import com.basecamp.hey.resolveReference
import io.ktor.http.Url
import com.basecamp.hey.generated.services.TopicsService as GeneratedTopicsService

/**
 * Topics service with the moves, the confirmed trashing and the undo that answers its entry
 * on top of the generated surface (`get`, `getEntries`, `trash`, `restore`, `markHam`, ...). [getEntries] is paged by geared
 * pagination, so its `page` is a cursor out of the previous answer's `Link` header rather
 * than an offset: a number is ignored and answered with the first page.
 */
class TopicsService(client: HeyClient) : GeneratedTopicsService(client) {
    /** Moves a topic to a box, by the box's id. The generated [moveTopic] takes the request body; this takes the one thing it carries. */
    suspend fun moveToBox(topicId: Long, boxId: Long) = moveTopic(topicId, MoveTopicRequestContent(boxId = boxId))

    /**
     * Trashes a topic. HEY will not trash a shared topic without being asked twice: it
     * answers the removal confirmation page instead, which comes back here as a usage error
     * rather than as a trashing that quietly did nothing. Confirming trashes the topic and
     * removes your access to it.
     *
     * The generated [trash] sends the same request from its parts, and follows that redirect
     * rather than reading it.
     */
    suspend fun trashTopic(topicId: Long, confirmDestroy: Boolean = false) {
        val operation = client.operation(Routes.TRASH_TOPIC, listOf(topicId))
        operation.resourceId(topicId)
        // An empty confirm_destroy reads as truthy on the server and skips the confirmation,
        // so it is sent only when it is asked for.
        if (confirmDestroy) operation.query("confirm_destroy", 1)
        operation.captureRedirects()
        // The answer is read inside the operation, so the hooks hear the refusal the caller gets.
        client.execute(operation) { response ->
            if (awaitingConfirmation(response, topicId)) {
                throw HeyException.Usage(
                    "topic $topicId is shared; HEY wants confirmation before trashing it",
                    hint = "Call trashTopic with confirmDestroy = true to trash it and remove your access",
                )
            }
        }
    }

    /**
     * Calls back the last message the caller sent on a topic, while Undo Send still holds
     * it, and answers the entry HEY called back: a draft again, unsent.
     *
     * HEY picks the message, the caller's last delayed entry on the topic, so compare the
     * entry answered with the one meant: two messages sent to one thread within the delay
     * are called back last first. A reply that broke out into a thread of its own is called
     * back on that thread, the one the send's `topicId` names.
     *
     * HEY answers with a redirect to the draft rather than JSON, the answer its own apps
     * read, and the redirect is the only place it names the entry: this takes it as the
     * answer rather than follow it, and reads the entry with [undoneEntryId]. The entry is
     * null when the redirect names none. Once the delay has run out and the message has
     * gone, or when nothing was held, HEY answers 400: a [HeyException.Api] whose
     * `httpStatus` is 400, and nothing was changed. It is never sent twice.
     *
     * The generated [undoSend] sends the same request and follows the redirect, which says
     * the undo went through and not which entry it was.
     */
    suspend fun undoTopicSend(topicId: Long): Long? {
        val operation = client.operation(Routes.UNDO_TOPIC_SEND, listOf(topicId))
        operation.resourceId(topicId)
        operation.captureRedirects()
        // The refusal is reworded inside the operation, so the hooks hear the error the caller gets.
        operation.quiet()
        return client.asOperation(operation.info) {
            try {
                calledBack(client.execute(operation))
            } catch (error: HeyException) {
                if (error.httpStatus == 400) {
                    throw HeyException.Api(NOTHING_TO_UNDO, httpStatus = 400, retryable = false, requestId = error.requestId, body = error.body)
                }
                throw error
            }
        }
    }

    /** The entry the redirect an undo answered with names, when it names one. */
    private fun calledBack(response: Response): Long? =
        if (response.status in 300..399) response.header("Location")?.let(::undoneEntryId) else null

    /**
     * Whether HEY answered by sending the caller to the topic's removal confirmation page,
     * which is how it says it will not trash a shared topic unasked. Any other redirect is HEY
     * sending the caller back to where the topic was, with the trashing done.
     */
    private fun awaitingConfirmation(response: Response, topicId: Long): Boolean {
        val location = response.header("Location") ?: return false
        return resolveReference(response.url, location)?.encodedPath == "/topics/$topicId/removal/new"
    }
}

/** What a 400 from an undo means: there was no delayed entry of the caller's on the topic. */
private const val NOTHING_TO_UNDO = "nothing to undo: the message was already sent, or none was held"

/**
 * The entry HEY called back, read out of the redirect an undo send answers with, for a
 * caller holding that `Location`: `/messages/{id}/edit` for a message on a thread of its
 * own, `/topics/{topicId}?expanded_draft={id}` for a reply. Anything else names no entry
 * and answers null: the thread alone, which is where HEY's pages send a browser with nothing
 * to undo, is not an undo.
 */
fun undoneEntryId(location: String): Long? {
    // A Location is often a path rather than a whole URL, so it is read against a base that
    // stands for wherever it came from: only its path and query are looked at.
    val target = resolveReference(LOCATION_BASE, location) ?: return null
    val segments = target.encodedPath.trim('/').split('/')
    val named = when {
        segments.size == 3 && segments[0] == "messages" && segments[2] == "edit" -> segments[1]
        segments.size == 2 && segments[0] == "topics" -> target.parameters["expanded_draft"]
        else -> null
    }
    return named?.toLongOrNull()?.takeIf { it > 0 }
}

private val LOCATION_BASE = Url("https://hey.invalid/")
