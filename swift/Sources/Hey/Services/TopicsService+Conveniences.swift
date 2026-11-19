import Foundation

/// The moves, the confirmed trashing and the undo that answers its entry on top of the generated
/// surface (`get`, `getEntries`, `trash`, `restore`, `markHam`, ...). ``getEntries(topicId:options:)`` is paged by geared
/// pagination, so its `page` is a cursor out of the previous answer's `Link` header rather than an
/// offset: a number is ignored and answered with the first page.
extension TopicsService {
    /// Moves a topic to a box, by the box's id. The generated ``moveTopic(topicId:body:)`` takes
    /// the request body; this takes the one thing it carries.
    public func moveToBox(topicId: Int, boxId: Int) async throws {
        try await moveTopic(topicId: topicId, body: MoveTopicRequestContent(boxId: boxId))
    }

    /// Trashes a topic. HEY will not trash a shared topic without being asked twice: it answers
    /// the removal confirmation page instead, which comes back here as a usage error rather than
    /// as a trashing that quietly did nothing. Confirming trashes the topic and removes your
    /// access to it.
    ///
    /// The generated ``trash(topicId:options:)`` sends the same request from its parts, and
    /// follows that redirect rather than reading it.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when HEY wants the trashing confirmed.
    public func trashTopic(topicId: Int, confirmDestroy: Bool = false) async throws {
        var operation = try client.operation(Routes.trashTopic, [topicId])
        operation.resourceId(topicId)
        // An empty confirm_destroy reads as truthy on the server and skips the confirmation, so it
        // is sent only when it is asked for.
        if confirmDestroy { operation.query("confirm_destroy", 1) }
        operation.captureRedirects()
        // The answer is read inside the operation, so the hooks hear the refusal the caller gets.
        try await client.execute(operation) { response in
            if Self.awaitingConfirmation(response, topicId) {
                throw HeyError.usage(
                    message: "topic \(topicId) is shared; HEY wants confirmation before trashing it",
                    hint: "Call trashTopic with confirmDestroy: true to trash it and remove your access")
            }
        }
    }

    /// Calls back the last message the caller sent on a topic, while Undo Send still holds it, and
    /// answers the entry HEY called back: a draft again, unsent.
    ///
    /// HEY picks the message, the caller's last delayed entry on the topic, so compare the entry
    /// answered with the one meant: two messages sent to one thread within the delay are called
    /// back last first. A reply that broke out into a thread of its own is called back on that
    /// thread, the one the send's `topicId` names.
    ///
    /// HEY answers with a redirect to the draft rather than JSON, the answer its own apps read,
    /// and the redirect is the only place it names the entry: this takes it as the answer rather
    /// than follow it, and reads the entry with ``undoneEntryId(_:)``. The entry is nil when the
    /// redirect names none. It is never sent twice.
    ///
    /// The generated ``undoSend(topicId:)`` sends the same request and follows the redirect, which
    /// says the undo went through and not which entry it was.
    ///
    /// - Throws: ``HeyError/api(message:httpStatus:retryable:detail:)`` with `httpStatus` 400 once
    ///   the delay has run out and the message has gone, or when nothing was held; nothing was
    ///   changed.
    public func undoTopicSend(topicId: Int) async throws -> Int? {
        var operation = try client.operation(Routes.undoTopicSend, [topicId])
        operation.resourceId(topicId)
        operation.captureRedirects()
        // The refusal is reworded inside the operation, so the hooks hear the error the caller gets.
        operation.quiet()
        return try await client.asOperation(operation.info) {
            do {
                return Self.calledBack(try await client.execute(operation))
            } catch let error as HeyError where error.httpStatus == 400 {
                throw HeyError.api(
                    message: "nothing to undo: the message was already sent, or none was held",
                    httpStatus: 400,
                    retryable: false,
                    detail: ErrorDetail(requestId: error.requestId, body: error.body))
            }
        }
    }

    /// The entry the redirect an undo answered with names, when it names one.
    private static func calledBack(_ response: Response) -> Int? {
        guard (300...399).contains(response.status), let location = response.header("Location") else { return nil }
        return undoneEntryId(location)
    }

    /// Whether HEY answered by sending the caller to the topic's removal confirmation page, which
    /// is how it says it will not trash a shared topic unasked. Any other redirect is HEY sending
    /// the caller back to where the topic was, with the trashing done.
    private static func awaitingConfirmation(_ response: Response, _ topicId: Int) -> Bool {
        guard let location = response.header("Location"), let target = resolveReference(response.url, location) else { return false }
        return URLComponents(url: target, resolvingAgainstBaseURL: true)?.percentEncodedPath == "/topics/\(topicId)/removal/new"
    }
}

/// The entry HEY called back, read out of the redirect an undo send answers with, for a caller
/// holding that `Location`: `/messages/{id}/edit` for a message on a thread of its own,
/// `/topics/{topicId}?expanded_draft={id}` for a reply. Anything else names no entry and answers
/// nil: the thread alone, which is where HEY's pages send a browser with nothing to undo, is not
/// an undo.
public func undoneEntryId(_ location: String) -> Int? {
    // A Location is often a path rather than a whole URL, so it is read against a base that
    // stands for wherever it came from: only its path and query are looked at.
    guard let base = URL(string: "https://hey.invalid/"),
          let target = resolveReference(base, location),
          let components = URLComponents(url: target, resolvingAgainstBaseURL: true)
    else { return nil }
    var path = Substring(components.percentEncodedPath)
    while path.hasPrefix("/") { path.removeFirst() }
    while path.hasSuffix("/") { path.removeLast() }
    let segments = path.split(separator: "/", omittingEmptySubsequences: false)
    let named: String?
    if segments.count == 3, segments[0] == "messages", segments[2] == "edit" {
        named = String(segments[1])
    } else if segments.count == 2, segments[0] == "topics" {
        named = components.queryItems?.first { $0.name == "expanded_draft" }?.value
    } else {
        named = nil
    }
    guard let named, let id = Int(named), id > 0 else { return nil }
    return id
}
