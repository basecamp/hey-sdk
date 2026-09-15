package com.basecamp.hey.services

import com.basecamp.hey.HeyException
import com.basecamp.hey.Operation
import com.basecamp.hey.generated.models.Calendar
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.parseAbsoluteUrl
import io.ktor.http.Url
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Where a read of a calendar changes feed starts. There are two feeds and they speak the
 * same cursor — the calendar-level feed behind a [CalendarList]'s `calendarChangesUrl`, and
 * each calendar's own recording feed behind its [ListedCalendar]'s `recordingChangesUrl`.
 * [since] is an ISO 8601 timestamp with milliseconds and is exclusive; [version] is the
 * contract version the caller speaks, HEY's `v`.
 *
 * Build one with [fromUrl] rather than by hand. The two server-issued URLs differ — a
 * recording changes URL carries `v=1`, which the recording feed refuses to answer without,
 * while a calendar changes URL carries no version at all — so only the server knows which
 * pair its feed wants.
 */
data class CalendarChangesCursor(
    /** The instant the changes come after. */
    val since: String? = null,
    /** The contract version the feed speaks. */
    val version: String? = null,
    /** The page within an increment, while it has more than one. */
    val page: String? = null,
    /** How many changes a page holds, when the URL named a size. */
    val perPage: String? = null,
) {
    /** Renders the cursor onto a request. The version is never invented here: a cursor read from a server-issued URL carries whichever version that feed speaks. */
    internal fun applyTo(operation: Operation) {
        operation.queryOptional("since", since)
        operation.queryOptional("v", version)
        operation.queryOptional("page", page)
        operation.queryOptional("per_page", perPage)
    }

    companion object {
        /**
         * Reads a cursor out of a changes URL the server issued: a calendar list's
         * `calendarChangesUrl`, a listed calendar's `recordingChangesUrl`, or the `Link`
         * header either feed answered with.
         */
        fun fromUrl(changesUrl: String): CalendarChangesCursor =
            fromParsed(parseAbsoluteUrl(changesUrl) ?: throw HeyException.Usage("changes URL is not an absolute URL"))

        internal fun fromParsed(url: Url): CalendarChangesCursor {
            val parameters = url.parameters
            fun parameter(name: String): String? = parameters[name]?.takeIf { it.isNotEmpty() }
            return CalendarChangesCursor(
                since = parameter("since"),
                version = parameter("v"),
                page = parameter("page"),
                perPage = parameter("per_page"),
            )
        }
    }
}

/** A calendar the changes feed reports gone. */
@Serializable
data class DeletedCalendar(
    /** The id the calendar had. */
    val id: Long,
    /** When it went. */
    @SerialName("deleted_at") val deletedAt: String,
)

/**
 * Everything that happened to the calendar list since a cursor. Added calendars arrive as
 * [ListedCalendar], so a new calendar comes with the changes URL and signed stream name a
 * live follower needs.
 *
 * [nextPage] is set while this increment has more pages to read now. [nextCursor] is set on
 * the last page and is where the next read should resume; it is null when nothing changed,
 * in which case the cursor that produced this page still stands. Unlike the recording feed,
 * this one never falls too far behind, so there is no full sync to ask for.
 */
data class CalendarChanges(
    /** The calendars that appeared, each with what a live follower needs. */
    val added: List<ListedCalendar> = emptyList(),
    /** The calendars that changed. */
    val updated: List<Calendar> = emptyList(),
    /** The calendars that went. */
    val deleted: List<DeletedCalendar> = emptyList(),
    /** The next page of this increment, while it has one: a whole cursor, as HEY issued it. */
    val nextPage: CalendarChangesCursor? = null,
    /** Where the next read resumes, once the increment is read to its end. */
    val nextCursor: CalendarChangesCursor? = null,
)

/** A recording the changes feed reports gone. [type] is the recordable type key the recording was grouped under while it existed. */
@Serializable
data class DeletedRecording(
    /** The id the recording had. */
    val id: Long,
    /** When it went. */
    @SerialName("deleted_at") val deletedAt: String,
    /** The recordable type key it was grouped under, `Calendar::Event` and the like. */
    val type: String? = null,
)

/**
 * Everything that happened to a calendar's recordings since a cursor.
 *
 * [added] and [updated] keep the wire's grouping by recordable type key —
 * `Calendar::Event`, `Calendar::Habit`, `Calendar::Habit::Completion`, `Calendar::DayTitle`,
 * `Calendar::DayBackground`, `Calendar::TimeTrack`, `Calendar::Todo`, `Calendar::Countdown`,
 * `Calendar::JournalEntry` — the server owns that vocabulary. [deleted] is one deduplicated
 * list instead: the wire groups deletions by type key too, but repeats the whole deleted
 * collection under every key it groups, so the map shape carries nothing beyond each
 * record's own `type`, which is authoritative.
 *
 * [nextPage] is set while this increment has more pages to read now. [nextCursor] is set on
 * the last page and is where the next read should resume; it is null when nothing changed,
 * in which case the cursor that produced this page still stands. [fullSyncRequired] is set
 * when the cursor is too far behind for an increment to carry the difference — or speaks a
 * version the feed no longer does — and the calendar has to be read in full instead;
 * nothing else is set then.
 */
data class RecordingChanges(
    /** The recordings that appeared, grouped by recordable type key. */
    val added: Map<String, List<Recording>> = emptyMap(),
    /** The recordings that changed, grouped by recordable type key. */
    val updated: Map<String, List<Recording>> = emptyMap(),
    /** The recordings that went, each once. */
    val deleted: List<DeletedRecording> = emptyList(),
    /** The next page of this increment, while it has one: a whole cursor, as HEY issued it. */
    val nextPage: CalendarChangesCursor? = null,
    /** Where the next read resumes, once the increment is read to its end. */
    val nextCursor: CalendarChangesCursor? = null,
    /** Whether the cursor is too far behind and the calendar has to be read in full. */
    val fullSyncRequired: Boolean = false,
)
