package com.basecamp.hey

import com.basecamp.hey.services.WORLD_ADDRESS
import com.basecamp.hey.services.importFilename
import com.basecamp.hey.services.postToken
import com.basecamp.hey.services.subscriberImportBody
import com.basecamp.hey.services.world
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class WorldServiceTest {
    private val csv = "email_address\njane.dawson@example.com\n"

    @Test
    fun publishingAddressesTheWorldAndAnswersThePostToken() = runTest {
        val hey = mockHey(ok(IDENTITY), status(302, headers = mapOf("Location" to "/world/posts/a1b2c3d4")))
        val log = OperationLog()
        val token = hey.client { hooks = log }.world.publish("On writing less", "<div>Fewer words, more meaning.</div>")
        assertEquals("a1b2c3d4", token)
        val request = hey.requests[1]
        assertEquals("POST", request.method)
        assertEquals("/messages", request.path)
        assertEquals(BROWSER_ACCEPT_HEADER, request.header("Accept"))
        assertEquals(
            listOf(
                "acting_sender_id" to "100",
                "message[subject]" to "On writing less",
                "message[content]" to "<div>Fewer words, more meaning.</div>",
                "entry[addressed][directly]" to WORLD_ADDRESS,
                "entry[status]" to "active",
            ),
            formPairs(request.body),
        )
        assertEquals("World.PublishWorldPost:world_post:true:null", log.started.last())
    }

    @Test
    fun aMessageThatDidNotBecomeAPostSaysWhereItLanded() = runTest {
        val hey = mockHey(ok(IDENTITY), status(302, headers = mapOf("Location" to "/topics/4471829")))
        val log = OperationLog()
        val refused = assertFailsWith<HeyException.Api> { hey.client { hooks = log }.world.publish("On writing less", "<div>Fewer words.</div>") }
        assertEquals("the message was sent but did not become a HEY World post (landed on \"/topics/4471829\")", refused.message)
        assertEquals("World.PublishWorldPost:api_error", log.ended.last(), "the hooks hear the failure the caller gets")
    }

    @Test
    fun anEditLeavesOutWhatItWasNotGiven() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/world/posts/a1b2c3d4")))
        val log = OperationLog()
        hey.client { hooks = log }.world.updatePost("a1b2c3d4", "On writing even less", "")
        val request = hey.requests.single()
        assertEquals("PATCH", request.method)
        assertEquals("/world/posts/a1b2c3d4", request.path)
        assertEquals(listOf("world_post[subject]" to "On writing even less"), formPairs(request.body))
        assertEquals(listOf("World.UpdateWorldPost:world_post:true:null"), log.started)
    }

    @Test
    fun deletingAPostNamesItByItsToken() = runTest {
        val hey = mockHey(status(303, headers = mapOf("Location" to "/world/lists/david@example.com")))
        val log = OperationLog()
        hey.client { hooks = log }.world.deletePost("a1b2c3d4")
        val request = hey.requests.single()
        assertEquals("DELETE", request.method)
        assertEquals("/world/posts/a1b2c3d4", request.path)
        assertEquals("", request.body)
        assertEquals(listOf("World.DeleteWorldPost:world_post:true:null"), log.started)
    }

    @Test
    fun subscribersExportAsTheCsvHeyStreamed() = runTest {
        val hey = mockHey(Answer(200, csv, headers = mapOf("Content-Type" to "text/csv")))
        val log = OperationLog()
        val exported = hey.client { hooks = log }.world.exportSubscribers("david@example.com")
        assertEquals(csv, exported.decodeToString())
        val request = hey.requests.single()
        assertEquals("GET", request.method)
        assertEquals("/world/lists/david%40example.com/export.csv", request.path, "the list is named by an address, escaped as a path parameter is")
        assertEquals("text/csv", request.header("Accept"))
        assertEquals(listOf("World.ExportWorldSubscribers:world_list:false:null"), log.started)
    }

    @Test
    fun anImportUploadsTheCsvAsThePartHeyReads() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/world/lists/david@example.com/imports/1")))
        val log = OperationLog()
        hey.client { hooks = log }.world.importSubscribers("david@example.com", "subscribers.csv", csv.encodeToByteArray())
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("/world/lists/david%40example.com/imports", request.path)
        val contentType = request.header("Content-Type").orEmpty()
        assertTrue(contentType.startsWith("multipart/form-data; boundary="), contentType)
        val boundary = contentType.removePrefix("multipart/form-data; boundary=")
        assertEquals(
            "--$boundary\r\nContent-Disposition: form-data; name=\"world_list_import[source]\"; filename=\"subscribers.csv\"\r\nContent-Type: application/octet-stream\r\n\r\n$csv\r\n--$boundary--\r\n",
            request.body,
        )
        assertEquals(listOf("World.ImportWorldSubscribers:world_list:true:null"), log.started)
    }

    @Test
    fun anImportWithoutACsvNameGetsOne() {
        assertEquals("subscribers.csv", importFilename(""))
        assertEquals("people.csv", importFilename("people"))
        assertEquals("people.csv", importFilename("people.csv"))
        assertEquals("PEOPLE.CSV.csv", importFilename("PEOPLE.CSV"), "the suffix check is case-sensitive, as Go's is")
        val (_, body) = subscriberImportBody("say \"hi\"\\now", ByteArray(0))
        assertTrue(body.decodeToString().contains("filename=\"say \\\"hi\\\"\\\\now.csv\""), "a quote or backslash in the name is escaped rather than ending the field")
    }

    @Test
    fun thePostTokenIsTheHexRunAfterTheFirstPostsPathThatHasOne() {
        assertEquals("a1b2c3d4", postToken("https://app.hey.com/world/posts/a1b2c3d4?welcome=1"))
        assertEquals("beef", postToken("/world/posts/beef/edit"))
        assertEquals("beef", postToken("/world/posts/?/world/posts/beef"), "an empty run is skipped for a later one")
        assertNull(postToken("/world/posts/"))
        assertNull(postToken("/world/posts/ABCD"), "only lowercase hex is a token")
        assertNull(postToken("/topics/4471829"))
    }

    @Test
    fun anImportFilenameWithALineBreakIsRefusedBeforeAnythingIsSent() = runTest {
        val hey = mockHey()
        for (filename in listOf("a\r\nContent-Type: text/html\r\n", "a\nb.csv", "a\rb")) {
            assertFailsWith<HeyException.Usage> { hey.client().world.importSubscribers("list@example.com", filename, "x".encodeToByteArray()) }
        }
        assertEquals(0, hey.requests.size)
    }
}
