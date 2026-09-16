import Foundation

/// A body an operation carries, as it goes over the wire.
public struct Body: Sendable, Equatable, CustomStringConvertible {
    /// The `Content-Type` the body is sent as.
    public var contentType: String
    /// The bytes.
    public var bytes: Data

    public init(contentType: String, bytes: Data) {
        self.contentType = contentType
        self.bytes = bytes
    }

    public var description: String { "Body(\(contentType), \(bytes.count) bytes)" }
}

/// The `Accept` a browser sends, for an endpoint HEY serves only as a form.
let browserAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

/// A request the client has not sent yet. Generated service methods build one from a ``Route``
/// with ``HeyClient/operation(_:_:)``; ``HeyClient/request(_:_:)`` builds one for a path the
/// model does not cover, and ``HeyClient/form(_:_:)`` one for a browser form.
///
/// An operation prints what it is and where it goes, not what it carries: the query's names
/// without their values, the body's shape without its bytes.
public struct HeyOperation: Sendable, CustomStringConvertible {
    let id: String
    /// What the call means, as the hooks hear it.
    public var info: OperationInfo
    /// The modelled route this sends, whose retry policy the client honours. A path the caller
    /// wrote has none; a page after the first carries the first's.
    public internal(set) var route: Route?
    /// The HTTP method the operation is sent with.
    public let method: HTTPMethod
    /// The path the operation is sent to, parameters already filled in.
    public let path: String
    var url: URL?

    var query: [(String, String)] = []
    var headers: [(String, String)] = []
    var body: Body?
    /// Whether the client may resend the operation after a transient failure.
    public var idempotent: Bool
    var emptyOn: [Int]
    /// What the operation asks for in `Accept`.
    public var accept: String
    /// HEY answers JSON to paths that end in `.json`, so a modelled route gets one put back on.
    var appendsJSONSuffix: Bool
    var skipsCache = false
    var capturesRedirects = false
    var isQuiet = false
    var isUnsigned = false

    init(id: String, info: OperationInfo, route: Route?, method: HTTPMethod, path: String, url: URL?) {
        self.id = id
        self.info = info
        self.route = route
        self.method = method
        self.path = path
        self.url = url
        self.idempotent = route?.idempotent ?? [.get, .put, .delete].contains(method)
        self.emptyOn = route?.emptyOn ?? []
        self.accept = route?.html == true ? "text/html" : "application/json"
        self.appendsJSONSuffix = route?.html != true
    }

    /// Adds a query parameter. The same name may be added more than once.
    public mutating func query(_ name: String, _ value: any CustomStringConvertible & Sendable) {
        query.append((name, value.description))
    }

    /// Adds a query parameter when there is a value for it, and nothing otherwise.
    public mutating func queryOptional(_ name: String, _ value: (any CustomStringConvertible & Sendable)?) {
        if let value { query(name, value) }
    }

    /// A JSON body, already encoded. ``json(_:)`` encodes a model for you.
    public mutating func jsonBody(_ encoded: Data) {
        body = Body(contentType: "application/json", bytes: encoded)
    }

    /// Encodes a model as the operation's JSON body, which is what every modelled write sends.
    public mutating func json<T: Encodable>(_ value: T) throws {
        jsonBody(try heyJSONEncoder().encode(value))
    }

    /// A form-encoded body, as a browser would post it. A name may repeat, for a list.
    public mutating func form(_ fields: [(String, String)]) {
        let encoded = fields.map { "\(percentEncodeComponent($0.0))=\(percentEncodeComponent($0.1))" }.joined(separator: "&")
        body = Body(contentType: "application/x-www-form-urlencoded", bytes: Data(encoded.utf8))
    }

    /// A body of the caller's own, with its content type.
    public mutating func bodyBytes(contentType: String, _ bytes: Data) {
        body = Body(contentType: contentType, bytes: bytes)
    }

    /// Adds a header HEY handed the request — the storage put's — as the SDK's own business,
    /// not a caller's: a credential is the auth strategy's to add.
    mutating func header(_ name: String, _ value: String) {
        headers.append((name, value))
    }

    /// Renames the operation the hooks hear, keeping the rest of what it means.
    public mutating func operationName(_ name: String) {
        info.operation = name
    }

    /// Names the record the call acts on, for the hooks.
    public mutating func resourceId(_ id: Int) {
        info.resourceId = id
    }

    /// Takes a redirect for the answer rather than following it, as a form post wants.
    public mutating func captureRedirects() {
        capturesRedirects = true
    }

    /// Sends the path as written, with no `.json` put on it: for an endpoint that answers JSON
    /// only under its bare path, or streams a file rather than a document.
    public mutating func withoutJSONSuffix() {
        appendsJSONSuffix = false
    }

    /// Sends the path as written, with a browser's `Accept`: the way in to an endpoint HEY
    /// serves only as a form.
    public mutating func formRepresentation() {
        accept = browserAccept
        appendsJSONSuffix = false
    }

    /// Leaves the response cache alone for this read.
    public mutating func noCache() {
        skipsCache = true
    }

    /// One request inside another operation rather than an operation of its own: the operation
    /// hooks are skipped while the request hooks still fire, so a read a convenience makes on
    /// the way to its own answer shows up as part of that answer.
    public mutating func quiet() {
        isQuiet = true
    }

    /// Sends the request without the client's credentials or account scope, and takes a 401 as
    /// the answer it is rather than a reason to refresh them: for a URL that authenticates
    /// itself, which the storage service's may do on HEY's own origin. The URL and its hops go
    /// exactly as named, and the hooks hear the URL cut back to its origin, since such a URL
    /// carries its signature in the open.
    mutating func unsigned() {
        isUnsigned = true
    }

    /// What an error or a log calls the operation.
    var label: String { info.service == "Raw" ? method.rawValue : info.operation }

    public var description: String {
        let bareId = id.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)[0]
        let barePath = path.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)[0]
        let names = query.map(\.0)
        return "HeyOperation(id=\(bareId), method=\(method.rawValue), path=\(barePath), query=\(names), body=\(body.map(\.description) ?? "nil"))"
    }

    static func forRoute(_ route: Route, _ params: [any CustomStringConvertible & Sendable]) throws -> HeyOperation {
        HeyOperation(
            id: route.id,
            info: OperationInfo(service: route.service, operation: route.id, resourceType: route.resourceType, isMutation: !route.readonly),
            route: route,
            method: route.method,
            path: try route.fill(params),
            url: nil
        )
    }

    static func raw(_ method: HTTPMethod, _ path: String) -> HeyOperation {
        let id = "\(method.rawValue) \(path)"
        return HeyOperation(
            id: id,
            info: OperationInfo(service: "Raw", operation: id, resourceType: "raw", isMutation: method != .get),
            route: nil,
            method: method,
            path: path,
            url: nil
        )
    }

    /// A read of a URL HEY handed out, sent under `route`'s policy when the read that got it had
    /// one.
    static func at(_ method: HTTPMethod, _ url: URL, route: Route? = nil) -> HeyOperation {
        var operation = raw(method, url.path)
        operation.url = url
        operation.route = route
        return operation
    }
}

/// The one JSON encoding every model goes through: no member written as `null`, keys in a
/// fixed order, and slashes left as they are.
func heyJSONEncoder() -> JSONEncoder {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
    return encoder
}
