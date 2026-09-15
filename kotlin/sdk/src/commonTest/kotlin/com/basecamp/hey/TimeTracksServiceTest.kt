package com.basecamp.hey

import com.basecamp.hey.generated.timeTracks
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class TimeTracksServiceTest {
    @Test
    fun aCategoryIsMadeRenamedAndRemovedAsForms() = runTest {
        val redirect = status(302, headers = mapOf("Location" to "/calendar/time_tracks/categories"))
        val hey = mockHey(redirect, redirect, redirect)
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.timeTracks.createCategory("Client work")
        client.timeTracks.updateCategory(31, "Billable")
        client.timeTracks.deleteCategory(31)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/calendar/time_tracks/categories", hey.requests[0].path)
        assertEquals("application/x-www-form-urlencoded", hey.requests[0].header("Content-Type"))
        assertEquals(BROWSER_ACCEPT_HEADER, hey.requests[0].header("Accept"))
        assertEquals(listOf("category[title]" to "Client work"), formPairs(hey.requests[0].body))
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals("/calendar/time_tracks/categories/31", hey.requests[1].path)
        assertEquals(listOf("category[title]" to "Billable"), formPairs(hey.requests[1].body))
        assertEquals("DELETE", hey.requests[2].method)
        assertEquals("/calendar/time_tracks/categories/31", hey.requests[2].path)
        assertEquals("", hey.requests[2].body)
        assertEquals(
            listOf(
                "TimeTracks.CreateTimeTrackCategory:category:true:null",
                "TimeTracks.UpdateTimeTrackCategory:category:true:31",
                "TimeTracks.DeleteTimeTrackCategory:category:true:31",
            ),
            log.started,
        )
    }

    @Test
    fun theExportIsTheCsvHeyStreamedFromTheBarePath() = runTest {
        val csv = "Start,End,Duration,Category,Notes\n2026-04-06 09:00,2026-04-06 11:00,2:00,Client work,Kitchen remodel call\n"
        val hey = mockHey(Answer(200, csv, headers = mapOf("Content-Type" to "text/csv")))
        val log = OperationLog()
        val exported = hey.client { hooks = log }.timeTracks.export()
        assertEquals(csv, exported.decodeToString(), "the CSV comes back verbatim")
        val request = hey.requests.single()
        assertEquals("GET", request.method)
        assertEquals("/calendar/time_tracks/exports", request.path, "HEY streams the export from the bare path, not a .json one")
        assertEquals("text/csv", request.header("Accept"))
        assertEquals(listOf("TimeTracks.ExportTimeTracks:time_track:false:null"), log.started)
    }
}
