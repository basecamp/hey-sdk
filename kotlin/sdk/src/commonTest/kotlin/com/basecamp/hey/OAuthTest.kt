package com.basecamp.hey

import com.basecamp.hey.generated.boxes
import com.basecamp.hey.generated.models.CreateTokenRequestContent
import com.basecamp.hey.generated.models.RevokeTokenRequestContent
import com.basecamp.hey.generated.oauth
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

/**
 * HEY's token endpoint, as the model describes it: the generated `OAuth` service, sent unsigned
 * and following no redirect because its routes say so, and a client built with [NoCredentials]
 * for an app that holds no token yet.
 */
class OAuthTest {
    /** A provider that counts what it is asked, so a test can say it was never consulted. */
    private class Counted : TokenProvider {
        var asked = 0
        var refreshes = 0

        override suspend fun accessToken(): String {
            asked += 1
            return "the-session"
        }

        override suspend fun refresh(): Boolean {
            refreshes += 1
            return true
        }
    }

    /** A DPoP strategy of the caller's own: what a client holding a bound token signs with. */
    private class Dpop : AuthStrategy {
        var signed = 0

        override suspend fun authenticate(request: HttpRequestBuilder) {
            signed += 1
            request.header(HttpHeaders.Authorization, "DPoP the-session")
            request.header("DPoP", "a-proof")
        }
    }

    private val code = CreateTokenRequestContent(
        grantType = "authorization_code",
        clientId = "the-app",
        installId = "the-install",
        code = SensitiveString("the-code"),
        redirectUri = "heyapp://oauth/callback",
        codeVerifier = SensitiveString("the-verifier"),
    )

    private val revocation = RevokeTokenRequestContent(clientId = "the-app", token = SensitiveString("refresh-to-revoke"))

    private fun json(body: String): JsonObject = Json.parseToJsonElement(body) as JsonObject

    @Test
    fun createTokenPostsTheGrantAsJsonToTheTokenEndpoint() = runTest {
        val hey = mockHey(ok("""{"access_token":"new-access","token_type":"bearer","expires_in":1209600,"refresh_token":"new-refresh"}"""))
        val token = hey.client().oauth.createToken(code)
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("/oauth/tokens.json", request.path, "the path's .json is the model's, and is not doubled")
        assertEquals("", request.url.encodedQuery)
        assertEquals("application/json", request.header(HttpHeaders.ContentType)?.substringBefore(';'))
        assertEquals(
            json("""{"grant_type":"authorization_code","client_id":"the-app","install_id":"the-install","code":"the-code","redirect_uri":"heyapp://oauth/callback","code_verifier":"the-verifier"}"""),
            json(request.body),
        )
        assertEquals("new-access", token.accessToken.expose())
        assertEquals("bearer", token.tokenType)
        assertEquals(1209600L, token.expiresIn)
        assertEquals("new-refresh", token.refreshToken?.expose())
        assertEquals("[REDACTED]", token.accessToken.toString(), "a token the grant answers prints as nothing")
    }

    @Test
    fun revokeTokenDeletesWithTheTokenInAJsonBodyNeverTheQuery() = runTest {
        val hey = mockHey(ok("{}"))
        hey.client().oauth.revokeToken(revocation)
        val request = hey.requests.single()
        assertEquals("DELETE", request.method)
        assertEquals("/oauth/tokens.json", request.path)
        assertEquals("", request.url.encodedQuery)
        assertEquals(json("""{"client_id":"the-app","token":"refresh-to-revoke"}"""), json(request.body))
    }

    // The route says unsigned, so the client's own credentials stay off it whatever they are:
    // the provider is not even asked for its token.
    @Test
    fun anUnsignedRouteCarriesNoAuthorizationAndNeverAsksTheProvider() = runTest {
        val hey = mockHey(ok("""{"access_token":"a","token_type":"bearer"}"""))
        val credentials = Counted()
        hey.client { accessToken(credentials) }.oauth.createToken(code)
        assertNull(hey.requests.single().header(HttpHeaders.Authorization))
        assertEquals(0, credentials.asked, "the token provider is not consulted for an unsigned route")
    }

    @Test
    fun anUnsignedRouteCarriesNoDpopProofAndNeverAsksTheStrategy() = runTest {
        val hey = mockHey(ok("""{"access_token":"a","token_type":"bearer"}"""), ok("{}"))
        val strategy = Dpop()
        val client = HeyClient {
            auth(strategy)
            engine = hey.engine
            timeout = Duration.INFINITE
        }
        client.oauth.createToken(code)
        client.oauth.revokeToken(revocation)
        for (request in hey.requests) {
            assertNull(request.header(HttpHeaders.Authorization))
            assertNull(request.header("DPoP"))
        }
        assertEquals(0, strategy.signed, "the auth strategy is not consulted for an unsigned route")
    }

    // A 401 from the token endpoint judges the grant in the body, not credentials the client
    // could renew: refreshing would spend the client's refresh token on nothing.
    @Test
    fun a401OnAnUnsignedRouteIsTheAnswerSentOnceWithNoRefresh() = runTest {
        val hey = mockHey(
            status(401, """{"error":"invalid_grant","error_description":"Invalid refresh token."}"""),
            ok("""{"access_token":"never","token_type":"bearer"}"""),
        )
        val credentials = Counted()
        val error = assertFailsWith<HeyException.Auth> {
            hey.client { accessToken(credentials) }.oauth.createToken(
                CreateTokenRequestContent(grantType = "refresh_token", clientId = "the-app", installId = "the-install", refreshToken = SensitiveString("dead")),
            )
        }
        assertEquals("auth_required", error.code)
        assertEquals(401, error.httpStatus)
        assertEquals(1, hey.requests.size)
        assertEquals(0, credentials.refreshes)
    }

    // A 307 keeps the body, so a grant that followed one would hand its code and verifier to
    // wherever the Location points.
    @Test
    fun aRedirectOnANoRedirectRouteIsAnErrorWithItsStatusAndNothingReachesTheLocation() = runTest {
        for (code3xx in listOf(301, 302, 303, 307, 308)) {
            val hey = mockHey(
                status(code3xx, headers = mapOf(HttpHeaders.Location to "/elsewhere/oauth/tokens.json")),
                ok("""{"access_token":"elsewhere","token_type":"bearer"}"""),
            )
            val error = assertFailsWith<HeyException> { hey.client { maxRetries = 3 }.oauth.createToken(code) }
            assertEquals(code3xx, error.httpStatus)
            assertEquals(listOf("/oauth/tokens.json"), hey.requests.map { it.path }, "a $code3xx is sent once and not followed")
        }
    }

    // The session the token names is the identity's, not an account's.
    @Test
    fun anAccountScopedClientSendsTheTokenEndpointUnscoped() = runTest {
        val hey = mockHey(ok(IDENTITY), ok("""{"access_token":"a","token_type":"bearer"}"""))
        hey.client().forAccount(42).oauth.createToken(code)
        assertEquals("/oauth/tokens.json", hey.requests[1].path)
        assertNull(hey.requests[1].query("filtered_account_id"))
    }

    // The hooks hear where an unsigned route goes: the URL is HEY's own and carries nothing in
    // the open, unlike a storage URL that signs itself.
    @Test
    fun theHooksHearTheTokenEndpointsPath() = runTest {
        val hey = mockHey(ok("{}"))
        val urls = mutableListOf<String>()
        val hooks = object : HeyHooks {
            override fun onRequestStart(info: RequestInfo) {
                urls += info.url
            }
        }
        hey.client { this.hooks = hooks }.oauth.revokeToken(revocation)
        assertEquals(listOf("https://app.hey.com/oauth/tokens.json"), urls)
    }

    // The flags belong to the route, not to OAuth: any route the model marks is sent the same
    // way, and one it does not mark is signed and follows its redirects as before.
    @Test
    fun theFlagsComeFromTheRouteWhateverItIs() = runTest {
        val plain = Route(
            id = "GetWidget", service = "Widgets", method = Method.GET, path = "/widgets.json", pattern = "/widgets",
            resource = "Widgets", resourceType = "widget", params = emptyList(), idempotent = true, readonly = true,
            html = false, emptyOn = emptyList(), pagination = Pagination.NONE, pageParameter = null,
            retry = RetryPolicy(max = 1, baseDelayMs = 1, retryOn = emptyList()),
        )
        val marked = plain.copy(unsigned = true, noRedirect = true)

        val signed = mockHey(status(302, headers = mapOf(HttpHeaders.Location to "/widgets/moved.json")), ok("{}"))
        val client = signed.client()
        client.sendUnit(client.operation(plain, emptyList()))
        assertEquals(listOf("/widgets.json", "/widgets/moved.json"), signed.requests.map { it.path }, "an unmarked route follows its redirect")
        assertEquals("Bearer test-token", signed.requests[0].header(HttpHeaders.Authorization), "and is signed")

        val unsigned = mockHey(status(302, headers = mapOf(HttpHeaders.Location to "/widgets/moved.json")), ok("{}"))
        val other = unsigned.client()
        val error = assertFailsWith<HeyException> { other.sendUnit(other.operation(marked, emptyList())) }
        assertEquals(302, error.httpStatus)
        assertEquals(listOf("/widgets.json"), unsigned.requests.map { it.path })
        assertNull(unsigned.requests[0].header(HttpHeaders.Authorization))
    }

    // An app that holds no token yet builds its client for the token endpoint alone.
    @Test
    fun aClientWithNoCredentialsSendsTheUnsignedOperations() = runTest {
        val hey = mockHey(ok("""{"access_token":"a","token_type":"bearer"}"""), ok("{}"))
        val client = HeyClient {
            auth(NoCredentials)
            engine = hey.engine
            timeout = Duration.INFINITE
        }
        assertEquals("a", client.oauth.createToken(code).accessToken.expose())
        client.oauth.revokeToken(revocation)
        assertEquals(listOf("POST", "DELETE"), hey.requests.map { it.method })
        for (request in hey.requests) assertNull(request.header(HttpHeaders.Authorization))
    }

    @Test
    fun aClientWithNoCredentialsRefusesASignedOperationWithoutSendingIt() = runTest {
        val hey = mockHey(ok("[]"))
        val log = OperationLog()
        val client = HeyClient {
            auth(NoCredentials)
            engine = hey.engine
            timeout = Duration.INFINITE
            hooks = log
        }
        val error = assertFailsWith<HeyException.Auth> { client.boxes.list() }
        assertEquals("auth_required", error.code)
        assertEquals(HeyException.CODE_AUTH, error.code)
        assertEquals(0, hey.requests.size, "nothing is sent")
        assertEquals(listOf("Boxes.ListBoxes:auth_required"), log.ended)
    }

    // NoCredentials is the deliberate opt-in; a client with nothing configured is still a
    // forgotten token, caught when it is built.
    @Test
    fun aClientWithNothingConfiguredIsStillRefusedWhenBuilt() {
        val error = assertFailsWith<HeyException.Usage> { HeyClient { } }
        assertEquals("usage", error.code)
    }

    /**
     * A refresher built on the client it refreshes: its refresh trades the refresh token at the
     * token endpoint through that same client. The refresh runs under the client's signing lock,
     * so the unsigned grant must not wait on it, or the refresh waits on itself forever.
     */
    private class SameClientRefresher : TokenProvider {
        lateinit var client: HeyClient
        var token = "stale-access"
        var refreshes = 0

        override suspend fun accessToken(): String = token

        override suspend fun refresh(): Boolean {
            refreshes += 1
            val granted = client.oauth.createToken(
                CreateTokenRequestContent(grantType = "refresh_token", clientId = "the-app", installId = "the-install", refreshToken = SensitiveString("the-refresh")),
            )
            token = granted.accessToken.expose()
            return true
        }
    }

    @Test
    fun aRefreshThatCallsTheTokenEndpointOnTheSameClientDoesNotWaitOnItself() = runTest(timeout = 10.seconds) {
        val hey = mockHey(status(401), ok("""{"access_token":"fresh-access","token_type":"bearer"}"""), ok("[]"))
        val refresher = SameClientRefresher()
        val client = hey.client { accessToken(refresher) }
        refresher.client = client
        client.boxes.list()
        assertEquals(listOf("/boxes.json", "/oauth/tokens.json", "/boxes.json"), hey.requests.map { it.path })
        assertEquals("Bearer stale-access", hey.requests[0].header(HttpHeaders.Authorization))
        assertNull(hey.requests[1].header(HttpHeaders.Authorization), "the grant goes unsigned")
        assertEquals("Bearer fresh-access", hey.requests[2].header(HttpHeaders.Authorization), "and the operation is resent with what it granted")
        assertEquals(1, refresher.refreshes)
    }

    /** The same refresher as a DPoP strategy, which names the nonce each origin handed out in its proofs. */
    private class SameClientDpopRefresher : AuthStrategy {
        lateinit var client: HeyClient
        var token = "stale-access"
        val nonces = mutableListOf<String?>()

        override suspend fun authenticate(request: HttpRequestBuilder) {
            nonces += request.dpopNonce
            request.header(HttpHeaders.Authorization, "DPoP $token")
            request.header("DPoP", "proof-for-${request.dpopNonce}")
        }

        override suspend fun refresh(): Boolean {
            val granted = client.oauth.createToken(
                CreateTokenRequestContent(grantType = "refresh_token", clientId = "the-app", installId = "the-install", refreshToken = SensitiveString("the-refresh")),
            )
            token = granted.accessToken.expose()
            return true
        }
    }

    // The token endpoint hands out a nonce of its own while the refresh holds the signing lock;
    // hearing it must not wait on that lock either, and the resend names it.
    @Test
    fun aDpopRefreshThroughTheSameClientHearsTheGrantsNonceWithoutWaitingOnItself() = runTest(timeout = 10.seconds) {
        val hey = mockHey(
            status(401, headers = mapOf("DPoP-Nonce" to "first")),
            ok("""{"access_token":"fresh-access","token_type":"bearer"}""", headers = mapOf("DPoP-Nonce" to "from-the-grant")),
            ok("[]"),
        )
        val strategy = SameClientDpopRefresher()
        val client = HeyClient {
            auth(strategy)
            engine = hey.engine
            timeout = Duration.INFINITE
        }
        strategy.client = client
        client.boxes.list()
        assertEquals(listOf("/boxes.json", "/oauth/tokens.json", "/boxes.json"), hey.requests.map { it.path })
        assertNull(hey.requests[1].header("DPoP"), "the grant carries no proof")
        assertEquals("DPoP fresh-access", hey.requests[2].header(HttpHeaders.Authorization))
        assertEquals(listOf(null, "from-the-grant"), strategy.nonces, "the resend names the nonce the grant's answer handed out")
    }

    // HEY's token endpoint refuses with RFC 6749's {"error", "error_description"}: the
    // description is HEY's sentence for a person, and the caller's message carries it, while
    // the code, the status and whether it may be retried stay what the status says.
    @Test
    fun aRefusedGrantsDescriptionIsInTheMessage() = runTest {
        val hey = mockHey(status(400, """{"error":"invalid_request","error_description":"install_id doesn't identify this install. Upgrade your client."}"""))
        val error = assertFailsWith<HeyException.Api> { hey.client().oauth.createToken(code) }
        assertContains(error.message!!, "install_id doesn't identify this install. Upgrade your client.")
        assertEquals("api_error", error.code)
        assertEquals(400, error.httpStatus)
        assertEquals(false, error.retryable)
        assertEquals("invalid_request", error.hint, "the OAuth code is the hint, where HEY's word on a failure goes")
    }

    // Shipped HEY clients sign out on invalid_grant, a dead refresh token, and not on
    // invalid_client, a misconfigured client: both are 401s, told apart by the code.
    @Test
    fun aDeadRefreshTokenIsToldApartFromAMisconfiguredClient() = runTest {
        val hey = mockHey(
            status(401, """{"error":"invalid_grant","error_description":"Invalid refresh token."}"""),
            status(401, """{"error":"invalid_client","error_description":"OAuth client is not authorized to revoke this token."}"""),
        )
        val client = hey.client()
        val dead = assertFailsWith<HeyException.Auth> {
            client.oauth.createToken(CreateTokenRequestContent(grantType = "refresh_token", clientId = "the-app", installId = "the-install", refreshToken = SensitiveString("dead")))
        }
        val misconfigured = assertFailsWith<HeyException.Auth> { client.oauth.revokeToken(revocation) }
        assertEquals("invalid_grant", dead.hint)
        assertEquals("invalid_client", misconfigured.hint)
        assertContains(dead.message!!, "Invalid refresh token.")
        assertContains(misconfigured.message!!, "OAuth client is not authorized to revoke this token.")
        assertEquals(listOf("auth_required", "auth_required"), listOf(dead.code, misconfigured.code))
    }
}
