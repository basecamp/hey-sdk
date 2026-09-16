import Foundation

/// The calendar as the periods it is drawn in — a day, a week, a year — on top of the generated
/// surface. Every read is scoped to the calendars the reader has switched on, which
/// ``CalendarsService/toggleSelection(calendarId:)`` changes.
///
/// A period is not the same answer as ``CalendarsService/getRecordings(calendarId:options:)``. A
/// calendar lists the recordings it holds, recurring ones included as the single rows they are
/// stored as; a period expands those into the occurrences that fall inside its window. Draw a week
/// from a calendar's recordings and a weekly meeting shows up once.
extension CalendarPeriodsService {
    /// Reads one day. The date is `YYYY-MM-DD`, or the literal `now` for today, which leaves it to
    /// HEY to decide what today is where the reader is.
    public func day(date: String) async throws -> CalendarPeriod {
        try await getDay(day: date)
    }

    /// Reads the days from a date onwards. HEY picks how many, so this is a window rather than a
    /// page: read on by asking again from the last day it answered. No date starts from today.
    public func days(startsAt: String? = nil) async throws -> [CalendarPeriod] {
        try await listDays(options: ListCalendarDaysOptions(startsAt: optionalDate(startsAt))).days
    }

    /// Reads the week a date falls in. The date is `YYYY-MM-DD`.
    public func week(date: String) async throws -> CalendarPeriod {
        try await getWeek(week: date)
    }

    /// Reads nine weeks. `startsAt` names the first of them; `centeredAt` centers them on a date
    /// instead, which is what the web app's scrolling week view asks for. Neither centers on
    /// today, and HEY takes `startsAt` when it is given both.
    public func weeks(startsAt: String? = nil, centeredAt: String? = nil) async throws -> [CalendarPeriod] {
        try await listWeeks(options: ListCalendarWeeksOptions(startsAt: optionalDate(startsAt), centeredAt: optionalDate(centeredAt))).weeks
    }

    /// Reads the year a date falls in, as the grid it is drawn as: one entry per day and the
    /// events that span more than one. A year does not carry every recording it holds.
    public func year(date: String) async throws -> CalendarYear {
        try await getYear(year: date)
    }
}

/// An empty date is left off the wire: sending `starts_at=` would ask HEY to parse an empty string
/// rather than pick the default itself.
private func optionalDate(_ value: String?) -> String? {
    guard let value, !value.isEmpty else { return nil }
    return value
}
