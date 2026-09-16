import Foundation
import XCTest

@testable import Hey

/// A provider with a token and a count of refreshes, whose refresh does what the test says.
final class ScriptedProvider: TokenProvider, @unchecked Sendable {
    private let lock = NSLock()
    private var tokenValue: String
    private var refreshCount = 0
    private var signingCount = 0
    private let onRefresh: @Sendable (ScriptedProvider, Int) async throws -> Bool
    private let onSign: (@Sendable (ScriptedProvider, Int) async throws -> String)?

    init(
        token: String = "stale",
        onSign: (@Sendable (ScriptedProvider, Int) async throws -> String)? = nil,
        onRefresh: @escaping @Sendable (ScriptedProvider, Int) async throws -> Bool
    ) {
        tokenValue = token
        self.onSign = onSign
        self.onRefresh = onRefresh
    }

    var token: String {
        get { lock.withLock { tokenValue } }
        set { lock.withLock { tokenValue = newValue } }
    }

    var refreshes: Int { lock.withLock { refreshCount } }
    var signings: Int { lock.withLock { signingCount } }

    func accessToken() async throws -> String {
        let signing = lock.withLock { signingCount += 1; return signingCount }
        if let onSign { return try await onSign(self, signing) }
        return token
    }

    func refresh() async throws -> Bool {
        let refresh = lock.withLock { refreshCount += 1; return refreshCount }
        return try await onRefresh(self, refresh)
    }
}

/// Hooks that open a gate when an operation ends.
struct OnOperationEnd: HeyHooks {
    let gate: AsyncGate

    func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
        Task { await gate.open() }
    }
}

/// Holds the first two requests until both have arrived, so both were signed before either 401
/// could start a refresh, then runs `after` for each arrival before answering.
func bothOutFirst(_ hey: MockHey, after: @escaping @Sendable (Int) async -> Void = { _ in }) {
    let bothOut = AsyncGate()
    hey.hold = { index, _ in
        if index == 1 { await bothOut.open() }
        if index <= 1 { await bothOut.wait() }
        await after(index)
    }
}

/// The auth failure a result ended with, or nil.
func authFailure<T>(_ result: Result<T, any Error>) -> HeyError? {
    guard case let .failure(error as HeyError) = result, case .auth = error else { return nil }
    return error
}

final class CredentialRefreshTests: XCTestCase {
    private func renewing() -> ScriptedProvider {
        ScriptedProvider { provider, _ in
            provider.token = "refreshed"
            return true
        }
    }

    func testA401IsAnsweredByARefreshAndOneResend() async throws {
        let hey = mockHey(status(401), ok("[]"))
        let credentials = renewing()
        _ = try await hey.client(auth: BearerAuth(tokenProvider: credentials)).boxes.list()
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[0].header("Authorization"), "Bearer stale")
        XCTAssertEqual(hey.requests[1].header("Authorization"), "Bearer refreshed")
        XCTAssertEqual(credentials.refreshes, 1)
    }

    func testAMutationIsResentAfterARefreshToo() async throws {
        let hey = mockHey(status(401), ok(#"{"id":1}"#))
        let body = CreateContactRequestContent(contact: ContactPayload(name: "Jane", emailAddress: SensitiveString("jane@example.com")))
        _ = try await hey.client(auth: BearerAuth(tokenProvider: renewing())).contacts.create(body: body)
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[1].path, "/contacts.json")
        XCTAssertEqual(hey.requests[0].body, hey.requests[1].body)
    }

    func testA401ThatOutlivesTheRefreshIsSurfacedAfterTheOneResend() async throws {
        let hey = mockHey(status(401), status(401))
        let credentials = renewing()
        let error = await assertThrows(HeyError.codeAuth, try await hey.client(auth: BearerAuth(tokenProvider: credentials)).boxes.list())
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(error?.httpStatus, 401)
        XCTAssertEqual(credentials.refreshes, 1)
    }

    func testA401IsNotResentWhenTheCredentialsCannotBeRefreshed() async throws {
        let hey = mockHey(status(401))
        let credentials = ScriptedProvider { _, _ in false }
        await assertThrows(HeyError.codeAuth, try await hey.client(auth: BearerAuth(tokenProvider: credentials)).boxes.list())
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testAFormRequestIsRefreshedAndResentButNeverRetriedOtherwise() async throws {
        let hey = mockHey(status(401), status(302, nil, [("Location", "/x/1")]))
        let client = try hey.client(auth: BearerAuth(tokenProvider: renewing()))
        _ = try await client.sendForm(client.form(.post, "/calendar/events/99"))
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[1].header("Authorization"), "Bearer refreshed")

        let failing = mockHey(status(503))
        let failingClient = try failing.client()
        await assertThrows(HeyError.codeAPI, try await failingClient.sendForm(failingClient.form(.post, "/calendar/events/99")))
        XCTAssertEqual(failing.requests.count, 1)
    }

    /// A refresh that fails is one refresh: every request signed with the credentials it could not
    /// renew gets its answer, rather than asking the issuer again for the same credentials during
    /// the same outage. A request signed after the failure asks again.
    func testAFailedRefreshIsSharedByEveryRequestSignedWithTheCredentialsItWasFor() async throws {
        for throwing in [false, true] {
            let credentials = ScriptedProvider { _, _ in
                if throwing { throw TransportFailure(description: "issuer down") }
                return false
            }
            let firstFailed = AsyncGate()
            let hey = MockHey(Array(repeating: status(401), count: 3))
            // The second 401 is answered only once the first request has failed, so its 401 finds
            // the failure already recorded.
            bothOutFirst(hey) { index in if index == 1 { await firstFailed.wait() } }
            let client = try hey.client(auth: BearerAuth(tokenProvider: credentials), hooks: OnOperationEnd(gate: firstFailed))
            let a = Task { try await client.boxes.list() }
            let b = Task { try await client.boxes.list() }
            let outcomes = [await a.result, await b.result]
            XCTAssertEqual(credentials.refreshes, 1, "one refresh for the one set of credentials, throwing=\(throwing)")
            XCTAssertEqual(hey.requests.count, 2, "and no resend, throwing=\(throwing)")
            for outcome in outcomes {
                guard let error = authFailure(outcome) else {
                    return XCTFail("expected an auth failure, got \(outcome), throwing=\(throwing)")
                }
                if throwing {
                    XCTAssertEqual(error.message, "credential refresh failed")
                    XCTAssertEqual((error.detail?.cause as? TransportFailure)?.description, "issuer down", "what the refresh threw reaches each request as the cause")
                }
            }

            // A request signed after the failure earns a refresh of its own: its 401 is news.
            await assertThrows(HeyError.codeAuth, try await client.boxes.list())
            XCTAssertEqual(credentials.refreshes, 2, "throwing=\(throwing)")
            XCTAssertEqual(hey.requests.count, 3)
        }
    }

    /// A request whose 401 arrives while the failing refresh is still running joins it, and gets
    /// its answer too.
    func testARequestJoinsTheFailingRefreshInFlight() async throws {
        let started = AsyncGate()
        let release = AsyncGate()
        let credentials = ScriptedProvider { _, _ in
            await started.open()
            await release.wait()
            return false
        }
        let hey = MockHey([status(401), status(401)])
        // The second is answered once the refresh the first earned is running, and the refresh is
        // let go only after that.
        bothOutFirst(hey) { index in
            if index == 1 {
                await started.wait()
                await release.open()
            }
        }
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        let a = Task { try await client.boxes.list() }
        let b = Task { try await client.boxes.list() }
        let outcomes = [await a.result, await b.result]
        for outcome in outcomes {
            XCTAssertNotNil(authFailure(outcome), "\(outcome)")
        }
        XCTAssertEqual(credentials.refreshes, 1)
        XCTAssertEqual(hey.requests.count, 2)
    }

    /// A failed refresh leaves the credentials as they were, so a later refresh of them is a
    /// refresh of every request's still signed under them: a 401 that arrives after it has renewed
    /// them is resent with the new credentials rather than handed the old failure.
    func testALateRequestSignedBeforeAFailedRefreshIsResentOnceALaterRefreshRenewsTheCredentials() async throws {
        let credentials = ScriptedProvider { provider, refresh in
            if refresh == 1 { return false }
            provider.token = "renewed"
            return true
        }
        let aIn = AsyncGate()
        let renewed = AsyncGate()
        let hey = MockHey([status(401), status(401), status(401), ok("[]"), ok("[]")])
        bothOutFirst(hey) { index in
            // b's 401 waits until c has been through a refresh that renews the credentials b was
            // signed with.
            if index == 1 { await renewed.wait() }
        }
        let firstHold = hey.hold
        hey.hold = { index, request in
            if index == 0 { await aIn.open() }
            await firstHold?(index, request)
        }
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        let a = Task { try await client.boxes.list() }
        await aIn.wait()
        let b = Task { try await client.boxes.list() }
        let aResult = await a.result
        XCTAssertNotNil(authFailure(aResult), "a's refresh fails")
        XCTAssertEqual(credentials.refreshes, 1)
        _ = try await client.boxes.list()
        XCTAssertEqual(credentials.refreshes, 2, "c, signed after the failure, refreshes again and is resent")
        await renewed.open()
        if case let .failure(error) = await b.result { return XCTFail("b's 401 is on credentials the second refresh has since renewed, so it is resent: \(error)") }
        XCTAssertEqual(credentials.refreshes, 2, "without a refresh of its own")
        XCTAssertEqual(hey.requests.map { $0.header("Authorization") ?? "" }, ["Bearer stale", "Bearer stale", "Bearer stale", "Bearer renewed", "Bearer renewed"])
    }

    /// A provider that renews ahead of expiry hands `accessToken` a new token without being asked to
    /// refresh. A 401 on the old token, arriving after the new one has signed a request, is resent
    /// with the new one; refreshing would burn it. A 401 on the new token itself is refreshed once.
    func testATokenTheProviderRotatesOnItsOwnIsARenewalA401OnTheOldOneIsResent() async throws {
        let credentials = ScriptedProvider(
            onSign: { provider, signing in signing == 1 ? "t0" : (provider.refreshes == 0 ? "t1" : "t2") },
            onRefresh: { _, _ in true })
        // Signings are serialised; the sends after them are not, so the server answers by the token
        // each request carries rather than by the order they arrive in.
        let rejected = Locked<Set<String>>(["Bearer t0"])
        let hey = MockHey { _, request in
            rejected.withLock { $0.contains(request.header("Authorization") ?? "") } ? status(401) : ok("[]")
        }
        bothOutFirst(hey)
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        let a = Task { try await client.boxes.list() }
        let b = Task { try await client.boxes.list() }
        _ = try await a.value
        _ = try await b.value
        XCTAssertEqual(credentials.refreshes, 0, "the 401 on t0 was answered by t1, which the provider had already handed over")
        let tokens = hey.requests.map { $0.header("Authorization") ?? "" }
        XCTAssertEqual(tokens.prefix(2).sorted(), ["Bearer t0", "Bearer t1"], "\(tokens)")
        XCTAssertEqual(tokens.dropFirst(2), ["Bearer t1"], "the t0 request was resent with t1")

        rejected.withLock { $0 = ["Bearer t1"] }
        _ = try await client.boxes.list()
        XCTAssertEqual(credentials.refreshes, 1, "a 401 on t1 itself is refreshed, once")
        XCTAssertEqual(hey.requests.map { $0.header("Authorization") ?? "" }.dropFirst(3), ["Bearer t1", "Bearer t2"])
    }

    /// Before the provider is asked to refresh, it is asked what it would sign with now, and a token
    /// other than the rejected one is a renewal already made: the request is resent with it and the
    /// refresh is not spent.
    func testAProviderAskedForItsTokenBeforeARefreshIsNotAskedToRefreshWhenItHasAlreadyRenewed() async throws {
        let credentials = ScriptedProvider(
            onSign: { provider, signing in signing == 1 ? "t0" : (provider.refreshes == 0 ? "t1" : "t2") },
            onRefresh: { _, _ in true })
        let hey = mockHey(status(401), ok("[]"), status(401), ok("[]"))
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        _ = try await client.boxes.list()
        XCTAssertEqual(credentials.refreshes, 0, "the provider had renewed on its own by the time the 401 came back")
        XCTAssertEqual(hey.requests.map { $0.header("Authorization") ?? "" }, ["Bearer t0", "Bearer t1"])
        _ = try await client.boxes.list()
        XCTAssertEqual(credentials.refreshes, 1, "a 401 on the token the provider would still sign with is refreshed")
        XCTAssertEqual(hey.requests.map { $0.header("Authorization") ?? "" }, ["Bearer t0", "Bearer t1", "Bearer t1", "Bearer t2"])
    }

    /// A provider that cannot hand over a token when asked before a refresh is the refresh failing:
    /// the request gets its 401 and the provider is not asked to refresh on top.
    func testAProviderThatCannotHandOverATokenFailsTheRefreshWithoutRefreshing() async throws {
        let credentials = ScriptedProvider(
            onSign: { _, signing in
                if signing == 1 { return "t0" }
                throw TransportFailure(description: "the token could not be renewed")
            },
            onRefresh: { _, _ in true })
        let hey = mockHey(status(401))
        let error = await assertThrows(HeyError.codeAuth, try await hey.client(auth: BearerAuth(tokenProvider: credentials)).boxes.list())
        XCTAssertEqual(error?.message, "credential refresh failed")
        XCTAssertEqual(credentials.refreshes, 0)
        XCTAssertEqual(hey.requests.count, 1)
    }

    /// Only the SDK's own bearer strategy is read for a renewal: a strategy of the caller's that
    /// signs every request differently is asked to refresh after a 401, once.
    func testAStrategyOfTheCallersThatSignsEveryRequestDifferentlyIsNotTakenForRenewing() async throws {
        final class Distinct: AuthStrategy, @unchecked Sendable {
            let lock = NSLock()
            var signings = 0
            var refreshes = 0
            func authenticate(_ request: inout HTTPRequest) async throws {
                let (signing, refreshed) = lock.withLock { signings += 1; return (signings, refreshes) }
                request.headers.set("Authorization", "Bearer \(refreshed == 0 ? "signature" : "renewed")-\(signing)")
            }
            func refresh() async throws -> Bool {
                lock.withLock { refreshes += 1 }
                return true
            }
        }
        let strategy = Distinct()
        // Answered by what each request carries, since the two signed at once may arrive in either order.
        let hey = MockHey { _, request in request.header("Authorization") == "Bearer signature-1" ? status(401) : ok("[]") }
        bothOutFirst(hey)
        let client = try hey.client(auth: strategy)
        let a = Task { try await client.boxes.list() }
        let b = Task { try await client.boxes.list() }
        _ = try await a.value
        _ = try await b.value
        XCTAssertEqual(strategy.lock.withLock { strategy.refreshes }, 1, "the differing signature was not taken for a renewal")
        let signatures = hey.requests.map { $0.header("Authorization") ?? "" }
        XCTAssertEqual(signatures.prefix(2).sorted(), ["Bearer signature-1", "Bearer signature-2"], "\(signatures)")
        XCTAssertEqual(signatures.dropFirst(2), ["Bearer renewed-3"])
    }

    /// A request is signed under the refresh lock, so a refresh cannot land between the signing and
    /// the count that says which credentials went out.
    func testSigningAndRefreshingNeverInterleave() async throws {
        let gate = AsyncGate()
        final class Inside: @unchecked Sendable {
            let lock = NSLock()
            var inside = 0
            var most = 0
        }
        let inside = Inside()
        let credentials = ScriptedProvider(
            onSign: { provider, signing in
                inside.lock.withLock { inside.inside += 1; inside.most = max(inside.most, inside.inside) }
                if signing == 1 { await gate.wait() }
                inside.lock.withLock { inside.inside -= 1 }
                return provider.token
            },
            onRefresh: { provider, _ in
                provider.token = "refreshed"
                return true
            })
        let hey = MockHey([status(401), status(401), ok("[]"), ok("[]")])
        bothOutFirst(hey)
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        let a = Task { try await client.boxes.list() }
        let b = Task { try await client.boxes.list() }
        try await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(credentials.signings, 1, "the second request waits for the first to be signed")
        await gate.open()
        _ = try await a.value
        _ = try await b.value
        XCTAssertEqual(inside.lock.withLock { inside.most }, 1, "one request is signed at a time")
        XCTAssertEqual(hey.requests.map { $0.header("Authorization") ?? "" }, ["Bearer stale", "Bearer stale", "Bearer refreshed", "Bearer refreshed"])
    }

    /// A hop signed again after someone else's refresh carries the new credentials, so a 401 on it
    /// is about those, and earns the refresh it deserves rather than a bare resend.
    func testA401OnAHopSignedAfterARefreshIsRefreshedAgain() async throws {
        let credentials = ScriptedProvider(token: "t0") { provider, refresh in
            provider.token = "t\(refresh)"
            return true
        }
        let firstAsked = AsyncGate()
        let firstAnswer = AsyncGate()
        final class Visits: @unchecked Sendable {
            let lock = NSLock()
            var counts: [String: Int] = [:]
            func visit(_ path: String) -> Int { lock.withLock { counts[path, default: 0] += 1; return counts[path]! } }
        }
        let visits = Visits()
        final class Routed: Transport, @unchecked Sendable {
            let inner: MockHey
            let answer: @Sendable (String, Int) async -> Answer
            init(_ inner: MockHey, _ answer: @escaping @Sendable (String, Int) async -> Answer) { self.inner = inner; self.answer = answer }
            func send(_ request: HTTPRequest, bodyLimit: @escaping @Sendable (Int, HTTPHeaders) -> Int?) async throws -> HTTPResponse {
                let path = URLComponents(url: request.url, resolvingAgainstBaseURL: true)?.percentEncodedPath ?? ""
                _ = try await inner.send(request) { _, _ in 0 }
                let canned = await answer(path, inner.requests.count)
                return HTTPResponse(status: canned.status, headers: HTTPHeaders(canned.headers + [("Content-Type", "application/json")]), body: Data((canned.body ?? "").utf8))
            }
        }
        let recorder = MockHey([])
        let transport = Routed(recorder) { path, _ in
            let visit = visits.visit(path)
            if path == "/a.json" {
                await firstAsked.open()
                await firstAnswer.wait()
                return status(302, nil, [("Location", "/a2.json")])
            }
            return visit == 1 ? status(401) : ok("[]")
        }
        let client = try HeyClient(
            auth: BearerAuth(tokenProvider: credentials), config: HeyConfig(timeout: nil, maxRetryJitter: .zero), hooks: nil,
            transport: transport, cache: nil, clock: ContinuousClock().monotonicNow, sleeper: { _ in })
        let a = Task { try await client.execute(client.request(.get, "/a")) }
        await firstAsked.wait()
        try await client.execute(client.request(.get, "/b"))
        XCTAssertEqual(credentials.refreshes, 1, "b's 401 refreshed while a's first answer was still to come")
        await firstAnswer.open()
        _ = try await a.value
        XCTAssertEqual(credentials.refreshes, 2, "a's hop went out with the refreshed credentials, and their 401 is refreshed again")
        XCTAssertEqual(
            recorder.requests.map { "\($0.path) \($0.header("Authorization") ?? "")" },
            ["/a.json Bearer t0", "/b.json Bearer t0", "/b.json Bearer t1", "/a2.json Bearer t1", "/a.json Bearer t2", "/a2.json Bearer t2"],
            "the resend starts the operation over from the URL asked for, through the redirect again")
    }

    /// A refresh belongs to the client: the request that earned it being cancelled stops waiting and
    /// leaves the refresh to finish, and the next stale request is signed with what it renewed
    /// rather than starting one of its own.
    func testACancelledRequestDoesNotCancelTheRefreshItStarted() async throws {
        let refreshing = AsyncGate()
        let gate = AsyncGate()
        let credentials = ScriptedProvider { provider, _ in
            await refreshing.open()
            await gate.wait()
            provider.token = "renewed"
            return true
        }
        let hey = mockHey(status(401), ok("[]"))
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        let first = Task { try await client.boxes.list() }
        await refreshing.wait()
        first.cancel()
        guard case .failure(let error) = await first.result, error is CancellationError else {
            return XCTFail("the cancelled request stops waiting as soon as it is cancelled")
        }
        XCTAssertEqual(hey.requests.count, 1)
        let second = Task { try await client.boxes.list() }
        try await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(hey.requests.count, 1, "the second request waits to be signed until the refresh is done")
        await gate.open()
        _ = try await second.value
        XCTAssertEqual(credentials.refreshes, 1, "the refresh the cancelled request started is the only one")
        XCTAssertEqual(hey.requests[1].header("Authorization"), "Bearer renewed")
        XCTAssertEqual(hey.requests.count, 2)
    }

    /// A request cancelled while it waits for its turn to be signed stops waiting, unsigned and
    /// unsent, rather than waiting out the refresh it will not use.
    func testARequestCancelledWhileWaitingToBeSignedIsNeverSent() async throws {
        let refreshing = AsyncGate()
        let gate = AsyncGate()
        let credentials = ScriptedProvider { provider, _ in
            await refreshing.open()
            await gate.wait()
            provider.token = "renewed"
            return true
        }
        let hey = mockHey(status(401), ok("[]"))
        let client = try hey.client(auth: BearerAuth(tokenProvider: credentials))
        let stale = Task { try await client.boxes.list() }
        await refreshing.wait()
        let waiting = Task { try await client.boxes.list() }
        try await Task.sleep(for: .milliseconds(50))
        waiting.cancel()
        guard case .failure(let error) = await waiting.result, error is CancellationError else {
            return XCTFail("the waiting request is refused as soon as it is cancelled")
        }
        XCTAssertEqual(credentials.signings, 2, "the stale request's signing and the probe before its refresh: the cancelled one was never signed")
        await gate.open()
        _ = try await stale.value
        XCTAssertEqual(credentials.signings, 3, "and the stale request's resend")
        XCTAssertEqual(hey.requests.count, 2, "the cancelled request was never sent")
    }

    func testACredentialThatIsNotAHeaderValueIsRefusedWithoutBeingQuoted() async throws {
        let transcript = Transcript()
        let hey = mockHey(ok("[]"))
        let bearer = try hey.client(auth: BearerAuth(tokenProvider: try StaticTokenProvider("secret\u{0B}token")), hooks: transcript)
        let refusedToken = await assertThrows(HeyError.codeAuth, try await bearer.boxes.list())
        XCTAssertEqual(hey.requests.count, 0)

        let cookie = try hey.client(auth: HeaderAuth(name: "Cookie", value: "session=\u{01}abc"), hooks: transcript)
        let refusedCookie = await assertThrows(HeyError.codeAuth, try await cookie.boxes.list())
        XCTAssertEqual(hey.requests.count, 0)
        for rendered in [String(describing: refusedToken), String(describing: refusedCookie), transcript.log.joined()] {
            XCTAssertFalse(rendered.contains("secret") || rendered.contains("abc"), rendered)
        }
    }
}
