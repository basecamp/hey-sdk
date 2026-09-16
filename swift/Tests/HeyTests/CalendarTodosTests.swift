import Foundation
import XCTest

@testable import Hey

final class CalendarTodosTests: XCTestCase {
    func testATodoIsFiledOnABareDayAndAnEditThatChangesNothingIsRefused() async throws {
        let todo = #"{"id":1,"type":"Calendar::Todo"}"#
        let hey = mockHey(ok(todo), ok(todo), ok(todo))
        let client = try hey.client()
        _ = try await client.calendarTodos.createTodo(title: "Buy milk", startsAt: "2026-09-16")
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/calendar/todos.json")
        let filed = try jsonMember(hey.requests[0].body, "calendar_todo")
        XCTAssertEqual(filed["title"] as? String, "Buy milk")
        XCTAssertEqual(filed["starts_at"] as? String, "2026-09-16")

        _ = try await client.calendarTodos.createTodo(title: "Buy milk")
        let today = try XCTUnwrap(try jsonMember(hey.requests[1].body, "calendar_todo")["starts_at"] as? String)
        XCTAssertNotNil(today.range(of: #"^\d{4}-\d{2}-\d{2}$"#, options: .regularExpression), "no day is today, as a bare date: \(today)")

        _ = try await client.calendarTodos.updateTodo(todoId: 1, changes: TodoChanges(title: "", focused: true))
        XCTAssertEqual(hey.requests[2].method, "PATCH")
        XCTAssertEqual(hey.requests[2].path, "/calendar/todos/1.json")
        let changed = try jsonMember(hey.requests[2].body, "calendar_todo")
        XCTAssertEqual(Set(changed.keys), ["focused"], "an empty title is no title, and says nothing")
        await assertThrows(HeyError.codeUsage, try await client.calendarTodos.updateTodo(todoId: 1, changes: TodoChanges(title: "")))
        await assertThrows(HeyError.codeUsage, try await client.calendarTodos.updateTodo(todoId: 1, changes: TodoChanges(startsAt: "2026-02-30")))
        await assertThrows(HeyError.codeUsage, try await client.calendarTodos.createTodo(title: "Buy milk", startsAt: "tomorrow"))
        XCTAssertEqual(hey.requests.count, 3)
    }
}
