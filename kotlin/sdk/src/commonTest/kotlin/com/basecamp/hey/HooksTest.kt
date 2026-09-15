package com.basecamp.hey

import com.basecamp.hey.generated.*
import com.basecamp.hey.services.UpdateCalendarEventParams
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.mock.MockEngine
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.time.Duration

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
        assertFailsWith<IllegalStateException> { client.boxes.list() }
        assertEquals(
            listOf(
                "a:start:Boxes.ListBoxes:box:false:null",
                "a:request:GET:1",
                "a:response:401:refresh exploded",
                "a:end:ListBoxes:refresh exploded",
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
}
