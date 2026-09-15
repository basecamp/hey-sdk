package com.basecamp.hey

import com.basecamp.hey.generated.postings
import com.basecamp.hey.services.BubbleUpSlot
import com.basecamp.hey.services.PostingChangesCursor
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class PostingsSugarTest {
    private fun body(request: RecordedRequest): JsonObject = Json.parseToJsonElement(request.body).jsonObject

    private fun ids(request: RecordedRequest): List<Long> = body(request).getValue("posting_ids").jsonArray.map { it.jsonPrimitive.content.toLong() }

    private val boxIndex = """[{"id":1,"kind":"imbox","name":"Imbox"},{"id":3,"kind":"feedbox","name":"The Feed"},{"id":5,"kind":"trailbox","name":"Paper Trail"}]"""

    @Test
    fun theFeedAndThePaperTrailAreResolvedFromOneReadOfTheIndex() = runTest {
        val hey = mockHey(ok(boxIndex), ok(""), ok(""))
        val client = hey.client()
        client.postings.moveToFeed(listOf(7))
        client.postings.moveToPaperTrail(listOf(8, 9))
        assertEquals(3, hey.requests.size)
        assertEquals("/boxes.json", hey.requests[0].path)
        assertEquals("/postings/moves.json", hey.requests[1].path)
        assertEquals(3L, body(hey.requests[1]).getValue("box_id").jsonPrimitive.content.toLong())
        assertEquals(5L, body(hey.requests[2]).getValue("box_id").jsonPrimitive.content.toLong())
        assertEquals(listOf(8L, 9L), ids(hey.requests[2]))
    }

    @Test
    fun trashingLeavesRemoveAccessOffUnlessItIsForEveryone() = runTest {
        val hey = mockHey(ok(""), ok(""))
        val client = hey.client()
        client.postings.moveToTrash(listOf(7))
        client.postings.trashForEveryone(listOf(7, 8))
        assertEquals("/postings/trash.json", hey.requests[0].path)
        assertNull(body(hey.requests[0])["remove_access"], "left out, HEY removes only your own access from a shared topic")
        assertEquals("false", body(hey.requests[1]).getValue("remove_access").jsonPrimitive.content)
        assertEquals(listOf(7L, 8L), ids(hey.requests[1]))
    }

    @Test
    fun theBodylessEndpointsCarryTheSelectionCommaJoinedInTheQuery() = runTest {
        val hey = mockHey(ok(""), ok(""), ok(""))
        val client = hey.client()
        client.postings.unmutePostings(listOf(7, 8))
        client.postings.removePostingsFromBoxGroup(listOf(9))
        client.postings.cancelPostingsBubbleUp(listOf(10, 11, 12))
        assertEquals("/postings/mutings.json", hey.requests[0].path)
        assertEquals("7,8", hey.requests[0].query("posting_ids"))
        assertEquals("", hey.requests[0].body)
        assertEquals("/postings/box_groups.json", hey.requests[1].path)
        assertEquals("DELETE", hey.requests[1].method)
        assertEquals("9", hey.requests[1].query("posting_ids"))
        assertEquals("/postings/bubble_up.json", hey.requests[2].path)
        assertEquals("DELETE", hey.requests[2].method)
        assertEquals("10,11,12", hey.requests[2].query("posting_ids"))
    }

    @Test
    fun spamAndBubbleUpNowSendTheSelectionAsABody() = runTest {
        val hey = mockHey(ok(""), ok(""))
        val client = hey.client()
        client.postings.markPostingsSpam(listOf(7))
        client.postings.bubbleUpPostingsNow(listOf(8, 9))
        assertEquals("/postings/spam.json", hey.requests[0].path)
        assertEquals(listOf(7L), ids(hey.requests[0]))
        assertEquals("/postings/bulk_bubble_up_now.json", hey.requests[1].path)
        assertEquals(listOf(8L, 9L), ids(hey.requests[1]))
    }

    @Test
    fun aBoxGroupTakesTheBoxAndTheGroup() = runTest {
        val hey = mockHey(ok(""))
        hey.client().postings.addPostingsToBoxGroup(2, 44, listOf(7, 8))
        val request = hey.requests.single()
        assertEquals("/postings/box_groups.json", request.path)
        assertEquals("POST", request.method)
        val sent = body(request)
        assertEquals(2L, sent.getValue("box_id").jsonPrimitive.content.toLong())
        assertEquals(44L, sent.getValue("box_group_id").jsonPrimitive.content.toLong())
        assertEquals(listOf(7L, 8L), ids(request))
    }

    @Test
    fun filingNamesTheFolderAndUnfilingLeavesAZeroFolderOff() = runTest {
        val hey = mockHey(ok(""), ok(""), ok(""), ok(""))
        val client = hey.client()
        client.postings.filePostings(31, listOf(7))
        client.postings.unfilePostings(31, listOf(7, 8))
        client.postings.unfilePostings(0, listOf(9))
        client.postings.createFolderForPostings("Receipts", listOf(10))
        assertEquals("/postings/filings.json", hey.requests[0].path)
        assertEquals("POST", hey.requests[0].method)
        assertEquals(31L, body(hey.requests[0]).getValue("folder_id").jsonPrimitive.content.toLong())
        assertEquals("/postings/filings.json", hey.requests[1].path)
        assertEquals("DELETE", hey.requests[1].method)
        assertEquals("7,8", hey.requests[1].query("posting_ids"))
        assertEquals("31", hey.requests[1].query("folder_id"))
        assertNull(hey.requests[2].query("folder_id"), "zero is a folder that does not exist to HEY, so it is not sent")
        assertEquals("9", hey.requests[2].query("posting_ids"))
        assertEquals("/postings/folders.json", hey.requests[3].path)
        val folder = body(hey.requests[3]).getValue("folder").jsonObject
        assertEquals("Receipts", folder.getValue("name").jsonPrimitive.content)
        assertNull(folder["status"])
    }

    @Test
    fun aBubbleUpSlotNamesItselfAndOnlyACustomOneCarriesADate() = runTest {
        val hey = mockHey(ok(""), ok(""))
        val client = hey.client()
        client.postings.schedulePostingsBubbleUp(BubbleUpSlot.NextWeek, listOf(7))
        client.postings.schedulePostingsBubbleUp(BubbleUpSlot.Custom("2026-10-01"), listOf(7, 8))
        assertEquals("/postings/bubble_up.json", hey.requests[0].path)
        assertEquals("next_week", body(hey.requests[0]).getValue("slot").jsonPrimitive.content)
        assertNull(body(hey.requests[0])["date"])
        assertEquals("custom", body(hey.requests[1]).getValue("slot").jsonPrimitive.content)
        assertEquals("2026-10-01", body(hey.requests[1]).getValue("date").jsonPrimitive.content)
        assertEquals("today", BubbleUpSlot.LaterToday.wire)
        assertEquals("tomorrow", BubbleUpSlot.Tomorrow.wire)
        assertEquals("weekend", BubbleUpSlot.ThisWeekend.wire)
        assertFailsWith<HeyException.Usage> { BubbleUpSlot.Custom("2026-02-30") }
        assertFailsWith<HeyException.Usage> { BubbleUpSlot.Custom("next tuesday") }
    }

    @Test
    fun anEmptySelectionIsRefusedBeforeAnythingIsSentAndASingleOneIsNamedToTheHooks() = runTest {
        val hey = mockHey(ok(""), ok(""))
        val named = mutableListOf<Long?>()
        val client = hey.client {
            hooks = object : HeyHooks {
                override fun onOperationStart(info: OperationInfo) {
                    named += info.resourceId
                }
            }
        }
        assertFailsWith<HeyException.Usage> { client.postings.unmutePostings(emptyList()) }
        assertFailsWith<HeyException.Usage> { client.postings.markPostingsSpam(emptyList()) }
        assertFailsWith<HeyException.Usage> { client.postings.unfilePostings(1, emptyList()) }
        assertFailsWith<HeyException.Usage> { client.postings.schedulePostingsBubbleUp(BubbleUpSlot.Tomorrow, emptyList()) }
        assertFailsWith<HeyException.Usage> { client.postings.trashForEveryone(emptyList()) }
        assertTrue(hey.requests.isEmpty())
        client.postings.unmutePostings(listOf(7))
        client.postings.markPostingsSpam(listOf(7, 8))
        assertEquals(listOf<Long?>(7, null), named)
    }

    @Test
    fun allChangesFollowsThePagesOfAnIncrementAndKeepsTheLastCursor() = runTest {
        val hey = mockHey(
            ok("""{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}""", mapOf("Link" to "</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&v=2&page=2>; rel=\"next\"")),
            ok("""{"added":[],"updated":[{"id":3,"kind":"topic"}],"deleted":[{"id":2}]}""", mapOf("Link" to "</boxes/7/postings/changes.json?since=2026-09-15T10:05:00Z&v=2>; rel=\"next\"")),
        )
        val store = InMemoryCache()
        val all = hey.client {
            enableCache = true
            cache = store
        }.postings.allChanges(7, PostingChangesCursor("2026-09-15T10:00:00Z", version = "2"))
        assertEquals(listOf(1L), all.added.map { it.id })
        assertEquals(listOf(3L), all.updated.map { it.id })
        assertEquals(listOf(2L), all.deleted.map { it.id })
        assertNull(all.nextPage, "the increment was read to its end")
        assertEquals(PostingChangesCursor("2026-09-15T10:05:00Z", version = "2"), all.nextCursor)
        assertEquals(2, hey.requests.size)
        assertEquals("2", hey.requests[1].query("page"))
        assertEquals("2026-09-15T10:01:00Z", hey.requests[1].query("since"), "the next page is read as HEY issued it")
        assertEquals(0, store.size, "and no page of the feed is held, as with one page")
    }

    @Test
    fun allChangesComesBackAsSoonAsAFullSyncIsAskedFor() = runTest {
        val hey = mockHey(
            ok("""{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}""", mapOf("Link" to "</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&page=2>; rel=\"next\"")),
            status(409, """{"error":"cursor too old"}"""),
        )
        val stale = hey.client().postings.allChanges(7, PostingChangesCursor("2026-09-15T10:00:00Z"))
        assertTrue(stale.fullSyncRequired)
        assertEquals(emptyList(), stale.added, "what was read before is dropped: the box has to be read in full anyway")
        assertEquals(2, hey.requests.size)
    }

    @Test
    fun allChangesStopsAtTheClientPageLimitAndSaysWhereItStopped() = runTest {
        val hey = mockHey(
            ok("""{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}""", mapOf("Link" to "</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&page=2>; rel=\"next\"")),
            ok("""{"added":[{"id":2,"kind":"topic"}],"updated":[],"deleted":[]}""", mapOf("Link" to "</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&page=3>; rel=\"next\"")),
            ok("""{"added":[{"id":3,"kind":"topic"}],"updated":[],"deleted":[]}"""),
        )
        val capped = hey.client { maxPages = 2 }.postings.allChanges(7, PostingChangesCursor("2026-09-15T10:00:00Z"))
        assertEquals(listOf(1L, 2L), capped.added.map { it.id })
        assertEquals(PostingChangesCursor("2026-09-15T10:01:00Z", page = "3"), capped.nextPage, "the page not read is named, so the answer does not look complete")
        assertEquals(2, hey.requests.size)
    }
}
