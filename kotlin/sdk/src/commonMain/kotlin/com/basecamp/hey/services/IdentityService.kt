package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.models.FirstWeekDayParams
import com.basecamp.hey.generated.models.UpdateFirstWeekDayRequestContent
import com.basecamp.hey.generated.models.UpdateTimeFormatRequestContent
import com.basecamp.hey.generated.services.IdentityService as GeneratedIdentityService

/**
 * A day of the week, as HEY's identity preferences take and answer it. HEY takes the day as
 * its lowercased English name and answers it as an index the way it serves the identity:
 * 0 is Sunday.
 */
enum class Weekday(
    /** The day as HEY's `first_week_day` parameter names it. */
    val wire: String,
    /** The day as HEY's identity numbers it, Sunday being 0. */
    val index: Int,
) {
    SUNDAY("sunday", 0),
    MONDAY("monday", 1),
    TUESDAY("tuesday", 2),
    WEDNESDAY("wednesday", 3),
    THURSDAY("thursday", 4),
    FRIDAY("friday", 5),
    SATURDAY("saturday", 6),
    ;

    companion object {
        /** The day HEY's identity numbers [index], or null for a number that is not a day of the week. */
        fun fromIndex(index: Int): Weekday? = entries.firstOrNull { it.index == index }
    }
}

/** The clock HEY renders times on. */
enum class TimeFormat(
    /** The format as HEY's identity names it. */
    val wire: String,
) {
    /** A 12-hour clock, HEY's `twelve_hour`. */
    TWELVE_HOUR("twelve_hour"),

    /** A 24-hour clock, HEY's `twenty_four_hour`. */
    TWENTY_FOUR_HOUR("twenty_four_hour"),
    ;

    companion object {
        /** Reads a format as HEY writes it. */
        fun parse(source: String): TimeFormat =
            entries.firstOrNull { it.wire == source }
                ?: throw HeyException.Usage("time format \"$source\" is neither \"twelve_hour\" nor \"twenty_four_hour\"")
    }
}

/** The two identity preferences HEY lets a client write, on top of the generated surface (`get`, `getNavigation`, `updateFirstWeekDay`, `updateTimeFormat`). */
class IdentityService(client: HeyClient) : GeneratedIdentityService(client) {
    /**
     * Sets which day the identity's calendar weeks start on, and answers the day HEY stored.
     * The write reaches every HEY client — web, mobile and this SDK read the same identity
     * preference.
     */
    suspend fun setFirstWeekDay(day: Weekday): Weekday {
        val stored = updateFirstWeekDay(UpdateFirstWeekDayRequestContent(FirstWeekDayParams(firstWeekDay = day.wire)))
        return Weekday.fromIndex(stored.firstWeekDay)
            ?: throw HeyException.Api("first week day ${stored.firstWeekDay} is not a day of the week", httpStatus = null, retryable = false)
    }

    /** Sets whether HEY renders times on a 12-hour or a 24-hour clock, and answers the format HEY stored. */
    suspend fun setTimeFormat(format: TimeFormat): TimeFormat {
        val stored = updateTimeFormat(UpdateTimeFormatRequestContent(twentyFourHourTimeFormat = format == TimeFormat.TWENTY_FOUR_HOUR))
        return TimeFormat.parse(stored.timeFormat)
    }
}
