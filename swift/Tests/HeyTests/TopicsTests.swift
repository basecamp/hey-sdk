import Foundation
import XCTest

@testable import Hey

final class TopicsTests: XCTestCase {
    func testRenameSendsTheNameIncludingAnEmptyOneWithoutReadingTheTopic() async throws {
        let hey = mockHey(status(204), status(204))
        let client = try hey.client()
        try await client.topics.rename(topicId: 9, body: RenameTopicRequestContent(topic: TopicNamePayload(name: "Kitchen renovation")))
        try await client.topics.rename(topicId: 9, body: RenameTopicRequestContent(topic: TopicNamePayload(name: "")))
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[0].method, "PATCH")
        XCTAssertEqual(hey.requests[0].path, "/topics/9.json")
        XCTAssertEqual(hey.requests[0].body, #"{"topic":{"name":"Kitchen renovation"}}"#)
        XCTAssertEqual(hey.requests[1].body, #"{"topic":{"name":""}}"#)
    }

    func testRenameReportsAnInaccessibleTopic() async throws {
        let hey = mockHey(status(404))
        _ = await assertThrows(HeyError.codeNotFound, try await hey.client().topics.rename(
            topicId: 9, body: RenameTopicRequestContent(topic: TopicNamePayload(name: "Kitchen renovation"))))
    }

    func testATopicIsMovedToABoxByItsId() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().topics.moveToBox(topicId: 9, boxId: 3)
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].path, "/topics/9/moves.json")
        XCTAssertEqual(hey.requests[0].body, #"{"box_id":3}"#)
    }

    func testConfirmDestroyIsSentOnlyWhenItIsAskedFor() async throws {
        let hey = mockHey(status(204), status(204))
        let client = try hey.client()
        try await client.topics.trashTopic(topicId: 9, confirmDestroy: true)
        try await client.topics.trashTopic(topicId: 9)
        XCTAssertEqual(hey.requests[0].method, "PUT")
        XCTAssertEqual(hey.requests[0].path, "/topics/9/status/trashed.json")
        XCTAssertEqual(hey.requests[0].query("confirm_destroy"), "1")
        XCTAssertNil(hey.requests[1].query("confirm_destroy"), "an empty confirm_destroy reads as truthy on the server")
    }

    func testASharedTopicComesBackAskingToBeConfirmed() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/topics/9/removal/new")]))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        let error = await assertThrows(HeyError.codeUsage, try await client.topics.trashTopic(topicId: 9))
        XCTAssertEqual(error?.message, "topic 9 is shared; HEY wants confirmation before trashing it")
        XCTAssertEqual(error?.hint, "Call trashTopic with confirmDestroy: true to trash it and remove your access")
        XCTAssertEqual(hey.requests.count, 1, "the confirmation page is read rather than followed")
        XCTAssertEqual(log.ended, ["Topics.TrashTopic:usage"], "the hooks hear the refusal the caller gets, not the redirect HEY answered")
    }

    func testARedirectBackToTheBoxIsTheTrashingGoingThrough() async throws {
        let hey = mockHey(status(302, nil, [("Location", "https://app.hey.com/imbox")]))
        try await hey.client().topics.trashTopic(topicId: 9, confirmDestroy: true)
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testAnyOtherRefusalStaysAFailure() async throws {
        let hey = mockHey(status(404), status(406))
        let client = try hey.client()
        await assertThrows(HeyError.codeNotFound, try await client.topics.trashTopic(topicId: 9, confirmDestroy: true))
        let error = await assertThrows(HeyError.codeAPI, try await client.topics.trashTopic(topicId: 9, confirmDestroy: true))
        XCTAssertEqual(error?.httpStatus, 406)
    }

    func testAnUndoAnswersTheEntryHeyCalledBackWithoutFollowingTheRedirect() async throws {
        for location in ["/messages/77/edit", "/topics/9?expanded_draft=77", "https://app.hey.com/messages/77/edit?native_undo=true"] {
            let hey = mockHey(status(303, nil, [("Location", location)]), ok())
            let log = OperationLog()
            let entryId = try await hey.client(hooks: log) { $0.maxRetries = 3 }.topics.undoTopicSend(topicId: 9)
            XCTAssertEqual(entryId, 77, location)
            XCTAssertEqual(hey.requests.count, 1, "the redirect to \(location) is the answer, not followed")
            let request = hey.requests[0]
            XCTAssertEqual(request.method, "POST")
            XCTAssertEqual(request.path, "/topics/9/undo_send.json")
            XCTAssertEqual(request.header("Accept"), "application/json")
            XCTAssertEqual(request.header("Authorization"), "Bearer test-token")
            XCTAssertEqual(request.body, "")
            XCTAssertEqual(log.started, ["Topics.UndoTopicSend:topic:true:9"])
            XCTAssertEqual(log.ended, ["Topics.UndoTopicSend:nil"])
        }
    }

    func testAnUndoWhoseRedirectNamesNoEntryAnswersNone() async throws {
        let hey = mockHey(status(303, nil, [("Location", "/somewhere/else")]), ok())
        let entryId = try await hey.client().topics.undoTopicSend(topicId: 9)
        XCTAssertNil(entryId, "HEY acted; it only can't be said which entry it was")
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testAnUndoWithNothingLeftToCallBackIsRefusedWithHeys400() async throws {
        let hey = mockHey(status(400, nil, [("X-Request-Id", "req-undo-400")]), status(303, nil, [("Location", "/messages/77/edit")]))
        let log = OperationLog()
        let client = try hey.client(hooks: log) { $0.maxRetries = 3 }
        let refused = await assertThrows(HeyError.codeAPI, try await client.topics.undoTopicSend(topicId: 9))
        XCTAssertEqual(refused?.httpStatus, 400)
        XCTAssertEqual(refused?.message, "nothing to undo: the message was already sent, or none was held")
        XCTAssertEqual(refused?.requestId, "req-undo-400")
        XCTAssertEqual(refused?.isRetryable, false)
        XCTAssertEqual(hey.requests.count, 1, "too late is not asked twice")
        XCTAssertEqual(log.ended, ["Topics.UndoTopicSend:api_error"], "the hooks hear the refusal the caller gets")
    }

    func testAnUndoIsNotSentAgainAfterAFailure() async throws {
        let hey = mockHey(status(503), status(303, nil, [("Location", "/messages/77/edit")]))
        let client = try hey.client { $0.maxRetries = 3 }
        let refused = await assertThrowsHeyError(try await client.topics.undoTopicSend(topicId: 9))
        XCTAssertEqual(refused?.httpStatus, 503)
        XCTAssertEqual(hey.requests.count, 1, "a second undo would call back the next delayed entry")
    }

    func testAnUndoOnATopicOutOfReachStaysNotFound() async throws {
        let hey = mockHey(status(404))
        await assertThrows(HeyError.codeNotFound, try await hey.client().topics.undoTopicSend(topicId: 9))
    }

    func testTheEntryCalledBackIsReadOnlyOutOfAnUndoRedirect() {
        for location in [
            "/messages/77/edit",
            "https://app.hey.com/messages/77/edit",
            "https://app.hey.com/messages/77/edit?native_undo=true&notice=x",
            "/topics/9?expanded_draft=77",
            "https://app.hey.com/topics/9?expanded_draft=77#__entry_77",
        ] {
            XCTAssertEqual(undoneEntryId(location), 77, location)
        }
        for location in [
            "",
            "/topics/9",  // where HEY's pages send a browser with nothing to undo
            "/topics/9?expanded_draft=",
            "/topics/9?expanded_draft=abc",
            "/topics/9?expanded_draft=-4",
            "/messages/77",  // the message, not its draft
            "/messages/abc/edit",
            "/messages/0/edit",
            "/entries/77/edit",
            "/topics/9/entries?expanded_draft=77",
            "not a url at all: %",
        ] {
            XCTAssertNil(undoneEntryId(location), location)
        }
    }

    func testANoteIsAddedToATopicAndComesBackAsItsEntry() async throws {
        let hey = mockHey(status(201, #"{"id":1019246358,"kind":"comment","topic_id":9,"summary":"Can you take a look at the spine?","creator":{"id":197214974,"name":"Jason Fried","email_address":"jason@example.com"},"content":"<div>Can you take a look at the <strong>spine</strong>?</div>","visible_to":[{"id":140958377,"name":"Andrea LaRowe","email_address":"andrea@example.com"}],"collection_only":false}"#))

        let entry = try await hey.client().topics.createComment(
            topicId: 9,
            body: CreateTopicCommentRequestContent(comment: TopicCommentPayload(content: "<div>Can you take a look at the <strong>spine</strong>?</div>")))

        XCTAssertEqual(entry.id, 1019246358)
        XCTAssertEqual(entry.kind, "comment")
        XCTAssertEqual(entry.topicId, 9)
        XCTAssertEqual(entry.creator?.name, "Jason Fried")
        XCTAssertEqual(entry.content, "<div>Can you take a look at the <strong>spine</strong>?</div>")
        XCTAssertEqual(entry.visibleTo?.map(\.id), [140958377])
        XCTAssertEqual(entry.collectionOnly, false)
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/topics/9/comments.json")
        let sent = try JSONSerialization.jsonObject(with: Data(hey.requests[0].body.utf8)) as? [String: Any]
        XCTAssertEqual((sent?["comment"] as? [String: Any])?["content"] as? String, "<div>Can you take a look at the <strong>spine</strong>?</div>")
    }

    func testABlankNoteIsRefusedAsInvalid() async throws {
        let hey = mockHey(status(422, #"{"errors":["Content can't be blank"]}"#))
        let error = await assertThrows(
            HeyError.codeValidation,
            try await hey.client().topics.createComment(topicId: 9, body: CreateTopicCommentRequestContent(comment: TopicCommentPayload(content: ""))))
        XCTAssertEqual(error?.httpStatus, 422)
        XCTAssertEqual(error?.hint, "Content can't be blank")
    }

    func testANoteOnATopicOutOfReachIsNotFound() async throws {
        let hey = mockHey(status(404))
        await assertThrows(
            HeyError.codeNotFound,
            try await hey.client().topics.createComment(topicId: 9, body: CreateTopicCommentRequestContent(comment: TopicCommentPayload(content: "<div>Following up</div>"))))
    }

    func testTheAudienceOfANoteIsReadWithoutPostingOne() async throws {
        let hey = mockHey(
            ok(#"{"visible_to":[{"id":140958377,"name":"Andrea LaRowe"},{"id":197214974,"name":"Jason Fried"}],"collection_only":false}"#),
            ok(#"{"visible_to":[],"collection_only":true}"#))
        let client = try hey.client()

        let shared = try await client.topics.getCommentAudience(topicId: 9)
        let privateNote = try await client.topics.getCommentAudience(topicId: 10)

        XCTAssertEqual(shared.visibleTo?.map(\.name), ["Andrea LaRowe", "Jason Fried"])
        XCTAssertEqual(shared.collectionOnly, false)
        XCTAssertEqual(privateNote.visibleTo?.count, 0)
        XCTAssertEqual(privateNote.collectionOnly, true)
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests.map(\.method), ["GET", "GET"])
        XCTAssertEqual(hey.requests.map(\.path), ["/topics/9/comments/new.json", "/topics/10/comments/new.json"])
    }
}
