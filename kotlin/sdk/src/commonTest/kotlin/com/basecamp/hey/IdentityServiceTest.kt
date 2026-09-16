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
        val log = OperationLog()
        val client = hey.client { hooks = log }
        assertEquals(Weekday.MONDAY, client.identity.setFirstWeekDay(Weekday.MONDAY))
        assertEquals("Identity.UpdateFirstWeekDay:identity:true:null", log.started.last())
        assertEquals("PUT", hey.requests[0].method)
        assertEquals("/calendar/identity/first_week_day.json", hey.requests[0].path)
        assertEquals("""{"identity_preference":{"first_week_day":"monday"}}""", hey.requests[0].body)
        assertEquals(Weekday.SUNDAY, client.identity.setFirstWeekDay(Weekday.SUNDAY), "0 is Sunday, as the identity serves it")
        val error = assertFailsWith<HeyException.Api> { client.identity.setFirstWeekDay(Weekday.SATURDAY) }
        assertEquals("first week day 9 is not a day of the week", error.message)
        assertEquals("Identity.UpdateFirstWeekDay:api_error", log.ended.last(), "the hooks hear the failure the caller gets")
        assertEquals(3, log.ended.size)
        assertNull(Weekday.fromIndex(7))
        assertEquals(Weekday.SATURDAY, Weekday.fromIndex(6))
    }

    @Test
    fun theTimeFormatGoesOutAsTheWebTogglesFlagAndComesBackByName() = runTest {
        val hey = mockHey(ok("""{"time_format":"twenty_four_hour"}"""), ok("""{"time_format":"twelve_hour"}"""), ok("""{"time_format":"decimal"}"""))
        val log = OperationLog()
        val client = hey.client { hooks = log }
        assertEquals(TimeFormat.TWENTY_FOUR_HOUR, client.identity.setTimeFormat(TimeFormat.TWENTY_FOUR_HOUR))
        assertEquals("/identity/time_format.json", hey.requests[0].path)
        assertEquals("""{"twenty_four_hour_time_format":true}""", hey.requests[0].body)
        assertEquals(TimeFormat.TWELVE_HOUR, client.identity.setTimeFormat(TimeFormat.TWELVE_HOUR))
        assertEquals("""{"twenty_four_hour_time_format":false}""", hey.requests[1].body)
        val unread = assertFailsWith<HeyException.Api> { client.identity.setTimeFormat(TimeFormat.TWELVE_HOUR) }
        assertEquals("time format \"decimal\" is neither \"twelve_hour\" nor \"twenty_four_hour\"", unread.message, "a format HEY stored that the SDK cannot read is HEY's answer failing, not the caller's mistake")
        assertEquals("Identity.UpdateTimeFormat:api_error", log.ended.last())
        assertFailsWith<HeyException.Usage> { TimeFormat.parse("decimal") }
    }
}
