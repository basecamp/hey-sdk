import Foundation

/// Where a published message lands, and what the token naming the post follows.
private let postPath = "/world/posts/"

/// The part a subscriber import is read from.
private let importPart = "world_list_import[source]"

/// The file name an import falls back to when the caller names none.
private let defaultImportFilename = "subscribers.csv"

/// HEY World — the blog you write by sending an email. None of it is JSON: a post is created by
/// emailing ``worldAddress``, an edit answers a redirect, and the subscriber list is a CSV
/// stream. There is no generated counterpart, so this is the whole of the service.
public final class WorldService: BaseService, @unchecked Sendable {
    /// The recipient that turns a message into a HEY World post.
    public static let worldAddress = "world@hey.com"

    /// Writes a HEY World post by sending a message to ``worldAddress``, and answers the post's
    /// token — the handle ``updatePost(token:subject:content:)`` and ``deletePost(token:)`` take.
    /// The message goes out either way, so a landing that is not a post is reported as what it is
    /// rather than as a failure to send.
    public func publish(subject: String, content: String) async throws -> String {
        let senderId = try await client.defaultSenderId()
        var operation = client.form(.post, "/messages")
        operation.info = writeInfo(service: "World", operation: "PublishWorldPost", resourceType: "world_post")
        operation.form([
            ("acting_sender_id", String(senderId)),
            ("message[subject]", subject),
            ("message[content]", content),
            ("entry[addressed][directly]", Self.worldAddress),
            ("entry[status]", "active"),
        ])
        // Where HEY sent the caller is read inside the operation, so a message that became
        // something other than a post ends the operation the hooks hear with that failure.
        return try await client.execute(operation) { response in
            let location = FormResponse.of(response).location ?? ""
            guard let token = postToken(location) else {
                throw HeyError.api(
                    message: "the message was sent but did not become a HEY World post (landed on \"\(redactLocation(location))\")",
                    httpStatus: nil, retryable: false, detail: ErrorDetail())
            }
            return token
        }
    }

    /// Edits a published post. An empty subject or body is left off the wire, and HEY leaves what
    /// a request does not name alone.
    public func updatePost(token: String, subject: String, content: String) async throws {
        var fields: [(String, String)] = []
        if !subject.isEmpty { fields.append(("world_post[subject]", subject)) }
        if !content.isEmpty { fields.append(("world_post[content]", content)) }
        var operation = client.form(.patch, worldPostPath(token))
        operation.info = writeInfo(service: "World", operation: "UpdateWorldPost", resourceType: "world_post")
        operation.form(fields)
        try await client.sendVoid(operation)
    }

    /// Takes a post off HEY World.
    public func deletePost(token: String) async throws {
        var operation = client.form(.delete, worldPostPath(token))
        operation.info = writeInfo(service: "World", operation: "DeleteWorldPost", resourceType: "world_post")
        try await client.sendVoid(operation)
    }

    /// The confirmed subscribers of a list as CSV, with the columns `email_address` and
    /// `subscribed_at`. The list is named by its author's email address, which goes into the path
    /// escaped as every modelled route escapes a parameter, so the `@` goes out as `%40`.
    public func exportSubscribers(listEmailAddress: String) async throws -> Data {
        var operation = client.request(.get, "/world/lists/\(percentEncodeComponent(listEmailAddress))/export.csv")
        operation.info = OperationInfo(service: "World", operation: "ExportWorldSubscribers", resourceType: "world_list", isMutation: false)
        operation.withoutJSONSuffix()
        operation.accept = "text/csv"
        return try await client.execute(operation) { $0.body }
    }

    /// Uploads a CSV of subscribers to a list. A blank `filename` becomes `subscribers.csv`, and
    /// one that does not already end in `.csv` gets it added: HEY reads the import by its
    /// extension.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` before anything is sent when `filename`
    ///   carries a line break.
    public func importSubscribers(listEmailAddress: String, filename: String, csv: Data) async throws {
        let (contentType, body) = try subscriberImportBody(filename: filename, csv: csv)
        var operation = client.form(.post, "/world/lists/\(percentEncodeComponent(listEmailAddress))/imports")
        operation.info = writeInfo(service: "World", operation: "ImportWorldSubscribers", resourceType: "world_list")
        operation.bodyBytes(contentType: contentType, body)
        try await client.sendVoid(operation)
    }
}

extension HeyClient {
    /// The HEY World service: publishing posts and keeping a list's subscribers.
    public var world: WorldService { WorldService(client: self) }
}

/// The token out of the location a publish redirected to, as Go's `/world/posts/([0-9a-f]+)`
/// reads it: the first `/world/posts/` followed by at least one hex digit, and the run of hex
/// digits after it. A message that landed anywhere else names no token.
func postToken(_ location: String) -> String? {
    var searchFrom = location.startIndex
    while let found = location.range(of: postPath, range: searchFrom..<location.endIndex) {
        let token = location[found.upperBound...].prefix { character in
            character.asciiValue.map { (0x30...0x39).contains($0) || (0x61...0x66).contains($0) } ?? false
        }
        if !token.isEmpty { return String(token) }
        searchFrom = location.index(after: found.lowerBound)
    }
    return nil
}

private func worldPostPath(_ token: String) -> String { postPath + percentEncodeComponent(token) }

/// The CSV wrapped in the multipart form the import endpoint expects, and the content type naming
/// the boundary it was built with.
func subscriberImportBody(filename: String, csv: Data) throws -> (contentType: String, body: Data) {
    // The filename goes into a header line of the multipart body as written; a line break in it
    // would end that line and start another part's headers.
    if filename.unicodeScalars.contains(where: { $0 == "\r" || $0 == "\n" }) {
        throw HeyError.usage(message: "a subscriber import filename cannot contain a line break")
    }
    let boundary = (0..<16).map { _ in String(format: "%02x", UInt8.random(in: .min ... .max)) }.joined()
    let head = "--\(boundary)\r\nContent-Disposition: form-data; name=\"\(importPart)\"; filename=\"\(escapeQuotes(importFilename(filename)))\"\r\nContent-Type: application/octet-stream\r\n\r\n"
    let tail = "\r\n--\(boundary)--\r\n"
    var body = Data(head.utf8)
    body.append(csv)
    body.append(Data(tail.utf8))
    return ("multipart/form-data; boundary=\(boundary)", body)
}

/// The same case-sensitive suffix check Go makes, so every SDK sends the same filename.
func importFilename(_ filename: String) -> String {
    if filename.isEmpty { return defaultImportFilename }
    return filename.hasSuffix(".csv") ? filename : "\(filename).csv"
}

/// A quote or a backslash would end the header field early, so both are escaped the way Go's
/// `mime/multipart` escapes them.
private func escapeQuotes(_ text: String) -> String {
    text.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"")
}
