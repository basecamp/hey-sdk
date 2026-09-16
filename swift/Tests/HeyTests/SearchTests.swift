import Foundation
import XCTest

@testable import Hey

final class SearchTests: XCTestCase {
    private let matches = #"{"matches":[{"topic":{"id":1,"subject":"Invoice"},"posting_id":9,"entries":[]}]}"#

    func testASearchSendsOnlyTheRefinementsThatAreSet() async throws {
        let hey = mockHey(ok(matches))
        let result = try await hey.client().search.search(
            SearchParams(query: "invoice", from: "billing@example.com", date: "2026", inBox: "papertrail"))
        XCTAssertEqual(result.matches.count, 1)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.path, "/advanced_search.json")
        XCTAssertEqual(request.query("q"), "invoice")
        XCTAssertEqual(request.query("refine[from]"), "billing@example.com")
        XCTAssertEqual(request.query("refine[in]"), "papertrail")
        XCTAssertEqual(request.query("refine[date]"), "2026")
        XCTAssertNil(request.query("refine[to]"), "an empty refinement is left off the wire")
        XCTAssertNil(request.query("refine[label]"))
        XCTAssertNil(request.query("page"), "the first page is asked for by saying nothing")
    }

    func testAPageIsNumberedAndTheNextOneComesOutOfTheLink() async throws {
        let hey = mockHey(ok(matches, [("Link", #"</advanced_search.json?q=invoice&page=3>; rel="next""#)]), ok(matches))
        let client = try hey.client()
        let page = try await client.search.searchPage(SearchParams(query: "invoice", page: 2))
        XCTAssertEqual(hey.requests[0].query("page"), "2")
        XCTAssertEqual(page.nextPage, 3)
        XCTAssertEqual(page.result.matches.count, 1)

        let last = try await client.search.searchPage(SearchParams(query: "invoice", page: 1))
        XCTAssertNil(hey.requests[1].query("page"), "zero and one both ask for the first")
        XCTAssertNil(last.nextPage, "HEY sends no Link on the last page, which is how a walk is told to stop")
    }
}
