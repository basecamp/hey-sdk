import Foundation
import XCTest

@testable import Hey

final class CalendarsTests: XCTestCase {
    func testTheCalendarIndexIsReadWithWhatALiveFollowerNeeds() async throws {
        let hey = mockHey(
            ok(#"{"calendars":[{"calendar":{"id":3,"name":"Work"},"recording_changes_url":"https://app.hey.com/calendars/3/recording/changes?since=2026-09-15T10:00:00.000Z&v=1","signed_stream_name":"stream-3"}],"calendar_changes_url":"https://app.hey.com/calendar/changes?since=2026-09-15T10:00:00.000Z","selected_calendar_ids":[3]}"#),
            ok(#"{"selected_calendar_ids":[]}"#))
        let client = try hey.client()
        let list = try await client.calendars.listWithChanges()
        XCTAssertEqual(hey.requests[0].path, "/calendars.json")
        XCTAssertEqual(list.calendars.count, 1)
        XCTAssertEqual(list.calendars.first?.calendar?.name, "Work")
        XCTAssertEqual(list.calendars.first?.signedStreamName, "stream-3")
        XCTAssertEqual(list.selectedCalendarIds, [3])
        XCTAssertEqual(
            try CalendarChangesCursor.fromUrl(try XCTUnwrap(list.calendarChangesUrl)), CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z"),
            "a calendar changes URL carries no version")
        XCTAssertEqual(
            try CalendarChangesCursor.fromUrl(try XCTUnwrap(list.calendars.first?.recordingChangesUrl)),
            CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z", version: "1"))
        assertThrowsSync(HeyError.codeUsage) { try CalendarChangesCursor.fromUrl("/calendar/changes?since=x") }

        let selection = try await client.calendars.toggleSelection(calendarId: 3)
        XCTAssertEqual(selection, [])
        XCTAssertEqual(hey.requests[1].method, "POST")
        XCTAssertEqual(hey.requests[1].path, "/calendars/3/toggle.json")
    }

    func testTheCalendarChangesFeedAnswersItsPagesThenTheCursorToPollNext() async throws {
        let hey = mockHey(
            ok(
                #"{"added":[{"calendar":{"id":4},"signed_stream_name":"stream-4"}],"updated":[],"deleted":[]}"#,
                [("Link", #"</calendar/changes.json?since=2026-09-15T10:00:00.000Z&page=2&per_page=50>; rel="next""#)]),
            ok(
                #"{"added":[],"updated":[{"id":3,"name":"Renamed"}],"deleted":[{"id":2,"deleted_at":"2026-09-15T10:04:00.000Z"}]}"#,
                [("Link", #"</calendar/changes.json?since=2026-09-15T10:05:00.000Z>; rel="next""#)]),
            ok(#"{"added":[],"updated":[],"deleted":[]}"#))
        let store = InMemoryCache()
        let transcript = AnnouncementLog()
        let client = try hey.client(hooks: transcript, cache: store) { $0.enableCache = true }
        await assertThrows(HeyError.codeUsage, try await client.calendars.calendarChanges(cursor: CalendarChangesCursor()))
        let all = try await client.calendars.allCalendarChanges(cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z"))
        XCTAssertEqual(all.added.map(\.signedStreamName), ["stream-4"])
        XCTAssertEqual(all.updated.map(\.name), ["Renamed"])
        XCTAssertEqual(all.deleted.map(\.id), [2])
        XCTAssertNil(all.nextPage)
        XCTAssertEqual(all.nextCursor, CalendarChangesCursor(since: "2026-09-15T10:05:00.000Z"), "the last page names where to resume")
        XCTAssertEqual(hey.requests[0].path, "/calendar/changes.json")
        XCTAssertEqual(hey.requests[0].query("since"), "2026-09-15T10:00:00.000Z")
        XCTAssertNil(hey.requests[0].query("v"), "the version is never invented")
        XCTAssertEqual(hey.requests[1].query("page"), "2")
        XCTAssertEqual(hey.requests[1].query("per_page"), "50", "the next read sends the cursor as HEY issued it")
        XCTAssertEqual(store.count, 0, "no page of the feed is held")
        XCTAssertEqual(transcript.operations.map(\.operation), ["GetCalendarChanges", "GetCalendarChanges"])
        XCTAssertEqual(transcript.operations.first?.resourceType, "calendar")
        XCTAssertEqual(transcript.operations.first?.isMutation, false)

        let quiet = try await client.calendars.calendarChanges(cursor: try XCTUnwrap(all.nextCursor))
        XCTAssertNil(quiet.nextCursor, "nothing changed, so the cursor that produced this page still stands")
        XCTAssertNil(quiet.nextPage)
    }

    func testTheRecordingChangesFeedKeepsTheWiresGroupingAndFoldsTheDeletions() async throws {
        let hey = mockHey(
            ok(
                #"{"added":{"Calendar::Event":[{"id":10,"type":"Calendar::Event"}]},"updated":{},"deleted":{"Calendar::Event":[{"id":11,"deleted_at":"2026-09-15T10:01:00.000Z","type":"Calendar::Event"}],"Calendar::Todo":[{"id":11,"deleted_at":"2026-09-15T10:01:00.000Z","type":"Calendar::Event"},{"id":12,"deleted_at":"2026-09-15T10:02:00.000Z","type":"Calendar::Todo"}]}}"#,
                [("Link", #"</calendars/3/recording/changes.json?since=2026-09-15T10:00:00.000Z&v=1&page=2>; rel="next""#)]),
            ok(
                #"{"added":{"Calendar::Event":[{"id":13,"type":"Calendar::Event"}],"Calendar::Habit":[{"id":14,"type":"Calendar::Habit"}]}}"#,
                [("Link", #"</calendars/3/recording/changes.json?since=2026-09-15T10:05:00.000Z&v=1>; rel="next""#)]),
            status(409, #"{"error":"too far behind"}"#))
        let transcript = AnnouncementLog()
        let client = try hey.client(hooks: transcript)
        await assertThrows(HeyError.codeUsage, try await client.calendars.recordingChanges(calendarId: 3, cursor: CalendarChangesCursor(version: "1")))
        await assertThrows(
            HeyError.codeUsage, try await client.calendars.recordingChanges(calendarId: 3, cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z")))
        XCTAssertTrue(hey.requests.isEmpty)

        let all = try await client.calendars.allRecordingChanges(
            calendarId: 3, cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z", version: "1"))
        XCTAssertEqual(all.added["Calendar::Event"]?.map(\.id), [10, 13], "pages merge within a type key")
        XCTAssertEqual(all.added["Calendar::Habit"]?.map(\.id), [14])
        XCTAssertEqual(all.deleted.map(\.id), [11, 12], "a deletion repeated under every key arrives once")
        XCTAssertEqual(all.deleted[1].type, "Calendar::Todo")
        XCTAssertEqual(all.nextCursor, CalendarChangesCursor(since: "2026-09-15T10:05:00.000Z", version: "1"))
        XCTAssertEqual(hey.requests[0].path, "/calendars/3/recording/changes.json")
        XCTAssertEqual(hey.requests[0].query("v"), "1")
        XCTAssertEqual(hey.requests[1].query("page"), "2")
        XCTAssertEqual(transcript.operations.first?.operation, "GetCalendarRecordingChanges")
        XCTAssertEqual(transcript.operations.first?.resourceType, "recording")
        XCTAssertEqual(transcript.operations.first?.resourceId, 3)

        let stale = try await client.calendars.allRecordingChanges(calendarId: 3, cursor: try XCTUnwrap(all.nextCursor))
        XCTAssertTrue(stale.fullSyncRequired, "a 409 is the feed refusing the cursor: read the calendar in full")
        XCTAssertEqual(stale.added, [:])
        XCTAssertEqual(hey.requests.count, 3, "and it is not resent")
    }

    func testDeletionsFoldTheSameWayWhateverOrderTheBucketsArriveIn() {
        let event = DeletedRecording(id: 11, deletedAt: "2026-09-15T10:01:00.000Z", type: "Calendar::Event")
        let todo = DeletedRecording(id: 12, deletedAt: "2026-09-15T10:02:00.000Z", type: "Calendar::Todo")
        XCTAssertEqual(flattenDeletedRecordings(["Calendar::Todo": [event, todo], "Calendar::Event": [event]]), [event, todo])
        XCTAssertEqual(flattenDeletedRecordings(["Calendar::Event": [todo, event], "Calendar::Todo": [event, todo]]), [todo, event])
        XCTAssertEqual(flattenDeletedRecordings([:]), [])
    }

    func testAChangesLinkOffTheOriginIsRefused() async throws {
        let hey = mockHey(
            ok(#"{"added":[],"updated":[],"deleted":[]}"#, [("Link", #"<https://evil.example.com/calendar/changes.json?since=x&page=2>; rel="next""#)]))
        let transcript = Transcript()
        let error = await assertThrows(
            HeyError.codeUsage,
            try await hey.client(hooks: transcript).calendars.calendarChanges(cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z")))
        XCTAssertFalse(error?.message.contains("since=x") ?? true, "the refusal names the origin, not the URL")
        XCTAssertEqual(transcript.log.last, "end:GetCalendarChanges:usage", "and the operation the hooks hear ends with it")
    }

    func testAChangesWalkStopsAtThePageLimitAndSaysWhereItStopped() async throws {
        let page = ok(
            #"{"added":[{"recording_changes_url":"/calendars/1/recording/changes.json"}],"updated":[],"deleted":[]}"#,
            [("Link", #"</calendar/changes.json?since=2026-09-15T10:00:00.000Z&page=2>; rel="next""#)])
        let hey = mockHey(page, page, page)
        let capped = try await hey.client { $0.maxPages = 2 }.calendars.allCalendarChanges(cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z"))
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(capped.added.count, 2, "what was read is answered")
        XCTAssertEqual(
            capped.nextPage, CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z", page: "2"),
            "and the page not read is named, so the answer does not look complete")
        XCTAssertNil(capped.nextCursor)

        let recordingPage = ok(
            #"{"added":{},"updated":{},"deleted":{}}"#,
            [("Link", #"</calendars/3/recording/changes.json?since=2026-09-15T10:00:00.000Z&v=1&page=2>; rel="next""#)])
        let recordings = mockHey(recordingPage, recordingPage, recordingPage)
        let cappedRecordings = try await recordings.client { $0.maxPages = 2 }.calendars.allRecordingChanges(
            calendarId: 3, cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z", version: "1"))
        XCTAssertEqual(recordings.requests.count, 2)
        XCTAssertEqual(cappedRecordings.nextPage, CalendarChangesCursor(since: "2026-09-15T10:00:00.000Z", version: "1", page: "2"))
        XCTAssertFalse(cappedRecordings.fullSyncRequired)
    }

    func testAFullSyncAnswerEndsItsOperationWell() async throws {
        // The 409 is what the caller is handed as an answer, so the hooks hear the request refused
        // and the operation succeed: what a trace or a failure count sees agrees with what the
        // caller got.
        let hey = mockHey(status(409), status(404, #"{"error":"gone"}"#))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        let behind = try await client.calendars.recordingChanges(calendarId: 3, cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00Z", version: "1"))
        XCTAssertTrue(behind.fullSyncRequired)
        XCTAssertEqual(transcript.log, [
            "start:Calendars.GetCalendarRecordingChanges:recording:false:3",
            "request:GET:1",
            "response:409:conflict",
            "end:GetCalendarRecordingChanges:nil",
        ])

        // Any other refusal still ends the operation with it.
        transcript.clear()
        await assertThrows(
            HeyError.codeNotFound,
            try await client.calendars.recordingChanges(calendarId: 3, cursor: CalendarChangesCursor(since: "2026-09-15T10:00:00Z", version: "1")))
        XCTAssertEqual(transcript.log.last, "end:GetCalendarRecordingChanges:not_found")
    }
}
