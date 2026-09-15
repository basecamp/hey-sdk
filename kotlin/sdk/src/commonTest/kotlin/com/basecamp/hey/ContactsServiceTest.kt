package com.basecamp.hey

import com.basecamp.hey.generated.contacts
import com.basecamp.hey.services.ClearanceStatus
import com.basecamp.hey.services.ContactConflict
import com.basecamp.hey.services.ContactParams
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class ContactsServiceTest {
    private fun body(request: RecordedRequest): JsonObject = Json.parseToJsonElement(request.body).jsonObject

    private fun contact(request: RecordedRequest): JsonObject = body(request).getValue("contact").jsonObject

    @Test
    fun aCreateSendsTheContactAndTheAccountUserItIsFiledUnder() = runTest {
        val hey = mockHey(ok("""{"id":77,"name":"Ann"}"""), ok("""{"id":78,"name":"Bob"}"""))
        val client = hey.client()
        val created = client.contacts.createContact(ContactParams("Ann", "ann@example.com", aliasEmailAddresses = listOf("a@example.com"), accountUserId = 1000))
        assertEquals(77L, created.id)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/contacts.json", hey.requests[0].path)
        assertEquals(1000L, body(hey.requests[0]).getValue("acting_user_id").jsonPrimitive.content.toLong())
        val sent = contact(hey.requests[0])
        assertEquals("Ann", sent.getValue("name").jsonPrimitive.content)
        assertEquals("ann@example.com", sent.getValue("email_address").jsonPrimitive.content)
        assertEquals(listOf("a@example.com"), sent.getValue("alias_email_addresses").jsonArray.map { it.jsonPrimitive.content })

        client.contacts.createContact(ContactParams("Bob"))
        assertNull(body(hey.requests[1])["acting_user_id"], "left unset, HEY files the contact under the first account")
        assertNull(contact(hey.requests[1])["email_address"], "an empty address is not sent as one")
        assertNull(contact(hey.requests[1])["alias_email_addresses"])
    }

    @Test
    fun aScopedClientFilesTheContactUnderItsAccountAndRefusesAnother() = runTest {
        val hey = mockHey(ok(IDENTITY), ok("""{"id":77,"name":"Ann"}"""))
        val scoped = hey.client().forAccount(42)
        scoped.contacts.createContact(ContactParams("Ann", "ann@example.com"))
        assertEquals(1000L, body(hey.requests[1]).getValue("acting_user_id").jsonPrimitive.content.toLong(), "the identity's user on the scoped account")
        val error = assertFailsWith<HeyException.Usage> { scoped.contacts.createContact(ContactParams("Ann", accountUserId = 7000)) }
        assertEquals("account user 7000 does not belong to selected account 42", error.message)
        assertEquals(2, hey.requests.size)
    }

    @Test
    fun anUpdateReadsTheContactFirstAndFillsInWhatTheCallerLeftUnset() = runTest {
        val current = """{"id":77,"name":"Ann","email_address":"ann@example.com","aliases":[{"id":78,"email_address":"a@example.com"},{"id":79,"email_address":"annie@example.com"}]}"""
        val hey = mockHey(ok(current), ok("""{"id":77,"name":"Ann Smith"}"""), ok(current), ok("""{"id":77,"name":"Ann"}"""))
        val client = hey.client()
        val updated = client.contacts.updateContact(77, ContactParams(name = "Ann Smith"))
        assertEquals("Ann Smith", updated.name)
        assertEquals("GET", hey.requests[0].method)
        assertEquals("/contacts/77.json", hey.requests[0].path)
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals("/contacts/77.json", hey.requests[1].path)
        val merged = contact(hey.requests[1])
        assertEquals("Ann Smith", merged.getValue("name").jsonPrimitive.content)
        assertEquals("ann@example.com", merged.getValue("email_address").jsonPrimitive.content, "the address is kept from the contact")
        assertEquals(listOf("a@example.com", "annie@example.com"), merged.getValue("alias_email_addresses").jsonArray.map { it.jsonPrimitive.content }, "and so are the aliases, since HEY removes any not submitted")

        client.contacts.updateContact(77, ContactParams(aliasEmailAddresses = emptyList()))
        val cleared = contact(hey.requests[3])
        assertEquals("Ann", cleared.getValue("name").jsonPrimitive.content)
        assertEquals(0, cleared.getValue("alias_email_addresses").jsonArray.size, "an explicit empty list clears the aliases")
    }

    @Test
    fun aClashIsAConflictInHeysWordsThatNamesTheContactsItWouldMergeWith() = runTest {
        val hey = mockHey(status(409, """{"errors":["Email address has already been taken"],"contact_id":80,"conflicting_contact_ids":[77,78]}"""))
        val error = assertFailsWith<HeyException.Conflict> { hey.client().contacts.createContact(ContactParams("Ann", "ann@example.com")) }
        assertEquals("Email address has already been taken", error.message)
        val conflict = assertNotNull(ContactConflict.fromError(error))
        assertEquals(80L, conflict.contactId, "a create that clashes still creates the contact")
        assertEquals(listOf(77L, 78L), conflict.conflictingContactIds)
        assertEquals("contact 80 conflicts with 77, 78", conflict.toString())
        assertEquals("contact 80 conflicts with one that already exists", ContactConflict(80).toString())
    }

    @Test
    fun aConflictFromElsewhereCarriesNoContactConflict() = runTest {
        val hey = mockHey(status(409, """{"error":"A time track is already running"}"""), status(409))
        val client = hey.client()
        val worded = assertFailsWith<HeyException.Conflict> { client.contacts.setNote(77, "note") }
        assertEquals("A time track is already running", worded.message, "a single message is read too")
        assertNull(ContactConflict.fromError(worded))
        val bare = assertFailsWith<HeyException.Conflict> { client.contacts.setNote(77, "note") }
        assertEquals("the contact conflicts with one that already exists", bare.message, "a 409 without a readable body still has to read as something")
        assertNull(ContactConflict.fromError(bare))
        assertNull(ContactConflict.fromError(HeyException.NotFound()))
    }

    @Test
    fun aRejectionIsAValidationInTheModelsWords() = runTest {
        val hey = mockHey(status(422, """{"errors":["Name can't be blank","Email address is invalid"]}"""))
        val error = assertFailsWith<HeyException.Validation> { hey.client().contacts.createContact(ContactParams()) }
        assertEquals("Name can't be blank; Email address is invalid", error.message)
    }

    @Test
    fun screeningAndTheNoteGoToTheContactsOwnEndpoints() = runTest {
        val hey = mockHey(ok(""), ok("""{"contact_id":77,"note":"Met at the conference","note_html":"<div>Met at the conference</div>"}"""))
        val client = hey.client()
        client.contacts.screen(77, ClearanceStatus.DENIED)
        assertEquals("/contacts/77/clearance.json", hey.requests[0].path)
        assertEquals("""{"status":"denied"}""", hey.requests[0].body)

        val note = client.contacts.setNote(77, "Met at the conference")
        assertEquals("Met at the conference", note.note)
        assertEquals("/contacts/77/note.json", hey.requests[1].path)
        assertEquals("Met at the conference", contact(hey.requests[1]).getValue("note").jsonPrimitive.content)
    }
}
