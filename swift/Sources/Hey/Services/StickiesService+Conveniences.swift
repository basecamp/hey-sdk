/// How much room a sticky takes on the board.
public enum StickySize: String, Sendable, Equatable, CaseIterable {
    /// The smallest.
    case small = "small"
    /// The middle size.
    case medium = "medium"
    /// The largest.
    case large = "large"

    /// The size as HEY's `size` parameter names it.
    public var wire: String { rawValue }

    /// Reads a size as HEY writes it.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for anything else.
    public static func parse(_ source: String) throws -> StickySize {
        guard let size = StickySize(rawValue: source) else {
            throw HeyError.usage(message: "sticky size \"\(source)\" is none of \"small\", \"medium\" or \"large\"")
        }
        return size
    }
}

/// The stickies board, in the terms the board itself is kept in — a size, a limit and a position
/// — on top of the generated surface (`list`, `create`, `update`, `delete`, `moveSticky`).
extension StickiesService {
    /// The largest page the stickies index answers with. The server clamps anything above it, so
    /// ``listUpTo(limit:)`` clamps too rather than sending a number it knows is ignored.
    public static let maxStickiesLimit = 100

    /// The highest board position ``moveTo(stickyId:position:)`` accepts. The wire format carries
    /// the position as a 32-bit integer.
    public static let maxStickyPosition = Int(Int32.max)

    /// The stickies in board order, at most `limit` of them. Zero asks for the server default,
    /// which is also its maximum of ``maxStickiesLimit``; a limit of zero is left off the query
    /// entirely, since `limit=0` is clamped to a single sticky rather than read as "no limit".
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a negative limit.
    public func listUpTo(limit: Int) async throws -> [Sticky] {
        if limit < 0 { throw HeyError.usage(message: "sticky limit must be at least 0, got \(limit)") }
        let sent = limit > 0 ? Int32(min(limit, Self.maxStickiesLimit)) : nil
        return try await list(options: ListStickiesOptions(limit: sent))
    }

    /// Writes a new sticky. No size leaves the server default in place.
    public func createSticky(body: String, size: StickySize? = nil) async throws -> Sticky {
        try await create(body: stickyBody(body, size))
    }

    /// Edits a sticky. An empty body and no size are left alone.
    public func updateSticky(stickyId: Int, body: String, size: StickySize? = nil) async throws -> Sticky {
        try await update(stickyId: stickyId, body: stickyBody(body, size))
    }

    /// Repositions a sticky on the board. Positions run from zero to ``maxStickyPosition``.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a position outside that range, before
    ///   anything is sent.
    public func moveTo(stickyId: Int, position: Int) async throws {
        guard (0...Self.maxStickyPosition).contains(position) else {
            throw HeyError.usage(message: "sticky position must be between 0 and \(Self.maxStickyPosition), got \(position)")
        }
        try await moveSticky(body: MoveStickyRequestContent(id: stickyId, position: Int32(position)))
    }

    private func stickyBody(_ body: String, _ size: StickySize?) -> StickyRequestContent {
        StickyRequestContent(sticky: StickyPayload(body: body.isEmpty ? nil : body, size: size?.wire))
    }
}
