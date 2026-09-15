package com.basecamp.hey.conformance

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import java.net.InetSocketAddress
import java.net.URLDecoder

/** What the mock server saw and answered, for the assertions to read afterwards. */
class Recorded {
    var count: Int = 0
    val times = mutableListOf<Long>()
    val paths = mutableListOf<String>()
    val methods = mutableListOf<String>()
    val queries = mutableListOf<List<Pair<String, String>>>()
    val bodies = mutableListOf<ByteArray>()
    val headers = mutableListOf<Map<String, List<String>>>()
    val statuses = mutableListOf<Int>()
    val links = mutableListOf<String?>()
    var served: Int = 0

    fun header(index: Int, name: String): String? =
        headers.getOrNull(index)?.entries?.firstOrNull { it.key.equals(name, ignoreCase = true) }?.value?.firstOrNull()
}

private const val NO_MORE_RESPONSES = """{"error": "No more mock responses"}"""

/**
 * A loopback server that answers each request with the case's next mock response, in order.
 * A request to `/` is refused and not recorded: HEY has no root operation.
 */
class MockServer private constructor(private val server: HttpServer, private val responses: List<MockResponse>) {
    val recorded = Recorded()
    val baseUrl: String get() = "http://127.0.0.1:${server.address.port}"

    fun shutdown(): Recorded {
        server.stop(0)
        return recorded
    }

    private fun answer(exchange: HttpExchange) {
        exchange.use {
            if (exchange.requestURI.rawPath == "/") {
                val body = "404 page not found".toByteArray()
                exchange.sendResponseHeaders(404, body.size.toLong())
                exchange.responseBody.write(body)
                return
            }
            val bytes = exchange.requestBody.readBytes()
            val index = synchronized(recorded) {
                recorded.count += 1
                recorded.times += System.nanoTime()
                recorded.paths += exchange.requestURI.rawPath
                recorded.methods += exchange.requestMethod
                recorded.queries += queryPairs(exchange.requestURI.rawQuery)
                recorded.bodies += bytes
                recorded.headers += exchange.requestHeaders.mapValues { it.value.toList() }
                recorded.served++
            }
            val mock = responses.getOrNull(index)
            if (mock == null) {
                val body = NO_MORE_RESPONSES.toByteArray()
                exchange.responseHeaders.add("Content-Type", "application/json")
                exchange.sendResponseHeaders(500, body.size.toLong())
                exchange.responseBody.write(body)
                return
            }
            if (mock.delay > 0) Thread.sleep(mock.delay)
            synchronized(recorded) {
                recorded.statuses += mock.status
                recorded.links += mock.headers.entries.firstOrNull { it.key.equals("link", ignoreCase = true) }?.value
            }
            for ((name, value) in mock.headers) exchange.responseHeaders.add(name, value)
            if (mock.contentType == null) exchange.responseHeaders.add("Content-Type", "application/json")
            val body = when {
                mock.body == null -> ByteArray(0)
                mock.body is JsonPrimitive && mock.body.isString && mock.servesHtml -> mock.body.content.toByteArray()
                else -> Json.encodeToString(kotlinx.serialization.json.JsonElement.serializer(), mock.body).toByteArray()
            }
            // A 304 or 204 carries no body; the JDK server refuses a length for them.
            val length = if (mock.status == 304 || mock.status == 204) -1L else if (body.isEmpty()) -1L else body.size.toLong()
            exchange.sendResponseHeaders(mock.status, length)
            if (length > 0) exchange.responseBody.write(body)
        }
    }

    companion object {
        fun start(responses: List<MockResponse>): MockServer {
            val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
            val mock = MockServer(server, responses)
            server.createContext("/") { exchange ->
                try {
                    mock.answer(exchange)
                } catch (error: Exception) {
                    System.err.println("mock server: $error")
                }
            }
            server.start()
            return mock
        }
    }
}

fun queryPairs(query: String?): List<Pair<String, String>> {
    if (query.isNullOrEmpty()) return emptyList()
    return query.split('&').filter { it.isNotEmpty() }.map { part ->
        val name = part.substringBefore('=')
        val value = if (part.contains('=')) part.substringAfter('=') else ""
        URLDecoder.decode(name, "UTF-8") to URLDecoder.decode(value, "UTF-8")
    }
}
