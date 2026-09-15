package com.basecamp.hey

import com.basecamp.hey.generated.identity
import com.basecamp.hey.services.TimeFormat
import com.basecamp.hey.services.Weekday
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class IdentityServiceTest {
    @Test
    fun theFirstWeekDayGoesOutByNameAndComesBackByNumber() = runTest {
        val hey = mockHey(ok("""{"first_week_day":1}"""), ok("""{"first_week_day":0}"""), ok("""{"first_week_day":9}"""))
        val client = hey.client()
        assertEquals(Weekday.MONDAY, client.identity.setFirstWeekDay(Weekday.MONDAY))
        assertEquals("PUT", hey.requests[0].method)
        assertEquals("/calendar/identity/first_week_day.json", hey.requests[0].path)
        assertEquals("""{"identity_preference":{"first_week_day":"monday"}}""", hey.requests[0].body)
        assertEquals(Weekday.SUNDAY, client.identity.setFirstWeekDay(Weekday.SUNDAY), "0 is Sunday, as the identity serves it")
        val error = assertFailsWith<HeyException.Api> { client.identity.setFirstWeekDay(Weekday.SATURDAY) }
        assertEquals("first week day 9 is not a day of the week", error.message)
        assertNull(Weekday.fromIndex(7))
        assertEquals(Weekday.SATURDAY, Weekday.fromIndex(6))
    }

    @Test
    fun theTimeFormatGoesOutAsTheWebTogglesFlagAndComesBackByName() = runTest {
        val hey = mockHey(ok("""{"time_format":"twenty_four_hour"}"""), ok("""{"time_format":"twelve_hour"}"""), ok("""{"time_format":"decimal"}"""))
        val client = hey.client()
        assertEquals(TimeFormat.TWENTY_FOUR_HOUR, client.identity.setTimeFormat(TimeFormat.TWENTY_FOUR_HOUR))
        assertEquals("/identity/time_format.json", hey.requests[0].path)
        assertEquals("""{"twenty_four_hour_time_format":true}""", hey.requests[0].body)
        assertEquals(TimeFormat.TWELVE_HOUR, client.identity.setTimeFormat(TimeFormat.TWELVE_HOUR))
        assertEquals("""{"twenty_four_hour_time_format":false}""", hey.requests[1].body)
        assertFailsWith<HeyException.Usage> { client.identity.setTimeFormat(TimeFormat.TWELVE_HOUR) }
    }
}
