package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.Method
import com.basecamp.hey.generated.models.CollectionPayload
import com.basecamp.hey.generated.models.UpdateCollectionRequestContent
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.CollectionsService as GeneratedCollectionsService

/** What a new collection is made of. */
data class CreateCollectionParams(
    /** What the collection is called. */
    val name: String,
    /** The blurb shown under the name. */
    val summary: String? = null,
    /** The account that owns it. Null leaves HEY to pick your first. */
    val accountId: Long? = null,
)

/**
 * What an edit changes about a collection. A field left unset — null or empty — is left off
 * the wire, and HEY leaves what a request does not name alone.
 */
data class UpdateCollectionParams(
    /** A new name. */
    val name: String? = null,
    /** A new blurb under the name. */
    val summary: String? = null,
)

/**
 * Collections service with the writes on top of the generated surface (`get`, `list`,
 * `update`). HEY serves no JSON endpoint for making a collection or filing a thread into
 * one, so each of those is a browser form post.
 */
class CollectionsService(client: HeyClient) : GeneratedCollectionsService(client) {
    /**
     * Makes a collection. The form post answers with a redirect to the collections index
     * rather than to the collection it made, so the new collection's id does not come back;
     * [list] afterwards is how to find it.
     */
    suspend fun create(params: CreateCollectionParams) {
        val fields = mutableListOf("collection[name]" to params.name)
        params.summary?.takeIf { it.isNotEmpty() }?.let { fields += "collection[summary]" to it }
        params.accountId?.let { fields += "account_id" to it.toString() }
        val operation = client.form(Method.POST, "/collections")
        operation.info(writeInfo("Collections", "CreateCollection", "collection"))
        operation.form(fields)
        client.sendUnit(operation)
    }

    /** Renames a collection or changes its summary. The generated [update] takes the same request as a body. */
    suspend fun updateCollection(collectionId: Long, params: UpdateCollectionParams) {
        val body = UpdateCollectionRequestContent(
            collection = CollectionPayload(name = present(params.name), summary = present(params.summary)),
        )
        update(collectionId, body)
    }

    /** Files a topic into a collection. */
    suspend fun addTopic(topicId: Long, collectionId: Long) {
        val operation = client.form(Method.POST, "/topics/$topicId/collecting")
        operation.info(writeInfo("Collections", "CreateTopicCollecting", "collecting", topicId))
        operation.query("collection_id", collectionId)
        operation.form(emptyList())
        client.sendUnit(operation)
    }

    /** Takes a topic back out of a collection. A shadowed topic is silently left alone. */
    suspend fun removeTopic(topicId: Long, collectionId: Long) {
        val operation = client.form(Method.DELETE, "/topics/$topicId/collecting")
        operation.info(writeInfo("Collections", "DeleteTopicCollecting", "collecting", topicId))
        operation.query("collection_id", collectionId)
        client.sendUnit(operation)
    }
}

/** An empty string is no value, and is left off the wire like a null one — the omission is what tells HEY to leave the field as it is. */
private fun present(value: String?): String? = value?.takeIf { it.isNotEmpty() }
