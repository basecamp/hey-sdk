package com.basecamp.hey

import com.basecamp.hey.generated.models.CreateMessageRequestContent
import com.basecamp.hey.generated.models.MessagePayload
import io.ktor.http.headersOf
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ErrorMappingTest {
    private fun map(status: Int, body: String = "", method: Method = Method.GET, vararg headers: Pair<String, String>): HeyException =
        HeyException.fromResponse(status, method, headersOf(*headers.map { it.first to listOf(it.second) }.toTypedArray()), body.toByteArray())

    @Test
    fun statusesMapToTheSharedVocabulary() {
        val auth = map(401, """{"error":"Unauthorized"}""")
        assertIs<HeyException.Auth>(auth)
        assertEquals(HeyException.CODE_AUTH, auth.code)
        assertEquals(401, auth.httpStatus)
        assertEquals(false, auth.retryable)
        assertEquals("Unauthorized", auth.hint)
        assertEquals(3, auth.exitCode)

        assertIs<HeyException.Forbidden>(map(403))
        val scope = map(403, method = Method.POST)
        assertEquals("Access denied: insufficient scope", scope.message)
        assertIs<HeyException.NotFound>(map(404))
        assertIs<HeyException.Conflict>(map(409))
        assertEquals(9, map(409).exitCode)
        val validation = map(422, """{"error":"Subject can't be blank"}""")
        assertIs<HeyException.Validation>(validation)
        assertEquals("Subject can't be blank", validation.hint)
        val limited = map(429, headers = arrayOf("Retry-After" to "5"))
        assertIs<HeyException.RateLimit>(limited)
        assertEquals(5L, limited.retryAfterSeconds)
        assertTrue(limited.retryable)
        val server = map(500, """{"error":"Internal server error"}""")
        assertIs<HeyException.Api>(server)
        assertTrue(server.retryable)
        assertEquals("API error: 500", server.message)
        assertEquals(false, map(400).retryable)
        assertEquals(7, map(503).exitCode)
    }

    @Test
    fun theRequestIdAndTheBodyAreKept() {
        val error = map(404, """{"error":"Not found"}""", headers = arrayOf("X-Request-Id" to "req-123"))
        assertEquals("req-123", error.requestId)
        assertEquals("""{"error":"Not found"}""", error.bodyText())
    }

    @Test
    fun anHtmlPageIsNeverEchoedAndAListOfErrorsIsJoined() {
        assertNull(map(500, "<html><body>Oops</body></html>").hint)
        assertEquals("a; b", map(422, """{"errors":["a","b"]}""").hint)
        assertEquals("fallback", map(422, """{"message":"fallback"}""").hint)
        assertNull(map(422, """{"error":{"nested":true}}""").hint)
        val long = map(422, """{"error":"${"x".repeat(600)}"}""").hint!!
        assertEquals(500, long.length)
        assertTrue(long.endsWith("..."))
    }

    @Test
    fun theBodyIsBounded() {
        val error = map(500, "x".repeat(HeyException.MAX_ERROR_BODY_BYTES + 10))
        assertEquals(HeyException.MAX_ERROR_BODY_BYTES, error.body!!.size)
    }

    @Test
    fun retryAfterIsReadAsSecondsOrAsADate() {
        assertEquals(2L, retryAfterSeconds("2"))
        assertEquals(0L, retryAfterSeconds("0"))
        assertNull(retryAfterSeconds(null))
        assertNull(retryAfterSeconds("soon"))
        val future = java.time.ZonedDateTime.now(java.time.ZoneOffset.UTC).plusSeconds(90)
        val seconds = retryAfterSeconds(java.time.format.DateTimeFormatter.RFC_1123_DATE_TIME.format(future))!!
        assertTrue(seconds in 85..91, "$seconds")
        assertEquals(0L, retryAfterSeconds("Sun, 06 Nov 1994 08:49:37 GMT"))
    }

    @Test
    fun aFailedCallCarriesItAll() = runTest {
        val hey = mockHey(status(422, """{"error":"Subject can't be blank"}""", mapOf("X-Request-Id" to "req-draft-422")))
        val error = assertFailsWith<HeyException.Validation> { hey.client().messages.create(CreateMessageRequestContent(actingSenderId = 1, message = MessagePayload("", "Final body"))) }
        assertEquals(422, error.httpStatus)
        assertEquals("req-draft-422", error.requestId)
        assertEquals(false, error.retryable)
        assertEquals("Validation: validation error: Subject can't be blank", error.toString())
    }
}
