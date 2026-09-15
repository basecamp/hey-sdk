package com.basecamp.hey

import com.basecamp.hey.services.BoxKind
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.services.Countdown
import com.basecamp.hey.services.CountdownUnit
import com.basecamp.hey.services.EventContent
import com.basecamp.hey.services.Repeat
import com.basecamp.hey.services.RepeatFrequency
import com.basecamp.hey.services.RepeatUntil
import com.basecamp.hey.services.UpdateCalendarEventParams
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
import kotlin.time.Duration.Companion.hours
import kotlin.time.Duration.Companion.minutes

class ServicesTest {
    private fun body(request: RecordedRequest): JsonObject = Json.parseToJsonElement(request.body).jsonObject

        /** Every value a form body carries under one name, in order, since a name may repeat for a list. */
    private fun formValues(body: String, name: String): List<String> =
        body.split('&').filter { it.isNotEmpty() }.map { pair ->
            val (key, value) = pair.split('=', limit = 2).let { it[0] to it.getOrElse(1) { "" } }
            key.decodeURLQueryComponent(plusIsSpace = true) to value.decodeURLQueryComponent(plusIsSpace = true)
        }.filter { it.first == name }.map { it.second }

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
    fun aWholeEventUpdateSendsEverythingHeyWouldOtherwiseClear() = runTest {
        val hey = mockHey(ok("""{"id":99,"type":"Calendar::Event","summary":"Standup"}"""), status(302, headers = mapOf("Location" to "/calendar/events/99")))
        val client = hey.client()
        val params = UpdateCalendarEventParams(
            title = "Standup",
            allDay = false,
            startTime = "09:30",
            endTime = "10:00",
            startTimeZone = "Europe/Zagreb",
            reminders = listOf(10.minutes, 1.hours),
            content = EventContent(notes = "<p>Agenda</p>", location = "Room 4", link = "https://example.com/call", entryId = 5),
            attendees = emptyList(),
            highlighted = true,
            countdown = Countdown(3, CountdownUnit.WEEKS),
            repeat = Repeat(RepeatFrequency.EVERY_WEEK, RepeatUntil.COUNT, count = 12),
        )
        val recording = client.calendarEvents.updateEvent(99, params)
        assertEquals(99L, recording.id)
        assertEquals("Standup", recording.summary)

        val request = hey.requests[0]
        assertEquals("PATCH", request.method)
        assertEquals("/calendar/events/99.json", request.path)
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        val fields = formFields(request.body)
        assertEquals("Standup", fields["calendar_event[summary]"])
        assertEquals("09:30:00", fields["calendar_event[starts_at_time]"])
        assertEquals("<p>Agenda</p>", fields["calendar_event[description]"])
        assertEquals("Room 4", fields["calendar_event[location]"])
        assertEquals("https://example.com/call", fields["calendar_event[url]"])
        assertEquals("5", fields["calendar_event[entry_id]"])
        assertEquals("", fields["calendar_event[attendance_email_addresses][]"], "an empty guest list goes out as one blank value")
        assertEquals("1", fields["calendar_event[highlighted]"])
        assertEquals("", fields["calendar_event[highlight_id]"])
        assertEquals("3", fields["countdown_interval_duration_value"])
        assertEquals("604800", fields["countdown_interval_duration_unit"])
        assertEquals("every_week", fields["repeat_frequency"])
        assertEquals("count", fields["calendar_recurrence_schedule[recurs_until_type]"])
        assertEquals("12", fields["calendar_recurrence_schedule[recurs_count]"])
        assertEquals("1", fields["calendar_event[set_time_zone]"])
        assertEquals("Europe/Zagreb", fields["calendar_event[starts_at_time_zone_name]"])
        assertEquals("Europe/Zagreb", fields["calendar_event[ends_at_time_zone_name]"], "the one zone named stands in for the other")
        assertEquals(listOf("600", "3600"), formValues(request.body, "timed_reminder_durations[]"))

        val defaults = UpdateCalendarEventParams(title = "Renamed")
        val fallback = client.calendarEvents.updateEvent(99, defaults)
        assertEquals(99L, fallback.id, "an older server answers a redirect, whose URL still names the recording")
        assertEquals("", fallback.type, "and nothing else about it, so the type is not invented")
        val cleared = formFields(hey.requests[1].body)
        assertEquals("", cleared["calendar_event[description]"], "what the params leave empty goes out empty, since HEY clears it either way")
        assertEquals("", cleared["calendar_event[entry_id]"])
        assertNull(cleared["countdown_interval_duration_value"], "a zero countdown sends nothing, which deletes it")
        assertNull(cleared["calendar_event[attendance_email_addresses][]"], "a null guest list is left alone")
        assertNull(cleared["calendar_event[set_time_zone]"], "no zone named says nothing about zones")
    }

    @Test
    fun anOccurrenceUpdateKeepsTheSeriesScheduleUnlessToldOtherwise() = runTest {
        val hey = mockHey(ok("""{"id":0,"type":"Calendar::Event","parent_id":153688907}"""))
        val client = hey.client()
        client.calendarEvents.updateOccurrence(OccurrenceId.parse("153688907_2026-08-21"), OccurrenceScope.THIS_AND_FOLLOWING, UpdateCalendarEventParams(title = "Moved"))
        val request = hey.requests.single()
        assertEquals("/calendar/events/153688907/occurrences/2026-08-21.json", request.path)
        val fields = formFields(request.body)
        assertEquals("1", fields["apply_to_future"])
        assertEquals("custom", fields["repeat_frequency"], "silence would end the series, so the schedule is kept in so many words")
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
