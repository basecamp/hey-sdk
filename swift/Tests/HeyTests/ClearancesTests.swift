import Foundation
import XCTest

@testable import Hey

final class ClearancesTests: XCTestCase {
    private let queue = #"{"pending_clearances_count":2,"signed_stream_name":"abc","clearances":[{"id":11,"status":"pending","petitioner":{"id":5,"name":"Ann"}},{"id":12,"status":"pending"}]}"#

    func testTheSummaryIsTheCheapReadAndTheCountComesOutOfIt() async throws {
        let hey = mockHey(ok(#"{"pending_clearances_count":3,"signed_stream_name":"abc"}"#), ok("{}"))
        let client = try hey.client()
        let summary = try await client.clearances.summary()
        XCTAssertEqual(summary.pendingClearancesCount, 3)
        XCTAssertEqual(summary.signedStreamName, "abc")
        XCTAssertNil(summary.clearances)
        XCTAssertEqual(hey.requests[0].path, "/clearances.json")
        XCTAssertNil(hey.requests[0].query("include_clearances"), "the summary asks for the count alone")
        let count = try await client.clearances.pendingCount()
        XCTAssertEqual(count, 0, "a count HEY leaves out reads as none waiting")
    }

    func testTheQueueIsAskedForByNameAndWalkedByCursor() async throws {
        let hey = mockHey(ok(queue, [("Link", #"</clearances.json?include_clearances=true&page=abc>; rel="next""#)]), ok(queue))
        let client = try hey.client()
        let first = try await client.clearances.pendingPage()
        XCTAssertEqual(hey.requests[0].query("include_clearances"), "true")
        XCTAssertNil(hey.requests[0].query("page"))
        XCTAssertEqual(first.value.clearances?.map(\.id), [11, 12])
        XCTAssertEqual(first.value.clearances?.first?.petitioner?.name, "Ann")
        XCTAssertEqual(first.nextPage, "abc")
        let next = try await client.clearances.pending(page: first.nextPage)
        XCTAssertEqual(hey.requests[1].query("page"), "abc")
        XCTAssertEqual(next.pendingClearancesCount, 2)
    }

    func testScreeningSendsTheDecisionAndOnlyTheOptionsThatAreOn() async throws {
        let hey = mockHey(ok(#"{"id":11,"status":"approved"}"#), ok(#"{"id":12,"status":"denied"}"#))
        let client = try hey.client()
        let approved = try await client.clearances.screen(
            clearanceId: 11, status: .approved, options: ScreenOptions(designationBoxId: 3, markTopicsAsSeen: true))
        XCTAssertEqual(approved.status, "approved")
        XCTAssertEqual(hey.requests[0].method, "PATCH")
        XCTAssertEqual(hey.requests[0].path, "/clearances/11.json")
        let sent = try jsonObject(hey.requests[0].body)
        XCTAssertEqual(sent["status"] as? String, "approved")
        XCTAssertEqual(sent["designation_box_id"] as? Int, 3)
        XCTAssertEqual(sent["mark_topics_as_seen"] as? Bool, true)
        XCTAssertNil(sent["spam"], "a flag HEY reads for truthiness stays off the wire when it is off")

        _ = try await client.clearances.screen(clearanceId: 12, status: .denied)
        XCTAssertEqual(Set(try jsonObject(hey.requests[1].body).keys), ["status"])
    }

    func testScreeningManyJoinsTheIdsAndAnswersWhatChanged() async throws {
        let hey = mockHey(ok(#"{"clearances":[{"id":11,"status":"denied"}]}"#))
        let client = try hey.client()
        let changed = try await client.clearances.screenMany(clearanceIds: [11, 12], status: .denied, spam: true)
        XCTAssertEqual(changed.map(\.id), [11], "a partial match answers only what it touched")
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].path, "/clearances/bulk.json")
        let sent = try jsonObject(hey.requests[0].body)
        XCTAssertEqual(sent["ids"] as? String, "11,12")
        XCTAssertEqual(sent["status"] as? String, "denied")
        XCTAssertEqual(sent["spam"] as? Bool, true)
        await assertThrows(HeyError.codeUsage, try await client.clearances.screenMany(clearanceIds: [], status: .approved))
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testTheDecidedListIsReadAndRescreenedSeparatelyFromTheQueue() async throws {
        let hey = mockHey(
            ok(#"{"clearances":[{"id":21,"status":"approved"}]}"#, [("Link", #"</my/clearances.json?page=xyz>; rel="next""#)]),
            ok(#"{"clearances":[]}"#),
            ok(#"{"id":21,"status":"denied"}"#))
        let client = try hey.client()
        let page = try await client.clearances.screenedPage()
        XCTAssertEqual(hey.requests[0].path, "/my/clearances.json")
        XCTAssertEqual(page.value.clearances?.map(\.id), [21])
        XCTAssertEqual(page.nextPage, "xyz")
        let screened = try await client.clearances.screened(page: "xyz")
        XCTAssertTrue(screened.isEmpty)
        XCTAssertEqual(hey.requests[1].query("page"), "xyz")

        let rescreened = try await client.clearances.rescreen(clearanceId: 21, status: .denied)
        XCTAssertEqual(rescreened.status, "denied")
        XCTAssertEqual(hey.requests[2].path, "/my/clearances/21.json")
        XCTAssertEqual(hey.requests[2].body, #"{"status":"denied"}"#)
    }

    func testAStatusIsReadAsHeyWritesIt() {
        XCTAssertEqual(try ClearanceStatus.parse("approved"), .approved)
        XCTAssertEqual(try ClearanceStatus.parse("denied"), .denied)
        assertThrowsSync(HeyError.codeValidation) { try ClearanceStatus.parse("maybe") }
    }
}
