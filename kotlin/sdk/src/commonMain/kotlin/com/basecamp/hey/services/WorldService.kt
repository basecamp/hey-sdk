package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.encodePathSegment
import com.basecamp.hey.redactLocation
import com.basecamp.hey.writeInfo
import kotlin.random.Random

/** The recipient that turns a message into a HEY World post. */
const val WORLD_ADDRESS: String = "world@hey.com"

/** Where a published message lands, and what the token naming the post follows. */
private const val POST_PATH = "/world/posts/"

/** The part a subscriber import is read from. */
private const val IMPORT_PART = "world_list_import[source]"

/** The file name an import falls back to when the caller names none. */
private const val DEFAULT_IMPORT_FILENAME = "subscribers.csv"

/**
 * HEY World — the blog you write by sending an email. None of it is JSON: a post is created
 * by emailing [WORLD_ADDRESS], an edit answers a redirect, and the subscriber list is a CSV
 * stream. There is no generated counterpart, so this is the whole of the service.
 */
class WorldService(client: HeyClient) : BaseService(client) {
    /**
     * Writes a HEY World post by sending a message to [WORLD_ADDRESS], and answers the post's
     * token — the handle [updatePost] and [deletePost] take. The message goes out either way,
     * so a landing that is not a post is reported as what it is rather than as a failure to
     * send.
     */
    suspend fun publish(subject: String, content: String): String {
        val senderId = client.defaultSenderId()
        val operation = client.form(Method.POST, "/messages")
        operation.info(writeInfo("World", "PublishWorldPost", "world_post"))
        operation.form(
            listOf(
                "acting_sender_id" to senderId.toString(),
                "message[subject]" to subject,
                "message[content]" to content,
                "entry[addressed][directly]" to WORLD_ADDRESS,
                "entry[status]" to "active",
            ),
        )
        val location = client.sendForm(operation).location.orEmpty()
        return postToken(location)
            ?: throw HeyException.Api(
                "the message was sent but did not become a HEY World post (landed on \"${redactLocation(location)}\")",
                httpStatus = null,
                retryable = false,
            )
    }

    /** Edits a published post. An empty subject or body is left off the wire, and HEY leaves what a request does not name alone. */
    suspend fun updatePost(token: String, subject: String, content: String) {
        val fields = mutableListOf<Pair<String, String>>()
        if (subject.isNotEmpty()) fields += "world_post[subject]" to subject
        if (content.isNotEmpty()) fields += "world_post[content]" to content
        val operation = client.form(Method.PATCH, postPath(token))
        operation.info(writeInfo("World", "UpdateWorldPost", "world_post"))
        operation.form(fields)
        client.sendUnit(operation)
    }

    /** Takes a post off HEY World. */
    suspend fun deletePost(token: String) {
        val operation = client.form(Method.DELETE, postPath(token))
        operation.info(writeInfo("World", "DeleteWorldPost", "world_post"))
        client.sendUnit(operation)
    }

    /**
     * The confirmed subscribers of a list as CSV, with the columns `email_address` and
     * `subscribed_at`. The list is named by its author's email address, which goes into the
     * path escaped as every modelled route escapes a parameter, so the `@` goes out as `%40`.
     */
    suspend fun exportSubscribers(listEmailAddress: String): ByteArray {
        val operation = client.request(Method.GET, "/world/lists/${encodePathSegment(listEmailAddress)}/export.csv")
        operation.info(OperationInfo(service = "World", operation = "ExportWorldSubscribers", resourceType = "world_list", isMutation = false))
        operation.withoutJsonSuffix()
        operation.accept("text/csv")
        return client.execute(operation) { it.body }
    }

    /**
     * Uploads a CSV of subscribers to a list. A blank [filename] becomes `subscribers.csv`,
     * and one that does not already end in `.csv` gets it added: HEY reads the import by its
     * extension.
     */
    suspend fun importSubscribers(listEmailAddress: String, filename: String, csv: ByteArray) {
        val (contentType, body) = subscriberImportBody(filename, csv)
        val operation = client.form(Method.POST, "/world/lists/${encodePathSegment(listEmailAddress)}/imports")
        operation.info(writeInfo("World", "ImportWorldSubscribers", "world_list"))
        operation.bodyBytes(contentType, body)
        client.sendUnit(operation)
    }
}

/** The HEY World service: publishing posts and keeping a list's subscribers. */
val HeyClient.world: WorldService
    get() = service("World") { WorldService(this) }

/**
 * The token out of the location a publish redirected to, as Go's `/world/posts/([0-9a-f]+)`
 * reads it: the first `/world/posts/` followed by at least one hex digit, and the run of hex
 * digits after it. A message that landed anywhere else names no token.
 */
internal fun postToken(location: String): String? {
    var at = location.indexOf(POST_PATH)
    while (at >= 0) {
        val token = location.substring(at + POST_PATH.length).takeWhile { it in '0'..'9' || it in 'a'..'f' }
        if (token.isNotEmpty()) return token
        at = location.indexOf(POST_PATH, at + 1)
    }
    return null
}

private fun postPath(token: String): String = POST_PATH + encodePathSegment(token)

/**
 * The CSV wrapped in the multipart form the import endpoint expects, and the content type
 * naming the boundary it was built with.
 */
internal fun subscriberImportBody(filename: String, csv: ByteArray): Pair<String, ByteArray> {
    val boundary = Random.nextBytes(16).joinToString("") { byte -> (byte.toInt() and 0xFF).toString(16).padStart(2, '0') }
    val head = "--$boundary\r\nContent-Disposition: form-data; name=\"$IMPORT_PART\"; filename=\"${escapeQuotes(importFilename(filename))}\"\r\nContent-Type: application/octet-stream\r\n\r\n"
    val tail = "\r\n--$boundary--\r\n"
    return "multipart/form-data; boundary=$boundary" to (head.encodeToByteArray() + csv + tail.encodeToByteArray())
}

/** The same case-sensitive suffix check Go makes, so every SDK sends the same filename. */
internal fun importFilename(filename: String): String = when {
    filename.isEmpty() -> DEFAULT_IMPORT_FILENAME
    filename.endsWith(".csv") -> filename
    else -> "$filename.csv"
}

/** A quote or a backslash would end the header field early, so both are escaped the way Go's `mime/multipart` escapes them. */
private fun escapeQuotes(text: String): String = text.replace("\\", "\\\\").replace("\"", "\\\"")
