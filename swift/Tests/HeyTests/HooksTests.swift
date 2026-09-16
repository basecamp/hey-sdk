import Foundation
import XCTest

@testable import Hey

/// A provider whose token cannot be had.
struct SealedProvider: TokenProvider {
    func accessToken() async throws -> String { throw TransportFailure(description: "vault sealed") }
}

/// A provider whose refresh throws.
struct ExplodingProvider: TokenProvider {
    func accessToken() async throws -> String { "stale" }
    func refresh() async throws -> Bool { throw TransportFailure(description: "refresh exploded") }
}

/// A clock that moves only when a test moves it, and never back.
final class HandClock: @unchecked Sendable {
    private let lock = NSLock()
    private var current: Duration = .zero

    var now: Duration {
        get { lock.withLock { current } }
        set { lock.withLock { current = newValue } }
    }

    func advance(_ by: Duration) { lock.withLock { current += by } }

    var reading: @Sendable () -> Duration { { [self] in now } }
}

final class HooksTests: XCTestCase {
    func testAnOperationAndItsRequestsAreReported() async throws {
        let hey = mockHey(status(503), ok(#"{"id":5,"kind":"imbox","name":"Imbox"}"#), status(404))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        _ = try await client.boxes.get(boxId: 5)
        XCTAssertEqual(transcript.log, [
            "start:Boxes.GetBox:box:false:5",
            "request:GET:1",
            "response:503:api_error",
            "retry:2:api_error",
            "request:GET:2",
            "response:200:nil",
            "end:GetBox:nil",
        ])
        transcript.clear()
        await assertThrows(HeyError.codeNotFound, try await client.boxes.get(boxId: 6))
        XCTAssertEqual(transcript.log.suffix(2), ["response:404:not_found", "end:GetBox:not_found"])
    }

    func testChainedHooksNest() async throws {
        final class Sink: @unchecked Sendable {
            let lock = NSLock()
            var entries: [String] = []
            func add(_ entry: String) { lock.withLock { entries.append(entry) } }
        }
        final class Named: HeyHooks, @unchecked Sendable {
            let name: String
            let sink: Sink
            init(_ name: String, _ sink: Sink) { self.name = name; self.sink = sink }
            func onOperationStart(_ info: OperationInfo) { sink.add("\(name):start") }
            func onOperationEnd(_ info: OperationInfo, result: OperationResult) { sink.add("\(name):end") }
            func onRequestStart(_ info: RequestInfo) { sink.add("\(name):request") }
            func onRequestEnd(_ info: RequestInfo, result: RequestResult) { sink.add("\(name):response") }
        }
        let sink = Sink()
        let hey = mockHey(ok("[]"))
        _ = try await hey.client(hooks: ChainHooks(Named("a", sink), NoopHooks(), Named("b", sink))).boxes.list()
        XCTAssertEqual(sink.entries, ["a:start", "b:start", "a:request", "b:request", "b:response", "a:response", "b:end", "a:end"])
    }

    func testATokenProviderThatThrowsStillEndsTheOperation() async throws {
        let hey = mockHey(ok("[]"))
        let transcript = Transcript()
        let client = try hey.client(auth: BearerAuth(tokenProvider: SealedProvider()), hooks: transcript)
        do {
            _ = try await client.boxes.list()
            XCTFail("expected the provider's failure")
        } catch let error as TransportFailure {
            XCTAssertEqual(error.description, "vault sealed")
        }
        XCTAssertEqual(transcript.log, ["start:Boxes.ListBoxes:box:false:nil", "end:ListBoxes:vault sealed"])
        XCTAssertEqual(hey.requests.count, 0)
    }

    func testARefreshThatThrowsStillEndsTheRequest() async throws {
        let hey = mockHey(status(401))
        let transcript = Transcript()
        let client = try hey.client(auth: BearerAuth(tokenProvider: ExplodingProvider()), hooks: transcript)
        // What the provider threw reaches the caller as an SDK failure, with it as the cause.
        let error = await assertThrows(HeyError.codeAuth, try await client.boxes.list())
        XCTAssertEqual(error?.message, "credential refresh failed")
        XCTAssertEqual((error?.detail?.cause as? TransportFailure)?.description, "refresh exploded")
        XCTAssertEqual(error?.hint, "refresh exploded")
        XCTAssertEqual(transcript.log, [
            "start:Boxes.ListBoxes:box:false:nil",
            "request:GET:1",
            "response:401:auth_required",
            "end:ListBoxes:auth_required",
        ])
    }

    func testACancelledRequestEndsWhatItStarted() async throws {
        let hey = mockHey(ok("[]"))
        let started = AsyncGate()
        hey.hold = { _, _ in
            await started.open()
            try? await Task.sleep(for: .seconds(3600))
        }
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        let task = Task { _ = try await client.boxes.list() }
        await started.wait()
        task.cancel()
        _ = await task.result
        XCTAssertEqual(transcript.log, [
            "start:Boxes.ListBoxes:box:false:nil",
            "request:GET:1",
            "response:0:network",
            "end:ListBoxes:network",
        ])
    }

    func testAnAnswerThatWillNotReadEndsTheOperationWithThatError() async throws {
        let hey = mockHey(ok(#"{"id":"not a number"}"#))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        await assertThrows(HeyError.codeAPI, try await client.boxes.get(boxId: 1))
        XCTAssertEqual(transcript.log.last, "end:GetBox:api_error")
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("end:") }.count, 1)
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("response:") }, ["response:200:nil"], "the request itself was answered")
    }

    func testAQuietRequestFiresOnlyTheRequestHooks() async throws {
        let hey = mockHey(ok("{}"))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        var operation = client.request(.get, "/thing")
        operation.quiet()
        try await client.execute(operation)
        XCTAssertEqual(transcript.log, ["request:GET:1", "response:200:nil"])
    }

    func testAnUnsignedRequestIsHeardByItsOriginAlone() async throws {
        let hey = mockHey(ok(""))
        let transcript = Transcript()
        final class URLs: HeyHooks, @unchecked Sendable {
            let lock = NSLock()
            var seen: [String] = []
            func onRequestStart(_ info: RequestInfo) { lock.withLock { seen.append(info.url) } }
        }
        let urls = URLs()
        let client = try hey.client(hooks: ChainHooks(transcript, urls))
        var put = HeyOperation.at(.put, try XCTUnwrap(URL(string: "https://storage.example.com/blobs/abc?signature=secret")))
        put.unsigned()
        try await client.execute(put)
        XCTAssertEqual(urls.seen, ["https://storage.example.com"])
        XCTAssertNil(hey.requests.first?.header("Authorization"))
    }

    func testDurationsAreMeasuredOnTheClientsOwnClock() async throws {
        let clock = HandClock()
        final class Durations: HeyHooks, @unchecked Sendable {
            let lock = NSLock()
            var entries: [String] = []
            func onOperationEnd(_ info: OperationInfo, result: OperationResult) { lock.withLock { entries.append("operation:\(result.duration)") } }
            func onRequestEnd(_ info: RequestInfo, result: RequestResult) { lock.withLock { entries.append("request:\(result.duration)") } }
        }
        let durations = Durations()
        let hey = mockHey(ok(#"{"id":5,"kind":"imbox","name":"Imbox"}"#), status(403, #"{"error":"no"}"#))
        hey.hold = { index, _ in clock.advance(index == 0 ? .milliseconds(1500) : .seconds(2)) }
        let client = try hey.client(hooks: durations, clock: clock.reading)
        _ = try await client.boxes.get(boxId: 5)
        XCTAssertEqual(durations.entries, ["request:\(Duration.milliseconds(1500))", "operation:\(Duration.milliseconds(1500))"])
        await assertThrows(HeyError.codeForbidden, try await client.boxes.get(boxId: 6))
        XCTAssertEqual(durations.entries.suffix(2), ["request:\(Duration.seconds(2))", "operation:\(Duration.seconds(2))"], "a failed operation's duration is measured the same way")
    }

    func testAnOperationOfSeveralRequestsIsOneOperation() async throws {
        let hey = mockHey(ok("{}"), status(422, #"{"error":"no"}"#))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        let info = writeInfo(service: "Things", operation: "DoTwoThings", resourceType: "thing", resourceId: 9)
        await assertThrows(HeyError.codeValidation, try await client.asOperation(info) {
            var first = client.request(.get, "/one")
            first.quiet()
            try await client.execute(first)
            var second = client.request(.post, "/two")
            second.quiet()
            try await client.execute(second)
        })
        XCTAssertEqual(transcript.log, [
            "start:Things.DoTwoThings:thing:true:9",
            "request:GET:1",
            "response:200:nil",
            "request:POST:1",
            "response:422:validation",
            "end:DoTwoThings:validation",
        ])
    }
}

/// A one-shot signal a test can wait on.
actor AsyncGate {
    private var isOpen = false
    private var waiters: [CheckedContinuation<Void, Never>] = []

    func open() {
        guard !isOpen else { return }
        isOpen = true
        for waiter in waiters { waiter.resume() }
        waiters.removeAll()
    }

    func wait() async {
        if isOpen { return }
        await withCheckedContinuation { waiters.append($0) }
    }
}
