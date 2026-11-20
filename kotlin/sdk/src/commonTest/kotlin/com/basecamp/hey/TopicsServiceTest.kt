package com.basecamp.hey

import com.basecamp.hey.generated.models.RenameTopicRequestContent
import com.basecamp.hey.generated.models.TopicNamePayload
import com.basecamp.hey.generated.models.CreateTopicCommentRequestContent
import com.basecamp.hey.generated.models.TopicCommentPayload
import com.basecamp.hey.generated.topics
import com.basecamp.hey.services.undoneEntryId
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class TopicsServiceTest {
    @Test
    fun renameSendsTheNameIncludingAnEmptyOneWithoutReadingTheTopic() = runTest {
        val hey = mockHey(status(204), status(204))
        val client = hey.client()
        client.topics.rename(9, RenameTopicRequestContent(TopicNamePayload("Kitchen renovation")))
        client.topics.rename(9, RenameTopicRequestContent(TopicNamePayload("")))
        assertEquals(2, hey.requests.size)
        assertEquals("PATCH", hey.requests[0].method)
        assertEquals("/topics/9.json", hey.requests[0].path)
        assertEquals("""{"topic":{"name":"Kitchen renovation"}}""", hey.requests[0].body)
        assertEquals("""{"topic":{"name":""}}""", hey.requests[1].body)
    }

    @Test
    fun renameReportsAnInaccessibleTopic() = runTest {
        val hey = mockHey(status(404))
        assertFailsWith<HeyException.NotFound> {
            hey.client().topics.rename(9, RenameTopicRequestContent(TopicNamePayload("Kitchen renovation")))
        }
    }

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
    fun anUndoAnswersTheEntryHeyCalledBackWithoutFollowingTheRedirect() = runTest {
        for (location in listOf("/messages/77/edit", "/topics/9?expanded_draft=77", "https://app.hey.com/messages/77/edit?native_undo=true")) {
            val hey = mockHey(status(303, headers = mapOf("Location" to location)), ok())
            val log = OperationLog()
            val entryId = hey.client { hooks = log; maxRetries = 3 }.topics.undoTopicSend(9)
            assertEquals(77L, entryId, location)
            assertEquals(1, hey.requests.size, "the redirect to $location is the answer, not followed")
            val request = hey.requests.single()
            assertEquals("POST", request.method)
            assertEquals("/topics/9/undo_send.json", request.path)
            assertEquals("application/json", request.header("Accept"))
            assertEquals("Bearer test-token", request.header("Authorization"))
            assertEquals("", request.body)
            assertEquals(listOf("Topics.UndoTopicSend:topic:true:9"), log.started)
            assertEquals(listOf("Topics.UndoTopicSend:null"), log.ended)
        }
    }

    @Test
    fun anUndoWhoseRedirectNamesNoEntryAnswersNone() = runTest {
        val hey = mockHey(status(303, headers = mapOf("Location" to "/somewhere/else")), ok())
        assertNull(hey.client().topics.undoTopicSend(9), "HEY acted; it only can't be said which entry it was")
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun anUndoWithNothingLeftToCallBackIsRefusedWithHeys400() = runTest {
        val hey = mockHey(status(400, headers = mapOf("X-Request-Id" to "req-undo-400")), status(303, headers = mapOf("Location" to "/messages/77/edit")))
        val log = OperationLog()
        val refused = assertFailsWith<HeyException.Api> { hey.client { hooks = log; maxRetries = 3 }.topics.undoTopicSend(9) }
        assertEquals(400, refused.httpStatus)
        assertEquals("nothing to undo: the message was already sent, or none was held", refused.message)
        assertEquals("req-undo-400", refused.requestId)
        assertEquals(false, refused.retryable)
        assertEquals(1, hey.requests.size, "too late is not asked twice")
        assertEquals(listOf("Topics.UndoTopicSend:api_error"), log.ended, "the hooks hear the refusal the caller gets")
    }

    @Test
    fun anUndoIsNotSentAgainAfterAFailure() = runTest {
        val hey = mockHey(status(503), status(303, headers = mapOf("Location" to "/messages/77/edit")))
        val refused = assertFailsWith<HeyException> { hey.client { maxRetries = 3 }.topics.undoTopicSend(9) }
        assertEquals(503, refused.httpStatus)
        assertEquals(1, hey.requests.size, "a second undo would call back the next delayed entry")
    }

    @Test
    fun anUndoOnATopicOutOfReachStaysNotFound() = runTest {
        val hey = mockHey(status(404))
        assertFailsWith<HeyException.NotFound> { hey.client().topics.undoTopicSend(9) }
    }

    @Test
    fun theEntryCalledBackIsReadOnlyOutOfAnUndoRedirect() {
        for (location in listOf(
            "/messages/77/edit",
            "https://app.hey.com/messages/77/edit",
            "https://app.hey.com/messages/77/edit?native_undo=true&notice=x",
            "/topics/9?expanded_draft=77",
            "https://app.hey.com/topics/9?expanded_draft=77#__entry_77",
        )) {
            assertEquals(77L, undoneEntryId(location), location)
        }
        for (location in listOf(
            "",
            "/topics/9", // where HEY's pages send a browser with nothing to undo
            "/topics/9?expanded_draft=",
            "/topics/9?expanded_draft=abc",
            "/topics/9?expanded_draft=-4",
            "/messages/77", // the message, not its draft
            "/messages/abc/edit",
            "/messages/0/edit",
            "/entries/77/edit",
            "/topics/9/entries?expanded_draft=77",
            "not a url at all: %",
        )) {
            assertNull(undoneEntryId(location), location)
        }
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
