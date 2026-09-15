package com.basecamp.hey

import com.basecamp.hey.services.BoxKind
import com.basecamp.hey.services.CalendarChangesCursor
import com.basecamp.hey.services.CreateCalendarEventParams
import com.basecamp.hey.services.HabitParams
import com.basecamp.hey.services.TodoChanges
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
import kotlin.test.assertContentEquals
import kotlin.test.assertFalse
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

    @Test
    fun remindersGoUnderTheListHeyWillRead() = runTest {
        val hey = mockHey(ok("""{"id":1,"type":"Calendar::Event"}"""), ok("""{"id":1,"type":"Calendar::Event"}"""), ok("""{"id":1,"type":"Calendar::Event"}"""))
        val client = hey.client()
        client.calendarEvents.updateEvent(1, UpdateCalendarEventParams(allDay = true, reminders = listOf(1.hours)))
        client.calendarEvents.updateEvent(1, UpdateCalendarEventParams(allDay = false, reminders = listOf(1.hours)))
        client.calendarEvents.updateEvent(1, UpdateCalendarEventParams(reminders = listOf(1.hours)))
        assertEquals(listOf("3600"), formValues(hey.requests[0].body, "all_day_reminder_durations[]"))
        assertEquals(emptyList(), formValues(hey.requests[0].body, "timed_reminder_durations[]"))
        assertEquals(listOf("3600"), formValues(hey.requests[1].body, "timed_reminder_durations[]"))
        assertEquals(emptyList(), formValues(hey.requests[1].body, "all_day_reminder_durations[]"))
        assertEquals(listOf("3600"), formValues(hey.requests[2].body, "all_day_reminder_durations[]"), "an update that leaves the flag alone cannot know which list HEY reads, so it sends both")
        assertEquals(listOf("3600"), formValues(hey.requests[2].body, "timed_reminder_durations[]"))
    }

    @Test
    fun anOccurrenceIdIsCheckedHoweverItIsMade() = runTest {
        assertEquals("2024-02-29", OccurrenceId.parse("9_2024-02-29").date)
        assertFailsWith<HeyException.Usage> { OccurrenceId.parse("9_2023-02-29") }
        assertFailsWith<HeyException.Usage> { OccurrenceId.parse("9_2026-02-30") }
        assertFailsWith<HeyException.Usage> { OccurrenceId.parse("9_2026-13-01") }
        assertFailsWith<HeyException.Usage> { OccurrenceId(9, "../../42") }
        assertFailsWith<HeyException.Usage> { OccurrenceId(0, "2026-01-01") }
        val hey = mockHey()
        assertFailsWith<HeyException.Usage> {
            hey.client().calendarEvents.updateOccurrence(OccurrenceId.parse("9_2026-04-31"), OccurrenceScope.THIS_ONLY, UpdateCalendarEventParams())
        }
        assertEquals(0, hey.requests.size)
    }

    /** The operations and requests the hooks heard, in order, for what a convenience announces itself as. */
    private class Transcript : HeyHooks {
        val operations = mutableListOf<OperationInfo>()
        val requests = mutableListOf<String>()
        override fun onOperationStart(info: OperationInfo) {
            operations += info
        }

        override fun onRequestStart(info: RequestInfo) {
            requests += "${info.method} ${info.url}"
        }
    }

    @Test
    fun aCalendarEventIsCreatedFromAFormPostedToTheJsonPath() = runTest {
        val hey = mockHey(ok("""{"id":7,"type":"Calendar::Event","summary":"Standup"}"""), status(302, headers = mapOf("Location" to "/calendar/events/8")))
        val transcript = Transcript()
        val client = hey.client { hooks = transcript }
        val timed = CreateCalendarEventParams(
            calendarId = 3,
            title = "Standup",
            startsAt = "2026-09-15",
            startTime = "09:30",
            endTime = "10:00",
            endTimeZone = "Europe/Zagreb",
            reminders = listOf(10.minutes),
            content = EventContent(notes = "<p>Agenda</p>", location = "Room 4"),
            attendees = listOf("a@example.com", "b@example.com"),
            highlighted = true,
            countdown = Countdown(2, CountdownUnit.DAYS),
            repeat = Repeat(RepeatFrequency.EVERY_WEEKDAY, RepeatUntil.DATE, untilDate = "2026-12-31"),
        )
        val created = client.calendarEvents.create(timed)
        assertEquals(7L, created.id)
        val request = hey.requests[0]
        assertEquals("POST", request.method)
        assertEquals("/calendar/events.json", request.path)
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        val fields = formFields(request.body)
        assertEquals("3", fields["calendar_event[calendar_id]"])
        assertEquals("Standup", fields["calendar_event[summary]"])
        assertEquals("2026-09-15", fields["calendar_event[starts_at]"])
        assertEquals("2026-09-15", fields["calendar_event[ends_at]"], "the end defaults to the start")
        assertEquals("0", fields["calendar_event[all_day]"])
        assertEquals("09:30:00", fields["calendar_event[starts_at_time]"])
        assertEquals("10:00:00", fields["calendar_event[ends_at_time]"])
        assertEquals("1", fields["calendar_event[set_time_zone]"])
        assertEquals("Europe/Zagreb", fields["calendar_event[starts_at_time_zone_name]"], "the one zone named stands in for the other")
        assertEquals("Europe/Zagreb", fields["calendar_event[ends_at_time_zone_name]"])
        assertEquals(listOf("600"), formValues(request.body, "timed_reminder_durations[]"))
        assertEquals(emptyList(), formValues(request.body, "all_day_reminder_durations[]"))
        assertEquals("<p>Agenda</p>", fields["calendar_event[description]"])
        assertEquals("Room 4", fields["calendar_event[location]"])
        assertEquals("", fields["calendar_event[url]"])
        assertEquals("", fields["calendar_event[entry_id]"])
        assertEquals(listOf("a@example.com", "b@example.com"), formValues(request.body, "calendar_event[attendance_email_addresses][]"))
        assertEquals("1", fields["calendar_event[highlighted]"])
        assertEquals("", fields["calendar_event[highlight_id]"])
        assertEquals("2", fields["countdown_interval_duration_value"])
        assertEquals("86400", fields["countdown_interval_duration_unit"])
        assertEquals("every_weekday", fields["repeat_frequency"])
        assertEquals("date", fields["calendar_recurrence_schedule[recurs_until_type]"])
        assertEquals("2026-12-31", fields["calendar_recurrence_schedule[recurs_until_date]"])
        assertEquals("CreateCalendarEvent", transcript.operations.single().operation)
        assertEquals("CalendarEvents", transcript.operations.single().service)
        assertNull(transcript.operations.single().resourceId, "a create names no record yet")

        val allDay = client.calendarEvents.create(CreateCalendarEventParams(calendarId = 3, title = "Holiday", startsAt = "2026-12-24", endsAt = "2026-12-26", allDay = true, reminders = listOf(1.hours)))
        assertEquals(8L, allDay.id, "an older server answers a redirect, whose URL still names the recording")
        assertEquals("", allDay.type)
        val whole = formFields(hey.requests[1].body)
        assertEquals("1", whole["calendar_event[all_day]"])
        assertEquals("2026-12-26", whole["calendar_event[ends_at]"])
        assertNull(whole["calendar_event[starts_at_time]"], "an all-day event has no clock times")
        assertNull(whole["calendar_event[set_time_zone]"], "and no zones to set them in")
        assertEquals(listOf("3600"), formValues(hey.requests[1].body, "all_day_reminder_durations[]"))
        assertNull(whole["countdown_interval_duration_value"])
        assertNull(whole["repeat_frequency"])
        assertNull(whole["calendar_event[highlighted]"], "unsaid on a create is not circled")
        assertNull(whole["calendar_event[attendance_email_addresses][]"])
    }

    @Test
    fun aTimedEventWithNoZoneIsReadInUtcAndOneWithoutTheEssentialsIsRefused() = runTest {
        val hey = mockHey(ok("""{"id":7,"type":"Calendar::Event"}"""))
        val client = hey.client()
        client.calendarEvents.create(CreateCalendarEventParams(calendarId = 3, title = "Call", startsAt = "2026-09-15", startTime = "09:00", endTime = "09:30"))
        val fields = formFields(hey.requests.single().body)
        assertEquals("0", fields["calendar_event[set_time_zone]"], "a create always says what it means about the zones")
        assertNull(fields["calendar_event[starts_at_time_zone_name]"])

        assertFailsWith<HeyException.Usage> { client.calendarEvents.create(CreateCalendarEventParams(calendarId = 3, title = "", startsAt = "2026-09-15", allDay = true)) }
        assertFailsWith<HeyException.Usage> { client.calendarEvents.create(CreateCalendarEventParams(calendarId = 3, title = "Call", startsAt = "", allDay = true)) }
        assertFailsWith<HeyException.Usage> { client.calendarEvents.create(CreateCalendarEventParams(calendarId = 3, title = "Call", startsAt = "2026-09-15", startTime = "09:00")) }
        assertEquals(1, hey.requests.size, "a refusal sends nothing")
    }

    @Test
    fun publishingReadsThePublicLinkBackUnderTheOperationThatAskedForIt() = runTest {
        val hey = mockHey(
            status(302, headers = mapOf("Location" to "/topics/5/sharing")),
            ok("""{"published":true,"url":"https://app.hey.com/p/abc"}"""),
            status(302, headers = mapOf("Location" to "/topics/5")),
        )
        val transcript = Transcript()
        val client = hey.client { hooks = transcript }
        val publication = client.publications.publish(5)
        assertTrue(publication.published)
        assertEquals("https://app.hey.com/p/abc", publication.url)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/topics/5/publication", hey.requests[0].path, "a form post goes to the path as written")
        assertEquals("application/x-www-form-urlencoded", hey.requests[0].header("Content-Type"))
        assertEquals("", hey.requests[0].body)
        assertEquals("GET", hey.requests[1].method)
        assertEquals("/topics/5/publication.json", hey.requests[1].path)
        assertEquals(listOf("CreateTopicPublication"), transcript.operations.map { it.operation }, "the read-back is quiet: one operation for two requests")
        assertEquals("publication", transcript.operations.single().resourceType)
        assertEquals(5L, transcript.operations.single().resourceId)
        assertEquals(2, transcript.requests.size, "while the request hooks hear both")

        client.publications.unpublish(5)
        assertEquals("DELETE", hey.requests[2].method)
        assertEquals("/topics/5/publication", hey.requests[2].path)
        assertEquals("", hey.requests[2].body)
        assertEquals("DeleteTopicPublication", transcript.operations[1].operation)
    }

    @Test
    fun aThreadThatMayNotBePublishedIsRefusedWithoutAReadBack() = runTest {
        val hey = mockHey(status(403, """{"error":"not eligible"}"""))
        assertFailsWith<HeyException.Forbidden> { hey.client().publications.publish(5) }
        assertEquals(1, hey.requests.size)
    }

    private fun directUpload(url: String, headers: String = """{"Content-Type":"application/pdf","Content-MD5":"XUFAKrxLKna5cZ2REBfFkg==","Authorization":"stale"}""") =
        """{"signed_id":"signed-123","attachable_sgid":"sgid-456","direct_upload":{"url":"$url","headers":$headers}}"""

    @Test
    fun anUploadReservesABlobAndPutsTheBytesWhereHeySaid() = runTest {
        val hey = mockHey(ok(directUpload("https://storage.example.com/blobs/abc?signature=secret")), ok(""))
        val transcript = Transcript()
        val client = hey.client { hooks = transcript }
        val upload = client.attachments.upload("report.pdf", "application/pdf", "hello".encodeToByteArray())
        assertEquals("signed-123", upload.signedId)
        assertEquals("sgid-456", upload.attachableSgid)

        val reservation = hey.requests[0]
        assertEquals("POST", reservation.method)
        assertEquals("/rails/active_storage/direct_uploads.json", reservation.path)
        val blob = body(reservation).getValue("blob").jsonObject
        assertEquals("report.pdf", blob.getValue("filename").jsonPrimitive.content)
        assertEquals(5L, blob.getValue("byte_size").jsonPrimitive.content.toLong())
        assertEquals("XUFAKrxLKna5cZ2REBfFkg==", blob.getValue("checksum").jsonPrimitive.content, "the MD5 of the bytes, base64 as Active Storage wants it")
        assertEquals("application/pdf", blob.getValue("content_type").jsonPrimitive.content)

        val stored = hey.requests[1]
        assertEquals("PUT", stored.method)
        assertEquals("storage.example.com", stored.url.host)
        assertEquals("/blobs/abc", stored.path)
        assertEquals("secret", stored.query("signature"))
        assertEquals("hello", stored.body)
        assertEquals("application/pdf", stored.header("Content-Type"))
        assertEquals("XUFAKrxLKna5cZ2REBfFkg==", stored.header("Content-MD5"))
        assertNull(stored.header("Authorization"), "HEY's credentials stay on HEY, and so does the stale one it echoed")
        assertEquals("Bearer test-token", reservation.header("Authorization"))
        assertEquals(listOf("CreateDirectUpload"), transcript.operations.map { it.operation }, "the reservation is the operation; the bytes are its second request")
        assertEquals("PUT https://storage.example.com", transcript.requests[1], "a URL that signs itself is heard as its origin alone")
    }

    @Test
    fun aStorageOnHeysOwnOriginIsPutToAsBuiltWithoutTheAccountScope() = runTest {
        val hey = mockHey(ok(IDENTITY), ok(directUpload("https://app.hey.com/rails/active_storage/disk/token123")), ok(""))
        val work = hey.client().forAccount(42)
        work.attachments.upload("a.txt", "text/plain", "x".encodeToByteArray())
        assertEquals("42", hey.requests[1].query("filtered_account_id"), "the reservation is HEY's and is scoped")
        assertNull(hey.requests[2].query("filtered_account_id"), "the put is the storage service's and is not")
        assertNull(hey.requests[2].header("Authorization"))
    }

    @Test
    fun aStorageThatRefusesOrRedirectsGetsNoCredentialsEitherWay() = runTest {
        var refreshes = 0
        val credentials = object : TokenProvider {
            override suspend fun accessToken(): String = "token"
            override suspend fun refresh(): Boolean { refreshes += 1; return true }
        }
        val refusing = mockHey(ok(directUpload("https://storage.example.com/blobs/abc")), status(401))
        val error = assertFailsWith<HeyException.Auth> { refusing.client { accessToken(credentials) }.attachments.upload("a.txt", "text/plain", "x".encodeToByteArray()) }
        assertEquals(401, error.httpStatus)
        assertEquals(0, refreshes, "a 401 from storage rejected none of HEY's credentials")
        assertEquals(2, refusing.requests.size, "and nothing is resent")

        val redirecting = mockHey(ok(directUpload("https://storage.example.com/blobs/abc")), status(307, headers = mapOf("Location" to "https://storage.example.com/blobs/abc-moved")), ok(""))
        redirecting.client { accessToken(credentials) }.attachments.upload("a.txt", "text/plain", "x".encodeToByteArray())
        assertEquals(3, redirecting.requests.size)
        assertEquals("/blobs/abc-moved", redirecting.requests[2].path)
        assertNull(redirecting.requests[2].header("Authorization"), "a hop on storage's own origin is not signed: the put never was")
        assertEquals("x", redirecting.requests[2].body, "a 307 keeps the bytes")
    }

    @Test
    fun anAttachmentWithNoContentTypeIsAStreamOfBytesAndOneWithoutAFilenameIsRefused() = runTest {
        val hey = mockHey(ok(directUpload("https://storage.example.com/blobs/abc", headers = """{"Content-Type":"application/octet-stream"}""")), ok(""))
        val client = hey.client()
        client.attachments.upload("notes.bin", null, ByteArray(0))
        assertEquals("application/octet-stream", body(hey.requests[0]).getValue("blob").jsonObject.getValue("content_type").jsonPrimitive.content)
        assertEquals("", hey.requests[1].body, "empty content is an empty attachment, not a mistake")
        assertFailsWith<HeyException.Usage> { client.attachments.upload("", "text/plain", "x".encodeToByteArray()) }
        assertEquals(2, hey.requests.size)
    }

    @Test
    fun anUploadThatCannotBeMadeSafelyIsRefusedBeforeTheBytesGo() = runTest {
        val empty = mockHey(ok("""{"signed_id":"","attachable_sgid":"","direct_upload":{"url":""}}"""))
        val refused = assertFailsWith<HeyException.Api> { empty.client().attachments.upload("a.txt", null, "x".encodeToByteArray()) }
        assertEquals("HEY returned an empty attachment upload response", refused.message)
        assertEquals(1, empty.requests.size)

        val insecure = mockHey(ok(directUpload("http://storage.example.com/blobs/abc")))
        val unsafe = assertFailsWith<HeyException.Usage> { insecure.client().attachments.upload("a.txt", null, "x".encodeToByteArray()) }
        assertTrue(unsafe.message!!.startsWith("unsafe attachment upload target"))
        assertEquals(1, insecure.requests.size)

        val storage = mockHey(ok(directUpload("https://storage.example.com/blobs/abc")), status(403, "<Error>denied</Error>", mapOf("Content-Type" to "application/xml")))
        val denied = assertFailsWith<HeyException.Forbidden> { storage.client().attachments.upload("a.txt", null, "x".encodeToByteArray()) }
        assertEquals(403, denied.httpStatus)
        assertEquals(2, storage.requests.size, "the bytes go once")
    }

    @Test
    fun aJournalEntryIsReadAndWrittenAsItsContent() = runTest {
        val hey = mockHey(
            status(204),
            ok("""{"id":1,"type":"Calendar::JournalEntry","content":"plain","content_html":"<p>rich</p>"}"""),
            ok("""{"id":1,"type":"Calendar::JournalEntry","content":"plain","content_html":""}"""),
            ok("""{"id":1,"type":"Calendar::JournalEntry","content":"written"}"""),
            status(204),
        )
        val transcript = Transcript()
        val client = hey.client { hooks = transcript }
        assertNull(client.journal.entry("2026-09-15"), "a day without an entry answers nothing, which is null rather than a body that will not decode")
        assertEquals("/calendar/days/2026-09-15/journal_entry.json", hey.requests[0].path)
        assertEquals("<p>rich</p>", client.journal.getContent("2026-09-15"))
        assertEquals("plain", client.journal.getContent("2026-09-15"), "a blank rendered body is not the entry")
        val written = client.journal.updateContent("2026-09-15", "written")
        assertEquals("written", written?.content)
        assertEquals("PATCH", hey.requests[3].method)
        assertEquals("written", body(hey.requests[3]).getValue("calendar_journal_entry").jsonObject.getValue("content").jsonPrimitive.content)
        assertNull(client.journal.updateContent("2026-09-15", ""), "empty content removes the entry, which HEY answers with nothing")
        assertEquals(listOf("GetJournalEntry", "GetJournalContent", "GetJournalContent", "UpdateJournalEntry", "UpdateJournalEntry"), transcript.operations.map { it.operation })
    }

    @Test
    fun theCalendarIndexIsReadWithWhatALiveFollowerNeeds() = runTest {
        val hey = mockHey(
            ok("""{"calendars":[{"calendar":{"id":3,"name":"Work"},"recording_changes_url":"https://app.hey.com/calendars/3/recording/changes?since=2026-09-15T10:00:00.000Z&v=1","signed_stream_name":"stream-3"}],"calendar_changes_url":"https://app.hey.com/calendar/changes?since=2026-09-15T10:00:00.000Z","selected_calendar_ids":[3]}"""),
            ok("""{"selected_calendar_ids":[]}"""),
        )
        val client = hey.client()
        val list = client.calendars.listWithChanges()
        assertEquals("/calendars.json", hey.requests[0].path)
        assertEquals("Work", list.calendars.single().calendar?.name)
        assertEquals("stream-3", list.calendars.single().signedStreamName)
        assertEquals(listOf(3L), list.selectedCalendarIds)
        assertEquals(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z"), CalendarChangesCursor.fromUrl(list.calendarChangesUrl!!), "a calendar changes URL carries no version")
        assertEquals(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z", version = "1"), CalendarChangesCursor.fromUrl(list.calendars.single().recordingChangesUrl!!))
        assertFailsWith<HeyException.Usage> { CalendarChangesCursor.fromUrl("/calendar/changes?since=x") }

        assertEquals(emptyList(), client.calendars.toggleSelection(3))
        assertEquals("POST", hey.requests[1].method)
        assertEquals("/calendars/3/toggle.json", hey.requests[1].path)
    }

    @Test
    fun theCalendarChangesFeedAnswersItsPagesThenTheCursorToPollNext() = runTest {
        val hey = mockHey(
            ok("""{"added":[{"calendar":{"id":4},"signed_stream_name":"stream-4"}],"updated":[],"deleted":[]}""", mapOf("Link" to "</calendar/changes.json?since=2026-09-15T10:00:00.000Z&page=2&per_page=50>; rel=\"next\"")),
            ok("""{"added":[],"updated":[{"id":3,"name":"Renamed"}],"deleted":[{"id":2,"deleted_at":"2026-09-15T10:04:00.000Z"}]}""", mapOf("Link" to "</calendar/changes.json?since=2026-09-15T10:05:00.000Z>; rel=\"next\"")),
            ok("""{"added":[],"updated":[],"deleted":[]}"""),
        )
        val store = InMemoryCache()
        val transcript = Transcript()
        val client = hey.client {
            enableCache = true
            cache = store
            hooks = transcript
        }
        assertFailsWith<HeyException.Usage> { client.calendars.calendarChanges(CalendarChangesCursor()) }
        val all = client.calendars.allCalendarChanges(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z"))
        assertEquals("stream-4", all.added.single().signedStreamName)
        assertEquals("Renamed", all.updated.single().name)
        assertEquals(listOf(2L), all.deleted.map { it.id })
        assertNull(all.nextPage)
        assertEquals(CalendarChangesCursor(since = "2026-09-15T10:05:00.000Z"), all.nextCursor, "the last page names where to resume")
        assertEquals("/calendar/changes.json", hey.requests[0].path)
        assertEquals("2026-09-15T10:00:00.000Z", hey.requests[0].query("since"))
        assertNull(hey.requests[0].query("v"), "the version is never invented")
        assertEquals("2", hey.requests[1].query("page"))
        assertEquals("50", hey.requests[1].query("per_page"), "the next read sends the cursor as HEY issued it")
        assertEquals(0, store.size, "no page of the feed is held")
        assertEquals(listOf("GetCalendarChanges", "GetCalendarChanges"), transcript.operations.map { it.operation })
        assertEquals("calendar", transcript.operations[0].resourceType)
        assertFalse(transcript.operations[0].isMutation)

        val quiet = client.calendars.calendarChanges(all.nextCursor!!)
        assertNull(quiet.nextCursor, "nothing changed, so the cursor that produced this page still stands")
        assertNull(quiet.nextPage)
    }

    @Test
    fun theRecordingChangesFeedKeepsTheWiresGroupingAndFoldsTheDeletions() = runTest {
        val hey = mockHey(
            ok(
                """{"added":{"Calendar::Event":[{"id":10,"type":"Calendar::Event"}]},"updated":{},"deleted":{"Calendar::Event":[{"id":11,"deleted_at":"2026-09-15T10:01:00.000Z","type":"Calendar::Event"}],"Calendar::Todo":[{"id":11,"deleted_at":"2026-09-15T10:01:00.000Z","type":"Calendar::Event"},{"id":12,"deleted_at":"2026-09-15T10:02:00.000Z","type":"Calendar::Todo"}]}}""",
                mapOf("Link" to "</calendars/3/recording/changes.json?since=2026-09-15T10:00:00.000Z&v=1&page=2>; rel=\"next\""),
            ),
            ok("""{"added":{"Calendar::Event":[{"id":13,"type":"Calendar::Event"}],"Calendar::Habit":[{"id":14,"type":"Calendar::Habit"}]}}""", mapOf("Link" to "</calendars/3/recording/changes.json?since=2026-09-15T10:05:00.000Z&v=1>; rel=\"next\"")),
            status(409, """{"error":"too far behind"}"""),
        )
        val transcript = Transcript()
        val client = hey.client { hooks = transcript }
        assertFailsWith<HeyException.Usage> { client.calendars.recordingChanges(3, CalendarChangesCursor(version = "1")) }
        assertFailsWith<HeyException.Usage> { client.calendars.recordingChanges(3, CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z")) }
        assertTrue(hey.requests.isEmpty())

        val all = client.calendars.allRecordingChanges(3, CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z", version = "1"))
        assertEquals(listOf(10L, 13L), all.added.getValue("Calendar::Event").map { it.id }, "pages merge within a type key")
        assertEquals(listOf(14L), all.added.getValue("Calendar::Habit").map { it.id })
        assertEquals(listOf(11L, 12L), all.deleted.map { it.id }, "a deletion repeated under every key arrives once")
        assertEquals("Calendar::Todo", all.deleted[1].type)
        assertEquals(CalendarChangesCursor(since = "2026-09-15T10:05:00.000Z", version = "1"), all.nextCursor)
        assertEquals("/calendars/3/recording/changes.json", hey.requests[0].path)
        assertEquals("1", hey.requests[0].query("v"))
        assertEquals("2", hey.requests[1].query("page"))
        assertEquals("GetCalendarRecordingChanges", transcript.operations[0].operation)
        assertEquals("recording", transcript.operations[0].resourceType)
        assertEquals(3L, transcript.operations[0].resourceId)

        val stale = client.calendars.allRecordingChanges(3, all.nextCursor!!)
        assertTrue(stale.fullSyncRequired, "a 409 is the feed refusing the cursor: read the calendar in full")
        assertEquals(emptyMap(), stale.added)
        assertEquals(3, hey.requests.size, "and it is not resent")
    }

    @Test
    fun aChangesLinkOffTheOriginIsRefused() = runTest {
        val hey = mockHey(ok("""{"added":[],"updated":[],"deleted":[]}""", mapOf("Link" to "<https://evil.example.com/calendar/changes.json?since=x&page=2>; rel=\"next\"")))
        val error = assertFailsWith<HeyException.Usage> { hey.client().calendars.calendarChanges(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z")) }
        assertFalse(error.message!!.contains("since=x"), "the refusal names the origin, not the URL")
    }

    @Test
    fun aChangesWalkStopsAtThePageLimitAndSaysWhereItStopped() = runTest {
        val page = ok("""{"added":[{"recording_changes_url":"/calendars/1/recording/changes.json"}],"updated":[],"deleted":[]}""", mapOf("Link" to "</calendar/changes.json?since=2026-09-15T10:00:00.000Z&page=2>; rel=\"next\""))
        val hey = mockHey(page, page, page)
        val client = hey.client { maxPages = 2 }
        val capped = client.calendars.allCalendarChanges(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z"))
        assertEquals(2, hey.requests.size)
        assertEquals(2, capped.added.size, "what was read is answered")
        assertEquals(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z", page = "2"), capped.nextPage, "and the page not read is named, so the answer does not look complete")
        assertNull(capped.nextCursor)

        val recordingPage = ok("""{"added":{},"updated":{},"deleted":{}}""", mapOf("Link" to "</calendars/3/recording/changes.json?since=2026-09-15T10:00:00.000Z&v=1&page=2>; rel=\"next\""))
        val recordings = mockHey(recordingPage, recordingPage, recordingPage)
        val cappedRecordings = recordings.client { maxPages = 2 }.calendars.allRecordingChanges(3, CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z", version = "1"))
        assertEquals(2, recordings.requests.size)
        assertEquals(CalendarChangesCursor(since = "2026-09-15T10:00:00.000Z", version = "1", page = "2"), cappedRecordings.nextPage)
        assertEquals(false, cappedRecordings.fullSyncRequired)
    }

    @Test
    fun calendarPeriodsAreReadByTheDateTheyAreDrawnFrom() = runTest {
        val period = """{"starts_at":"2026-09-15","ends_at":"2026-09-15","kind":"day","recordings":{}}"""
        val year = """{"starts_at":"2026-01-01","ends_at":"2026-12-31","kind":"year","padding_days_count":3,"days":[],"spanned_events":[]}"""
        val hey = mockHey(ok(period), ok("""{"days":[$period]}"""), ok(period), ok("""{"weeks":[]}"""), ok(year))
        val client = hey.client()
        client.calendarPeriods.day("now")
        assertEquals("/calendar/days/now.json", hey.requests[0].path)
        assertEquals(1, client.calendarPeriods.days("").size)
        assertEquals("/calendar/days.json", hey.requests[1].path)
        assertNull(hey.requests[1].query("starts_at"), "an empty date is left off the wire for HEY to pick the default")
        client.calendarPeriods.week("2026-09-15")
        assertEquals("/calendar/weeks/2026-09-15.json", hey.requests[2].path)
        client.calendarPeriods.weeks(centeredAt = "2026-09-15")
        assertEquals("/calendar/weeks.json", hey.requests[3].path)
        assertEquals("2026-09-15", hey.requests[3].query("centered_at"))
        assertNull(hey.requests[3].query("starts_at"))
        client.calendarPeriods.year("2026-01-01")
        assertEquals("/calendar/years/2026-01-01.json", hey.requests[4].path)
    }

    @Test
    fun aTodoIsFiledOnABareDayAndAnEditThatChangesNothingIsRefused() = runTest {
        val hey = mockHey(ok("""{"id":1,"type":"Calendar::Todo"}"""), ok("""{"id":1,"type":"Calendar::Todo"}"""), ok("""{"id":1,"type":"Calendar::Todo"}"""))
        val client = hey.client()
        client.calendarTodos.createTodo("Buy milk", "2026-09-16")
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/calendar/todos.json", hey.requests[0].path)
        val filed = body(hey.requests[0]).getValue("calendar_todo").jsonObject
        assertEquals("Buy milk", filed.getValue("title").jsonPrimitive.content)
        assertEquals("2026-09-16", filed.getValue("starts_at").jsonPrimitive.content)

        client.calendarTodos.createTodo("Buy milk")
        val today = body(hey.requests[1]).getValue("calendar_todo").jsonObject.getValue("starts_at").jsonPrimitive.content
        assertTrue(Regex("\\d{4}-\\d{2}-\\d{2}").matches(today), "no day is today, as a bare date: $today")

        client.calendarTodos.updateTodo(1, TodoChanges(title = "", focused = true))
        assertEquals("PATCH", hey.requests[2].method)
        assertEquals("/calendar/todos/1.json", hey.requests[2].path)
        val changed = body(hey.requests[2]).getValue("calendar_todo").jsonObject
        assertEquals(setOf("focused"), changed.keys, "an empty title is no title, and says nothing")
        assertFailsWith<HeyException.Usage> { client.calendarTodos.updateTodo(1, TodoChanges(title = "")) }
        assertFailsWith<HeyException.Usage> { client.calendarTodos.updateTodo(1, TodoChanges(startsAt = "2026-02-30")) }
        assertFailsWith<HeyException.Usage> { client.calendarTodos.createTodo("Buy milk", "tomorrow") }
        assertEquals(3, hey.requests.size)
    }

    @Test
    fun aHabitIsWrittenInItsPartsWithTheEmptyOnesLeftOff() = runTest {
        val hey = mockHey(ok("""{"id":5,"type":"Calendar::Habit"}"""), ok("""{"id":5,"type":"Calendar::Habit"}"""))
        val client = hey.client()
        val created = client.habits.createHabit(HabitParams(name = "Run", icon = "shoe", color = "green", days = listOf(1, 3, 5)))
        assertEquals(5L, created.id)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/calendar/habits.json", hey.requests[0].path)
        val habit = body(hey.requests[0]).getValue("calendar_habit").jsonObject
        assertEquals("Run", habit.getValue("name").jsonPrimitive.content)
        assertEquals(listOf(1, 3, 5), habit.getValue("days").jsonArray.map { it.jsonPrimitive.content.toInt() })

        client.habits.updateHabit(5, HabitParams(name = "Jog"))
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals("/calendar/habits/5.json", hey.requests[1].path)
        assertEquals(setOf("name"), body(hey.requests[1]).getValue("calendar_habit").jsonObject.keys, "fields left empty are kept by HEY, so they are not sent")
    }
}
