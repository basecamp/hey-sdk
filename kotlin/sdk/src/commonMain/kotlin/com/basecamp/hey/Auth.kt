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
 * lock forever.
 */
interface TokenProvider {
    /** Returns the current access token. */
    suspend fun accessToken(): String

    /**
     * Asked once when a request is answered with 401. Answer `true` when the next
     * [accessToken] will hand out renewed credentials, and the request is sent again.
     */
    suspend fun refresh(): Boolean = false
}

/** A [TokenProvider] that always returns the same token. It prints as `[REDACTED]`. */
class StaticTokenProvider(token: String) : TokenProvider {
    private val token = SensitiveString(token)

    init {
        require(token.isNotBlank()) { "Access token must not be blank" }
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
        if (token.any { it == '\r' || it == '\n' }) {
            throw HeyException.Auth("access token is not a valid header value")
        }
        request.header(HttpHeaders.Authorization, "Bearer $token")
    }

    override suspend fun refresh(): Boolean = tokenProvider.refresh()
}
