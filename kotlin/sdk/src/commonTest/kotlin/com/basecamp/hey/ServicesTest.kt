package com.basecamp.hey

import com.basecamp.hey.services.BoxKind
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.services.DraftContent
import com.basecamp.hey.services.MessageContent
import com.basecamp.hey.services.OccurrenceId
import com.basecamp.hey.services.OccurrenceScope
import com.basecamp.hey.services.ReplyContent
import com.basecamp.hey.generated.*
import io.ktor.http.decodeURLQueryComponent
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

class ServicesTest {
    private fun body(request: RecordedRequest): JsonObject = Json.parseToJsonElement(request.body).jsonObject

    private fun formFields(body: String): Map<String, String> = body.split('&').associate {
        it.substringBefore('=').decodeURLQueryComponent() to it.substringAfter('=').decodeURLQueryComponent()
    }

    @Test
    fun sendRefusesAMessageAddressedToNobody() = runTest {
        val hey = mockHey()
        assertFailsWith<HeyException.Usage> { hey.client().messages.send(MessageContent(subject = "Hi", content = "Body")) }
        assertTrue(hey.requests.isEmpty())
    }

    @Test
    fun sendDeliversAsTheChosenSenderWithTheRecipientsThatNameSomebody() = runTest {
        val hey = mockHey(ok(""))
        hey.client().messages.send(MessageContent("Subject", "<div>Body</div>", to = listOf("someone@example.com"), actingSenderId = 314))
        val sent = body(hey.requests.single())
        assertEquals(314L, sent.getValue("acting_sender_id").jsonPrimitive.content.toLong())
        assertEquals("Subject", sent.getValue("message").jsonObject.getValue("subject").jsonPrimitive.content)
        val addressed = sent.getValue("entry").jsonObject.getValue("addressed").jsonObject
        assertEquals(setOf("directly"), addressed.keys)
        assertNull(sent.getValue("entry").jsonObject["status"])
    }

    @Test
    fun sendResolvesTheDefaultSenderWhenNoneIsChosen() = runTest {
        val hey = mockHey(ok(IDENTITY), ok(""))
        hey.client().messages.send(MessageContent("Subject", "Body", to = listOf("a@example.com")))
        assertEquals("/identity.json", hey.requests[0].path)
        assertEquals(100L, body(hey.requests[1]).getValue("acting_sender_id").jsonPrimitive.content.toLong())
    }

    @Test
    fun aDraftIsSavedDraftedAndAnswersItsEntryId() = runTest {
        val hey = mockHey(status(204, headers = mapOf("Location" to "https://app.hey.com/messages/777")), ok(""), ok(""))
        val client = hey.client()
        val draft = DraftContent("Subject", "Body", actingSenderId = 314)
        assertEquals(777L, client.messages.createDraft(draft))
        val saved = body(hey.requests[0])
        assertEquals("drafted", saved.getValue("entry").jsonObject.getValue("status").jsonPrimitive.content)
        val addressed = saved.getValue("entry").jsonObject.getValue("addressed").jsonObject
        assertEquals(setOf("directly", "copied", "blindcopied"), addressed.keys)
        assertEquals(0, addressed.getValue("directly").jsonArray.size)

        client.messages.updateDraft(777, draft)
        assertEquals("PUT", hey.requests[1].method)
        assertEquals("/messages/777.json", hey.requests[1].path)

        client.messages.sendDraft(777, draft.copy(to = listOf("someone@example.com")))
        val delivered = body(hey.requests[2])
        assertNull(delivered.getValue("entry").jsonObject["status"])
        assertEquals("someone@example.com", delivered.getValue("entry").jsonObject.getValue("addressed").jsonObject.getValue("directly").jsonArray[0].jsonPrimitive.content)
    }

    @Test
    fun sendingADraftIsNeverRetried() = runTest {
        val hey = mockHey(status(503))
        assertFailsWith<HeyException.Api> {
            hey.client().messages.sendDraft(777, DraftContent("S", "B", to = listOf("a@example.com"), actingSenderId = 1))
        }
        assertEquals(1, hey.requests.size)
        assertFailsWith<HeyException.Usage> { hey.client().messages.sendDraft(777, DraftContent("S", "B", actingSenderId = 1)) }
    }

    @Test
    fun aReplyCarriesThePrefillsSenderUntouched() = runTest {
        val hey = mockHey(ok(""), status(204, headers = mapOf("Location" to "/messages/778")))
        val client = hey.client()
        val reply = ReplyContent(314, "Re: Hello", "Reply text", to = listOf("someone@example.com"))
        client.entries.reply(456, reply)
        val sent = body(hey.requests[0])
        assertEquals("/entries/456/replies.json", hey.requests[0].path)
        assertEquals(314L, sent.getValue("acting_sender_id").jsonPrimitive.content.toLong())
        assertEquals("Re: Hello", sent.getValue("message").jsonObject.getValue("subject").jsonPrimitive.content)

        assertEquals(778L, client.entries.replyDraft(456, reply.copy(to = emptyList(), subject = "")))
        val drafted = body(hey.requests[1])
        assertEquals("drafted", drafted.getValue("entry").jsonObject.getValue("status").jsonPrimitive.content)
        assertNull(drafted.getValue("message").jsonObject["subject"])
        assertFailsWith<HeyException.Usage> { client.entries.reply(456, reply.copy(to = emptyList())) }
    }

    @Test
    fun aCalendarEventUpdateIsAFormPostToTheJsonPath() = runTest {
        val hey = mockHey(ok(""), ok(""))
        val client = hey.client()
        client.calendarEvents.update(99, CalendarEventUpdate("Sarah's birthday", "2026-09-02", "2026-09-02", allDay = true, startTime = "", endTime = ""))
        val request = hey.requests[0]
        assertEquals("PATCH", request.method)
        assertEquals("/calendar/events/99.json", request.path)
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        assertEquals("application/json", request.header("Accept"))
        val fields = formFields(request.body)
        assertEquals("Sarah's birthday", fields["calendar_event[summary]"])
        assertEquals("1", fields["calendar_event[all_day]"])
        assertNull(fields["calendar_event[starts_at_time]"])

        client.calendarEvents.update(99, CalendarEventUpdate(allDay = false, startTime = "09:30", endTime = "10:00"))
        val timed = formFields(hey.requests[1].body)
        assertEquals("09:30:00", timed["calendar_event[starts_at_time]"])
        assertEquals("0", timed["calendar_event[all_day]"])
    }

    @Test
    fun anOccurrenceDeleteCarriesTheScopeInTheQuery() = runTest {
        val hey = mockHey(ok(""))
        hey.client().calendarEvents.deleteOccurrenceScoped(OccurrenceId.parse("153688907_2026-08-21"), OccurrenceScope.THIS_AND_FOLLOWING)
        assertEquals("/calendar/events/153688907/occurrences/2026-08-21.json", hey.requests.single().path)
        assertEquals("true", hey.requests.single().query("apply_to_future"))
        assertFailsWith<HeyException.Usage> { OccurrenceId.parse("nope") }
        assertFailsWith<HeyException.Usage> { OccurrenceScope.parse("everything") }
    }

    @Test
    fun aRunningTrackIsAConflictAndStoppingAnnouncesItself() = runTest {
        val hey = mockHey(status(409, """{"error":"A time track is already running"}"""), ok("""{"id":1,"type":"Calendar::TimeTrack"}"""))
        val operations = mutableListOf<String>()
        val client = hey.client {
            hooks = object : HeyHooks {
                override fun onOperationStart(info: OperationInfo) {
                    operations += info.operation
                }
            }
        }
        val error = assertFailsWith<HeyException.Conflict> { client.timeTracks.startTracking() }
        assertEquals("A time track is already running", error.message)
        client.timeTracks.stop(1)
        assertEquals(listOf("StartTimeTrack", "StopTimeTrack"), operations)
        assertEquals("PUT", hey.requests[1].method)
        assertTrue(body(hey.requests[1]).getValue("calendar_time_track").jsonObject.containsKey("ends_at"))
    }

    @Test
    fun boxesAreResolvedByKindFromOneReadOfTheIndex() = runTest {
        val hey = mockHey(ok("""[{"id":1,"kind":"imbox","name":"Imbox"},{"id":2,"kind":"asidebox","name":"Set Aside"}]"""), ok(""))
        val client = hey.client()
        assertEquals(2L, client.boxes.idByKind(BoxKind.SET_ASIDE))
        assertEquals(1L, client.boxes.idByKind(BoxKind.IMBOX))
        client.postings.moveToSetAside(listOf(7, 8))
        assertEquals(2, hey.requests.size)
        assertEquals(2L, body(hey.requests[1]).getValue("box_id").jsonPrimitive.content.toLong())
        assertFailsWith<HeyException.Api> { client.boxes.idByKind(BoxKind.BUBBLE_UP) }
        assertFailsWith<HeyException.Usage> { client.postings.moveToSetAside(emptyList()) }

        val cold = mockHey()
        assertFailsWith<HeyException.Usage> { cold.client().postings.moveTo(BoxKind.IMBOX, emptyList()) }
        assertEquals(0, cold.requests.size, "an empty selection is refused before the box index is read")
    }
}
