package com.basecamp.hey

import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.toByteArray
import io.ktor.http.Headers
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.Url
import io.ktor.http.headers
import kotlin.time.Duration

/** One canned answer the mock serves, in order. */
internal class Answer(
    val status: Int,
    val body: String? = null,
    val headers: Map<String, String> = emptyMap(),
    val failure: Exception? = null,
)

internal class RecordedRequest(val method: String, val url: Url, val headers: Headers, val body: String, private val contentType: String?) {
    val path: String get() = url.encodedPath
    fun query(name: String): String? = url.parameters[name]

    /** Ktor carries a body's content type on the body rather than in the headers, so it is read from either. */
    fun header(name: String): String? =
        headers[name] ?: contentType?.takeIf { name.equals(HttpHeaders.ContentType, ignoreCase = true) }
}

/** A mock HEY that answers each request with the next canned answer and remembers what it saw. */
internal class MockHey(private val answers: List<Answer>) {
    val requests = mutableListOf<RecordedRequest>()

    val engine = MockEngine { request ->
        val body = request.body.toByteArray().decodeToString()
        requests += RecordedRequest(request.method.value, request.url, request.headers, body, request.body.contentType?.toString())
        val answer = answers.getOrNull(requests.size - 1) ?: Answer(500, """{"error":"No more mock responses"}""")
        answer.failure?.let { throw it }
        respond(
            content = answer.body ?: "",
            status = HttpStatusCode.fromValue(answer.status),
            headers = headers {
                if (answer.headers.keys.none { it.equals(HttpHeaders.ContentType, ignoreCase = true) }) {
                    append(HttpHeaders.ContentType, "application/json")
                }
                for ((name, value) in answer.headers) append(name, value)
            },
        )
    }

    fun client(configure: HeyClientBuilder.() -> Unit = {}): HeyClient = HeyClient {
        accessToken("test-token")
        engine = this@MockHey.engine
        timeout = Duration.INFINITE
        maxRetryJitter = Duration.ZERO
        configure()
    }
}

internal fun mockHey(vararg answers: Answer): MockHey = MockHey(answers.toList())

internal fun ok(body: String = "{}", headers: Map<String, String> = emptyMap()): Answer = Answer(200, body, headers)

internal fun status(code: Int, body: String? = null, headers: Map<String, String> = emptyMap()): Answer = Answer(code, body, headers)

internal const val IDENTITY = """{"id":1,"primary_contact":{"id":9},"accounts":[{"id":42,"status":"active"},{"id":43,"status":"inactive","purpose":"personal"}],"senders":[{"id":100,"account_id":42,"default":true},{"id":101,"account_id":42},{"id":200,"account_id":7,"default":true}],"all_users":[{"id":1000,"account_id":42},{"id":7000,"account_id":7}]}"""
