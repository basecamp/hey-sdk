import Foundation

/// A calendar as the index serves it, wrapped with what a live follower needs.
///
/// ``recordingChangesUrl`` is where the calendar's own recording changes feed starts; read it with
/// ``CalendarChangesCursor/fromUrl(_:)``. ``signedStreamName`` subscribes the calendar's stream
/// over Action Cable — a frame arriving there means the calendar changed, and the name is stable
/// for the calendar's life. The calendar changes feed's added bucket carries this same shape, so a
/// calendar learned of either way arrives subscribable.
public struct ListedCalendar: Codable, Sendable, Equatable {
    /// The calendar itself, as ``CalendarsService/list()`` serves it.
    public var calendar: HeyCalendar?
    /// Where the calendar's recording changes feed starts.
    public var recordingChangesUrl: String?
    /// The Action Cable stream that announces the calendar's changes.
    public var signedStreamName: String?

    /// A listed calendar with the parts given.
    public init(calendar: HeyCalendar? = nil, recordingChangesUrl: String? = nil, signedStreamName: String? = nil) {
        self.calendar = calendar
        self.recordingChangesUrl = recordingChangesUrl
        self.signedStreamName = signedStreamName
    }

    enum CodingKeys: String, CodingKey {
        case calendar
        case recordingChangesUrl = "recording_changes_url"
        case signedStreamName = "signed_stream_name"
    }
}

/// The full calendars index: every calendar with its changes URL and signed stream name, the
/// calendar-level changes feed's own URL, and the calendars the reader has switched on.
public struct CalendarList: Codable, Sendable, Equatable {
    /// Every calendar the identity sees, each with what a live follower needs.
    public var calendars: [ListedCalendar]
    /// Where the calendar-level changes feed starts.
    public var calendarChangesUrl: String?
    /// The calendars the reader has switched on, which every period read is scoped to.
    public var selectedCalendarIds: [Int]

    /// An index with the parts given.
    public init(calendars: [ListedCalendar] = [], calendarChangesUrl: String? = nil, selectedCalendarIds: [Int] = []) {
        self.calendars = calendars
        self.calendarChangesUrl = calendarChangesUrl
        self.selectedCalendarIds = selectedCalendarIds
    }

    /// Decodes the index, taking a list HEY left out as an empty one.
    public init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        calendars = try container.decodeIfPresent([ListedCalendar].self, forKey: .calendars) ?? []
        calendarChangesUrl = try container.decodeIfPresent(String.self, forKey: .calendarChangesUrl)
        selectedCalendarIds = try container.decodeIfPresent([Int].self, forKey: .selectedCalendarIds) ?? []
    }

    enum CodingKeys: String, CodingKey {
        case calendars
        case calendarChangesUrl = "calendar_changes_url"
        case selectedCalendarIds = "selected_calendar_ids"
    }
}

/// Where a read of a calendar changes feed starts. There are two feeds and they speak the same
/// cursor — the calendar-level feed behind a ``CalendarList``'s `calendarChangesUrl`, and each
/// calendar's own recording feed behind its ``ListedCalendar``'s `recordingChangesUrl`. ``since``
/// is an ISO 8601 timestamp with milliseconds and is exclusive; ``version`` is the contract
/// version the caller speaks, HEY's `v`.
///
/// Build one with ``fromUrl(_:)`` rather than by hand. The two server-issued URLs differ — a
/// recording changes URL carries `v=1`, which the recording feed refuses to answer without, while
/// a calendar changes URL carries no version at all — so only the server knows which pair its feed
/// wants.
public struct CalendarChangesCursor: Sendable, Equatable, Hashable {
    /// The instant the changes come after.
    public var since: String?
    /// The contract version the feed speaks.
    public var version: String?
    /// The page within an increment, while it has more than one.
    public var page: String?
    /// How many changes a page holds, when the URL named a size.
    public var perPage: String?

    /// A cursor with the parts given.
    public init(since: String? = nil, version: String? = nil, page: String? = nil, perPage: String? = nil) {
        self.since = since
        self.version = version
        self.page = page
        self.perPage = perPage
    }

    /// Renders the cursor onto a request. The version is never invented here: a cursor read from a
    /// server-issued URL carries whichever version that feed speaks.
    func apply(to operation: inout Operation) {
        operation.queryOptional("since", since)
        operation.queryOptional("v", version)
        operation.queryOptional("page", page)
        operation.queryOptional("per_page", perPage)
    }

    /// Reads a cursor out of a changes URL the server issued: a calendar list's
    /// `calendarChangesUrl`, a listed calendar's `recordingChangesUrl`, or the `Link` header either
    /// feed answered with.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when the URL is not absolute.
    public static func fromUrl(_ changesUrl: String) throws -> CalendarChangesCursor {
        guard let url = parseAbsoluteURL(changesUrl) else { throw HeyError.usage(message: "changes URL is not an absolute URL") }
        return fromParsed(url)
    }

    static func fromParsed(_ url: URL) -> CalendarChangesCursor {
        let items = URLComponents(url: url, resolvingAgainstBaseURL: true)?.queryItems ?? []
        func parameter(_ name: String) -> String? {
            guard let value = items.first(where: { $0.name == name })?.value, !value.isEmpty else { return nil }
            return value
        }
        return CalendarChangesCursor(since: parameter("since"), version: parameter("v"), page: parameter("page"), perPage: parameter("per_page"))
    }
}

/// A calendar the changes feed reports gone.
public struct DeletedCalendar: Codable, Sendable, Equatable {
    /// The id the calendar had.
    public var id: Int
    /// When it went.
    public var deletedAt: String

    /// A deletion of a calendar at an instant.
    public init(id: Int, deletedAt: String) {
        self.id = id
        self.deletedAt = deletedAt
    }

    enum CodingKeys: String, CodingKey {
        case id
        case deletedAt = "deleted_at"
    }
}

/// Everything that happened to the calendar list since a cursor. Added calendars arrive as
/// ``ListedCalendar``, so a new calendar comes with the changes URL and signed stream name a live
/// follower needs.
///
/// ``nextPage`` is set while this increment has more pages to read now. ``nextCursor`` is set on
/// the last page and is where the next read should resume; it is nil when nothing changed, in
/// which case the cursor that produced this page still stands. Unlike the recording feed, this one
/// never falls too far behind, so there is no full sync to ask for.
public struct CalendarChanges: Sendable, Equatable {
    /// The calendars that appeared, each with what a live follower needs.
    public var added: [ListedCalendar]
    /// The calendars that changed.
    public var updated: [HeyCalendar]
    /// The calendars that went.
    public var deleted: [DeletedCalendar]
    /// The next page of this increment, while it has one: a whole cursor, as HEY issued it.
    public var nextPage: CalendarChangesCursor?
    /// Where the next read resumes, once the increment is read to its end.
    public var nextCursor: CalendarChangesCursor?

    /// A set of changes with the parts given.
    public init(
        added: [ListedCalendar] = [], updated: [HeyCalendar] = [], deleted: [DeletedCalendar] = [],
        nextPage: CalendarChangesCursor? = nil, nextCursor: CalendarChangesCursor? = nil
    ) {
        self.added = added
        self.updated = updated
        self.deleted = deleted
        self.nextPage = nextPage
        self.nextCursor = nextCursor
    }
}

/// A recording the changes feed reports gone. ``type`` is the recordable type key the recording
/// was grouped under while it existed.
public struct DeletedRecording: Codable, Sendable, Equatable {
    /// The id the recording had.
    public var id: Int
    /// When it went.
    public var deletedAt: String
    /// The recordable type key it was grouped under, `Calendar::Event` and the like.
    public var type: String?

    /// A deletion of a recording at an instant.
    public init(id: Int, deletedAt: String, type: String? = nil) {
        self.id = id
        self.deletedAt = deletedAt
        self.type = type
    }

    enum CodingKeys: String, CodingKey {
        case id
        case deletedAt = "deleted_at"
        case type
    }
}

/// Everything that happened to a calendar's recordings since a cursor.
///
/// ``added`` and ``updated`` keep the wire's grouping by recordable type key — `Calendar::Event`,
/// `Calendar::Habit`, `Calendar::Habit::Completion`, `Calendar::DayTitle`,
/// `Calendar::DayBackground`, `Calendar::TimeTrack`, `Calendar::Todo`, `Calendar::Countdown`,
/// `Calendar::JournalEntry` — the server owns that vocabulary. ``deleted`` is one deduplicated
/// list instead: the wire groups deletions by type key too, but repeats the whole deleted
/// collection under every key it groups, so the map shape carries nothing beyond each record's
/// own `type`, which is authoritative.
///
/// ``nextPage`` is set while this increment has more pages to read now. ``nextCursor`` is set on
/// the last page and is where the next read should resume; it is nil when nothing changed, in
/// which case the cursor that produced this page still stands. ``fullSyncRequired`` is set when
/// the cursor is too far behind for an increment to carry the difference — or speaks a version the
/// feed no longer does — and the calendar has to be read in full instead; nothing else is set then.
public struct RecordingChanges: Sendable, Equatable {
    /// The recordings that appeared, grouped by recordable type key.
    public var added: [String: [Recording]]
    /// The recordings that changed, grouped by recordable type key.
    public var updated: [String: [Recording]]
    /// The recordings that went, each once.
    public var deleted: [DeletedRecording]
    /// The next page of this increment, while it has one: a whole cursor, as HEY issued it.
    public var nextPage: CalendarChangesCursor?
    /// Where the next read resumes, once the increment is read to its end.
    public var nextCursor: CalendarChangesCursor?
    /// Whether the cursor is too far behind and the calendar has to be read in full.
    public var fullSyncRequired: Bool

    /// A set of changes with the parts given.
    public init(
        added: [String: [Recording]] = [:], updated: [String: [Recording]] = [:], deleted: [DeletedRecording] = [],
        nextPage: CalendarChangesCursor? = nil, nextCursor: CalendarChangesCursor? = nil, fullSyncRequired: Bool = false
    ) {
        self.added = added
        self.updated = updated
        self.deleted = deleted
        self.nextPage = nextPage
        self.nextCursor = nextCursor
        self.fullSyncRequired = fullSyncRequired
    }
}

/// Calendars with the index as a live follower needs it, the two change feeds, and the selection
/// every period read is scoped to, on top of the generated surface (`list`, `toggle`,
/// `getRecordings`).
///
/// Neither feed's answers are cached. A cursor URL never repeats, so a cached response would never
/// be revalidated, and a long-running watch would grow the cache by one dead entry per read.
extension CalendarsService {
    /// Lists the calendars with everything ``list()`` throws away: each calendar's recording
    /// changes URL and signed stream name, and the calendar changes URL. It is the same read,
    /// decoded into the fuller shape the wire already carries.
    public func listWithChanges() async throws -> CalendarList {
        try await client.send(try client.operation(Routes.listCalendars, []))
    }

    /// Switches a calendar in or out of the reader's selection and answers the ids the selection
    /// is left holding. Every ``CalendarPeriodsService`` read is scoped to that selection, so this
    /// is how a client changes which calendars a day, week or year is drawn from.
    public func toggleSelection(calendarId: Int) async throws -> [Int] {
        try await toggle(calendarId: calendarId).selectedCalendarIds
    }

    /// Reads the calendar changes feed from a cursor to its end, following the pages the feed
    /// hands out. Reading stops at the client's page limit; the answer then names the page it did
    /// not read in ``CalendarChanges/nextPage``, which a complete answer never does, so it cannot
    /// pass for the end of the feed.
    public func allCalendarChanges(cursor: CalendarChangesCursor) async throws -> CalendarChanges {
        var all = CalendarChanges()
        var next = cursor
        for _ in 0..<client.config.maxPages {
            let changes = try await calendarChanges(cursor: next)
            all.added += changes.added
            all.updated += changes.updated
            all.deleted += changes.deleted
            all.nextCursor = changes.nextCursor
            guard let page = changes.nextPage else {
                all.nextPage = nil
                return all
            }
            next = page
        }
        all.nextPage = next
        return all
    }

    /// Reads one page of the calendar changes feed.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when the cursor has no `since`.
    public func calendarChanges(cursor: CalendarChangesCursor) async throws -> CalendarChanges {
        if (cursor.since ?? "").isEmpty {
            throw HeyError.usage(message: "a since cursor is required — start from the list's calendarChangesUrl")
        }
        var operation = client.request(.get, "/calendar/changes")
        operation.info = calendarChangesInfo("GetCalendarChanges", "calendar")
        cursor.apply(to: &operation)
        operation.noCache()
        // Read inside the operation, as the recording feed is, so a link the client refuses ends
        // the operation the hooks hear with that refusal.
        operation.quiet()
        let request = operation
        return try await client.asOperation(operation.info) {
            let page = try await client.sendPage(request, as: CalendarChangesPayload.self)
            let (nextPage, nextCursor) = try changesCursors(page.nextURL, page.nextCursor)
            return CalendarChanges(
                added: page.value.added, updated: page.value.updated, deleted: page.value.deleted, nextPage: nextPage,
                nextCursor: nextCursor)
        }
    }

    /// Reads a calendar's recording changes feed from a cursor to its end, following the pages the
    /// feed hands out. A cursor the feed has left behind ends the walk on the spot with
    /// ``RecordingChanges/fullSyncRequired``. Reading stops at the client's page limit; the answer
    /// then names the page it did not read in ``RecordingChanges/nextPage``, which a complete
    /// answer never does.
    public func allRecordingChanges(calendarId: Int, cursor: CalendarChangesCursor) async throws -> RecordingChanges {
        var all = RecordingChanges()
        var next = cursor
        for _ in 0..<client.config.maxPages {
            let changes = try await recordingChanges(calendarId: calendarId, cursor: next)
            if changes.fullSyncRequired { return changes }
            all.added.merge(changes.added) { $0 + $1 }
            all.updated.merge(changes.updated) { $0 + $1 }
            all.deleted += changes.deleted
            all.nextCursor = changes.nextCursor
            guard let page = changes.nextPage else {
                all.nextPage = nil
                return all
            }
            next = page
        }
        all.nextPage = next
        return all
    }

    /// Reads one page of a calendar's recording changes feed.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when the cursor has no `since` or no version.
    public func recordingChanges(calendarId: Int, cursor: CalendarChangesCursor) async throws -> RecordingChanges {
        if (cursor.since ?? "").isEmpty {
            throw HeyError.usage(message: "a since cursor is required — start from the calendar's recordingChangesUrl")
        }
        if (cursor.version ?? "").isEmpty {
            throw HeyError.usage(
                message: "a feed version is required — read the calendar's recordingChangesUrl with CalendarChangesCursor.fromUrl")
        }
        var operation = client.request(.get, "/calendars/\(calendarId)/recording/changes")
        operation.info = calendarChangesInfo("GetCalendarRecordingChanges", "recording")
        operation.resourceId(calendarId)
        cursor.apply(to: &operation)
        operation.noCache()
        // A 409 is the feed saying the cursor is too far behind for an increment to carry the
        // difference, or speaks a version it no longer does: an answer, not a failure. So the
        // request goes quiet inside an operation that ends well either way, and the hooks hear the
        // read succeed when what they are handed is a full-sync answer.
        operation.quiet()
        let request = operation
        return try await client.asOperation(operation.info) {
            let page: Page<RecordingChangesPayload>
            do {
                page = try await client.sendPage(request, as: RecordingChangesPayload.self)
            } catch HeyError.conflict {
                return RecordingChanges(fullSyncRequired: true)
            }
            let (nextPage, nextCursor) = try changesCursors(page.nextURL, page.nextCursor)
            return RecordingChanges(
                added: page.value.added, updated: page.value.updated, deleted: flattenDeletedRecordings(page.value.deleted),
                nextPage: nextPage, nextCursor: nextCursor, fullSyncRequired: false)
        }
    }

    /// The cursors the feed's `Link` header names, the page one first: while an increment has more
    /// pages the link carries a page cursor, and the last page carries a fresh `since` cursor
    /// instead. Both are read whole, as HEY issued them, and a link off HEY's origin is refused
    /// rather than followed.
    private func changesCursors(_ nextURL: URL?, _ nextCursor: URL?) throws -> (CalendarChangesCursor?, CalendarChangesCursor?) {
        guard let linked = nextURL ?? nextCursor else { return (nil, nil) }
        guard isSameOrigin(linked, client.baseURL) else {
            throw HeyError.usage(message: "changes Link header points to a different origin: \(originDescription(linked))")
        }
        let cursor = CalendarChangesCursor.fromParsed(linked)
        return cursor.page != nil ? (cursor, nil) : (nil, cursor)
    }
}

private func calendarChangesInfo(_ operation: String, _ resourceType: String) -> OperationInfo {
    OperationInfo(service: "Calendars", operation: operation, resourceType: resourceType, isMutation: false)
}

/// Folds the wire's per-type deleted buckets into one list. The server repeats the whole deleted
/// collection under every type key it groups, so the same deletion arrives once per key: the id
/// dedupe drops the repeats, and each record's own `type` says what it was. The buckets are read
/// in key order, as Rust's are, since a decoded dictionary keeps no order of its own.
func flattenDeletedRecordings(_ buckets: [String: [DeletedRecording]]) -> [DeletedRecording] {
    var seen = Set<Int>()
    return buckets.keys.sorted().flatMap { buckets[$0] ?? [] }.filter { seen.insert($0.id).inserted }
}

struct CalendarChangesPayload: Decodable, Sendable {
    var added: [ListedCalendar]
    var updated: [HeyCalendar]
    var deleted: [DeletedCalendar]

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        added = try container.decodeIfPresent([ListedCalendar].self, forKey: .added) ?? []
        updated = try container.decodeIfPresent([HeyCalendar].self, forKey: .updated) ?? []
        deleted = try container.decodeIfPresent([DeletedCalendar].self, forKey: .deleted) ?? []
    }

    enum CodingKeys: String, CodingKey {
        case added, updated, deleted
    }
}

struct RecordingChangesPayload: Decodable, Sendable {
    var added: [String: [Recording]]
    var updated: [String: [Recording]]
    var deleted: [String: [DeletedRecording]]

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        added = try container.decodeIfPresent([String: [Recording]].self, forKey: .added) ?? [:]
        updated = try container.decodeIfPresent([String: [Recording]].self, forKey: .updated) ?? [:]
        deleted = try container.decodeIfPresent([String: [DeletedRecording]].self, forKey: .deleted) ?? [:]
    }

    enum CodingKeys: String, CodingKey {
        case added, updated, deleted
    }
}
