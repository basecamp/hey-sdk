import Foundation

/// Turning a thread into a public web page, on top of the generated `get`. Publishing has no JSON
/// surface — both writes are browser form posts that redirect, and the public link only appears
/// once the publication is read back.
extension PublicationsService {
    /// Publishes a thread and answers its public link. The redirect lands on the sharing panel
    /// rather than carrying the link, so the publication is read back. Both requests go quiet
    /// inside one operation, so the hooks hear `Publications.CreateTopicPublication` once, see both
    /// requests under it, as they do in Go, and hear it end only once the link is in hand — with
    /// the failure, when the read-back fails.
    ///
    /// HEY answers a forbidden error on accounts that are not eligible to publish.
    public func publish(topicId: Int) async throws -> TopicPublication {
        let info = writeInfo(service: "Publications", operation: "CreateTopicPublication", resourceType: "publication", resourceId: topicId)
        return try await client.asOperation(info) {
            var operation = client.form(.post, "/topics/\(topicId)/publication")
            operation.form([])
            operation.quiet()
            try await client.sendVoid(operation)
            var read = try client.operation(Routes.getTopicPublication, [topicId])
            read.resourceId(topicId)
            read.quiet()
            return try await client.send(read, as: TopicPublication.self)
        }
    }

    /// Unpublishes a thread, breaking its public link.
    public func unpublish(topicId: Int) async throws {
        var operation = client.form(.delete, "/topics/\(topicId)/publication")
        operation.info = writeInfo(service: "Publications", operation: "DeleteTopicPublication", resourceType: "publication", resourceId: topicId)
        try await client.sendVoid(operation)
    }
}
