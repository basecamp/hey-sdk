/// The kinds of box a HEY account has, as ``BoxesService/list()`` reports them.
public enum BoxKind: String, Sendable, Equatable, CaseIterable {
    /// The Imbox, where screened-in mail lands.
    case imbox = "imbox"
    /// The Feed, for newsletters and the like.
    case feed = "feedbox"
    /// Set Aside, for threads kept close to hand.
    case setAside = "asidebox"
    /// Reply Later, for threads waiting on an answer.
    case replyLater = "laterbox"
    /// The Paper Trail, for receipts and confirmations.
    case paperTrail = "trailbox"
    /// Bubble Up, holding postings until the day they resurface.
    case bubbleUp = "bubblebox"

    /// The kind as the box index names it — the `kind` a listed box carries.
    public var wire: String { rawValue }

    /// The kind a box index names.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a kind that is none of them.
    public static func parse(_ source: String) throws -> BoxKind {
        guard let kind = BoxKind(rawValue: source) else {
            throw HeyError.usage(message: "box kind \"\(source)\" is none of imbox, feedbox, asidebox, laterbox, trailbox, bubblebox")
        }
        return kind
    }
}

/// Kind resolution on top of the generated surface (`list`, `get`, `getImbox`, ...).
extension BoxesService {
    /// The id of the box of a kind. The client reads the box index once and answers every kind
    /// from that reading for as long as it lives; a client derived with
    /// ``HeyClient/forAccount(_:)`` reads it again for the account it presents. Use ``kinds()``
    /// to read the index afresh.
    ///
    /// - Throws: ``HeyError/api(message:httpStatus:retryable:detail:)`` when the account has no
    ///   box of that kind.
    public func idByKind(_ kind: BoxKind) async throws -> Int {
        let scope = client.scope
        return try await scope.lock.withLock {
            let kinds: [String: Int]
            if let known = scope.boxKinds {
                kinds = known
            } else {
                kinds = try await self.kinds()
                scope.boxKinds = kinds
            }
            guard let id = kinds[kind.wire] else {
                throw HeyError.api(message: "no box of kind \"\(kind.wire)\"", httpStatus: nil, retryable: false, detail: ErrorDetail())
            }
            return id
        }
    }

    /// Reads the box index and maps every box's kind to its id. This is the read itself, so it
    /// goes to HEY every time.
    public func kinds() async throws -> [String: Int] {
        var kinds: [String: Int] = [:]
        for box in try await list().value where !box.kind.isEmpty {
            kinds[box.kind] = box.id
        }
        return kinds
    }

    /// Gathers a selection of postings into a new Set Aside group. The generated
    /// ``createGroup(boxId:body:)`` takes the same request as a body.
    public func createBoxGroup(boxId: Int, postingIds: [Int]) async throws -> CreateBoxGroupResponseContent {
        try await createGroup(boxId: boxId, body: CreateBoxGroupRequestContent(postingIds: postingIds))
    }
}
