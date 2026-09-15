package com.basecamp.hey

import io.ktor.http.Url
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.serialization.DeserializationStrategy

/**
 * One page of a paginated read, with the cursor HEY handed out for the next one.
 *
 * ```kotlin
 * val first = client.boxes.getImbox()
 * client.eachPage(first) { page -> println(page.value.postings?.size); true }
 * ```
 */
class Page<T> internal constructor(
    /** The page's value: the response as HEY answered it. */
    val value: T,
    /** The URL of the page after this one, as HEY's `Link` header named it. */
    val nextUrl: Url?,
    /** The opaque cursor for the page after this one, to pass as `page` on the same read. */
    val nextPage: String?,
    /**
     * Where to read next once the pages run out, when HEY named one: a change feed's last
     * page links to the cursor to poll from later rather than to another page. A walk stops
     * here; the URL is the caller's to come back to.
     */
    val nextCursor: Url?,
    /** The `X-Total-Count` header, when the read carried one. */
    val totalCount: Long?,
    internal val info: OperationInfo,
    internal val route: Route?,
    internal val deserializer: DeserializationStrategy<T>,
) {
    /** Whether HEY named a page after this one. */
    val hasNext: Boolean get() = nextUrl != null

    override fun toString(): String = "Page(value=$value, nextPage=$nextPage, nextCursor=$nextCursor, totalCount=$totalCount)"

    internal companion object {
        fun <T> of(
            value: T,
            response: Response,
            info: OperationInfo,
            route: Route?,
            deserializer: DeserializationStrategy<T>,
        ): Page<T> {
            val linked = response.header("Link")?.let(::nextLink)?.let { target -> resolveReference(response.url, target) }
            // A Link that names a further page carries the page parameter; one that does not is
            // where to poll next — a change feed's last page says so — and no page at all.
            val pageParameter = route?.pageParameter ?: "page"
            val nextPage = linked?.parameters?.get(pageParameter)
            val nextUrl = linked?.takeIf { nextPage != null }
            val nextCursor = linked?.takeIf { nextPage == null }
            val totalCount = response.header("X-Total-Count")?.trim()?.toLongOrNull()
            return Page(value, nextUrl, nextPage, nextCursor, totalCount, info, route, deserializer)
        }
    }
}

/**
 * The target of the `rel="next"` link in a `Link` header, or null when the header names
 * none.
 */
fun nextLink(header: String): String? {
    var remaining = header
    while (true) {
        val start = remaining.indexOf('<')
        if (start < 0) return null
        val afterStart = remaining.substring(start + 1)
        val end = afterStart.indexOf('>')
        if (end < 0) return null
        val target = afterStart.substring(0, end)
        val rest = afterStart.substring(end + 1)
        val paramsEnd = rest.indexOf('<').let { if (it < 0) rest.length else it }
        if (linkIsNext(rest.substring(0, paramsEnd))) return target
        remaining = rest.substring(paramsEnd)
    }
}

private fun linkIsNext(params: String): Boolean =
    params.split(';').any { param ->
        val parts = param.split('=', limit = 2)
        val name = parts[0].trim()
        val value = parts.getOrNull(1)?.trim()?.trim('"') ?: ""
        name.equals("rel", ignoreCase = true) && value.split(' ').any { it.equals("next", ignoreCase = true) }
    }

/**
 * Reads the page after the given one, or null when HEY named no next page. A `Link` header
 * pointing off the HEY origin is refused rather than followed. The read announces itself as
 * the operation the first page came from, and is resent under that operation's retry policy.
 */
suspend fun <T> HeyClient.nextPage(page: Page<T>): Page<T>? {
    val next = page.nextUrl ?: return null
    if (!isSameOrigin(next, baseUrl)) {
        throw HeyException.Usage("pagination Link header points to a different origin: ${next.protocol.name}://${next.host}")
    }
    val operation = Operation.at(Method.GET, next, page.route)
    operation.info(page.info)
    return sendPage(operation, page.deserializer)
}

/**
 * Reads every page after the first, calling [visit] with each one, until it answers false or
 * the pages run out. A walk that reaches the client's page limit with pages still to read
 * stops there and fails, rather than answering a shorter list that looks complete.
 */
suspend fun <T> HeyClient.eachPage(first: Page<T>, visit: suspend (Page<T>) -> Boolean) {
    var page = first
    var count = 1
    while (visit(page)) {
        if (!page.hasNext) break
        if (count >= config.maxPages) {
            throw HeyException.Api("pagination stopped after ${config.maxPages} pages with more to read", httpStatus = null, retryable = false)
        }
        page = nextPage(page) ?: break
        count += 1
    }
}

/** Every page from the first on, as a cold flow that reads each page as it is collected. */
fun <T> HeyClient.pages(first: Page<T>): Flow<Page<T>> = flow {
    eachPage(first) { page ->
        emit(page)
        true
    }
}
