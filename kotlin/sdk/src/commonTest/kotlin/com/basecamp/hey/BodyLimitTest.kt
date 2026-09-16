package com.basecamp.hey

import com.basecamp.hey.generated.*
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import io.ktor.utils.io.ByteChannel
import io.ktor.utils.io.writeFully
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlin.concurrent.Volatile
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

class BodyLimitTest {
    @Volatile
    private var produced = 0L

    /**
     * The producer and the read both run on real threads rather than the test scheduler, so
     * the only thing between them is the channel's own backpressure: the producer can get no
     * further ahead of the SDK than the channel's buffer allows, and how far it got is how
     * much the SDK let through.
     */
    @Test
    fun aBodyPastTheCapIsRefusedWhileItIsStillArriving() = runTest(timeout = 30.seconds) {
        val cap = 100 * 1024
        val chunk = ByteArray(64 * 1024) { '1'.code.toByte() }
        val total = 64L * 1024 * 1024
        val channel = ByteChannel(autoFlush = true)
        val producers = CoroutineScope(Dispatchers.Default + SupervisorJob())
        producers.launch {
            try {
                while (produced < total) {
                    channel.writeFully(chunk)
                    produced += chunk.size
                }
                channel.flushAndClose()
            } catch (_: Throwable) {
                // The reader went away: the SDK stopped consuming, which is what the test is for.
            }
        }
        val engine = MockEngine {
            respond(channel, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
        }
        val client = HeyClient {
            accessToken("test-token")
            this.engine = engine
            timeout = Duration.INFINITE
            maxResponseBodyBytes = cap
        }
        try {
            val error = withContext(Dispatchers.Default) { assertFailsWith<HeyException.Api> { client.boxes.list() } }
            assertEquals(true, error.responseTooLarge)
            assertTrue(produced < 8L * 1024 * 1024, "the SDK read on well past its cap: $produced bytes were produced of $total")
        } finally {
            producers.cancel()
            channel.cancel(null)
            client.close()
        }
    }

    @Test
    fun aBodyExactlyAtTheCapIsRead() = runTest {
        val cap = 100 * 1024
        val body = "[" + "1,".repeat((cap - 4) / 2) + "1]"
        val exact = body.padEnd(cap, ' ')
        assertEquals(cap, exact.length)
        val hey = mockHey(ok(exact))
        val client = hey.client { maxResponseBodyBytes = cap }
        val response = client.execute(client.request(Method.GET, "/big"))
        assertEquals(cap, response.body.size)

        val over = mockHey(ok(exact + " "))
        val error = assertFailsWith<HeyException.Api> { over.client { maxResponseBodyBytes = cap }.execute(over.client { maxResponseBodyBytes = cap }.request(Method.GET, "/big")) }
        assertEquals(true, error.responseTooLarge)
    }
}
