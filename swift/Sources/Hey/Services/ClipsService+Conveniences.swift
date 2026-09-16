import Foundation

/// The writes on top of the generated surface (`list`). Clips have no JSON surface for writes —
/// a write answers with a Turbo Stream rather than a record — so both of them are browser form
/// posts.
extension ClipsService {
    /// Clips a piece of an entry, so it can be found again without the thread.
    public func create(entryId: Int, content: String) async throws {
        var operation = client.form(.post, "/clips")
        operation.info = writeInfo(service: "Clips", operation: "CreateClip", resourceType: "clip", resourceId: entryId)
        operation.form([("clip[entry_id]", String(entryId)), ("clip[content]", content)])
        try await client.sendVoid(operation)
    }

    /// Throws a clip away.
    public func delete(clipId: Int) async throws {
        var operation = client.form(.delete, "/clips/\(clipId)")
        operation.info = writeInfo(service: "Clips", operation: "DeleteClip", resourceType: "clip", resourceId: clipId)
        try await client.sendVoid(operation)
    }
}
