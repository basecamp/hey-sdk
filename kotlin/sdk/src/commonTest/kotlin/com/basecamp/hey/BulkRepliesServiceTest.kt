package com.basecamp.hey

import com.basecamp.hey.generated.bulkReplies
import com.basecamp.hey.services.undoSendId
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class BulkRepliesServiceTest {
    @Test
    fun aDraftResolvesThePostingsIntoTheEntriesTheReplyGoesTo() = runTest {
        val hey = mockHey(ok("""{"content":"<div>Jane Doe</div>","entries":[{"id":1,"topic_id":2,"topic_name":"Hello","addressed":{}}]}"""))
        val draft = hey.client().bulkReplies.draft(listOf(7, 8, 9))
        assertEquals("<div>Jane Doe</div>", draft.content)
        assertEquals(listOf(1L), draft.entries.map { it.id })
        val request = hey.requests.single()
        assertEquals("/bulk_replies/new.json", request.path)
        assertEquals("7,8,9", request.query("posting_ids"), "the ids go out comma-joined, as the generated method takes them")
    }

    @Test
    fun sendingABulkReplyAnswersWhatWasQueued() = runTest {
        val hey = mockHey(ok("""{"id":9,"entries_count":2,"delayed":true,"undo_send_url":"https://app.hey.com/bulk_replies/9/undo_send"}"""))
        val delivery = hey.client().bulkReplies.send(listOf(1, 2), "<div>Thanks, all.</div>")
        assertEquals(9L, delivery.id)
        assertEquals(2, delivery.entriesCount)
        assertTrue(delivery.delayed)
        assertEquals(9L, undoSendId(delivery.undoSendUrl.orEmpty()))
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("/bulk_replies.json", request.path)
        val body = Json.parseToJsonElement(request.body).jsonObject
        assertEquals(listOf(1L, 2L), body.getValue("entry_ids").jsonArray.map { it.jsonPrimitive.content.toLong() })
        assertEquals("<div>Thanks, all.</div>", body.getValue("message").jsonObject.getValue("content").jsonPrimitive.content)
    }

    @Test
    fun aBulkReplyWithNothingSelectedIsRefusedBeforeAnythingIsSent() = runTest {
        val hey = mockHey()
        val client = hey.client()
        val noPostings = assertFailsWith<HeyException.Usage> { client.bulkReplies.draft(emptyList()) }
        assertEquals("at least one posting is required", noPostings.message)
        val noEntries = assertFailsWith<HeyException.Usage> { client.bulkReplies.send(emptyList(), "<div>Thanks</div>") }
        assertEquals("at least one entry is required", noEntries.message)
        assertTrue(hey.requests.isEmpty())
    }

    @Test
    fun callingABulkReplyBackPostsAnEmptyFormToItsUndoPath() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/imbox")))
        val log = OperationLog()
        hey.client { hooks = log }.bulkReplies.undo(9)
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("/bulk_replies/9/undo_send", request.path)
        assertEquals(BROWSER_ACCEPT_HEADER, request.header("Accept"))
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        assertEquals("", request.body)
        assertEquals(listOf("BulkReplies.UndoBulkReplySend:bulk_reply:true:9"), log.started)
    }

    @Test
    fun callingBackAReplyThatHasGoneOutSurfacesHeysRefusal() = runTest {
        val hey = mockHey(status(422, """{"error":"already sent"}"""))
        val refused = assertFailsWith<HeyException> { hey.client().bulkReplies.undo(9) }
        assertEquals(422, refused.httpStatus)
    }

    @Test
    fun theUndoIdIsReadOnlyOutOfAnUndoUrl() {
        assertEquals(9L, undoSendId("/bulk_replies/9/undo_send"))
        assertEquals(9L, undoSendId("https://app.hey.com/bulk_replies/9/undo_send?from=imbox"))
        assertEquals(9L, undoSendId("/bulk_replies/9/undo_send#top"))
        for (other in listOf("/bulk_replies/9", "/bulk_replies/nine/undo_send", "/topics/9/undo_send", "", "https://app.hey.com/")) {
            val refused = assertFailsWith<HeyException.Usage> { undoSendId(other) }
            assertEquals("not a bulk reply undo URL: $other", refused.message)
        }
    }
}
