import Foundation
import XCTest

@testable import Hey

/// A strategy that signs with headers of its own naming, as a custom `AuthStrategy` may.
struct APIKeyAuth: AuthStrategy {
    func authenticate(_ request: inout HTTPRequest) async throws {
        request.headers.set("X-Api-Key", "key-123")
        request.headers.set("Authorization", "Bearer secret")
        request.headers.set("Cookie", "session=abc")
    }
}

/// A strategy that signs the method and path, as an HMAC scheme does: a hop to another URL needs
/// a signature of its own.
struct SigningAuth: AuthStrategy {
    func authenticate(_ request: inout HTTPRequest) async throws {
        let path = URLComponents(url: request.url, resolvingAgainstBaseURL: true)?.percentEncodedPath ?? ""
        request.headers.set("X-Signature", "\(request.method) \(path)")
    }
}

/// A provider that counts its refreshes and renews the token each time.
final class CountingProvider: TokenProvider, @unchecked Sendable {
    private let lock = NSLock()
    private var count = 0
    private let renews: Bool

    init(renews: Bool = true) {
        self.renews = renews
    }

    var refreshes: Int { lock.withLock { count } }

    func accessToken() async throws -> String {
        lock.withLock { "token-\(count)" }
    }

    func refresh() async throws -> Bool {
        lock.withLock { count += 1 }
        return renews
    }
}

final class RedirectTests: XCTestCase {
    func testAHopToAnotherOriginGoesOutWithoutAnyHeaderTheStrategySet() async throws {
        let hey = mockHey(status(302, nil, [("Location", "https://files.example.com/boxes.json")]), ok("[]"))
        _ = try await hey.client(auth: APIKeyAuth()).boxes.list()

        XCTAssertEqual(hey.requests.count, 2)
        let hop = hey.requests[1]
        XCTAssertEqual(hop.url.host, "files.example.com")
        XCTAssertNil(hop.header("X-Api-Key"))
        XCTAssertNil(hop.header("Authorization"))
        XCTAssertNil(hop.header("Cookie"))
        XCTAssertEqual(hop.header("Accept"), "application/json")
        XCTAssertEqual(hop.header("User-Agent"), HeyConfig.defaultUserAgent)
    }

    func testAHopOnTheSameOriginKeepsThem() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/boxes/all.json")]), ok("[]"))
        _ = try await hey.client(auth: APIKeyAuth()).boxes.list()

        let hop = hey.requests[1]
        XCTAssertEqual(hop.path, "/boxes/all.json")
        XCTAssertEqual(hop.header("X-Api-Key"), "key-123")
        XCTAssertEqual(hop.header("Authorization"), "Bearer secret")
        XCTAssertEqual(hop.header("Cookie"), "session=abc")
        XCTAssertEqual(hop.headers.values(for: "Authorization").count, 1, "signed once, not twice")
    }

    func testAHopOnHEYKeepsTheAccountScopeWhateverTheLocationSaid() async throws {
        let hey = mockHey(
            ok(identityJSON),
            status(302, nil, [("Location", "/boxes/all.json")]),
            ok("[]"),
            status(302, nil, [("Location", "/boxes/all.json?filtered_account_id=7")]),
            ok("[]"))
        let work = try await hey.client().forAccount(42)
        _ = try await work.boxes.list()
        _ = try await work.boxes.list()

        XCTAssertEqual(hey.requests[1].query("filtered_account_id"), "42")
        XCTAssertEqual(hey.requests[2].query("filtered_account_id"), "42", "the hop is scoped when the Location leaves the filter off")
        XCTAssertEqual(hey.requests[4].query("filtered_account_id"), "42", "and when the Location names another account")
        XCTAssertEqual(hey.requests[4].queryAll("filtered_account_id"), ["42"])
    }

    func testAHopOffHEYCarriesNoAccountScope() async throws {
        let hey = mockHey(ok(identityJSON), status(302, nil, [("Location", "https://files.example.com/export.json")]), ok("[]"))
        _ = try await hey.client().forAccount(42).boxes.list()
        XCTAssertNil(hey.requests[2].query("filtered_account_id"))
    }

    func testAHopDoesNotTakeTheQueryTheRequestWentOutWith() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/moved")]), ok("{}"))
        let client = try hey.client()
        try await client.execute(client.request(.get, "/thing?token=secret"))
        XCTAssertNil(hey.requests[1].query("token"))
        XCTAssertEqual(hey.requests[1].path, "/moved")
    }

    func testARedirectNeitherCarriesNorTakesTheCacheEntryOfTheURLAskedFor() async throws {
        let hey = mockHey(
            ok(#"{"which":"a"}"#, [("ETag", #""x""#)]),
            status(302, nil, [("Location", "/b.json")]),
            ok(#"{"which":"b"}"#, [("ETag", #""x""#)]),
            status(304, nil, [("ETag", #""x""#)]))
        let client = try hey.client { $0.enableCache = true }
        let first = try await client.execute(client.request(.get, "/a")).text()
        XCTAssertEqual(first, #"{"which":"a"}"#)
        let redirected = try await client.execute(client.request(.get, "/a")).text()
        XCTAssertEqual(redirected, #"{"which":"b"}"#, "the answer reached through the redirect is b's")
        XCTAssertNil(hey.requests[2].header("If-None-Match"), "b is not asked to validate a's entry")
        XCTAssertEqual(hey.requests[2].path, "/b.json")
        let again = try await client.execute(client.request(.get, "/a")).text()
        XCTAssertEqual(again, #"{"which":"a"}"#, "a's entry is still a's, not b's")
        XCTAssertEqual(hey.requests[3].header("If-None-Match"), #""x""#)
    }

    func testA401FromAHopThatCarriedNoCredentialsRefreshesNothing() async throws {
        let provider = CountingProvider()
        let hey = mockHey(status(302, nil, [("Location", "https://files.example.com/export.json")]), status(401), ok("[]"))
        await assertThrows(HeyError.codeAuth, try await hey.client(auth: BearerAuth(tokenProvider: provider)).boxes.list())
        XCTAssertEqual(provider.refreshes, 0, "HEY's credentials were not the ones rejected")
        XCTAssertEqual(hey.requests.count, 2, "and nothing is sent again")
    }

    func testAHopOnTheSameOriginIsSignedForWhereItGoes() async throws {
        let hey = mockHey(
            status(302, nil, [("Location", "/boxes/all.json")]),
            ok("[]"),
            status(303, nil, [("Location", "/postings/seen.json")]),
            ok(""),
            status(302, nil, [("Location", "https://files.example.com/export.json")]),
            ok("[]"))
        let client = try hey.client(auth: SigningAuth())
        _ = try await client.boxes.list()
        XCTAssertEqual(hey.requests[0].header("X-Signature"), "GET /boxes.json")
        XCTAssertEqual(hey.requests[1].header("X-Signature"), "GET /boxes/all.json", "signed again for the URL the hop goes to")

        var post = client.request(.post, "/postings/mark")
        post.jsonBody(Data("{}".utf8))
        try await client.execute(post)
        XCTAssertEqual(hey.requests[2].header("X-Signature"), "POST /postings/mark.json")
        XCTAssertEqual(hey.requests[3].method, "GET", "a 303 turns the POST into a GET")
        XCTAssertEqual(hey.requests[3].header("X-Signature"), "GET /postings/seen.json", "and the signature says so")

        _ = try await client.boxes.list()
        XCTAssertNil(hey.requests[5].header("X-Signature"), "a hop off the origin is never signed")
    }

    func testAStrategyThatCannotSignAHopFailsAsItselfAndIsNotResent() async throws {
        final class Failing: AuthStrategy, @unchecked Sendable {
            let lock = NSLock()
            var signings = 0
            func authenticate(_ request: inout HTTPRequest) async throws {
                let count = lock.withLock { signings += 1; return signings }
                if count > 1 { throw TransportFailure(description: "signer offline") }
                request.headers.set("X-Signature", "ok")
            }
        }
        final class Log: HeyHooks, @unchecked Sendable {
            let lock = NSLock()
            var entries: [String] = []
            func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration) { lock.withLock { entries.append("retry") } }
            func onRequestEnd(_ info: RequestInfo, result: RequestResult) { lock.withLock { entries.append("request:\(describe(result.error))") } }
            func onOperationEnd(_ info: OperationInfo, result: OperationResult) { lock.withLock { entries.append("operation:\(describe(result.error))") } }
        }
        let hey = mockHey(status(302, nil, [("Location", "/boxes/all.json")]), ok("[]"))
        let log = Log()
        let client = try hey.client(auth: Failing(), hooks: log)
        do {
            _ = try await client.boxes.list()
            XCTFail("expected the strategy's failure")
        } catch let error as TransportFailure {
            XCTAssertEqual(error.description, "signer offline")
        }
        XCTAssertEqual(hey.requests.count, 1, "the hop is never sent and nothing is resent")
        XCTAssertEqual(log.entries, ["request:signer offline", "operation:signer offline"])
    }

    func testARedirectToNowhereIsTheAnswer() async throws {
        for location in ["", "   "] {
            let hey = mockHey(status(302, nil, [("Location", location)]))
            let error = await assertThrows(HeyError.codeAPI, try await hey.client().boxes.get(boxId: 7))
            XCTAssertEqual(error?.httpStatus, 302)
            XCTAssertEqual(hey.requests.count, 1, "a redirect that names nowhere is not followed anywhere")
        }
    }

    func testA301Or302TurnsOnlyAPostIntoAGet() async throws {
        for code in [301, 302] {
            let hey = mockHey(status(code, nil, [("Location", "/moved.json")]), ok("{}"), status(code, nil, [("Location", "/moved.json")]), ok("{}"))
            let client = try hey.client()
            var put = client.request(.put, "/thing")
            put.jsonBody(Data(#"{"a":1}"#.utf8))
            try await client.execute(put)
            XCTAssertEqual(hey.requests[1].method, "PUT", "a \(code) leaves a PUT a PUT")
            XCTAssertEqual(hey.requests[1].body, #"{"a":1}"#, "with its body")
            XCTAssertEqual(hey.requests[1].header("Content-Type"), "application/json")
            var post = client.request(.post, "/thing")
            post.jsonBody(Data(#"{"a":1}"#.utf8))
            try await client.execute(post)
            XCTAssertEqual(hey.requests[3].method, "GET", "and turns a POST into a GET")
            XCTAssertEqual(hey.requests[3].body, "")
            XCTAssertNil(hey.requests[3].header("Content-Type"), "without what described the body it dropped")
        }
        let hey = mockHey(status(303, nil, [("Location", "/answer.json")]), ok("{}"))
        let client = try hey.client()
        try await client.execute(client.request(.delete, "/thing"))
        XCTAssertEqual(hey.requests[1].method, "GET", "a 303 says fetch the answer, whatever the method")
    }

    func testAHopThatDropsTheBodyDropsEveryContentHeader() async throws {
        let hey = mockHey(status(303, nil, [("Location", "/answer.json")]), ok("{}"))
        let client = try hey.client()
        var put = client.request(.put, "/thing")
        put.bodyBytes(contentType: "application/pdf", Data("hello".utf8))
        put.header("Content-MD5", "XUFAKrxLKna5cZ2REBfFkg==")
        put.header("Content-Disposition", #"inline; filename="a.pdf""#)
        put.header("Digest", "md5=x")
        put.header("X-Kept", "yes")
        try await client.execute(put)
        let hop = hey.requests[1]
        for name in ["Content-Type", "Content-MD5", "Content-Disposition", "Digest"] {
            XCTAssertNil(hop.header(name), "\(name) described bytes the hop no longer carries")
        }
        XCTAssertEqual(hop.header("X-Kept"), "yes")
    }

    func testTenHopsIsTheMost() async throws {
        let answers = (0...10).map { index in status(302, nil, [("Location", "/hop\(index)")]) }
        let hey = MockHey(answers)
        let error = await assertThrows(HeyError.codeNetwork, try await hey.client().boxes.list())
        XCTAssertEqual(error?.message, "ListBoxes redirected more than 10 times")
        XCTAssertEqual(error?.isRetryable, false)
        XCTAssertEqual(hey.requests.count, 11)
    }

    func testAHopToPlainHTTPOffThisMachineIsRefused() async throws {
        let hey = mockHey(status(302, nil, [("Location", "http://files.example.com/export.json")]))
        let error = await assertThrows(HeyError.codeUsage, try await hey.client().boxes.list())
        XCTAssertTrue(error?.message.contains("must use HTTPS") == true)
        XCTAssertEqual(hey.requests.count, 1)
    }
}
