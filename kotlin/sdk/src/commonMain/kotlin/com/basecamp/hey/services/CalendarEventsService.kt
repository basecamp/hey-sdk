package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.services.DeleteCalendarEventOccurrenceOptions
import com.basecamp.hey.FormResponse
import com.basecamp.hey.writeInfo
import kotlin.time.Duration
import com.basecamp.hey.generated.services.CalendarEventsService as GeneratedCalendarEventsService

/**
 * A partial revision of an event, and partial only in what it names: the six fields here
 * are sent when set and HEY leaves the title, dates and times alone otherwise. The notes,
 * location, link, attached entry, attendees, reminders and countdown a caller says nothing
 * about are cleared, because HEY reads every calendar write as a form and defaults each of
 * those to nothing. A revision that keeps any of them goes through [UpdateCalendarEventParams]
 * and [CalendarEventsService.updateEvent], which take them all.
 *
 * Clock times belong to a timed event, so an all-day revision leaves them off however they
 * are set here.
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
 * An event's content, which is a replacement rather than a patch.
 *
 * HEY reads these four out of the submitted parameters and then defaults every one of them
 * to nothing, so a write that says nothing about a field clears it. There is no way to send
 * a subset: the fields left empty here are the fields the event loses. An update therefore
 * has to read the event first and pass back whatever it means to keep.
 *
 * The title is not in here, and that is not an oversight: HEY leaves the summary alone when
 * it is not submitted, so it stays a partial field like the rest of a partial write.
 */
data class EventContent(
    /**
     * HEY's `calendar_event[description]` — Trix rich text, so HTML going in. It does not
     * round-trip: HEY serves the notes back as plain text, so keeping formatted notes through
     * an update means holding the HTML the caller sent, not the text HEY answered.
     */
    val notes: String = "",
    /** A plain string. HEY truncates it at 3900 characters rather than refusing it. */
    val location: String = "",
    /** Validated as a URL and capped at 2500 characters, so a malformed one is a 422 rather than a silent drop. */
    val link: String? = null,
    /** The email attached to the event, HEY's `calendar_event[entry_id]`. A read serves it back as `attached_entry`. */
    val entryId: Long? = null,
)

/** A countdown's unit, written as the number of seconds HEY's own form submits and the only form it reads. */
enum class CountdownUnit(val seconds: Int) {
    /** A day: 86,400 seconds. */
    DAYS(86_400),

    /** A week: 604,800 seconds. */
    WEEKS(604_800),

    /** A month as HEY averages one: 2,629,746 seconds. */
    MONTHS(2_629_746),
}

/**
 * The countdown HEY runs up to an event. Like [EventContent] it is resend-or-lose-it: HEY
 * reads the pair on every editable write and a missing value deletes the countdown, so a
 * zero [value] means the event has none once the write lands. A countdown is a child
 * recording rather than a field on the event, so it cannot be read back from one. 1 through
 * 30 is what the web app offers.
 */
data class Countdown(
    /** How many units. Zero is no countdown. */
    val value: Int = 0,
    /** What the value counts in. */
    val unit: CountdownUnit = CountdownUnit.DAYS,
)

/**
 * How often an event repeats. HEY has no day-of-week parameter, so [EVERY_WEEKDAY] — a
 * hardcoded Monday to Friday — is the only weekday set that can be expressed.
 */
enum class RepeatFrequency(val wire: String) {
    EVERY_DAY("every_day"),
    EVERY_WEEKDAY("every_weekday"),
    EVERY_WEEK("every_week"),
    EVERY_OTHER_WEEK("every_other_week"),
    EVERY_DAY_OF_MONTH("every_day_of_month"),
    EVERY_YEAR("every_year"),

    /** Keeps whatever schedule the event already has instead of naming a new one. */
    CUSTOM("custom"),
}

/** When a recurrence stops. */
enum class RepeatUntil(val wire: String) {
    /** The event never stops repeating. */
    FOREVER("forever"),

    /** It stops after [Repeat.untilDate]. */
    DATE("date"),

    /** It stops after [Repeat.count] occurrences. */
    COUNT("count"),
}

/**
 * An event's recurrence. The default is [RepeatFrequency.CUSTOM] with no end, which says
 * "keep the schedule the event already has". On an occurrence update that makes a default
 * [Repeat] and a null one mean the same thing, since [CalendarEventsService.updateOccurrence]
 * sends `custom` for a null one anyway. On a whole-event update a null writes no recurrence
 * field at all.
 */
data class Repeat(
    val frequency: RepeatFrequency = RepeatFrequency.CUSTOM,
    /** When it stops. Null says nothing about an end. */
    val until: RepeatUntil? = null,
    /** `YYYY-MM-DD`, read only when [until] is [RepeatUntil.DATE]. */
    val untilDate: String? = null,
    /** Read only when [until] is [RepeatUntil.COUNT]. */
    val count: Int? = null,
)

/**
 * A new calendar event. Nothing exists to lose on a create, so the resend-or-lose-it fields
 * of an update — the content, reminders and countdown — default to an event with none of
 * them.
 */
data class CreateCalendarEventParams(
    /** The calendar the event is filed on. */
    val calendarId: Long,
    /** The event's title, HEY's summary. */
    val title: String,
    /** `YYYY-MM-DD`. */
    val startsAt: String,
    /** `YYYY-MM-DD`. Defaults to [startsAt]. */
    val endsAt: String? = null,
    /** Whether the event takes the whole day rather than a clock time. */
    val allDay: Boolean = false,
    /** `HH:MM`, required unless the event is all-day. */
    val startTime: String? = null,
    /** `HH:MM`, required unless the event is all-day. */
    val endTime: String? = null,
    /**
     * The IANA name of the zone the start is written in — `Europe/Zagreb`, `America/New_York`.
     * Leave both zones null and the times are read in UTC, which is the zone HEY parses an
     * API request in. HEY keeps a zone per end, as its own form offers, so an event can start
     * in one and finish in another; one zone named stands in for the other.
     */
    val startTimeZone: String? = null,
    /** The zone the end is written in, read as [startTimeZone] is. */
    val endTimeZone: String? = null,
    /**
     * How long before the event each reminder goes out. HEY takes several in one write and
     * de-duplicates them, and accepts any duration rather than only the presets the web app
     * offers. Only the list matching [allDay] is read, and an empty list is an event with no
     * reminders.
     */
    val reminders: List<Duration> = emptyList(),
    /** The notes, location, link and attached entry. */
    val content: EventContent = EventContent(),
    /** The guest list. Submitting one makes the caller the organizer and sends invitations. */
    val attendees: List<String>? = null,
    /** Circles the event. HEY reads it only when it is submitted, so null is "not circled" on a create. */
    val highlighted: Boolean? = null,
    /** Counts down to the event. The default creates none. */
    val countdown: Countdown = Countdown(),
    /** Makes the event recurring. Null is a one-off. */
    val repeat: Repeat? = null,
)

/**
 * A revision of a calendar event, whole. The nullable fields are a partial update: only the
 * ones named are sent, and HEY leaves the rest as they are.
 *
 * The rest are not, and the reason is on HEY's side. It reads the zones, the content, the
 * reminders and the countdown out of the submitted parameters on every write and defaults
 * each of them to nothing, so an update saying nothing about one clears it. A caller keeping
 * any of them reads the event and sends them back.
 */
data class UpdateCalendarEventParams(
    /**
     * Moves the event to another calendar. It has to be one the identity can file on — owned
     * or shared, not a subscription; the personal calendar answers 404 all the same.
     */
    val calendarId: Long? = null,
    val title: String? = null,
    /** `YYYY-MM-DD`. */
    val startsAt: String? = null,
    /** `YYYY-MM-DD`. */
    val endsAt: String? = null,
    val allDay: Boolean? = null,
    /** `HH:MM`. Clock times belong to a timed event, so an all-day revision leaves them off however they are set here. */
    val startTime: String? = null,
    /** `HH:MM`. */
    val endTime: String? = null,
    /**
     * The IANA zone the start is written in, as on a create. An empty string says the time
     * is UTC and clears the zone the event was saved with; null leaves it out of the request,
     * which HEY also reads as clearing it.
     */
    val startTimeZone: String? = null,
    /** The zone the end is written in, read as [startTimeZone] is. */
    val endTimeZone: String? = null,
    /**
     * Resend-or-lose-it, like the zones: HEY reads the list on every write and unschedules
     * everything when it is empty. Several go in one write and HEY de-duplicates them; only
     * the list matching the event's all-day flag is read.
     */
    val reminders: List<Duration> = emptyList(),
    /** The notes, location, link and attached entry, a replacement rather than a patch. Read [EventContent] before using it. */
    val content: EventContent = EventContent(),
    /** Replaces the guest list. Null leaves it alone; an empty list removes every guest. */
    val attendees: List<String>? = null,
    /** Circles or uncircles the event. Null leaves it as it is. */
    val highlighted: Boolean? = null,
    /** Resend-or-lose-it too: a zero value deletes the event's countdown. */
    val countdown: Countdown = Countdown(),
    /** Changes the recurrence. Null leaves it untouched on a whole-event update, and keeps the series' schedule on an occurrence update. */
    val repeat: Repeat? = null,
)

/**
 * One day of a repeating event, addressed by the series it belongs to plus the day it falls
 * on, as HEY's `occurrence_id` (`<event id>_<YYYY-MM-DD>`) names it.
 */
data class OccurrenceId(val eventId: Long, val date: String) {
    init {
        // Both parts go into an authenticated write's path, so both are checked here, whichever
        // way the id was made: a positive event, and a day the calendar actually has.
        if (eventId <= 0) throw HeyException.Usage("occurrence id names no event: $eventId")
        if (!isCalendarDate(date)) throw HeyException.Usage("occurrence id names no date: \"$date\" is not a YYYY-MM-DD the calendar has")
    }

    companion object {
        /** Reads HEY's `occurrence_id`. */
        fun parse(source: String): OccurrenceId {
            val separator = source.indexOf('_')
            if (separator < 0) throw HeyException.Usage("occurrence id \"$source\" is not <event id>_<YYYY-MM-DD>")
            val eventId = source.substring(0, separator).toLongOrNull()?.takeIf { it > 0 }
                ?: throw HeyException.Usage("occurrence id \"$source\" names no event")
            return OccurrenceId(eventId, source.substring(separator + 1))
        }

        /** Whether the text is a `YYYY-MM-DD` the calendar has: a month of 1 to 12 and a day that month has, February 29 in leap years only. */
        internal fun isCalendarDate(date: String): Boolean {
            val match = Regex("(\\d{4})-(\\d{2})-(\\d{2})").matchEntire(date) ?: return false
            val (year, month, day) = match.destructured.toList().map { it.toInt() }
            if (month !in 1..12 || day < 1) return false
            val leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
            val days = when (month) {
                2 -> if (leap) 29 else 28
                4, 6, 9, 11 -> 30
                else -> 31
            }
            return day <= days
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
 * Calendar events service with the form-backed writes on top of the generated surface
 * (`delete`, `deleteOccurrence`). HEY's calendar writes are Rails form posts rather than
 * JSON, so a create or an update goes out form-encoded under `calendar_event[...]`.
 * [updateEvent] and [updateOccurrence] take the whole of an event, as Go's and Rust's do;
 * [update] names the six fields a revision usually means, and clears the rest — read it
 * before using it.
 */
class CalendarEventsService(client: HeyClient) : GeneratedCalendarEventsService(client) {
    /**
     * Creates an event and answers it as a recording. An event needs a title and a day, and
     * a timed one both clock times; those are refused here rather than sent for HEY to
     * refuse.
     */
    suspend fun create(params: CreateCalendarEventParams): Recording {
        if (params.title.isEmpty()) throw HeyException.Usage("a calendar event needs a title")
        if (params.startsAt.isEmpty()) throw HeyException.Usage("a calendar event needs a day: startsAt is required")
        if (!params.allDay && (params.startTime.isNullOrEmpty() || params.endTime.isNullOrEmpty())) {
            throw HeyException.Usage("a timed calendar event needs a start and an end time; all-day events take neither")
        }
        return write(
            Method.POST,
            "/calendar/events.json",
            writeInfo("CalendarEvents", "CreateCalendarEvent", "calendar_event"),
            createFields(params),
        )
    }

    /**
     * Revises an event's title, dates and times, and nothing else — partial only in what it
     * names. HEY reads a calendar write out of submitted form parameters and defaults the
     * notes, location, link, attached entry, attendees, reminders and countdown to nothing,
     * so every one of those the event had is cleared by this call. A revision that keeps any
     * of them goes through [updateEvent], which takes them all; read the event first and
     * send back what it should keep.
     */
    suspend fun update(eventId: Long, update: CalendarEventUpdate) {
        val operation = client.request(Method.PATCH, "/calendar/events/$eventId")
        operation.info(writeInfo("CalendarEvents", "UpdateCalendarEvent", "calendar_event", eventId))
        operation.form(updateFields(update))
        operation.accept("application/json")
        client.sendUnit(operation)
    }

    /** Revises an event from the whole of [params] and answers it as a recording. */
    suspend fun updateEvent(eventId: Long, params: UpdateCalendarEventParams): Recording =
        write(
            Method.PATCH,
            "/calendar/events/$eventId.json",
            writeInfo("CalendarEvents", "UpdateCalendarEvent", "calendar_event", eventId),
            updateEventFields(params),
        )

    /**
     * Revises one day of a repeating event and answers it as a recording. A date that is not
     * an occurrence of that series is a 404, as is an event the caller cannot edit. A null
     * [UpdateCalendarEventParams.repeat] keeps the series' schedule: HEY would read silence
     * here as "stop repeating".
     */
    suspend fun updateOccurrence(occurrence: OccurrenceId, scope: OccurrenceScope, params: UpdateCalendarEventParams): Recording {
        val fields = updateEventFields(params).toMutableList()
        fields += "apply_to_future" to checkbox(scope == OccurrenceScope.THIS_AND_FOLLOWING)
        if (params.repeat == null) fields += "repeat_frequency" to RepeatFrequency.CUSTOM.wire
        return write(
            Method.PATCH,
            "/calendar/events/${occurrence.eventId}/occurrences/${occurrence.date}.json",
            writeInfo("CalendarEvents", "UpdateCalendarEventOccurrence", "calendar_event", occurrence.eventId),
            fields,
        )
    }

    /**
     * Posts a calendar form to a `.json` path and reads the recording it answers, inside the
     * operation the hooks hear. An older server answers a redirect instead, whose URL still
     * names the recording's id; the type is not in it, so it stays empty as Go's and Rust's do.
     */
    private suspend fun write(method: Method, path: String, info: OperationInfo, fields: List<Pair<String, String>>): Recording {
        val operation = client.form(method, path)
        operation.info(info)
        operation.form(fields)
        return client.execute(operation) { response ->
            val answered = FormResponse.of(response)
            if (answered.body.isEmpty()) Recording(id = answered.extractId(), type = "") else response.json(Recording.serializer())
        }
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

private const val ATTENDEES = "calendar_event[attendance_email_addresses][]"
private const val ALL_DAY_REMINDERS = "all_day_reminder_durations[]"
private const val TIMED_REMINDERS = "timed_reminder_durations[]"

private fun checkbox(value: Boolean): String = if (value) "1" else "0"

/** Form-encodes a new event. */
internal fun createFields(params: CreateCalendarEventParams): List<Pair<String, String>> {
    val fields = mutableListOf<Pair<String, String>>()
    fields += "calendar_event[calendar_id]" to params.calendarId.toString()
    fields += "calendar_event[summary]" to params.title
    fields += "calendar_event[starts_at]" to params.startsAt
    fields += "calendar_event[ends_at]" to (params.endsAt?.takeIf { it.isNotEmpty() } ?: params.startsAt)
    fields.addContent(params.content)
    fields.addAttendees(params.attendees)
    fields.addHighlighted(params.highlighted)
    fields.addCountdown(params.countdown)
    fields.addRepeat(params.repeat)
    if (params.allDay) {
        fields += "calendar_event[all_day]" to checkbox(true)
        fields.addReminders(ALL_DAY_REMINDERS, params.reminders)
    } else {
        fields += "calendar_event[all_day]" to checkbox(false)
        fields += "calendar_event[starts_at_time]" to "${params.startTime}:00"
        fields += "calendar_event[ends_at_time]" to "${params.endTime}:00"
        // A create always says what it means about the zones: naming none is "read the times in UTC".
        fields.addTimeZones(params.startTimeZone.orEmpty(), params.endTimeZone.orEmpty())
        fields.addReminders(TIMED_REMINDERS, params.reminders)
    }
    return fields
}

/** Form-encodes a whole-event update; the occurrence update builds on it. */
internal fun updateEventFields(params: UpdateCalendarEventParams): List<Pair<String, String>> {
    val fields = mutableListOf<Pair<String, String>>()
    params.title?.let { fields += "calendar_event[summary]" to it }
    params.startsAt?.let { fields += "calendar_event[starts_at]" to it }
    params.endsAt?.let { fields += "calendar_event[ends_at]" to it }
    params.allDay?.let { fields += "calendar_event[all_day]" to checkbox(it) }
    if (params.allDay != true) {
        params.startTime?.let { fields += "calendar_event[starts_at_time]" to "$it:00" }
        params.endTime?.let { fields += "calendar_event[ends_at_time]" to "$it:00" }
    }
    params.calendarId?.let { fields += "calendar_event[calendar_id]" to it.toString() }
    fields.addContent(params.content)
    fields.addAttendees(params.attendees)
    fields.addHighlighted(params.highlighted)
    fields.addCountdown(params.countdown)
    fields.addRepeat(params.repeat)

    // Naming no zone on an update says nothing about zones; naming one, or an empty one, is
    // an answer, and the empty answer is "convert to UTC".
    if (params.startTimeZone != null || params.endTimeZone != null) {
        fields.addTimeZones(params.startTimeZone.orEmpty(), params.endTimeZone.orEmpty())
    }

    // HEY reads the list matching the event's all-day flag as it stands after the write. An
    // update that leaves the flag alone cannot know which that is, so it sends both lists: the
    // one HEY does not read is ignored, and the one it does keeps the reminders scheduled.
    val remindersKeys = when (params.allDay) {
        true -> listOf(ALL_DAY_REMINDERS)
        false -> listOf(TIMED_REMINDERS)
        null -> listOf(ALL_DAY_REMINDERS, TIMED_REMINDERS)
    }
    for (key in remindersKeys) fields.addReminders(key, params.reminders)
    return fields
}

/** The content goes out whole every time, because HEY clears what it is not sent. */
private fun MutableList<Pair<String, String>>.addContent(content: EventContent) {
    this += "calendar_event[description]" to content.notes
    this += "calendar_event[location]" to content.location
    this += "calendar_event[url]" to content.link.orEmpty()
    // A zero id names no entry, so it goes out blank rather than as "0", which HEY would look up and refuse.
    this += "calendar_event[entry_id]" to (content.entryId?.takeIf { it != 0L }?.toString().orEmpty())
}

/**
 * The guest list is replaced wholesale when it is submitted at all; an empty list needs a
 * blank value on the wire to say so, since a form carries no empty array.
 */
private fun MutableList<Pair<String, String>>.addAttendees(attendees: List<String>?) {
    val addresses = attendees ?: return
    if (addresses.isEmpty()) this += ATTENDEES to "" else addresses.forEach { this += ATTENDEES to it }
}

/**
 * The empty highlight_id is what makes "off" mean off: HEY destroys the existing highlight
 * when the key is there and empty, and builds one when the flag is on.
 */
private fun MutableList<Pair<String, String>>.addHighlighted(highlighted: Boolean?) {
    val circled = highlighted ?: return
    this += "calendar_event[highlighted]" to checkbox(circled)
    this += "calendar_event[highlight_id]" to ""
}

/** A zero countdown sends no value at all, which is how HEY is told to delete it. */
private fun MutableList<Pair<String, String>>.addCountdown(countdown: Countdown) {
    if (countdown.value <= 0) return
    this += "countdown_interval_duration_value" to countdown.value.toString()
    this += "countdown_interval_duration_unit" to countdown.unit.seconds.toString()
}

private fun MutableList<Pair<String, String>>.addRepeat(repeat: Repeat?) {
    if (repeat == null) return
    this += "repeat_frequency" to repeat.frequency.wire
    repeat.until?.let { this += "calendar_recurrence_schedule[recurs_until_type]" to it.wire }
    if (repeat.until == RepeatUntil.DATE) this += "calendar_recurrence_schedule[recurs_until_date]" to repeat.untilDate.orEmpty()
    if (repeat.until == RepeatUntil.COUNT) this += "calendar_recurrence_schedule[recurs_count]" to (repeat.count ?: 0).toString()
}

/**
 * The zones, and the flag that makes HEY honour them: without it both names are dropped
 * and the times are read in UTC. Naming none is a complete answer — convert to UTC — and
 * one zone named stands in for the other.
 */
private fun MutableList<Pair<String, String>>.addTimeZones(start: String, end: String) {
    if (start.isEmpty() && end.isEmpty()) {
        this += "calendar_event[set_time_zone]" to checkbox(false)
        return
    }
    this += "calendar_event[set_time_zone]" to checkbox(true)
    this += "calendar_event[starts_at_time_zone_name]" to start.ifEmpty { end }
    this += "calendar_event[ends_at_time_zone_name]" to end.ifEmpty { start }
}

private fun MutableList<Pair<String, String>>.addReminders(key: String, reminders: List<Duration>) {
    for (reminder in reminders) this += key to reminder.inWholeSeconds.toString()
}
