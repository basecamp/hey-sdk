/// The two decisions the Screener takes.
public enum ClearanceStatus: String, Sendable, Equatable, CaseIterable {
    /// Screened in: the sender's mail arrives.
    case approved = "approved"
    /// Screened out: the sender's mail is kept away.
    case denied = "denied"

    /// The decision as HEY's `status` parameter names it.
    public var wire: String { rawValue }

    /// Reads a decision as HEY writes it.
    ///
    /// - Throws: ``HeyError/validation(message:httpStatus:detail:)`` for anything else.
    public static func parse(_ source: String) throws -> ClearanceStatus {
        guard let status = ClearanceStatus(rawValue: source) else {
            throw HeyError.validation(
                message: "clearance status must be \"approved\" or \"denied\", got \"\(source)\"", httpStatus: 422, detail: ErrorDetail())
        }
        return status
    }
}

/// What to do beyond setting the status. HEY reads each of these for truthiness, so one left alone
/// stays off the wire entirely rather than going out as a false.
public struct ScreenOptions: Sendable, Equatable {
    /// Files everything the sender sends into that box rather than the Imbox.
    public var designationBoxId: Int?
    /// Marks the topics already waiting as spam and trains the filter on them.
    public var spam: Bool
    /// Screens the sender in without their waiting mail arriving unread.
    public var markTopicsAsSeen: Bool

    /// Options from their parts; every one is off unless given.
    public init(designationBoxId: Int? = nil, spam: Bool = false, markTopicsAsSeen: Bool = false) {
        self.designationBoxId = designationBoxId
        self.spam = spam
        self.markTopicsAsSeen = markTopicsAsSeen
    }
}

/// The Screener: who is waiting to be let in, and letting them in or turning them away, on top of
/// the generated surface (`get`, `getMy`, `update`, `updateMy`, `bulkUpdate`, `punt`). Clearing
/// the Screener is the generated ``punt()``. The work it starts is queued, so everyone waiting is
/// still pending when it answers; they are dropped and reexamined the next time they write, so
/// nothing is decided for them.
extension ClearancesService {
    /// How many senders are waiting, without fetching them. This is the cheap read HEY's own apps
    /// sync for the Screener badge; ``pending(page:)`` is the senders themselves.
    public func pendingCount() async throws -> Int {
        Int(try await summary().pendingClearancesCount ?? 0)
    }

    /// Everything HEY says about the Screener without the queue itself: how many senders are
    /// waiting, and the signed stream name to subscribe to on HEY's cable server to be told when
    /// that changes.
    public func summary() async throws -> ClearanceSummary {
        try await get().value
    }

    /// The senders waiting to be screened, a page at a time. Each one carries the petitioner and
    /// the most recent entry they sent, so a caller can show who is asking and what they wrote
    /// without a second read.
    public func pending(page: String? = nil) async throws -> ClearanceSummary {
        try await pendingPage(page: page).value
    }

    /// The same queue as ``pending(page:)``, keeping the cursor for the page after it so a caller
    /// walking the queue is told when it has reached the end.
    public func pendingPage(page: String? = nil) async throws -> Page<ClearanceSummary> {
        try await get(options: GetClearancesOptions(includeClearances: true, page: page))
    }

    /// Answers the Screener for one sender.
    public func screen(clearanceId: Int, status: ClearanceStatus, options: ScreenOptions = ScreenOptions()) async throws -> Clearance {
        try await update(
            clearanceId: clearanceId,
            body: UpdateClearanceRequestContent(
                status: status.wire,
                designationBoxId: options.designationBoxId,
                spam: truthy(options.spam),
                markTopicsAsSeen: truthy(options.markTopicsAsSeen)))
    }

    /// Screens several senders at once and answers the clearances it changed. HEY answers 404
    /// when none of the ids belong to the caller. A partial match succeeds and answers only what
    /// it touched, so compare the answer against what was sent.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for an empty selection, before anything is sent.
    public func screenMany(clearanceIds: [Int], status: ClearanceStatus, spam: Bool = false) async throws -> [Clearance] {
        if clearanceIds.isEmpty { throw HeyError.usage(message: "at least one clearance is required") }
        let body = BulkUpdateClearancesRequestContent(
            ids: clearanceIds.map(String.init).joined(separator: ","), status: status.wire, spam: truthy(spam))
        return try await bulkUpdate(body: body).clearances ?? []
    }

    /// The senders already screened in or out, newest decision first, a page at a time.
    public func screened(page: String? = nil) async throws -> [Clearance] {
        try await screenedPage(page: page).value.clearances ?? []
    }

    /// The same decisions as ``screened(page:)``, keeping the cursor for the page after it.
    public func screenedPage(page: String? = nil) async throws -> Page<ClearanceListResponse> {
        try await getMy(options: GetMyClearancesOptions(page: page))
    }

    /// Changes its mind about a sender already screened in or out. This is the decided list, not
    /// the queue: ``screen(clearanceId:status:options:)`` is what answers a pending sender.
    public func rescreen(clearanceId: Int, status: ClearanceStatus) async throws -> Clearance {
        try await updateMy(clearanceId: clearanceId, body: UpdateMyClearanceRequestContent(status: status.wire))
    }

    /// A flag HEY reads for truthiness goes out only when it is on.
    private func truthy(_ value: Bool) -> Bool? {
        value ? true : nil
    }
}
