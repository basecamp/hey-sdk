import Foundation

/// HTTP header fields in the order they were added, looked up without regard to case, and
/// allowed to repeat a name.
public struct HTTPHeaders: Sendable, Equatable, Sequence {
    private var fields: [(name: String, value: String)]

    public init(_ fields: [(String, String)] = []) {
        self.fields = fields.map { (name: $0.0, value: $0.1) }
    }

    public static func == (lhs: HTTPHeaders, rhs: HTTPHeaders) -> Bool {
        lhs.fields.count == rhs.fields.count && zip(lhs.fields, rhs.fields).allSatisfy { $0 == $1 }
    }

    /// The first value under `name`, whatever its case.
    public subscript(name: String) -> String? {
        fields.first { $0.name.caseInsensitiveCompare(name) == .orderedSame }?.value
    }

    /// Every value under `name`, whatever its case, in order.
    public func values(for name: String) -> [String] {
        fields.filter { $0.name.caseInsensitiveCompare(name) == .orderedSame }.map(\.value)
    }

    /// The distinct names, lowercased, in the order they first appear.
    public var names: [String] {
        var seen = Set<String>()
        return fields.map { $0.name.lowercased() }.filter { seen.insert($0).inserted }
    }

    /// Adds a value under `name`, keeping any already there.
    public mutating func add(_ name: String, _ value: String) {
        fields.append((name: name, value: value))
    }

    /// Replaces whatever is under `name` with one value.
    public mutating func set(_ name: String, _ value: String) {
        remove(name)
        add(name, value)
    }

    /// Removes every value under `name`.
    public mutating func remove(_ name: String) {
        fields.removeAll { $0.name.caseInsensitiveCompare(name) == .orderedSame }
    }

    public var isEmpty: Bool { fields.isEmpty }

    public func makeIterator() -> AnyIterator<(name: String, value: String)> {
        AnyIterator(fields.makeIterator())
    }
}

/// One request as it goes on the wire.
public struct HTTPRequest: Sendable {
    public var method: String
    public var url: URL
    public var headers: HTTPHeaders
    public var body: Data?

    public init(method: String, url: URL, headers: HTTPHeaders = HTTPHeaders(), body: Data? = nil) {
        self.method = method
        self.url = url
        self.headers = headers
        self.body = body
    }
}

/// One answer as it came off the wire: the status and headers, and as much of the body as the
/// client asked to be read.
public struct HTTPResponse: Sendable {
    public var status: Int
    public var headers: HTTPHeaders
    public var body: Data
    /// The body ran past the limit it was read to, so ``body`` is not all of it.
    public var bodyExceeded: Bool

    public init(status: Int, headers: HTTPHeaders = HTTPHeaders(), body: Data = Data(), bodyExceeded: Bool = false) {
        self.status = status
        self.headers = headers
        self.body = body
        self.bodyExceeded = bodyExceeded
    }
}

/// What sends a request and brings back its answer. ``URLSessionTransport`` is the one the
/// SDK ships; a test hands over one of its own.
///
/// A transport sends the request exactly as given and does not follow a redirect: the client
/// follows redirects itself, since it decides what credentials, validators and account scope a
/// hop may carry. Once the status and headers are in it asks `bodyLimit` how much of the body
/// to read: `nil` to read none of it, or a number of bytes, in which case it reads no more than
/// one byte past that and reports the overrun in ``HTTPResponse/bodyExceeded`` rather than
/// holding the rest.
public protocol Transport: Sendable {
    func send(
        _ request: HTTPRequest, bodyLimit: @escaping @Sendable (_ status: Int, _ headers: HTTPHeaders) -> Int?
    ) async throws -> HTTPResponse
}
