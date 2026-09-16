import Foundation
import XCTest

@testable import Hey

final class EntriesTests: XCTestCase {
    func testAReplyCarriesThePrefillsSenderUntouched() async throws {
        let hey = mockHey(ok(""), status(204, nil, [("Location", "/messages/778")]))
        let client = try hey.client()
        var reply = ReplyContent(actingSenderId: 314, subject: "Re: Hello", content: "Reply text", to: ["someone@example.com"])
        try await client.entries.reply(entryId: 456, reply: reply)
        let sent = try jsonObject(hey.requests[0].body)
        XCTAssertEqual(hey.requests[0].path, "/entries/456/replies.json")
        XCTAssertEqual(sent["acting_sender_id"] as? Int, 314)
        XCTAssertEqual((sent["message"] as? [String: Any])?["subject"] as? String, "Re: Hello")

        var draft = reply
        draft.to = []
        draft.subject = ""
        let draftId = try await client.entries.replyDraft(entryId: 456, reply: draft)
        XCTAssertEqual(draftId, 778)
        let drafted = try jsonObject(hey.requests[1].body)
        XCTAssertEqual((drafted["entry"] as? [String: Any])?["status"] as? String, "drafted")
        XCTAssertNil((drafted["message"] as? [String: Any])?["subject"])
        reply.to = []
        await assertThrows(HeyError.codeUsage, try await client.entries.reply(entryId: 456, reply: reply))
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testAReplyWithNoSenderGoesOutAsTheDefaultOne() async throws {
        let hey = mockHey(ok(identityJSON), ok(""))
        try await hey.client().entries.reply(entryId: 456, reply: ReplyContent(content: "Reply text", cc: ["a@example.com"]))
        XCTAssertEqual(hey.requests[0].path, "/identity.json")
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["acting_sender_id"] as? Int, 100)
    }
}
