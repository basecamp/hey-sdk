package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.Method
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.SnippetsService as GeneratedSnippetsService

/**
 * Snippets service with the writes on top of the generated surface (`list`). Snippets have
 * no JSON surface for writes — every one of them redirects — so they are browser form posts.
 */
class SnippetsService(client: HeyClient) : GeneratedSnippetsService(client) {
    /** Saves a snippet. */
    suspend fun create(name: String, content: String) {
        val operation = client.form(Method.POST, "/snippets")
        operation.info(writeInfo("Snippets", "CreateSnippet", "snippet"))
        operation.form(snippetFields(name, content))
        client.sendUnit(operation)
    }

    /** Edits a snippet. An empty field is left out of the form, and so left as it was. */
    suspend fun update(snippetId: Long, name: String, content: String) {
        val operation = client.form(Method.PATCH, "/snippets/$snippetId")
        operation.info(writeInfo("Snippets", "UpdateSnippet", "snippet", snippetId))
        operation.form(snippetFields(name, content))
        client.sendUnit(operation)
    }

    /** Throws a snippet away. */
    suspend fun delete(snippetId: Long) {
        val operation = client.form(Method.DELETE, "/snippets/$snippetId")
        operation.info(writeInfo("Snippets", "DeleteSnippet", "snippet", snippetId))
        client.sendUnit(operation)
    }
}

private fun snippetFields(name: String, content: String): List<Pair<String, String>> {
    val fields = mutableListOf<Pair<String, String>>()
    if (name.isNotEmpty()) fields += "snippet[name]" to name
    if (content.isNotEmpty()) fields += "snippet[content]" to content
    return fields
}
