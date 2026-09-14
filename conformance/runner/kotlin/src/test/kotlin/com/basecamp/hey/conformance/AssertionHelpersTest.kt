package com.basecamp.hey.conformance

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
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
    }

    @Test
    fun queryPairsAreDecoded() {
        assertEquals(listOf("posting_ids" to "1,2", "page" to "older"), queryPairs("posting_ids=1%2C2&page=older"))
        assertEquals(emptyList(), queryPairs(null))
    }
}
