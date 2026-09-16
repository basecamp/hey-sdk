import Foundation
import XCTest

@testable import Hey

final class PostingsTests: XCTestCase {
    private let boxIndex = #"[{"id":1,"kind":"imbox","name":"Imbox"},{"id":3,"kind":"feedbox","name":"The Feed"},{"id":5,"kind":"trailbox","name":"Paper Trail"}]"#

    private func ids(_ request: RecordedRequest) throws -> [Int]? {
        try jsonObject(request.body)["posting_ids"] as? [Int]
    }

    func testTheFeedAndThePaperTrailAreResolvedFromOneReadOfTheIndex() async throws {
        let hey = mockHey(ok(boxIndex), ok(""), ok(""))
        let client = try hey.client()
        try await client.postings.moveToFeed(postingIds: [7])
        try await client.postings.moveToPaperTrail(postingIds: [8, 9])
        XCTAssertEqual(hey.requests.count, 3)
        XCTAssertEqual(hey.requests[0].path, "/boxes.json")
        XCTAssertEqual(hey.requests[1].path, "/postings/moves.json")
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["box_id"] as? Int, 3)
        XCTAssertEqual(try jsonObject(hey.requests[2].body)["box_id"] as? Int, 5)
        XCTAssertEqual(try ids(hey.requests[2]), [8, 9])
    }

    func testTrashingLeavesRemoveAccessOffUnlessItIsForEveryone() async throws {
        let hey = mockHey(ok(""), ok(""))
        let client = try hey.client()
        try await client.postings.moveToTrash(postingIds: [7])
        try await client.postings.trashForEveryone(postingIds: [7, 8])
        XCTAssertEqual(hey.requests[0].path, "/postings/trash.json")
        XCTAssertNil(try jsonObject(hey.requests[0].body)["remove_access"], "left out, HEY removes only your own access from a shared topic")
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["remove_access"] as? String, "false")
        XCTAssertEqual(try ids(hey.requests[1]), [7, 8])
    }

    func testTheBodylessEndpointsCarryTheSelectionCommaJoinedInTheQuery() async throws {
        let hey = mockHey(ok(""), ok(""), ok(""))
        let client = try hey.client()
        try await client.postings.unmutePostings(postingIds: [7, 8])
        try await client.postings.removePostingsFromBoxGroup(postingIds: [9])
        try await client.postings.cancelPostingsBubbleUp(postingIds: [10, 11, 12])
        XCTAssertEqual(hey.requests[0].path, "/postings/mutings.json")
        XCTAssertEqual(hey.requests[0].query("posting_ids"), "7,8")
        XCTAssertEqual(hey.requests[0].body, "")
        XCTAssertEqual(hey.requests[1].path, "/postings/box_groups.json")
        XCTAssertEqual(hey.requests[1].method, "DELETE")
        XCTAssertEqual(hey.requests[1].query("posting_ids"), "9")
        XCTAssertEqual(hey.requests[2].path, "/postings/bubble_up.json")
        XCTAssertEqual(hey.requests[2].method, "DELETE")
        XCTAssertEqual(hey.requests[2].query("posting_ids"), "10,11,12")
    }

    func testSpamAndBubbleUpNowSendTheSelectionAsABody() async throws {
        let hey = mockHey(ok(""), ok(""))
        let client = try hey.client()
        try await client.postings.markPostingsSpam(postingIds: [7])
        try await client.postings.bubbleUpPostingsNow(postingIds: [8, 9])
        XCTAssertEqual(hey.requests[0].path, "/postings/spam.json")
        XCTAssertEqual(try ids(hey.requests[0]), [7])
        XCTAssertEqual(hey.requests[1].path, "/postings/bulk_bubble_up_now.json")
        XCTAssertEqual(try ids(hey.requests[1]), [8, 9])
    }

    func testABoxGroupTakesTheBoxAndTheGroup() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().postings.addPostingsToBoxGroup(boxId: 2, boxGroupId: 44, postingIds: [7, 8])
        XCTAssertEqual(hey.requests.count, 1)
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.path, "/postings/box_groups.json")
        XCTAssertEqual(request.method, "POST")
        let sent = try jsonObject(request.body)
        XCTAssertEqual(sent["box_id"] as? Int, 2)
        XCTAssertEqual(sent["box_group_id"] as? Int, 44)
        XCTAssertEqual(try ids(request), [7, 8])
    }

    func testFilingNamesTheFolderAndUnfilingLeavesAZeroFolderOff() async throws {
        let hey = mockHey(ok(""), ok(""), ok(""), ok(""))
        let client = try hey.client()
        try await client.postings.filePostings(folderId: 31, postingIds: [7])
        try await client.postings.unfilePostings(folderId: 31, postingIds: [7, 8])
        try await client.postings.unfilePostings(folderId: 0, postingIds: [9])
        try await client.postings.createFolderForPostings(name: "Receipts", postingIds: [10])
        XCTAssertEqual(hey.requests[0].path, "/postings/filings.json")
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(try jsonObject(hey.requests[0].body)["folder_id"] as? Int, 31)
        XCTAssertEqual(hey.requests[1].path, "/postings/filings.json")
        XCTAssertEqual(hey.requests[1].method, "DELETE")
        XCTAssertEqual(hey.requests[1].query("posting_ids"), "7,8")
        XCTAssertEqual(hey.requests[1].query("folder_id"), "31")
        XCTAssertNil(hey.requests[2].query("folder_id"), "zero is a folder that does not exist to HEY, so it is not sent")
        XCTAssertEqual(hey.requests[2].query("posting_ids"), "9")
        XCTAssertEqual(hey.requests[3].path, "/postings/folders.json")
        let folder = try XCTUnwrap(try jsonObject(hey.requests[3].body)["folder"] as? [String: Any])
        XCTAssertEqual(folder["name"] as? String, "Receipts")
        XCTAssertNil(folder["status"])
    }

    func testABubbleUpSlotNamesItselfAndOnlyACustomOneCarriesADate() async throws {
        let hey = mockHey(ok(""), ok(""))
        let client = try hey.client()
        try await client.postings.schedulePostingsBubbleUp(slot: .nextWeek, postingIds: [7])
        try await client.postings.schedulePostingsBubbleUp(slot: try .custom(date: "2026-10-01"), postingIds: [7, 8])
        XCTAssertEqual(hey.requests[0].path, "/postings/bubble_up.json")
        XCTAssertEqual(try jsonObject(hey.requests[0].body)["slot"] as? String, "next_week")
        XCTAssertNil(try jsonObject(hey.requests[0].body)["date"])
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["slot"] as? String, "custom")
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["date"] as? String, "2026-10-01")
        XCTAssertEqual(BubbleUpSlot.laterToday.wire, "today")
        XCTAssertEqual(BubbleUpSlot.tomorrow.wire, "tomorrow")
        XCTAssertEqual(BubbleUpSlot.thisWeekend.wire, "weekend")
        assertThrowsSync(HeyError.codeUsage) { try BubbleUpSlot.custom(date: "2026-02-30") }
        assertThrowsSync(HeyError.codeUsage) { try BubbleUpSlot.custom(date: "next tuesday") }
        XCTAssertNoThrow(try BubbleUpSlot.custom(date: "2028-02-29"), "February 29 in a leap year is a day the calendar has")
    }

    func testAnEmptySelectionIsRefusedBeforeAnythingIsSentAndASingleOneIsNamedToTheHooks() async throws {
        let hey = mockHey(ok(""), ok(""))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        await assertThrows(HeyError.codeUsage, try await client.postings.unmutePostings(postingIds: []))
        await assertThrows(HeyError.codeUsage, try await client.postings.markPostingsSpam(postingIds: []))
        await assertThrows(HeyError.codeUsage, try await client.postings.unfilePostings(folderId: 1, postingIds: []))
        await assertThrows(HeyError.codeUsage, try await client.postings.schedulePostingsBubbleUp(slot: .tomorrow, postingIds: []))
        await assertThrows(HeyError.codeUsage, try await client.postings.trashForEveryone(postingIds: []))
        XCTAssertTrue(hey.requests.isEmpty)
        try await client.postings.unmutePostings(postingIds: [7])
        try await client.postings.markPostingsSpam(postingIds: [7, 8])
        XCTAssertEqual(log.started.map { $0.split(separator: ":").last.map(String.init) }, ["7", "nil"])
    }

    func testASelectionOfOnePostingNamesItToTheHooks() async throws {
        let hey = mockHey(ok(""), ok(""), ok(""), ok(""), ok(""), ok(""))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        try await client.postings.markPostingsSeen(postingIds: [7])
        try await client.postings.markPostingsUnseen(postingIds: [7, 8])
        try await client.postings.trashPostings(postingIds: [9])
        try await client.postings.mutePostings(postingIds: [10])
        try await client.postings.moveToBox(boxId: 3, postingIds: [11])
        try await client.postings.moveToBox(boxId: 3, postingIds: [11, 12])
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("start:") }, [
            "start:Postings.MarkPostingsSeen:posting:true:7",
            "start:Postings.MarkPostingsUnseen:posting:true:nil",
            "start:Postings.TrashPostings:posting:true:9",
            "start:Postings.MutePostings:posting:true:10",
            "start:Postings.MovePostings:posting:true:11",
            "start:Postings.MovePostings:posting:true:nil",
        ])
    }

    // MARK: - The change feed

    func testAChangeFeedAnswersItsPagesThenTheCursorToPollNext() async throws {
        let hey = mockHey(
            ok(#"{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&v=2&per_page=50&page=2>; rel="next""#)]),
            ok(#"{"added":[],"updated":[],"deleted":[{"id":2}]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:05:00Z&v=2>; rel="next""#)]),
            status(409, #"{"error":"cursor too old"}"#))
        let store = InMemoryCache()
        let client = try hey.client(cache: store, configure: { $0.enableCache = true })
        await assertThrows(HeyError.codeUsage, try await client.postings.changes(boxId: 7, cursor: PostingChangesCursor(since: "")))
        let first = try await client.postings.changes(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z"))
        XCTAssertEqual(first.added.map(\.id), [1])
        XCTAssertEqual(
            first.nextPage, PostingChangesCursor(since: "2026-09-15T10:01:00Z", version: "2", page: "2", perPage: "50"),
            "the increment has another page, and the link to it is kept whole")
        XCTAssertNil(first.nextCursor)
        let second = try await client.postings.changes(boxId: 7, cursor: try XCTUnwrap(first.nextPage))
        XCTAssertEqual(second.deleted.map(\.id), [2])
        XCTAssertNil(second.nextPage)
        XCTAssertEqual(second.nextCursor, PostingChangesCursor(since: "2026-09-15T10:05:00Z", version: "2"), "and the last page names where to resume")
        XCTAssertEqual(hey.requests[1].query("page"), "2")
        XCTAssertEqual(hey.requests[1].query("since"), "2026-09-15T10:01:00Z", "the next read sends the cursor as HEY issued it")
        XCTAssertEqual(hey.requests[1].query("v"), "2")
        XCTAssertEqual(hey.requests[1].query("per_page"), "50")
        XCTAssertEqual(store.count, 0, "no page of the feed is held")

        let stale = try await client.postings.changes(boxId: 7, cursor: try XCTUnwrap(second.nextCursor))
        XCTAssertTrue(stale.fullSyncRequired, "a 409 is HEY refusing the cursor: read the box in full")
        XCTAssertEqual(stale.added, [])
        XCTAssertEqual(hey.requests.count, 3, "and it is not resent")
    }

    func testACursorOffTheOriginIsRefused() async throws {
        let hey = mockHey(ok(#"{"added":[],"updated":[],"deleted":[]}"#, [("Link", #"<https://evil.example.com/changes.json?since=x>; rel="next""#)]))
        let error = await assertThrows(
            HeyError.codeUsage, try await hey.client().postings.changes(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z")))
        XCTAssertEqual(error?.message.contains("since=x"), false, "the refusal names the origin, not the URL")
    }

    func testANextPageOffTheOriginIsRefusedForTheChangeFeedToo() async throws {
        let hey = mockHey(ok(#"{"added":[],"updated":[],"deleted":[]}"#, [("Link", #"<https://evil.example.com/changes.json?since=x&page=2>; rel="next""#)]))
        let error = await assertThrows(
            HeyError.codeUsage, try await hey.client().postings.changes(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z")))
        XCTAssertEqual(error?.message.contains("since=x"), false)
    }

    func testACursorIsReadOutOfAChangesURL() throws {
        XCTAssertEqual(
            try PostingChangesCursor.fromURL("https://app.hey.com/boxes/7/postings/changes.json?since=2026-09-15T10:00:00Z&v=2&page=3&per_page=50"),
            PostingChangesCursor(since: "2026-09-15T10:00:00Z", version: "2", page: "3", perPage: "50"))
        assertThrowsSync(HeyError.codeUsage) { try PostingChangesCursor.fromURL("/boxes/7/postings/changes.json?since=x") }
    }

    func testAFullSyncAnswerEndsItsOperationWell() async throws {
        // The 409 is what the caller is handed as an answer, so the hooks hear the request refused
        // and the operation succeed: what a trace or a failure count sees agrees with what the
        // caller got.
        let postings = mockHey(status(409, #"{"error":"cursor too old"}"#))
        let transcript = Transcript()
        let stale = try await postings.client(hooks: transcript).postings.changes(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z"))
        XCTAssertTrue(stale.fullSyncRequired)
        XCTAssertEqual(transcript.log, [
            "start:Postings.GetBoxPostingChanges:posting:false:7",
            "request:GET:1",
            "response:409:conflict",
            "end:GetBoxPostingChanges:nil",
        ])

        // Any other refusal still ends the operation with it.
        let gone = mockHey(status(404, #"{"error":"gone"}"#))
        transcript.clear()
        await assertThrows(
            HeyError.codeNotFound,
            try await gone.client(hooks: transcript).postings.changes(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z")))
        XCTAssertEqual(transcript.log.last, "end:GetBoxPostingChanges:not_found")
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("end:") }.count, 1)
    }

    func testAllChangesFollowsThePagesOfAnIncrementAndKeepsTheLastCursor() async throws {
        let hey = mockHey(
            ok(#"{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&v=2&page=2>; rel="next""#)]),
            ok(#"{"added":[],"updated":[{"id":3,"kind":"topic"}],"deleted":[{"id":2}]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:05:00Z&v=2>; rel="next""#)]))
        let store = InMemoryCache()
        let all = try await hey.client(cache: store, configure: { $0.enableCache = true })
            .postings.allChanges(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z", version: "2"))
        XCTAssertEqual(all.added.map(\.id), [1])
        XCTAssertEqual(all.updated.map(\.id), [3])
        XCTAssertEqual(all.deleted.map(\.id), [2])
        XCTAssertNil(all.nextPage, "the increment was read to its end")
        XCTAssertEqual(all.nextCursor, PostingChangesCursor(since: "2026-09-15T10:05:00Z", version: "2"))
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[1].query("page"), "2")
        XCTAssertEqual(hey.requests[1].query("since"), "2026-09-15T10:01:00Z", "the next page is read as HEY issued it")
        XCTAssertEqual(store.count, 0, "and no page of the feed is held, as with one page")
    }

    func testAllChangesComesBackAsSoonAsAFullSyncIsAskedFor() async throws {
        let hey = mockHey(
            ok(#"{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&page=2>; rel="next""#)]),
            status(409, #"{"error":"cursor too old"}"#))
        let stale = try await hey.client().postings.allChanges(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z"))
        XCTAssertTrue(stale.fullSyncRequired)
        XCTAssertEqual(stale.added, [], "what was read before is dropped: the box has to be read in full anyway")
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testAllChangesStopsAtTheClientPageLimitAndSaysWhereItStopped() async throws {
        let hey = mockHey(
            ok(#"{"added":[{"id":1,"kind":"topic"}],"updated":[],"deleted":[]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&page=2>; rel="next""#)]),
            ok(#"{"added":[{"id":2,"kind":"topic"}],"updated":[],"deleted":[]}"#, [("Link", #"</boxes/7/postings/changes.json?since=2026-09-15T10:01:00Z&page=3>; rel="next""#)]),
            ok(#"{"added":[{"id":3,"kind":"topic"}],"updated":[],"deleted":[]}"#))
        let capped = try await hey.client(configure: { $0.maxPages = 2 })
            .postings.allChanges(boxId: 7, cursor: PostingChangesCursor(since: "2026-09-15T10:00:00Z"))
        XCTAssertEqual(capped.added.map(\.id), [1, 2])
        XCTAssertEqual(
            capped.nextPage, PostingChangesCursor(since: "2026-09-15T10:01:00Z", page: "3"),
            "the page not read is named, so the answer does not look complete")
        XCTAssertEqual(hey.requests.count, 2)
    }
}
