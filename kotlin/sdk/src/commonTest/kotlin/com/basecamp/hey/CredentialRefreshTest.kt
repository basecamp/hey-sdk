package com.basecamp.hey

import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
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
     * credentials during the same outage.
     */
    @Test
    fun aFailedRefreshIsSharedByEveryRequestSignedWithTheCredentialsItWasFor() = runTest {
        for (throwing in listOf(false, true)) {
            val outage = IllegalStateException("issuer down")
            var refreshes = 0
            val credentials = object : TokenProvider {
                override suspend fun accessToken(): String = "stale"
                override suspend fun refresh(): Boolean {
                    refreshes += 1
                    if (throwing) throw outage
                    return false
                }
            }
            // Neither 401 is answered until both requests are out, so both were signed
            // with the credentials the one refresh fails to renew.
            val bothOut = CompletableDeferred<Unit>()
            var arrived = 0
            val engine = MockEngine {
                arrived += 1
                if (arrived == 2) bothOut.complete(Unit)
                bothOut.await()
                respond("", HttpStatusCode.Unauthorized)
            }
            val client = HeyClient {
                accessToken(credentials)
                this.engine = engine
                timeout = Duration.INFINITE
            }
            val a = async { runCatching { client.boxes.list() } }
            val b = async { runCatching { client.boxes.list() } }
            val outcomes = listOf(a.await(), b.await())
            assertEquals(1, refreshes, "one refresh for the one set of credentials, throwing=$throwing")
            assertEquals(2, arrived, "and no resend, throwing=$throwing")
            for (outcome in outcomes) {
                val error = outcome.exceptionOrNull()
                // A joined await hands back a copy of what was thrown, so it is matched by kind and message.
                if (throwing) assertEquals("issuer down", assertIs<IllegalStateException>(error, "what the refresh threw is what each request gets").message) else assertIs<HeyException.Auth>(error)
            }

            // A request signed after the failure earns a refresh of its own: its 401 is news.
            var again = 0
            val late = MockEngine { respond("", HttpStatusCode.Unauthorized) }
            val fresh = HeyClient {
                accessToken(object : TokenProvider {
                    override suspend fun accessToken(): String = "stale"
                    override suspend fun refresh(): Boolean { again += 1; return false }
                })
                this.engine = late
                timeout = Duration.INFINITE
            }
            assertFailsWith<HeyException.Auth> { fresh.boxes.list() }
            assertFailsWith<HeyException.Auth> { fresh.boxes.list() }
            assertEquals(2, again, "each 401 on credentials no refresh has failed since they were signed is refreshed")
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
        val hey = mockHey(status(401), status(401), ok("[]"), ok("[]"))
        val client = hey.client { accessToken(credentials) }
        val a = launch { client.boxes.list() }
        val b = launch { client.boxes.list() }
        runCurrent()
        assertEquals(1, calls, "the second request waits for the first to be signed")
        gate.complete(Unit)
        a.join()
        b.join()
        assertEquals(1, mostInside, "one request is signed at a time")
        assertEquals(4, hey.requests.size)
        assertEquals(listOf("Bearer stale", "Bearer stale", "Bearer refreshed", "Bearer refreshed"), hey.requests.map { it.header("Authorization") })
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
