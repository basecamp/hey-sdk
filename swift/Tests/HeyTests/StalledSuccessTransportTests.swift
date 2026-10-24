import Foundation
import XCTest

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

@testable import Hey

/// Answers every request with a success and the start of a 1000-byte body, then never sends the
/// rest, as a connection that stalls mid-body does. Counts the requests it sees.
final class StallingSuccessProtocol: URLProtocol {
    nonisolated(unsafe) static var status = 200
    private static let lock = NSLock()
    nonisolated(unsafe) private static var seen = 0

    static var requests: Int {
        lock.lock()
        defer { lock.unlock() }
        return seen
    }

    static func reset(status: Int) {
        lock.lock()
        seen = 0
        self.status = status
        lock.unlock()
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Self.lock.lock()
        Self.seen += 1
        let status = Self.status
        Self.lock.unlock()
        let response = HTTPURLResponse(
            url: request.url!, statusCode: status, httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json", "Content-Length": "1000"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(#"{"id":2201,"topic_id":"#.utf8))
    }

    override func stopLoading() {}
}

/// The transport the SDK ships, through a session whose answer stalls after a success's headers:
/// the deadline passing then is the body lost, not a failed request.
final class StalledSuccessTransportTests: XCTestCase {
    private func client() throws -> HeyClient {
        let transport = URLSessionTransport(timeout: .milliseconds(300), protocolClasses: [StallingSuccessProtocol.self])
        return try HeyClient(
            accessToken: "test-token",
            config: HeyConfig(timeout: .milliseconds(300), maxRetries: 2, baseRetryDelay: .milliseconds(1), maxRetryJitter: .zero),
            transport: transport)
    }

    func testARawWriteWhoseSuccessStallsPastTheTimeoutIsTheSuccessItWasAndIsSentOnce() async throws {
        for method in [HTTPMethod.put, .delete, .post] {
            StallingSuccessProtocol.reset(status: 200)
            let client = try client()
            let response = try await client.execute(client.request(method, "/messages/1"))
            XCTAssertEqual(response.status, 200, "\(method)")
            XCTAssertTrue(response.body.isEmpty, "\(method)")
            let bodyError = try XCTUnwrap(response.bodyError, "\(method): the timeout is attached")
            XCTAssertEqual(bodyError.code, HeyError.codeNetwork, "\(method)")
            XCTAssertFalse(bodyError.isRetryable, "\(method)")
            XCTAssertEqual(StallingSuccessProtocol.requests, 1, "\(method): a write that took effect is never resent")
        }
    }

    func testARawReadWhoseSuccessStallsIsStillAFailure() async throws {
        StallingSuccessProtocol.reset(status: 200)
        let client = try client()
        do {
            _ = try await client.execute(client.request(.get, "/topics/1"))
            XCTFail("a GET whose body stalled must fail")
        } catch let error as HeyError {
            XCTAssertEqual(error.code, HeyError.codeNetwork)
        }
    }
}
