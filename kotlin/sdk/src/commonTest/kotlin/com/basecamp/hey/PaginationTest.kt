package com.basecamp.hey

import kotlinx.coroutines.flow.toList
import com.basecamp.hey.generated.*
import com.basecamp.hey.generated.services.GetContactOptions
import io.ktor.http.Url
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class PaginationTest {
    @Test
    fun theNextLinkIsFoundAmongOthers() {
        assertEquals(
            "https://app.hey.com/imbox.json?page=c",
            nextLink("""<https://app.hey.com/imbox.json?page=a,b>; rel="prev", <https://app.hey.com/imbox.json?page=c>; rel="next""""),
        )
        assertEquals("/boxes.json?page=2", nextLink("""</boxes.json?page=2>; rel="next""""))
        assertEquals("/x", nextLink("""</x>; rel="next prev""""))
        assertEquals("/y", nextLink("""</x>; rel=prev, </y>; REL=NEXT"""))
        assertNull(nextLink("""</x>; rel="prev""""))
        assertNull(nextLink("garbage"))
    }

    @Test
    fun aReferenceTakesNothingOfTheRequestsQuery() {
        val base = Url("https://app.hey.com/contacts/88.json?page=older&filtered_account_id=42#x")
        assertEquals("https://app.hey.com/contacts/88.json?page=next", resolveReference(base, "/contacts/88.json?page=next").toString())
        assertEquals("https://app.hey.com/contacts/88.json", resolveReference(base, "/contacts/88.json").toString())
        assertEquals("https://files.example.com/export.json", resolveReference(base, "https://files.example.com/export.json").toString())
        assertEquals("https://app.hey.com/contacts/89.json?page=1", resolveReference(base, "89.json?page=1").toString())
    }

    @Test
    fun theNextPageDoesNotInheritTheCursorItWasAskedWith() = runTest {
        val hey = mockHey(ok("""{"id":88}""", mapOf("Link" to "</contacts/88.json?page=next>; rel=\"next\"")), ok("""{"id":88}"""))
        val client = hey.client()
        val first = client.contacts.get(88, GetContactOptions(page = "older"))
        assertEquals("next", first.nextPage)
        client.nextPage(first)
        assertEquals(listOf("next"), hey.requests[1].url.parameters.getAll("page"))
    }

    @Test
    fun aPageCarriesTheCursorAndTheTotal() = runTest {
        val hey = mockHey(
            ok("""[{"id":1,"kind":"imbox","name":"a"}]""", mapOf("Link" to "</boxes.json?page=2>; rel=\"next\"", "X-Total-Count" to "50")),
            ok("""[{"id":2,"kind":"imbox","name":"b"}]"""),
        )
        val client = hey.client()
        val first = client.boxes.list()
        assertEquals("2", first.nextPage)
        assertEquals(50L, first.totalCount)
        assertEquals("https://app.hey.com/boxes.json?page=2", first.nextUrl.toString())
        val second = client.nextPage(first)!!
        assertEquals(listOf(2L), second.value.map { it.id })
        assertNull(second.nextPage)
        assertNull(client.nextPage(second))
        assertEquals("2", hey.requests[1].query("page"))
    }

    @Test
    fun aLinkOffTheOriginIsRefused() = runTest {
        for (link in listOf("<https://evil.example.com/boxes.json?page=2>; rel=\"next\"", "<http://app.hey.com/boxes.json?page=2>; rel=\"next\"")) {
            val hey = mockHey(ok("[]", mapOf("Link" to link)))
            val client = hey.client()
            val first = client.boxes.list()
            assertFailsWith<HeyException.Usage> { client.nextPage(first) }
            assertEquals(1, hey.requests.size)
        }
    }

    @Test
    fun theNextPageIsReadUnderTheFirstPagesRetryPolicy() = runTest {
        val link = mapOf("Link" to "</boxes.json?page=2>; rel=\"next\"")

        val unnamed = mockHey(ok("[]", link), status(500), ok("[]"))
        var client = unnamed.client()
        val error = assertFailsWith<HeyException.Api> { client.nextPage(client.boxes.list()) }
        assertEquals(500, error.httpStatus)
        assertEquals(2, unnamed.requests.size, "ListBoxes' policy does not name 500, so the next page is not resent on it either")

        val exhausted = mockHey(ok("[]", link), status(503), status(503), status(503), status(503), ok("[]"))
        client = exhausted.client()
        assertFailsWith<HeyException.Api> { client.nextPage(client.boxes.list()) }
        assertEquals(4, exhausted.requests.size, "three sends for the next page, the policy's most, not the client's four")
    }

    @Test
    fun aWalkStopsAtThePageLimitAndSaysSo() = runTest {
        val endless = ok("[]", mapOf("Link" to "</boxes.json?page=next>; rel=\"next\""))
        val hey = mockHey(endless, endless, endless, endless)
        val client = hey.client { maxPages = 2 }
        val visited = mutableListOf<Page<*>>()
        val error = assertFailsWith<HeyException.Api> { client.eachPage(client.boxes.list()) { visited += it; true } }
        assertEquals(2, visited.size)
        assertEquals(2, hey.requests.size)
        assertEquals(false, error.retryable)
    }

    @Test
    fun aFlowReadsEveryPage() = runTest {
        val hey = mockHey(
            ok("""[{"id":1,"kind":"imbox","name":"a"}]""", mapOf("Link" to "</boxes.json?page=2>; rel=\"next\"")),
            ok("""[{"id":2,"kind":"imbox","name":"b"}]""", mapOf("Link" to "</boxes.json?page=3>; rel=\"next\"")),
            ok("""[{"id":3,"kind":"imbox","name":"c"}]"""),
        )
        val client = hey.client()
        val ids = client.pages(client.boxes.list()).toList().flatMap { page -> page.value.map { it.id } }
        assertEquals(listOf(1L, 2L, 3L), ids)
    }
}
