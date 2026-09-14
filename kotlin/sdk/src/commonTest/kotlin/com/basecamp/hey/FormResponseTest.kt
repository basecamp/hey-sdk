package com.basecamp.hey

import io.ktor.http.Url
import io.ktor.http.headersOf
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class FormResponseTest {
    private fun form(status: Int, location: String?): FormResponse =
        FormResponse.of(Response(status, location?.let { headersOf("Location", it) } ?: headersOf(), ByteArray(0), Url("https://app.hey.com/x"), false, true))

    @Test
    fun theIdIsTheRightmostNumberInTheLocation() {
        assertEquals(42L, form(302, "/calendar/events/42").extractId())
        assertEquals(42L, form(303, "https://app.hey.com/calendar/events/42/edit?x=1").extractId())
        assertEquals(42L, form(302, "/calendar/events/42/edit#top").extractId())
        assertFailsWith<HeyException.Api> { form(302, "/calendar/events/new").extractId() }
        assertFailsWith<HeyException.Api> { form(302, null).extractId() }
    }

    @Test
    fun aFormRequestCapturesTheRedirectAndIsNotRetried() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/workflows/7")), status(503))
        val client = hey.client()
        val operation = client.form(Method.POST, "/workflows").info(writeInfo("Workflows", "CreateWorkflow", "workflow"))
        operation.form(listOf("workflow[name]" to "Launch"))
        val response = client.sendForm(operation)
        assertEquals(302, response.status)
        assertEquals("/workflows/7", response.location)
        assertEquals(7L, response.extractId())
        val request = hey.requests.single()
        assertEquals("/workflows", request.path, "a form path is sent as written")
        assertEquals("workflow%5Bname%5D=Launch", request.body)
        assertEquals(true, request.header("Accept")!!.startsWith("text/html"))
        assertNull(request.header("Accept")?.takeIf { it == "application/json" })

        assertFailsWith<HeyException.Api> { client.sendForm(client.form(Method.POST, "/workflows")) }
        assertEquals(2, hey.requests.size)
    }
}
