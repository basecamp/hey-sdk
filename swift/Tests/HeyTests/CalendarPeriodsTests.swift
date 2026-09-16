import Foundation
import XCTest

@testable import Hey

final class CalendarPeriodsTests: XCTestCase {
    func testCalendarPeriodsAreReadByTheDateTheyAreDrawnFrom() async throws {
        let period = #"{"starts_at":"2026-09-15","ends_at":"2026-09-15","kind":"day","recordings":{}}"#
        let year = #"{"starts_at":"2026-01-01","ends_at":"2026-12-31","kind":"year","padding_days_count":3,"days":[],"spanned_events":[]}"#
        let hey = mockHey(ok(period), ok(#"{"days":[\#(period)]}"#), ok(period), ok(#"{"weeks":[]}"#), ok(year))
        let client = try hey.client()
        _ = try await client.calendarPeriods.day(date: "now")
        XCTAssertEqual(hey.requests[0].path, "/calendar/days/now.json")
        let days = try await client.calendarPeriods.days(startsAt: "")
        XCTAssertEqual(days.count, 1)
        XCTAssertEqual(hey.requests[1].path, "/calendar/days.json")
        XCTAssertNil(hey.requests[1].query("starts_at"), "an empty date is left off the wire for HEY to pick the default")
        _ = try await client.calendarPeriods.week(date: "2026-09-15")
        XCTAssertEqual(hey.requests[2].path, "/calendar/weeks/2026-09-15.json")
        _ = try await client.calendarPeriods.weeks(centeredAt: "2026-09-15")
        XCTAssertEqual(hey.requests[3].path, "/calendar/weeks.json")
        XCTAssertEqual(hey.requests[3].query("centered_at"), "2026-09-15")
        XCTAssertNil(hey.requests[3].query("starts_at"))
        let drawn = try await client.calendarPeriods.year(date: "2026-01-01")
        XCTAssertEqual(drawn.paddingDaysCount, 3)
        XCTAssertEqual(hey.requests[4].path, "/calendar/years/2026-01-01.json")
    }

    func testADateThatIsGivenGoesOnTheWire() async throws {
        let hey = mockHey(ok(#"{"days":[]}"#), ok(#"{"weeks":[]}"#))
        let client = try hey.client()
        _ = try await client.calendarPeriods.days(startsAt: "2026-09-15")
        XCTAssertEqual(hey.requests[0].query("starts_at"), "2026-09-15")
        _ = try await client.calendarPeriods.weeks(startsAt: "2026-09-14", centeredAt: "")
        XCTAssertEqual(hey.requests[1].query("starts_at"), "2026-09-14")
        XCTAssertNil(hey.requests[1].query("centered_at"))
    }
}
