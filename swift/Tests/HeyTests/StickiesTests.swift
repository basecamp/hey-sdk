import Foundation
import XCTest

@testable import Hey

final class StickiesTests: XCTestCase {
    func testALimitIsClampedToTheServersAndZeroIsLeftOff() async throws {
        let hey = mockHey(ok(#"[{"id":1,"body":"Milk"}]"#), ok("[]"), ok("[]"))
        let client = try hey.client()
        let first = try await client.stickies.listUpTo(limit: 10)
        XCTAssertEqual(first.map(\.id), [1])
        _ = try await client.stickies.listUpTo(limit: 500)
        _ = try await client.stickies.listUpTo(limit: 0)
        XCTAssertEqual(hey.requests[0].path, "/stickies.json")
        XCTAssertEqual(hey.requests[0].query("limit"), "10")
        XCTAssertEqual(hey.requests[1].query("limit"), "100", "the server clamps anything above 100, so the client does too")
        XCTAssertNil(hey.requests[2].query("limit"), "limit=0 would be clamped to one sticky rather than read as no limit")
        await assertThrows(HeyError.codeUsage, try await client.stickies.listUpTo(limit: -1))
    }

    func testAStickyIsWrittenUnderItsKeyWithOnlyWhatIsSaid() async throws {
        let hey = mockHey(ok(#"{"id":1,"body":"Milk","size":"large"}"#), ok(#"{"id":1,"body":"Milk","size":"small"}"#))
        let client = try hey.client()
        let created = try await client.stickies.createSticky(body: "Milk", size: .large)
        XCTAssertEqual(created.size, "large")
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/stickies.json")
        let sticky = try XCTUnwrap(try jsonObject(hey.requests[0].body)["sticky"] as? [String: Any])
        XCTAssertEqual(sticky["body"] as? String, "Milk")
        XCTAssertEqual(sticky["size"] as? String, "large")

        _ = try await client.stickies.updateSticky(stickyId: 1, body: "", size: .small)
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, "/stickies/1.json")
        let revised = try XCTUnwrap(try jsonObject(hey.requests[1].body)["sticky"] as? [String: Any])
        XCTAssertNil(revised["body"], "an empty body is left alone")
        XCTAssertEqual(revised["size"] as? String, "small")
    }

    func testAMoveCarriesTheIdAndPositionAtTheTopLevelWithinTheBoardsRange() async throws {
        let hey = mockHey(ok(""))
        let client = try hey.client()
        try await client.stickies.moveTo(stickyId: 1, position: 4)
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].path, "/stickies/moves.json")
        XCTAssertEqual(hey.requests[0].body, #"{"id":1,"position":4}"#)
        await assertThrows(HeyError.codeUsage, try await client.stickies.moveTo(stickyId: 1, position: -1))
        await assertThrows(HeyError.codeUsage, try await client.stickies.moveTo(stickyId: 1, position: StickiesService.maxStickyPosition + 1))
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testASizeIsReadAsHeyWritesIt() {
        XCTAssertEqual(try StickySize.parse("medium"), .medium)
        assertThrowsSync(HeyError.codeUsage) { try StickySize.parse("huge") }
    }
}
