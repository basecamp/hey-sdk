package com.basecamp.hey

import com.basecamp.hey.generated.models.CreateMessageRequestContent
import com.basecamp.hey.generated.models.MessagePayload
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import java.io.IOException
import com.basecamp.hey.generated.Routes
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.milliseconds

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class RetryTest {
    private val message = CreateMessageRequestContent(actingSenderId = 1, message = MessagePayload("Hello", "World"))

    @Test
    fun aReadIsResentOnTheStatusesItsPolicyNames() = runTest {
        val hey = mockHey(status(503), status(503), ok("[]"))
        val retries = mutableListOf<Int>()
        val client = hey.client {
            hooks = object : HeyHooks {
                override fun onRetry(info: RequestInfo, attempt: Int, error: Throwable, delayMs: Long) {
                    retries += attempt
                }
            }
        }
        client.boxes.list()
        assertEquals(3, hey.requests.size)
        assertEquals(listOf(2, 3), retries)
        assertTrue(testScheduler.currentTime >= 3000, "waited ${testScheduler.currentTime}ms for 1s then 2s of backoff")
    }

    @Test
    fun aMutationIsSentOnce() = runTest {
        val hey = mockHey(status(503))
        val error = assertFailsWith<HeyException.Api> { hey.client().messages.create(message) }
        assertEquals(1, hey.requests.size)
        assertEquals(503, error.httpStatus)
        assertTrue(error.retryable)
    }

    @Test
    fun aPutTheModelCallsNotIdempotentIsSentOnce() = runTest {
        val hey = mockHey(status(503))
        assertFailsWith<HeyException.Api> { hey.client().messages.update(456, message) }
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun aPostTheModelCallsIdempotentIsResent() = runTest {
        val hey = mockHey(status(503), ok("""{"id":12345,"type":"Calendar::Todo"}"""))
        hey.client().calendarTodos.complete(12345)
        assertEquals(2, hey.requests.size)
    }

    @Test
    fun theClientCeilingOnlyLowersThePolicy() = runTest {
        val hey = mockHey(status(503), status(503), ok("[]"))
        val error = assertFailsWith<HeyException.Api> { hey.client { maxRetries = 1 }.boxes.list() }
        assertEquals(2, hey.requests.size)
        assertEquals(503, error.httpStatus)

        val raised = mockHey(status(503), status(503), status(503), ok(""))
        assertFailsWith<HeyException.Api> { raised.client { maxRetries = 5 }.extenzions.delete(1, 2) }
        assertEquals(2, raised.requests.size, "DeleteExtenzion's policy allows two sends however high the client's ceiling")
    }

    @Test
    fun aStatusThePolicyDoesNotNameIsNotResent() = runTest {
        val hey = mockHey(status(502), ok("[]"))
        val error = assertFailsWith<HeyException.Api> { hey.client().boxes.list() }
        assertEquals(1, hey.requests.size)
        assertEquals(502, error.httpStatus)
    }

    @Test
    fun aRetryAfterOnA429IsHonouredAsGiven() = runTest {
        val hey = mockHey(status(429, headers = mapOf("Retry-After" to "2")), ok("""{"id":1,"kind":"imbox","name":"Imbox"}"""))
        hey.client().boxes.get(1)
        assertEquals(2, hey.requests.size)
        assertTrue(testScheduler.currentTime >= 2000)
    }

    @Test
    fun theRateLimitErrorSurvivesTheBudget() = runTest {
        val hey = mockHey(status(429), status(429), status(429), status(429))
        val error = assertFailsWith<HeyException.RateLimit> { hey.client().boxes.get(1) }
        assertEquals(3, hey.requests.size)
        assertEquals(429, error.httpStatus)
        assertTrue(error.retryable)
    }

    @Test
    fun thePolicyBaseDelayHoldsUnderAClientAskingForLess() = runTest {
        val hey = mockHey(status(503), ok("[]"))
        hey.client { baseRetryDelay = 1.milliseconds }.boxes.list()
        assertTrue(testScheduler.currentTime >= 1000)
    }

    @Test
    fun aClientBaseDelayLongerThanThePolicyIsUsed() = runTest {
        val hey = mockHey(status(503), ok("[]"))
        hey.client { baseRetryDelay = 5000.milliseconds }.boxes.list()
        assertTrue(testScheduler.currentTime >= 5000)
    }

    @Test
    fun aNetworkFailureIsResentForAnIdempotentOperation() = runTest {
        val hey = mockHey(Answer(0, failure = IOException("connection reset")), ok("[]"))
        hey.client().boxes.list()
        assertEquals(2, hey.requests.size)

        val mutation = mockHey(Answer(0, failure = IOException("connection reset")))
        val error = assertFailsWith<HeyException.Network> { mutation.client().messages.create(message) }
        assertEquals(1, mutation.requests.size)
        assertEquals("connection reset", error.hint)
        assertTrue(error.retryable)
    }

    @Test
    fun aClientErrorIsNotResent() = runTest {
        val hey = mockHey(status(404, """{"error":"Not found"}"""))
        assertFailsWith<HeyException.NotFound> { hey.client().boxes.get(99999) }
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun aTimeoutIsResentLikeAnyOtherFailureToGetAnAnswer() = runTest {
        val timeout = io.ktor.client.plugins.HttpRequestTimeoutException("https://app.hey.com/boxes.json", 30_000L)
        val hey = mockHey(Answer(0, failure = timeout), ok("[]"))
        hey.client().boxes.list()
        assertEquals(2, hey.requests.size)

        val exhausted = mockHey(Answer(0, failure = timeout), Answer(0, failure = timeout), Answer(0, failure = timeout))
        val error = assertFailsWith<HeyException.Network> { exhausted.client().boxes.list() }
        assertTrue(error.retryable, "still retryable once the budget is spent: the caller may try again")
        assertEquals(3, exhausted.requests.size)
    }

    @Test
    fun aPatchTheModelCallsIdempotentIsResent() = runTest {
        val hey = mockHey(status(503), ok(""))
        val client = hey.client()
        client.sendUnit(client.operation(Routes.UPDATE_STICKY, listOf(1)).jsonBody("{}"))
        assertEquals(2, hey.requests.size, "UpdateSticky is a PATCH the model calls idempotent")
        assertEquals(true, Routes.UPDATE_STICKY.idempotent)
        assertEquals(false, Routes.UPDATE_MESSAGE.idempotent, "and UpdateMessage's override still stands")
    }

    @Test
    fun aRetryCeilingAsHighAsAnIntHoldsDoesNotWrap() = runTest {
        val hey = mockHey(status(503), ok("[]"))
        hey.client { maxRetries = Int.MAX_VALUE }.boxes.list()
        assertEquals(2, hey.requests.size)
    }
}
