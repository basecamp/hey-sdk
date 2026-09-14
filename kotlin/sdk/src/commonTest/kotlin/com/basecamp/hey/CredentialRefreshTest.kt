package com.basecamp.hey

import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
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
}
