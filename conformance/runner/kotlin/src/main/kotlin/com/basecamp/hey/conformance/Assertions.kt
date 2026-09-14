package com.basecamp.hey.conformance

import com.basecamp.hey.HeyException
import com.basecamp.hey.nextLink
import com.basecamp.hey.generated.Routes
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.longOrNull
import java.net.URI

class AssertionFailure(message: String) : RuntimeException(message)

/** One case after it ran: what the SDK answered and what the mock server saw. */
class Run(
    val case: TestCase,
    val outcome: Result<Outcome>,
    val recorded: Recorded,
    val baseUrl: String,
)

fun checkAll(run: Run) {
    for (assertion in run.case.assertions) check(run, assertion)
}

private fun fail(message: String): Nothing = throw AssertionFailure(message)

private fun check(run: Run, assertion: Assertion) {
    when (assertion.type) {
        "requestCount" -> checkRequestCount(run, assertion)
        "delayBetweenRequests" -> checkDelayBetweenRequests(run, assertion)
        "noError" -> checkNoError(run)
        "errorCode" -> checkErrorCode(run, assertion)
        "errorField" -> checkErrorField(run, assertion)
        "statusCode" -> checkStatusCode(run, assertion)
        "requestPath" -> checkRequestPath(run, assertion, Which.FIRST)
        "lastRequestPath" -> checkRequestPath(run, assertion, Which.LAST)
        "requestMethod" -> checkRequestMethod(run, assertion)
        "requestQuery" -> checkRequestQuery(run, assertion, Which.FIRST)
        "lastRequestQuery" -> checkRequestQuery(run, assertion, Which.LAST)
        "requestBody" -> checkRequestBody(run, assertion)
        "requestForm" -> checkRequestForm(run, assertion, Which.FIRST)
        "lastRequestForm" -> checkRequestForm(run, assertion, Which.LAST)
        "headerPresent" -> checkHeaderPresent(run, assertion)
        "lastRequestHeader" -> checkLastRequestHeader(run, assertion)
        "responseMeta" -> checkResponseMeta(run, assertion)
        "urlOrigin" -> checkUrlOrigin(run, assertion)
        "responseBody" -> checkResponseBody(run, assertion)
        else -> fail("Unknown assertion type: ${assertion.type}")
    }
}

private fun checkRequestCount(run: Run, assertion: Assertion) {
    val expected = expectedInt(assertion, "requestCount")
    if (run.recorded.count.toLong() != expected) fail("Expected $expected requests, got ${run.recorded.count}")
}

private fun checkDelayBetweenRequests(run: Run, assertion: Assertion) {
    if (run.recorded.times.size < 2) return
    val delay = (run.recorded.times[1] - run.recorded.times[0]) / 1_000_000
    val minimum = assertion.min.toLong()
    if (delay < minimum) fail("Expected delay >= ${minimum}ms, got ${delay}ms")
}

private fun emptyStatuses(operation: String): List<Int> = Routes.ALL.firstOrNull { it.id == operation }?.emptyOn.orEmpty()

private fun checkNoError(run: Run) {
    val error = run.outcome.exceptionOrNull() ?: return
    if (lastStatus(run) in emptyStatuses(run.case.operation)) {
        fail("Expected no error, got: $error (the route treats ${lastStatus(run)} as empty for ${run.case.operation})")
    }
    fail("Expected no error, got: $error")
}

private fun checkErrorCode(run: Run, assertion: Assertion) {
    val expected = expectedString(assertion, "errorCode")
    val error = run.outcome.exceptionOrNull() ?: fail("Expected error code \"$expected\", but got no error")
    val code = (error as? HeyException)?.code ?: fail("Expected error code \"$expected\", got a non-SDK failure: $error")
    if (code != expected) fail("Expected error code \"$expected\", got \"$code\"")
}

private fun checkErrorField(run: Run, assertion: Assertion) {
    val error = run.outcome.exceptionOrNull() as? HeyException
        ?: fail("Expected error field \"${assertion.path}\", but got no error")
    when (assertion.path) {
        "httpStatus" -> {
            val expected = expectedInt(assertion, "errorField.httpStatus")
            val actual = error.httpStatus ?: 0
            if (actual.toLong() != expected) fail("Expected error httpStatus $expected, got $actual")
        }
        "retryable" -> {
            val expected = expectedBool(assertion, "errorField.retryable")
            if (error.retryable != expected) fail("Expected error retryable=$expected, got ${error.retryable}")
        }
        "requestId" -> {
            val expected = expectedString(assertion, "errorField.requestId")
            val actual = error.requestId ?: ""
            if (actual != expected) fail("Expected error requestId \"$expected\", got \"$actual\"")
        }
        else -> fail("Unknown error field: ${assertion.path}")
    }
}

/** A failure has to carry the status on the error itself; only a success reads it off the server. */
private fun checkStatusCode(run: Run, assertion: Assertion) {
    val expected = expectedInt(assertion, "statusCode")
    val error = run.outcome.exceptionOrNull()
    val actual = if (error != null) {
        (error as? HeyException)?.httpStatus?.takeIf { it > 0 }
            ?: fail("Expected status code $expected, but the SDK error carries no HTTP status: $error")
    } else {
        lastStatus(run)
    }
    if (actual.toLong() != expected) fail("Expected status code $expected, got $actual")
}

private fun lastStatus(run: Run): Int = run.recorded.statuses.lastOrNull() ?: 0

private fun checkRequestPath(run: Run, assertion: Assertion, which: Which) {
    val raw = expectedString(assertion, "requestPath")
    val expected = if (run.case.asksForHtml) raw else withJsonExtension(raw)
    val actual = which.pick(run.recorded.paths) ?: fail(noRequests())
    if (actual != expected) fail("Expected ${which.label} request path \"$expected\", got \"$actual\"")
}

/**
 * HEY answers JSON to paths ending in `.json`, and this SDK puts the extension back on a
 * path whose last segment has none, so the expected path is normalised the same way. A page
 * HEY serves as HTML is the exception: the SDK asks for it as written.
 */
fun withJsonExtension(path: String): String {
    val lastSegment = path.substringAfterLast('/')
    return if (path.isEmpty() || path.endsWith("/") || lastSegment.contains('.')) path else "$path.json"
}

private fun checkRequestMethod(run: Run, assertion: Assertion) {
    val expected = expectedString(assertion, "requestMethod")
    val actual = run.recorded.methods.firstOrNull() ?: fail(noRequests())
    if (actual != expected) fail("Expected request method \"$expected\", got \"$actual\"")
}

private fun checkRequestQuery(run: Run, assertion: Assertion, which: Which) {
    val expected = expectedObject(assertion, assertion.type)
    val query = which.pick(run.recorded.queries) ?: fail(noRequests())
    for ((name, want) in expected) {
        val got = query.firstOrNull { it.first == name }?.second
        when {
            want is JsonNull && got == null -> {}
            want is JsonNull -> fail("Expected ${which.label} query param \"$name\" to be absent, got \"$got\"")
            got == null -> fail("Expected ${which.label} query param $name=${display(want)}, got \"\"")
            got != display(want) -> fail("Expected ${which.label} query param $name=${display(want)}, got \"$got\"")
        }
    }
}

private fun checkRequestBody(run: Run, assertion: Assertion) {
    val expected = expectedObject(assertion, "requestBody")
    val raw = run.recorded.bodies.firstOrNull() ?: fail(noRequests())
    val body: JsonElement = if (raw.isEmpty()) {
        JsonNull
    } else {
        runCatching { kotlinx.serialization.json.Json.parseToJsonElement(raw.decodeToString()) }
            .getOrElse { fail("requestBody: request body is not JSON: $it") }
    }
    for ((path, want) in expected) {
        val got = lookup(body, path)
        when {
            want is JsonNull && got == null -> {}
            want is JsonNull -> fail("Expected body key \"$path\" to be absent, got ${display(got!!)}")
            got == null -> fail("Expected body key \"$path\" = ${display(want)}, but it is absent")
            !valuesMatch(want, got) -> fail("Expected body key \"$path\" = ${display(want)}, got ${display(got)}")
        }
    }
}

private fun checkRequestForm(run: Run, assertion: Assertion, which: Which) {
    val expected = expectedObject(assertion, assertion.type)
    val raw = which.pick(run.recorded.bodies) ?: fail(noRequests())
    val fields = queryPairs(raw.decodeToString())
    for ((name, want) in expected) {
        val got = fields.firstOrNull { it.first == name }?.second
        when {
            want is JsonNull && got == null -> {}
            want is JsonNull -> fail("${assertion.type}: expected form field \"$name\" to be absent, got \"$got\"")
            got == null -> fail("${assertion.type}: expected form field \"$name\" = ${display(want)}, but it is absent")
            got != display(want) -> fail("${assertion.type}: expected form field \"$name\" = ${display(want)}, got \"$got\"")
        }
    }
}

private fun checkHeaderPresent(run: Run, assertion: Assertion) {
    val name = assertion.path
    if (run.recorded.headers.isEmpty()) fail("Expected request with header \"$name\", but no requests were recorded")
    if (run.recorded.header(0, name).isNullOrEmpty()) fail("Expected header \"$name\" to be present, but it was not")
}

private fun checkLastRequestHeader(run: Run, assertion: Assertion) {
    val name = assertion.path
    val expected = expectedString(assertion, "lastRequestHeader")
    if (run.recorded.headers.isEmpty()) fail("Expected request with header \"$name\", but no requests were recorded")
    val actual = run.recorded.header(run.recorded.headers.size - 1, name) ?: ""
    if (actual != expected) fail("Expected last request header \"$name\" = \"$expected\", got \"$actual\"")
}

private fun checkResponseMeta(run: Run, assertion: Assertion) {
    val page = run.outcome.getOrNull() as? Outcome.Page
        ?: fail("Expected a paginated result to read responseMeta.${assertion.path} from")
    when (assertion.path) {
        "totalCount" -> {
            val expected = expectedInt(assertion, "responseMeta.totalCount")
            val actual = page.totalCount ?: fail("X-Total-Count header not present in response")
            if (actual != expected) fail("Expected X-Total-Count=$expected, got $actual")
        }
        "nextPage" -> {
            val expected = expectedString(assertion, "responseMeta.nextPage")
            val actual = page.nextPage ?: fail("Link header does not contain a valid next URL")
            if (actual != expected) fail("Expected next page \"$expected\", got \"$actual\"")
        }
        else -> fail("Unknown responseMeta path: ${assertion.path}")
    }
}

private fun checkUrlOrigin(run: Run, assertion: Assertion) {
    val expected = expectedString(assertion, "urlOrigin")
    if (expected != "rejected") fail("urlOrigin: unsupported expected value \"$expected\" (only \"rejected\" is supported)")
    val link = run.recorded.links.lastOrNull() ?: fail("No Link header in response to validate origin")
    if (link.isNullOrEmpty()) fail("No Link header in response to validate origin")
    val target = nextLink(link) ?: fail("No next URL found in Link header: $link")
    val server = URI(run.baseUrl)
    val next = runCatching { URI(target) }.getOrNull()?.takeIf { it.isAbsolute }
        ?: fail("Expected cross-origin Link URL for rejection test, but got relative URL: $target")
    if (sameOrigin(next, server)) fail("Expected cross-origin Link URL for rejection test, but $target has same origin as server")
    checkNextPageRefused(run)
}

private fun port(uri: URI): Int = if (uri.port >= 0) uri.port else if (uri.scheme.equals("https", true)) 443 else 80

private fun sameOrigin(a: URI, b: URI): Boolean =
    a.scheme.equals(b.scheme, ignoreCase = true) && a.host.orEmpty().equals(b.host.orEmpty(), ignoreCase = true) && port(a) == port(b)

private fun checkNextPageRefused(run: Run) {
    val page = run.outcome.getOrElse { fail("Expected a paginated result to read the next page from, got: $it") } as? Outcome.Page
        ?: fail("Expected a paginated result to read the next page from")
    val check = page.nextUrlCheck ?: fail("Expected a paginated result to read the next page from")
    val error = check.exceptionOrNull() ?: fail("Expected the cross-origin next page to be refused, but the SDK followed it")
    val code = (error as? HeyException)?.code
    if (code != HeyException.CODE_USAGE) fail("Expected the cross-origin next page to be refused as a usage error, got ${code ?: error}")
}

private fun checkResponseBody(run: Run, assertion: Assertion) {
    val path = assertion.path
    val outcome = run.outcome.getOrElse { fail("Expected responseBody.$path, got: $it") }
    val body = outcome.body() ?: fail("Expected responseBody.$path, but no response body captured")
    val actual = lookup(body, path) ?: fail("Expected responseBody.$path, but field not present")
    if (!valuesMatch(assertion.expected, actual)) fail("Expected responseBody.$path = ${display(assertion.expected)}, got ${display(actual)}")
}

private enum class Which(val label: String) {
    FIRST("first"),
    LAST("last"),
    ;

    fun <T> pick(values: List<T>): T? = if (this == FIRST) values.firstOrNull() else values.lastOrNull()
}

private fun noRequests(): String = "Expected a request, but none were recorded"

private fun expectedInt(assertion: Assertion, label: String): Long =
    (assertion.expected as? JsonPrimitive)?.longOrNull ?: fail("$label: expected an integer, got ${display(assertion.expected)}")

private fun expectedBool(assertion: Assertion, label: String): Boolean =
    (assertion.expected as? JsonPrimitive)?.booleanOrNull ?: fail("$label: expected a bool, got ${display(assertion.expected)}")

private fun expectedString(assertion: Assertion, label: String): String =
    (assertion.expected as? JsonPrimitive)?.takeIf { it.isString }?.content ?: fail("$label: expected a string, got ${display(assertion.expected)}")

private fun expectedObject(assertion: Assertion, label: String): JsonObject =
    assertion.expected as? JsonObject ?: fail("$label: expected an object, got ${display(assertion.expected)}")

/** Walks a JSON value by a dot-separated path, reading integer segments as array indexes. */
fun lookup(value: JsonElement, path: String): JsonElement? {
    var current: JsonElement = value
    for (segment in path.split('.')) {
        current = when (current) {
            is JsonObject -> current[segment] ?: return null
            is JsonArray -> segment.toIntOrNull()?.let { current.getOrNull(it) } ?: return null
            else -> return null
        }
    }
    return current
}

fun valuesMatch(expected: JsonElement, actual: JsonElement): Boolean {
    val expectedLong = (expected as? JsonPrimitive)?.longOrNull
    val actualLong = (actual as? JsonPrimitive)?.longOrNull
    if (expectedLong != null && actualLong != null) return expectedLong == actualLong
    val expectedDouble = (expected as? JsonPrimitive)?.doubleOrNull
    val actualDouble = (actual as? JsonPrimitive)?.doubleOrNull
    if (expectedDouble != null && actualDouble != null) return expectedDouble == actualDouble
    val expectedBool = (expected as? JsonPrimitive)?.booleanOrNull
    val actualBool = (actual as? JsonPrimitive)?.booleanOrNull
    if (expectedBool != null && actualBool != null) return expectedBool == actualBool
    return display(expected) == display(actual)
}

fun display(value: JsonElement): String = if (value is JsonPrimitive && value.isString) value.content else value.toString()
