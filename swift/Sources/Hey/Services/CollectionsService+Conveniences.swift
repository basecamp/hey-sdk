import Foundation

/// What a new collection is made of.
public struct CreateCollectionParams: Sendable, Equatable {
    /// What the collection is called.
    public var name: String
    /// The blurb shown under the name.
    public var summary: String?
    /// The account that owns it. Nil leaves HEY to pick your first.
    public var accountId: Int?

    public init(name: String, summary: String? = nil, accountId: Int? = nil) {
        self.name = name
        self.summary = summary
        self.accountId = accountId
    }
}

/// What an edit changes about a collection. A field left unset — nil or empty — is left off the
/// wire, and HEY leaves what a request does not name alone.
public struct UpdateCollectionParams: Sendable, Equatable {
    /// A new name.
    public var name: String?
    /// A new blurb under the name.
    public var summary: String?

    public init(name: String? = nil, summary: String? = nil) {
        self.name = name
        self.summary = summary
    }
}

/// The writes on top of the generated surface (`get`, `list`, `update`). HEY serves no JSON
/// endpoint for making a collection or filing a thread into one, so each of those is a browser
/// form post.
extension CollectionsService {
    /// Makes a collection. The form post answers with a redirect to the collections index rather
    /// than to the collection it made, so the new collection's id does not come back; ``list()``
    /// afterwards is how to find it.
    public func create(params: CreateCollectionParams) async throws {
        var fields = [("collection[name]", params.name)]
        if let summary = params.summary, !summary.isEmpty { fields.append(("collection[summary]", summary)) }
        if let accountId = params.accountId { fields.append(("account_id", String(accountId))) }
        var operation = client.form(.post, "/collections")
        operation.info = writeInfo(service: "Collections", operation: "CreateCollection", resourceType: "collection")
        operation.form(fields)
        try await client.sendVoid(operation)
    }

    /// Renames a collection or changes its summary. The generated
    /// ``update(collectionId:body:)`` takes the same request as a body.
    public func updateCollection(collectionId: Int, params: UpdateCollectionParams) async throws {
        let body = UpdateCollectionRequestContent(
            collection: CollectionPayload(name: present(params.name), summary: present(params.summary)))
        try await update(collectionId: collectionId, body: body)
    }

    /// Files a topic into a collection.
    public func addTopic(topicId: Int, collectionId: Int) async throws {
        var operation = client.form(.post, "/topics/\(topicId)/collecting")
        operation.info = writeInfo(service: "Collections", operation: "CreateTopicCollecting", resourceType: "collecting", resourceId: topicId)
        operation.query("collection_id", collectionId)
        operation.form([])
        try await client.sendVoid(operation)
    }

    /// Takes a topic back out of a collection. A shadowed topic is silently left alone.
    public func removeTopic(topicId: Int, collectionId: Int) async throws {
        var operation = client.form(.delete, "/topics/\(topicId)/collecting")
        operation.info = writeInfo(service: "Collections", operation: "DeleteTopicCollecting", resourceType: "collecting", resourceId: topicId)
        operation.query("collection_id", collectionId)
        try await client.sendVoid(operation)
    }
}

/// An empty string is no value, and is left off the wire like a nil one — the omission is what
/// tells HEY to leave the field as it is.
private func present(_ value: String?) -> String? {
    guard let value, !value.isEmpty else { return nil }
    return value
}
