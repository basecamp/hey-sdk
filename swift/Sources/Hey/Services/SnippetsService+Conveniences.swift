import Foundation

/// The writes on top of the generated surface (`list`). Snippets have no JSON surface for
/// writes — every one of them redirects — so they are browser form posts.
extension SnippetsService {
    /// Saves a snippet.
    public func create(name: String, content: String) async throws {
        var operation = client.form(.post, "/snippets")
        operation.info = writeInfo(service: "Snippets", operation: "CreateSnippet", resourceType: "snippet")
        operation.form(snippetFields(name: name, content: content))
        try await client.sendVoid(operation)
    }

    /// Edits a snippet. An empty field is left out of the form, and so left as it was.
    public func update(snippetId: Int, name: String, content: String) async throws {
        var operation = client.form(.patch, "/snippets/\(snippetId)")
        operation.info = writeInfo(service: "Snippets", operation: "UpdateSnippet", resourceType: "snippet", resourceId: snippetId)
        operation.form(snippetFields(name: name, content: content))
        try await client.sendVoid(operation)
    }

    /// Throws a snippet away.
    public func delete(snippetId: Int) async throws {
        var operation = client.form(.delete, "/snippets/\(snippetId)")
        operation.info = writeInfo(service: "Snippets", operation: "DeleteSnippet", resourceType: "snippet", resourceId: snippetId)
        try await client.sendVoid(operation)
    }
}

private func snippetFields(name: String, content: String) -> [(String, String)] {
    var fields: [(String, String)] = []
    if !name.isEmpty { fields.append(("snippet[name]", name)) }
    if !content.isEmpty { fields.append(("snippet[content]", content)) }
    return fields
}
