package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.Method
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.ClipsService as GeneratedClipsService

/**
 * Clips service with the writes on top of the generated surface (`list`). Clips have no
 * JSON surface for writes — a write answers with a Turbo Stream rather than a record — so
 * both of them are browser form posts.
 */
class ClipsService(client: HeyClient) : GeneratedClipsService(client) {
    /** Clips a piece of an entry, so it can be found again without the thread. */
    suspend fun create(entryId: Long, content: String) {
        val operation = client.form(Method.POST, "/clips")
        operation.info(writeInfo("Clips", "CreateClip", "clip", entryId))
        operation.form(listOf("clip[entry_id]" to entryId.toString(), "clip[content]" to content))
        client.sendUnit(operation)
    }

    /** Throws a clip away. */
    suspend fun delete(clipId: Long) {
        val operation = client.form(Method.DELETE, "/clips/$clipId")
        operation.info(writeInfo("Clips", "DeleteClip", "clip", clipId))
        client.sendUnit(operation)
    }
}
