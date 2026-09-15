package com.basecamp.hey

import io.ktor.http.Headers
import io.ktor.http.fromHttpToGmtDate
import io.ktor.util.date.GMTDate
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * Sealed class hierarchy for HEY API errors.
 *
 * Enables exhaustive `when` matching for error handling:
 * ```kotlin
 * try {
 *     client.boxes.get(boxId)
 * } catch (e: HeyException) {
 *     when (e) {
 *         is HeyException.Auth -> println("Token expired")
 *         is HeyException.NotFound -> println("Not found")
 *         is HeyException.RateLimit -> println("Retry in ${e.retryAfterSeconds}s")
 *         is HeyException.Forbidden -> println("Access denied")
 *         is HeyException.Validation -> println("Invalid input: ${e.message}")
 *         is HeyException.Conflict -> println("Conflict: ${e.message}")
 *         is HeyException.Network -> println("Network error")
 *         is HeyException.Api -> println("Server error: ${e.httpStatus}")
 *         is HeyException.Usage -> println("Bad arguments: ${e.message}")
 *         is HeyException.Ambiguous -> println("Ambiguous: ${e.message}")
 *     }
 * }
 * ```
 */
sealed class HeyException(
    message: String,
    /** Error category code matching the Go and Rust SDKs. */
    val code: String,
    /** What to do about it, when there is a word to say: HEY's own message, or when to try again. */
    val hint: String? = null,
    /** HTTP status code that caused the error, if applicable. */
    val httpStatus: Int? = null,
    /** Whether the operation can be retried. */
    val retryable: Boolean = false,
    /** Request ID from the server for debugging. */
    val requestId: String? = null,
    /** What HEY answered the failure with, up to [MAX_ERROR_BODY_BYTES]; null when there was no body or the SDK raised the error itself. */
    val body: ByteArray? = null,
    cause: Throwable? = null,
) : Exception(message, cause) {
    /** Exit code for CLI applications (matches the Go and Rust SDKs). */
    val exitCode: Int get() = exitCodeFor(code)

    /** The answer was longer than the client will hold in memory, so [body] is missing and [hint] says so. */
    var responseTooLarge: Boolean = false
        internal set

    /** The failure body as text, when there is one. */
    fun bodyText(): String? = body?.decodeToString()

    override fun toString(): String {
        val name = this::class.simpleName
        return if (hint == null) "$name: $message" else "$name: $message: $hint"
    }

    /** Authentication error (401). */
    class Auth(
        message: String = "authentication required",
        hint: String? = null,
        requestId: String? = null,
        body: ByteArray? = null,
    ) : HeyException(message, CODE_AUTH, hint, 401, false, requestId, body)

    /** Forbidden error (403). */
    class Forbidden(
        message: String = "access denied",
        hint: String? = null,
        requestId: String? = null,
        body: ByteArray? = null,
    ) : HeyException(message, CODE_FORBIDDEN, hint, 403, false, requestId, body)

    /** Not found error (404). */
    class NotFound(
        message: String = "resource not found",
        hint: String? = null,
        requestId: String? = null,
        body: ByteArray? = null,
    ) : HeyException(message, CODE_NOT_FOUND, hint, 404, false, requestId, body)

    /** Rate limit error (429). Retryable with optional Retry-After. */
    class RateLimit(
        /** Number of seconds to wait before retrying, from the Retry-After header. */
        val retryAfterSeconds: Long? = null,
        message: String = "rate limited - try again later",
        hint: String? = retryHint(retryAfterSeconds),
        requestId: String? = null,
        body: ByteArray? = null,
    ) : HeyException(message, CODE_RATE_LIMIT, hint, 429, true, requestId, body)

    /**
     * Network error (connection failures, DNS, timeout). Retryable because the request can be
     * sent again, not because it never arrived: the client only resends on its own for an
     * operation the model calls idempotent.
     */
    class Network(
        message: String = "Network error",
        hint: String? = null,
        cause: Throwable? = null,
        retryable: Boolean = true,
    ) : HeyException(message, CODE_NETWORK, hint, null, retryable, null, null, cause)

    /** Generic API error (5xx or unexpected status codes), or an answer the SDK could not read. */
    class Api(
        message: String,
        httpStatus: Int? = null,
        hint: String? = null,
        retryable: Boolean = httpStatus != null && httpStatus in 500..599,
        requestId: String? = null,
        body: ByteArray? = null,
        cause: Throwable? = null,
        responseTooLarge: Boolean = false,
    ) : HeyException(message, CODE_API, hint, httpStatus, retryable, requestId, body, cause) {
        init {
            this.responseTooLarge = responseTooLarge
        }
    }

    /** Validation error: HEY's 422. A 400 is an [Api] error with that status, as it is in the Go and Rust SDKs. */
    class Validation(
        message: String = "validation error",
        hint: String? = null,
        httpStatus: Int = 422,
        requestId: String? = null,
        body: ByteArray? = null,
    ) : HeyException(message, CODE_VALIDATION, hint, httpStatus, false, requestId, body)

    /** Conflict error (409): the request conflicts with what HEY already holds, such as a time track already running. */
    class Conflict(
        message: String = "conflict",
        hint: String? = null,
        requestId: String? = null,
        body: ByteArray? = null,
    ) : HeyException(message, CODE_CONFLICT, hint, 409, false, requestId, body)

    /** Ambiguous match error (multiple resources match a name/identifier). */
    class Ambiguous(
        /** The type of resource that was ambiguous. */
        val resource: String,
        /** The matching resources. */
        val matches: List<String> = emptyList(),
        hint: String? = if (matches.isNotEmpty() && matches.size <= 5) "Did you mean: ${matches.joinToString(", ")}" else "Be more specific",
    ) : HeyException("Ambiguous $resource", CODE_AMBIGUOUS, hint)

    /** Usage error (bad arguments, configuration errors). */
    class Usage(
        message: String,
        hint: String? = null,
    ) : HeyException(message, CODE_USAGE, hint)

    companion object {
        const val CODE_AUTH = "auth_required"
        const val CODE_FORBIDDEN = "forbidden"
        const val CODE_NOT_FOUND = "not_found"
        const val CODE_RATE_LIMIT = "rate_limit"
        const val CODE_NETWORK = "network"
        const val CODE_API = "api_error"
        const val CODE_VALIDATION = "validation"
        const val CODE_CONFLICT = "conflict"
        const val CODE_AMBIGUOUS = "ambiguous"
        const val CODE_USAGE = "usage"

        private const val EXIT_USAGE = 1
        private const val EXIT_NOT_FOUND = 2
        private const val EXIT_AUTH = 3
        private const val EXIT_FORBIDDEN = 4
        private const val EXIT_RATE_LIMIT = 5
        private const val EXIT_NETWORK = 6
        private const val EXIT_API = 7
        private const val EXIT_AMBIGUOUS = 8
        private const val EXIT_VALIDATION = 9

        /** The most of a failure's body an error keeps. */
        const val MAX_ERROR_BODY_BYTES: Int = 1 shl 20

        /** Maximum length for error messages to prevent unbounded memory growth. */
        const val MAX_ERROR_MESSAGE_LENGTH: Int = 500

        /** Maps an error code to a CLI exit code. A conflict leaves as a validation failure, as it does in Go. */
        fun exitCodeFor(code: String): Int = when (code) {
            CODE_USAGE -> EXIT_USAGE
            CODE_NOT_FOUND -> EXIT_NOT_FOUND
            CODE_AUTH -> EXIT_AUTH
            CODE_FORBIDDEN -> EXIT_FORBIDDEN
            CODE_RATE_LIMIT -> EXIT_RATE_LIMIT
            CODE_NETWORK -> EXIT_NETWORK
            CODE_API -> EXIT_API
            CODE_AMBIGUOUS -> EXIT_AMBIGUOUS
            CODE_VALIDATION, CODE_CONFLICT -> EXIT_VALIDATION
            else -> EXIT_API
        }

        /** Truncates error messages to a safe length. */
        fun truncateMessage(s: String): String =
            if (s.length <= MAX_ERROR_MESSAGE_LENGTH) s else s.take(MAX_ERROR_MESSAGE_LENGTH - 3) + "..."

        /**
         * Maps a non-2xx response onto the SDK's error vocabulary. The hint carries whatever
         * message the server put in the body, when it sent one as JSON — an HTML error page
         * is never echoed — and the body itself is kept on the error for a caller that needs
         * more of it than a hint.
         */
        fun fromResponse(status: Int, method: Method, headers: Headers, body: ByteArray): HeyException = fromResponse(status, method, headers, body, null)

        /**
         * As [fromResponse], for a failure whose body the client refused to hold: the error
         * is still the one the status maps to, its hint is why the body is missing, and it
         * says so in [responseTooLarge].
         */
        internal fun fromResponse(status: Int, method: Method, headers: Headers, body: ByteArray, refusal: HeyException?): HeyException {
            val requestId = headers["X-Request-Id"]
            val kept = if (body.size > MAX_ERROR_BODY_BYTES) body.copyOf(MAX_ERROR_BODY_BYTES) else body
            val stored = kept.takeIf { it.isNotEmpty() }
            val message = serverMessage(body) ?: refusal?.message
            return mapped(status, method, headers, message, requestId, stored).also { it.responseTooLarge = refusal != null }
        }

        private fun mapped(status: Int, method: Method, headers: Headers, message: String?, requestId: String?, stored: ByteArray?): HeyException {
            return when (status) {
                401 -> Auth(hint = message, requestId = requestId, body = stored)
                403 -> if (method == Method.GET) {
                    Forbidden(hint = message, requestId = requestId, body = stored)
                } else {
                    Forbidden("Access denied: insufficient scope", hint = message ?: "Re-authenticate with full scope", requestId = requestId, body = stored)
                }
                404 -> NotFound(hint = message, requestId = requestId, body = stored)
                409 -> Conflict(hint = message, requestId = requestId, body = stored)
                422 -> Validation(hint = message, requestId = requestId, body = stored)
                429 -> {
                    val retryAfter = retryAfterSeconds(headers["Retry-After"])
                    RateLimit(retryAfter, hint = message ?: retryHint(retryAfter), requestId = requestId, body = stored)
                }
                else -> Api("API error: $status", httpStatus = status, hint = message, retryable = status in 500..599, requestId = requestId, body = stored)
            }
        }

        private fun retryHint(retryAfter: Long?): String =
            if (retryAfter != null && retryAfter > 0) "Retry after $retryAfter seconds" else "Try again later"

        /**
         * The message a JSON failure body carries: `error`, then `message`, then an `errors`
         * list joined. Anything that is not a JSON object — an HTML page, a bare string —
         * carries none.
         */
        internal fun serverMessage(body: ByteArray): String? {
            if (body.isEmpty()) return null
            val parsed = runCatching { heyJson.parseToJsonElement(body.decodeToString()) }.getOrNull()
            val fields = parsed as? JsonObject ?: return null
            stringMember(fields, "error")?.let { return truncateMessage(it) }
            stringMember(fields, "message")?.let { return truncateMessage(it) }
            val errors = fields["errors"] as? JsonArray ?: return null
            val messages = errors.mapNotNull { (it as? JsonPrimitive)?.takeIf { primitive -> primitive.isString }?.content }
            return messages.takeIf { it.isNotEmpty() }?.let { truncateMessage(it.joinToString("; ")) }
        }

        private fun stringMember(fields: JsonObject, key: String): String? =
            (fields[key] as? JsonPrimitive)?.takeIf { it.isString }?.content?.takeIf { it.isNotBlank() }
    }
}

/**
 * The seconds a `Retry-After` header names: an integer, or an HTTP-date reduced to the time
 * left until it. Null for a header that is missing or unreadable.
 */
internal fun retryAfterSeconds(value: String?): Long? {
    val trimmed = value?.trim()?.takeIf { it.isNotEmpty() } ?: return null
    // delay-seconds is digits only: a negative number is not a wait of nothing, it is no wait HEY named.
    trimmed.toLongOrNull()?.let { return if (it < 0) null else it }
    val target = runCatching { trimmed.fromHttpToGmtDate() }.getOrNull() ?: return null
    val remaining = target.timestamp - GMTDate().timestamp
    return if (remaining > 0) (remaining + 999) / 1000 else 0
}
