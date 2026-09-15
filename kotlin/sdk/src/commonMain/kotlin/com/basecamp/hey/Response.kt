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
            throw undecodable(error)
        } catch (error: IllegalArgumentException) {
            throw undecodable(error)
        }
    }

    /**
     * The error for a body that will not decode. It says where the body went wrong — the
     * field, the path, the offset — and never what the body held: the decoder's own message
     * quotes the input, and a body can carry an address or a name, so neither the message
     * nor the exception that carried it is kept.
     */
    private fun undecodable(error: Exception): HeyException.Api =
        HeyException.Api(
            "unexpected JSON in the response",
            httpStatus = status,
            hint = decodeHint(error.message),
            retryable = false,
            requestId = header("X-Request-Id"),
        )

    /** Decodes the body as [T]. */
    inline fun <reified T> json(): T = json(serializer<T>())
}

/**
 * What a decoder's message says about where it stopped, with the input it quotes left out:
 * a required field it names, the path it reached, the offset it reached.
 */
internal fun decodeHint(message: String?): String {
    if (message == null) return "body does not decode"
    // The decoder quotes the input after "JSON input:"; only what comes before is its own
    // diagnostic, and only names shaped like the model's — a field, a path of fields and
    // indexes — are taken from that, since a map key in a path is the body's.
    val diagnostic = message.substringBefore("JSON input")
    val parts = mutableListOf<String>()
    Regex("Field '([A-Za-z0-9_]+)' is required")
        .find(diagnostic)
        ?.let { parts += "missing required field '${it.groupValues[1]}'" }
    Regex("path: (\\$(?:\\.[A-Za-z0-9_]+|\\[\\d+\\])*)(?=[\\s,]|$)").find(diagnostic)?.let { parts += "at path ${it.groupValues[1]}" }
    Regex("offset (\\d+)").find(diagnostic)?.let { parts += "at offset ${it.groupValues[1]}" }
    return if (parts.isEmpty()) "body does not decode" else "body does not decode: " + parts.joinToString(", ")
}
