package com.basecamp.hey

import com.basecamp.hey.generated.models.MarkPostingsRequestContent
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.services.GetContactOptions
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class RequestTest {
    @Test
    fun aGeneratedReadCarriesTheHeadersAndTheJsonSuffix() = runTest {
        val hey = mockHey(ok("""[{"id":7,"kind":"imbox","name":"Imbox"}]"""))
        val boxes = hey.client().boxes.list()

        val request = hey.requests.single()
        assertEquals("GET", request.method)
        assertEquals("/boxes.json", request.path)
        assertEquals("Bearer test-token", request.header("Authorization"))
        assertEquals("application/json", request.header("Accept"))
        assertEquals(HeyConfig.DEFAULT_USER_AGENT, request.header("User-Agent"))
        assertEquals(listOf(7L), boxes.value.map { it.id })
        assertEquals("Imbox", boxes.value.single().name)
    }

    @Test
    fun pathParametersAreFilledAndEncoded() = runTest {
        val hey = mockHey(ok("""{"id":123,"kind":"imbox","name":"Imbox"}"""), ok("""{"id":1,"type":"Calendar::JournalEntry","date":"2026-03-04"}"""))
        val client = hey.client()
        client.boxes.get(123)
        client.journal.getEntry("2026-03-04")
        assertEquals("/boxes/123.json", hey.requests[0].path)
        assertEquals("/calendar/days/2026-03-04/journal_entry.json", hey.requests[1].path)
        assertEquals("/boxes/a%2Fb", Routes.GET_BOX.fill(listOf("a/b")))
    }

    @Test
    fun anHtmlRouteIsAskedForAsWritten() = runTest {
        val hey = mockHey(ok("<section id=\"container_workflow_stage_5\"><h2>Applied</h2></section>", mapOf("Content-Type" to "text/html")))
        val page = hey.client().workflows.getStage(8801, 5)
        val request = hey.requests.single()
        assertEquals("/workflows/8801/stages/5", request.path)
        assertEquals("text/html", request.header("Accept"))
        assertEquals("<section id=\"container_workflow_stage_5\"><h2>Applied</h2></section>", page)
    }

    @Test
    fun queryParametersGoOutWhenTheyAreSet() = runTest {
        val hey = mockHey(ok("""{"id":88}"""), ok("""{"id":88}"""))
        val client = hey.client()
        client.contacts.get(88, GetContactOptions(page = "older-threads"))
        client.contacts.get(88)
        assertEquals("older-threads", hey.requests[0].query("page"))
        assertNull(hey.requests[1].query("page"))
        assertEquals("/contacts/88.json", hey.requests[1].path)
    }

    @Test
    fun aBodyIsEncodedAsTheModelSaysAndNullsAreLeftOff() = runTest {
        val hey = mockHey(ok(""))
        hey.client().postings.markSeen(MarkPostingsRequestContent(listOf(1, 2)))
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("application/json", request.header("Content-Type"))
        val body = Json.parseToJsonElement(request.body).jsonObject
        assertEquals(listOf(1L, 2L), body.getValue("posting_ids").jsonArray.map { it.jsonPrimitive.content.toLong() })
        assertEquals(setOf("posting_ids"), body.keys)
    }

    @Test
    fun largeIdsKeepTheirPrecision() = runTest {
        val hey = mockHey(ok("""{"id":9007199254740993,"kind":"imbox","name":"Large"}"""))
        val box = hey.client().boxes.get(9007199254740993)
        assertEquals(9007199254740993L, box.value.id)
        assertEquals("/boxes/9007199254740993.json", hey.requests.single().path)
    }

    @Test
    fun aMissingRequiredMemberIsAnApiError() = runTest {
        val hey = mockHey(ok("""{"id":12345,"title":"Imbox"}"""))
        val error = assertFailsWith<HeyException.Api> { hey.client().boxes.get(12345) }
        assertEquals(HeyException.CODE_API, error.code)
        assertEquals(false, error.retryable)
    }

    @Test
    fun anEmptyOnStatusAnswersNullAndOthersStillFail() = runTest {
        val hey = mockHey(status(404, """{"error":"Not found"}"""), ok("""{"id":123,"type":"Calendar::TimeTrack","starts_at":"2026-03-04T10:00:00Z"}"""), status(500, """{"error":"boom"}"""))
        val client = hey.client()
        assertNull(client.timeTracks.getOngoing())
        assertNotNull(client.timeTracks.getOngoing())
        val error = assertFailsWith<HeyException.Api> { client.timeTracks.getOngoing() }
        assertEquals(500, error.httpStatus)
    }

    @Test
    fun aBodyThatWillNotDecodeIsAnApiErrorNamingTheOperation() = runTest {
        val hey = mockHey(ok("""{"id":"not a number"}"""))
        val error = assertFailsWith<HeyException.Api> { hey.client().boxes.get(1) }
        assertEquals(200, error.httpStatus)
        assertEquals(false, error.retryable)
        assertEquals("GetBox: unexpected JSON in the response", error.message)
    }

    @Test
    fun aBodyPastTheCapIsRefused() = runTest {
        val hey = mockHey(ok("[" + "1,".repeat(2000) + "1]"))
        val error = assertFailsWith<HeyException.Api> { hey.client { maxResponseBodyBytes = 100 }.boxes.list() }
        assertEquals(true, error.responseTooLarge)
    }

    @Test
    fun aRawPathGetsTheSameTreatment() = runTest {
        val hey = mockHey(ok("""{"ok":true}"""))
        val client = hey.client()
        val response = client.execute(client.request(Method.GET, "/anything?x=1").query("y", "2"))
        assertEquals("/anything.json", hey.requests.single().path)
        assertEquals("1", hey.requests.single().query("x"))
        assertEquals("2", hey.requests.single().query("y"))
        assertEquals(200, response.status)
    }

    @Test
    fun aRawPathsQueryGoesOutAsWritten() = runTest {
        val hey = mockHey(ok("""{"ok":true}"""))
        val client = hey.client()
        client.execute(client.request(Method.GET, "/search?q=a%26b&plus=1%2B1"))
        val url = hey.requests.single().url
        assertEquals("q=a%26b&plus=1%2B1", url.encodedQuery)
        assertEquals("a&b", url.parameters["q"])
        assertEquals("1+1", url.parameters["plus"])
    }

    @Test
    fun aDocumentIsHeldToTheConfiguredCapWhateverTheAcceptListSays() = runTest {
        assertEquals(true, isParsed("text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"))
        assertEquals(true, isParsed("application/vnd.api+json"))
        assertEquals(true, isParsed(""))
        assertEquals(false, isParsed("image/png"))
        assertEquals(false, isParsed("*/*"))

        val hey = mockHey(ok("<html>" + "x".repeat(2000) + "</html>", mapOf("Content-Type" to "text/html")))
        val client = hey.client { maxResponseBodyBytes = 100 }
        val error = assertFailsWith<HeyException.Api> { client.sendForm(client.form(Method.GET, "/workflows/new")) }
        assertEquals(true, error.responseTooLarge)
    }

    @Test
    fun aRecordingIsRecognisedByEitherSpellingOfItsType() {
        val direct = heyJson.decodeFromString(Recording.serializer(), """{"id":1,"type":"CalendarEvent"}""")
        val namespaced = heyJson.decodeFromString(Recording.serializer(), """{"id":1,"type":"Calendar::Event"}""")
        assertEquals(true, direct.isCalendarEvent)
        assertEquals(true, namespaced.isCalendarEvent)
        assertEquals(false, direct.isCalendarTodo)
        assertEquals(true, heyJson.decodeFromString(Recording.serializer(), """{"id":2,"type":"CalendarTodo"}""").isCalendarTodo)
    }

    @Test
    fun aStatusTakenForNothingThereIsNotReadPastTheCap() = runTest {
        val big = "x".repeat(2000)
        val hey = mockHey(status(404, """{"error":"$big"}"""), status(302, """<html>$big</html>""", mapOf("Location" to "/workflows/7")))
        val client = hey.client { maxResponseBodyBytes = 100 }
        assertNull(client.timeTracks.getOngoing(), "an oversized 404 the route takes for nothing there is still nothing there")
        val answer = client.sendForm(client.form(Method.POST, "/workflows"))
        assertEquals("/workflows/7", answer.location, "and an oversized redirect a form takes for its answer still answers")
    }
}
