package com.basecamp.hey

/** A response the cache holds for a URL: the validator HEY sent with it, and the body it validates. */
class CachedResponse(
    /** The `ETag` HEY sent with the body, sent back as `If-None-Match` on the next read. */
    val etag: String,
    /** The body as HEY answered it. */
    val body: ByteArray,
) {
    override fun equals(other: Any?): Boolean =
        other is CachedResponse && other.etag == etag && other.body.contentEquals(body)

    override fun hashCode(): Int = 31 * etag.hashCode() + body.contentHashCode()

    override fun toString(): String = "CachedResponse(etag=$etag, ${body.size} bytes)"
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
