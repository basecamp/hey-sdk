import Foundation

/// A habit, as its writes take it.
public struct HabitParams: Sendable, Equatable {
    /// What the habit is called.
    public var name: String
    /// The icon HEY draws it with.
    public var icon: String
    /// The color HEY draws it in.
    public var color: String
    /// The days of the week the habit runs on, 0 for Sunday through 6 for Saturday.
    public var days: [Int]

    /// A habit with the parts given; a part left empty is not sent.
    public init(name: String = "", icon: String = "", color: String = "", days: [Int] = []) {
        self.name = name
        self.icon = icon
        self.color = color
        self.days = days
    }
}

/// Habits with the writes that take a habit in its parts, on top of the generated surface
/// (`create`, `update`, `complete`, `uncomplete`, `stop`, `resume`, `delete`). Both writes answer
/// the habit as a recording: HEY renders it as JSON on create and on update. There is no redirect
/// fallback, so a caller never sees a "success" that wrote nothing.
extension HabitsService {
    /// Starts a new habit and answers it as a recording.
    public func createHabit(params: HabitParams) async throws -> Recording {
        try await create(body: try habitBody(params))
    }

    /// Edits a habit and answers it as a recording. `habitId` is the recording's id, and fields
    /// left empty are kept.
    public func updateHabit(habitId: Int, params: HabitParams) async throws -> Recording {
        try await update(habitId: habitId, body: try habitBody(params))
    }
}

/// An empty field is left off the wire, so HEY keeps what the habit had. The model holds days as
/// 32-bit integers, so a day past that range is refused rather than sent as some other number.
func habitBody(_ params: HabitParams) throws -> HabitRequestContent {
    let days = try params.days.map { day in
        guard let exact = Int32(exactly: day) else {
            throw HeyError.usage(message: "habit day \(day) is out of range")
        }
        return exact
    }
    return HabitRequestContent(
        calendarHabit: HabitPayload(
            name: params.name.isEmpty ? nil : params.name,
            icon: params.icon.isEmpty ? nil : params.icon,
            color: params.color.isEmpty ? nil : params.color,
            days: days.isEmpty ? nil : days))
}
