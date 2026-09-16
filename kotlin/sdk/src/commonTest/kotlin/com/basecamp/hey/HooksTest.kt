package com.basecamp.hey

import com.basecamp.hey.generated.*
import com.basecamp.hey.services.UpdateCalendarEventParams
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import com.basecamp.hey.services.DraftContent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeMark
import kotlin.time.TimeSource
import com.basecamp.hey.services.CalendarChangesCursor
import com.basecamp.hey.services.PostingChangesCursor

class HooksTest {
    private class Recording(val log: MutableList<String>, val name: String) : HeyHooks {
        override fun onOperationStart(info: OperationInfo) {
            log += "$name:start:${info.service}.${info.operation}:${info.resourceType}:${info.isMutation}:${info.resourceId}"
        }

        override fun onOperationEnd(info: OperationInfo, result: OperationResult) {
            log += "$name:end:${info.operation}:${describe(result.error)}"
        }

        override fun onRequestStart(info: RequestInfo) {
            log += "$name:request:${info.method}:${info.attempt}"
        }

        override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
            log += "$name:response:${result.statusCode}:${describe(result.error)}"
        }

        /** An SDK error by its code, anything else by its message. */
        private fun describe(error: Throwable?): String? = when (error) {
            null -> null
            is HeyException -> error.code
            else -> error.message
        }
    }

    private fun HeyClientBuilder.record(log: MutableList<String>, engine: HttpClientEngine) {
        this.engine = engine
        hooks = Recording(log, "a")
        timeout = Duration.INFINITE
        maxRetryJitter = Duration.ZERO
    }

    @Test
    fun anOperationAndItsRequestsAreReported() = runTest {
        val hey = mockHey(status(503), ok("""{"id":5,"kind":"imbox","name":"Imbox"}"""), status(404))
        val log = mutableListOf<String>()
        val client = hey.client { hooks = Recording(log, "a") }
        client.boxes.get(5)
        assertEquals(
            listOf(
                "a:start:Boxes.GetBox:box:false:5",
                "a:request:GET:1",
                "a:response:503:api_error",
                "a:request:GET:2",
                "a:response:200:null",
                "a:end:GetBox:null",
            ),
            log,
        )
        log.clear()
        assertFailsWith<HeyException.NotFound> { client.boxes.get(6) }
        assertEquals("a:end:GetBox:not_found", log.last())
        assertEquals("a:response:404:not_found", log[log.size - 2])
    }

    @Test
    fun chainedHooksNestAndAThrowingHookIsIgnored() = runTest {
        val hey = mockHey(ok("[]"))
        val log = mutableListOf<String>()
        val throwing = object : HeyHooks {
            override fun onOperationStart(info: OperationInfo) = throw IllegalStateException("boom")
        }
        hey.client { hooks = chainHooks(Recording(log, "a"), NoopHooks, throwing, Recording(log, "b")) }.boxes.list()
        assertEquals(listOf("a:start", "b:start", "a:request", "b:request", "b:response", "a:response", "b:end", "a:end"), log.map { it.split(':').take(2).joinToString(":") })
        assertNotNull(chainHooks(NoopHooks) as? NoopHooks)
        assertNull((chainHooks(Recording(log, "x")) as? ChainHooks))
    }

    @Test
    fun aTokenProviderThatThrowsStillEndsTheOperation() = runTest {
        val hey = mockHey(ok("[]"))
        val log = mutableListOf<String>()
        val client = HeyClient {
            accessToken(object : TokenProvider {
                override suspend fun accessToken(): String = throw IllegalStateException("vault sealed")
            })
            record(log, hey.engine)
        }
        assertFailsWith<IllegalStateException> { client.boxes.list() }
        assertEquals(listOf("a:start:Boxes.ListBoxes:box:false:null", "a:end:ListBoxes:vault sealed"), log)
        assertEquals(0, hey.requests.size)
    }

    @Test
    fun aRefreshThatThrowsStillEndsTheRequest() = runTest {
        val hey = mockHey(status(401))
        val log = mutableListOf<String>()
        val client = HeyClient {
            accessToken(object : TokenProvider {
                override suspend fun accessToken(): String = "stale"
                override suspend fun refresh(): Boolean = throw IllegalStateException("refresh exploded")
            })
            record(log, hey.engine)
        }
        // What the provider threw reaches the caller as an SDK failure, with it as the cause.
        val error = assertFailsWith<HeyException.Auth> { client.boxes.list() }
        assertEquals("credential refresh failed", error.message)
        assertEquals("refresh exploded", error.cause?.message)
        assertEquals(
            listOf(
                "a:start:Boxes.ListBoxes:box:false:null",
                "a:request:GET:1",
                "a:response:401:auth_required",
                "a:end:ListBoxes:auth_required",
            ),
            log,
        )
    }

    @Test
    fun aCancelledRequestEndsWhatItStarted() = runTest {
        val started = CompletableDeferred<Unit>()
        val stalled = MockEngine {
            started.complete(Unit)
            awaitCancellation()
        }
        val log = mutableListOf<String>()
        val client = HeyClient {
            accessToken("test-token")
            record(log, stalled)
        }
        val job = launch { client.boxes.list() }
        started.await()
        job.cancelAndJoin()
        assertEquals(
            listOf(
                "a:start:Boxes.ListBoxes:box:false:null",
                "a:request:GET:1",
                "a:response:0:network",
                "a:end:ListBoxes:network",
            ),
            log,
        )
    }

    @Test
    fun anAnswerThatWillNotReadEndsTheOperationWithThatError() = runTest {
        val hey = mockHey(
            ok("""{"id":"not a number"}"""),
            ok("""{"summary":"no id or type"}"""),
            ok("<section id=\"container_workflow_stage_1\"></section>", mapOf("Content-Type" to "text/html")),
        )
        val log = mutableListOf<String>()
        val client = hey.client { hooks = Recording(log, "a") }
        assertFailsWith<HeyException.Api> { client.boxes.get(1) }
        assertEquals("a:end:GetBox:api_error", log.last())
        assertEquals(1, log.count { it.startsWith("a:end:") })

        log.clear()
        assertFailsWith<HeyException.Api> { client.calendarEvents.updateEvent(99, UpdateCalendarEventParams(title = "x")) }
        assertEquals("a:end:UpdateCalendarEvent:api_error", log.last(), "a calendar write that answers something other than a recording ends the same way")
        assertEquals(1, log.count { it.startsWith("a:end:") })

        log.clear()
        assertFailsWith<HeyException.NotFound> { client.workflows.stage(8801, 5) }
        assertEquals(listOf("a:start:Workflows.GetWorkflowStage:workflow_stage:false:5", "a:request:GET:1", "a:response:200:null", "a:end:GetWorkflowStage:not_found"), log)
    }

    @Test
    fun aSelectionOfOnePostingNamesItToTheHooks() = runTest {
        val hey = mockHey(ok(""), ok(""), ok(""), ok(""), ok(""), ok(""))
        val log = mutableListOf<String>()
        val client = hey.client { hooks = Recording(log, "a") }
        client.postings.markPostingsSeen(listOf(7))
        client.postings.markPostingsUnseen(listOf(7, 8))
        client.postings.trashPostings(listOf(9))
        client.postings.mutePostings(listOf(10))
        client.postings.moveToBox(3, listOf(11))
        client.postings.moveToBox(3, listOf(11, 12))
        assertEquals(
            listOf(
                "a:start:Postings.MarkPostingsSeen:posting:true:7",
                "a:start:Postings.MarkPostingsUnseen:posting:true:null",
                "a:start:Postings.TrashPostings:posting:true:9",
                "a:start:Postings.MutePostings:posting:true:10",
                "a:start:Postings.MovePostings:posting:true:11",
                "a:start:Postings.MovePostings:posting:true:null",
            ),
            log.filter { it.startsWith("a:start:") },
        )
    }

    @Test
    fun aDraftSaveWhoseLocationNamesNoIdEndsTheOperationWithThatError() = runTest {
        val hey = mockHey(status(204), status(204, headers = mapOf("Location" to "/messages/new")))
        val log = mutableListOf<String>()
        val client = hey.client { hooks = Recording(log, "a") }
        val draft = DraftContent(subject = "s", content = "c", actingSenderId = 100)
        for (attempt in 1..2) {
            log.clear()
            assertFailsWith<HeyException.Api> { client.messages.createDraft(draft) }
            assertEquals("a:end:CreateMessage:api_error", log.last())
            assertEquals(1, log.count { it.startsWith("a:end:") })
        }
    }

    @Test
    fun anOperationOfTwoRequestsEndsOnceBothAreInWithTheErrorTheCallerGets() = runTest {
        val staged = mockHey(ok(""), status(422, """{"error":"no such stage"}"""))
        val log = mutableListOf<String>()
        val client = staged.client { hooks = Recording(log, "a") }
        assertFailsWith<HeyException.Validation> { client.workflows.stageTopic(5, 8801, 99) }
        assertEquals(
            listOf("a:start:Workflows.CreateWorkflowStaging:workflow_staging:true:5", "a:request:POST:1", "a:response:200:null", "a:request:PATCH:1", "a:response:422:validation", "a:end:CreateWorkflowStaging:validation"),
            log,
        )

        val published = mockHey(status(302, headers = mapOf("Location" to "/topics/5/sharing")), status(404, """{"error":"gone"}"""))
        log.clear()
        assertFailsWith<HeyException.NotFound> { published.client { hooks = Recording(log, "a") }.publications.publish(5) }
        assertEquals(1, log.count { it.startsWith("a:start:") })
        assertEquals("a:end:CreateTopicPublication:not_found", log.last())
        assertEquals(listOf("a:response:302:null", "a:response:404:not_found"), log.filter { it.startsWith("a:response:") })
    }

    @Test
    fun anUploadEndsItsOperationOnlyOnceTheBytesAreStored() = runTest {
        val hey = mockHey(
            ok("""{"signed_id":"s","attachable_sgid":"g","direct_upload":{"url":"https://storage.example.com/blobs/abc","headers":{"Content-Type":"text/plain"}}}"""),
            status(403, """{"error":"no"}"""),
        )
        val log = mutableListOf<String>()
        val client = hey.client { hooks = Recording(log, "a") }
        val error = assertFailsWith<HeyException.Forbidden> { client.attachments.upload("a.txt", "text/plain", "x".encodeToByteArray()) }
        assertEquals(403, error.httpStatus)
        assertEquals(1, log.count { it.startsWith("a:start:") })
        assertEquals("a:start:Attachments.CreateDirectUpload:attachment:true:null", log.first(), "the operation is the reservation's, as the model describes it")
        assertEquals(listOf("a:response:200:null", "a:response:403:forbidden"), log.filter { it.startsWith("a:response:") })
        assertEquals("a:end:CreateDirectUpload:forbidden", log.last(), "the operation ends after the put, with the put's failure")
    }

    @Test
    fun aFullSyncAnswerEndsItsOperationWell() = runTest {
        // The 409 is what the caller is handed as an answer, so the hooks hear the request
        // refused and the operation succeed: what a trace or a failure count sees agrees
        // with what the caller got.
        val postings = mockHey(status(409, """{"error":"cursor too old"}"""))
        val log = mutableListOf<String>()
        val stale = postings.client { hooks = Recording(log, "a") }.postings.changes(7, PostingChangesCursor("2026-09-15T10:00:00Z"))
        assertTrue(stale.fullSyncRequired)
        assertEquals(
            listOf("a:start:Postings.GetBoxPostingChanges:posting:false:7", "a:request:GET:1", "a:response:409:conflict", "a:end:GetBoxPostingChanges:null"),
            log,
        )

        val calendars = mockHey(status(409))
        log.clear()
        val behind = calendars.client { hooks = Recording(log, "a") }.calendars.recordingChanges(3, CalendarChangesCursor(since = "2026-09-15T10:00:00Z", version = "1"))
        assertTrue(behind.fullSyncRequired)
        assertEquals(
            listOf("a:start:Calendars.GetCalendarRecordingChanges:recording:false:3", "a:request:GET:1", "a:response:409:conflict", "a:end:GetCalendarRecordingChanges:null"),
            log,
        )

        // Any other refusal still ends the operation with it.
        val gone = mockHey(status(404, """{"error":"gone"}"""))
        log.clear()
        assertFailsWith<HeyException.NotFound> { gone.client { hooks = Recording(log, "a") }.postings.changes(7, PostingChangesCursor("2026-09-15T10:00:00Z")) }
        assertEquals("a:end:GetBoxPostingChanges:not_found", log.last())
    }

    /** A clock that moves only when a test moves it, and never back. */
    private class HandClock : TimeSource {
        var now: Duration = Duration.ZERO

        override fun markNow(): TimeMark = object : TimeMark {
            private val at = now
            override fun elapsedNow(): Duration = now - at
        }
    }

    @Test
    fun durationsAreMeasuredOnTheClientsOwnClock() = runTest {
        val clock = HandClock()
        val durations = mutableListOf<Pair<String, Duration>>()
        val hooks = object : HeyHooks {
            override fun onOperationEnd(info: OperationInfo, result: OperationResult) { durations += "operation" to result.duration }
            override fun onRequestEnd(info: RequestInfo, result: RequestResult) { durations += "request" to result.duration }
        }
        val engine = MockEngine { request ->
            // HEY takes its time answering, on the client's clock and no other.
            clock.now += if (request.url.encodedPath.endsWith("/boxes/5.json")) 1500.milliseconds else 2.seconds
            respond(
                if (request.url.encodedPath.endsWith("/boxes/5.json")) """{"id":5,"kind":"imbox","name":"Imbox"}""" else """{"error":"no"}""",
                if (request.url.encodedPath.endsWith("/boxes/5.json")) HttpStatusCode.OK else HttpStatusCode.Forbidden,
                headersOf(HttpHeaders.ContentType, "application/json"),
            )
        }
        val client = HeyClient {
            accessToken("t")
            this.engine = engine
            this.hooks = hooks
            timeout = Duration.INFINITE
            timeSource = clock
        }
        client.boxes.get(5)
        assertEquals(listOf("request" to 1500.milliseconds, "operation" to 1500.milliseconds), durations)

        durations.clear()
        assertFailsWith<HeyException.Forbidden> { client.boxes.get(6) }
        assertEquals(listOf("request" to 2.seconds, "operation" to 2.seconds), durations, "and a failed operation's duration is measured the same way")

        durations.clear()
        clock.now = Duration.ZERO
        // A clock that does not move — or, for the wall clock, one set back — reports nothing
        // at all rather than a negative.
        val still = HeyClient {
            accessToken("t")
            this.engine = MockEngine { respond("""{"id":5,"kind":"imbox","name":"Imbox"}""", HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json")) }
            this.hooks = hooks
            timeSource = clock
        }
        still.boxes.get(5)
        assertEquals(listOf("request" to Duration.ZERO, "operation" to Duration.ZERO), durations)
    }
}
