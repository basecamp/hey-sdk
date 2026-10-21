package com.basecamp.hey

import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders
import kotlinx.coroutines.test.runTest
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import io.ktor.utils.io.writeFully
import io.ktor.utils.io.writer
import kotlinx.coroutines.DelicateCoroutinesApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.GlobalScope
import kotlinx.coroutines.delay
import kotlinx.io.IOException
import kotlin.time.Duration.Companion.milliseconds
import kotlin.test.Test
import kotlin.time.Duration
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class RevocationTest {
    private class Credentials : TokenProvider {
        var refreshes = 0

        override suspend fun accessToken(): String = "the-session"

        override suspend fun refresh(): Boolean {
            refreshes += 1
            return true
        }
    }

    // The token goes in a form body, never the query, where logs would keep it; and the request
    // carries none of the client's credentials, since the token they would carry may be the one
    // being revoked.
    @Test
    fun aRevocationSendsTheClientAndTheTokenInAFormWithNoCredentials() = runTest {
        val hey = mockHey(ok("{}"))
        hey.client { accessToken(Credentials()) }.revokeToken("the-app", "refresh-to-revoke")
        val request = hey.requests.single()
        assertEquals("DELETE", request.method)
        assertEquals("/oauth/tokens", request.path)
        assertEquals("", request.url.encodedQuery)
        assertEquals(listOf("client_id" to "the-app", "token" to "refresh-to-revoke"), formPairs(request.body))
        assertNull(request.header(HttpHeaders.Authorization))
    }

    // A strategy of the caller's own signs nothing on it either, whatever headers it would set.
    @Test
    fun aRevocationCarriesNoneOfACallersStrategysHeaders() = runTest {
        val hey = mockHey(ok("{}"))
        val strategy = object : AuthStrategy {
            override suspend fun authenticate(request: HttpRequestBuilder) {
                request.header(HttpHeaders.Authorization, "DPoP the-session")
                request.header("DPoP", "a-proof")
            }
        }
        HeyClient {
            auth(strategy)
            engine = hey.engine
            timeout = Duration.INFINITE
        }.revokeToken("the-app", "refresh-to-revoke")
        val request = hey.requests.single()
        assertNull(request.header(HttpHeaders.Authorization))
        assertNull(request.header("DPoP"))
    }

    // The session is the identity's, not an account's: a scoped client revokes it unfiltered.
    @Test
    fun anAccountScopedClientRevokesWithNoAccountFilter() = runTest {
        val hey = mockHey(ok(IDENTITY), ok("{}"))
        hey.client().forAccount(42).revokeToken("the-app", "refresh-to-revoke")
        assertEquals("/oauth/tokens", hey.requests[1].path)
        assertNull(hey.requests[1].query("filtered_account_id"))
    }

    // HEY answers 200 {} for a token that names nothing: the point of revoking it is reached.
    @Test
    fun aTokenHeyNoLongerKnowsIsRevokedWithoutAnError() = runTest {
        val hey = mockHey(ok("{}"))
        hey.client().revokeToken("the-app", "a-token-nobody-knows")
        assertEquals(1, hey.requests.size)
    }

    // A token another app was handed is refused invalid_client. Refreshing on that 401 would
    // renew the very credentials the caller set out to end.
    @Test
    fun aRevocationRefused401IsAnErrorSentOnceWithNoRefresh() = runTest {
        val hey = mockHey(status(401, """{"error":"invalid_client"}"""))
        val credentials = Credentials()
        assertFailsWith<HeyException.Auth> {
            hey.client { accessToken(credentials) }.revokeToken("the-app", "another-apps-token")
        }
        assertEquals(1, hey.requests.size)
        assertEquals(0, credentials.refreshes)
    }

    // A DELETE keeps its body on a 307, so a revocation that followed a redirect would hand the
    // token to wherever the Location points. HEY never redirects it: a 3xx is a failure, no hop.
    @Test
    fun aRedirectedRevocationIsAnErrorAndItsTokenGoesNowhereElse() = runTest {
        val hey = mockHey(
            status(307, headers = mapOf(HttpHeaders.Location to "https://elsewhere.example/oauth/tokens")),
            ok("{}"),
        )
        val heard = mutableListOf<Int>()
        val hooks = object : HeyHooks {
            override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
                heard += result.statusCode
            }
        }
        val error = assertFailsWith<HeyException> { hey.client { this.hooks = hooks }.revokeToken("the-app", "refresh-to-revoke") }
        assertEquals("api_error", error.code)
        assertEquals(307, error.httpStatus)
        assertEquals(1, hey.requests.size)
        assertEquals(listOf(307), heard, "the hooks hear the redirect that was received")
    }

    // A refused redirect is the answer whatever its body does: one lost mid-read must not turn the
    // 3xx into a network failure, which the retry loop would answer by revoking again, and the
    // hooks would hear as no response at all.
    @OptIn(DelicateCoroutinesApi::class)
    @Test
    fun aRedirectWhoseBodyBreaksMidReadIsStillTheRefusedAnswerSentOnce() = runTest {
        var requests = 0
        val engine = MockEngine {
            requests += 1
            val body = GlobalScope.writer(Dispatchers.Default) {
                channel.writeFully("<html><body>Moved".encodeToByteArray())
                channel.flush()
                delay(200.milliseconds)
                throw IOException("connection reset by peer")
            }.channel
            respond(body, HttpStatusCode.TemporaryRedirect, headersOf(HttpHeaders.Location, "https://elsewhere.example/oauth/tokens"))
        }
        val heard = mutableListOf<Int>()
        val hooks = object : HeyHooks {
            override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
                heard += result.statusCode
            }
        }
        val client = HeyClient {
            accessToken("the-session")
            this.engine = engine
            this.hooks = hooks
            timeout = Duration.INFINITE
            maxRetries = 2
            baseRetryDelay = 1.milliseconds
            maxRetryJitter = Duration.ZERO
        }
        val error = assertFailsWith<HeyException> { client.revokeToken("the-app", "refresh-to-revoke") }
        assertEquals("api_error", error.code)
        assertEquals(307, error.httpStatus)
        assertEquals(1, requests, "a refused redirect is not answered by revoking again")
        assertEquals(listOf(307), heard)
    }

    @Test
    fun aRevocationIsHeardAsTheOAuthWriteItIs() = runTest {
        val hey = mockHey(ok("{}"))
        val log = OperationLog()
        hey.client { hooks = log }.revokeToken("the-app", "refresh-to-revoke")
        assertEquals(listOf("OAuth.RevokeToken:token:true:null"), log.started)
    }
}
