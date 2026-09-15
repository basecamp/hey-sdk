package com.basecamp.hey

/** Creates a thread-safe mutable map for service caching. */
@PublishedApi
internal expect fun <V> createServiceCache(): MutableMap<String, V>

/**
 * The value under [key] in a cache made by [createServiceCache], made with [factory] the
 * first time and only then: two callers asking at the same moment get the same value.
 */
@PublishedApi
internal expect fun <V : Any> MutableMap<String, V>.getOrCreate(key: String, factory: () -> V): V

/** The current time in milliseconds since the epoch. */
internal expect fun currentTimeMillis(): Long

/** The SHA-256 of [bytes], as lowercase hex. */
internal expect fun sha256Hex(bytes: ByteArray): String

/** The current instant as an ISO 8601 timestamp, the form HEY takes a time in. */
internal expect fun nowIso8601(): String
