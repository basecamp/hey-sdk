package com.basecamp.hey

import kotlin.time.Duration

/**
 * Observability hooks for the HEY SDK.
 *
 * Two levels of hooks:
 * - **Operation-level**: [onOperationStart]/[onOperationEnd] for semantic SDK operations
 *   (e.g. `Boxes.ListBoxes`).
 * - **Request-level**: [onRequestStart]/[onRequestEnd] for HTTP requests (retries, cache, timing).
 *
 * All callbacks are optional. Implementations should not throw — exceptions are caught and
 * silently ignored to prevent hooks from breaking SDK operations.
 */
interface HeyHooks {
    /** Called when a service operation starts. */
    fun onOperationStart(info: OperationInfo) {}

    /** Called when a service operation completes (success or failure). Always called. */
    fun onOperationEnd(info: OperationInfo, result: OperationResult) {}

    /** Called when an HTTP request starts. Called for each attempt including retries. */
    fun onRequestStart(info: RequestInfo) {}

    /** Called when an HTTP request completes. Called for each attempt including retries. */
    fun onRequestEnd(info: RequestInfo, result: RequestResult) {}

    /** Called before a retry attempt, with the attempt about to be made and the wait before it. */
    fun onRetry(info: RequestInfo, attempt: Int, error: Throwable, delayMs: Long) {}
}

/**
 * Describes a semantic SDK operation: the service and operation as the model names them,
 * the kind of record they touch, and whether they change anything. A hand-written wrapper
 * is free to report itself as something other than the route it sends.
 */
data class OperationInfo(
    /** Logical service (e.g. "Boxes", "TimeTracks"). */
    val service: String,
    /** The operation as the model names it (e.g. "ListBoxes"). */
    val operation: String,
    /** HEY resource type in `snake_case` (e.g. "box", "time_track"). */
    val resourceType: String,
    /** Whether this operation modifies state. */
    val isMutation: Boolean,
    /** Specific resource ID if applicable. */
    val resourceId: Long? = null,
)

/** Result of a service operation. */
data class OperationResult(
    /** Operation duration. */
    val duration: Duration,
    /** Error if the operation failed. */
    val error: Throwable? = null,
)

/** Information about an HTTP request. */
data class RequestInfo(
    /** HTTP method (GET, POST, etc.). */
    val method: String,
    /** Full request URL. */
    val url: String,
    /** Current attempt number (1-based), counted across the whole operation. */
    val attempt: Int,
)

/** Result of an HTTP request. */
data class RequestResult(
    /** HTTP status code (0 if request failed before response). */
    val statusCode: Int,
    /** Request duration. */
    val duration: Duration,
    /** Whether the response was served from cache. */
    val fromCache: Boolean = false,
    /** Error if the request failed, whether or not the SDK went on to resend it. */
    val error: Throwable? = null,
)

/** No-op hooks implementation. Zero overhead when unused. */
object NoopHooks : HeyHooks

/**
 * Combines multiple hooks. Start events fire in order, end events in reverse order
 * (proper nesting of spans/traces).
 */
class ChainHooks(private val hooks: List<HeyHooks>) : HeyHooks {
    override fun onOperationStart(info: OperationInfo) {
        for (h in hooks) runCatching { h.onOperationStart(info) }
    }

    override fun onOperationEnd(info: OperationInfo, result: OperationResult) {
        for (h in hooks.asReversed()) runCatching { h.onOperationEnd(info, result) }
    }

    override fun onRequestStart(info: RequestInfo) {
        for (h in hooks) runCatching { h.onRequestStart(info) }
    }

    override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
        for (h in hooks.asReversed()) runCatching { h.onRequestEnd(info, result) }
    }

    override fun onRetry(info: RequestInfo, attempt: Int, error: Throwable, delayMs: Long) {
        for (h in hooks) runCatching { h.onRetry(info, attempt, error, delayMs) }
    }
}

/** Combines multiple hooks implementations. */
fun chainHooks(vararg hooks: HeyHooks): HeyHooks {
    val active = hooks.filter { it !is NoopHooks }
    return when (active.size) {
        0 -> NoopHooks
        1 -> active[0]
        else -> ChainHooks(active)
    }
}

/** Console logging hooks for debugging and development. */
fun consoleHooks(
    logOperations: Boolean = true,
    logRequests: Boolean = false,
    logRetries: Boolean = true,
): HeyHooks = object : HeyHooks {
    override fun onOperationStart(info: OperationInfo) {
        if (!logOperations) return
        val mutation = if (info.isMutation) " [mutation]" else ""
        val resource = info.resourceId?.let { " #$it" } ?: ""
        println("[HEY] ${info.service}.${info.operation}$resource$mutation")
    }

    override fun onOperationEnd(info: OperationInfo, result: OperationResult) {
        if (!logOperations) return
        if (result.error != null) {
            println("[HEY] ${info.service}.${info.operation} failed (${result.duration}): ${result.error.message}")
        } else {
            println("[HEY] ${info.service}.${info.operation} completed (${result.duration})")
        }
    }

    override fun onRequestStart(info: RequestInfo) {
        if (!logRequests) return
        val retry = if (info.attempt > 1) " (attempt ${info.attempt})" else ""
        println("[HEY] -> ${info.method} ${info.url}$retry")
    }

    override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
        if (!logRequests) return
        val cache = if (result.fromCache) " (cached)" else ""
        println("[HEY] <- ${info.method} ${info.url} ${result.statusCode} (${result.duration})$cache")
    }

    override fun onRetry(info: RequestInfo, attempt: Int, error: Throwable, delayMs: Long) {
        if (!logRetries) return
        println("[HEY] Retrying ${info.method} ${info.url} (attempt $attempt, waiting ${delayMs}ms): ${error.message}")
    }
}

internal fun HeyHooks.safeOperationStart(info: OperationInfo) {
    runCatching { onOperationStart(info) }
}

internal fun HeyHooks.safeOperationEnd(info: OperationInfo, result: OperationResult) {
    runCatching { onOperationEnd(info, result) }
}

internal fun HeyHooks.safeRequestStart(info: RequestInfo) {
    runCatching { onRequestStart(info) }
}

internal fun HeyHooks.safeRequestEnd(info: RequestInfo, result: RequestResult) {
    runCatching { onRequestEnd(info, result) }
}

internal fun HeyHooks.safeRetry(info: RequestInfo, attempt: Int, error: Throwable, delayMs: Long) {
    runCatching { onRetry(info, attempt, error, delayMs) }
}
