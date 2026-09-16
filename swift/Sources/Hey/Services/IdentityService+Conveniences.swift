/// A day of the week, as HEY's identity preferences take and answer it. HEY takes the day as its
/// lowercased English name and answers it as an index the way it serves the identity: 0 is Sunday.
public enum Weekday: Int, Sendable, Equatable, CaseIterable {
    /// Sunday, HEY's 0.
    case sunday = 0
    /// Monday.
    case monday = 1
    /// Tuesday.
    case tuesday = 2
    /// Wednesday.
    case wednesday = 3
    /// Thursday.
    case thursday = 4
    /// Friday.
    case friday = 5
    /// Saturday.
    case saturday = 6

    /// The day as HEY's `first_week_day` parameter names it.
    public var wire: String {
        switch self {
        case .sunday: return "sunday"
        case .monday: return "monday"
        case .tuesday: return "tuesday"
        case .wednesday: return "wednesday"
        case .thursday: return "thursday"
        case .friday: return "friday"
        case .saturday: return "saturday"
        }
    }

    /// The day as HEY's identity numbers it, Sunday being 0.
    public var index: Int { rawValue }

    /// The day HEY's identity numbers `index`, or nil for a number that is not a day of the week.
    public static func fromIndex(_ index: Int) -> Weekday? {
        Weekday(rawValue: index)
    }
}

/// The clock HEY renders times on.
public enum TimeFormat: String, Sendable, Equatable, CaseIterable {
    /// A 12-hour clock, HEY's `twelve_hour`.
    case twelveHour = "twelve_hour"
    /// A 24-hour clock, HEY's `twenty_four_hour`.
    case twentyFourHour = "twenty_four_hour"

    /// The format as HEY's identity names it.
    public var wire: String { rawValue }

    /// Reads a format as HEY writes it.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for anything else.
    public static func parse(_ source: String) throws -> TimeFormat {
        guard let format = TimeFormat(rawValue: source) else {
            throw HeyError.usage(message: "time format \"\(source)\" is neither \"twelve_hour\" nor \"twenty_four_hour\"")
        }
        return format
    }
}

/// The two identity preferences HEY lets a client write, on top of the generated surface (`get`,
/// `getNavigation`, `updateFirstWeekDay`, `updateTimeFormat`).
extension IdentityService {
    /// Sets which day the identity's calendar weeks start on, and answers the day HEY stored. The
    /// write reaches every HEY client — web, mobile and this SDK read the same identity
    /// preference. A stored day that is not one is read inside the operation, so the hooks hear
    /// the failure the caller gets.
    ///
    /// - Throws: ``HeyError/api(message:httpStatus:retryable:detail:)`` when HEY answers a day
    ///   that is not a day of the week.
    public func setFirstWeekDay(_ day: Weekday) async throws -> Weekday {
        var operation = try client.operation(Routes.updateFirstWeekDay, [])
        try operation.json(UpdateFirstWeekDayRequestContent(identityPreference: FirstWeekDayParams(firstWeekDay: day.wire)))
        operation.quiet()
        let client = self.client
        let request = operation
        return try await client.asOperation(operation.info) {
            let stored = try await client.send(request, as: UpdateFirstWeekDayResponseContent.self)
            guard let weekday = Weekday.fromIndex(Int(stored.firstWeekDay)) else {
                throw HeyError.api(
                    message: "first week day \(stored.firstWeekDay) is not a day of the week", httpStatus: nil, retryable: false,
                    detail: ErrorDetail())
            }
            return weekday
        }
    }

    /// Sets whether HEY renders times on a 12-hour or a 24-hour clock, and answers the format HEY
    /// stored. A stored format that is neither is HEY's answer failing to read, not a mistake of
    /// the caller's, so it is an API error, heard by the hooks as the caller gets it.
    ///
    /// - Throws: ``HeyError/api(message:httpStatus:retryable:detail:)`` when HEY answers a format
    ///   that is neither.
    public func setTimeFormat(_ format: TimeFormat) async throws -> TimeFormat {
        var operation = try client.operation(Routes.updateTimeFormat, [])
        try operation.json(UpdateTimeFormatRequestContent(twentyFourHourTimeFormat: format == .twentyFourHour))
        operation.quiet()
        let client = self.client
        let request = operation
        return try await client.asOperation(operation.info) {
            let stored = try await client.send(request, as: UpdateTimeFormatResponseContent.self)
            guard let format = TimeFormat(rawValue: stored.timeFormat) else {
                throw HeyError.api(
                    message: "time format \"\(stored.timeFormat)\" is neither \"twelve_hour\" nor \"twenty_four_hour\"",
                    httpStatus: nil, retryable: false, detail: ErrorDetail())
            }
            return format
        }
    }
}
