package com.basecamp.hey.conformance

import com.basecamp.hey.generated.Routes
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.longOrNull

/** One conformance case, as the JSON files under conformance/tests write it. Keys the runner does not read are ignored. */
@Serializable
data class TestCase(
    val name: String,
    val description: String = "",
    val operation: String,
    val method: String = "",
    val path: String = "",
    val pathParams: JsonObject = JsonObject(emptyMap()),
    val queryParams: JsonObject = JsonObject(emptyMap()),
    val requestBody: JsonObject = JsonObject(emptyMap()),
    val mockResponses: List<MockResponse> = emptyList(),
    val assertions: List<Assertion> = emptyList(),
    val tags: List<String> = emptyList(),
    val configOverrides: ConfigOverrides = ConfigOverrides(),
    /** Invokes the operation this many times against one client. Zero means once. */
    val repeatOperation: Int = 0,
) {
    val isHeyLayer: Boolean get() = configOverrides.clientLayer == "hey"

    val runs: Int get() = maxOf(repeatOperation, 1)

    /** Whether the case's operation reads a page HEY serves as HTML, which the SDK asks for as written. */
    val asksForHtml: Boolean get() = Routes.ALL.any { it.id == operation && it.html }

    /** Whether the case looks at what the SDK does with the `Link` header. */
    val followsNextPage: Boolean get() = assertions.any { it.type == "urlOrigin" }
}

@Serializable
data class ConfigOverrides(
    val baseUrl: String? = null,
    val clientLayer: String? = null,
    val cacheEnabled: Boolean = false,
    val refreshableCredentials: Boolean = false,
    val accountId: Long? = null,
    val maxRetries: Int? = null,
    val baseDelayMs: Long? = null,
)

@Serializable
data class MockResponse(
    val status: Int = 0,
    val headers: Map<String, String> = emptyMap(),
    val body: JsonElement? = null,
    val delay: Long = 0,
) {
    val contentType: String? get() = headers.entries.firstOrNull { it.key.equals("content-type", ignoreCase = true) }?.value

    val servesHtml: Boolean get() = contentType?.startsWith("text/html") == true
}

@Serializable
data class Assertion(
    val type: String,
    val expected: JsonElement = JsonNull,
    val min: Double = 0.0,
    val max: Double = 0.0,
    val path: String = "",
)

fun JsonObject.int64(key: String): Long = (this[key] as? JsonPrimitive)?.longOrNull ?: 0L

fun JsonObject.int32(key: String): Int = int64(key).toInt()

fun JsonObject.string(key: String): String = (this[key] as? JsonPrimitive)?.contentOrNull ?: ""

fun JsonObject.stringOrNull(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.content

fun JsonObject.boolOrNull(key: String): Boolean? = (this[key] as? JsonPrimitive)?.booleanOrNull

/** The value when it is a non-empty string, for the wire fields an empty string omits. */
fun JsonObject.nonEmptyString(key: String): String? = stringOrNull(key)?.takeIf { it.isNotEmpty() }

/** The value when the key is there at all, for the optional parameters a case sends by naming them. */
fun JsonObject.gatedString(key: String): String? = if (containsKey(key)) string(key) else null

fun JsonObject.gatedInt64(key: String): Long? = if (containsKey(key)) int64(key) else null

fun JsonObject.gatedInt32(key: String): Int? = if (containsKey(key)) int32(key) else null

fun JsonObject.int64List(key: String): List<Long> = (this[key]?.let { it as? kotlinx.serialization.json.JsonArray })?.mapNotNull { (it as? JsonPrimitive)?.longOrNull }.orEmpty()

fun JsonObject.int32ListOrNull(key: String): List<Int>? = this[key]?.jsonArray?.mapNotNull { (it as? JsonPrimitive)?.longOrNull?.toInt() }

fun JsonObject.stringList(key: String): List<String> = (this[key]?.let { it as? kotlinx.serialization.json.JsonArray })?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.orEmpty()

fun JsonObject.stringListOrNull(key: String): List<String>? = this[key]?.jsonArray?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }
