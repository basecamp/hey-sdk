import Foundation

/// Selection conveniences on top of the generated surface. Every HEY posting endpoint is a bulk
/// one, so the methods here take the ids of the postings to act on; an empty selection is
/// refused before anything is sent.
extension PostingsService {
    /// Marks postings seen. The generated ``markSeen(body:)`` takes the request body.
    public func markPostingsSeen(postingIds: [Int]) async throws {
        try await bulk(Routes.markPostingsSeen, postingIds) { MarkPostingsRequestContent(postingIds: $0) }
    }

    /// Marks postings unseen.
    public func markPostingsUnseen(postingIds: [Int]) async throws {
        try await bulk(Routes.markPostingsUnseen, postingIds) { MarkPostingsRequestContent(postingIds: $0) }
    }

    /// Trashes postings. On a shared topic HEY removes your own access rather than trashing the
    /// thread for everybody on it.
    public func trashPostings(postingIds: [Int]) async throws {
        try await trashSelection(removeAccess: nil, postingIds)
    }

    /// Moves postings to the trash: ``trashPostings(postingIds:)`` under the name Go and Rust
    /// give it.
    public func moveToTrash(postingIds: [Int]) async throws {
        try await trashSelection(removeAccess: nil, postingIds)
    }

    /// Trashes postings, and trashes a shared topic for everybody on it rather than only dropping
    /// your own access.
    public func trashForEveryone(postingIds: [Int]) async throws {
        try await trashSelection(removeAccess: "false", postingIds)
    }

    /// A nil `remove_access` is left out of the request, which HEY reads as removing only your own
    /// access from a shared topic.
    private func trashSelection(removeAccess: String?, _ postingIds: [Int]) async throws {
        try await bulk(Routes.trashPostings, postingIds) { TrashPostingsRequestContent(postingIds: $0, removeAccess: removeAccess) }
    }

    /// Mutes postings, so their threads stop notifying.
    public func mutePostings(postingIds: [Int]) async throws {
        try await bulk(Routes.mutePostings, postingIds) { MarkPostingsRequestContent(postingIds: $0) }
    }

    /// Unmutes postings.
    public func unmutePostings(postingIds: [Int]) async throws {
        try await byIds(Routes.unmutePostings, postingIds)
    }

    /// Marks postings as spam. Past ten postings HEY hands the work to a background job, so the
    /// call comes back before the postings have moved.
    public func markPostingsSpam(postingIds: [Int]) async throws {
        try await bulk(Routes.markPostingsSpam, postingIds) { MarkPostingsRequestContent(postingIds: $0) }
    }

    /// Files postings into an existing Set Aside group.
    public func addPostingsToBoxGroup(boxId: Int, boxGroupId: Int, postingIds: [Int]) async throws {
        try await bulk(Routes.addPostingsToBoxGroup, postingIds) {
            AddPostingsToBoxGroupRequestContent(postingIds: $0, boxId: boxId, boxGroupId: boxGroupId)
        }
    }

    /// Takes postings out of whatever Set Aside group they are in.
    public func removePostingsFromBoxGroup(postingIds: [Int]) async throws {
        try await byIds(Routes.removePostingsFromBoxGroup, postingIds)
    }

    /// Labels postings with an existing folder.
    public func filePostings(folderId: Int, postingIds: [Int]) async throws {
        try await bulk(Routes.filePostings, postingIds) { FilePostingsRequestContent(postingIds: $0, folderId: folderId) }
    }

    /// Takes a label off postings, or every label when `folderId` is 0. Zero is not "every
    /// folder" to HEY, it is a folder that does not exist, so it is left out of the request
    /// rather than sent.
    public func unfilePostings(folderId: Int, postingIds: [Int]) async throws {
        var operation = try selection(Routes.unfilePostings, postingIds)
        operation.query("posting_ids", joinPostingIds(postingIds))
        if folderId != 0 { operation.query("folder_id", folderId) }
        try await client.sendVoid(operation)
    }

    /// Creates a folder and files postings into it. HEY serves no JSON endpoint for creating a
    /// folder on its own.
    public func createFolderForPostings(name: String, postingIds: [Int]) async throws {
        try await bulk(Routes.createFolderForPostings, postingIds) {
            CreateFolderForPostingsRequestContent(postingIds: $0, folder: FolderPayload(name: name))
        }
    }

    /// Bubbles postings up right away.
    public func bubbleUpPostingsNow(postingIds: [Int]) async throws {
        try await bulk(Routes.bubbleUpPostingsNow, postingIds) { MarkPostingsRequestContent(postingIds: $0) }
    }

    /// Schedules postings to bubble back up at a slot.
    public func schedulePostingsBubbleUp(slot: BubbleUpSlot, postingIds: [Int]) async throws {
        try await bulk(Routes.schedulePostingsBubbleUp, postingIds) {
            SchedulePostingsBubbleUpRequestContent(postingIds: $0, slot: slot.wire, date: slot.date)
        }
    }

    /// Drops the scheduled bubble up on postings.
    public func cancelPostingsBubbleUp(postingIds: [Int]) async throws {
        try await byIds(Routes.cancelPostingsBubbleUp, postingIds)
    }

    /// Reads a box's change feed from a cursor to the end of the increment, following the pages
    /// HEY names, and answers them combined. A full sync comes back as soon as HEY asks for one,
    /// with whatever was read before it dropped: the box has to be read in full anyway. Reading
    /// stops at the client's page limit; the answer then carries the cursor of the last page read
    /// rather than the end of the feed, and names the page it did not read in
    /// ``PostingChanges/nextPage``, which a complete answer never does.
    public func allChanges(boxId: Int, cursor: PostingChangesCursor) async throws -> PostingChanges {
        var added: [Posting] = []
        var updated: [Posting] = []
        var deleted: [DeletedPosting] = []
        var nextCursor: PostingChangesCursor?
        var next = cursor
        for _ in 0..<client.config.maxPages {
            let changes = try await self.changes(boxId: boxId, cursor: next)
            if changes.fullSyncRequired { return changes }
            added += changes.added
            updated += changes.updated
            deleted += changes.deleted
            nextCursor = changes.nextCursor
            guard let page = changes.nextPage else {
                return PostingChanges(added: added, updated: updated, deleted: deleted, nextPage: nil, nextCursor: nextCursor)
            }
            next = page
        }
        return PostingChanges(added: added, updated: updated, deleted: deleted, nextPage: next, nextCursor: nextCursor)
    }

    /// Reads everything that happened to a box's postings since a cursor, as Go's and Rust's
    /// conveniences do: the generated ``getBoxChanges(boxId:since:options:)`` answers the page as
    /// HEY serves it, this answers a ``PostingChanges`` with the cursor to resume from and the one
    /// answer the page cannot carry — a 409, which is HEY saying the cursor is too old or speaks
    /// another version, so the box has to be read in full. It goes without the response cache: a
    /// cursor URL never repeats, so a cached answer would never be revalidated and a long-running
    /// watch would grow the cache one dead entry per read.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a cursor with no `since`, or a page link
    ///   HEY named off its own origin.
    public func changes(boxId: Int, cursor: PostingChangesCursor) async throws -> PostingChanges {
        if cursor.since.isEmpty {
            throw HeyError.usage(message: "a change feed is read from a cursor: since is required")
        }
        var operation = try client.operation(Routes.getBoxPostingChanges, [boxId])
        operation.resourceId(boxId)
        operation.query("since", cursor.since)
        operation.queryOptional("v", cursor.version)
        operation.queryOptional("page", cursor.page)
        operation.queryOptional("per_page", cursor.perPage)
        operation.noCache()
        // The 409 is the answer the caller gets, not a failure, so the request goes quiet inside
        // an operation that ends well either way: the hooks hear the read succeed when what they
        // are handed is a full-sync answer.
        operation.quiet()
        let client = self.client
        let request = operation
        return try await client.asOperation(operation.info) {
            let page: Page<GetBoxPostingChangesResponseContent>
            do {
                page = try await client.sendPage(request)
            } catch HeyError.conflict {
                return PostingChanges(fullSyncRequired: true)
            }
            // Both links are read whole: the page within an increment can move the since, the
            // version or the size along with the page, and the read that follows sends what HEY
            // issued, not a page number pinned to the cursor this read started from.
            var nextPage: PostingChangesCursor?
            if let next = page.nextURL {
                guard isSameOrigin(next, client.baseURL) else {
                    throw HeyError.usage(message: "pagination Link header points to a different origin: \(originDescription(next))")
                }
                nextPage = try PostingChangesCursor.fromURL(next.absoluteString)
            }
            return PostingChanges(
                added: page.value.added ?? [],
                updated: page.value.updated ?? [],
                deleted: page.value.deleted ?? [],
                nextPage: nextPage,
                nextCursor: try page.nextCursor.map { try PostingChangesCursor.fromURL($0.absoluteString) },
                fullSyncRequired: false)
        }
    }

    /// Moves postings to a box.
    public func moveToBox(boxId: Int, postingIds: [Int]) async throws {
        try await bulk(Routes.movePostings, postingIds) { MovePostingsRequestContent(postingIds: $0, boxId: boxId) }
    }

    /// Moves postings to the box of a kind, resolving the box index once per client. An empty
    /// selection is refused before the index is read.
    public func moveTo(kind: BoxKind, postingIds: [Int]) async throws {
        let selected = try requireSelection(postingIds)
        try await moveToBox(boxId: try await client.boxes.idByKind(kind), postingIds: selected)
    }

    /// Moves postings to Set Aside.
    public func moveToSetAside(postingIds: [Int]) async throws {
        try await moveTo(kind: .setAside, postingIds: postingIds)
    }

    /// Moves postings to Reply Later.
    public func moveToReplyLater(postingIds: [Int]) async throws {
        try await moveTo(kind: .replyLater, postingIds: postingIds)
    }

    /// Moves postings to the Imbox.
    public func moveToImbox(postingIds: [Int]) async throws {
        try await moveTo(kind: .imbox, postingIds: postingIds)
    }

    /// Moves postings to The Feed.
    public func moveToFeed(postingIds: [Int]) async throws {
        try await moveTo(kind: .feed, postingIds: postingIds)
    }

    /// Moves postings to the Paper Trail.
    public func moveToPaperTrail(postingIds: [Int]) async throws {
        try await moveTo(kind: .paperTrail, postingIds: postingIds)
    }

    // MARK: - Selections

    private func requireSelection(_ postingIds: [Int]) throws -> [Int] {
        if postingIds.isEmpty { throw HeyError.usage(message: "at least one posting id is required") }
        return postingIds
    }

    /// Sends a bulk route over a selection: an empty one is refused before anything is sent, and a
    /// selection of one names the posting it acts on, for the hooks, as Go's and Rust's do.
    private func bulk<Body: Encodable>(_ route: Route, _ postingIds: [Int], _ body: ([Int]) -> Body) async throws {
        var operation = try selection(route, postingIds)
        try operation.json(body(postingIds))
        try await client.sendVoid(operation)
    }

    /// Sends the selection in the query, comma-joined, for the endpoints whose method carries no
    /// body — which is what the generated forms of those operations take as a raw string.
    private func byIds(_ route: Route, _ postingIds: [Int]) async throws {
        var operation = try selection(route, postingIds)
        operation.query("posting_ids", joinPostingIds(postingIds))
        try await client.sendVoid(operation)
    }

    /// The operation for a bulk route: an empty selection is refused before anything is sent, and a
    /// selection of one names the posting it acts on.
    private func selection(_ route: Route, _ postingIds: [Int]) throws -> Operation {
        _ = try requireSelection(postingIds)
        var operation = try client.operation(route, [])
        if postingIds.count == 1 { operation.resourceId(postingIds[0]) }
        return operation
    }

    private func joinPostingIds(_ postingIds: [Int]) -> String {
        postingIds.map(String.init).joined(separator: ",")
    }
}

/// When a posting bubbles back up. HEY resurfaces a posting at its morning hour of the day the
/// slot names — ``laterToday`` at its evening hour of the current day instead — and reads both
/// hours in UTC, like every hour it takes out of a JSON request.
public struct BubbleUpSlot: Sendable, Equatable {
    /// The slot as HEY's `slot` parameter names it.
    public let wire: String
    /// The day a ``custom(date:)`` slot names, and nil for the named ones: HEY works those out
    /// itself.
    public let date: String?

    private init(wire: String, date: String?) {
        self.wire = wire
        self.date = date
    }

    /// This evening. HEY's `today`.
    public static let laterToday = BubbleUpSlot(wire: "today", date: nil)

    /// Tomorrow morning.
    public static let tomorrow = BubbleUpSlot(wire: "tomorrow", date: nil)

    /// Saturday.
    public static let thisWeekend = BubbleUpSlot(wire: "weekend", date: nil)

    /// Monday.
    public static let nextWeek = BubbleUpSlot(wire: "next_week", date: nil)

    /// A day of the caller's choosing, as `YYYY-MM-DD`. HEY does not refuse one that has already
    /// passed — those postings bubble up on the next run of its scheduler — but a day the calendar
    /// does not have is refused here rather than sent.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a date the calendar does not have.
    public static func custom(date: String) throws -> BubbleUpSlot {
        guard isCalendarDay(date) else {
            throw HeyError.usage(message: "bubble up date \"\(date)\" is not a YYYY-MM-DD the calendar has")
        }
        return BubbleUpSlot(wire: "custom", date: date)
    }

    /// Whether the text is a `YYYY-MM-DD` the calendar has: a month of 1 to 12 and a day that month
    /// has, February 29 in leap years only.
    private static func isCalendarDay(_ text: String) -> Bool {
        let parts = text.split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0].count == 4, parts[1].count == 2, parts[2].count == 2,
              parts.allSatisfy({ $0.allSatisfy { $0.isASCII && $0.isNumber } }),
              let year = Int(parts[0]), let month = Int(parts[1]), let day = Int(parts[2])
        else { return false }
        guard (1...12).contains(month), day >= 1 else { return false }
        let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
        let days: Int
        switch month {
        case 2: days = leap ? 29 : 28
        case 4, 6, 9, 11: days = 30
        default: days = 31
        }
        return day <= days
    }
}

/// Where a read of a box's change feed starts: the instant the changes come after, the contract
/// version the feed speaks (HEY's `v`), the page within an increment while it has more than one,
/// and the page size when the URL named one. Read out of a changes URL HEY issued — a box's
/// `posting_changes_url`, or the cursor a page hands back — with ``fromURL(_:)``.
public struct PostingChangesCursor: Sendable, Equatable {
    /// The instant the changes come after.
    public var since: String
    /// The contract version the feed speaks, HEY's `v`.
    public var version: String?
    /// The page within an increment, while it has more than one.
    public var page: String?
    /// The page size, when the URL named one.
    public var perPage: String?

    /// A cursor from its parts.
    public init(since: String, version: String? = nil, page: String? = nil, perPage: String? = nil) {
        self.since = since
        self.version = version
        self.page = page
        self.perPage = perPage
    }

    /// Reads a cursor out of a changes URL HEY issued.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a URL that is not absolute.
    public static func fromURL(_ changesURL: String) throws -> PostingChangesCursor {
        guard let url = parseAbsoluteURL(changesURL) else {
            throw HeyError.usage(message: "changes URL is not an absolute URL")
        }
        return PostingChangesCursor(
            since: queryValue(url, "since") ?? "",
            version: queryValue(url, "v"),
            page: queryValue(url, "page"),
            perPage: queryValue(url, "per_page"))
    }
}

/// Everything that happened to a box's postings since a cursor. ``nextPage`` is set while this
/// increment has more pages to read now; ``nextCursor`` is set on the last page and is where the
/// next read resumes, and nil when nothing changed, in which case the cursor that produced this
/// page still stands. ``fullSyncRequired`` is HEY's 409: the cursor is too old or speaks another
/// version, nothing else here is set, and the box has to be read in full.
public struct PostingChanges: Sendable, Equatable {
    /// The postings that appeared in the box.
    public var added: [Posting]
    /// The postings that changed.
    public var updated: [Posting]
    /// The postings that left the box.
    public var deleted: [DeletedPosting]
    /// The page within this increment to read next, while there is one: a whole cursor, as HEY
    /// issued it.
    public var nextPage: PostingChangesCursor?
    /// Where to resume once this increment is read, when HEY named one.
    public var nextCursor: PostingChangesCursor?
    /// Whether HEY refused the cursor and the box has to be read in full.
    public var fullSyncRequired: Bool

    /// A set of changes from its parts.
    public init(
        added: [Posting] = [], updated: [Posting] = [], deleted: [DeletedPosting] = [],
        nextPage: PostingChangesCursor? = nil, nextCursor: PostingChangesCursor? = nil, fullSyncRequired: Bool = false
    ) {
        self.added = added
        self.updated = updated
        self.deleted = deleted
        self.nextPage = nextPage
        self.nextCursor = nextCursor
        self.fullSyncRequired = fullSyncRequired
    }
}
