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
     * panel rather than carrying the link, so the publication is read back. Both requests
     * go quiet inside one operation, so the hooks hear `Publications.CreateTopicPublication`
     * once, see both requests under it, as they do in Go, and hear it end only once the
     * link is in hand — with the failure, when the read-back fails.
     *
     * HEY answers a forbidden error on accounts that are not eligible to publish.
     */
    suspend fun publish(topicId: Long): TopicPublication =
        client.asOperation(writeInfo("Publications", "CreateTopicPublication", "publication", topicId)) {
            val operation = client.form(Method.POST, "/topics/$topicId/publication")
            operation.form(emptyList())
            operation.quiet()
            client.sendUnit(operation)
            val read = client.operation(Routes.GET_TOPIC_PUBLICATION, listOf(topicId))
            read.resourceId(topicId)
            read.quiet()
            client.send(read)
        }

    /** Unpublishes a thread, breaking its public link. */
    suspend fun unpublish(topicId: Long) {
        val operation = client.form(Method.DELETE, "/topics/$topicId/publication")
        operation.info(writeInfo("Publications", "DeleteTopicPublication", "publication", topicId))
        client.sendUnit(operation)
    }
}
