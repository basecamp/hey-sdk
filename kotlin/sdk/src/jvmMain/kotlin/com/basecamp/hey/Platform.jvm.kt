package com.basecamp.hey

import java.security.MessageDigest
import java.time.Instant
import java.time.LocalDate
import java.util.concurrent.ConcurrentHashMap

@PublishedApi
internal actual fun <V> createServiceCache(): MutableMap<String, V> = ConcurrentHashMap()

/** `computeIfAbsent` runs the factory once per key, atomically, and hands every concurrent caller the one it made. */
@PublishedApi
internal actual fun <V : Any> MutableMap<String, V>.getOrCreate(key: String, factory: () -> V): V =
    (this as ConcurrentHashMap<String, V>).computeIfAbsent(key) { factory() }

internal actual fun currentTimeMillis(): Long = System.currentTimeMillis()

internal actual fun sha256Hex(bytes: ByteArray): String =
    MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

internal actual fun nowIso8601(): String = Instant.now().toString()

internal actual fun md5(bytes: ByteArray): ByteArray = MessageDigest.getInstance("MD5").digest(bytes)

internal actual fun todayLocalDate(): String = LocalDate.now().toString()
