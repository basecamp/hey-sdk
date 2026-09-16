import Foundation
import XCTest

@testable import Hey

final class IdentityTests: XCTestCase {
    func testTheFirstWeekDayGoesOutByNameAndComesBackByNumber() async throws {
        let hey = mockHey(ok(#"{"first_week_day":1}"#), ok(#"{"first_week_day":0}"#), ok(#"{"first_week_day":9}"#))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        let monday = try await client.identity.setFirstWeekDay(.monday)
        XCTAssertEqual(monday, .monday)
        XCTAssertEqual(log.started.last, "Identity.UpdateFirstWeekDay:identity:true:nil")
        XCTAssertEqual(hey.requests[0].method, "PUT")
        XCTAssertEqual(hey.requests[0].path, "/calendar/identity/first_week_day.json")
        XCTAssertEqual(hey.requests[0].body, #"{"identity_preference":{"first_week_day":"monday"}}"#)
        let sunday = try await client.identity.setFirstWeekDay(.sunday)
        XCTAssertEqual(sunday, .sunday, "0 is Sunday, as the identity serves it")
        let error = await assertThrows(HeyError.codeAPI, try await client.identity.setFirstWeekDay(.saturday))
        XCTAssertEqual(error?.message, "first week day 9 is not a day of the week")
        XCTAssertEqual(log.ended.last, "Identity.UpdateFirstWeekDay:api_error", "the hooks hear the failure the caller gets")
        XCTAssertEqual(log.ended.count, 3)
        XCTAssertNil(Weekday.fromIndex(7))
        XCTAssertEqual(Weekday.fromIndex(6), .saturday)
    }

    func testTheTimeFormatGoesOutAsTheWebTogglesFlagAndComesBackByName() async throws {
        let hey = mockHey(ok(#"{"time_format":"twenty_four_hour"}"#), ok(#"{"time_format":"twelve_hour"}"#), ok(#"{"time_format":"decimal"}"#))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        let twentyFour = try await client.identity.setTimeFormat(.twentyFourHour)
        XCTAssertEqual(twentyFour, .twentyFourHour)
        XCTAssertEqual(hey.requests[0].path, "/identity/time_format.json")
        XCTAssertEqual(hey.requests[0].body, #"{"twenty_four_hour_time_format":true}"#)
        let twelve = try await client.identity.setTimeFormat(.twelveHour)
        XCTAssertEqual(twelve, .twelveHour)
        XCTAssertEqual(hey.requests[1].body, #"{"twenty_four_hour_time_format":false}"#)
        let unread = await assertThrows(HeyError.codeAPI, try await client.identity.setTimeFormat(.twelveHour))
        XCTAssertEqual(
            unread?.message, #"time format "decimal" is neither "twelve_hour" nor "twenty_four_hour""#,
            "a format HEY stored that the SDK cannot read is HEY's answer failing, not the caller's mistake")
        XCTAssertEqual(log.ended.last, "Identity.UpdateTimeFormat:api_error")
        assertThrowsSync(HeyError.codeUsage) { try TimeFormat.parse("decimal") }
    }

    func testARefusedWriteEndsItsOneOperationWithTheRefusal() async throws {
        let hey = mockHey(status(422, #"{"error":"no"}"#))
        let transcript = Transcript()
        await assertThrows(HeyError.codeValidation, try await hey.client(hooks: transcript).identity.setFirstWeekDay(.monday))
        XCTAssertEqual(transcript.log, [
            "start:Identity.UpdateFirstWeekDay:identity:true:nil",
            "request:PUT:1",
            "response:422:validation",
            "end:UpdateFirstWeekDay:validation",
        ])
    }
}
