package com.basecamp.hey.conformance

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class AssertionHelpersTest {
    @Test
    fun aJsonSuffixIsPutOnAPathWithoutAnExtension() {
        assertEquals("/boxes.json", withJsonExtension("/boxes"))
        assertEquals("/boxes.json", withJsonExtension("/boxes.json"))
        assertEquals("/workflows/1/stages/2.json", withJsonExtension("/workflows/1/stages/2"))
        assertEquals("/boxes/", withJsonExtension("/boxes/"))
    }

    @Test
    fun aDottedPathWalksObjectsAndArrays() {
        val value = Json.parseToJsonElement("""{"postings":[{"id":7,"kind":"topic"}],"id":33}""")
        assertEquals(JsonPrimitive(7), lookup(value, "postings.0.id"))
        assertEquals(JsonPrimitive(33), lookup(value, "id"))
        assertNull(lookup(value, "postings.1.id"))
        assertNull(lookup(value, "postings.id"))
    }

    @Test
    fun valuesMatchAcrossNumberSpellings() {
        assertTrue(valuesMatch(JsonPrimitive(9007199254740993L), JsonPrimitive(9007199254740993L)))
        assertTrue(valuesMatch(JsonPrimitive("Imbox"), JsonPrimitive("Imbox")))
        assertTrue(valuesMatch(JsonPrimitive(true), JsonPrimitive(true)))
        assertEquals(false, valuesMatch(JsonPrimitive(1), JsonPrimitive(2)))
        assertTrue(valuesMatch(JsonPrimitive(1), JsonPrimitive(1.0)), "a number is the same number however it is spelled")
        assertEquals(false, valuesMatch(JsonPrimitive(9007199254740993L), Json.parseToJsonElement("9007199254740992.0")), "and a double that lost a digit is not the same number")
        assertEquals(false, valuesMatch(JsonPrimitive(1), JsonPrimitive("1")), "but a string of a number is not a number")
        assertEquals(false, valuesMatch(JsonPrimitive(true), JsonPrimitive("true")))
        assertEquals(false, valuesMatch(JsonPrimitive("1"), JsonPrimitive(1)))
    }

    @Test
    fun queryPairsAreDecoded() {
        assertEquals(listOf("posting_ids" to "1,2", "page" to "older"), queryPairs("posting_ids=1%2C2&page=older"))
        assertEquals(emptyList(), queryPairs(null))
    }

    private fun run(recorded: Recorded, vararg assertions: Assertion): Run =
        Run(TestCase(name = "t", operation = "ListBoxes", assertions = assertions.toList()), Result.success(Outcome.Unit), recorded, "http://127.0.0.1:1")

    @Test
    fun everyWaitBetweenRequestsIsChecked() {
        val recorded = Recorded()
        recorded.times += listOf(0L, 1_000_000_000L, 1_001_000_000L)
        val minimum = Assertion(type = "delayBetweenRequests", min = 1000.0)
        val error = assertFailsWith<AssertionFailure> { checkAll(run(recorded, minimum)) }
        assertTrue(error.message!!.contains("before request 3"), error.message)
        recorded.times[2] = 2_000_000_000L
        checkAll(run(recorded, minimum))
    }

    @Test
    fun aScalarQueryParameterHasToBeThereExactlyOnce() {
        val recorded = Recorded()
        recorded.queries += listOf(listOf("filtered_account_id" to "42", "filtered_account_id" to "42"))
        val expected = Assertion(type = "requestQuery", expected = Json.parseToJsonElement("""{"filtered_account_id":"42"}"""))
        val error = assertFailsWith<AssertionFailure> { checkAll(run(recorded, expected)) }
        assertTrue(error.message!!.contains("once, got it 2 times"), error.message)
        recorded.queries[0] = listOf("filtered_account_id" to "42")
        checkAll(run(recorded, expected))
    }

    @Test
    fun aScalarFormFieldHasToBeThereExactlyOnce() {
        val recorded = Recorded()
        recorded.bodies += "calendar_event%5Bsummary%5D=Expected&calendar_event%5Bsummary%5D=Wrong".encodeToByteArray()
        val expected = Assertion(type = "requestForm", expected = Json.parseToJsonElement("""{"calendar_event[summary]":"Expected"}"""))
        val error = assertFailsWith<AssertionFailure> { checkAll(run(recorded, expected)) }
        assertTrue(error.message!!.contains("once, got it 2 times"), error.message)
        recorded.bodies[0] = "calendar_event%5Bsummary%5D=Expected".encodeToByteArray()
        checkAll(run(recorded, expected))
    }
}
