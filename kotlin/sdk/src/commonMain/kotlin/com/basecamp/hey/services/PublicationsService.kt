package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.Method
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.TopicPublication
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.PublicationsService as GeneratedPublicationsService

/**
 * Publications service: turning a thread into a public web page, on top of the generated
 * `get`. Publishing has no JSON surface — both writes are browser form posts that redirect,
 * and the public link only appears once the publication is read back.
 */
class PublicationsService(client: HeyClient) : GeneratedPublicationsService(client) {
    /**
     * Publishes a thread and answers its public link. The redirect lands on the sharing
     * panel rather than carrying the link, so the publication is read back; that read is a
     * quiet one, so the hooks hear `Publications.CreateTopicPublication` once and see both
     * requests under it, as they do in Go. The operation's own end still lands after the
     * first of the two, the SDK having no seam for wrapping a block of them.
     *
     * HEY answers a forbidden error on accounts that are not eligible to publish.
     */
    suspend fun publish(topicId: Long): TopicPublication {
        val operation = client.form(Method.POST, "/topics/$topicId/publication")
        operation.info(writeInfo("Publications", "CreateTopicPublication", "publication", topicId))
        operation.form(emptyList())
        client.sendUnit(operation)
        val read = client.operation(Routes.GET_TOPIC_PUBLICATION, listOf(topicId))
        read.resourceId(topicId)
        read.quiet()
        return client.send(read)
    }

    /** Unpublishes a thread, breaking its public link. */
    suspend fun unpublish(topicId: Long) {
        val operation = client.form(Method.DELETE, "/topics/$topicId/publication")
        operation.info(writeInfo("Publications", "DeleteTopicPublication", "publication", topicId))
        client.sendUnit(operation)
    }
}
