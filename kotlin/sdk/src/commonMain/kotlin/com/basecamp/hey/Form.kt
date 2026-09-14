package com.basecamp.hey

import io.ktor.http.parseUrl

/**
 * The answer to a form request. A redirect is captured rather than followed, so a 302 or
 * 303 arrives here with its `Location` intact; an endpoint reached on a `.json` path
 * answers the record itself, which lands in [body] instead.
 */
class FormResponse internal constructor(
    /** Where the redirect pointed, exactly as HEY wrote it — often a path rather than a whole URL. */
    val location: String?,
    /** The status the endpoint answered: a 302 or 303 for a redirect, a 200 for a document. */
    val status: Int,
    /** What the endpoint answered when it answered a document instead of a redirect. */
    val body: String,
) {
    /**
     * The id of the record the redirect named: the rightmost path segment that reads as a
     * number, so `/calendar/events/42` and `/calendar/events/42/edit` both answer 42.
     */
    fun extractId(): Long {
        val target = location?.takeIf { it.isNotEmpty() }
            ?: throw HeyException.Api("no location header in response", httpStatus = null, retryable = false)
        val path = if (target.contains("://")) {
            (parseUrl(target) ?: throw HeyException.Api("failed to parse location URL: $target", httpStatus = null, retryable = false)).encodedPath
        } else {
            target.substringBefore('?').substringBefore('#')
        }
        return path.trimEnd('/').split('/').asReversed().firstNotNullOfOrNull { it.toLongOrNull() }
            ?: throw HeyException.Api("no numeric ID found in location: $target", httpStatus = null, retryable = false)
    }

    internal companion object {
        /** The client hands over a 302 or 303 with its `Location`, or the document a `.json` path answered; any other redirect failed before it got here. */
        fun of(response: Response): FormResponse =
            if (response.empty && response.status in 300..399) {
                FormResponse(response.header("Location"), response.status, "")
            } else {
                FormResponse(null, response.status, response.text())
            }
    }
}

/** What a write to a path outside the model announces itself as. */
fun writeInfo(service: String, operation: String, resourceType: String, resourceId: Long? = null): OperationInfo =
    OperationInfo(service = service, operation = operation, resourceType = resourceType, isMutation = true, resourceId = resourceId)
