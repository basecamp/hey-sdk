import Foundation
import XCTest

@testable import Hey

final class PaginationTests: XCTestCase {
    func testTheNextLinkIsFoundAmongOthers() {
        XCTAssertEqual(
            nextLink(#"<https://app.hey.com/imbox.json?page=a,b>; rel="prev", <https://app.hey.com/imbox.json?page=c>; rel="next""#),
            "https://app.hey.com/imbox.json?page=c")
        XCTAssertEqual(nextLink(#"</boxes.json?page=2>; rel="next""#), "/boxes.json?page=2")
        XCTAssertEqual(nextLink(#"</x>; rel="next prev""#), "/x")
        XCTAssertEqual(nextLink("</x>; rel=prev, </y>; REL=NEXT"), "/y")
        XCTAssertEqual(nextLink(#"</boxes.json?page=2>; rel="next", </boxes.json?page=1>; rel="prev""#), "/boxes.json?page=2", "next before another relation, quoted")
        XCTAssertEqual(nextLink("</boxes.json?page=2>; rel=next, </boxes.json?page=1>; rel=prev"), "/boxes.json?page=2", "and unquoted")
        XCTAssertEqual(nextLink(#"</x?a=1,2>; rel="next", </y>; rel="prev""#), "/x?a=1,2", "a comma in the target stays")
        XCTAssertNil(nextLink(#"</x>; rel="prev""#))
        XCTAssertNil(nextLink("garbage"))
    }

    func testAReferenceTakesNothingOfTheRequestsQuery() throws {
        let base = try XCTUnwrap(URL(string: "https://app.hey.com/contacts/88.json?page=older&filtered_account_id=42#x"))
        XCTAssertEqual(resolveReference(base, "/contacts/88.json?page=next")?.absoluteString, "https://app.hey.com/contacts/88.json?page=next")
        XCTAssertEqual(resolveReference(base, "/contacts/88.json")?.absoluteString, "https://app.hey.com/contacts/88.json")
        XCTAssertEqual(resolveReference(base, "https://files.example.com/export.json")?.absoluteString, "https://files.example.com/export.json")
        XCTAssertEqual(resolveReference(base, "89.json?page=1")?.absoluteString, "https://app.hey.com/contacts/89.json?page=1")
        XCTAssertEqual(resolveReference(base, "?page=next")?.absoluteString, "https://app.hey.com/contacts/88.json?page=next", "a query-only reference keeps the base's path")
        XCTAssertEqual(resolveReference(base, "#top")?.absoluteString, "https://app.hey.com/contacts/88.json#top", "a fragment-only reference keeps the path")
        XCTAssertEqual(resolveReference(base, "//files.example.com/x")?.absoluteString, "https://files.example.com/x", "a network-path reference takes the base's scheme")
        XCTAssertNil(resolveReference(base, ""), "a reference that names nothing resolves to nothing")
        XCTAssertNil(resolveReference(base, "   "))
    }

    func testTheNextPageDoesNotInheritTheCursorItWasAskedWith() async throws {
        let hey = mockHey(ok(#"{"id":88}"#, [("Link", #"</contacts/88.json?page=next>; rel="next""#)]), ok(#"{"id":88}"#))
        let client = try hey.client()
        let first = try await client.contacts.get(contactId: 88, options: GetContactOptions(page: "older"))
        XCTAssertEqual(first.nextPage, "next")
        _ = try await client.nextPage(first)
        XCTAssertEqual(hey.requests[1].queryAll("page"), ["next"])
    }

    func testAPageCarriesTheCursorAndTheTotal() async throws {
        let hey = mockHey(
            ok(#"[{"id":1,"kind":"imbox","name":"a"}]"#, [("Link", #"</boxes.json?page=2>; rel="next""#), ("X-Total-Count", "50")]),
            ok(#"[{"id":2,"kind":"imbox","name":"b"}]"#))
        let client = try hey.client()
        let first = try await client.boxes.list()
        XCTAssertEqual(first.nextPage, "2")
        XCTAssertEqual(first.totalCount, 50)
        XCTAssertEqual(first.nextURL?.absoluteString, "https://app.hey.com/boxes.json?page=2")
        let second = try await client.nextPage(first)
        XCTAssertEqual(second?.value.map(\.id), [2])
        XCTAssertNil(second?.nextPage)
        let none = try await client.nextPage(try XCTUnwrap(second))
        XCTAssertNil(none)
        XCTAssertEqual(hey.requests[1].query("page"), "2")
    }

    func testALinkOffTheOriginIsRefused() async throws {
        for link in [#"<https://evil.example.com/boxes.json?page=2>; rel="next""#, #"<http://app.hey.com/boxes.json?page=2>; rel="next""#] {
            let hey = mockHey(ok("[]", [("Link", link)]))
            let client = try hey.client()
            let first = try await client.boxes.list()
            await assertThrows(HeyError.codeUsage, try await client.nextPage(first))
            XCTAssertEqual(hey.requests.count, 1)
        }
    }

    func testTheNextPageIsReadUnderTheFirstPagesRetryPolicy() async throws {
        let link = [("Link", #"</boxes.json?page=2>; rel="next""#)]

        let unnamed = mockHey(ok("[]", link), status(500), ok("[]"))
        let client = try unnamed.client()
        let page = try await client.boxes.list()
        let error = await assertThrows(HeyError.codeAPI, try await client.nextPage(page))
        XCTAssertEqual(error?.httpStatus, 500)
        XCTAssertEqual(unnamed.requests.count, 2, "ListBoxes' policy does not name 500, so the next page is not resent on it either")

        let exhausted = mockHey(ok("[]", link), status(503), status(503), status(503), status(503), ok("[]"))
        let exhaustedClient = try exhausted.client()
        let first = try await exhaustedClient.boxes.list()
        await assertThrows(HeyError.codeAPI, try await exhaustedClient.nextPage(first))
        XCTAssertEqual(exhausted.requests.count, 4, "three sends for the next page, the policy's most, not the client's four")
    }

    func testTheNextPageIsAnnouncedAsTheOperationTheFirstCameFrom() async throws {
        let hey = mockHey(ok("[]", [("Link", #"</boxes.json?page=2>; rel="next""#)]), ok("[]"))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        _ = try await client.nextPage(try await client.boxes.list())
        XCTAssertEqual(log.started, ["Boxes.ListBoxes:box:false:nil", "Boxes.ListBoxes:box:false:nil"])
    }

    func testAWalkStopsAtThePageLimitAndSaysSo() async throws {
        let endless = ok("[]", [("Link", #"</boxes.json?page=next>; rel="next""#)])
        let hey = mockHey(endless, endless, endless, endless)
        let client = try hey.client { $0.maxPages = 2 }
        var visited = 0
        let first = try await client.boxes.list()
        let error = await assertThrows(HeyError.codeAPI, try await client.eachPage(first) { _ in visited += 1; return true })
        XCTAssertEqual(visited, 2)
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(error?.isRetryable, false)
    }

    func testAVisitThatAnswersFalseStopsTheWalk() async throws {
        let endless = ok("[]", [("Link", #"</boxes.json?page=next>; rel="next""#)])
        let hey = mockHey(endless, endless, endless)
        let client = try hey.client()
        var visited = 0
        try await client.eachPage(try await client.boxes.list()) { _ in visited += 1; return visited < 2 }
        XCTAssertEqual(visited, 2)
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testAStreamReadsEveryPage() async throws {
        let hey = mockHey(
            ok(#"[{"id":1,"kind":"imbox","name":"a"}]"#, [("Link", #"</boxes.json?page=2>; rel="next""#)]),
            ok(#"[{"id":2,"kind":"imbox","name":"b"}]"#, [("Link", #"</boxes.json?page=3>; rel="next""#)]),
            ok(#"[{"id":3,"kind":"imbox","name":"c"}]"#))
        let client = try hey.client()
        var ids: [Int] = []
        for try await page in client.pages(from: try await client.boxes.list()) {
            ids.append(contentsOf: page.value.map(\.id))
        }
        XCTAssertEqual(ids, [1, 2, 3])
    }

    func testAWalkFollowsANextThatComesBeforeAPrev() async throws {
        let hey = mockHey(
            ok(#"[{"id":1,"kind":"imbox","name":"a"}]"#, [("Link", #"</boxes.json?page=2>; rel="next", </boxes.json?page=0>; rel="prev""#)]),
            ok(#"[{"id":2,"kind":"imbox","name":"b"}]"#, [("Link", #"</boxes.json?page=1>; rel="prev""#)]))
        let client = try hey.client()
        var ids: [Int] = []
        for try await page in client.pages(from: try await client.boxes.list()) {
            ids.append(contentsOf: page.value.map(\.id))
        }
        XCTAssertEqual(ids, [1, 2])
        XCTAssertEqual(hey.requests[1].query("page"), "2")
    }

    func testAQueryOnlyNextLinkIsAPage() async throws {
        let hey = mockHey(ok("[]", [("Link", #"<?page=2>; rel="next""#)]), ok("[]"))
        let client = try hey.client()
        let first = try await client.boxes.list()
        XCTAssertEqual(first.nextPage, "2")
        XCTAssertNil(first.nextCursor)
        _ = try await client.nextPage(first)
        XCTAssertEqual(hey.requests[1].path, "/boxes.json")
        XCTAssertEqual(hey.requests[1].query("page"), "2")
    }

    func testALinkWithoutThePageParameterIsACursorToPollAndOffTheOriginIsRefused() async throws {
        let hey = mockHey(ok("[]", [("Link", #"</boxes.json?since=later>; rel="next""#)]), ok("[]", [("Link", #"<https://evil.example.com/boxes.json?since=x>; rel="next""#)]))
        let client = try hey.client()
        let first = try await client.boxes.list()
        XCTAssertNil(first.nextPage)
        XCTAssertNil(first.nextURL)
        XCTAssertEqual(first.nextCursor?.absoluteString, "https://app.hey.com/boxes.json?since=later")
        let error = await assertThrows(HeyError.codeUsage, try await client.boxes.list())
        XCTAssertFalse(error?.message.contains("since=x") == true, "the refusal names the origin, not the URL")
    }

    func testABodyExactlyAtTheCapIsRead() async throws {
        let cap = 100 * 1024
        var exact = "[" + Array(repeating: "1", count: (cap - 4) / 2).joined(separator: ",") + "]"
        exact += String(repeating: " ", count: cap - exact.utf8.count)
        XCTAssertEqual(exact.utf8.count, cap)
        let hey = mockHey(ok(exact))
        let client = try hey.client { $0.maxResponseBodyBytes = cap }
        let response = try await client.execute(client.request(.get, "/big"))
        XCTAssertEqual(response.body.count, cap)

        let over = mockHey(ok(exact + " "))
        let overClient = try over.client { $0.maxResponseBodyBytes = cap }
        let error = await assertThrows(HeyError.codeAPI, try await overClient.execute(overClient.request(.get, "/big")))
        XCTAssertEqual(error?.responseTooLarge, true)
    }

    func testABlobIsHeldToTheFixedCapRatherThanTheConfiguredOne() async throws {
        let hey = mockHey(ok(String(repeating: "x", count: 1000), [("Content-Type", "text/csv")]))
        let client = try hey.client { $0.maxResponseBodyBytes = 100 }
        var export = client.request(.get, "/export.csv")
        export.accept = "text/csv"
        let response = try await client.execute(export)
        XCTAssertEqual(response.body.count, 1000, "an export is not held to the cap for documents the client parses")
    }
}
