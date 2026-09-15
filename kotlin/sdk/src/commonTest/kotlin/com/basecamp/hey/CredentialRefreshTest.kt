package com.basecamp.hey

import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
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
}
