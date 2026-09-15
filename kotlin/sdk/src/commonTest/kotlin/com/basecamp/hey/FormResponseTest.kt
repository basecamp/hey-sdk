package com.basecamp.hey

import io.ktor.http.Url
import io.ktor.http.headersOf
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import com.basecamp.hey.services.DraftContent
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

    @Test
    fun onlyA302Or303CompletesAFormWrite() = runTest {
        for (accepted in listOf(302, 303)) {
            val hey = mockHey(status(accepted, headers = mapOf("Location" to "/workflows/7")))
            val response = hey.client().sendForm(hey.client().form(Method.POST, "/workflows"))
            assertEquals(accepted, response.status)
            assertEquals(7L, response.extractId())
        }
        for (refused in listOf(301, 307, 308)) {
            val hey = mockHey(status(refused, headers = mapOf("Location" to "/workflows/7")))
            val error = assertFailsWith<HeyException.Api> { hey.client().sendForm(hey.client().form(Method.POST, "/workflows")) }
            assertEquals(refused, error.httpStatus)
            assertEquals(1, hey.requests.size, "a $refused is not followed by a form request either")
        }
    }

    @Test
    fun aLocationWithoutAnIdIsNeverQuotedWhole() = runTest {
        for (location in listOf("/done?sig=distinctive-secret", "https://app.hey.com/done?sig=distinctive-secret#f", "https://u:distinctive-secret@app.hey.com/done", "::not a url::?sig=distinctive-secret")) {
            val error = assertFailsWith<HeyException.Api> { form(302, location).extractId() }
            assertEquals(false, error.message!!.contains("distinctive"), error.message)
            assertEquals(false, error.toString().contains("distinctive"))
        }
        assertEquals("/done", redactLocation("/done?sig=x#y"))
        assertEquals("https://app.hey.com/done", redactLocation("https://u:p@app.hey.com/done?sig=x"))

        val hey = mockHey(status(204, headers = mapOf("Location" to "/messages/new?sig=distinctive-secret")))
        val seen = mutableListOf<Throwable>()
        val client = hey.client { hooks = object : HeyHooks { override fun onOperationEnd(info: OperationInfo, result: OperationResult) { result.error?.let { seen += it } } } }
        val error = assertFailsWith<HeyException.Api> { client.messages.createDraft(DraftContent(subject = "s", content = "c", to = listOf("a@example.com"), actingSenderId = 100)) }
        assertEquals(false, (error.message + error.toString() + seen.joinToString { it.toString() }).contains("distinctive"))
    }
}
