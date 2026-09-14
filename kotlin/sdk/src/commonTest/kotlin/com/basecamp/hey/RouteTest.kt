package com.basecamp.hey

import com.basecamp.hey.generated.Routes
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class RouteTest {
    @Test
    fun everyOperationHasOneRoute() {
        assertEquals(131, Routes.ALL.size)
        assertEquals(Routes.ALL.size, Routes.ALL.map { it.id }.toSet().size)
        assertTrue(Routes.ALL.all { it.retry.max >= 0 })
        val stage = Routes.GET_WORKFLOW_STAGE
        assertTrue(stage.html)
        assertEquals(listOf(404), Routes.GET_ONGOING_TIME_TRACK.emptyOn)
        assertEquals(Pagination.LINK, Routes.LIST_BOXES.pagination)
        assertEquals(false, Routes.UPDATE_MESSAGE.idempotent)
        assertEquals(true, Routes.COMPLETE_CALENDAR_TODO.idempotent)
        assertEquals("Clearances", Routes.GET_CLEARANCES.service)
        assertEquals("Contacts", Routes.GET_CLEARANCES.resource)
    }

    @Test
    fun fillingAndRecognizingAPath() {
        val route = Routes.GET_BOX_GROUP
        assertEquals("/boxes/1/groups/2", route.fill(listOf(1, 2)))
        assertFailsWith<HeyException.Usage> { route.fill(listOf(1)) }
        assertEquals(mapOf("boxId" to "1", "groupId" to "2"), route.recognize("/boxes/1/groups/2"))
        assertNull(route.recognize("/boxes/1/groups"))
        assertNull(route.recognize("/boxes//groups/2"))
        assertEquals(listOf(ParamRole.PARENT, ParamRole.RECORDING), route.params.map { it.role })
        assertEquals("/calendar/days/2026-03-04/habits/7/completions", Routes.COMPLETE_HABIT.fill(listOf("2026-03-04", 7)))
        assertEquals("a%20b%2Fc%3Fd", encodePathSegment("a b/c?d"))
        assertEquals("caf%C3%A9", encodePathSegment("café"))
    }
}
