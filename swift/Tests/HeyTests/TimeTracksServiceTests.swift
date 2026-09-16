import Foundation
import XCTest

@testable import Hey

final class TimeTracksServiceTests: XCTestCase {
    func testACategoryIsMadeRenamedAndRemovedAsForms() async throws {
        let redirect = status(302, nil, [("Location", "/calendar/time_tracks/categories")])
        let hey = mockHey(redirect, redirect, redirect)
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.timeTracks.createCategory(title: "Client work")
        try await client.timeTracks.updateCategory(categoryId: 31, title: "Billable")
        try await client.timeTracks.deleteCategory(categoryId: 31)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/calendar/time_tracks/categories")
        XCTAssertEqual(hey.requests[0].header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(hey.requests[0].header("Accept"), browserAcceptHeader)
        XCTAssertEqual(fields(hey.requests[0].body), ["category[title]=Client work"])
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, "/calendar/time_tracks/categories/31")
        XCTAssertEqual(fields(hey.requests[1].body), ["category[title]=Billable"])
        XCTAssertEqual(hey.requests[2].method, "DELETE")
        XCTAssertEqual(hey.requests[2].path, "/calendar/time_tracks/categories/31")
        XCTAssertEqual(hey.requests[2].body, "")
        XCTAssertEqual(log.started, [
            "TimeTracks.CreateTimeTrackCategory:category:true:nil",
            "TimeTracks.UpdateTimeTrackCategory:category:true:31",
            "TimeTracks.DeleteTimeTrackCategory:category:true:31",
        ])
    }

    func testTheExportIsTheCsvHeyStreamedFromTheBarePath() async throws {
        let csv = "Start,End,Duration,Category,Notes\n2026-04-06 09:00,2026-04-06 11:00,2:00,Client work,Kitchen remodel call\n"
        let hey = mockHey(Answer(status: 200, body: csv, headers: [("Content-Type", "text/csv")]))
        let log = OperationLog()
        let exported = try await hey.client(hooks: log).timeTracks.export()
        XCTAssertEqual(String(decoding: exported, as: UTF8.self), csv, "the CSV comes back verbatim")
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "GET")
        XCTAssertEqual(request.path, "/calendar/time_tracks/exports", "HEY streams the export from the bare path, not a .json one")
        XCTAssertEqual(request.header("Accept"), "text/csv")
        XCTAssertEqual(log.started, ["TimeTracks.ExportTimeTracks:time_track:false:nil"])
    }

    func testARunningTrackIsAConflictAndStoppingAnnouncesItself() async throws {
        let hey = mockHey(status(409, #"{"error":"A time track is already running"}"#), ok(#"{"id":1,"type":"Calendar::TimeTrack"}"#))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        let error = await assertThrows(HeyError.codeConflict, try await client.timeTracks.startTracking())
        XCTAssertEqual(error?.message, "A time track is already running")
        try await client.timeTracks.stop(timeTrackId: 1)
        XCTAssertEqual(log.started.map { $0.split(separator: ":")[0] }, ["TimeTracks.StartTimeTrack", "TimeTracks.StopTimeTrack"])
        XCTAssertEqual(
            log.ended, ["TimeTracks.StartTimeTrack:conflict", "TimeTracks.StopTimeTrack:nil"],
            "the rewording happens inside the operation, so the hooks hear the failure the caller gets")
        XCTAssertEqual(hey.requests[1].method, "PUT")
        XCTAssertEqual(hey.requests[1].path, "/calendar/time_tracks/1.json")
        let track = try XCTUnwrap(try jsonObject(hey.requests[1].body)["calendar_time_track"] as? [String: Any])
        XCTAssertNotNil(track["ends_at"])
        XCTAssertNil(track["category_title"], "a plain stop files the track under nothing")
    }

    func testStoppingAndFilingNamesTheCategoryAndAnEmptyOneIsNone() async throws {
        let hey = mockHey(ok(#"{"id":1,"type":"Calendar::TimeTrack"}"#), ok(#"{"id":1,"type":"Calendar::TimeTrack"}"#))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.timeTracks.stopAndFile(timeTrackId: 1, categoryTitle: "Billable")
        try await client.timeTracks.stopAndFile(timeTrackId: 1, categoryTitle: "")
        let filed = try XCTUnwrap(try jsonObject(hey.requests[0].body)["calendar_time_track"] as? [String: Any])
        XCTAssertEqual(filed["category_title"] as? String, "Billable")
        XCTAssertNotNil(filed["ends_at"])
        let unfiled = try XCTUnwrap(try jsonObject(hey.requests[1].body)["calendar_time_track"] as? [String: Any])
        XCTAssertNil(unfiled["category_title"], "an empty category is no category")
        XCTAssertEqual(log.started, ["TimeTracks.StopTimeTrack:time_track:true:1", "TimeTracks.StopTimeTrack:time_track:true:1"])
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
