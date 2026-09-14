package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.generated.services.DeleteCalendarEventOccurrenceOptions
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.CalendarEventsService as GeneratedCalendarEventsService

/**
 * A partial revision of an event: only the fields named are sent, and HEY leaves the rest as
 * they are. Clock times belong to a timed event, so an all-day revision leaves them off
 * however they are set here.
 */
data class CalendarEventUpdate(
    val title: String? = null,
    /** `YYYY-MM-DD`. */
    val startsAt: String? = null,
    /** `YYYY-MM-DD`. */
    val endsAt: String? = null,
    val allDay: Boolean? = null,
    /** `HH:MM`. */
    val startTime: String? = null,
    /** `HH:MM`. */
    val endTime: String? = null,
)

/**
 * One day of a repeating event, addressed by the series it belongs to plus the day it falls
 * on, as HEY's `occurrence_id` (`<event id>_<YYYY-MM-DD>`) names it.
 */
data class OccurrenceId(val eventId: Long, val date: String) {
    companion object {
        /** Reads HEY's `occurrence_id`. */
        fun parse(source: String): OccurrenceId {
            val separator = source.indexOf('_')
            if (separator < 0) throw HeyException.Usage("occurrence id \"$source\" is not <event id>_<YYYY-MM-DD>")
            val eventId = source.substring(0, separator).toLongOrNull()?.takeIf { it > 0 }
                ?: throw HeyException.Usage("occurrence id \"$source\" names no event")
            val date = source.substring(separator + 1)
            if (!Regex("\\d{4}-\\d{2}-\\d{2}").matches(date)) throw HeyException.Usage("occurrence id \"$source\" names no date")
            return OccurrenceId(eventId, date)
        }
    }

    override fun toString(): String = "${eventId}_$date"
}

/** How much of a repeating event a write to one of its occurrences reaches. */
enum class OccurrenceScope(val wire: String) {
    /** The named day alone. */
    THIS_ONLY("this_event"),

    /** The named day and every one after it. */
    THIS_AND_FOLLOWING("this_and_following"),
    ;

    companion object {
        /** Reads the scope as HEY writes it. */
        fun parse(source: String): OccurrenceScope =
            entries.firstOrNull { it.wire == source }
                ?: throw HeyException.Usage("occurrence scope \"$source\" is neither this_event nor this_and_following")
    }
}

/**
 * Calendar events service with the form-backed update on top of the generated surface
 * (`delete`, `deleteOccurrence`). HEY's calendar writes are Rails form posts rather than
 * JSON, so an update goes out form-encoded under `calendar_event[...]`.
 */
class CalendarEventsService(client: HeyClient) : GeneratedCalendarEventsService(client) {
    /**
     * Revises an event. HEY reads a calendar write out of submitted form parameters, so this
     * posts a form to the JSON path and names only the fields the revision carries; the rest
     * keep their value.
     */
    suspend fun update(eventId: Long, update: CalendarEventUpdate) {
        val operation = client.request(Method.PATCH, "/calendar/events/$eventId")
        operation.info(writeInfo("CalendarEvents", "UpdateCalendarEvent", "calendar_event", eventId))
        operation.form(updateFields(update))
        operation.accept("application/json")
        client.sendUnit(operation)
    }

    /**
     * Removes one day of a repeating event, or that day and every one after it. The generated
     * [deleteOccurrence] takes the same request in its parts.
     */
    suspend fun deleteOccurrenceScoped(occurrence: OccurrenceId, scope: OccurrenceScope) {
        deleteOccurrence(
            occurrence.eventId,
            occurrence.date,
            DeleteCalendarEventOccurrenceOptions(applyToFuture = scope == OccurrenceScope.THIS_AND_FOLLOWING),
        )
    }
}

internal fun updateFields(update: CalendarEventUpdate): List<Pair<String, String>> {
    val fields = mutableListOf<Pair<String, String>>()
    update.title?.let { fields += "calendar_event[summary]" to it }
    update.startsAt?.let { fields += "calendar_event[starts_at]" to it }
    update.endsAt?.let { fields += "calendar_event[ends_at]" to it }
    update.allDay?.let { fields += "calendar_event[all_day]" to if (it) "1" else "0" }
    if (update.allDay != true) {
        update.startTime?.let { fields += "calendar_event[starts_at_time]" to "$it:00" }
        update.endTime?.let { fields += "calendar_event[ends_at_time]" to "$it:00" }
    }
    return fields
}
