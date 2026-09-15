package com.basecamp.hey

import com.basecamp.hey.generated.stickies
import com.basecamp.hey.services.MAX_STICKY_POSITION
import com.basecamp.hey.services.StickySize
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class StickiesServiceTest {
    private fun body(request: RecordedRequest): JsonObject = Json.parseToJsonElement(request.body).jsonObject

    @Test
    fun aLimitIsClampedToTheServersAndZeroIsLeftOff() = runTest {
        val hey = mockHey(ok("""[{"id":1,"body":"Milk"}]"""), ok("[]"), ok("[]"))
        val client = hey.client()
        assertEquals(listOf(1L), client.stickies.listUpTo(10).map { it.id })
        client.stickies.listUpTo(500)
        client.stickies.listUpTo(0)
        assertEquals("/stickies.json", hey.requests[0].path)
        assertEquals("10", hey.requests[0].query("limit"))
        assertEquals("100", hey.requests[1].query("limit"), "the server clamps anything above 100, so the client does too")
        assertNull(hey.requests[2].query("limit"), "limit=0 would be clamped to one sticky rather than read as no limit")
        assertFailsWith<HeyException.Usage> { client.stickies.listUpTo(-1) }
    }

    @Test
    fun aStickyIsWrittenUnderItsKeyWithOnlyWhatIsSaid() = runTest {
        val hey = mockHey(ok("""{"id":1,"body":"Milk","size":"large"}"""), ok("""{"id":1,"body":"Milk","size":"small"}"""))
        val client = hey.client()
        val created = client.stickies.createSticky("Milk", StickySize.LARGE)
        assertEquals("large", created.size)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/stickies.json", hey.requests[0].path)
        val sticky = body(hey.requests[0]).getValue("sticky").jsonObject
        assertEquals("Milk", sticky.getValue("body").jsonPrimitive.content)
        assertEquals("large", sticky.getValue("size").jsonPrimitive.content)

        client.stickies.updateSticky(1, "", StickySize.SMALL)
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals("/stickies/1.json", hey.requests[1].path)
        val revised = body(hey.requests[1]).getValue("sticky").jsonObject
        assertNull(revised["body"], "an empty body is left alone")
        assertEquals("small", revised.getValue("size").jsonPrimitive.content)
    }

    @Test
    fun aMoveCarriesTheIdAndPositionAtTheTopLevelWithinTheBoardsRange() = runTest {
        val hey = mockHey(ok(""))
        val client = hey.client()
        client.stickies.moveTo(1, 4)
        assertEquals("/stickies/moves.json", hey.requests.single().path)
        assertEquals("""{"id":1,"position":4}""", hey.requests.single().body)
        assertFailsWith<HeyException.Usage> { client.stickies.moveTo(1, -1) }
        assertFailsWith<HeyException.Usage> { client.stickies.moveTo(1, MAX_STICKY_POSITION + 1) }
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun aSizeIsReadAsHeyWritesIt() {
        assertEquals(StickySize.MEDIUM, StickySize.parse("medium"))
        assertFailsWith<HeyException.Usage> { StickySize.parse("huge") }
    }
}
