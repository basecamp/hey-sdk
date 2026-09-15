package com.basecamp.hey

import com.basecamp.hey.generated.search
import com.basecamp.hey.services.SearchParams
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class SearchServiceTest {
    private val matches = """{"matches":[{"topic":{"id":1,"subject":"Invoice"},"posting_id":9,"entries":[]}]}"""

    @Test
    fun aSearchSendsOnlyTheRefinementsThatAreSet() = runTest {
        val hey = mockHey(ok(matches))
        val result = hey.client().search.search(SearchParams(query = "invoice", from = "billing@example.com", inBox = "papertrail", date = "2026"))
        assertEquals(1, result.matches.size)
        val request = hey.requests.single()
        assertEquals("/advanced_search.json", request.path)
        assertEquals("invoice", request.query("q"))
        assertEquals("billing@example.com", request.query("refine[from]"))
        assertEquals("papertrail", request.query("refine[in]"))
        assertEquals("2026", request.query("refine[date]"))
        assertNull(request.query("refine[to]"), "an empty refinement is left off the wire")
        assertNull(request.query("refine[label]"))
        assertNull(request.query("page"), "the first page is asked for by saying nothing")
    }

    @Test
    fun aPageIsNumberedAndTheNextOneComesOutOfTheLink() = runTest {
        val hey = mockHey(ok(matches, mapOf("Link" to "</advanced_search.json?q=invoice&page=3>; rel=\"next\"")), ok(matches))
        val client = hey.client()
        val page = client.search.searchPage(SearchParams(query = "invoice", page = 2))
        assertEquals("2", hey.requests[0].query("page"))
        assertEquals(3, page.nextPage)
        assertEquals(1, page.result.matches.size)

        val last = client.search.searchPage(SearchParams(query = "invoice", page = 1))
        assertNull(hey.requests[1].query("page"), "zero and one both ask for the first")
        assertNull(last.nextPage, "HEY sends no Link on the last page, which is how a walk is told to stop")
    }
}
