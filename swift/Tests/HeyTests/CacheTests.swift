import Foundation
import XCTest

@testable import Hey

/// A strategy that signs with a header of its own: an API key, a cookie, whatever the caller's
/// HEY takes.
struct HeaderAuth: AuthStrategy {
    let name: String
    let value: String
    var bearer: String?

    func authenticate(_ request: inout HTTPRequest) async throws {
        request.headers.set(name, value)
        if let bearer { request.headers.set("Authorization", "Bearer \(bearer)") }
    }
}

/// A strategy that sets one header several times, as a list.
struct ListAuth: AuthStrategy {
    let values: [String]

    func authenticate(_ request: inout HTTPRequest) async throws {
        for value in values { request.headers.add("X-Key", value) }
    }
}

/// Hooks that record whether each request was answered from the cache.
final class CacheLog: HeyHooks, @unchecked Sendable {
    private let lock = NSLock()
    private var entries: [Bool] = []

    var fromCache: [Bool] { lock.withLock { entries } }

    func onRequestEnd(_ info: RequestInfo, result: RequestResult) {
        lock.withLock { entries.append(result.fromCache) }
    }
}

final class CacheTests: XCTestCase {
    func testACachedReadRevalidatesAndReadsThe304FromTheCache() async throws {
        let hey = mockHey(
            ok(#"[{"id":7,"kind":"imbox","name":"Imbox"}]"#, [("ETag", #""v1""#)]),
            status(304, nil, [("ETag", #""v1""#)]),
            ok(#"[{"id":8,"kind":"imbox","name":"Imbox"}]"#, [("ETag", #""v2""#)]),
            status(304, nil, [("ETag", #""v2""#)]))
        let log = CacheLog()
        let client = try hey.client(hooks: log) { $0.enableCache = true }
        var ids: [Int] = []
        for _ in 0..<4 { ids.append(contentsOf: try await client.boxes.list().value.map(\.id)) }
        XCTAssertEqual(ids, [7, 7, 8, 8])

        XCTAssertNil(hey.requests[0].header("If-None-Match"))
        XCTAssertEqual(hey.requests[1].header("If-None-Match"), #""v1""#)
        XCTAssertEqual(hey.requests[2].header("If-None-Match"), #""v1""#)
        XCTAssertEqual(hey.requests[3].header("If-None-Match"), #""v2""#)
        XCTAssertEqual(log.fromCache, [false, true, false, true])
    }

    func testAClientWithoutACacheNeverSendsAConditionalRequest() async throws {
        let hey = mockHey(ok("[]", [("ETag", #""v1""#)]), ok("[]"))
        let client = try hey.client()
        _ = try await client.boxes.list()
        _ = try await client.boxes.list()
        XCTAssertNil(hey.requests[1].header("If-None-Match"))
    }

    func testA304WithNothingCachedIsAnError() async throws {
        let hey = mockHey(status(304))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client { $0.enableCache = true }.boxes.list())
        XCTAssertEqual(error?.httpStatus, 304)
    }

    func testANoStoreAnswerIsNotHeld() async throws {
        let hey = mockHey(ok("[]", [("ETag", #""v1""#), ("Cache-Control", "private, No-Store")]), ok("[]", [("ETag", #""v2""#)]))
        let store = InMemoryCache()
        let client = try hey.client(cache: store) { $0.enableCache = true }
        _ = try await client.boxes.list()
        XCTAssertEqual(store.count, 0)
        _ = try await client.boxes.list()
        XCTAssertNil(hey.requests[1].header("If-None-Match"))
    }

    func testASuccessWithoutAValidatorEndsWhatWasHeld() async throws {
        let hey = mockHey(ok(#"{"n":"a"}"#, [("ETag", #""v1""#)]), ok(#"{"n":"b"}"#), ok(#"{"n":"c"}"#))
        let store = InMemoryCache()
        let client = try hey.client(cache: store) { $0.enableCache = true }
        let a = try await client.execute(client.request(.get, "/thing")).text()
        let b = try await client.execute(client.request(.get, "/thing")).text()
        XCTAssertEqual(a, #"{"n":"a"}"#)
        XCTAssertEqual(b, #"{"n":"b"}"#)
        XCTAssertEqual(hey.requests[1].header("If-None-Match"), #""v1""#)
        XCTAssertEqual(store.count, 0, "b replaced a, and cannot be revalidated, so nothing is held")
        try await client.execute(client.request(.get, "/thing"))
        XCTAssertNil(hey.requests[2].header("If-None-Match"), "a's validator is not sent for a body HEY has moved on from")
    }

    func testANoStoreAnswerEvictsWhatWasHeldForTheKey() async throws {
        let hey = mockHey(ok("[]", [("ETag", #""v1""#)]), ok("[]", [("ETag", #""v2""#), ("Cache-Control", "no-store")]), ok("[]"))
        let store = InMemoryCache()
        let client = try hey.client(cache: store) { $0.enableCache = true }
        _ = try await client.boxes.list()
        XCTAssertEqual(store.count, 1)
        _ = try await client.boxes.list()
        XCTAssertEqual(hey.requests[1].header("If-None-Match"), #""v1""#)
        XCTAssertEqual(store.count, 0)
        _ = try await client.boxes.list()
        XCTAssertNil(hey.requests[2].header("If-None-Match"))
    }

    func testA304AnswersThePageUnderTheHeadersItWasCachedWith() async throws {
        let hey = mockHey(
            ok(
                #"[{"id":1,"kind":"imbox","name":"a"}]"#,
                [("ETag", #""v1""#), ("Link", #"</boxes.json?page=2>; rel="next""#), ("X-Total-Count", "2"), ("Set-Cookie", "session=abc")]),
            status(304, nil, [("ETag", #""v1""#), ("X-Total-Count", "3")]),
            ok(#"[{"id":2,"kind":"imbox","name":"b"}]"#))
        let store = InMemoryCache()
        let client = try hey.client(cache: store) { $0.enableCache = true }
        _ = try await client.boxes.list()
        let held = try XCTUnwrap(store.get(cacheKey(url: "https://app.hey.com/boxes.json", credentials: "13:authorization1:17:Bearer test-token")))
        XCTAssertEqual(Set(held.headers.names), ["etag", "link", "x-total-count", "content-type"], "the entry keeps what came with the body, less any credential")

        let revalidated = try await client.boxes.list()
        XCTAssertEqual(revalidated.value.map(\.id), [1])
        XCTAssertEqual(revalidated.nextPage, "2", "the Link the 304 left out is the cached one")
        XCTAssertEqual(revalidated.totalCount, 3, "the header the 304 did carry replaces the cached one")
        let second = try await client.nextPage(revalidated)
        XCTAssertEqual(second?.value.map(\.id), [2])
        XCTAssertEqual(hey.requests[2].query("page"), "2")
    }

    func testWhatA304MovesIsWhatTheNextReadGoesOutWith() async throws {
        let hey = mockHey(
            ok("[]", [("ETag", #""v1""#), ("Link", #"</boxes.json?page=2>; rel="next""#)]),
            status(304, nil, [("ETag", #""v2""#), ("Link", #"</boxes.json?page=3>; rel="next""#)]),
            status(304, nil, [("ETag", #""v2""#)]))
        let client = try hey.client { $0.enableCache = true }
        _ = try await client.boxes.list()
        let moved = try await client.boxes.list()
        let kept = try await client.boxes.list()
        XCTAssertEqual(moved.nextPage, "3")
        XCTAssertEqual(kept.nextPage, "3", "the cursor the 304 moved is what the entry keeps")
        XCTAssertEqual(hey.requests[1].header("If-None-Match"), #""v1""#)
        XCTAssertEqual(hey.requests[2].header("If-None-Match"), #""v2""#, "the validator the 304 moved is what the next read sends")
    }

    func testA304SayingNoStoreEndsTheEntry() async throws {
        let hey = mockHey(ok("[]", [("ETag", #""v1""#)]), status(304, nil, [("ETag", #""v1""#), ("Cache-Control", "no-store")]), ok("[]"))
        let store = InMemoryCache()
        let client = try hey.client(cache: store) { $0.enableCache = true }
        _ = try await client.boxes.list()
        _ = try await client.boxes.list()
        XCTAssertEqual(store.count, 0)
        _ = try await client.boxes.list()
        XCTAssertNil(hey.requests[2].header("If-None-Match"))
    }

    func testTheKeyNeverHoldsTheCredential() {
        let key = cacheKey(url: "https://app.hey.com/boxes.json", credentials: "Bearer secret")
        XCTAssertEqual(key.count, 64)
        XCTAssertFalse(key.contains("secret"))
        XCTAssertEqual(key, cacheKey(url: "https://app.hey.com/boxes.json", credentials: "Bearer secret"))
        XCTAssertNotEqual(key, cacheKey(url: "https://app.hey.com/boxes.json", credentials: "Bearer other"))
    }

    func testAnEntryPastTheCapIsNotSentBackAsAValidator() async throws {
        let hey = mockHey(ok("[]"))
        let store = InMemoryCache()
        store.set(
            cacheKey(url: "https://app.hey.com/boxes.json", credentials: "13:authorization1:17:Bearer test-token"),
            CachedResponse(etag: #""big""#, body: Data(repeating: 0x20, count: 200)))
        let client = try hey.client(cache: store) {
            $0.enableCache = true
            $0.maxResponseBodyBytes = 100
        }
        _ = try await client.boxes.list()
        XCTAssertNil(hey.requests[0].header("If-None-Match"), "an entry the client would not hold is not relied on")
    }

    func testAStrategyWithoutABearerStillGetsRevalidation() async throws {
        let hey = mockHey(ok("[]", [("ETag", #""v1""#)]), status(304, nil, [("ETag", #""v1""#)]))
        let client = try hey.client(auth: HeaderAuth(name: "X-Api-Key", value: "key-123")) { $0.enableCache = true }
        _ = try await client.boxes.list()
        _ = try await client.boxes.list()
        XCTAssertEqual(hey.requests[1].header("If-None-Match"), #""v1""#, "the key partitions the cache as a bearer would")
    }

    func testTwoIdentitiesThatShareABearerButNotACookieNeverShareAnEntry() async throws {
        let store = InMemoryCache()
        let hey = mockHey(ok(#"{"who":"a"}"#, [("ETag", #""same""#)]), ok(#"{"who":"b"}"#, [("ETag", #""same""#)]), status(304))
        let a = try hey.client(auth: HeaderAuth(name: "Cookie", value: "session=a", bearer: "shared"), cache: store) { $0.enableCache = true }
        let b = try hey.client(auth: HeaderAuth(name: "Cookie", value: "session=b", bearer: "shared"), cache: store) { $0.enableCache = true }
        try await a.execute(a.request(.get, "/me"))
        let bs = try await b.execute(b.request(.get, "/me")).text()
        XCTAssertEqual(bs, #"{"who":"b"}"#)
        XCTAssertNil(hey.requests[1].header("If-None-Match"), "b is not asked to validate a's entry")
        XCTAssertEqual(store.count, 2, "one entry each")
        let again = try await a.execute(a.request(.get, "/me")).text()
        XCTAssertEqual(again, #"{"who":"a"}"#, "and a's 304 answers a's body")
    }

    func testTwoSpellingsOfAHeaderThatReadTheSameNeverShareAnEntry() async throws {
        let store = InMemoryCache()
        let hey = mockHey(ok(#"{"who":"A SECRET"}"#, [("ETag", #""same""#)]), ok(#"{"who":"b"}"#, [("ETag", #""same""#)]))
        let two = try hey.client(auth: ListAuth(values: ["alpha", "beta"]), cache: store) { $0.enableCache = true }
        let one = try hey.client(auth: ListAuth(values: ["alpha, beta"]), cache: store) { $0.enableCache = true }
        try await two.execute(two.request(.get, "/me"))
        let ones = try await one.execute(one.request(.get, "/me")).text()
        XCTAssertEqual(ones, #"{"who":"b"}"#)
        XCTAssertNil(hey.requests[1].header("If-None-Match"), "the one-value identity is not asked to validate the two-value identity's entry")
        XCTAssertEqual(store.count, 2)
    }

    func testACredentialHEYEchoesIsNotKeptInTheCache() async throws {
        let hey = mockHey(
            ok("[]", [("ETag", #""v1""#), ("X-Api-Key", "key-123"), ("X-Echo", "fine")]),
            status(304, nil, [("ETag", #""v1""#), ("X-Api-Key", "key-123")]))
        let store = InMemoryCache()
        let client = try hey.client(auth: HeaderAuth(name: "X-Api-Key", value: "key-123"), cache: store) { $0.enableCache = true }
        _ = try await client.boxes.list()
        let key = cacheKey(url: "https://app.hey.com/boxes.json", credentials: "9:x-api-key1:7:key-123")
        let entry = try XCTUnwrap(store.get(key))
        XCTAssertNil(entry.headers["X-Api-Key"], "the key the strategy signs with is not kept, echoed or not")
        XCTAssertEqual(entry.headers.values(for: "X-Echo"), ["fine"])
        _ = try await client.boxes.list()
        XCTAssertNil(try XCTUnwrap(store.get(key)).headers["X-Api-Key"], "nor after a 304 that echoes it")
    }

    func testOnlyAJSONGetIsCached() async throws {
        let hey = mockHey(ok("{}", [("ETag", #""v1""#)]), ok("{}", [("ETag", #""v1""#)]), ok("<p>", [("ETag", #""v1""#), ("Content-Type", "text/html")]))
        let store = InMemoryCache()
        let client = try hey.client(cache: store) { $0.enableCache = true }
        try await client.execute(client.request(.post, "/thing"))
        var uncached = client.request(.get, "/thing")
        uncached.noCache()
        try await client.execute(uncached)
        _ = try await client.workflows.getStage(workflowId: 1, stageId: 2)
        XCTAssertEqual(store.count, 0)
    }
}
