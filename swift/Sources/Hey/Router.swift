import Foundation

/// What a pasted HEY URL or path is: the pattern it matched, the part of HEY it belongs to, every
/// operation that pattern serves by HTTP method, and the parameters read out of it.
///
/// ```swift
/// let match = Router.shared.recognize("https://app.hey.com/topics/456?x=1")
/// match?.operation   // "GetTopic"
/// match?.resourceId  // "456"
/// ```
public struct RouteMatch: Sendable, Equatable, CustomStringConvertible {
    /// The pattern the path matched, `{param}` placeholders included.
    public let pattern: String
    /// The part of HEY the pattern belongs to, as the model titles it.
    public let resource: String
    /// The operations the pattern serves, by HTTP method.
    public let operations: [HTTPMethod: String]
    /// The path parameters, in the order they appear in the pattern.
    public let params: [Param]

    public struct Param: Sendable, Equatable {
        public let name: String
        public let value: String
    }

    /// The operation a pasted URL most likely means: the GET when the pattern serves one, else the
    /// first by name.
    public var operation: String {
        operations[.get] ?? operations.values.min() ?? ""
    }

    /// The id the path ends in, when it ends in one: the record the URL is of.
    public var resourceId: String? { params.last?.value }

    public var description: String {
        "RouteMatch(pattern=\(pattern), operation=\(operation), params=\(params.map { "\($0.name)=\($0.value)" }))"
    }
}

/// Recognises HEY URLs and paths against the route table, as the Rust, Go and Kotlin routers do:
/// a whole URL or a bare path, with or without a `.json` suffix, a query, a fragment or a trailing
/// slash. Deeper patterns are tried first, so `/boxes/{boxId}/groups/{groupId}` wins over anything
/// shorter that would also fit.
public struct Router: Sendable {
    private struct Candidate: Sendable {
        let pattern: String
        let routes: [Route]
    }

    private let candidates: [Candidate]

    /// The router over every modelled route.
    public static let shared = Router()

    public init(routes: [Route] = Routes.all) {
        var grouped: [String: [Route]] = [:]
        var order: [String] = []
        for route in routes {
            if grouped[route.pattern] == nil { order.append(route.pattern) }
            grouped[route.pattern, default: []].append(route)
        }
        candidates = order.map { Candidate(pattern: $0, routes: grouped[$0] ?? []) }
            .sorted { lhs, rhs in
                let left = lhs.pattern.filter { $0 == "/" }.count
                let right = rhs.pattern.filter { $0 == "/" }.count
                return left != right ? left > right : lhs.pattern < rhs.pattern
            }
    }

    /// The route a URL or path names, or nil when the table has none for it.
    public func recognize(_ pathOrURL: String) -> RouteMatch? {
        var raw: String
        if let url = parseAbsoluteURL(pathOrURL) {
            raw = URLComponents(url: url, resolvingAgainstBaseURL: true)?.percentEncodedPath ?? url.path
        } else {
            raw = String(pathOrURL.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)[0]
                .split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false)[0])
        }
        while raw.hasSuffix("/") { raw.removeLast() }
        if raw.hasSuffix(".json") { raw.removeLast(5) }
        for candidate in candidates {
            guard let first = candidate.routes.first, let params = first.recognize(raw) else { continue }
            var operations: [HTTPMethod: String] = [:]
            for route in candidate.routes { operations[route.method] = route.id }
            return RouteMatch(
                pattern: candidate.pattern,
                resource: first.resource,
                operations: operations,
                params: params.map { RouteMatch.Param(name: $0.0, value: $0.1) }
            )
        }
        return nil
    }
}
