import Foundation
import XCTest

@testable import Hey

final class BulkRepliesServiceTests: XCTestCase {
    func testADraftResolvesThePostingsIntoTheEntriesTheReplyGoesTo() async throws {
        let hey = mockHey(ok(#"{"content":"<div>Jane Doe</div>","entries":[{"id":1,"topic_id":2,"topic_name":"Hello","addressed":{}}]}"#))
        let draft = try await hey.client().bulkReplies.draft(postingIds: [7, 8, 9])
        XCTAssertEqual(draft.content, "<div>Jane Doe</div>")
        XCTAssertEqual(draft.entries.map(\.id), [1])
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.path, "/bulk_replies/new.json")
        XCTAssertEqual(request.query("posting_ids"), "7,8,9", "the ids go out comma-joined, as the generated method takes them")
    }

    func testSendingABulkReplyAnswersWhatWasQueued() async throws {
        let hey = mockHey(ok(#"{"id":9,"entries_count":2,"delayed":true,"undo_send_url":"https://app.hey.com/bulk_replies/9/undo_send"}"#))
        let delivery = try await hey.client().bulkReplies.send(entryIds: [1, 2], content: "<div>Thanks, all.</div>")
        XCTAssertEqual(delivery.id, 9)
        XCTAssertEqual(delivery.entriesCount, 2)
        XCTAssertTrue(delivery.delayed)
        XCTAssertEqual(try undoSendId(delivery.undoSendUrl ?? ""), 9)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/bulk_replies.json")
        let body = try jsonObject(request.body)
        XCTAssertEqual((body["entry_ids"] as? [Any])?.compactMap { ($0 as? NSNumber)?.intValue }, [1, 2])
        XCTAssertEqual((body["message"] as? [String: Any])?["content"] as? String, "<div>Thanks, all.</div>")
    }

    func testABulkReplyWithNothingSelectedIsRefusedBeforeAnythingIsSent() async throws {
        let hey = mockHey()
        let client = try hey.client()
        let noPostings = await assertThrows(HeyError.codeUsage, try await client.bulkReplies.draft(postingIds: []))
        XCTAssertEqual(noPostings?.message, "at least one posting is required")
        let noEntries = await assertThrows(HeyError.codeUsage, try await client.bulkReplies.send(entryIds: [], content: "<div>Thanks</div>"))
        XCTAssertEqual(noEntries?.message, "at least one entry is required")
        XCTAssertTrue(hey.requests.isEmpty)
    }

    func testCallingABulkReplyBackPostsAnEmptyFormToItsUndoPath() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/imbox")]))
        let log = OperationLog()
        try await hey.client(hooks: log).bulkReplies.undo(bulkReplyId: 9)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/bulk_replies/9/undo_send")
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(request.body, "")
        XCTAssertEqual(log.started, ["BulkReplies.UndoBulkReplySend:bulk_reply:true:9"])
    }

    func testCallingBackAReplyThatHasGoneOutSurfacesHeysRefusal() async throws {
        let hey = mockHey(status(422, #"{"error":"already sent"}"#))
        let refused = await assertThrowsHeyError(try await hey.client().bulkReplies.undo(bulkReplyId: 9))
        XCTAssertEqual(refused?.httpStatus, 422)
    }

    func testTheUndoIdIsReadOnlyOutOfAnUndoUrl() throws {
        XCTAssertEqual(try undoSendId("/bulk_replies/9/undo_send"), 9)
        XCTAssertEqual(try undoSendId("https://app.hey.com/bulk_replies/9/undo_send?from=imbox"), 9)
        XCTAssertEqual(try undoSendId("/bulk_replies/9/undo_send#top"), 9)
        for other in ["/bulk_replies/9", "/bulk_replies/nine/undo_send", "/topics/9/undo_send", "", "https://app.hey.com/"] {
            let refused = assertThrowsSync(HeyError.codeUsage) { try undoSendId(other) }
            XCTAssertEqual(refused?.message, "not a bulk reply undo URL: \(other)")
        }
    }
}
