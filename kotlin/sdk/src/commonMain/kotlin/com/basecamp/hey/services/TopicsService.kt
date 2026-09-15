package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Response
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.MoveTopicRequestContent
import com.basecamp.hey.resolveReference
import com.basecamp.hey.generated.services.TopicsService as GeneratedTopicsService

/**
 * Topics service with the moves and the confirmed trashing on top of the generated surface
 * (`get`, `getEntries`, `trash`, `restore`, `markHam`, ...). [getEntries] is paged by geared
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
        val response = client.execute(operation)
        if (awaitingConfirmation(response, topicId)) {
            throw HeyException.Usage(
                "topic $topicId is shared; HEY wants confirmation before trashing it",
                hint = "Call trashTopic with confirmDestroy = true to trash it and remove your access",
            )
        }
    }

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
