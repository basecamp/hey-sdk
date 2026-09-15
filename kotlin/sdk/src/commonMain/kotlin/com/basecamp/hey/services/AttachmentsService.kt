package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.Operation
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.json
import com.basecamp.hey.generated.models.CreateDirectUploadRequestContent
import com.basecamp.hey.generated.models.DirectUpload
import com.basecamp.hey.generated.models.DirectUploadBlob
import com.basecamp.hey.generated.models.DirectUploadTarget
import com.basecamp.hey.md5
import com.basecamp.hey.parseAbsoluteUrl
import com.basecamp.hey.requireSecureEndpoint
import io.ktor.http.HttpHeaders
import io.ktor.http.headers
import kotlin.io.encoding.Base64
import com.basecamp.hey.generated.services.AttachmentsService as GeneratedAttachmentsService

/** What an attachment is taken to be when the caller names no content type. */
private const val DEFAULT_CONTENT_TYPE = "application/octet-stream"

/**
 * Attachments service: uploading an outgoing attachment on top of the generated
 * `createDirectUpload`. An upload takes two requests — HEY reserves an Active Storage blob
 * and names a storage URL, and the bytes then go to that URL rather than to HEY.
 */
class AttachmentsService(client: HeyClient) : GeneratedAttachmentsService(client) {
    /**
     * Reserves an Active Storage blob and uploads the bytes to the storage URL HEY named.
     * The answer's `attachableSgid` is what embeds the attachment in Trix rich text.
     *
     * Empty content is an empty attachment rather than a mistake, so only a missing
     * filename is refused.
     */
    suspend fun upload(filename: String, contentType: String? = null, content: ByteArray): DirectUpload {
        if (filename.isEmpty()) throw HeyException.Usage("an attachment needs a filename")
        val body = CreateDirectUploadRequestContent(
            blob = DirectUploadBlob(
                filename = filename,
                byteSize = content.size.toLong(),
                checksum = Base64.encode(md5(content)),
                contentType = contentType ?: DEFAULT_CONTENT_TYPE,
            ),
        )
        // Both requests go quiet inside one operation — the reservation's, as the model
        // describes it — so the hooks hear Attachments.CreateDirectUpload once and hear it
        // end only once the bytes are stored, with the failure when the storage service
        // refuses them.
        val reservation = client.operation(Routes.CREATE_DIRECT_UPLOAD, emptyList())
        reservation.json(body)
        reservation.quiet()
        return client.asOperation(reservation.info) {
            val upload = reserved(client.send<DirectUpload>(reservation))
            store(upload.directUpload, content)
            upload
        }
    }

    /**
     * Puts the bytes to the storage service. This is the one request the SDK makes outside
     * the HEY API: the storage URL authenticates itself and takes exactly the headers HEY
     * named — including any `Authorization` the storage service wants, which is why the HEY
     * credentials must not ride along — so the request goes out unsigned, once, and quietly:
     * the reservation and this are the two requests of the one operation the hooks hear.
     */
    private suspend fun store(target: DirectUploadTarget, content: ByteArray) {
        val url = parseAbsoluteUrl(target.url)
            ?: throw HeyException.Api("HEY named an attachment upload target that is not a URL", httpStatus = null, retryable = false)
        try {
            requireSecureEndpoint(url)
        } catch (refused: HeyException.Usage) {
            throw HeyException.Usage("unsafe attachment upload target: ${refused.message}")
        }
        val operation = Operation.at(Method.PUT, url)
        operation.unsigned()
        operation.quiet()
        operation.idempotent(false)
        operation.accept("*/*")
        var contentType = DEFAULT_CONTENT_TYPE
        for ((name, value) in storageHeaders(target)) {
            if (name.equals(HttpHeaders.ContentType, ignoreCase = true)) contentType = value else operation.header(name, value)
        }
        operation.bodyBytes(contentType, content)
        client.sendUnit(operation)
    }
}

/**
 * The blob HEY reserved, once it carries everything the upload needs. HEY answers 200 with
 * a payload rather than a status when it has nothing to give, so the fields are what say
 * whether there is an upload to make.
 */
private fun reserved(upload: DirectUpload): DirectUpload {
    if (upload.signedId.isEmpty() || upload.attachableSgid.isEmpty() || upload.directUpload.url.isEmpty()) {
        throw HeyException.Api("HEY returned an empty attachment upload response", httpStatus = null, retryable = false)
    }
    return upload
}

/**
 * The headers HEY named, with any `Authorization` among them dropped: the SDK's own
 * credentials never reach the storage service, and neither does a stale one HEY echoed. A
 * header the transport would refuse is refused here, without its value, since the
 * transport's refusal would quote it.
 */
internal fun storageHeaders(target: DirectUploadTarget): List<Pair<String, String>> =
    target.headers.orEmpty().entries
        .filterNot { (name, _) -> name.equals(HttpHeaders.Authorization, ignoreCase = true) }
        .map { (name, value) ->
            try {
                headers { append(name, value) }
            } catch (refused: IllegalArgumentException) {
                throw HeyException.Api("HEY named an attachment upload header \"$name\" that cannot be sent", httpStatus = null, retryable = false)
            }
            name to value
        }
