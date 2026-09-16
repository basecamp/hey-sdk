import Foundation
import Hey

public struct AssertionFailure: Error, CustomStringConvertible {
    public let description: String
    public init(_ description: String) { self.description = description }
}

/// What an operation answered, in a shape the assertions can read: the parsed model as JSON, plus
/// a page's metadata.
public enum Outcome: Sendable {
    case unit
    case json(FixtureJSON)
    /// `nextURLCheck` is what following the next page did, when the case asked.
    case page(value: FixtureJSON, nextPage: String?, totalCount: Int?, nextURLCheck: Result<Void, any Error>?)

    public var body: FixtureJSON? {
        switch self {
        case .unit: return nil
        case let .json(value): return value
        case let .page(value, _, _, _): return value
        }
    }
}

/// One case after it ran: what the SDK answered and what the mock server saw.
public struct Run: Sendable {
    public var testCase: TestCase
    public var outcome: Result<Outcome, any Error>
    public var recorded: Recorded
    public var baseURL: String

    public init(testCase: TestCase, outcome: Result<Outcome, any Error>, recorded: Recorded, baseURL: String) {
        self.testCase = testCase
        self.outcome = outcome
        self.recorded = recorded
        self.baseURL = baseURL
    }

    var error: (any Error)? {
        if case let .failure(error) = outcome { return error }
        return nil
    }
}

public func checkAll(_ run: Run) throws {
    for assertion in run.testCase.assertions {
        try check(run, assertion)
    }
}

private func fail(_ message: String) -> AssertionFailure { AssertionFailure(message) }

private enum Which {
    case first, last

    var label: String { self == .first ? "first" : "last" }

    func pick<T>(_ values: [T]) -> T? { self == .first ? values.first : values.last }
}

private func check(_ run: Run, _ assertion: Assertion) throws {
    switch assertion.type {
    case "requestCount": try checkRequestCount(run, assertion)
    case "delayBetweenRequests": try checkDelayBetweenRequests(run, assertion)
    case "noError": try checkNoError(run)
    case "errorCode": try checkErrorCode(run, assertion)
    case "errorField": try checkErrorField(run, assertion)
    case "statusCode": try checkStatusCode(run, assertion)
    case "requestPath": try checkRequestPath(run, assertion, .first)
    case "lastRequestPath": try checkRequestPath(run, assertion, .last)
    case "requestMethod": try checkRequestMethod(run, assertion)
    case "requestQuery": try checkRequestQuery(run, assertion, .first)
    case "lastRequestQuery": try checkRequestQuery(run, assertion, .last)
    case "requestBody": try checkRequestBody(run, assertion)
    case "requestForm": try checkRequestForm(run, assertion, .first)
    case "lastRequestForm": try checkRequestForm(run, assertion, .last)
    case "headerPresent": try checkHeaderPresent(run, assertion)
    case "lastRequestHeader": try checkLastRequestHeader(run, assertion)
    case "responseMeta": try checkResponseMeta(run, assertion)
    case "urlOrigin": try checkURLOrigin(run, assertion)
    case "responseBody": try checkResponseBody(run, assertion)
    default: throw fail("Unknown assertion type: \(assertion.type)")
    }
}

private func checkRequestCount(_ run: Run, _ assertion: Assertion) throws {
    let expected = try expectedInt(assertion, "requestCount")
    if run.recorded.count != expected { throw fail("Expected \(expected) requests, got \(run.recorded.count)") }
}

/// Every wait between one request and the next, not only the first: a backoff that collapses after
/// the second send is still a broken backoff.
private func checkDelayBetweenRequests(_ run: Run, _ assertion: Assertion) throws {
    let minimum = Int64(assertion.min)
    let times = run.recorded.times
    for index in times.indices.dropFirst() {
        let delay = (Int64(times[index]) - Int64(times[index - 1])) / 1_000_000
        if delay < minimum { throw fail("Expected delay >= \(minimum)ms before request \(index + 1), got \(delay)ms") }
    }
}

private func lastStatus(_ run: Run) -> Int { run.recorded.statuses.last ?? 0 }

private func checkNoError(_ run: Run) throws {
    guard let error = run.error else { return }
    let emptyOn = Routes.all.first { $0.id == run.testCase.operation }?.emptyOn ?? []
    if emptyOn.contains(lastStatus(run)) {
        throw fail("Expected no error, got: \(error) (the route treats \(lastStatus(run)) as empty for \(run.testCase.operation))")
    }
    throw fail("Expected no error, got: \(error)")
}

private func checkErrorCode(_ run: Run, _ assertion: Assertion) throws {
    let expected = try expectedString(assertion, "errorCode")
    guard let error = run.error else { throw fail("Expected error code \"\(expected)\", but got no error") }
    guard let heyError = error as? HeyError else {
        throw fail("Expected error code \"\(expected)\", got a non-SDK failure: \(error)")
    }
    if heyError.code != expected { throw fail("Expected error code \"\(expected)\", got \"\(heyError.code)\"") }
}

private func checkErrorField(_ run: Run, _ assertion: Assertion) throws {
    guard let error = run.error as? HeyError else {
        throw fail("Expected error field \"\(assertion.path)\", but got no error")
    }
    switch assertion.path {
    case "httpStatus":
        let expected = try expectedInt(assertion, "errorField.httpStatus")
        let actual = error.httpStatus ?? 0
        if actual != expected { throw fail("Expected error httpStatus \(expected), got \(actual)") }
    case "retryable":
        let expected = try expectedBool(assertion, "errorField.retryable")
        if error.isRetryable != expected { throw fail("Expected error retryable=\(expected), got \(error.isRetryable)") }
    case "requestId":
        let expected = try expectedString(assertion, "errorField.requestId")
        let actual = error.requestId ?? ""
        if actual != expected { throw fail("Expected error requestId \"\(expected)\", got \"\(actual)\"") }
    default:
        throw fail("Unknown error field: \(assertion.path)")
    }
}

/// A failure has to carry the status on the error itself; only a success reads it off the server.
private func checkStatusCode(_ run: Run, _ assertion: Assertion) throws {
    let expected = try expectedInt(assertion, "statusCode")
    let actual: Int
    if let error = run.error {
        guard let status = (error as? HeyError)?.httpStatus, status > 0 else {
            throw fail("Expected status code \(expected), but the SDK error carries no HTTP status: \(error)")
        }
        actual = status
    } else {
        actual = lastStatus(run)
    }
    if actual != expected { throw fail("Expected status code \(expected), got \(actual)") }
}

private func checkRequestPath(_ run: Run, _ assertion: Assertion, _ which: Which) throws {
    let raw = try expectedString(assertion, "requestPath")
    let expected = run.testCase.asksForHTML ? raw : withJSONExtension(raw)
    guard let actual = which.pick(run.recorded.paths) else { throw fail(noRequests) }
    if actual != expected { throw fail("Expected \(which.label) request path \"\(expected)\", got \"\(actual)\"") }
}

/// HEY answers JSON to paths ending in `.json`, and this SDK puts the extension back on a path
/// whose last segment has none, so the expected path is normalised the same way. A page HEY serves
/// as HTML is the exception: the SDK asks for it as written.
public func withJSONExtension(_ path: String) -> String {
    let lastSegment = path.split(separator: "/", omittingEmptySubsequences: false).last ?? ""
    return path.isEmpty || path.hasSuffix("/") || lastSegment.contains(".") ? path : path + ".json"
}

private func checkRequestMethod(_ run: Run, _ assertion: Assertion) throws {
    let expected = try expectedString(assertion, "requestMethod")
    guard let actual = run.recorded.methods.first else { throw fail(noRequests) }
    if actual != expected { throw fail("Expected request method \"\(expected)\", got \"\(actual)\"") }
}

private func checkRequestQuery(_ run: Run, _ assertion: Assertion, _ which: Which) throws {
    let expected = try expectedObject(assertion, assertion.type)
    guard let query = which.pick(run.recorded.queries) else { throw fail(noRequests) }
    for (name, want) in expected {
        // A scalar the fixture expects once has to be there exactly once: a second copy of an
        // account filter is a request scoped to two accounts, whichever HEY reads.
        let values = query.filter { $0.0 == name }.map(\.1)
        if want == .null {
            if !values.isEmpty {
                throw fail("Expected \(which.label) query param \"\(name)\" to be absent, got \"\(values.joined(separator: "&"))\"")
            }
        } else if values.isEmpty {
            throw fail("Expected \(which.label) query param \(name)=\(display(want)), got \"\"")
        } else if values.count > 1 {
            throw fail("Expected \(which.label) query param \(name)=\(display(want)) once, got it \(values.count) times")
        } else if values[0] != display(want) {
            throw fail("Expected \(which.label) query param \(name)=\(display(want)), got \"\(values[0])\"")
        }
    }
}

private func checkRequestBody(_ run: Run, _ assertion: Assertion) throws {
    let expected = try expectedObject(assertion, "requestBody")
    guard let raw = run.recorded.bodies.first else { throw fail(noRequests) }
    let body: FixtureJSON
    if raw.isEmpty {
        body = .null
    } else {
        do {
            body = try FixtureJSON.parse(String(decoding: raw, as: UTF8.self))
        } catch {
            throw fail("requestBody: request body is not JSON: \(error)")
        }
    }
    for (path, want) in expected {
        let got = lookup(body, path)
        switch (want, got) {
        case (.null, nil): continue
        case let (.null, got?): throw fail("Expected body key \"\(path)\" to be absent, got \(display(got))")
        case (_, nil): throw fail("Expected body key \"\(path)\" = \(display(want)), but it is absent")
        case let (_, got?):
            if !valuesMatch(want, got) { throw fail("Expected body key \"\(path)\" = \(display(want)), got \(display(got))") }
        }
    }
}

private func checkRequestForm(_ run: Run, _ assertion: Assertion, _ which: Which) throws {
    let expected = try expectedObject(assertion, assertion.type)
    guard let raw = which.pick(run.recorded.bodies) else { throw fail(noRequests) }
    let fields = queryPairs(String(decoding: raw, as: UTF8.self))
    for (name, want) in expected {
        // A scalar the fixture expects once has to be there exactly once: a second copy with
        // another value is a form the server may read either way.
        let values = fields.filter { $0.0 == name }.map(\.1)
        let type = assertion.type
        if want == .null {
            if !values.isEmpty {
                throw fail("\(type): expected form field \"\(name)\" to be absent, got \"\(values.joined(separator: "&"))\"")
            }
        } else if values.isEmpty {
            throw fail("\(type): expected form field \"\(name)\" = \(display(want)), but it is absent")
        } else if values.count > 1 {
            throw fail("\(type): expected form field \"\(name)\" = \(display(want)) once, got it \(values.count) times")
        } else if values[0] != display(want) {
            throw fail("\(type): expected form field \"\(name)\" = \(display(want)), got \"\(values[0])\"")
        }
    }
}

private func checkHeaderPresent(_ run: Run, _ assertion: Assertion) throws {
    let name = assertion.path
    if run.recorded.headers.isEmpty { throw fail("Expected request with header \"\(name)\", but no requests were recorded") }
    if (run.recorded.header(0, name) ?? "").isEmpty { throw fail("Expected header \"\(name)\" to be present, but it was not") }
}

private func checkLastRequestHeader(_ run: Run, _ assertion: Assertion) throws {
    let name = assertion.path
    let expected = try expectedString(assertion, "lastRequestHeader")
    if run.recorded.headers.isEmpty { throw fail("Expected request with header \"\(name)\", but no requests were recorded") }
    let actual = run.recorded.header(run.recorded.headers.count - 1, name) ?? ""
    if actual != expected { throw fail("Expected last request header \"\(name)\" = \"\(expected)\", got \"\(actual)\"") }
}

private func checkResponseMeta(_ run: Run, _ assertion: Assertion) throws {
    guard case let .success(.page(_, nextPage, totalCount, _)) = run.outcome else {
        throw fail("Expected a paginated result to read responseMeta.\(assertion.path) from")
    }
    switch assertion.path {
    case "totalCount":
        let expected = try expectedInt(assertion, "responseMeta.totalCount")
        guard let actual = totalCount else { throw fail("X-Total-Count header not present in response") }
        if actual != expected { throw fail("Expected X-Total-Count=\(expected), got \(actual)") }
    case "nextPage":
        let expected = try expectedString(assertion, "responseMeta.nextPage")
        guard let actual = nextPage else { throw fail("Link header does not contain a valid next URL") }
        if actual != expected { throw fail("Expected next page \"\(expected)\", got \"\(actual)\"") }
    default:
        throw fail("Unknown responseMeta path: \(assertion.path)")
    }
}

private func checkURLOrigin(_ run: Run, _ assertion: Assertion) throws {
    let expected = try expectedString(assertion, "urlOrigin")
    if expected != "rejected" {
        throw fail("urlOrigin: unsupported expected value \"\(expected)\" (only \"rejected\" is supported)")
    }
    guard let last = run.recorded.links.last, let link = last, !link.isEmpty else {
        throw fail("No Link header in response to validate origin")
    }
    guard let target = nextLink(link) else { throw fail("No next URL found in Link header: \(link)") }
    guard let next = URLComponents(string: target), next.scheme != nil, next.host != nil else {
        throw fail("Expected cross-origin Link URL for rejection test, but got relative URL: \(target)")
    }
    if let server = URLComponents(string: run.baseURL), sameOrigin(next, server) {
        throw fail("Expected cross-origin Link URL for rejection test, but \(target) has same origin as server")
    }
    try checkNextPageRefused(run)
}

private func port(_ url: URLComponents) -> Int {
    url.port ?? (url.scheme?.lowercased() == "https" ? 443 : 80)
}

private func sameOrigin(_ a: URLComponents, _ b: URLComponents) -> Bool {
    a.scheme?.lowercased() == b.scheme?.lowercased() && (a.host ?? "").lowercased() == (b.host ?? "").lowercased()
        && port(a) == port(b)
}

private func checkNextPageRefused(_ run: Run) throws {
    let outcome: Outcome
    switch run.outcome {
    case let .failure(error): throw fail("Expected a paginated result to read the next page from, got: \(error)")
    case let .success(value): outcome = value
    }
    guard case let .page(_, _, _, check?) = outcome else { throw fail("Expected a paginated result to read the next page from") }
    guard case let .failure(error) = check else {
        throw fail("Expected the cross-origin next page to be refused, but the SDK followed it")
    }
    let code = (error as? HeyError)?.code
    if code != HeyError.codeUsage {
        throw fail("Expected the cross-origin next page to be refused as a usage error, got \(code ?? "\(error)")")
    }
}

private func checkResponseBody(_ run: Run, _ assertion: Assertion) throws {
    let path = assertion.path
    let outcome: Outcome
    switch run.outcome {
    case let .failure(error): throw fail("Expected responseBody.\(path), got: \(error)")
    case let .success(value): outcome = value
    }
    guard let body = outcome.body else { throw fail("Expected responseBody.\(path), but no response body captured") }
    guard let actual = lookup(body, path) else { throw fail("Expected responseBody.\(path), but field not present") }
    if !valuesMatch(assertion.expected, actual) {
        throw fail("Expected responseBody.\(path) = \(display(assertion.expected)), got \(display(actual))")
    }
}

private let noRequests = "Expected a request, but none were recorded"

private func expectedInt(_ assertion: Assertion, _ label: String) throws -> Int {
    guard let value = assertion.expected.int else { throw fail("\(label): expected an integer, got \(display(assertion.expected))") }
    return value
}

private func expectedBool(_ assertion: Assertion, _ label: String) throws -> Bool {
    guard let value = assertion.expected.bool else { throw fail("\(label): expected a bool, got \(display(assertion.expected))") }
    return value
}

private func expectedString(_ assertion: Assertion, _ label: String) throws -> String {
    guard let value = assertion.expected.string else { throw fail("\(label): expected a string, got \(display(assertion.expected))") }
    return value
}

private func expectedObject(_ assertion: Assertion, _ label: String) throws -> [(String, FixtureJSON)] {
    guard let value = assertion.expected.object else { throw fail("\(label): expected an object, got \(display(assertion.expected))") }
    return value
}

/// Walks a JSON value by a dot-separated path, reading integer segments as array indexes.
public func lookup(_ value: FixtureJSON, _ path: String) -> FixtureJSON? {
    var current = value
    for segment in path.split(separator: ".", omittingEmptySubsequences: false) {
        switch current {
        case .object:
            guard let next = current[String(segment)] else { return nil }
            current = next
        case let .array(elements):
            guard let index = Int(segment), elements.indices.contains(index) else { return nil }
            current = elements[index]
        default:
            return nil
        }
    }
    return current
}

/// Whether two JSON values are the same value of the same kind. A number and the string of that
/// number are not: an SDK that writes `"1"` where the fixture expects `1` has changed the type on
/// the wire, which is what these fixtures exist to catch. Numbers compare as numbers, so `1` and
/// `1.0` agree.
public func valuesMatch(_ expected: FixtureJSON, _ actual: FixtureJSON) -> Bool {
    switch (expected, actual) {
    case let (.number(a), .number(b)):
        // Exact decimal arithmetic: 1 and 1.0 are the same number, 9007199254740993 and
        // 9007199254740992.0 are not, whatever a double would make of them.
        guard let x = Decimal(string: a, locale: Locale(identifier: "en_US_POSIX")),
              let y = Decimal(string: b, locale: Locale(identifier: "en_US_POSIX"))
        else { return a == b }
        return x == y
    case let (.array(a), .array(b)):
        return a.count == b.count && zip(a, b).allSatisfy { valuesMatch($0, $1) }
    case let (.object(a), .object(b)):
        return a.count == b.count && a.allSatisfy { key, value in
            guard let other = actual[key] else { return false }
            return valuesMatch(value, other)
        }
    default:
        return expected == actual
    }
}

public func display(_ value: FixtureJSON) -> String {
    if case let .string(text) = value { return text }
    return value.text
}
