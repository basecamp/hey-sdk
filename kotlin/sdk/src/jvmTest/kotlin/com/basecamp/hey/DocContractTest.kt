package com.basecamp.hey

import java.io.File
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * What a method's own documentation promises has to be what the method does: the one place a
 * caller reads before calling. The partial calendar update clears what it does not name,
 * and its KDoc has to say so and point at the whole-event update.
 */
class DocContractTest {
    @Test
    fun thePartialCalendarUpdateSaysWhatItClears() {
        val source = File("src/commonMain/kotlin/com/basecamp/hey/services/CalendarEventsService.kt").readText()
        val kdoc = source.substringBefore("suspend fun update(eventId: Long, update: CalendarEventUpdate)").substringAfterLast("/**")
        assertTrue(kdoc.contains("cleared"), "the update's KDoc says what is cleared:\n$kdoc")
        assertTrue(kdoc.contains("[updateEvent]"), "and points at the whole-event update:\n$kdoc")
        for (field in listOf("notes", "attendees", "reminders", "countdown")) assertTrue(kdoc.contains(field), "names $field")
        assertTrue(!kdoc.contains("the rest keep their value"), "and no longer claims the rest is kept")
    }
}
