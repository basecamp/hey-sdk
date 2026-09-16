import Foundation
import XCTest

@testable import Hey

/// One canned answer the mock serves, in order.
struct Answer: Sendable {
    var status: Int
    var body: String?
    var headers: [(String, String)]
    var failure: (any Error)?

    init(status: Int, body: String? = nil, headers: [(String, String)] = [], failure: (any Error)? = nil) {
        self.status = status
        self.body = body
        self.headers = headers
        self.failure = failure
    }
}

func ok(_ body: String = "{}", _ headers: [(String, String)] = []) -> Answer {
    Answer(status: 200, body: body, headers: headers)
}

func status(_ code: Int, _ body: String? = nil, _ headers: [(String, String)] = []) -> Answer {
    Answer(status: code, body: body, headers: headers)
}

func failure(_ error: any Error) -> Answer {
    Answer(status: 0, failure: error)
}

/// A request the mock saw, as it went on the wire.
struct RecordedRequest: Sendable {
    let method: String
    let url: URL
    let headers: HTTPHeaders
    let body: String

    var path: String { URLComponents(url: url, resolvingAgainstBaseURL: true)?.percentEncodedPath ?? url.path }
    var encodedQuery: String? { URLComponents(url: url, resolvingAgainstBaseURL: true)?.percentEncodedQuery }

    func query(_ name: String) -> String? {
        URLComponents(url: url, resolvingAgainstBaseURL: true)?.queryItems?.first { $0.name == name }?.value
    }

    func queryAll(_ name: String) -> [String] {
        URLComponents(url: url, resolvingAgainstBaseURL: true)?.queryItems?.filter { $0.name == name }.compactMap(\.value) ?? []
    }

    func header(_ name: String) -> String? { headers[name] }
}

/// A mock HEY that answers each request with the next canned answer and remembers what it saw.
/// Requests may arrive from several tasks at once, so they are recorded under a lock and take the
/// answer at the index they were recorded at.
final class MockHey: Transport, @unchecked Sendable {
    private let answers: [Answer]
    private let lock = NSLock()
    private var recorded: [RecordedRequest] = []
    private var waits: [Duration] = []
    /// Run with each request once it is recorded and before it is answered, to hold an answer back.
    var hold: (@Sendable (_ index: Int, _ request: RecordedRequest) async -> Void)?

    init(_ answers: [Answer]) {
        self.answers = answers
    }

    var requests: [RecordedRequest] {
        lock.lock()
        defer { lock.unlock() }
        return recorded
    }

    /// Every wait the client asked to sleep, in order.
    var sleeps: [Duration] {
        lock.lock()
        defer { lock.unlock() }
        return waits
    }

    func send(_ request: HTTPRequest, bodyLimit: @escaping @Sendable (Int, HTTPHeaders) -> Int?) async throws -> HTTPResponse {
        let seen = RecordedRequest(
            method: request.method, url: request.url, headers: request.headers,
            body: request.body.map { String(decoding: $0, as: UTF8.self) } ?? "")
        let index = record(seen)
        if let hold { await hold(index, seen) }
        let answer = index < answers.count ? answers[index] : Answer(status: 500, body: #"{"error":"No more mock responses"}"#)
        if let failure = answer.failure { throw failure }
        var headers = HTTPHeaders()
        if !answer.headers.contains(where: { $0.0.caseInsensitiveCompare("Content-Type") == .orderedSame }) {
            headers.add("Content-Type", "application/json")
        }
        for (name, value) in answer.headers { headers.add(name, value) }
        let bytes = Data((answer.body ?? "").utf8)
        guard let limit = bodyLimit(answer.status, headers) else {
            return HTTPResponse(status: answer.status, headers: headers)
        }
        if bytes.count > limit {
            return HTTPResponse(status: answer.status, headers: headers, body: bytes.prefix(limit + 1), bodyExceeded: true)
        }
        return HTTPResponse(status: answer.status, headers: headers, body: bytes)
    }

    private func record(_ request: RecordedRequest) -> Int {
        lock.lock()
        defer { lock.unlock() }
        recorded.append(request)
        return recorded.count - 1
    }

    fileprivate func recordSleep(_ duration: Duration) {
        lock.lock()
        defer { lock.unlock() }
        waits.append(duration)
    }

    /// A client of this mock, signed with `test-token` unless told otherwise, whose retry waits are
    /// recorded rather than slept.
    func client(
        auth: (any AuthStrategy)? = nil,
        hooks: (any HeyHooks)? = nil,
        cache: (any ResponseCache)? = nil,
        configure: (inout HeyConfig) -> Void = { _ in },
        clock: (@Sendable () -> Duration)? = nil
    ) throws -> HeyClient {
        var config = HeyConfig(timeout: nil, maxRetryJitter: .zero)
        configure(&config)
        return try HeyClient(
            auth: auth ?? BearerAuth(tokenProvider: try StaticTokenProvider("test-token")),
            config: config,
            hooks: hooks,
            transport: self,
            cache: cache,
            clock: clock ?? ContinuousClock().monotonicNow,
            sleeper: { [weak self] duration in self?.recordSleep(duration) }
        )
    }
}

func mockHey(_ answers: Answer...) -> MockHey { MockHey(answers) }

let identityJSON = #"{"id":1,"primary_contact":{"id":9},"accounts":[{"id":42,"status":"active"},{"id":43,"status":"inactive","purpose":"personal"}],"senders":[{"id":100,"account_id":42,"default":true},{"id":101,"account_id":42},{"id":200,"account_id":7,"default":true}],"all_users":[{"id":1000,"account_id":42},{"id":7000,"account_id":7}]}"#

/// The `Accept` a form request goes out with.
let browserAcceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

/// The operations the hooks heard start and end, for a test of what a convenience announces itself as.
final class OperationLog: HeyHooks, @unchecked Sendable {
    private let lock = NSLock()
    private var startedLog: [String] = []
    private var endedLog: [String] = []

    var started: [String] { lock.withLock { startedLog } }
    var ended: [String] { lock.withLock { endedLog } }

    func onOperationStart(_ info: OperationInfo) {
        lock.withLock { startedLog.append("\(info.service).\(info.operation):\(info.resourceType):\(info.isMutation):\(info.resourceId.map(String.init) ?? "nil")") }
    }

    func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
        lock.withLock { endedLog.append("\(info.service).\(info.operation):\(describe(result.error))") }
    }
}

/// Everything the hooks heard, in order, one line each.
final class Transcript: HeyHooks, @unchecked Sendable {
    private let lock = NSLock()
    private var entries: [String] = []

    var log: [String] { lock.withLock { entries } }

    func clear() { lock.withLock { entries.removeAll() } }

    func onOperationStart(_ info: OperationInfo) {
        append("start:\(info.service).\(info.operation):\(info.resourceType):\(info.isMutation):\(info.resourceId.map(String.init) ?? "nil")")
    }

    func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
        append("end:\(info.operation):\(describe(result.error))")
    }

    func onRequestStart(_ info: RequestInfo) {
        append("request:\(info.method):\(info.attempt)")
    }

    func onRequestEnd(_ info: RequestInfo, result: RequestResult) {
        append("response:\(result.statusCode):\(describe(result.error))")
    }

    func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration) {
        append("retry:\(attempt):\(describe(error))")
    }

    private func append(_ entry: String) { lock.withLock { entries.append(entry) } }
}

/// An SDK error by its code, anything else by its description, nothing as `nil`.
func describe(_ error: (any Error)?) -> String {
    guard let error else { return "nil" }
    if let error = error as? HeyError { return error.code }
    return String(describing: error)
}

/// A form body as the pairs it carries, in order, since a name may repeat for a list.
func formPairs(_ body: String) -> [(String, String)] {
    body.split(separator: "&").map { pair in
        let parts = pair.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
        let decode = { (text: Substring) in
            String(text).replacingOccurrences(of: "+", with: " ").removingPercentEncoding ?? String(text)
        }
        return (decode(parts[0]), parts.count > 1 ? decode(parts[1]) : "")
    }
}

/// The first value a form body carries under `name`.
func formValue(_ body: String, _ name: String) -> String? {
    formPairs(body).first { $0.0 == name }?.1
}

/// Every value a form body carries under `name`.
func formValues(_ body: String, _ name: String) -> [String] {
    formPairs(body).filter { $0.0 == name }.map(\.1)
}

/// A JSON body parsed into Foundation values, for asserting on its members.
func jsonObject(_ body: String) throws -> [String: Any] {
    try XCTUnwrap(try JSONSerialization.jsonObject(with: Data(body.utf8)) as? [String: Any])
}

/// Asserts that `body` throws a `HeyError`, and hands it back.
@discardableResult
func assertThrowsHeyError<T>(
    _ body: @autoclosure () async throws -> T, file: StaticString = #filePath, line: UInt = #line
) async -> HeyError? {
    do {
        _ = try await body()
        XCTFail("expected a HeyError", file: file, line: line)
        return nil
    } catch let error as HeyError {
        return error
    } catch {
        XCTFail("expected a HeyError, got \(error)", file: file, line: line)
        return nil
    }
}

/// Asserts that `body` throws a `HeyError` with the given code, and hands it back.
@discardableResult
func assertThrows<T>(
    _ code: String, _ body: @autoclosure () async throws -> T, file: StaticString = #filePath, line: UInt = #line
) async -> HeyError? {
    guard let error = await assertThrowsHeyError(try await body(), file: file, line: line) else { return nil }
    XCTAssertEqual(error.code, code, "\(error)", file: file, line: line)
    return error
}

/// Asserts that a synchronous `body` throws a `HeyError` with the given code, and hands it back.
@discardableResult
func assertThrowsSync<T>(_ code: String, _ body: () throws -> T, file: StaticString = #filePath, line: UInt = #line) -> HeyError? {
    do {
        _ = try body()
        XCTFail("expected a HeyError", file: file, line: line)
        return nil
    } catch let error as HeyError {
        XCTAssertEqual(error.code, code, "\(error)", file: file, line: line)
        return error
    } catch {
        XCTFail("expected a HeyError, got \(error)", file: file, line: line)
        return nil
    }
}
