package com.basecamp.hey

import io.ktor.http.Url
import io.ktor.http.parseUrl

/**
 * Parses an absolute URL with Ktor's own parser — the same parser the transport dials with
 * — so a guard can never disagree with the client about which host a URL targets. Null
 * (fail closed) when the input is malformed or not absolute.
 */
internal fun parseAbsoluteUrl(url: String): Url? {
    val parsed = parseUrl(url) ?: return null
    if (!url.startsWith("${parsed.protocol.name}://", ignoreCase = true)) return null
    return parsed
}

/** Whether a host is this machine: `localhost`, a `.localhost` name, or a loopback address. */
internal fun isLocalhostHost(host: String): Boolean {
    val lowered = host.lowercase().removePrefix("[").removeSuffix("]")
    return lowered == "localhost" || lowered == "127.0.0.1" || lowered == "::1" || lowered.endsWith(".localhost")
}

internal fun isLocalhost(url: Url): Boolean = isLocalhostHost(url.host)

/** Whether two URLs share a scheme, host and port, default ports treated as equal to their explicit form. */
internal fun isSameOrigin(a: Url, b: Url): Boolean =
    a.protocol.name.equals(b.protocol.name, ignoreCase = true) &&
        a.host.equals(b.host, ignoreCase = true) &&
        a.port == b.port

/** Refuses an endpoint that would carry credentials over plain HTTP, unless it is on this machine. */
internal fun requireSecureEndpoint(url: Url) {
    val scheme = url.protocol.name.lowercase()
    if (scheme == "https" || (scheme == "http" && isLocalhost(url))) return
    throw HeyException.Usage("$url must use HTTPS")
}

private val SENSITIVE_HEADERS = setOf("authorization", "cookie", "set-cookie", "x-csrf-token")

/** Whether a header carries credentials and must be neither logged nor sent to another origin. */
internal fun isSensitiveHeader(name: String): Boolean = name.lowercase() in SENSITIVE_HEADERS
