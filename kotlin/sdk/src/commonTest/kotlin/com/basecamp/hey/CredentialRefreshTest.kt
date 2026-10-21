package com.basecamp.hey

import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlin.time.Duration
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.yield
import kotlin.concurrent.Volatile
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs
import kotlin.test.assertNull

class CredentialRefreshTest {
    private class Credentials(private val refreshable: Boolean) : TokenProvider {
        var token = "stale"
        var refreshes = 0

        override suspend fun accessToken(): String = token

        override suspend fun refresh(): Boolean {
            refreshes += 1
            if (!refreshable) return false
            token = "refreshed"
            return true
        }
    }

    @Test
    fun a401IsAnsweredByARefreshAndOneResend() = runTest {
        val hey = mockHey(status(401), ok("[]"))
        val credentials = Credentials(refreshable = true)
        hey.client { accessToken(credentials) }.boxes.list()
        assertEquals(2, hey.requests.size)
        assertEquals("Bearer stale", hey.requests[0].header("Authorization"))
        assertEquals("Bearer refreshed", hey.requests[1].header("Authorization"))
        assertEquals(1, credentials.refreshes)
    }

    @Test
    fun aMutationIsResentAfterARefreshToo() = runTest {
        val hey = mockHey(status(401), ok("""{"id":1}"""))
        val credentials = Credentials(refreshable = true)
        hey.client { accessToken(credentials) }.contacts.create(
            CreateContactRequestContent(contact = ContactPayload(name = "Jane", emailAddress = SensitiveString("jane@example.com"))),
        )
        assertEquals(2, hey.requests.size)
        assertEquals("/contacts.json", hey.requests[1].path)
        assertEquals(hey.requests[0].body, hey.requests[1].body)
    }

    @Test
    fun a401ThatOutlivesTheRefreshIsSurfacedAfterTheOneResend() = runTest {
        val hey = mockHey(status(401), status(401))
        val credentials = Credentials(refreshable = true)
        val error = assertFailsWith<HeyException.Auth> { hey.client { accessToken(credentials) }.boxes.list() }
        assertEquals(2, hey.requests.size)
        assertEquals(401, error.httpStatus)
        assertEquals(1, credentials.refreshes)
    }

    // A DPoP-bound token signs each request with a proof of its own (RFC 9449); a proof whose
    // time or jti the server refused costs nothing of the token.
    private class Proofs : AuthStrategy {
        var proofs = 0
        var refreshes = 0

        override suspend fun authenticate(request: HttpRequestBuilder) {
            proofs += 1
            request.header(HttpHeaders.Authorization, "DPoP bound")
            request.header("DPoP", "proof-$proofs")
        }

        override suspend fun refresh(): Boolean {
            refreshes += 1
            return true
        }
    }

    private fun AuthStrategy.client(hey: MockHey): HeyClient = HeyClient {
        auth(this@client)
        engine = hey.engine
        timeout = Duration.INFINITE
    }

    private val proofRefused = mapOf(HttpHeaders.WWWAuthenticate to "DPoP error=\"invalid_dpop_proof\", algs=\"ES256\"")

    // RFC 9449, 7.1: invalid_dpop_proof refuses the proof, not the token, so the request goes
    // again with a fresh proof and the token is not refreshed.
    @Test
    fun a401RefusingTheProofIsResentOnceWithAFreshProofAndNoRefresh() = runTest {
        val hey = mockHey(status(401, headers = proofRefused), ok("[]"))
        val proofs = Proofs()
        proofs.client(hey).boxes.list()
        assertEquals(listOf("proof-1", "proof-2"), hey.requests.map { it.header("DPoP") })
        assertEquals(0, proofs.refreshes)
    }

    @Test
    fun aProofRefusedAgainAfterItsResendIsSurfacedWithNoRefresh() = runTest {
        val hey = mockHey(status(401, headers = proofRefused), status(401, headers = proofRefused))
        val proofs = Proofs()
        val error = assertFailsWith<HeyException.Auth> { proofs.client(hey).boxes.list() }
        assertEquals(2, hey.requests.size)
        assertEquals(401, error.httpStatus)
        assertEquals(0, proofs.refreshes)
    }

    // A mutation that went through and was redirected is never sent again for its hop's refused
    // proof: the 401 is the caller's, and the contact is made once.
    @Test
    fun aRedirectedMutationWhoseHopHadItsProofRefusedIsNotSentAgain() = runTest {
        val hey =
            mockHey(
                status(303, headers = mapOf(HttpHeaders.Location to "/contacts/1.json")),
                status(401, headers = proofRefused),
                ok("""{"id":1}"""),
            )
        val proofs = Proofs()
        assertFailsWith<HeyException.Auth> {
            proofs.client(hey).contacts.create(
                CreateContactRequestContent(contact = ContactPayload(name = "Jane", emailAddress = SensitiveString("jane@example.com"))),
            )
        }
        assertEquals(listOf("POST", "GET"), hey.requests.map { it.method })
        assertEquals(0, proofs.refreshes)
    }

    // A write refused on its own first send was not made: it goes again with a fresh proof.
    @Test
    fun aMutationWhoseOwnProofWasRefusedIsResentWithAFreshProof() = runTest {
        val hey = mockHey(status(401, headers = proofRefused), ok("""{"id":1}"""))
        val proofs = Proofs()
        proofs.client(hey).contacts.create(
            CreateContactRequestContent(contact = ContactPayload(name = "Jane", emailAddress = SensitiveString("jane@example.com"))),
        )
        assertEquals(listOf("POST", "POST"), hey.requests.map { it.method })
        assertEquals(listOf("proof-1", "proof-2"), hey.requests.map { it.header("DPoP") })
        assertEquals(0, proofs.refreshes)
    }

    // A read whose redirect's hop had its proof refused is read again, from the URL asked for.
    @Test
    fun aRedirectedReadWhoseHopHadItsProofRefusedIsResent() = runTest {
        val hey =
            mockHey(
                status(302, headers = mapOf(HttpHeaders.Location to "/boxes/again.json")),
                status(401, headers = proofRefused),
                status(302, headers = mapOf(HttpHeaders.Location to "/boxes/again.json")),
                ok("[]"),
            )
        val proofs = Proofs()
        proofs.client(hey).boxes.list()
        assertEquals(listOf("/boxes.json", "/boxes/again.json", "/boxes.json", "/boxes/again.json"), hey.requests.map { it.path })
        assertEquals(0, proofs.refreshes)
    }

    // A server that takes Bearer and DPoP may challenge for each in a header of its own; the
    // proof's refusal is heard in whichever one carries it.
    @Test
    fun aProofRefusedInASecondChallengeHeaderIsResentWithNoRefresh() = runTest {
        var answered = 0
        val engine = MockEngine {
            answered += 1
            if (answered == 1) {
                respond("", HttpStatusCode.Unauthorized, headersOf(HttpHeaders.WWWAuthenticate, listOf("Bearer realm=\"hey\"", "DPoP error=\"invalid_dpop_proof\"")))
            } else {
                respond("[]", HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
            }
        }
        val proofs = Proofs()
        HeyClient {
            auth(proofs)
            this.engine = engine
            timeout = Duration.INFINITE
        }.boxes.list()
        assertEquals(2, answered)
        assertEquals(2, proofs.proofs)
        assertEquals(0, proofs.refreshes)
    }

    // A strategy that signs DPoP proofs, each naming the nonce the client hands it for the
    // request's server (RFC 9449, 9).
    private class Nonces : AuthStrategy {
        var refreshes = 0

        override suspend fun authenticate(request: HttpRequestBuilder) {
            request.header(HttpHeaders.Authorization, "DPoP bound")
            request.header("DPoP", "nonce=${request.dpopNonce}")
        }

        override suspend fun refresh(): Boolean {
            refreshes += 1
            return true
        }
    }

    private val nonceAsked =
        mapOf(HttpHeaders.WWWAuthenticate to "DPoP error=\"use_dpop_nonce\"", "DPoP-Nonce" to "n1")

    // RFC 9449, 9: use_dpop_nonce refuses no token; the request goes again with the nonce the
    // server handed out beside it, and the token is not refreshed.
    @Test
    fun a401AskingForANonceIsResentOnceWithTheServersNonceAndNoRefresh() = runTest {
        val hey = mockHey(status(401, headers = nonceAsked), ok("[]"))
        val nonces = Nonces()
        nonces.client(hey).boxes.list()
        assertEquals(listOf("nonce=null", "nonce=n1"), hey.requests.map { it.header("DPoP") })
        assertEquals(0, nonces.refreshes)
    }

    // RFC 9449, 8.2: a server hands out its next nonce with any answer.
    @Test
    fun aNonceAnyAnswerHandsOutSignsTheNextRequestToItsServer() = runTest {
        val hey = mockHey(ok("[]", mapOf("DPoP-Nonce" to "n2")), ok("[]"))
        val client = Nonces().client(hey)
        client.boxes.list()
        client.boxes.list()
        assertEquals(listOf("nonce=null", "nonce=n2"), hey.requests.map { it.header("DPoP") })
    }

    // A same-origin redirect's nonce signs the hop it leads to, as any answer's does.
    @Test
    fun aNonceASameOriginRedirectHandsOutSignsTheHopItLeadsTo() = runTest {
        val hey =
            mockHey(
                status(302, headers = mapOf(HttpHeaders.Location to "/boxes/again.json", "DPoP-Nonce" to "n3")),
                ok("[]"),
            )
        Nonces().client(hey).boxes.list()
        assertEquals(listOf("nonce=null", "nonce=n3"), hey.requests.map { it.header("DPoP") })
    }

    @Test
    fun aChallengesErrorIsReadQuotedOrBareAndNeverFromItsDescription() {
        assertEquals(listOf("invalid_dpop_proof"), challengeErrors("""DPoP error="invalid_dpop_proof", algs="ES256""""))
        assertEquals(listOf("use_dpop_nonce"), challengeErrors("""DPoP realm="hey", ERROR=use_dpop_nonce"""))
        assertEquals(listOf("invalid_token"), challengeErrors("""Bearer error_description="no error here", error="invalid_token""""))
        assertEquals(emptyList(), challengeErrors("""Bearer error_description="nothing named""""))
        assertEquals(emptyList(), challengeErrors("Bearer"))
        assertEquals(emptyList(), challengeErrors(null))
    }

    // A field may list several challenges (RFC 9110, 11.6.1), and a quoted value is read whole:
    // text inside it that looks like a parameter is not one.
    @Test
    fun everyChallengeInAFieldIsReadAndNoParameterIsFoundInsideAQuotedValue() {
        assertEquals(
            listOf("invalid_token", "use_dpop_nonce"),
            challengeErrors("""Bearer error="invalid_token", DPoP error="use_dpop_nonce", algs="ES256""""),
        )
        assertEquals(
            listOf("invalid_token"),
            challengeErrors("""Bearer error_description="prior error=invalid_dpop_proof, retry later", error="invalid_token""""),
        )
        assertEquals(listOf("say \"no\""), challengeErrors("""Bearer error="say \"no\"""""))
    }

    // A field listing a Bearer and a DPoP challenge together is heard for the DPoP one: the proof
    // is what the server refused, and the token is not refreshed.
    @Test
    fun aNonceAskedInAFieldThatAlsoChallengesBearerIsResentWithNoRefresh() = runTest {
        val combined = mapOf(
            HttpHeaders.WWWAuthenticate to "Bearer error=\"invalid_token\", DPoP error=\"use_dpop_nonce\"",
            "DPoP-Nonce" to "n4",
        )
        val hey = mockHey(status(401, headers = combined), ok("[]"))
        val nonces = Nonces()
        nonces.client(hey).boxes.list()
        assertEquals(listOf("nonce=null", "nonce=n4"), hey.requests.map { it.header("DPoP") })
        assertEquals(0, nonces.refreshes)
    }

    // A description quoting a proof error is no proof refusal: the token's own error is heard.
    @Test
    fun aProofErrorQuotedInADescriptionIsNotTakenForARefusal() = runTest {
        val quoted = mapOf(
            HttpHeaders.WWWAuthenticate to "Bearer error_description=\"prior error=invalid_dpop_proof\", error=\"invalid_token\"",
        )
        val hey = mockHey(status(401, headers = quoted), ok("[]"))
        val proofs = Proofs()
        proofs.client(hey).boxes.list()
        assertEquals(1, proofs.refreshes)
    }

    // A nonce is its server's: one another origin handed out never goes in a proof for this one.
    @Test
    fun aNonceAnotherOriginHandedOutNeverSignsForThisOne() = runTest {
        val hey =
            mockHey(
                status(302, headers = mapOf(HttpHeaders.Location to "https://elsewhere.example/boxes.json")),
                ok("[]", mapOf("DPoP-Nonce" to "theirs")),
                ok("[]"),
            )
        val client = Nonces().client(hey)
        client.boxes.list()
        client.boxes.list()
        assertEquals("elsewhere.example", hey.requests[1].url.host)
        assertEquals("nonce=null", hey.requests[2].header("DPoP"))
    }

    @Test
    fun a401IsNotResentWhenTheCredentialsCannotBeRefreshed() = runTest {
        val hey = mockHey(status(401))
        assertFailsWith<HeyException.Auth> { hey.client { accessToken(Credentials(refreshable = false)) }.boxes.list() }
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun aFormRequestIsRefreshedAndResentButNeverRetriedOtherwise() = runTest {
        val hey = mockHey(status(401), ok(""))
        val credentials = Credentials(refreshable = true)
        hey.client { accessToken(credentials) }.calendarEvents.update(99, CalendarEventUpdate(title = "After"))
        assertEquals(2, hey.requests.size)
        assertEquals("Bearer refreshed", hey.requests[1].header("Authorization"))
        assertEquals("calendar_event%5Bsummary%5D=After", hey.requests[1].body)

        val failing = mockHey(status(503))
        assertFailsWith<HeyException.Api> { failing.client().calendarEvents.update(99, CalendarEventUpdate(title = "After")) }
        assertEquals(1, failing.requests.size)
    }

    /**
     * A refresh that fails is one refresh: every request signed with the credentials it
     * could not renew gets its answer, rather than asking the issuer again for the same
     * credentials during the same outage. A request signed after the failure asks again.
     */
    @Test
    fun aFailedRefreshIsSharedByEveryRequestSignedWithTheCredentialsItWasFor() = runTest {
        for (throwing in listOf(false, true)) {
            var refreshes = 0
            val credentials = object : TokenProvider {
                override suspend fun accessToken(): String = "stale"
                override suspend fun refresh(): Boolean {
                    refreshes += 1
                    if (throwing) throw IllegalStateException("issuer down")
                    return false
                }
            }
            // Both requests go out before either is answered, so both are signed with the
            // credentials the one refresh fails to renew; the second is answered only once
            // the first has failed, so its 401 finds the failure already recorded.
            val firstFailed = CompletableDeferred<Unit>()
            val hey = Gated { arrival -> if (arrival == 2) firstFailed.await() }
            hey.bothOutFirst()
            val client = HeyClient {
                accessToken(credentials)
                engine = hey.engine
                timeout = Duration.INFINITE
                hooks = object : HeyHooks {
                    override fun onOperationEnd(info: OperationInfo, result: OperationResult) { firstFailed.complete(Unit) }
                }
            }
            val a = async { runCatching { client.boxes.list() } }
            val b = async { runCatching { client.boxes.list() } }
            val outcomes = listOf(a.await(), b.await())
            assertEquals(1, refreshes, "one refresh for the one set of credentials, throwing=$throwing")
            assertEquals(2, hey.arrivals, "and no resend, throwing=$throwing")
            for (outcome in outcomes) {
                val error = assertIs<HeyException.Auth>(outcome.exceptionOrNull(), "throwing=$throwing")
                if (throwing) {
                    assertEquals("credential refresh failed", error.message)
                    assertEquals("issuer down", error.cause?.message, "what the refresh threw reaches each request as the cause")
                }
            }

            // A request signed after the failure earns a refresh of its own: its 401 is news.
            assertFailsWith<HeyException.Auth> { client.boxes.list() }
            assertEquals(2, refreshes, "throwing=$throwing")
            assertEquals(3, hey.arrivals)
        }
    }

    /** A request whose 401 arrives while the failing refresh is still running joins it, and gets its answer too. */
    @Test
    fun aRequestJoinsTheFailingRefreshInFlight() = runTest {
        val started = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var refreshes = 0
        val credentials = object : TokenProvider {
            override suspend fun accessToken(): String = "stale"
            override suspend fun refresh(): Boolean {
                refreshes += 1
                started.complete(Unit)
                release.await()
                return false
            }
        }
        // Both go out before either is answered; the second is answered once the refresh
        // the first earned is running, and the refresh is let go only after that.
        val hey = Gated { arrival -> if (arrival == 2) started.await() }
        hey.bothOutFirst()
        val client = HeyClient {
            accessToken(credentials)
            engine = hey.engine
            timeout = Duration.INFINITE
        }
        val a = async { runCatching { client.boxes.list() } }
        val b = async { runCatching { client.boxes.list() } }
        started.await()
        release.complete(Unit)
        assertIs<HeyException.Auth>(a.await().exceptionOrNull())
        assertIs<HeyException.Auth>(b.await().exceptionOrNull())
        assertEquals(1, refreshes)
        assertEquals(2, hey.arrivals)
    }

    /**
     * A failed refresh leaves the credentials as they were, so a later refresh of them is a
     * refresh of every request's still signed under them: a 401 that arrives after it has
     * renewed them is resent with the new credentials rather than handed the old failure.
     */
    @Test
    fun aLateRequestSignedBeforeAFailedRefreshIsResentOnceALaterRefreshRenewsTheCredentials() = runTest {
        var refreshes = 0
        val credentials = object : TokenProvider {
            @Volatile var token = "stale"
            override suspend fun accessToken(): String = token
            override suspend fun refresh(): Boolean {
                refreshes += 1
                if (refreshes == 1) return false
                token = "renewed"
                return true
            }
        }
        val aIn = CompletableDeferred<Unit>()
        val bothOut = CompletableDeferred<Unit>()
        val renewed = CompletableDeferred<Unit>()
        val lock = Any()
        var arrivals = 0
        val tokens = mutableListOf<String>()
        val engine = MockEngine { request ->
            val arrival = synchronized(lock) { arrivals += 1; tokens += request.headers["Authorization"].orEmpty(); arrivals }
            when (arrival) {
                // a is in before b is sent, so the first arrival is a's whichever thread the
                // handlers run on; both are out before either is answered; and b's 401 waits
                // until c has been through a refresh that renews the credentials b was signed with.
                1 -> { aIn.complete(Unit); bothOut.await(); respond("", HttpStatusCode.Unauthorized) }
                2 -> { bothOut.complete(Unit); renewed.await(); respond("", HttpStatusCode.Unauthorized) }
                3 -> respond("", HttpStatusCode.Unauthorized)
                else -> respond("[]", HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
            }
        }
        val client = HeyClient {
            accessToken(credentials)
            this.engine = engine
            timeout = Duration.INFINITE
        }
        val a = async { runCatching { client.boxes.list() } }
        aIn.await()
        val b = async { runCatching { client.boxes.list() } }
        assertIs<HeyException.Auth>(a.await().exceptionOrNull(), "a's refresh fails")
        assertEquals(1, refreshes)
        client.boxes.list()
        assertEquals(2, refreshes, "c, signed after the failure, refreshes again and is resent")
        renewed.complete(Unit)
        assertEquals(true, b.await().isSuccess, "b's 401 is on credentials the second refresh has since renewed, so it is resent")
        assertEquals(2, refreshes, "without a refresh of its own")
        assertEquals(listOf("Bearer stale", "Bearer stale", "Bearer stale", "Bearer renewed", "Bearer renewed"), tokens)
    }

    /**
     * A provider that renews ahead of expiry hands `accessToken` a new token without being
     * asked to refresh. A 401 on the old token, arriving after the new one has signed a
     * request, is resent with the new one; refreshing would burn it. A 401 on the new token
     * itself is refreshed once, as ever.
     */
    @Test
    fun aTokenTheProviderRotatesOnItsOwnIsARenewalA401OnTheOldOneIsResentUnder() = runTest {
        var signings = 0
        var refreshes = 0
        val credentials = object : TokenProvider {
            override suspend fun accessToken(): String {
                signings += 1
                return if (signings == 1) "t0" else if (refreshes == 0) "t1" else "t2"
            }
            override suspend fun refresh(): Boolean { refreshes += 1; return true }
        }
        // The first two requests are signed t0 and t1 in turn, and neither is answered until
        // both are out; the t0 one is answered 401, the t1 one 200. The engine answers by the
        // token, not by arrival: the two may reach it in either order.
        val bothOut = CompletableDeferred<Unit>()
        val lock = Any()
        var arrivals = 0
        val tokens = mutableListOf<String>()
        val engine = MockEngine { request ->
            val token = request.headers["Authorization"].orEmpty()
            val arrival = synchronized(lock) { arrivals += 1; tokens += token; arrivals }
            if (arrival == 2) bothOut.complete(Unit)
            if (arrival <= 2) bothOut.await()
            if (token == "Bearer t0" || arrival == 4) {
                respond("", HttpStatusCode.Unauthorized)
            } else {
                respond("[]", HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
            }
        }
        val client = HeyClient {
            accessToken(credentials)
            this.engine = engine
            timeout = Duration.INFINITE
        }
        val a = async { client.boxes.list() }
        val b = async { client.boxes.list() }
        a.await()
        b.await()
        assertEquals(0, refreshes, "the 401 on t0 was answered by t1, which the provider had already handed over")
        assertEquals(listOf("Bearer t0", "Bearer t1", "Bearer t1"), tokens.take(2).sorted() + tokens.drop(2))

        client.boxes.list()
        assertEquals(1, refreshes, "a 401 on t1 itself is refreshed, once")
        assertEquals(listOf("Bearer t1", "Bearer t1", "Bearer t2"), tokens.drop(2))
    }

    /**
     * The rotation need not have signed anything yet: before the provider is asked to refresh,
     * it is asked what it would sign with now, and a token other than the rejected one is a
     * renewal already made. The request is resent with it and the refresh is not spent.
     */
    @Test
    fun aProviderAskedForItsTokenBeforeARefreshIsNotAskedToRefreshWhenItHasAlreadyRenewed() = runTest {
        var signings = 0
        var refreshes = 0
        val credentials = object : TokenProvider {
            override suspend fun accessToken(): String {
                signings += 1
                return if (signings == 1) "t0" else if (refreshes == 0) "t1" else "t2"
            }
            override suspend fun refresh(): Boolean { refreshes += 1; return true }
        }
        val hey = mockHey(status(401), ok("[]"), status(401), ok("[]"))
        val client = hey.client { accessToken(credentials) }
        client.boxes.list()
        assertEquals(0, refreshes, "the provider had renewed on its own by the time the 401 came back")
        assertEquals(listOf("Bearer t0", "Bearer t1"), hey.requests.map { it.header("Authorization") })
        client.boxes.list()
        assertEquals(1, refreshes, "a 401 on the token the provider would still sign with is refreshed")
        assertEquals(listOf("Bearer t0", "Bearer t1", "Bearer t1", "Bearer t2"), hey.requests.map { it.header("Authorization") })
    }

    /** A timeout the provider puts on its own refresh is the refresh's answer, not the client's cancellation. */
    @Test
    fun aProviderThatTimesItselfOutFailsTheRefreshRatherThanCancellingTheRequest() = runTest {
        var refreshes = 0
        val credentials = object : TokenProvider {
            override suspend fun accessToken(): String = "stale"
            override suspend fun refresh(): Boolean {
                refreshes += 1
                return withTimeout(1) { awaitCancellation() }
            }
        }
        val firstFailed = CompletableDeferred<Unit>()
        val hey = Gated { arrival -> if (arrival == 2) firstFailed.await() }
        hey.bothOutFirst()
        val client = HeyClient {
            accessToken(credentials)
            engine = hey.engine
            timeout = Duration.INFINITE
            hooks = object : HeyHooks {
                override fun onOperationEnd(info: OperationInfo, result: OperationResult) { firstFailed.complete(Unit) }
            }
        }
        val a = async { runCatching { client.boxes.list() } }
        val b = async { runCatching { client.boxes.list() } }
        for (outcome in listOf(a.await(), b.await())) {
            val error = assertIs<HeyException.Auth>(outcome.exceptionOrNull())
            assertIs<TimeoutCancellationException>(error.cause)
        }
        assertEquals(1, refreshes, "the timed-out refresh is shared like any other failure")
        assertEquals(2, hey.arrivals)
    }

    /**
     * A HEY that answers every request 401, holding each answer until [hold] lets it go;
     * handlers run on the engine's threads, so arrivals are counted under a lock. With
     * [bothOutFirst], neither of the first two requests is answered until both have arrived,
     * so both were signed before either 401 could start a refresh — the handler may run
     * inside the request's own coroutine, in which case an answer given at once would let
     * the first request refresh before the second was signed.
     */
    private class Gated(private val hold: suspend (arrival: Int) -> Unit) {
        private val lock = Any()
        private var barrier: CompletableDeferred<Unit>? = null
        var arrivals = 0
            private set

        fun bothOutFirst() {
            barrier = CompletableDeferred()
        }

        val engine = MockEngine {
            val arrival = synchronized(lock) { arrivals += 1; arrivals }
            barrier?.let { bothOut ->
                if (arrival == 2) bothOut.complete(Unit)
                if (arrival <= 2) bothOut.await()
            }
            hold(arrival)
            respond("", HttpStatusCode.Unauthorized)
        }
    }

    /** A request is signed under the refresh lock, so a refresh cannot land between the signing and the count that says which credentials went out. */
    @Test
    fun signingAndRefreshingNeverInterleave() = runTest {
        val gate = CompletableDeferred<Unit>()
        var inside = 0
        var mostInside = 0
        var calls = 0
        val credentials = object : TokenProvider {
            var token = "stale"
            override suspend fun accessToken(): String {
                calls += 1
                inside += 1
                mostInside = maxOf(mostInside, inside)
                if (calls == 1) gate.await()
                inside -= 1
                return token
            }

            override suspend fun refresh(): Boolean {
                token = "refreshed"
                return true
            }
        }
        // Neither 401 goes back until both requests are out, so both are signed with the
        // stale credentials and the one refresh answers both.
        val bothOut = CompletableDeferred<Unit>()
        val lock = Any()
        var arrivals = 0
        val tokens = mutableListOf<String>()
        val engine = MockEngine { request ->
            val arrival = synchronized(lock) { arrivals += 1; tokens += request.headers["Authorization"].orEmpty(); arrivals }
            if (arrival == 2) bothOut.complete(Unit)
            if (arrival <= 2) {
                bothOut.await()
                respond("", HttpStatusCode.Unauthorized)
            } else {
                respond("[]", HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
            }
        }
        val client = HeyClient {
            accessToken(credentials)
            this.engine = engine
            timeout = Duration.INFINITE
        }
        val a = launch { client.boxes.list() }
        val b = launch { client.boxes.list() }
        runCurrent()
        assertEquals(1, calls, "the second request waits for the first to be signed")
        gate.complete(Unit)
        a.join()
        b.join()
        assertEquals(1, mostInside, "one request is signed at a time")
        assertEquals(4, arrivals)
        assertEquals(listOf("Bearer stale", "Bearer stale", "Bearer refreshed", "Bearer refreshed"), tokens)
    }

    /**
     * A hop signed again after someone else's refresh carries the new credentials, so a 401
     * on it is about those, and earns the refresh it deserves rather than a bare resend.
     */
    @Test
    fun a401OnAHopSignedAfterARefreshIsRefreshedAgain() = runTest {
        var refreshes = 0
        val credentials = object : TokenProvider {
            var token = "t0"
            override suspend fun accessToken(): String = token
            override suspend fun refresh(): Boolean {
                refreshes += 1
                token = "t$refreshes"
                return true
            }
        }
        val firstAnswer = CompletableDeferred<Unit>()
        val firstAsked = CompletableDeferred<Unit>()
        val seen = mutableMapOf<String, Int>()
        val tokens = mutableListOf<String>()
        val recording = Any()
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            // Handlers run on the engine's own threads: the record is taken under a lock, and
            // b is not sent until a's first request has been seen, so the order is the test's.
            val visit = synchronized(recording) {
                val visit = (seen[path] ?: 0) + 1
                seen[path] = visit
                tokens += "$path ${request.headers["Authorization"]}"
                visit
            }
            when {
                path == "/a.json" -> {
                    firstAsked.complete(Unit)
                    firstAnswer.await()
                    respond("", HttpStatusCode.Found, headersOf("Location", "/a2.json"))
                }
                visit == 1 -> respond("", HttpStatusCode.Unauthorized)
                else -> respond("[]", HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
            }
        }
        val client = HeyClient {
            accessToken(credentials)
            this.engine = engine
            timeout = Duration.INFINITE
        }
        val a = launch { client.execute(client.request(Method.GET, "/a")) }
        firstAsked.await()
        client.execute(client.request(Method.GET, "/b"))
        assertEquals(1, refreshes, "b's 401 refreshed while a's first answer was still to come")
        firstAnswer.complete(Unit)
        a.join()
        assertEquals(2, refreshes, "a's hop went out with the refreshed credentials, and their 401 is refreshed again")
        assertEquals(
            listOf("/a.json Bearer t0", "/b.json Bearer t0", "/b.json Bearer t1", "/a2.json Bearer t1", "/a.json Bearer t2", "/a2.json Bearer t2"),
            tokens,
            "the resend starts the operation over from the URL asked for, through the redirect again",
        )
    }

    /**
     * A refresh belongs to the client: the request that earned it being cancelled leaves it
     * to finish, and the next stale request is signed with what it renewed rather than
     * starting one of its own.
     */
    @Test
    fun aCancelledRequestDoesNotCancelTheRefreshItStarted() = runTest {
        val gate = CompletableDeferred<Unit>()
        val credentials = object : TokenProvider {
            @Volatile var token = "stale"
            @Volatile var refreshes = 0
            override suspend fun accessToken(): String = token
            override suspend fun refresh(): Boolean {
                refreshes += 1
                gate.await()
                token = "renewed"
                return true
            }
        }
        val hey = mockHey(status(401), ok("[]"))
        val client = hey.client { accessToken(credentials) }
        val first = launch { client.boxes.list() }
        while (credentials.refreshes == 0) yield()
        first.cancelAndJoin()
        assertEquals(1, hey.requests.size)
        val second = launch { client.boxes.list() }
        runCurrent()
        assertEquals(1, hey.requests.size, "the second request waits to be signed until the refresh is done")
        gate.complete(Unit)
        second.join()
        assertEquals(1, credentials.refreshes, "the refresh the cancelled request started is the only one")
        assertEquals("Bearer renewed", hey.requests[1].header("Authorization"))
        assertEquals(2, hey.requests.size)
    }

    @Test
    fun aCredentialThatIsNotAHeaderValueIsRefusedWithoutBeingQuoted() = runTest {
        val seen = mutableListOf<Throwable>()
        val recording = object : HeyHooks {
            override fun onOperationEnd(info: OperationInfo, result: OperationResult) { result.error?.let { seen += it } }
            override fun onRequestEnd(info: RequestInfo, result: RequestResult) { result.error?.let { seen += it } }
        }
        val hey = mockHey(ok("[]"))
        val bearer = hey.client {
            accessToken("secret\u000Btoken")
            hooks = recording
        }
        val refusedToken = assertFailsWith<HeyException.Auth> { bearer.boxes.list() }
        assertEquals(0, hey.requests.size)

        val cookie = HeyClient {
            auth(object : AuthStrategy {
                override suspend fun authenticate(request: HttpRequestBuilder) { request.header(HttpHeaders.Cookie, "session=\u0001abc") }
            })
            engine = hey.engine
            hooks = recording
        }
        val refusedCookie = assertFailsWith<HeyException.Auth> { cookie.boxes.list() }
        for (error in seen + refusedToken + refusedCookie) {
            val rendered = error.toString() + (error as? HeyException)?.hint + error.stackTraceToString()
            assertEquals(false, rendered.contains("secret") || rendered.contains("abc"), rendered)
        }
    }
}
