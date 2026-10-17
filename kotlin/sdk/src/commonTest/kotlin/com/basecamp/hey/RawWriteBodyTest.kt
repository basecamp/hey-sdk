package com.basecamp.hey

import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import io.ktor.utils.io.ByteChannel
import io.ktor.utils.io.writeFully
import kotlinx.coroutines.test.runTest
import kotlin.concurrent.atomics.AtomicInt
import kotlin.concurrent.atomics.ExperimentalAtomicApi
import kotlin.concurrent.atomics.incrementAndFetch
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertSame
import kotlin.test.assertTrue
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds

/**
 * A raw write HEY answered a success has taken effect, so a body lost after the status must
 * not fail the call: a caller told a send failed would send it again.
 */
@OptIn(ExperimentalAtomicApi::class)
class RawWriteBodyTest {
    /** A HEY that answers [status], sends the start of a JSON body and then drops the connection mid-body. Counts the requests it sees. */
    private class BrokenAnswer(private val status: Int = 200) {
        val requests = AtomicInt(0)

        val engine = MockEngine {
            requests.incrementAndFetch()
            val channel = ByteChannel(autoFlush = true)
            channel.writeFully("""{"id":2201,"topic_id":""".encodeToByteArray())
            channel.cancel(RuntimeException("connection reset by peer"))
            respond(channel, HttpStatusCode.fromValue(status), headersOf(HttpHeaders.ContentType, "application/json"))
        }

        fun client(): HeyClient = HeyClient {
            accessToken("test-token")
            engine = this@BrokenAnswer.engine
            timeout = Duration.INFINITE
            maxRetries = 2
            baseRetryDelay = 1.milliseconds
            maxRetryJitter = Duration.ZERO
        }
    }

    @Test
    fun aRawWriteWhoseSuccessBodyBreaksMidReadIsTheSuccessItWasAndIsSentOnce() = runTest {
        for (method in listOf(Method.POST, Method.PUT, Method.PATCH, Method.DELETE)) {
            for (status in listOf(200, 201)) {
                val hey = BrokenAnswer(status)
                val client = hey.client()
                try {
                    val response = client.execute(client.request(method, "/messages/1").jsonBody("""{"subject":"Lunch"}"""))
                    assertEquals(status, response.status, "$method $status")
                    assertEquals(0, response.body.size, "$method $status")
                    val error = assertNotNull(response.bodyError, "$method $status: the read failure is attached")
                    assertTrue(error is HeyException.Network, "$method $status: $error")
                    assertEquals(1, hey.requests.load(), "$method $status: a write that took effect is never resent")
                    assertSame(error, assertFailsWith<HeyException> { response.json<Map<String, Int>>() })
                } finally {
                    client.close()
                }
            }
        }
    }

    @Test
    fun sendUnitSucceedsAndSendThrowsTheBodyErrorForABrokenRawWrite() = runTest {
        val hey = BrokenAnswer()
        val client = hey.client()
        try {
            client.sendUnit(client.request(Method.POST, "/messages").jsonBody("{}"))
            val error = assertFailsWith<HeyException.Network> { client.send<Map<String, Int>>(client.request(Method.POST, "/messages").jsonBody("{}")) }
            assertTrue(error.message!!.contains("Network error"), error.message)
            assertFailsWith<HeyException.Network> { client.sendText(client.request(Method.PUT, "/messages/1").jsonBody("{}")) }
            assertEquals(3, hey.requests.load(), "each write is sent once")
        } finally {
            client.close()
        }
    }

    @Test
    fun aRawFormWriteWhoseSuccessBodyBreaksIsTheSuccessItWas() = runTest {
        val hey = BrokenAnswer()
        val client = hey.client()
        try {
            val answer = client.sendForm(client.form(Method.POST, "/entries.json").form(listOf("subject" to "Lunch")))
            assertEquals(200, answer.status)
            assertEquals("", answer.body)
            assertNotNull(answer.bodyError)
            assertEquals(1, hey.requests.load())
        } finally {
            client.close()
        }
    }

    @Test
    fun aRawGetWhoseBodyBreaksStillFails() = runTest {
        val hey = BrokenAnswer()
        val client = hey.client()
        try {
            assertFailsWith<HeyException.Network> { client.execute(client.request(Method.GET, "/topics/1")) }
        } finally {
            client.close()
        }
    }

    @Test
    fun aRawWriteSuccessPastTheBodyLimitIsTheSuccessItWas() = runTest {
        val hey = mockHey(ok("""{"id":2201,"topic_id":880,"notice":"Message sent"}"""))
        val client = hey.client { maxResponseBodyBytes = 10 }
        try {
            val response = client.execute(client.request(Method.POST, "/messages").jsonBody("{}"))
            assertEquals(200, response.status)
            val error = assertNotNull(response.bodyError)
            assertTrue(error is HeyException.Api && error.responseTooLarge, "$error")
            assertEquals(1, hey.requests.size)
        } finally {
            client.close()
        }
    }

    @Test
    fun aReadableRawWriteCarriesNoBodyError() = runTest {
        val hey = mockHey(ok("""{"id":2201}"""))
        val client = hey.client()
        try {
            val response = client.execute(client.request(Method.POST, "/messages").jsonBody("{}"))
            assertNull(response.bodyError)
            assertEquals(mapOf("id" to 2201), response.json<Map<String, Int>>())
        } finally {
            client.close()
        }
    }
}
