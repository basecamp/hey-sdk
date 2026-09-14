package com.basecamp.hey

import java.security.MessageDigest
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap

@PublishedApi
internal actual fun <V> createServiceCache(): MutableMap<String, V> = ConcurrentHashMap()

internal actual fun currentTimeMillis(): Long = System.currentTimeMillis()

internal actual fun sha256Hex(bytes: ByteArray): String =
    MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

internal actual fun nowIso8601(): String = Instant.now().toString()
