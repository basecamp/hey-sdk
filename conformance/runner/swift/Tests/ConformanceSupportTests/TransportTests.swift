import ConformanceSupport
import Foundation
import Hey
import XCTest

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// The library's own transport, driven over a real loopback connection.
final class TransportTests: XCTestCase {
    private func url(_ server: LoopbackServer, _ path: String) -> URL {
        URL(string: server.baseURL + path)!
    }

    func testAnAnswerComesBackWithItsStatusHeadersAndBody() async throws {
        let server = try LoopbackServer { _, request in
            ServedResponse(status: 201, headers: [("X-Total-Count", "3"), ("Link", "</next>; rel=\"next\"")], body: Data(#"{"ok":true}"#.utf8))
        }
        defer { server.stop() }
        let transport = URLSessionTransport(timeout: .seconds(10))
        var request = HTTPRequest(method: "POST", url: url(server, "/things.json?a=1%2B1"))
        request.headers.set("Content-Type", "application/json")
        request.headers.set("X-Custom", "value")
        let body = Data(String(repeating: "x", count: 5000).utf8)
        request.body = body
        let response = try await transport.send(request) { _, _ in 1 << 20 }
        XCTAssertEqual(response.status, 201)
        XCTAssertEqual(response.headers["x-total-count"], "3")
        XCTAssertEqual(String(decoding: response.body, as: UTF8.self), #"{"ok":true}"#)
        XCTAssertFalse(response.bodyExceeded)
        let seen = try XCTUnwrap(server.requests.first)
        XCTAssertEqual(seen.method, "POST")
        XCTAssertEqual(seen.path, "/things.json")
        XCTAssertEqual(seen.query, "a=1%2B1", "the query goes out as the client encoded it")
        XCTAssertEqual(seen.header("X-Custom"), "value")
        XCTAssertEqual(seen.body, body, "a body large enough for an Expect: 100-continue still arrives whole")
    }

    func testARedirectIsHandedBackRatherThanFollowed() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 302, headers: [("Location", "/elsewhere")]) }
        defer { server.stop() }
        let response = try await URLSessionTransport().send(HTTPRequest(method: "GET", url: url(server, "/start"))) { _, _ in 1024 }
        XCTAssertEqual(response.status, 302)
        XCTAssertEqual(response.headers["Location"], "/elsewhere")
        XCTAssertEqual(server.requests.count, 1, "the transport never follows it")
    }

    func testABodyPastTheLimitIsCutOffAndSaidToBe() async throws {
        let big = Data(repeating: 0x31, count: 4 << 20)
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: big) }
        defer { server.stop() }
        let limit = 100 * 1024
        let response = try await URLSessionTransport().send(HTTPRequest(method: "GET", url: url(server, "/big"))) { _, _ in limit }
        XCTAssertTrue(response.bodyExceeded)
        XCTAssertLessThanOrEqual(response.body.count, limit + 1, "no more than one byte past the limit is held")
    }

    func testABodyAtTheLimitIsReadWhole() async throws {
        let exact = Data(repeating: 0x31, count: 4096)
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: exact) }
        defer { server.stop() }
        let response = try await URLSessionTransport().send(HTTPRequest(method: "GET", url: url(server, "/exact"))) { _, _ in 4096 }
        XCTAssertFalse(response.bodyExceeded)
        XCTAssertEqual(response.body, exact)
    }

    func testABodyTheClientDoesNotWantIsNotHeld() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 404, body: Data("nothing here".utf8)) }
        defer { server.stop() }
        let response = try await URLSessionTransport().send(HTTPRequest(method: "GET", url: url(server, "/gone"))) { _, _ in nil }
        XCTAssertEqual(response.status, 404)
        XCTAssertTrue(response.body.isEmpty)
    }

    func testACookieIsNeitherKeptNorSentBack() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, headers: [("Set-Cookie", "session=abc; Path=/")], body: Data("{}".utf8)) }
        defer { server.stop() }
        let transport = URLSessionTransport()
        _ = try await transport.send(HTTPRequest(method: "GET", url: url(server, "/one"))) { _, _ in 1024 }
        _ = try await transport.send(HTTPRequest(method: "GET", url: url(server, "/two"))) { _, _ in 1024 }
        XCTAssertNil(server.requests.last?.header("Cookie"))
    }

    func testARequestThatTakesTooLongTimesOut() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: Data("{}".utf8), delay: .seconds(3)) }
        defer { server.stop() }
        let started = ContinuousClock.now
        do {
            _ = try await URLSessionTransport(timeout: .milliseconds(300)).send(HTTPRequest(method: "GET", url: url(server, "/slow"))) { _, _ in 1024 }
            XCTFail("expected a timeout")
        } catch let error as URLError {
            XCTAssertEqual(error.code, .timedOut)
        }
        let waited = ContinuousClock.now - started
        XCTAssertGreaterThanOrEqual(waited, .milliseconds(250), "it waited for the timeout rather than failing at once")
        XCTAssertLessThan(waited, .seconds(2))
    }

    func testATimeoutOfMoreThanASecondIsKeptToTheMillisecond() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: Data("{}".utf8), delay: .seconds(4)) }
        defer { server.stop() }
        let started = ContinuousClock.now
        do {
            _ = try await URLSessionTransport(timeout: .milliseconds(1600)).send(HTTPRequest(method: "GET", url: url(server, "/slow"))) { _, _ in 1024 }
            XCTFail("expected a timeout")
        } catch let error as URLError {
            XCTAssertEqual(error.code, .timedOut)
        }
        let waited = ContinuousClock.now - started
        XCTAssertGreaterThanOrEqual(waited, .milliseconds(1550), "not rounded down to the second")
        XCTAssertLessThan(waited, .seconds(3))
    }

    func testARequestWithNoTimeoutWaitsForItsAnswer() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: Data("{}".utf8), delay: .milliseconds(1200)) }
        defer { server.stop() }
        let response = try await URLSessionTransport(timeout: nil).send(HTTPRequest(method: "GET", url: url(server, "/slow"))) { _, _ in 1024 }
        XCTAssertEqual(response.status, 200)
        XCTAssertEqual(response.body, Data("{}".utf8))
    }

    func testAnAnswerInsideItsTimeoutIsNotCutOff() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: Data("{}".utf8), delay: .milliseconds(100)) }
        defer { server.stop() }
        let transport = URLSessionTransport(timeout: .milliseconds(600))
        for _ in 0..<3 {
            let response = try await transport.send(HTTPRequest(method: "GET", url: url(server, "/soon"))) { _, _ in 1024 }
            XCTAssertEqual(response.status, 200)
        }
        // The deadlines of answered requests are called off, not left to fire at a later one.
        try await Task.sleep(for: .milliseconds(700))
        let response = try await transport.send(HTTPRequest(method: "GET", url: url(server, "/later"))) { _, _ in 1024 }
        XCTAssertEqual(response.status, 200)
    }

    func testACancelledRequestStopsAtOnce() async throws {
        let server = try LoopbackServer { _, _ in ServedResponse(status: 200, body: Data("{}".utf8), delay: .seconds(3)) }
        defer { server.stop() }
        let transport = URLSessionTransport()
        let target = url(server, "/slow")
        let task = Task { try await transport.send(HTTPRequest(method: "GET", url: target)) { _, _ in 1024 } }
        try await Task.sleep(for: .milliseconds(200))
        let started = ContinuousClock.now
        task.cancel()
        let result = await task.result
        XCTAssertLessThan(ContinuousClock.now - started, .seconds(2))
        guard case let .failure(error) = result else { return XCTFail("expected a cancellation") }
        XCTAssertTrue(error is CancellationError || (error as? URLError)?.code == .cancelled, "\(error)")
    }

    func testTheClientTalksToALoopbackHEY() async throws {
        let server = try LoopbackServer { index, _ in
            index == 0
                ? ServedResponse(status: 503, headers: [("Retry-After", "0")])
                : ServedResponse(status: 200, headers: [("X-Total-Count", "1")], body: Data(#"[{"id":7,"kind":"imbox","name":"Imbox"}]"#.utf8))
        }
        defer { server.stop() }
        let client = try HeyClient(accessToken: "loopback-token", config: HeyConfig(baseURL: server.baseURL))
        let page = try await client.boxes.list()
        XCTAssertEqual(page.value.map(\.id), [7])
        XCTAssertEqual(page.totalCount, 1)
        XCTAssertEqual(server.requests.count, 2, "resent once after the 503")
        XCTAssertEqual(server.requests.last?.path, "/boxes.json")
        XCTAssertEqual(server.requests.last?.header("Authorization"), "Bearer loopback-token")
        XCTAssertEqual(server.requests.last?.header("Accept"), "application/json")
        XCTAssertEqual(server.requests.last?.header("User-Agent"), HeyConfig.defaultUserAgent)
    }
}
