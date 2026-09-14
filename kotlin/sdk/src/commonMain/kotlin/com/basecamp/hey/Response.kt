package com.basecamp.hey

import io.ktor.http.Headers
import io.ktor.http.Url
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationException
import kotlinx.serialization.serializer

/** What came back from HEY, before it is decoded. */
class Response internal constructor(
    /** What HEY answered. */
    val status: Int,
    /** The headers that came with it. */
    val headers: Headers,
    /** The body, read whole. */
    val body: ByteArray,
    /** Where the answer came from, once any redirects were followed. */
    val url: Url,
    /** The body came out of the response cache: HEY answered 304 and the SDK read the entry it was holding. */
    val fromCache: Boolean,
    /** The operation takes this status for an answer rather than a failure: a 404 that means "nothing there", or the redirect a form request went out to collect. */
    val empty: Boolean,
) {
    /** One header's value, when HEY sent it. */
    fun header(name: String): String? = headers[name]

    /** The body as text. */
    fun text(): String = body.decodeToString()

    /**
     * Decodes the body as JSON. A body that will not decode is an error that still says what
     * HEY answered: the status, and the request id when the answer named one.
     */
    fun <T> json(deserializer: DeserializationStrategy<T>): T {
        if (body.isEmpty()) {
            throw HeyException.Api("empty response body", httpStatus = status, requestId = header("X-Request-Id"), retryable = false)
        }
        return try {
            heyJson.decodeFromString(deserializer, text())
        } catch (error: SerializationException) {
            throw HeyException.Api(
                "unexpected JSON in the response",
                httpStatus = status,
                hint = HeyException.truncateMessage(error.message ?: "unreadable"),
                retryable = false,
                requestId = header("X-Request-Id"),
                cause = error,
            )
        } catch (error: IllegalArgumentException) {
            throw HeyException.Api(
                "unexpected JSON in the response",
                httpStatus = status,
                hint = HeyException.truncateMessage(error.message ?: "unreadable"),
                retryable = false,
                requestId = header("X-Request-Id"),
                cause = error,
            )
        }
    }

    /** Decodes the body as [T]. */
    inline fun <reified T> json(): T = json(serializer<T>())
}
