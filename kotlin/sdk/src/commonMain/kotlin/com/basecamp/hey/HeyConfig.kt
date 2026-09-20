package com.basecamp.hey

import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

/**
 * Configuration for a [HeyClient].
 *
 * Use the builder DSL via the [HeyClient] factory function rather than constructing this
 * directly.
 */
data class HeyConfig(
    /** Where HEY is. Plain HTTP is refused anywhere but this machine. */
    val baseUrl: String = DEFAULT_BASE_URL,
    /** User-Agent header value. */
    val userAgent: String = DEFAULT_USER_AGENT,
    /** Enable ETag-based HTTP caching. */
    val enableCache: Boolean = false,
    /** Enable automatic retry of transient failures. */
    val enableRetry: Boolean = true,
    /** Request timeout. */
    val timeout: Duration = DEFAULT_TIMEOUT,
    /**
     * The most times any operation is resent after a transient failure. A modelled route is
     * resent as many times as its own policy allows and no more; this only lowers that.
     */
    val maxRetries: Int = DEFAULT_MAX_RETRIES,
    /** Maximum pages a walk reads before it stops (safety cap). */
    val maxPages: Int = DEFAULT_MAX_PAGES,
    /** The least the client waits before the first resend; a route's own policy holds it up when longer. */
    val baseRetryDelay: Duration? = null,
    /** The most the client waits between attempts, jitter included. */
    val maxRetryDelay: Duration = DEFAULT_MAX_RETRY_DELAY,
    /** The most added at random to each wait, so resends from many clients do not land together. */
    val maxRetryJitter: Duration = DEFAULT_MAX_RETRY_JITTER,
    /** The most a JSON or HTML answer may deliver before the client refuses to hold it. */
    val maxResponseBodyBytes: Int = DEFAULT_MAX_RESPONSE_BODY_BYTES,
) {
    init {
        // A setting the client cannot run on is a usage error, from here as from the builder.
        if (maxPages <= 0) throw HeyException.Usage("maxPages must be > 0, got: $maxPages")
        if (maxRetries < 0) throw HeyException.Usage("maxRetries must be >= 0, got: $maxRetries")
        // A wait has to be one the client can wait: finite, and not negative.
        if (baseRetryDelay != null && !(baseRetryDelay.isFinite() && !baseRetryDelay.isNegative())) {
            throw HeyException.Usage("baseRetryDelay must be finite and >= 0, got: $baseRetryDelay")
        }
        if (!(maxRetryDelay.isFinite() && !maxRetryDelay.isNegative())) throw HeyException.Usage("maxRetryDelay must be finite and >= 0, got: $maxRetryDelay")
        if (!(maxRetryJitter.isFinite() && !maxRetryJitter.isNegative())) throw HeyException.Usage("maxRetryJitter must be finite and >= 0, got: $maxRetryJitter")
    }

    companion object {
        /** The SDK's own version. `make bump VERSION=x.y.z` moves it with the other SDKs'. */
        const val VERSION = "0.31.1"

        /** The HEY API version this SDK targets; `scripts/sync-api-version.sh` moves it with the spec. */
        const val API_VERSION = "2026-09-20"
        const val DEFAULT_BASE_URL = "https://app.hey.com"

        /** What the client calls itself: the SDK and the API contract it was built against. */
        const val DEFAULT_USER_AGENT = "hey-sdk-kotlin/$VERSION (api:$API_VERSION)"
        const val DEFAULT_MAX_RETRIES = 3
        const val DEFAULT_MAX_PAGES = 10_000
        const val DEFAULT_MAX_RESPONSE_BODY_BYTES = 16 shl 20

        /** The most the client buffers of an answer the configurable cap leaves alone: a blob, whatever a form request answered. */
        const val MAX_RESPONSE_BODY_BYTES = 50 shl 20
        val DEFAULT_TIMEOUT: Duration = 30.seconds
        val DEFAULT_MAX_RETRY_DELAY: Duration = 30.seconds
        val DEFAULT_MAX_RETRY_JITTER: Duration = 100.milliseconds
    }
}
