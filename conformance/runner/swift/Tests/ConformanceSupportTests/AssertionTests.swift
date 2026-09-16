import ConformanceSupport
import Foundation
import Hey
import XCTest

final class AssertionTests: XCTestCase {
    private func json(_ text: String) throws -> FixtureJSON { try FixtureJSON.parse(text) }

    private func run(_ recorded: Recorded, _ assertions: Assertion..., outcome: Result<Outcome, any Error> = .success(.unit)) -> Run {
        Run(
            testCase: TestCase(name: "t", operation: "ListBoxes", assertions: assertions), outcome: outcome, recorded: recorded,
            baseURL: "http://127.0.0.1:1")
    }

    private func failure(_ run: Run, file: StaticString = #filePath, line: UInt = #line) -> String {
        do {
            try checkAll(run)
            XCTFail("expected the assertion to fail", file: file, line: line)
            return ""
        } catch {
            return "\(error)"
        }
    }

    func testAJSONSuffixIsPutOnAPathWithoutAnExtension() {
        XCTAssertEqual(withJSONExtension("/boxes"), "/boxes.json")
        XCTAssertEqual(withJSONExtension("/boxes.json"), "/boxes.json")
        XCTAssertEqual(withJSONExtension("/workflows/1/stages/2"), "/workflows/1/stages/2.json")
        XCTAssertEqual(withJSONExtension("/boxes/"), "/boxes/")
        XCTAssertEqual(withJSONExtension("/a.b/c"), "/a.b/c.json", "only the last segment's dot counts")
    }

    func testADottedPathWalksObjectsAndArrays() throws {
        let value = try json(#"{"postings":[{"id":7,"kind":"topic"}],"id":33}"#)
        XCTAssertEqual(lookup(value, "postings.0.id"), .number("7"))
        XCTAssertEqual(lookup(value, "id"), .number("33"))
        XCTAssertNil(lookup(value, "postings.1.id"))
        XCTAssertNil(lookup(value, "postings.id"))
    }

    func testValuesMatchAcrossNumberSpellings() {
        XCTAssertTrue(valuesMatch(.number("9007199254740993"), .number("9007199254740993")))
        XCTAssertTrue(valuesMatch(.string("Imbox"), .string("Imbox")))
        XCTAssertTrue(valuesMatch(.bool(true), .bool(true)))
        XCTAssertFalse(valuesMatch(.number("1"), .number("2")))
        XCTAssertTrue(valuesMatch(.number("1"), .number("1.0")), "a number is the same number however it is spelled")
        XCTAssertFalse(valuesMatch(.number("9007199254740993"), .number("9007199254740992.0")), "and a double that lost a digit is not")
        XCTAssertFalse(valuesMatch(.number("1"), .string("1")), "but a string of a number is not a number")
        XCTAssertFalse(valuesMatch(.bool(true), .string("true")))
        XCTAssertFalse(valuesMatch(.string("1"), .number("1")))
        XCTAssertTrue(valuesMatch(.array([.number("1")]), .array([.number("1.0")])), "numbers inside arrays compare as numbers too")
    }

    func testQueryPairsAreDecodedAsAFormIs() {
        XCTAssertTrue(queryPairs("posting_ids=1%2C2&page=older").elementsEqual([("posting_ids", "1,2"), ("page", "older")], by: ==))
        XCTAssertTrue(queryPairs("q=two+words&flag").elementsEqual([("q", "two words"), ("flag", "")], by: ==))
        XCTAssertTrue(queryPairs(nil).isEmpty)
    }

    func testEveryWaitBetweenRequestsIsChecked() {
        var recorded = Recorded()
        recorded.times = [0, 1_000_000_000, 1_001_000_000]
        let minimum = Assertion(type: "delayBetweenRequests", min: 1000)
        XCTAssertTrue(failure(run(recorded, minimum)).contains("before request 3"))
        recorded.times[2] = 2_000_000_000
        XCTAssertNoThrow(try checkAll(run(recorded, minimum)))
    }

    func testAScalarQueryParameterHasToBeThereExactlyOnce() throws {
        var recorded = Recorded()
        recorded.paths = ["/boxes.json"]
        recorded.queries = [[("filtered_account_id", "42"), ("filtered_account_id", "42")]]
        let expected = Assertion(type: "requestQuery", expected: try json(#"{"filtered_account_id":"42"}"#))
        XCTAssertTrue(failure(run(recorded, expected)).contains("once, got it 2 times"))
        recorded.queries[0] = [("filtered_account_id", "42")]
        XCTAssertNoThrow(try checkAll(run(recorded, expected)))
    }

    func testAScalarFormFieldHasToBeThereExactlyOnce() throws {
        var recorded = Recorded()
        recorded.bodies = [Data("calendar_event%5Bsummary%5D=Expected&calendar_event%5Bsummary%5D=Wrong".utf8)]
        let expected = Assertion(type: "requestForm", expected: try json(#"{"calendar_event[summary]":"Expected"}"#))
        XCTAssertTrue(failure(run(recorded, expected)).contains("once, got it 2 times"))
        recorded.bodies[0] = Data("calendar_event%5Bsummary%5D=Expected".utf8)
        XCTAssertNoThrow(try checkAll(run(recorded, expected)))
    }

    func testANullExpectationMeansAbsent() throws {
        var recorded = Recorded()
        recorded.bodies = [Data(#"{"a":1,"b":null}"#.utf8)]
        XCTAssertNoThrow(try checkAll(run(recorded, Assertion(type: "requestBody", expected: try json(#"{"c":null}"#)))))
        XCTAssertTrue(failure(run(recorded, Assertion(type: "requestBody", expected: try json(#"{"a":null}"#)))).contains("to be absent"))
        XCTAssertTrue(failure(run(recorded, Assertion(type: "requestBody", expected: try json(#"{"a":"1"}"#)))).contains("got 1"))
    }

    func testAnErrorCodeIsReadOffTheSDKError() {
        let recorded = Recorded()
        let expected = Assertion(type: "errorCode", expected: .string("not_found"))
        XCTAssertNoThrow(try checkAll(run(recorded, expected, outcome: .failure(HeyError.notFound(message: "gone", detail: ErrorDetail())))))
        XCTAssertTrue(failure(run(recorded, expected, outcome: .success(.unit))).contains("got no error"))
        XCTAssertTrue(failure(run(recorded, expected, outcome: .failure(CancellationError()))).contains("non-SDK failure"))
    }

    func testAStatusOnAFailureHasToComeFromTheError() {
        var recorded = Recorded()
        recorded.statuses = [500]
        let expected = Assertion(type: "statusCode", expected: .number("500"))
        XCTAssertTrue(failure(run(recorded, expected, outcome: .failure(CancellationError()))).contains("carries no HTTP status"))
        XCTAssertNoThrow(try checkAll(run(recorded, expected)))
    }

    func testACrossOriginNextPageHasToBeRefusedAsAUsageError() {
        var recorded = Recorded()
        recorded.links = [#"<https://evil.example/boxes.json?page=2>; rel="next""#]
        let assertion = Assertion(type: "urlOrigin", expected: .string("rejected"))
        func page(_ check: Result<Void, any Error>?) -> Result<Outcome, any Error> {
            .success(.page(value: .array([]), nextPage: nil, totalCount: nil, nextURLCheck: check))
        }
        XCTAssertNoThrow(try checkAll(run(recorded, assertion, outcome: page(.failure(HeyError.usage(message: "cross-origin"))))))
        XCTAssertTrue(failure(run(recorded, assertion, outcome: page(.success(())))).contains("the SDK followed it"))
        XCTAssertTrue(failure(run(recorded, assertion, outcome: page(.failure(CancellationError())))).contains("as a usage error"))
        recorded.links = [#"<http://127.0.0.1:1/boxes.json?page=2>; rel="next""#]
        XCTAssertTrue(failure(run(recorded, assertion, outcome: page(.failure(HeyError.usage(message: "x"))))).contains("same origin"))
    }

    func testAnUnknownAssertionTypeFails() {
        XCTAssertTrue(failure(run(Recorded(), Assertion(type: "somethingNew"))).contains("Unknown assertion type"))
    }

    func testAFixtureReadsWithItsDefaults() throws {
        let cases = try TestCase.load(
            #"""
            [{"name":"n","operation":"GetBox","pathParams":{"boxId":7},
              "mockResponses":[{"status":200,"headers":{"Content-Type":"text/html"},"body":"<p>hi</p>","delay":5}],
              "assertions":[{"type":"requestCount","expected":1},{"type":"delayBetweenRequests","min":250}],
              "configOverrides":{"clientLayer":"hey","maxRetries":2,"baseDelayMs":10},
              "tags":["ignored"],"repeatOperation":3}]
            """#)
        let only = try XCTUnwrap(cases.first)
        XCTAssertEqual(only.pathParams.int("boxId"), 7)
        XCTAssertEqual(only.queryParams, .object([]))
        XCTAssertTrue(only.isHeyLayer)
        XCTAssertEqual(only.configOverrides.maxRetries, 2)
        XCTAssertEqual(only.configOverrides.baseDelayMs, 10)
        XCTAssertEqual(only.runs, 3)
        XCTAssertEqual(only.assertions[1].min, 250)
        let mock = try XCTUnwrap(only.mockResponses.first)
        XCTAssertEqual(mock.delay, 5)
        XCTAssertEqual(String(decoding: mock.bodyData, as: UTF8.self), "<p>hi</p>", "an HTML body goes out as written")
        XCTAssertEqual(
            String(decoding: MockResponse(status: 200, body: .string("text")).bodyData, as: UTF8.self), #""text""#,
            "any other string body is JSON")
        XCTAssertThrowsError(try TestCase.load(#"{"name":"not an array"}"#))
        XCTAssertThrowsError(try TestCase.load(#"[{"operation":"GetBox"}]"#))
    }

    func testTheMockAnswersInOrderAndRecordsWhatItSaw() async throws {
        let mock = try MockHEY([
            MockResponse(status: 503, headers: [("Retry-After", "0")]),
            MockResponse(status: 200, headers: [("Link", "</boxes.json?page=2>; rel=\"next\"")], body: .array([])),
        ])
        let client = try HeyClient(accessToken: "t", config: HeyConfig(baseURL: mock.baseURL))
        _ = try await client.boxes.list()
        let extra = HTTPRequest(method: "POST", url: URL(string: mock.baseURL + "/extra.json?x=1")!, body: Data("a=1".utf8))
        let response = try await URLSessionTransport().send(extra) { _, _ in 1024 }.status
        XCTAssertEqual(response, 500, "a request past the last mock response is answered with a 500")
        let recorded = mock.shutdown()
        XCTAssertEqual(recorded.count, 3)
        XCTAssertEqual(recorded.paths, ["/boxes.json", "/boxes.json", "/extra.json"])
        XCTAssertEqual(recorded.statuses, [503, 200])
        XCTAssertEqual(recorded.links, [nil, "</boxes.json?page=2>; rel=\"next\""])
        XCTAssertTrue(recorded.queries[2].elementsEqual([("x", "1")], by: ==))
        XCTAssertEqual(recorded.bodies[2], Data("a=1".utf8))
        XCTAssertEqual(recorded.header(0, "authorization"), "Bearer t")
    }
}
