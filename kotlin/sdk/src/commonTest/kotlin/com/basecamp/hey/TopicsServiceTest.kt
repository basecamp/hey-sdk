package com.basecamp.hey

import com.basecamp.hey.generated.models.CreateTopicCommentRequestContent
import com.basecamp.hey.generated.models.TopicCommentPayload
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

    @Test
    fun aNoteIsAddedToATopicAndComesBackAsItsEntry() = runTest {
        val hey = mockHey(
            status(
                201,
                """{"id":1019246358,"kind":"comment","topic_id":9,"summary":"Can you take a look at the spine?",""" +
                    """"creator":{"id":197214974,"name":"Jason Fried","email_address":"jason@example.com"},""" +
                    """"content":"<div>Can you take a look at the <strong>spine</strong>?</div>",""" +
                    """"visible_to":[{"id":140958377,"name":"Andrea LaRowe","email_address":"andrea@example.com"}],"collection_only":false}""",
            ),
        )

        val entry = hey.client().topics.createComment(
            9,
            CreateTopicCommentRequestContent(TopicCommentPayload("<div>Can you take a look at the <strong>spine</strong>?</div>")),
        )

        assertEquals(1019246358L, entry.id)
        assertEquals("comment", entry.kind)
        assertEquals(9L, entry.topicId)
        assertEquals("Jason Fried", entry.creator?.name)
        assertEquals("<div>Can you take a look at the <strong>spine</strong>?</div>", entry.content)
        assertEquals(listOf(140958377L), entry.visibleTo?.map { it.id })
        assertEquals(false, entry.collectionOnly)
        assertEquals("POST", hey.requests.single().method)
        assertEquals("/topics/9/comments.json", hey.requests.single().path)
        assertEquals("""{"comment":{"content":"<div>Can you take a look at the <strong>spine</strong>?</div>"}}""", hey.requests.single().body)
    }

    @Test
    fun aBlankNoteIsRefusedAsInvalid() = runTest {
        val hey = mockHey(status(422, """{"errors":["Content can't be blank"]}"""))
        val error = assertFailsWith<HeyException.Validation> {
            hey.client().topics.createComment(9, CreateTopicCommentRequestContent(TopicCommentPayload("")))
        }
        assertEquals(422, error.httpStatus)
        assertEquals("Content can't be blank", error.hint)
    }

    @Test
    fun aNoteOnATopicOutOfReachIsNotFound() = runTest {
        val hey = mockHey(status(404))
        assertFailsWith<HeyException.NotFound> {
            hey.client().topics.createComment(9, CreateTopicCommentRequestContent(TopicCommentPayload("<div>Following up</div>")))
        }
    }

    @Test
    fun theAudienceOfANoteIsReadWithoutPostingOne() = runTest {
        val hey = mockHey(
            ok("""{"visible_to":[{"id":140958377,"name":"Andrea LaRowe"},{"id":197214974,"name":"Jason Fried"}],"collection_only":false}"""),
            ok("""{"visible_to":[],"collection_only":true}"""),
        )
        val client = hey.client()

        val shared = client.topics.getCommentAudience(9)
        val private = client.topics.getCommentAudience(10)

        assertEquals(listOf("Andrea LaRowe", "Jason Fried"), shared.visibleTo?.map { it.name })
        assertEquals(false, shared.collectionOnly)
        assertEquals(emptyList(), private.visibleTo)
        assertEquals(true, private.collectionOnly)
        assertEquals(listOf("GET", "GET"), hey.requests.map { it.method })
        assertEquals(listOf("/topics/9/comments/new.json", "/topics/10/comments/new.json"), hey.requests.map { it.path })
    }
}
