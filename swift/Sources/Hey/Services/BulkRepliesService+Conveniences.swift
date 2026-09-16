import Foundation

/// One reply to many threads, and calling a delayed one back, on top of the generated surface
/// (`newBulkReply`, `create`).
extension BulkRepliesService {
    /// Works out which entries a bulk reply would answer, and how it starts. HEY replies to the
    /// last replyable entry of each thread and skips threads it has no reply address for, so the
    /// postings you hold are not the entries the reply goes to. Send the entries this answers — or
    /// a subset of them — to ``send(entryIds:content:)``. The generated
    /// ``newBulkReply(postingIds:)`` takes the ids already comma-joined.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` before anything is sent when `postingIds` is
    ///   empty.
    public func draft(postingIds: [Int]) async throws -> NewBulkReplyResponseContent {
        guard !postingIds.isEmpty else { throw HeyError.usage(message: "at least one posting is required") }
        return try await newBulkReply(postingIds: postingIds.map(String.init).joined(separator: ","))
    }

    /// Replies to every entry with the same content, and answers what was sent. Delivery is
    /// queued: while the sender has undo enabled the send is held open, and the answer says so —
    /// `delayed` is true and `undoSendUrl` is where to call it back with ``undo(bulkReplyId:)``.
    /// The generated ``create(body:)`` takes the same request as a body.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` before anything is sent when `entryIds` is
    ///   empty.
    public func send(entryIds: [Int], content: String) async throws -> CreateBulkReplyResponseContent {
        guard !entryIds.isEmpty else { throw HeyError.usage(message: "at least one entry is required") }
        return try await create(body: BulkReplyRequestContent(entryIds: entryIds, message: BulkReplyMessagePayload(content: content)))
    }

    /// Calls a delayed bulk reply back before it goes out. HEY answers this one with a redirect
    /// rather than JSON — the same answer its own apps read. Once the replies have gone out there
    /// is nothing left to call back, and HEY refuses; the SDK does not check the delivery first.
    public func undo(bulkReplyId: Int) async throws {
        var operation = client.form(.post, "/bulk_replies/\(bulkReplyId)/undo_send")
        operation.info = writeInfo(service: "BulkReplies", operation: "UndoBulkReplySend", resourceType: "bulk_reply", resourceId: bulkReplyId)
        operation.form([])
        try await client.sendVoid(operation)
    }
}

/// The bulk reply id in a delivery's `undo_send_url`, for a caller holding the URL rather than the
/// delivery. Only that URL is read: anything else the caller might be holding — the bulk reply
/// itself, some other action on it — is refused rather than guessed at.
///
/// - Throws: ``HeyError/usage(message:hint:)`` when the URL is not a bulk reply's undo URL.
public func undoSendId(_ undoSendUrl: String) throws -> Int {
    let path: String
    if let absolute = parseAbsoluteURL(undoSendUrl) {
        path = URLComponents(url: absolute, resolvingAgainstBaseURL: false)?.percentEncodedPath ?? absolute.path
    } else {
        path = String(undoSendUrl.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)[0]
            .split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false)[0])
    }
    var trimmed = Substring(path)
    while trimmed.hasPrefix("/") { trimmed.removeFirst() }
    while trimmed.hasSuffix("/") { trimmed.removeLast() }
    let segments = trimmed.split(separator: "/", omittingEmptySubsequences: false)
    guard segments.count == 3, segments[0] == "bulk_replies", segments[2] == "undo_send", let id = Int(segments[1]) else {
        throw HeyError.usage(message: "not a bulk reply undo URL: \(undoSendUrl)")
    }
    return id
}
