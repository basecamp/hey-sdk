import Foundation
import XCTest

@testable import Hey

final class BoxesTests: XCTestCase {
    func testBoxesAreResolvedByKindFromOneReadOfTheIndex() async throws {
        let hey = mockHey(ok(#"[{"id":1,"kind":"imbox","name":"Imbox"},{"id":2,"kind":"asidebox","name":"Set Aside"}]"#), ok(""))
        let client = try hey.client()
        let setAside = try await client.boxes.idByKind(.setAside)
        XCTAssertEqual(setAside, 2)
        let imbox = try await client.boxes.idByKind(.imbox)
        XCTAssertEqual(imbox, 1)
        try await client.postings.moveToSetAside(postingIds: [7, 8])
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["box_id"] as? Int, 2)
        await assertThrows(HeyError.codeAPI, try await client.boxes.idByKind(.bubbleUp))
        await assertThrows(HeyError.codeUsage, try await client.postings.moveToSetAside(postingIds: []))

        let cold = mockHey()
        await assertThrows(HeyError.codeUsage, try await cold.client().postings.moveTo(kind: .imbox, postingIds: []))
        XCTAssertEqual(cold.requests.count, 0, "an empty selection is refused before the box index is read")
    }

    func testAnIndexReadThatFailsIsNotKept() async throws {
        let hey = mockHey(status(503), ok(#"[{"id":1,"kind":"imbox","name":"Imbox"}]"#))
        let client = try hey.client(configure: { $0.enableRetry = false })
        await assertThrows(HeyError.codeAPI, try await client.boxes.idByKind(.imbox))
        let imbox = try await client.boxes.idByKind(.imbox)
        XCTAssertEqual(imbox, 1, "the next ask reads the index again")
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testCallersAskingAtOnceShareOneReadOfTheIndex() async throws {
        let hey = MockHey(respond: { _, request in
            request.path == "/boxes.json" ? ok(#"[{"id":1,"kind":"imbox","name":"Imbox"},{"id":3,"kind":"feedbox","name":"The Feed"}]"#) : status(404)
        })
        let client = try hey.client()
        async let imbox = client.boxes.idByKind(.imbox)
        async let feed = client.boxes.idByKind(.feed)
        let ids = try await [imbox, feed]
        XCTAssertEqual(ids, [1, 3])
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testAClientForAnAccountReadsTheIndexForItself() async throws {
        let hey = MockHey(respond: { _, request in
            switch request.path {
            case "/identity.json": return ok(identityJSON)
            case "/boxes.json":
                return request.query("filtered_account_id") == "42"
                    ? ok(#"[{"id":42,"kind":"imbox","name":"Imbox"}]"#)
                    : ok(#"[{"id":1,"kind":"imbox","name":"Imbox"}]"#)
            default: return status(404)
            }
        })
        let client = try hey.client()
        let everything = try await client.boxes.idByKind(.imbox)
        XCTAssertEqual(everything, 1)
        let work = try await client.forAccount(42)
        let scoped = try await work.boxes.idByKind(.imbox)
        XCTAssertEqual(scoped, 42)
        XCTAssertEqual(hey.requests.filter { $0.path == "/boxes.json" }.count, 2)
    }

    func testTheKindsAreReadAfreshEveryTime() async throws {
        let index = ok(#"[{"id":1,"kind":"imbox","name":"Imbox"},{"id":9,"kind":"","name":"Unkinded"}]"#)
        let hey = mockHey(index, index)
        let client = try hey.client()
        let first = try await client.boxes.kinds()
        XCTAssertEqual(first, ["imbox": 1], "a box with no kind is left out")
        _ = try await client.boxes.kinds()
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testABoxGroupIsGatheredFromASelection() async throws {
        let hey = mockHey(ok(#"{"id":44}"#))
        _ = try await hey.client().boxes.createBoxGroup(boxId: 2, postingIds: [7, 8])
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/boxes/2/groups.json")
        XCTAssertEqual(request.body, #"{"posting_ids":[7,8]}"#)
    }

    func testAKindIsReadAsTheIndexNamesIt() {
        XCTAssertEqual(try BoxKind.parse("laterbox"), .replyLater)
        XCTAssertEqual(BoxKind.paperTrail.wire, "trailbox")
        assertThrowsSync(HeyError.codeUsage) { try BoxKind.parse("inbox") }
    }
}
