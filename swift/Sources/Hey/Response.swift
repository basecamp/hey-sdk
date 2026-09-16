import Foundation

/// What came back from HEY, before it is decoded.
public struct Response: Sendable {
    /// What HEY answered.
    public let status: Int
    /// The headers that came with it.
    public let headers: HTTPHeaders
    /// The body, read whole.
    public let body: Data
    /// Where the answer came from, once any redirects were followed.
    public let url: URL
    /// The body came out of the response cache: HEY answered 304 and the SDK read the entry it
    /// was holding.
    public let fromCache: Bool
    /// The operation takes this status for an answer rather than a failure: a 404 that means
    /// "nothing there", or the redirect a form request went out to collect.
    public let empty: Bool

    /// One header's value, when HEY sent it.
    public func header(_ name: String) -> String? { headers[name] }

    /// The body as text.
    public func text() -> String { String(decoding: body, as: UTF8.self) }

    /// Decodes the body as JSON. A body that will not decode is an error that still says what
    /// HEY answered: the status, and the request id when the answer named one.
    public func json<T: Decodable>(_ type: T.Type = T.self) throws -> T {
        guard !body.isEmpty else {
            throw HeyError.api(
                message: "empty response body", httpStatus: status, retryable: false,
                detail: ErrorDetail(requestId: header("X-Request-Id")))
        }
        do {
            return try JSONDecoder().decode(type, from: body)
        } catch let error as DecodingError {
            throw undecodable(decodeHint(error))
        } catch {
            throw undecodable("body does not decode")
        }
    }

    /// The error for a body that will not decode. It says where the body went wrong — the
    /// field, the path — and never what the body held: a body can carry an address or a name,
    /// so neither the decoder's own words nor the error that carried them are kept.
    private func undecodable(_ hint: String) -> HeyError {
        .api(
            message: "unexpected JSON in the response", httpStatus: status, retryable: false,
            detail: ErrorDetail(hint: hint, requestId: header("X-Request-Id")))
    }
}

/// What a decoding failure says about where it stopped, with the input left out: a required
/// field it names, and the path it reached.
func decodeHint(_ error: DecodingError) -> String {
    var parts: [String] = []
    let context: DecodingError.Context?
    switch error {
    case let .keyNotFound(key, keyContext):
        if isModelName(key.stringValue) { parts.append("missing required field '\(key.stringValue)'") }
        context = keyContext
    case let .valueNotFound(_, valueContext), let .typeMismatch(_, valueContext), let .dataCorrupted(valueContext):
        context = valueContext
    @unknown default:
        context = nil
    }
    if let path = context.map({ jsonPath($0.codingPath) }), path != "$" {
        parts.append("at path \(path)")
    }
    return parts.isEmpty ? "body does not decode" : "body does not decode: " + parts.joined(separator: ", ")
}

/// A coding path in the form `$.boxes[0].name`, with only names shaped like the model's: a map
/// key in a path is the body's, and is left out.
private func jsonPath(_ codingPath: [any CodingKey]) -> String {
    var path = "$"
    for key in codingPath {
        if let index = key.intValue {
            path += "[\(index)]"
        } else if isModelName(key.stringValue) {
            path += ".\(key.stringValue)"
        } else {
            break
        }
    }
    return path
}

private func isModelName(_ name: String) -> Bool {
    !name.isEmpty && name.unicodeScalars.allSatisfy { CharacterSet.alphanumerics.contains($0) || $0 == "_" }
        && name.unicodeScalars.allSatisfy(\.isASCII)
}
