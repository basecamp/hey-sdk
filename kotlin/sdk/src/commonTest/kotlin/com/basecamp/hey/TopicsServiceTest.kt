package com.basecamp.hey

import com.basecamp.hey.generated.topics
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class TopicsServiceTest {
    @Test
    fun aTopicIsMovedToABoxByItsId() = runTest {
        val hey = mockHey(ok(""))
        hey.client().topics.moveToBox(9, 3)
        assertEquals("/topics/9/moves.json", hey.requests.single().path)
        assertEquals("""{"box_id":3}""", hey.requests.single().body)
    }

    @Test
    fun confirmDestroyIsSentOnlyWhenItIsAskedFor() = runTest {
        val hey = mockHey(status(204), status(204))
        val client = hey.client()
        client.topics.trashTopic(9, confirmDestroy = true)
        client.topics.trashTopic(9)
        assertEquals("PUT", hey.requests[0].method)
        assertEquals("/topics/9/status/trashed.json", hey.requests[0].path)
        assertEquals("1", hey.requests[0].query("confirm_destroy"))
        assertNull(hey.requests[1].query("confirm_destroy"), "an empty confirm_destroy reads as truthy on the server")
    }

    @Test
    fun aSharedTopicComesBackAskingToBeConfirmed() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/topics/9/removal/new")))
        val ended = mutableListOf<String?>()
        val client = hey.client {
            hooks = object : HeyHooks {
                override fun onOperationEnd(info: OperationInfo, result: OperationResult) {
                    ended += (result.error as? HeyException)?.code
                }
            }
        }
        val error = assertFailsWith<HeyException.Usage> { client.topics.trashTopic(9) }
        assertEquals("topic 9 is shared; HEY wants confirmation before trashing it", error.message)
        assertEquals("Call trashTopic with confirmDestroy = true to trash it and remove your access", error.hint)
        assertEquals(1, hey.requests.size, "the confirmation page is read rather than followed")
        assertEquals(listOf<String?>("usage"), ended, "the hooks hear the refusal the caller gets, not the redirect HEY answered")
    }

    @Test
    fun aRedirectBackToTheBoxIsTheTrashingGoingThrough() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "https://app.hey.com/imbox")))
        hey.client().topics.trashTopic(9, confirmDestroy = true)
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun anyOtherRefusalStaysAFailure() = runTest {
        val hey = mockHey(status(404), status(406))
        val client = hey.client()
        assertFailsWith<HeyException.NotFound> { client.topics.trashTopic(9, confirmDestroy = true) }
        val error = assertFailsWith<HeyException.Api> { client.topics.trashTopic(9, confirmDestroy = true) }
        assertEquals(406, error.httpStatus)
    }
}
