import Foundation

/// A message to deliver: the subject, the Trix HTML body and the recipients per kind.
public struct MessageContent: Sendable, Equatable {
    /// The subject line.
    public var subject: String
    /// The body, as Trix HTML.
    public var content: String
    /// The addresses the message goes to.
    public var to: [String]
    /// The addresses copied on it.
    public var cc: [String]
    /// The addresses copied on it without the others seeing.
    public var bcc: [String]
    /// The identity the message goes out as. Nil resolves the client's default sender.
    public var actingSenderId: Int?

    /// A message from its parts.
    public init(
        subject: String = "", content: String = "", to: [String] = [], cc: [String] = [], bcc: [String] = [],
        actingSenderId: Int? = nil
    ) {
        self.subject = subject
        self.content = content
        self.to = to
        self.cc = cc
        self.bcc = bcc
        self.actingSenderId = actingSenderId
    }
}

/// When a draft goes out, read in the identity's time zone. HEY schedules to the hour.
public struct DeliverySchedule: Sendable, Equatable {
    /// `YYYY-MM-DD`, "today" or "tomorrow".
    public var date: String
    /// 0 through 23.
    public var hour: Int

    /// A schedule from its parts.
    public init(date: String, hour: Int) {
        self.date = date
        self.hour = hour
    }
}

/// The whole of what a draft carries. HEY revises a draft from the whole of it, so a caller edits
/// by reading the draft, changing fields and sending everything back: empty recipients remove
/// them, and a nil schedule clears one already set.
public struct DraftContent: Sendable, Equatable {
    /// The subject line.
    public var subject: String
    /// The body, as Trix HTML.
    public var content: String
    /// The addresses the draft goes to.
    public var to: [String]
    /// The addresses copied on it.
    public var cc: [String]
    /// The addresses copied on it without the others seeing.
    public var bcc: [String]
    /// The identity the draft is saved — and ultimately delivered — as. Nil resolves the client's
    /// default sender. A caller who chose an alternate identity carries it on every revision, since
    /// a revision that leaves it out hands the draft back to the default one.
    public var actingSenderId: Int?
    /// When HEY delivers the draft on its own. Nil is a draft that waits to be sent.
    public var schedule: DeliverySchedule?

    /// A draft from its parts.
    public init(
        subject: String = "", content: String = "", to: [String] = [], cc: [String] = [], bcc: [String] = [],
        actingSenderId: Int? = nil, schedule: DeliverySchedule? = nil
    ) {
        self.subject = subject
        self.content = content
        self.to = to
        self.cc = cc
        self.bcc = bcc
        self.actingSenderId = actingSenderId
        self.schedule = schedule
    }
}

/// Delivery and draft conveniences on top of the generated surface (`create`, `update`, `get`,
/// `getEdit`).
extension MessagesService {
    /// Delivers a new message through HEY's undo-delay window. Delivery needs somebody to deliver
    /// to, so at least one recipient is required.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a message addressed to nobody, before
    ///   anything is sent.
    public func send(_ message: MessageContent) async throws {
        guard messageHasRecipients(message.to, message.cc, message.bcc) else {
            throw HeyError.usage(message: "a message needs at least one recipient (to, cc or bcc)")
        }
        let body = CreateMessageRequestContent(
            actingSenderId: try await senderId(message.actingSenderId),
            message: MessagePayload(subject: message.subject, content: message.content),
            entry: deliveredMessageEntry(message.to, message.cc, message.bcc))
        var operation = try client.operation(Routes.createMessage, [])
        try operation.json(body)
        try await client.sendVoid(operation)
    }

    /// Saves a new message as a draft instead of delivering it, and answers the draft's entry id —
    /// the id ``getEdit(messageId:)``, ``updateDraft(entryId:draft:)``,
    /// ``sendDraft(entryId:draft:)`` and ``EntriesService/deleteDraft(entryId:)`` take. A draft
    /// needs no recipients.
    ///
    /// - Throws: ``HeyError/api(message:httpStatus:retryable:detail:)`` when the save names no
    ///   entry id in its `Location`, which the hooks hear as the operation's end.
    public func createDraft(_ draft: DraftContent) async throws -> Int {
        var operation = try client.operation(Routes.createMessage, [])
        try operation.json(try await draftedRequest(draft))
        return try await client.execute(operation) { try draftEntryIdFromLocation($0) }
    }

    /// Revises a draft in place from the whole of `draft`. A trashed draft is silently restored by
    /// the revision.
    public func updateDraft(entryId: Int, draft: DraftContent) async throws {
        var operation = try client.operation(Routes.updateMessage, [entryId])
        operation.resourceId(entryId)
        try operation.json(try await draftedRequest(draft))
        try await client.sendVoid(operation)
    }

    /// Delivers a draft through HEY's undo-delay window. The revision and the delivery are one
    /// request, so the draft's final state rides along. It is never retried, despite the PUT: it
    /// triggers a delivery, and a resend after an ambiguous first attempt could send twice.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` for a draft addressed to nobody, before anything
    ///   is sent.
    public func sendDraft(entryId: Int, draft: DraftContent) async throws {
        guard messageHasRecipients(draft.to, draft.cc, draft.bcc) else {
            throw HeyError.usage(message: "sending a draft needs at least one recipient (to, cc or bcc)")
        }
        let body = CreateMessageRequestContent(
            actingSenderId: try await senderId(draft.actingSenderId),
            message: MessagePayload(subject: draft.subject, content: draft.content),
            entry: deliveredMessageEntry(draft.to, draft.cc, draft.bcc))
        var operation = try client.operation(Routes.updateMessage, [entryId])
        operation.resourceId(entryId)
        try operation.json(body)
        operation.idempotent = false
        try await client.sendVoid(operation)
    }

    private func senderId(_ chosen: Int?) async throws -> Int {
        if let chosen { return chosen }
        return try await client.defaultSenderId()
    }

    private func draftedRequest(_ draft: DraftContent) async throws -> CreateMessageRequestContent {
        var entry = draftedMessageEntry(draft.to, draft.cc, draft.bcc)
        if let schedule = draft.schedule {
            entry.scheduledDelivery = "true"
            entry.scheduledDeliveryAtDate = schedule.date
            entry.scheduledDeliveryAtHour = String(schedule.hour)
        }
        return CreateMessageRequestContent(
            actingSenderId: try await senderId(draft.actingSenderId),
            message: MessagePayload(subject: draft.subject, content: draft.content),
            entry: entry)
    }
}

/// What `entry.status` carries to keep an entry a draft. Any other value, or omitting it, has HEY
/// deliver it.
private let draftedStatus = "drafted"

func messageHasRecipients(_ to: [String], _ cc: [String], _ bcc: [String]) -> Bool {
    !to.isEmpty || !cc.isEmpty || !bcc.isEmpty
}

/// The entry of a message HEY delivers, whose recipient kinds are the ones that name somebody.
func deliveredMessageEntry(_ to: [String], _ cc: [String], _ bcc: [String]) -> MessageEntryPayload {
    MessageEntryPayload(
        addressed: MessageAddressed(
            directly: to.isEmpty ? nil : to,
            copied: cc.isEmpty ? nil : cc,
            blindcopied: bcc.isEmpty ? nil : bcc))
}

/// The entry of a message HEY keeps as a draft. Every recipient kind is present, empty ones
/// included: a draft addressed to nobody yet is the normal case, and on a revision an empty list is
/// how recipients are removed.
func draftedMessageEntry(_ to: [String], _ cc: [String], _ bcc: [String]) -> MessageEntryPayload {
    MessageEntryPayload(addressed: MessageAddressed(directly: to, copied: cc, blindcopied: bcc), status: draftedStatus)
}

/// The entry id out of the `Location` a draft save answers with: the save serves no body, so the
/// header is the only place the id is named.
func draftEntryIdFromLocation(_ response: Response) throws -> Int {
    guard let location = response.header("Location") else {
        throw HeyError.api(
            message: "draft saved but the response named no Location; cannot report the draft's id", httpStatus: response.status,
            retryable: false, detail: ErrorDetail())
    }
    guard let target = resolveReference(response.url, location),
          let path = URLComponents(url: target, resolvingAgainstBaseURL: true)?.percentEncodedPath
    else {
        throw HeyError.api(
            message: "draft saved but its Location \"\(redactLocation(location))\" is unreadable", httpStatus: response.status,
            retryable: false, detail: ErrorDetail())
    }
    var trimmed = Substring(path)
    while trimmed.hasSuffix("/") { trimmed.removeLast() }
    let last = trimmed.split(separator: "/", omittingEmptySubsequences: false).last ?? ""
    guard let entryId = Int(last), entryId > 0 else {
        throw HeyError.api(
            message: "draft saved but its Location \"\(redactLocation(location))\" names no entry id", httpStatus: response.status,
            retryable: false, detail: ErrorDetail())
    }
    return entryId
}
