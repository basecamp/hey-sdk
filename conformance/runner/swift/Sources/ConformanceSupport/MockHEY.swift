import Foundation

/// What the mock server saw and answered, for the assertions to read afterwards.
public struct Recorded: Sendable {
    public var times: [UInt64] = []
    public var paths: [String] = []
    public var methods: [String] = []
    public var queries: [[(String, String)]] = []
    public var bodies: [Data] = []
    public var headers: [[(String, String)]] = []
    public var statuses: [Int] = []
    public var links: [String?] = []

    public init() {}

    public var count: Int { paths.count }

    public func header(_ index: Int, _ name: String) -> String? {
        guard headers.indices.contains(index) else { return nil }
        return headers[index].first { $0.0.caseInsensitiveCompare(name) == .orderedSame }?.1
    }
}

/// A loopback server that answers each request with the case's next mock response, in order, and
/// a 500 once they run out. A request to `/` is refused and not recorded: HEY has no root operation.
public final class MockHEY: @unchecked Sendable {
    private var server: LoopbackServer!
    private let lock = NSLock()
    private var answered: [Int: (status: Int, link: String?)] = [:]

    public var baseURL: String { server.baseURL }

    public init(_ responses: [MockResponse]) throws {
        server = try LoopbackServer { [weak self] index, _ in
            guard index < responses.count else {
                return ServedResponse(status: 500, body: Data(#"{"error": "No more mock responses"}"#.utf8))
            }
            let mock = responses[index]
            if let self {
                let link = mock.headers.first { $0.0.caseInsensitiveCompare("Link") == .orderedSame }?.1
                self.lock.lock()
                self.answered[index] = (mock.status, link)
                self.lock.unlock()
            }
            return ServedResponse(status: mock.status, headers: mock.headers, body: mock.bodyData, delay: .milliseconds(mock.delay))
        }
    }

    /// Stops the server and answers what it saw.
    public func shutdown() -> Recorded {
        server.stop()
        var recorded = Recorded()
        for request in server.requests {
            recorded.times.append(request.arrivedAt)
            recorded.paths.append(request.path)
            recorded.methods.append(request.method)
            recorded.queries.append(queryPairs(request.query))
            recorded.bodies.append(request.body)
            recorded.headers.append(request.headers)
        }
        lock.lock()
        for index in answered.keys.sorted() {
            recorded.statuses.append(answered[index]!.status)
            recorded.links.append(answered[index]!.link)
        }
        lock.unlock()
        return recorded
    }
}

/// The pairs of a query or form body, decoded as a browser form is: `+` is a space.
public func queryPairs(_ query: String?) -> [(String, String)] {
    guard let query, !query.isEmpty else { return [] }
    func decode(_ text: Substring) -> String {
        let spaced = text.replacingOccurrences(of: "+", with: " ")
        return spaced.removingPercentEncoding ?? spaced
    }
    return query.split(separator: "&").map { part in
        guard let equals = part.firstIndex(of: "=") else { return (decode(part), "") }
        return (decode(part[..<equals]), decode(part[part.index(after: equals)...]))
    }
}
