import Foundation

/// The entry a day holds, read and written as its content, on top of the generated surface
/// (`getEntry`, `listEntries`, `updateEntry`). A day has at most one entry. HEY answers it as a
/// calendar recording carrying the full text (`content`) and the rich-text HTML (`contentHtml`); a
/// day with no entry answers 204 and no body, which reads here as nil.
extension JournalService {
    /// The rich-text HTML of the day's journal entry, falling back to its plain text, or nil when
    /// the day has no entry. `day` is `YYYY-MM-DD`.
    ///
    /// The fallback covers an empty `contentHtml` as well as a missing one: HEY serves the key
    /// blank on an entry it has no rendered body for, and blank is not the entry.
    public func getContent(day: String) async throws -> String? {
        var operation = try client.operation(Routes.getJournalEntry, [day])
        operation.operationName("GetJournalContent")
        guard let entry = try await journalRecording(operation) else { return nil }
        if let html = entry.contentHtml, !html.isEmpty { return html }
        return entry.content
    }

    /// The day's journal entry, or nil when it has none. The generated ``getEntry(day:)`` reads the
    /// same route but takes the empty answer for a day without an entry as a body it could not
    /// decode.
    public func entry(day: String) async throws -> Recording? {
        try await journalRecording(try client.operation(Routes.getJournalEntry, [day]))
    }

    /// Writes the day's journal entry, creating it if needed, and answers it as a recording. Empty
    /// content removes the entry, which HEY answers with nothing, so the result is nil.
    public func updateContent(day: String, content: String) async throws -> Recording? {
        var operation = try client.operation(Routes.updateJournalEntry, [day])
        try operation.json(UpdateJournalEntryRequestContent(calendarJournalEntry: JournalEntryPayload(content: content)))
        return try await journalRecording(operation)
    }

    /// The recording an answer carries, or nil for the bodiless answer a day without an entry gets.
    private func journalRecording(_ operation: HeyOperation) async throws -> Recording? {
        try await client.execute(operation) { response in
            response.body.isEmpty ? nil : try response.json(Recording.self)
        }
    }
}
