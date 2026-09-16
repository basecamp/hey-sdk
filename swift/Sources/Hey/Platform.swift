import Foundation

/// The current instant as an ISO 8601 timestamp with milliseconds, the form HEY takes a time in.
func nowISO8601() -> String {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter.string(from: Date())
}

/// Today where this machine is, as `YYYY-MM-DD`: the day a calendar write with no day named is
/// filed on, as the other SDKs file it.
func todayLocalDate() -> String {
    let formatter = DateFormatter()
    formatter.locale = Locale(identifier: "en_US_POSIX")
    formatter.calendar = Calendar(identifier: .gregorian)
    formatter.timeZone = .current
    formatter.dateFormat = "yyyy-MM-dd"
    return formatter.string(from: Date())
}
