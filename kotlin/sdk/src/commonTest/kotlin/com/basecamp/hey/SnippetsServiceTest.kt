package com.basecamp.hey

import com.basecamp.hey.generated.snippets
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class SnippetsServiceTest {
    @Test
    fun aSnippetIsSavedEditedAndThrownAwayAsForms() = runTest {
        val redirect = status(302, headers = mapOf("Location" to "/snippets"))
        val hey = mockHey(redirect, redirect, redirect)
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.snippets.create("Sign-off", "<div>Cheers, Jane</div>")
        client.snippets.update(12, "", "<div>Best, Jane</div>")
        client.snippets.delete(12)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/snippets", hey.requests[0].path)
        assertEquals("application/x-www-form-urlencoded", hey.requests[0].header("Content-Type"))
        assertEquals(BROWSER_ACCEPT_HEADER, hey.requests[0].header("Accept"))
        assertEquals(listOf("snippet[name]" to "Sign-off", "snippet[content]" to "<div>Cheers, Jane</div>"), formPairs(hey.requests[0].body))
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals("/snippets/12", hey.requests[1].path)
        assertEquals(listOf("snippet[content]" to "<div>Best, Jane</div>"), formPairs(hey.requests[1].body), "an empty field is left out, and so left as it was")
        assertEquals("DELETE", hey.requests[2].method)
        assertEquals("/snippets/12", hey.requests[2].path)
        assertEquals("", hey.requests[2].body)
        assertEquals(
            listOf("Snippets.CreateSnippet:snippet:true:null", "Snippets.UpdateSnippet:snippet:true:12", "Snippets.DeleteSnippet:snippet:true:12"),
            log.started,
        )
    }
}
