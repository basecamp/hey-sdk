import Foundation
import XCTest

@testable import Hey

final class TopicsTests: XCTestCase {
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
}
