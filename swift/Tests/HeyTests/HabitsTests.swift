import Foundation
import XCTest

@testable import Hey

final class HabitsTests: XCTestCase {
    func testAHabitIsWrittenInItsPartsWithTheEmptyOnesLeftOff() async throws {
        let habit = #"{"id":5,"type":"Calendar::Habit"}"#
        let hey = mockHey(ok(habit), ok(habit))
        let client = try hey.client()
        let created = try await client.habits.createHabit(params: HabitParams(name: "Run", icon: "shoe", color: "green", days: [1, 3, 5]))
        XCTAssertEqual(created.id, 5)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/calendar/habits.json")
        let written = try jsonMember(hey.requests[0].body, "calendar_habit")
        XCTAssertEqual(written["name"] as? String, "Run")
        XCTAssertEqual(written["days"] as? [Int], [1, 3, 5])

        _ = try await client.habits.updateHabit(habitId: 5, params: HabitParams(name: "Jog"))
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, "/calendar/habits/5.json")
        XCTAssertEqual(
            Set(try jsonMember(hey.requests[1].body, "calendar_habit").keys), ["name"], "fields left empty are kept by HEY, so they are not sent")
    }

    func testADayTheModelCannotHoldIsRefusedBeforeSending() async throws {
        let hey = mockHey()
        let client = try hey.client()
        await assertThrows(HeyError.codeUsage, try await client.habits.createHabit(params: HabitParams(name: "Run", days: [4_294_967_297])))
        XCTAssertTrue(hey.requests.isEmpty, "not sent as day 1")
    }
}
