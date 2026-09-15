package com.basecamp.hey

import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.CompletableDeferred
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
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

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
        val seen = mutableMapOf<String, Int>()
        val tokens = mutableListOf<String>()
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            val visit = (seen[path] ?: 0) + 1
            seen[path] = visit
            tokens += "$path ${request.headers["Authorization"]}"
            when {
                path == "/a.json" -> {
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
        runCurrent()
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
}
