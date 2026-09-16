import Foundation

/// What an edit changes about a todo. A field left nil is left alone: HEY applies what it is sent
/// and keeps the rest, so a rename carries a title and says nothing about the day.
public struct TodoChanges: Sendable, Equatable {
    /// A new title. An empty one is no title, and changes nothing.
    public var title: String?
    /// The day the todo moves to, `YYYY-MM-DD`.
    public var startsAt: String?
    /// Whether the todo is in focus.
    public var focused: Bool?

    /// Changes naming only the fields given.
    public init(title: String? = nil, startsAt: String? = nil, focused: Bool? = nil) {
        self.title = title
        self.startsAt = startsAt
        self.focused = focused
    }
}

/// Calendar todos with the writes that take a todo in its parts, on top of the generated surface
/// (`create`, `update`, `complete`, `uncomplete`, `delete`). The day goes on the wire as a bare
/// `YYYY-MM-DD`, which HEY casts in the reader's time zone; an instant at UTC midnight would land
/// on the previous day once cast, so a day is checked to be one before it is sent.
extension CalendarTodosService {
    /// Creates a todo, filed on a day. No day files it on today where this machine is.
    public func createTodo(title: String, startsAt: String? = nil) async throws -> Recording {
        let day = try startsAt.map(todoCalendarDate) ?? todayLocalDate()
        return try await create(body: CreateCalendarTodoRequestContent(calendarTodo: CalendarTodoPayload(title: title, startsAt: day)))
    }

    /// Edits a todo. `todoId` is the recording's id. Changing nothing is refused rather than sent:
    /// an empty payload asks HEY to do nothing and answers as though it had done something.
    public func updateTodo(todoId: Int, changes: TodoChanges) async throws -> Recording {
        // An empty title is no title: HEY refuses a todo without one, so an empty one is left out
        // and reads as changing nothing rather than as clearing it.
        let title = changes.title.flatMap { $0.isEmpty ? nil : $0 }
        let startsAt = try changes.startsAt.map(todoCalendarDate)
        if title == nil && startsAt == nil && changes.focused == nil {
            throw HeyError.usage(message: "update calendar todo \(todoId): nothing to change")
        }
        return try await update(
            todoId: todoId,
            body: UpdateCalendarTodoRequestContent(calendarTodo: CalendarTodoChanges(title: title, startsAt: startsAt, focused: changes.focused)))
    }
}

/// A day as the calendar has it, or a refusal: Go and Rust hold the day in a date type that cannot
/// name a day the calendar lacks.
private func todoCalendarDate(_ day: String) throws -> String {
    guard OccurrenceId.isCalendarDate(day) else { throw HeyError.usage(message: "\"\(day)\" is not a YYYY-MM-DD the calendar has") }
    return day
}
