package com.basecamp.hey

import com.basecamp.hey.generated.models.CreateMessageRequestContent
import com.basecamp.hey.generated.models.MessagePayload
import io.ktor.http.headersOf
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import java.io.IOException
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

    @Test
    fun aBodyThatWillNotDecodeIsNeverQuotedBack() = runTest {
        val hey = mockHey(ok("""{"email_address":"distinctive@example.com","id":}"""), ok("""{"email_address":"distinctive@example.com","id":"x"}"""))
        val client = hey.client()
        for (attempt in 1..2) {
            val error = assertFailsWith<HeyException.Api> { client.contacts.get(1) }
            val everything = listOf(error.toString(), error.message, error.hint, error.stackTraceToString()) +
                generateSequence(error.cause) { it.cause }.map { it.toString() }
            assertEquals(false, everything.any { it?.contains("distinctive") == true }, "the address leaked: $everything")
            assertNull(error.cause, "the decoder's own exception quotes the body, so it is not kept")
        }
        assertEquals("body does not decode: missing required field 'kind', at path \$", decodeHint("Field 'kind' is required for type 'Box', but it was missing at path: \$"))
        assertEquals("body does not decode: at path \$.id, at offset 42", decodeHint("Unexpected JSON token at offset 42: Expected quotation mark '\"', but had '}' instead at path: \$.id\nJSON input: {\"secret\":1}"))
        assertEquals("body does not decode", decodeHint(null))
        assertEquals(
            "body does not decode",
            decodeHint("Unexpected JSON token at path: \$.entries['distinctive@example.com']\nJSON input: {\"entries\":{\"distinctive@example.com\":1}}"),
            "a map key in the path is the body's, so the path is left out",
        )
        assertEquals("body does not decode: at path \$.entries[0].id", decodeHint("Unexpected JSON token at path: \$.entries[0].id"))
        assertEquals("body does not decode: at path \$.id", decodeHint("boom at path: \$.id\nJSON input: Field 'distinctive@example.com' is required"))

        val echoing = mockHey(ok("""{"id":"bad","email_address":"Field 'distinctive@example.com' is required for type 'Contact', but it was missing at path: ${'$'}.x"}"""))
        val error = assertFailsWith<HeyException.Api> { echoing.client().contacts.get(1) }
        assertEquals(false, (error.toString() + error.hint + error.stackTraceToString()).contains("distinctive"), error.hint)
    }

    @Test
    fun aFailureOnAHopNeverQuotesWhereTheHopWent() = runTest {
        val secret = "sig=distinctive-secret"
        val hey = mockHey(
            status(302, headers = mapOf("Location" to "https://files.example.com/export.json?$secret")),
            Answer(0, failure = IOException("connect to https://files.example.com/export.json?$secret failed")),
        )
        val seen = mutableListOf<Throwable>()
        val client = hey.client {
            enableRetry = false
            hooks = object : HeyHooks {
                override fun onRequestEnd(info: RequestInfo, result: RequestResult) { result.error?.let { seen += it } }
                override fun onOperationEnd(info: OperationInfo, result: OperationResult) { result.error?.let { seen += it } }
            }
        }
        val error = assertFailsWith<HeyException.Network> { client.boxes.list() }
        val everything = listOf(error.toString(), error.message, error.hint, error.stackTraceToString()) +
            generateSequence(error.cause) { it.cause }.map { it.toString() } + seen.map { it.toString() + (it as? HeyException)?.hint }
        assertEquals(false, everything.any { it?.contains("distinctive") == true }, "the signed target leaked: $everything")
        assertEquals("connect to https://files.example.com failed", error.hint)

        val downgrade = mockHey(status(302, headers = mapOf("Location" to "http://app.hey.com/export.json?$secret")))
        val refused = assertFailsWith<HeyException.Usage> { downgrade.client().boxes.list() }
        assertEquals("http://app.hey.com must use HTTPS", refused.message)
    }

    @Test
    fun aUrlInATransportMessageIsCutBackToItsOrigin() {
        assertEquals("connect to https://host:8443 failed", redactUrls("connect to https://host:8443/path?sig=1 failed"))
        assertEquals("https://files.example.com timed out", redactUrls("https://user:pw@files.example.com/x?sig=1 timed out"))
        assertEquals("https://[::1]:8443", redactUrls("https://[::1]:8443/x#frag"))
        assertEquals("[url=https://a request_timeout=30000 ms]", redactUrls("[url=https://a/b?c, request_timeout=30000 ms]"))
        assertEquals("no url here", redactUrls("no url here"))
        for (text in listOf("connect to https://host/path?safe=x,token=distinctive failed", "https://host/(a)b=distinctive failed", "see https://host/x?a=1]&t=distinctive.")) {
            val redacted = redactUrls(text)
            assertEquals(false, redacted.contains("distinctive"), "$text -> $redacted")
            assertEquals(true, redacted.startsWith("connect to https://host") || redacted.startsWith("https://host") || redacted.startsWith("see https://host"), redacted)
        }
    }

    @Test
    fun anOversizedErrorBodyKeepsTheErrorItsStatusMeans() = runTest {
        val big = """{"errors":["${"x".repeat(2000)}"]}"""
        val hey = mockHey(
            status(422, big, mapOf("X-Request-Id" to "req-422")),
            status(429, big, mapOf("Retry-After" to "3")),
            status(404, big),
            status(500, big),
        )
        val client = hey.client { maxResponseBodyBytes = 100; enableRetry = false }
        val validation = assertFailsWith<HeyException.Validation> { client.contacts.get(1) }
        assertEquals(422, validation.httpStatus)
        assertEquals(HeyException.CODE_VALIDATION, validation.code)
        assertEquals("req-422", validation.requestId)
        assertNull(validation.body, "the body the client refused is not kept")
        assertEquals(true, validation.responseTooLarge)
        assertEquals("response body exceeds 100 bytes", validation.hint)
        val limited = assertFailsWith<HeyException.RateLimit> { client.contacts.get(1) }
        assertEquals(3L, limited.retryAfterSeconds)
        assertFailsWith<HeyException.NotFound> { client.contacts.get(1) }
        val api = assertFailsWith<HeyException.Api> { client.contacts.get(1) }
        assertEquals(500, api.httpStatus)
        assertEquals(true, api.responseTooLarge)
    }

    @Test
    fun a400IsAnApiErrorAsItIsInTheOtherSdks() = runTest {
        val hey = mockHey(status(400, """{"error":"unparsable timestamp"}"""))
        val error = assertFailsWith<HeyException.Api> { hey.client().boxes.list() }
        assertEquals(400, error.httpStatus)
        assertEquals(HeyException.CODE_API, error.code)
        assertEquals(false, error.retryable)
        assertEquals("unparsable timestamp", error.hint)
    }
}
