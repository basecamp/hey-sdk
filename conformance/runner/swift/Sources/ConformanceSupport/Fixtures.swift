import Foundation
import Hey

/// One conformance case, as the JSON files under conformance/tests write it. Keys the runner does
/// not read are ignored.
public struct TestCase: Sendable {
    public var name: String
    public var operation: String
    public var pathParams: FixtureJSON
    public var queryParams: FixtureJSON
    public var requestBody: FixtureJSON
    public var mockResponses: [MockResponse]
    public var assertions: [Assertion]
    public var configOverrides: ConfigOverrides
    /// Invokes the operation this many times against one client. Zero means once.
    public var repeatOperation: Int

    public init(
        name: String, operation: String, pathParams: FixtureJSON = .object([]), queryParams: FixtureJSON = .object([]),
        requestBody: FixtureJSON = .object([]), mockResponses: [MockResponse] = [], assertions: [Assertion] = [],
        configOverrides: ConfigOverrides = ConfigOverrides(), repeatOperation: Int = 0
    ) {
        self.name = name
        self.operation = operation
        self.pathParams = pathParams
        self.queryParams = queryParams
        self.requestBody = requestBody
        self.mockResponses = mockResponses
        self.assertions = assertions
        self.configOverrides = configOverrides
        self.repeatOperation = repeatOperation
    }

    public var isHeyLayer: Bool { configOverrides.clientLayer == "hey" }

    public var runs: Int { max(repeatOperation, 1) }

    /// Whether the case's operation reads a page HEY serves as HTML, which the SDK asks for as written.
    public var asksForHTML: Bool { Routes.all.contains { $0.id == operation && $0.html } }

    /// Whether the case looks at what the SDK does with the `Link` header.
    public var followsNextPage: Bool { assertions.contains { $0.type == "urlOrigin" } }

    /// Every case in a fixture file.
    public static func load(_ text: String) throws -> [TestCase] {
        guard let cases = try FixtureJSON.parse(text).array else { throw FixtureError("a fixture file is a JSON array of cases") }
        return try cases.map(TestCase.init(fixture:))
    }

    init(fixture: FixtureJSON) throws {
        guard fixture.object != nil else { throw FixtureError("a case is a JSON object") }
        guard let name = fixture["name"]?.string else { throw FixtureError("a case has no name") }
        guard let operation = fixture["operation"]?.string else { throw FixtureError("case \"\(name)\" has no operation") }
        self.init(
            name: name,
            operation: operation,
            pathParams: try Self.object(fixture, "pathParams", name),
            queryParams: try Self.object(fixture, "queryParams", name),
            requestBody: try Self.object(fixture, "requestBody", name),
            mockResponses: try (fixture["mockResponses"]?.array ?? []).map(MockResponse.init(fixture:)),
            assertions: try (fixture["assertions"]?.array ?? []).map(Assertion.init(fixture:)),
            configOverrides: try fixture["configOverrides"].map(ConfigOverrides.init(fixture:)) ?? ConfigOverrides(),
            repeatOperation: fixture["repeatOperation"]?.int ?? 0)
    }

    private static func object(_ fixture: FixtureJSON, _ key: String, _ name: String) throws -> FixtureJSON {
        guard let value = fixture[key], value != .null else { return .object([]) }
        guard value.object != nil else { throw FixtureError("case \"\(name)\": \(key) is not an object") }
        return value
    }
}

public struct ConfigOverrides: Sendable {
    public var baseURL: String?
    public var clientLayer: String?
    public var cacheEnabled = false
    public var refreshableCredentials = false
    public var accountId: Int?
    public var maxRetries: Int?
    public var baseDelayMs: Int?

    public init() {}

    init(fixture: FixtureJSON) throws {
        baseURL = fixture["baseUrl"]?.string
        clientLayer = fixture["clientLayer"]?.string
        cacheEnabled = fixture["cacheEnabled"]?.bool ?? false
        refreshableCredentials = fixture["refreshableCredentials"]?.bool ?? false
        accountId = fixture["accountId"]?.int
        maxRetries = fixture["maxRetries"]?.int
        baseDelayMs = fixture["baseDelayMs"]?.int
    }
}

public struct MockResponse: Sendable {
    public var status: Int
    public var headers: [(String, String)]
    public var body: FixtureJSON?
    /// Milliseconds to wait before answering.
    public var delay: Int

    public init(status: Int, headers: [(String, String)] = [], body: FixtureJSON? = nil, delay: Int = 0) {
        self.status = status
        self.headers = headers
        self.body = body
        self.delay = delay
    }

    init(fixture: FixtureJSON) throws {
        let headers = try (fixture["headers"]?.object ?? []).map { name, value in
            guard let text = value.string else { throw FixtureError("mock header \(name) is not a string") }
            return (name, text)
        }
        let body = fixture["body"]
        self.init(
            status: fixture["status"]?.int ?? 0, headers: headers, body: body == .null ? nil : body,
            delay: fixture["delay"]?.int ?? 0)
    }

    public var contentType: String? {
        headers.first { $0.0.caseInsensitiveCompare("Content-Type") == .orderedSame }?.1
    }

    public var servesHTML: Bool { contentType?.hasPrefix("text/html") == true }

    /// The bytes the mock answers with: a page HEY serves as HTML as written, anything else as JSON.
    public var bodyData: Data {
        switch body {
        case nil: return Data()
        case let .string(text)? where servesHTML: return Data(text.utf8)
        case let value?: return Data(value.text.utf8)
        }
    }
}

public struct Assertion: Sendable {
    public var type: String
    public var expected: FixtureJSON
    public var min: Double
    public var path: String

    public init(type: String, expected: FixtureJSON = .null, min: Double = 0, path: String = "") {
        self.type = type
        self.expected = expected
        self.min = min
        self.path = path
    }

    init(fixture: FixtureJSON) throws {
        guard let type = fixture["type"]?.string else { throw FixtureError("an assertion has no type") }
        self.init(
            type: type, expected: fixture["expected"] ?? .null,
            min: fixture["min"].flatMap { if case let .number(text) = $0 { Double(text) } else { nil } } ?? 0,
            path: fixture["path"]?.string ?? "")
    }
}

extension FixtureJSON {
    /// The integer at `key`, or zero.
    public func int(_ key: String) -> Int { self[key]?.int ?? 0 }

    /// The string at `key`, or empty.
    public func string(_ key: String) -> String { self[key]?.string ?? "" }

    /// The value when it is a string.
    public func stringOrNil(_ key: String) -> String? { self[key]?.string }

    public func boolOrNil(_ key: String) -> Bool? { self[key]?.bool }

    /// The value when it is a non-empty string, for the wire fields an empty string omits.
    public func nonEmptyString(_ key: String) -> String? { stringOrNil(key).flatMap { $0.isEmpty ? nil : $0 } }

    /// Whether the key is there at all, for the optional parameters a case sends by naming them.
    public func has(_ key: String) -> Bool { object?.contains { $0.0 == key } ?? false }

    public func gatedString(_ key: String) -> String? { has(key) ? string(key) : nil }

    public func gatedInt(_ key: String) -> Int? { has(key) ? int(key) : nil }

    public func intList(_ key: String) -> [Int] { self[key]?.array?.compactMap(\.int) ?? [] }

    public func intListOrNil(_ key: String) -> [Int]? { self[key]?.array?.compactMap(\.int) }

    public func stringList(_ key: String) -> [String] { self[key]?.array?.compactMap(\.string) ?? [] }

    public func stringListOrNil(_ key: String) -> [String]? { self[key]?.array?.compactMap(\.string) }
}
