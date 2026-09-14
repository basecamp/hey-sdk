package com.basecamp.hey

import io.ktor.http.Headers
import io.ktor.http.headers

/**
 * A response the cache holds for a URL: the validator HEY sent with it, the body it
 * validates, and the headers that came with the body, so a 304 answers the page as it was
 * first read — `Link` and `X-Total-Count` included, which a 304 need not repeat.
 */
class CachedResponse(
    /** The `ETag` HEY sent with the body, sent back as `If-None-Match` on the next read. */
    val etag: String,
    /** The body as HEY answered it. */
    val body: ByteArray,
    /** The headers HEY answered the body with, less any credential. */
    val headers: Map<String, List<String>> = emptyMap(),
) {
    /** The cached headers, with each one the 304 carried replacing what was held under its name. */
    internal fun headersUpdatedBy(fresh: Headers): Headers = headers {
        for ((name, values) in this@CachedResponse.headers) appendAll(name, values)
        fresh.forEach { name, values ->
            remove(name)
            appendAll(name, values)
        }
    }

    override fun equals(other: Any?): Boolean =
        other is CachedResponse && other.etag == etag && other.body.contentEquals(body) && other.headers == headers

    override fun hashCode(): Int = 31 * (31 * etag.hashCode() + body.contentHashCode()) + headers.hashCode()

    override fun toString(): String = "CachedResponse(etag=$etag, ${body.size} bytes, ${headers.size} headers)"
}

/** Stores JSON responses by `ETag` so a repeated read can be answered from a 304. */
interface ResponseCache {
    /** What is held under [key], if anything. */
    fun get(key: String): CachedResponse?

    /** Holds a response under [key], replacing whatever was there. */
    fun set(key: String, response: CachedResponse)

    /** Forgets what is held under [key]. */
    fun invalidate(key: String)

    /** Forgets everything. */
    fun clear()
}

/** Keeps cached responses for the life of the process. What `enableCache` turns on. */
class InMemoryCache : ResponseCache {
    private val entries: MutableMap<String, CachedResponse> = createServiceCache()

    override fun get(key: String): CachedResponse? = entries[key]

    override fun set(key: String, response: CachedResponse) {
        entries[key] = response
    }

    override fun invalidate(key: String) {
        entries.remove(key)
    }

    override fun clear() {
        entries.clear()
    }

    /** How many responses are held. */
    val size: Int get() = entries.size
}

/**
 * The key a read is cached under: the URL and the credential it went out with, hashed, so
 * one identity's reading is never answered to another and the token never sits in a key.
 */
internal fun cacheKey(url: String, credential: String): String = sha256Hex("$credential\n$url".encodeToByteArray())
