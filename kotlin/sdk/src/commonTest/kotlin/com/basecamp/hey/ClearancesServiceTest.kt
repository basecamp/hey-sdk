package com.basecamp.hey

import com.basecamp.hey.generated.clearances
import com.basecamp.hey.services.ClearanceStatus
import com.basecamp.hey.services.ScreenOptions
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ClearancesServiceTest {
    private fun body(request: RecordedRequest): JsonObject = Json.parseToJsonElement(request.body).jsonObject

    private val queue = """{"pending_clearances_count":2,"signed_stream_name":"abc","clearances":[{"id":11,"status":"pending","petitioner":{"id":5,"name":"Ann"}},{"id":12,"status":"pending"}]}"""

    @Test
    fun theSummaryIsTheCheapReadAndTheCountComesOutOfIt() = runTest {
        val hey = mockHey(ok("""{"pending_clearances_count":3,"signed_stream_name":"abc"}"""), ok("""{}"""))
        val client = hey.client()
        val summary = client.clearances.summary()
        assertEquals(3, summary.pendingClearancesCount)
        assertEquals("abc", summary.signedStreamName)
        assertNull(summary.clearances)
        assertEquals("/clearances.json", hey.requests[0].path)
        assertNull(hey.requests[0].query("include_clearances"), "the summary asks for the count alone")
        assertEquals(0, client.clearances.pendingCount(), "a count HEY leaves out reads as none waiting")
    }

    @Test
    fun theQueueIsAskedForByNameAndWalkedByCursor() = runTest {
        val hey = mockHey(ok(queue, mapOf("Link" to "</clearances.json?include_clearances=true&page=abc>; rel=\"next\"")), ok(queue))
        val client = hey.client()
        val first = client.clearances.pendingPage()
        assertEquals("true", hey.requests[0].query("include_clearances"))
        assertNull(hey.requests[0].query("page"))
        assertEquals(listOf(11L, 12L), first.value.clearances!!.map { it.id })
        assertEquals("Ann", first.value.clearances!![0].petitioner?.name)
        assertEquals("abc", first.nextPage)
        val next = client.clearances.pending(first.nextPage)
        assertEquals("abc", hey.requests[1].query("page"))
        assertEquals(2, next.pendingClearancesCount)
    }

    @Test
    fun screeningSendsTheDecisionAndOnlyTheOptionsThatAreOn() = runTest {
        val hey = mockHey(ok("""{"id":11,"status":"approved"}"""), ok("""{"id":12,"status":"denied"}"""))
        val client = hey.client()
        val approved = client.clearances.screen(11, ClearanceStatus.APPROVED, ScreenOptions(designationBoxId = 3, markTopicsAsSeen = true))
        assertEquals("approved", approved.status)
        assertEquals("PATCH", hey.requests[0].method)
        assertEquals("/clearances/11.json", hey.requests[0].path)
        val sent = body(hey.requests[0])
        assertEquals("approved", sent.getValue("status").jsonPrimitive.content)
        assertEquals(3L, sent.getValue("designation_box_id").jsonPrimitive.content.toLong())
        assertEquals("true", sent.getValue("mark_topics_as_seen").jsonPrimitive.content)
        assertNull(sent["spam"], "a flag HEY reads for truthiness stays off the wire when it is off")

        client.clearances.screen(12, ClearanceStatus.DENIED)
        assertEquals(setOf("status"), body(hey.requests[1]).keys)
    }

    @Test
    fun screeningManyJoinsTheIdsAndAnswersWhatChanged() = runTest {
        val hey = mockHey(ok("""{"clearances":[{"id":11,"status":"denied"}]}"""))
        val client = hey.client()
        val changed = client.clearances.screenMany(listOf(11, 12), ClearanceStatus.DENIED, spam = true)
        assertEquals(listOf(11L), changed.map { it.id }, "a partial match answers only what it touched")
        assertEquals("/clearances/bulk.json", hey.requests.single().path)
        val sent = body(hey.requests.single())
        assertEquals("11,12", sent.getValue("ids").jsonPrimitive.content)
        assertEquals("denied", sent.getValue("status").jsonPrimitive.content)
        assertEquals("true", sent.getValue("spam").jsonPrimitive.content)
        assertFailsWith<HeyException.Usage> { client.clearances.screenMany(emptyList(), ClearanceStatus.APPROVED) }
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun theDecidedListIsReadAndRescreenedSeparatelyFromTheQueue() = runTest {
        val hey = mockHey(
            ok("""{"clearances":[{"id":21,"status":"approved"}]}""", mapOf("Link" to "</my/clearances.json?page=xyz>; rel=\"next\"")),
            ok("""{"clearances":[]}"""),
            ok("""{"id":21,"status":"denied"}"""),
        )
        val client = hey.client()
        val page = client.clearances.screenedPage()
        assertEquals("/my/clearances.json", hey.requests[0].path)
        assertEquals(listOf(21L), page.value.clearances!!.map { it.id })
        assertEquals("xyz", page.nextPage)
        assertTrue(client.clearances.screened("xyz").isEmpty())
        assertEquals("xyz", hey.requests[1].query("page"))

        val rescreened = client.clearances.rescreen(21, ClearanceStatus.DENIED)
        assertEquals("denied", rescreened.status)
        assertEquals("/my/clearances/21.json", hey.requests[2].path)
        assertEquals("""{"status":"denied"}""", hey.requests[2].body)
    }

    @Test
    fun aStatusIsReadAsHeyWritesIt() {
        assertEquals(ClearanceStatus.APPROVED, ClearanceStatus.parse("approved"))
        assertEquals(ClearanceStatus.DENIED, ClearanceStatus.parse("denied"))
        assertFailsWith<HeyException.Validation> { ClearanceStatus.parse("maybe") }
    }
}
