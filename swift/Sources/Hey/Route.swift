import Foundation

/// The HTTP methods the model's routes are sent with.
public enum Method: String, Sendable, Equatable, Hashable, CaseIterable {
    case get = "GET"
    case post = "POST"
    case put = "PUT"
    case patch = "PATCH"
    case delete = "DELETE"
}

/// Where a path parameter sits: the last segment names the record itself, anything before it
/// a parent.
public enum ParamRole: String, Sendable, Equatable {
    /// Names a record the one the route acts on belongs to: the box a group is in.
    case parent
    /// Names the record the route acts on.
    case recording
}

/// The type the model gives a path parameter's value.
public enum ParamKind: String, Sendable, Equatable {
    case string, bool, int32, int64
}

/// How a route pages its answer.
public enum Pagination: String, Sendable, Equatable {
    /// The whole answer comes at once.
    case unpaged
    /// HEY names the next page in a `Link` header; see ``Page``.
    case link
    /// The read covers a window of dates, and the caller moves the window to read on.
    case window
}

/// The retry policy the model attaches to a route: how many attempts in all, how long the
/// first wait is, and which statuses are worth another try.
public struct RetryPolicy: Sendable, Equatable {
    /// The most attempts the route is given, the first one included. Zero is one send and no
    /// resend.
    public var max: Int
    /// The wait before the second attempt, in milliseconds; later waits double it.
    public var baseDelayMs: Int
    /// The statuses that are worth another attempt.
    public var retryOn: [Int]

    public init(max: Int, baseDelayMs: Int, retryOn: [Int]) {
        self.max = max
        self.baseDelayMs = baseDelayMs
        self.retryOn = retryOn
    }
}

/// One `{param}` placeholder in a route's path.
public struct RouteParam: Sendable, Equatable {
    /// The placeholder's name as it appears in the path: `boxId`.
    public var name: String
    /// Whether the parameter names the record itself or a parent of it.
    public var role: ParamRole
    /// The type the model gives the value.
    public var kind: ParamKind

    public init(name: String, role: ParamRole, kind: ParamKind) {
        self.name = name
        self.role = role
        self.kind = kind
    }
}

/// One API operation: its method, its path template and the behaviour the Smithy model
/// attaches to it. Every route the SDK knows is in ``Routes``.
public struct Route: Sendable, Equatable {
    /// The operation as the model names it: `ListBoxes`, `GetTopic`.
    public var id: String
    /// The service whose method sends this route, as every HEY SDK names it: `Boxes`,
    /// `TimeTracks`.
    public var service: String
    /// The HTTP method the route is sent with.
    public var method: Method
    /// The path as HEY serves it, `{param}` placeholders included.
    public var path: String
    /// The path without a `.json` suffix, for recognizing pasted URLs.
    public var pattern: String
    /// The part of HEY the route belongs to, as the model titles it: `Boxes`,
    /// `Calendar Time Tracks`.
    public var resource: String
    /// The kind of record the route acts on, in `snake_case`: `box`, `box_group`.
    public var resourceType: String
    /// The path parameters, in the order they appear in ``path``.
    public var params: [RouteParam]
    /// The route may be sent again after a failure without doing its work twice.
    public var idempotent: Bool
    /// The route only reads; nothing it does changes anything.
    public var readonly: Bool
    /// The route answers a page as HTML, so it is asked for as written — no `.json` suffix —
    /// with `Accept: text/html`.
    public var html: Bool
    /// The statuses that mean HEY has nothing for this route rather than that it failed.
    public var emptyOn: [Int]
    /// How the route pages, when it does.
    public var pagination: Pagination
    /// The query parameter a `Link` names a further page with, when the route pages; a `Link`
    /// without it is a cursor to poll next, not a page.
    public var pageParameter: String?
    /// The retry policy the model attaches to the route.
    public var retry: RetryPolicy

    public init(
        id: String, service: String, method: Method, path: String, pattern: String, resource: String,
        resourceType: String, params: [RouteParam], idempotent: Bool, readonly: Bool, html: Bool,
        emptyOn: [Int], pagination: Pagination, pageParameter: String?, retry: RetryPolicy
    ) {
        self.id = id
        self.service = service
        self.method = method
        self.path = path
        self.pattern = pattern
        self.resource = resource
        self.resourceType = resourceType
        self.params = params
        self.idempotent = idempotent
        self.readonly = readonly
        self.html = html
        self.emptyOn = emptyOn
        self.pagination = pagination
        self.pageParameter = pageParameter
        self.retry = retry
    }

    /// Substitutes the path parameters, in order, percent-encoding each value.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when `values` is not exactly as long as
    ///   ``params``: a short list would leave a `{param}` in the path and send it to HEY as
    ///   written.
    public func fill(_ values: [any CustomStringConvertible & Sendable]) throws -> String {
        guard values.count == params.count else {
            throw HeyError.usage(message: "\(id) takes \(params.count) path parameters, got \(values.count)")
        }
        var filled = path
        for (param, value) in zip(params, values) {
            filled = filled.replacingOccurrences(of: "{\(param.name)}", with: percentEncodeComponent(value.description))
        }
        return filled
    }

    /// Matches a path against the route's pattern and answers the captured parameters, in
    /// order, or nil.
    public func recognize(_ path: String) -> [(String, String)]? {
        let patternSegments = pattern.split(separator: "/", omittingEmptySubsequences: false)
        let pathSegments = path.split(separator: "/", omittingEmptySubsequences: false)
        guard patternSegments.count == pathSegments.count else { return nil }
        var captured: [(String, String)] = []
        for (expected, actual) in zip(patternSegments, pathSegments) {
            if expected.hasPrefix("{"), expected.hasSuffix("}") {
                if actual.isEmpty { return nil }
                let name = String(expected.dropFirst().dropLast())
                guard params.contains(where: { $0.name == name }) else { return nil }
                captured.append((name, String(actual)))
            } else if expected != actual {
                return nil
            }
        }
        return captured
    }
}

private let unreserved = Set("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~".utf8)

/// Percent-encodes everything but the unreserved characters, so a value with a `/`, a `&` or a
/// `+` in it stays one path segment or one query value.
func percentEncodeComponent(_ value: String) -> String {
    var out = ""
    out.reserveCapacity(value.utf8.count)
    for byte in value.utf8 {
        if unreserved.contains(byte) {
            out.unicodeScalars.append(Unicode.Scalar(byte))
        } else {
            out += String(format: "%%%02X", byte)
        }
    }
    return out
}
