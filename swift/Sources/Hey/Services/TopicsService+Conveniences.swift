import Foundation

/// The moves and the confirmed trashing on top of the generated surface (`get`, `getEntries`,
/// `trash`, `restore`, `markHam`, ...). ``getEntries(topicId:options:)`` is paged by geared
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

    /// Whether HEY answered by sending the caller to the topic's removal confirmation page, which
    /// is how it says it will not trash a shared topic unasked. Any other redirect is HEY sending
    /// the caller back to where the topic was, with the trashing done.
    private static func awaitingConfirmation(_ response: Response, _ topicId: Int) -> Bool {
        guard let location = response.header("Location"), let target = resolveReference(response.url, location) else { return false }
        return URLComponents(url: target, resolvingAgainstBaseURL: true)?.percentEncodedPath == "/topics/\(topicId)/removal/new"
    }
}
