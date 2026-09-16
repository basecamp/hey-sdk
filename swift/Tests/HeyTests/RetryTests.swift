import Foundation
import XCTest

@testable import Hey

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// A transport failure that describes itself plainly.
struct TransportFailure: Error, CustomStringConvertible {
    let description: String
}

/// Hooks that record each retry's attempt and wait.
final class RetryLog: HeyHooks, @unchecked Sendable {
    private let lock = NSLock()
    private var entries: [(Int, Duration)] = []

    var attempts: [Int] { lock.withLock { entries.map(\.0) } }
    var waits: [Duration] { lock.withLock { entries.map(\.1) } }

    func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration) {
        lock.withLock { entries.append((attempt, delay)) }
    }
}

final class RetryTests: XCTestCase {
    private let message = CreateMessageRequestContent(actingSenderId: 1, message: MessagePayload(subject: "Hello", content: "World"))

    private func total(_ sleeps: [Duration]) -> Duration { sleeps.reduce(.zero, +) }

    func testAReadIsResentOnTheStatusesItsPolicyNames() async throws {
        let hey = mockHey(status(503), status(503), ok("[]"))
        let retries = RetryLog()
        _ = try await hey.client(hooks: retries).boxes.list()
        XCTAssertEqual(hey.requests.count, 3)
        XCTAssertEqual(retries.attempts, [2, 3])
        XCTAssertEqual(hey.sleeps, [.seconds(1), .seconds(2)], "1s then 2s of backoff")
    }

    func testAMutationIsSentOnce() async throws {
        let hey = mockHey(status(503))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client().messages.create(body: message))
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(error?.httpStatus, 503)
        XCTAssertEqual(error?.isRetryable, true)
    }

    func testAPutTheModelCallsNotIdempotentIsSentOnce() async throws {
        let hey = mockHey(status(503))
        await assertThrows(HeyError.codeAPI, try await hey.client().messages.update(messageId: 456, body: message))
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testAPostTheModelCallsIdempotentIsResent() async throws {
        let hey = mockHey(status(503), ok(#"{"id":12345,"type":"Calendar::Todo"}"#))
        _ = try await hey.client().calendarTodos.complete(todoId: 12345)
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testTheClientCeilingOnlyLowersThePolicy() async throws {
        let hey = mockHey(status(503), status(503), ok("[]"))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client { $0.maxRetries = 1 }.boxes.list())
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(error?.httpStatus, 503)

        let raised = mockHey(status(503), status(503), status(503), ok(""))
        await assertThrows(HeyError.codeAPI, try await raised.client { $0.maxRetries = 5 }.extenzions.delete(accountId: 1, extenzionId: 2))
        XCTAssertEqual(raised.requests.count, 2, "DeleteExtenzion's policy allows two sends however high the client's ceiling")
    }

    func testAStatusThePolicyDoesNotNameIsNotResent() async throws {
        let hey = mockHey(status(502), ok("[]"))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client().boxes.list())
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(error?.httpStatus, 502)
    }

    func testARetryAfterOnA429IsHonouredAsGiven() async throws {
        let hey = mockHey(status(429, nil, [("Retry-After", "2")]), ok(#"{"id":1,"kind":"imbox","name":"Imbox"}"#))
        _ = try await hey.client().boxes.get(boxId: 1)
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.sleeps, [.seconds(2)])
    }

    func testTheRateLimitErrorSurvivesTheBudget() async throws {
        let hey = mockHey(status(429), status(429), status(429), status(429))
        let error = await assertThrows(HeyError.codeRateLimit, try await hey.client().boxes.get(boxId: 1))
        XCTAssertEqual(hey.requests.count, 3)
        XCTAssertEqual(error?.httpStatus, 429)
        XCTAssertEqual(error?.isRetryable, true)
    }

    func testThePolicyBaseDelayHoldsUnderAClientAskingForLess() async throws {
        let hey = mockHey(status(503), ok("[]"))
        _ = try await hey.client { $0.baseRetryDelay = .milliseconds(1) }.boxes.list()
        XCTAssertEqual(hey.sleeps, [.seconds(1)])
    }

    func testAClientBaseDelayLongerThanThePolicyIsUsed() async throws {
        let hey = mockHey(status(503), ok("[]"))
        _ = try await hey.client { $0.baseRetryDelay = .milliseconds(5000) }.boxes.list()
        XCTAssertEqual(hey.sleeps, [.seconds(5)])
    }

    func testTheWaitNeverRunsPastTheMostTheClientWaits() async throws {
        let hey = mockHey(status(503), status(503), ok("[]"))
        _ = try await hey.client {
            $0.baseRetryDelay = .seconds(20)
            $0.maxRetryDelay = .seconds(3)
            $0.maxRetryJitter = .milliseconds(500)
        }.boxes.list()
        XCTAssertEqual(hey.sleeps.count, 2)
        XCTAssertTrue(hey.sleeps.allSatisfy { $0 <= .seconds(3) }, "\(hey.sleeps)")
    }

    func testANetworkFailureIsResentForAnIdempotentOperation() async throws {
        let hey = mockHey(failure(TransportFailure(description: "connection reset")), ok("[]"))
        _ = try await hey.client().boxes.list()
        XCTAssertEqual(hey.requests.count, 2)

        let mutation = mockHey(failure(TransportFailure(description: "connection reset")))
        let error = await assertThrows(HeyError.codeNetwork, try await mutation.client().messages.create(body: message))
        XCTAssertEqual(mutation.requests.count, 1)
        XCTAssertEqual(error?.hint, "connection reset")
        XCTAssertEqual(error?.isRetryable, true)
    }

    func testATransportFailureNamesNoMoreOfAURLThanItsOrigin() async throws {
        let hey = mockHey(failure(TransportFailure(description: "could not reach https://storage.example.com/blob?signature=distinctive-secret")))
        let error = await assertThrows(HeyError.codeNetwork, try await hey.client().messages.create(body: message))
        XCTAssertEqual(error?.hint, "could not reach https://storage.example.com")
    }

    func testAClientErrorIsNotResent() async throws {
        let hey = mockHey(status(404, #"{"error":"Not found"}"#))
        await assertThrows(HeyError.codeNotFound, try await hey.client().boxes.get(boxId: 99999))
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testATimeoutIsResentLikeAnyOtherFailureToGetAnAnswer() async throws {
        let hey = mockHey(failure(URLError(.timedOut)), ok("[]"))
        _ = try await hey.client().boxes.list()
        XCTAssertEqual(hey.requests.count, 2)

        let exhausted = mockHey(failure(URLError(.timedOut)), failure(URLError(.timedOut)), failure(URLError(.timedOut)))
        let error = await assertThrows(HeyError.codeNetwork, try await exhausted.client().boxes.list())
        XCTAssertEqual(error?.isRetryable, true, "still retryable once the budget is spent: the caller may try again")
        XCTAssertEqual(exhausted.requests.count, 3)
    }

    func testAPatchTheModelCallsIdempotentIsResent() async throws {
        let hey = mockHey(status(503), ok(""))
        let client = try hey.client()
        var operation = try client.operation(Routes.updateSticky, [1])
        operation.jsonBody(Data("{}".utf8))
        try await client.sendVoid(operation)
        XCTAssertEqual(hey.requests.count, 2, "UpdateSticky is a PATCH the model calls idempotent")
        XCTAssertTrue(Routes.updateSticky.idempotent)
        XCTAssertFalse(Routes.updateMessage.idempotent, "and UpdateMessage's override still stands")
    }

    func testARetryCeilingAsHighAsAnIntHoldsDoesNotWrap() async throws {
        let hey = mockHey(status(503), ok("[]"))
        _ = try await hey.client { $0.maxRetries = Int.max }.boxes.list()
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testARetryAfterOfZeroMeansNow() async throws {
        let hey = mockHey(
            status(429, nil, [("Retry-After", "0")]),
            status(429, nil, [("Retry-After", "Thu, 01 Jan 2015 00:00:00 GMT")]),
            ok("[]"))
        _ = try await hey.client().boxes.list()
        XCTAssertEqual(hey.requests.count, 3)
        XCTAssertEqual(total(hey.sleeps), .zero, "neither a literal zero nor a date already past is a reason to wait the backoff")
    }

    func testARetryAfterOnA503IsHonouredToo() async throws {
        let hey = mockHey(status(503, nil, [("Retry-After", "2")]), ok("[]"))
        let retries = RetryLog()
        _ = try await hey.client(hooks: retries).boxes.list()
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(retries.waits, [.seconds(2)], "the outage window HEY named is the wait")
        XCTAssertEqual(hey.sleeps, [.seconds(2)])
    }

    func testANegativeRetryAfterIsNoWaitHEYNamed() async throws {
        XCTAssertNil(retryAfterSeconds("-1"))
        XCTAssertEqual(retryAfterSeconds("0"), 0)
        XCTAssertNil(retryAfterSeconds("soon"))
        XCTAssertNil(retryAfterSeconds("99999999999999999999999"))
        let future = Date().addingTimeInterval(90)
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(identifier: "GMT")
        formatter.dateFormat = "EEE, dd MMM yyyy HH:mm:ss 'GMT'"
        let seconds = try XCTUnwrap(retryAfterSeconds(formatter.string(from: future)))
        XCTAssertTrue((88...91).contains(seconds), "\(seconds)")

        let hey = mockHey(status(503, nil, [("Retry-After", "-1")]), ok("[]"))
        let retries = RetryLog()
        _ = try await hey.client(hooks: retries).boxes.list()
        XCTAssertEqual(retries.waits, [.seconds(1)], "the backoff the policy names, not a wait of nothing")
    }

    func testARawPathIsResentOnTheClientsOwnTerms() async throws {
        let hey = mockHey(status(500), status(502), status(504), ok("{}"))
        let client = try hey.client { $0.maxRetries = 3 }
        try await client.execute(client.request(.get, "/anything"))
        XCTAssertEqual(hey.requests.count, 4, "a path the model does not cover is resent on the statuses the client names, up to its ceiling")
        XCTAssertEqual(hey.sleeps, [.seconds(1), .seconds(2), .seconds(4)])
        let post = mockHey(status(503))
        let posting = try post.client()
        await assertThrows(HeyError.codeAPI, try await posting.execute(posting.request(.post, "/anything")))
        XCTAssertEqual(post.requests.count, 1, "and a POST it wrote is sent once")
    }

    func testRetriesOffSendsEverythingOnce() async throws {
        let hey = mockHey(status(503), ok("[]"))
        await assertThrows(HeyError.codeAPI, try await hey.client { $0.enableRetry = false }.boxes.list())
        XCTAssertEqual(hey.requests.count, 1)
    }
}
