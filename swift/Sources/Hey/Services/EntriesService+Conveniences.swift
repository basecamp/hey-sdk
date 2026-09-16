/// A reply, as the reply prefill (``EntriesService/newReply(entryId:)``) hands its parts over.
public struct ReplyContent: Sendable, Equatable {
    /// The identity the reply goes out as. HEY resolves a reply's sender from the thread and hands
    /// it back as the prefill's sender; pass that id. Zero falls back to the client's default
    /// sender, which on a shared address is the wrong identity.
    public var actingSenderId: Int
    /// HEY derives no subject for a reply, so pass the prefill's "Re: …". Empty leaves it off the
    /// wire.
    public var subject: String
    /// The reply body alone: HEY appends the quoted original at delivery.
    public var content: String
    /// The addresses the reply goes to.
    public var to: [String]
    /// The addresses copied on it.
    public var cc: [String]
    /// The addresses copied on it without the others seeing.
    public var bcc: [String]

    /// A reply from its parts.
    public init(actingSenderId: Int = 0, subject: String = "", content: String = "", to: [String] = [], cc: [String] = [], bcc: [String] = []) {
        self.actingSenderId = actingSenderId
        self.subject = subject
        self.content = content
        self.to = to
        self.cc = cc
        self.bcc = bcc
    }
}

/// Reply conveniences on top of the generated surface (`createReply`, `newReply`, `deleteDraft`,
/// ...).
extension EntriesService {
    /// Delivers a reply to an entry. HEY does not reply-all on the caller's behalf, and saves an
    /// unaddressed reply as a draft rather than delivering it, so the recipients are required.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a reply addressed to nobody, before anything
    ///   is sent.
    public func reply(entryId: Int, reply: ReplyContent) async throws {
        guard messageHasRecipients(reply.to, reply.cc, reply.bcc) else {
            throw HeyError.usage(message: "a reply needs at least one recipient (to, cc or bcc); HEY saves an unaddressed reply as a draft")
        }
        let body = CreateReplyRequestContent(
            actingSenderId: try await senderFor(reply),
            message: replyPayload(reply),
            entry: deliveredMessageEntry(reply.to, reply.cc, reply.bcc))
        var operation = try client.operation(Routes.createReply, [entryId])
        operation.resourceId(entryId)
        try operation.json(body)
        try await client.sendVoid(operation)
    }

    /// Saves a reply as a draft instead of delivering it, and answers the draft's entry id. It
    /// needs no recipients.
    public func replyDraft(entryId: Int, reply: ReplyContent) async throws -> Int {
        let body = CreateReplyRequestContent(
            actingSenderId: try await senderFor(reply),
            message: replyPayload(reply),
            entry: draftedMessageEntry(reply.to, reply.cc, reply.bcc))
        var operation = try client.operation(Routes.createReply, [entryId])
        operation.resourceId(entryId)
        try operation.json(body)
        return try await client.execute(operation) { try draftEntryIdFromLocation($0) }
    }

    private func senderFor(_ reply: ReplyContent) async throws -> Int {
        reply.actingSenderId == 0 ? try await client.defaultSenderId() : reply.actingSenderId
    }

    private func replyPayload(_ reply: ReplyContent) -> ReplyMessagePayload {
        ReplyMessagePayload(content: reply.content, subject: reply.subject.isEmpty ? nil : reply.subject)
    }
}
