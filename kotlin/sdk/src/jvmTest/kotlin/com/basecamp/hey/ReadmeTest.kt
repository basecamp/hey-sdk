package com.basecamp.hey

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The README names the builder's properties, in prose and in snippets; a rename that left
 * them behind would be a README whose examples do not compile. Every backticked identifier
 * that looks like a builder setting has to be one.
 */
class ReadmeTest {
    private val readme = File("../README.md").readText()

    /** The builder's properties, from its setters; a value-class setter carries a mangling suffix after a dash. */
    private val builderProperties: Set<String> = HeyClientBuilder::class.java.methods
        .map { it.name.substringBefore('-') }
        .filter { it.startsWith("set") && it.length > 3 }
        .map { it[3].lowercaseChar() + it.substring(4) }
        .toSet()

    private val looksLikeASetting = Regex("^(enable|max|base|timeout|hooks|cache|engine|userAgent|baseUrl)")

    @Test
    fun everySettingTheReadmeNamesExistsOnTheBuilder() {
        val named = Regex("`([a-z][A-Za-z]*)( = [^`]*)?`").findAll(readme)
            .map { it.groupValues[1] }
            .filter { looksLikeASetting.containsMatchIn(it) }
            .toSet()
        assertTrue("maxRetries" in named && "baseRetryDelay" in named && "maxRetryDelay" in named && "enableCache" in named, "the README names the retry and cache settings: $named")
        assertEquals(emptySet(), named - builderProperties, "the README names settings the builder does not have")
    }
}
