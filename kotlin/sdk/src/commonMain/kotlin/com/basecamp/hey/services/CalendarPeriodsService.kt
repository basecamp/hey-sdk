package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.generated.models.CalendarPeriod
import com.basecamp.hey.generated.models.CalendarYear
import com.basecamp.hey.generated.services.ListCalendarDaysOptions
import com.basecamp.hey.generated.services.ListCalendarWeeksOptions
import com.basecamp.hey.generated.services.CalendarPeriodsService as GeneratedCalendarPeriodsService

/**
 * Calendar periods service: the calendar as the periods it is drawn in — a day, a week, a
 * year — on top of the generated surface. Every read is scoped to the calendars the reader
 * has switched on, which [CalendarsService.toggleSelection] changes.
 *
 * A period is not the same answer as [CalendarsService.getRecordings]. A calendar lists the
 * recordings it holds, recurring ones included as the single rows they are stored as; a
 * period expands those into the occurrences that fall inside its window. Draw a week from a
 * calendar's recordings and a weekly meeting shows up once.
 */
class CalendarPeriodsService(client: HeyClient) : GeneratedCalendarPeriodsService(client) {
    /**
     * Reads one day. The date is `YYYY-MM-DD`, or the literal `now` for today, which leaves
     * it to HEY to decide what today is where the reader is.
     */
    suspend fun day(date: String): CalendarPeriod = getDay(date)

    /**
     * Reads the days from a date onwards. HEY picks how many, so this is a window rather
     * than a page: read on by asking again from the last day it answered. No date starts
     * from today.
     */
    suspend fun days(startsAt: String? = null): List<CalendarPeriod> =
        listDays(ListCalendarDaysOptions(startsAt = optional(startsAt))).days

    /** Reads the week a date falls in. The date is `YYYY-MM-DD`. */
    suspend fun week(date: String): CalendarPeriod = getWeek(date)

    /**
     * Reads nine weeks. [startsAt] names the first of them; [centeredAt] centers them on a
     * date instead, which is what the web app's scrolling week view asks for. Neither
     * centers on today, and HEY takes [startsAt] when it is given both.
     */
    suspend fun weeks(startsAt: String? = null, centeredAt: String? = null): List<CalendarPeriod> =
        listWeeks(ListCalendarWeeksOptions(startsAt = optional(startsAt), centeredAt = optional(centeredAt))).weeks

    /**
     * Reads the year a date falls in, as the grid it is drawn as: one entry per day and the
     * events that span more than one. A year does not carry every recording it holds.
     */
    suspend fun year(date: String): CalendarYear = getYear(date)
}

/** An empty date is left off the wire: sending `starts_at=` would ask HEY to parse an empty string rather than pick the default itself. */
private fun optional(value: String?): String? = value?.takeIf { it.isNotEmpty() }
