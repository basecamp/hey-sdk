package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.Operation
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.JournalEntryPayload
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.models.UpdateJournalEntryRequestContent
import com.basecamp.hey.json
import com.basecamp.hey.generated.services.JournalService as GeneratedJournalService

/**
 * Journal service: the entry a day holds, read and written as its content, on top of the
 * generated surface (`getEntry`, `listEntries`, `updateEntry`). A day has at most one
 * entry. HEY answers it as a calendar recording carrying the full text (`content`) and the
 * rich-text HTML (`contentHtml`); a day with no entry answers 204 and no body, which reads
 * here as null.
 */
class JournalService(client: HeyClient) : GeneratedJournalService(client) {
    /**
     * The rich-text HTML of the day's journal entry, falling back to its plain text, or null
     * when the day has no entry. [day] is `YYYY-MM-DD`.
     *
     * The fallback covers an empty `contentHtml` as well as a missing one: HEY serves the
     * key blank on an entry it has no rendered body for, and blank is not the entry.
     */
    suspend fun getContent(day: String): String? {
        val operation = client.operation(Routes.GET_JOURNAL_ENTRY, listOf(day))
        operation.operationName("GetJournalContent")
        return recording(operation)?.let { entry -> entry.contentHtml?.takeIf { it.isNotEmpty() } ?: entry.content }
    }

    /**
     * The day's journal entry, or null when it has none. The generated [getEntry] reads the
     * same route but takes the empty answer for a day without an entry as a body it could
     * not decode.
     */
    suspend fun entry(day: String): Recording? = recording(client.operation(Routes.GET_JOURNAL_ENTRY, listOf(day)))

    /**
     * Writes the day's journal entry, creating it if needed, and answers it as a recording.
     * Empty content removes the entry, which HEY answers with nothing, so the result is null.
     */
    suspend fun updateContent(day: String, content: String): Recording? {
        val operation = client.operation(Routes.UPDATE_JOURNAL_ENTRY, listOf(day))
        operation.json(UpdateJournalEntryRequestContent(JournalEntryPayload(content)))
        return recording(operation)
    }

    /** The recording an answer carries, or null for the bodiless answer a day without an entry gets. */
    private suspend fun recording(operation: Operation): Recording? =
        client.execute(operation) { response -> if (response.body.isEmpty()) null else response.json(Recording.serializer()) }
}
