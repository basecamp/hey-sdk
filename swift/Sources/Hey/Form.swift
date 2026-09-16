import Foundation

/// The answer to a form request. A redirect is captured rather than followed, so a 302 or 303
/// arrives here with its `Location` intact; an endpoint reached on a `.json` path answers the
/// record itself, which lands in ``body`` instead.
public struct FormResponse: Sendable, Equatable {
    /// Where the redirect pointed, exactly as HEY wrote it — often a path rather than a whole URL.
    public let location: String?
    /// The status the endpoint answered: a 302 or 303 for a redirect, a 200 for a document.
    public let status: Int
    /// What the endpoint answered when it answered a document instead of a redirect.
    public let body: String

    /// The id of the record the redirect named: the rightmost path segment that reads as a
    /// number, so `/calendar/events/42` and `/calendar/events/42/edit` both answer 42.
    public func extractId() throws -> Int {
        guard let target = location, !target.isEmpty else {
            throw HeyError.api(message: "no location header in response", httpStatus: nil, retryable: false, detail: ErrorDetail())
        }
        let path: String
        if target.contains("://") {
            guard let parsed = URLComponents(string: target) else {
                throw HeyError.api(
                    message: "failed to parse location URL: \(redactLocation(target))", httpStatus: nil, retryable: false,
                    detail: ErrorDetail())
            }
            path = parsed.percentEncodedPath
        } else {
            path = redactLocation(target)
        }
        var trimmed = path
        while trimmed.hasSuffix("/") { trimmed.removeLast() }
        for segment in trimmed.split(separator: "/").reversed() {
            if let id = Int(segment) { return id }
        }
        throw HeyError.api(
            message: "no numeric ID found in location: \(redactLocation(target))", httpStatus: nil, retryable: false,
            detail: ErrorDetail())
    }

    /// The client hands over a 302 or 303 with its `Location`, or the document a `.json` path
    /// answered; any other redirect failed before it got here.
    static func of(_ response: Response) -> FormResponse {
        if response.empty, (300...399).contains(response.status) {
            return FormResponse(location: response.header("Location"), status: response.status, body: "")
        }
        return FormResponse(location: nil, status: response.status, body: response.text())
    }
}

/// What a write to a path outside the model announces itself as.
public func writeInfo(service: String, operation: String, resourceType: String, resourceId: Int? = nil) -> OperationInfo {
    OperationInfo(service: service, operation: operation, resourceType: resourceType, isMutation: true, resourceId: resourceId)
}
