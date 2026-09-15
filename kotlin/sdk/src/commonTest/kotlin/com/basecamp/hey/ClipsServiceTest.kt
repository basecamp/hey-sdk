package com.basecamp.hey

import com.basecamp.hey.generated.clips
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class ClipsServiceTest {
    @Test
    fun aClipIsSavedAndThrownAwayAsForms() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/topics/4471829")), status(302, headers = mapOf("Location" to "/clips")))
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.clips.create(4471829, "<div>The part worth keeping.</div>")
        client.clips.delete(7)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/clips", hey.requests[0].path)
        assertEquals("application/x-www-form-urlencoded", hey.requests[0].header("Content-Type"))
        assertEquals(BROWSER_ACCEPT_HEADER, hey.requests[0].header("Accept"))
        assertEquals(listOf("clip[entry_id]" to "4471829", "clip[content]" to "<div>The part worth keeping.</div>"), formPairs(hey.requests[0].body))
        assertEquals("DELETE", hey.requests[1].method)
        assertEquals("/clips/7", hey.requests[1].path)
        assertEquals("", hey.requests[1].body)
        assertEquals(listOf("Clips.CreateClip:clip:true:4471829", "Clips.DeleteClip:clip:true:7"), log.started, "a new clip names the entry it comes from, a deleted one the clip")
    }
}
