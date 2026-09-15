package com.basecamp.hey

import io.ktor.http.URLBuilder
import io.ktor.http.Url
import io.ktor.http.parseUrl
import io.ktor.http.takeFrom

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

/**
 * A URL reference HEY sent — a `Location`, a `Link` target — resolved against the URL it came
 * in. An absolute reference stands on its own; a relative one takes the base's origin and
 * nothing else of it, so the query the request went out with never rides along to where the
 * answer points. Null when the reference will not parse.
 */
internal fun resolveReference(base: Url, reference: String): Url? =
    parseAbsoluteUrl(reference) ?: runCatching {
        URLBuilder(base).apply {
            encodedParameters.clear()
            fragment = ""
            takeFrom(reference)
        }.build()
    }.getOrNull()

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
    throw HeyException.Usage("${url.protocol.name}://${url.host} must use HTTPS")
}

private val URL_IN_TEXT = Regex("([a-zA-Z][a-zA-Z0-9+.-]*://[^/?#\\s\"'<>]+)[^\\s\"'<>]*")

/** The text with every URL in it cut back to its origin: a path or a query is what carries a signed credential. */
internal fun redactUrls(text: String): String = URL_IN_TEXT.replace(text) { it.groupValues[1] }

private val SENSITIVE_HEADERS = setOf("authorization", "proxy-authorization", "cookie", "set-cookie", "x-csrf-token")

/** Whether a header carries credentials and must be neither logged nor sent to another origin. */
internal fun isSensitiveHeader(name: String): Boolean = name.lowercase() in SENSITIVE_HEADERS
