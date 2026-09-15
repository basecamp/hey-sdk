package com.basecamp.hey

import com.basecamp.hey.generated.extenzions
import com.basecamp.hey.services.CreateExtenzionParams
import com.basecamp.hey.services.Extenzion
import com.basecamp.hey.services.UpdateExtenzionParams
import com.basecamp.hey.services.contactIdFromUrl
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ExtenzionsServiceTest {
    private val sales = Extenzion(10, "sales", "https://app.hey.com/contacts/10")
    private val navigation = """{"items":[{"title":"Boxes","menu_items":[{"title":"Imbox","app_url":"https://app.hey.com/imbox"}]},""" +
        """{"title":"Extensions","menu_items":[{"title":"All Extensions","app_url":"https://app.hey.com/accounts/1/domains/extenzions"},""" +
        """{"title":"sales","app_url":"https://app.hey.com/contacts/10"},{"title":"support","app_url":"https://app.hey.com/contacts/11"}]}]}"""

    @Test
    fun listingReadsTheExtensionsGroupOutOfNavigationAsItsOwnOperation() = runTest {
        val hey = mockHey(ok(navigation))
        val log = OperationLog()
        val listed = hey.client { hooks = log }.extenzions.list()
        assertEquals(listOf(sales, Extenzion(11, "support", "https://app.hey.com/contacts/11")), listed, "the group's own link names no contact and falls out")
        assertEquals("/my/navigation.json", hey.requests.single().path)
        assertEquals(listOf("Extenzions.ListExtenzions:extenzion:false:null"), log.started)
    }

    @Test
    fun creatingAnswersTheExtenzionUnderItsContactId() = runTest {
        val hey = mockHey(Answer(201, """{"id":55,"name":"sales","app_url":"https://app.hey.com/contacts/10"}"""))
        val log = OperationLog()
        val created = hey.client { hooks = log }.extenzions.create(1, CreateExtenzionParams("sales", listOf("jane.dawson@example.com")))
        assertEquals(sales, created, "the id is the contact's, not the record's")
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("/accounts/1/domains/extenzions.json", request.path)
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        assertEquals(listOf("extenzion[name]" to "sales", "extenzion[members][]" to "jane.dawson@example.com"), formPairs(request.body))
        assertEquals(listOf("Extenzions.CreateExtenzion:extenzion:true:null"), log.started)
    }

    @Test
    fun aWriteThatOnlyRedirectsHandsNothingBack() = runTest {
        val redirect = status(302, headers = mapOf("Location" to "/accounts/1/domains/extenzions"))
        val hey = mockHey(redirect, redirect)
        val client = hey.client()
        assertNull(client.extenzions.create(1, CreateExtenzionParams("sales")))
        assertNull(client.extenzions.update(1, 10, UpdateExtenzionParams(name = "support")))
    }

    @Test
    fun aRevisionNamesOnlyWhatItWasAskedToChange() = runTest {
        val answer = """{"id":55,"name":"support","app_url":"https://app.hey.com/contacts/10"}"""
        val hey = mockHey(ok(answer), ok(answer))
        val log = OperationLog()
        val client = hey.client { hooks = log }
        val updated = client.extenzions.update(1, 10, UpdateExtenzionParams(name = "support"))
        assertEquals("support", updated?.name)
        assertEquals("PATCH", hey.requests[0].method)
        assertEquals("/accounts/1/domains/extenzions/10.json", hey.requests[0].path)
        assertEquals(listOf("extenzion[name]" to "support"), formPairs(hey.requests[0].body))
        client.extenzions.update(1, 10, UpdateExtenzionParams(name = "", members = listOf("a@example.com", "b@example.com")))
        assertEquals(
            listOf("extenzion[members][]" to "a@example.com", "extenzion[members][]" to "b@example.com"),
            formPairs(hey.requests[1].body),
            "a membership replaces the whole of what the extenzion had, and an empty name is no name",
        )
        assertEquals("Extenzions.UpdateExtenzion:extenzion:true:10", log.started[0])
    }

    @Test
    fun aContactUrlThatNamesNoReadableIdIsRefusedRatherThanSkipped() = runTest {
        assertNull(contactIdFromUrl("https://app.hey.com/accounts/1/domains/extenzions"))
        assertNull(contactIdFromUrl("/contacts/"))
        assertEquals(4821L, contactIdFromUrl("https://app.hey.com/contacts/4821/edit"))
        val refused = assertFailsWith<HeyException.Api> { contactIdFromUrl("/contacts/99999999999999999999") }
        assertTrue(refused.message.orEmpty().contains("is not a number"), refused.message)

        val hey = mockHey(Answer(201, """{"id":55,"name":"sales","app_url":"https://app.hey.com/accounts/1"}"""))
        val unreadable = assertFailsWith<HeyException.Api> { hey.client().extenzions.create(1, CreateExtenzionParams("sales")) }
        assertTrue(unreadable.message.orEmpty().startsWith("no contact id in"), unreadable.message)
    }
}
