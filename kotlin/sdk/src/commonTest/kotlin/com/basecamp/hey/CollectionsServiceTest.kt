package com.basecamp.hey

import com.basecamp.hey.generated.collections
import com.basecamp.hey.services.CreateCollectionParams
import com.basecamp.hey.services.UpdateCollectionParams
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class CollectionsServiceTest {
    @Test
    fun aCollectionIsMadeWithWhatItWasGiven() = runTest {
        val redirect = status(302, headers = mapOf("Location" to "/collections"))
        val hey = mockHey(redirect, redirect)
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.collections.create(CreateCollectionParams("Reading", summary = "Long reads for the weekend", accountId = 77))
        client.collections.create(CreateCollectionParams("Reading", summary = ""))
        val request = hey.requests[0]
        assertEquals("POST", request.method)
        assertEquals("/collections", request.path)
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        assertEquals(BROWSER_ACCEPT_HEADER, request.header("Accept"))
        assertEquals(
            listOf("collection[name]" to "Reading", "collection[summary]" to "Long reads for the weekend", "account_id" to "77"),
            formPairs(request.body),
        )
        assertEquals(listOf("collection[name]" to "Reading"), formPairs(hey.requests[1].body), "an empty summary and no account are left off the wire")
        assertEquals("Collections.CreateCollection:collection:true:null", log.started[0])
    }

    @Test
    fun aRevisionSendsOnlyWhatItNamesAsTheGeneratedUpdateDoes() = runTest {
        val hey = mockHey(ok(""))
        hey.client().collections.updateCollection(5, UpdateCollectionParams(name = "Renamed", summary = ""))
        val request = hey.requests.single()
        assertEquals("/collections/5.json", request.path)
        val collection = Json.parseToJsonElement(request.body).jsonObject.getValue("collection").jsonObject
        assertEquals("Renamed", collection.getValue("name").jsonPrimitive.content)
        assertTrue(collection["summary"] == null || collection["summary"] is JsonNull, "an empty summary is no summary, and is left as it was")
    }

    @Test
    fun aTopicIsFiledIntoAndTakenOutOfACollectionByTheQuery() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/topics/4471829")), status(302, headers = mapOf("Location" to "/topics/4471829")))
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.collections.addTopic(4471829, 5)
        client.collections.removeTopic(4471829, 5)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/topics/4471829/collecting", hey.requests[0].path)
        assertEquals("5", hey.requests[0].query("collection_id"))
        assertEquals("application/x-www-form-urlencoded", hey.requests[0].header("Content-Type"), "an empty form is still a form")
        assertEquals("", hey.requests[0].body)
        assertEquals("DELETE", hey.requests[1].method)
        assertEquals("/topics/4471829/collecting", hey.requests[1].path)
        assertEquals("5", hey.requests[1].query("collection_id"))
        assertEquals(
            listOf("Collections.CreateTopicCollecting:collecting:true:4471829", "Collections.DeleteTopicCollecting:collecting:true:4471829"),
            log.started,
        )
    }
}
