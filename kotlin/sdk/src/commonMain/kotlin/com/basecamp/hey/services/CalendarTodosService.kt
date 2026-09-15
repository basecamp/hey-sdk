package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.models.CalendarTodoChanges
import com.basecamp.hey.generated.models.CalendarTodoPayload
import com.basecamp.hey.generated.models.CreateCalendarTodoRequestContent
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.models.UpdateCalendarTodoRequestContent
import com.basecamp.hey.todayLocalDate
import com.basecamp.hey.generated.services.CalendarTodosService as GeneratedCalendarTodosService

/**
 * What an edit changes about a todo. A field left null is left alone: HEY applies what it
 * is sent and keeps the rest, so a rename carries a title and says nothing about the day.
 */
data class TodoChanges(
    /** A new title. An empty one is no title, and changes nothing. */
    val title: String? = null,
    /** The day the todo moves to, `YYYY-MM-DD`. */
    val startsAt: String? = null,
    /** Whether the todo is in focus. */
    val focused: Boolean? = null,
)

/**
 * Calendar todos service with the writes that take a todo in its parts, on top of the
 * generated surface (`create`, `update`, `complete`, `uncomplete`, `delete`). The day goes
 * on the wire as a bare `YYYY-MM-DD`, which HEY casts in the reader's time zone; an instant
 * at UTC midnight would land on the previous day once cast, so a day is checked to be one
 * before it is sent.
 */
class CalendarTodosService(client: HeyClient) : GeneratedCalendarTodosService(client) {
    /** Creates a todo, filed on a day. No day files it on today where this machine is. */
    suspend fun createTodo(title: String, startsAt: String? = null): Recording =
        create(CreateCalendarTodoRequestContent(CalendarTodoPayload(title = title, startsAt = startsAt?.let(::calendarDate) ?: todayLocalDate())))

    /**
     * Edits a todo. [todoId] is the recording's id. Changing nothing is refused rather than
     * sent: an empty payload asks HEY to do nothing and answers as though it had done
     * something.
     */
    suspend fun updateTodo(todoId: Long, changes: TodoChanges): Recording {
        // An empty title is no title: HEY refuses a todo without one, so an empty one is left
        // out and reads as changing nothing rather than as clearing it.
        val title = changes.title?.takeIf { it.isNotEmpty() }
        val startsAt = changes.startsAt?.let(::calendarDate)
        if (title == null && startsAt == null && changes.focused == null) {
            throw HeyException.Usage("update calendar todo $todoId: nothing to change")
        }
        return update(todoId, UpdateCalendarTodoRequestContent(CalendarTodoChanges(title = title, startsAt = startsAt, focused = changes.focused)))
    }
}

/** A day as the calendar has it, or a refusal: Go and Rust hold the day in a date type that cannot name a day the calendar lacks. */
private fun calendarDate(day: String): String {
    if (!OccurrenceId.isCalendarDate(day)) throw HeyException.Usage("\"$day\" is not a YYYY-MM-DD the calendar has")
    return day
}
