package com.basecamp.hey

import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class AccountScopeTest {
    @Test
    fun aScopedClientChecksTheAccountAndFiltersEveryRequest() = runTest {
        val hey = mockHey(ok(IDENTITY), ok("[]", mapOf("Link" to "</boxes.json?page=2>; rel=\"next\"")), ok("[]"))
        val work = hey.client().forAccount(42)
        val page = work.boxes.list()
        work.nextPage(page)

        assertEquals("/identity.json", hey.requests[0].path)
        assertNull(hey.requests[0].query("filtered_account_id"))
        assertEquals("42", hey.requests[1].query("filtered_account_id"))
        assertEquals("42", hey.requests[2].query("filtered_account_id"), "the next page is filtered too")
        assertEquals("2", hey.requests[2].query("page"))
        assertEquals(42L, work.accountId)
    }

    @Test
    fun anAccountTheIdentityCannotReachIsRefused() = runTest {
        val hey = mockHey(ok(IDENTITY), ok(IDENTITY))
        val client = hey.client()
        assertFailsWith<HeyException.NotFound> { client.forAccount(43) }
        assertFailsWith<HeyException.NotFound> { client.forAccount(99) }
        assertFailsWith<HeyException.Usage> { client.forAccount(0) }
    }

    @Test
    fun theDefaultSenderFollowsTheScope() = runTest {
        val hey = mockHey(ok(IDENTITY), ok(IDENTITY), ok(IDENTITY))
        val client = hey.client()
        assertEquals(100L, client.defaultSenderId(), "the identity's default sender")
        assertEquals(100L, client.defaultSenderId(), "read once")
        val work = client.forAccount(42)
        assertEquals(100L, work.defaultSenderId())
        assertEquals(1000L, work.accountUserId())
        assertEquals(2, hey.requests.size)
        assertFailsWith<HeyException.Usage> { client.accountUserId() }
    }

    @Test
    fun withoutADefaultSenderTheFirstOneOrThePrimaryContactStandsIn() = runTest {
        val hey = mockHey(ok("""{"id":1,"primary_contact":{"id":9},"senders":[{"id":5},{"id":6}]}"""), ok("""{"id":1,"primary_contact":{"id":9}}"""))
        assertEquals(5L, hey.client().defaultSenderId())
        assertEquals(9L, hey.client().defaultSenderId())
    }
}
