package com.basecamp.hey

import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders

/**
 * Provides access tokens for authenticating with the HEY API.
 *
 * ```kotlin
 * // Static token
 * val provider = StaticTokenProvider("your-token")
 *
 * // Dynamic token with refresh
 * val provider = object : TokenProvider {
 *     override suspend fun accessToken() = store.token
 *     override suspend fun refresh() = store.renew()
 * }
 * ```
 *
 * The client asks for the token under the same lock it refreshes under, so a request is
 * signed with credentials a refresh cannot change halfway: [accessToken] and [refresh] are
 * called one at a time, and neither may use the client itself, which would wait on that
 * lock forever. A refresh runs in the client's own scope: the request that earned it being
 * cancelled does not cancel it, and every stale request waits on the same one.
 */
interface TokenProvider {
    /** Returns the current access token. */
    suspend fun accessToken(): String

    /**
     * Asked once when a request is answered with 401. Answer `true` when the next
     * [accessToken] will hand out renewed credentials, and the request is sent again. A
     * refresh that answers `false` or throws is not repeated for the requests already signed
     * with the credentials it could not renew: each of them fails with that answer, what was
     * thrown reaching them as the cause of a [HeyException.Auth]. Only a request signed after
     * the failure asks again. A provider that renews of its own accord — [accessToken] handing
     * over a new token ahead of the old one's expiry — has refreshed as surely as this would:
     * a 401 on the old token is answered by resending with the new one, and this is not asked.
     */
    suspend fun refresh(): Boolean = false
}

/**
 * A [TokenProvider] that always returns the same token. It prints as `[REDACTED]`. A blank
 * token — what an unset environment variable hands over — is refused as a
 * [HeyException.Usage], the failure every other mistake in the client's setup is.
 */
class StaticTokenProvider(token: String) : TokenProvider {
    private val token = SensitiveString(token)

    init {
        if (token.isBlank()) throw HeyException.Usage("Access token must not be blank")
    }

    override suspend fun accessToken(): String = token.expose()

    override fun toString(): String = "StaticTokenProvider($token)"
}

/**
 * Controls how authentication is applied to HTTP requests. The default strategy is
 * [BearerAuth], which uses a [TokenProvider] to set the Authorization header with a Bearer
 * token. Custom strategies can implement alternative auth schemes such as cookie-based auth.
 * [authenticate] and [refresh] are called one at a time, under the client's refresh lock, and
 * neither may send a request through the client itself.
 */
interface AuthStrategy {
    /** Apply authentication to the given request builder. Called before every HTTP request. */
    suspend fun authenticate(request: HttpRequestBuilder)

    /** Asked once when a request is answered with 401; see [TokenProvider.refresh]. */
    suspend fun refresh(): Boolean = false
}

/** Bearer token authentication strategy (default). Sets `Authorization: Bearer {token}` from a [TokenProvider]. */
class BearerAuth(private val tokenProvider: TokenProvider) : AuthStrategy {
    override suspend fun authenticate(request: HttpRequestBuilder) {
        val token = tokenProvider.accessToken()
        // The whole of what a header value may not carry, not only the line breaks: the
        // transport would refuse the rest too, quoting the token in its refusal.
        if (token.any { (it.code < 0x20 && it != '\t') || it.code == 0x7F }) {
            throw HeyException.Auth("access token is not a valid header value")
        }
        request.header(HttpHeaders.Authorization, "Bearer $token")
    }

    override suspend fun refresh(): Boolean = tokenProvider.refresh()

    /** What this would sign with now, as the header value: asked before a refresh, so a token the provider has already renewed is not renewed again. */
    internal suspend fun bearer(): String = "Bearer ${tokenProvider.accessToken()}"
}
