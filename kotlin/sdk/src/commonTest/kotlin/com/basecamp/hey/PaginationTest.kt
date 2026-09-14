package com.basecamp.hey

import kotlinx.coroutines.flow.toList
import com.basecamp.hey.generated.*
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
