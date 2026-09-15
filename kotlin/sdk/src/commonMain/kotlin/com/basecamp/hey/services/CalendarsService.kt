package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.Calendar
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.isSameOrigin
import io.ktor.http.Url
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import com.basecamp.hey.generated.services.CalendarsService as GeneratedCalendarsService

/**
 * A calendar as the index serves it, wrapped with what a live follower needs.
 *
 * [recordingChangesUrl] is where the calendar's own recording changes feed starts; read it
 * with [CalendarChangesCursor.fromUrl]. [signedStreamName] subscribes the calendar's stream
 * over Action Cable — a frame arriving there means the calendar changed, and the name is
 * stable for the calendar's life. The calendar changes feed's added bucket carries this
 * same shape, so a calendar learned of either way arrives subscribable.
 */
@Serializable
data class ListedCalendar(
    /** The calendar itself, as [CalendarsService.list] serves it. */
    val calendar: Calendar? = null,
    /** Where the calendar's recording changes feed starts. */
    @SerialName("recording_changes_url") val recordingChangesUrl: String? = null,
    /** The Action Cable stream that announces the calendar's changes. */
    @SerialName("signed_stream_name") val signedStreamName: String? = null,
)

/**
 * The full calendars index: every calendar with its changes URL and signed stream name,
 * the calendar-level changes feed's own URL, and the calendars the reader has switched on.
 */
@Serializable
data class CalendarList(
    /** Every calendar the identity sees, each with what a live follower needs. */
    val calendars: List<ListedCalendar> = emptyList(),
    /** Where the calendar-level changes feed starts. */
    @SerialName("calendar_changes_url") val calendarChangesUrl: String? = null,
    /** The calendars the reader has switched on, which every period read is scoped to. */
    @SerialName("selected_calendar_ids") val selectedCalendarIds: List<Long> = emptyList(),
)

/**
 * Calendars service with the index as a live follower needs it, the two change feeds, and
 * the selection every period read is scoped to, on top of the generated surface (`list`,
 * `toggle`, `getRecordings`).
 *
 * Neither feed's answers are cached. A cursor URL never repeats, so a cached response would
 * never be revalidated, and a long-running watch would grow the cache by one dead entry per
 * read.
 */
class CalendarsService(client: HeyClient) : GeneratedCalendarsService(client) {
    /**
     * Lists the calendars with everything [list] throws away: each calendar's recording
     * changes URL and signed stream name, and the calendar changes URL. It is the same read,
     * decoded into the fuller shape the wire already carries.
     */
    suspend fun listWithChanges(): CalendarList = client.send(client.operation(Routes.LIST_CALENDARS, emptyList()))

    /**
     * Switches a calendar in or out of the reader's selection and answers the ids the
     * selection is left holding. Every [CalendarPeriodsService] read is scoped to that
     * selection, so this is how a client changes which calendars a day, week or year is
     * drawn from.
     */
    suspend fun toggleSelection(calendarId: Long): List<Long> = toggle(calendarId).selectedCalendarIds

    /**
     * Reads the calendar changes feed from a cursor to its end, following the pages the feed
     * hands out. A walk that reaches the client's page limit with pages still to read stops
     * there and fails, as every walk in this SDK does, rather than answering a shorter list
     * that looks complete.
     */
    suspend fun allCalendarChanges(cursor: CalendarChangesCursor): CalendarChanges {
        val added = mutableListOf<ListedCalendar>()
        val updated = mutableListOf<Calendar>()
        val deleted = mutableListOf<DeletedCalendar>()
        var next = cursor
        repeat(client.config.maxPages) {
            val changes = calendarChanges(next)
            added += changes.added
            updated += changes.updated
            deleted += changes.deleted
            next = changes.nextPage ?: return CalendarChanges(added, updated, deleted, nextPage = null, nextCursor = changes.nextCursor)
        }
        throw pagesExhausted("calendar changes")
    }

    /** Reads one page of the calendar changes feed. */
    suspend fun calendarChanges(cursor: CalendarChangesCursor): CalendarChanges {
        if (cursor.since.isNullOrEmpty()) {
            throw HeyException.Usage("a since cursor is required — start from the list's calendarChangesUrl")
        }
        val operation = client.request(Method.GET, "/calendar/changes")
        operation.info(changesInfo("GetCalendarChanges", "calendar"))
        cursor.applyTo(operation)
        operation.noCache()
        val page = client.sendPage<CalendarChangesPayload>(operation)
        val (nextPage, nextCursor) = nextCursors(page.nextUrl, page.nextCursor)
        return CalendarChanges(
            added = page.value.added,
            updated = page.value.updated,
            deleted = page.value.deleted,
            nextPage = nextPage,
            nextCursor = nextCursor,
        )
    }

    /**
     * Reads a calendar's recording changes feed from a cursor to its end, following the
     * pages the feed hands out. A cursor the feed has left behind ends the walk on the spot
     * with [RecordingChanges.fullSyncRequired]; a walk that reaches the client's page limit
     * with pages still to read stops there and fails.
     */
    suspend fun allRecordingChanges(calendarId: Long, cursor: CalendarChangesCursor): RecordingChanges {
        val added = linkedMapOf<String, MutableList<Recording>>()
        val updated = linkedMapOf<String, MutableList<Recording>>()
        val deleted = mutableListOf<DeletedRecording>()
        var next = cursor
        repeat(client.config.maxPages) {
            val changes = recordingChanges(calendarId, next)
            if (changes.fullSyncRequired) return changes
            added.merge(changes.added)
            updated.merge(changes.updated)
            deleted += changes.deleted
            next = changes.nextPage ?: return RecordingChanges(added, updated, deleted, nextPage = null, nextCursor = changes.nextCursor)
        }
        throw pagesExhausted("recording changes")
    }

    /** Reads one page of a calendar's recording changes feed. */
    suspend fun recordingChanges(calendarId: Long, cursor: CalendarChangesCursor): RecordingChanges {
        if (cursor.since.isNullOrEmpty()) {
            throw HeyException.Usage("a since cursor is required — start from the calendar's recordingChangesUrl")
        }
        if (cursor.version.isNullOrEmpty()) {
            throw HeyException.Usage("a feed version is required — read the calendar's recordingChangesUrl with CalendarChangesCursor.fromUrl")
        }
        val operation = client.request(Method.GET, "/calendars/$calendarId/recording/changes")
        operation.info(changesInfo("GetCalendarRecordingChanges", "recording"))
        operation.resourceId(calendarId)
        cursor.applyTo(operation)
        operation.noCache()
        val page = try {
            client.sendPage<RecordingChangesPayload>(operation)
        } catch (tooFarBehind: HeyException.Conflict) {
            // A 409 is the feed saying the cursor is too far behind for an increment to carry
            // the difference, or speaks a version it no longer does: an answer, not a failure.
            return RecordingChanges(fullSyncRequired = true)
        }
        val (nextPage, nextCursor) = nextCursors(page.nextUrl, page.nextCursor)
        return RecordingChanges(
            added = page.value.added,
            updated = page.value.updated,
            deleted = flattenDeletedRecordings(page.value.deleted),
            nextPage = nextPage,
            nextCursor = nextCursor,
            fullSyncRequired = false,
        )
    }

    /**
     * The cursors the feed's `Link` header names, the page one first: while an increment has
     * more pages the link carries a page cursor, and the last page carries a fresh `since`
     * cursor instead. Both are read whole, as HEY issued them, and a link off HEY's origin is
     * refused rather than followed.
     */
    private fun nextCursors(nextUrl: Url?, nextCursor: Url?): Pair<CalendarChangesCursor?, CalendarChangesCursor?> {
        val linked = nextUrl ?: nextCursor ?: return null to null
        if (!isSameOrigin(linked, client.baseUrl)) {
            throw HeyException.Usage("changes Link header points to a different origin: ${linked.protocol.name}://${linked.host}")
        }
        val cursor = CalendarChangesCursor.fromParsed(linked)
        return if (cursor.page != null) cursor to null else null to cursor
    }

    private fun pagesExhausted(feed: String): HeyException =
        HeyException.Api("$feed pagination stopped after ${client.config.maxPages} pages with more to read", httpStatus = null, retryable = false)
}

private fun changesInfo(operation: String, resourceType: String): OperationInfo =
    OperationInfo(service = "Calendars", operation = operation, resourceType = resourceType, isMutation = false)

private fun MutableMap<String, MutableList<Recording>>.merge(from: Map<String, List<Recording>>) {
    for ((key, recordings) in from) getOrPut(key) { mutableListOf() } += recordings
}

/**
 * Folds the wire's per-type deleted buckets into one list. The server repeats the whole
 * deleted collection under every type key it groups, so the same deletion arrives once per
 * key: the id dedupe drops the repeats, and each record's own `type` says what it was.
 */
internal fun flattenDeletedRecordings(buckets: Map<String, List<DeletedRecording>>): List<DeletedRecording> {
    val seen = mutableSetOf<Long>()
    return buckets.values.flatten().filter { seen.add(it.id) }
}

@Serializable
internal data class CalendarChangesPayload(
    val added: List<ListedCalendar> = emptyList(),
    val updated: List<Calendar> = emptyList(),
    val deleted: List<DeletedCalendar> = emptyList(),
)

@Serializable
internal data class RecordingChangesPayload(
    val added: Map<String, List<Recording>> = emptyMap(),
    val updated: Map<String, List<Recording>> = emptyMap(),
    val deleted: Map<String, List<DeletedRecording>> = emptyMap(),
)
