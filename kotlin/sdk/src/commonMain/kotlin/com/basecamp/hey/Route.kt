package com.basecamp.hey

/** The HTTP methods the model's routes are sent with. */
enum class Method { GET, POST, PUT, PATCH, DELETE }

/** Where a path parameter sits: the last segment names the record itself, anything before it a parent. */
enum class ParamRole {
    /** Names a record the one the route acts on belongs to: the box a group is in. */
    PARENT,

    /** Names the record the route acts on. */
    RECORDING,
}

/** The type the model gives a path parameter's value. */
enum class ParamKind { STRING, BOOL, INT32, INT64 }

/** How a route pages its answer. */
enum class Pagination {
    /** The whole answer comes at once. */
    NONE,

    /** HEY names the next page in a `Link` header; see [Page]. */
    LINK,

    /** The read covers a window of dates, and the caller moves the window to read on. */
    WINDOW,
}

/**
 * The retry policy the model attaches to a route: how many attempts in all, how long the
 * first wait is, and which statuses are worth another try.
 */
data class RetryPolicy(
    /** The most attempts the route is given, the first one included. Zero is one send and no resend. */
    val max: Int,
    /** The wait before the second attempt, in milliseconds; later waits double it. */
    val baseDelayMs: Long,
    /** The statuses that are worth another attempt. */
    val retryOn: List<Int>,
)

/** One `{param}` placeholder in a route's path. */
data class RouteParam(
    /** The placeholder's name as it appears in the path: `boxId`. */
    val name: String,
    /** Whether the parameter names the record itself or a parent of it. */
    val role: ParamRole,
    /** The type the model gives the value. */
    val kind: ParamKind,
)

/**
 * One API operation: its method, its path template and the behaviour the Smithy model
 * attaches to it. Every route the SDK knows is in [com.basecamp.hey.routes.Routes].
 */
data class Route(
    /** The operation as the model names it: `ListBoxes`, `GetTopic`. */
    val id: String,
    /** The service handle whose method sends this route, as every HEY SDK names it: `Boxes`, `TimeTracks`. */
    val service: String,
    /** The HTTP method the route is sent with. */
    val method: Method,
    /** The path as HEY serves it, `{param}` placeholders included. */
    val path: String,
    /** The path without a `.json` suffix, for recognizing pasted URLs. */
    val pattern: String,
    /** The part of HEY the route belongs to, as the model titles it: `Boxes`, `Calendar Time Tracks`. */
    val resource: String,
    /** The kind of record the route acts on, in `snake_case`: `box`, `box_group`. */
    val resourceType: String,
    /** The path parameters, in the order they appear in [path]. */
    val params: List<RouteParam>,
    /** The route may be sent again after a failure without doing its work twice. */
    val idempotent: Boolean,
    /** The route only reads; nothing it does changes anything. */
    val readonly: Boolean,
    /** The route answers a page as HTML, so it is asked for as written — no `.json` suffix — with `Accept: text/html`. */
    val html: Boolean,
    /** The statuses that mean HEY has nothing for this route rather than that it failed. */
    val emptyOn: List<Int>,
    /** How the route pages, when it does. */
    val pagination: Pagination,
    /** The query parameter a `Link` names a further page with, when the route pages; a `Link` without it is a cursor to poll next, not a page. */
    val pageParameter: String?,
    /** The retry policy the model attaches to the route. */
    val retry: RetryPolicy,
) {
    /**
     * Substitutes the path parameters, in order, percent-encoding each value.
     *
     * @throws HeyException.Usage when [values] is not exactly as long as [params]: a short
     *   list would leave a `{param}` in the path and send it to HEY as written.
     */
    fun fill(values: List<Any>): String {
        if (values.size != params.size) {
            throw HeyException.Usage("$id takes ${params.size} path parameters, got ${values.size}")
        }
        var filled = path
        for ((param, value) in params.zip(values)) {
            filled = filled.replace("{${param.name}}", encodePathSegment(value.toString()))
        }
        return filled
    }

    /** Matches a path against the route's pattern and answers the captured parameters, or null. */
    fun recognize(path: String): Map<String, String>? {
        val patternSegments = pattern.split('/')
        val pathSegments = path.split('/')
        if (patternSegments.size != pathSegments.size) return null
        val captured = linkedMapOf<String, String>()
        for ((expected, actual) in patternSegments.zip(pathSegments)) {
            if (expected.startsWith("{") && expected.endsWith("}")) {
                if (actual.isEmpty()) return null
                val name = expected.substring(1, expected.length - 1)
                if (params.none { it.name == name }) return null
                captured[name] = actual
            } else if (expected != actual) {
                return null
            }
        }
        return captured
    }
}

private const val UNRESERVED = "-_.~"

/** Percent-encodes everything but the unreserved characters, so a value with a `/` in it stays one segment. */
internal fun encodePathSegment(value: String): String = buildString {
    for (byte in value.encodeToByteArray()) {
        val character = byte.toInt().toChar()
        if (character.isLetterOrDigit() && byte >= 0 || character in UNRESERVED) {
            append(character)
        } else {
            append('%')
            append(((byte.toInt() shr 4) and 0xF).toString(16).uppercase())
            append((byte.toInt() and 0xF).toString(16).uppercase())
        }
    }
}
