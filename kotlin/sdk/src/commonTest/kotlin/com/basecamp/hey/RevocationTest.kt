package com.basecamp.hey

import io.ktor.http.HttpHeaders
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
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
        val error = assertFailsWith<HeyException> { hey.client().revokeToken("the-app", "refresh-to-revoke") }
        assertEquals("api_error", error.code)
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun aRevocationIsHeardAsTheOAuthWriteItIs() = runTest {
        val hey = mockHey(ok("{}"))
        val log = OperationLog()
        hey.client { hooks = log }.revokeToken("the-app", "refresh-to-revoke")
        assertEquals(listOf("OAuth.RevokeToken:token:true:null"), log.started)
    }
}
