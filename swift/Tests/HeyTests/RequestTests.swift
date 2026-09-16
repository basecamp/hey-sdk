import Foundation
import XCTest

@testable import Hey

final class RequestTests: XCTestCase {
    func testAGeneratedReadCarriesTheHeadersAndTheJSONSuffix() async throws {
        let hey = mockHey(ok(#"[{"id":7,"kind":"imbox","name":"Imbox"}]"#))
        let boxes = try await hey.client().boxes.list()

        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(request.method, "GET")
        XCTAssertEqual(request.path, "/boxes.json")
        XCTAssertEqual(request.header("Authorization"), "Bearer test-token")
        XCTAssertEqual(request.header("Accept"), "application/json")
        XCTAssertEqual(request.header("User-Agent"), HeyConfig.defaultUserAgent)
        XCTAssertEqual(boxes.value.map(\.id), [7])
        XCTAssertEqual(boxes.value.first?.name, "Imbox")
    }

    func testPathParametersAreFilledAndEncoded() async throws {
        let hey = mockHey(ok(#"{"id":123,"kind":"imbox","name":"Imbox"}"#), ok(#"{"id":1,"type":"Calendar::JournalEntry","date":"2026-03-04"}"#))
        let client = try hey.client()
        _ = try await client.boxes.get(boxId: 123)
        _ = try await client.journal.getEntry(day: "2026-03-04")
        XCTAssertEqual(hey.requests[0].path, "/boxes/123.json")
        XCTAssertEqual(hey.requests[1].path, "/calendar/days/2026-03-04/journal_entry.json")
        XCTAssertEqual(try Routes.getBox.fill(["a/b"]), "/boxes/a%2Fb")
    }

    func testAnHTMLRouteIsAskedForAsWritten() async throws {
        let html = #"<section id="container_workflow_stage_5"><h2>Applied</h2></section>"#
        let hey = mockHey(ok(html, [("Content-Type", "text/html")]))
        let page = try await hey.client().workflows.getStage(workflowId: 8801, stageId: 5)
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.path, "/workflows/8801/stages/5")
        XCTAssertEqual(request.header("Accept"), "text/html")
        XCTAssertEqual(page, html)
    }

    func testQueryParametersGoOutWhenTheyAreSet() async throws {
        let hey = mockHey(ok(#"{"id":88}"#), ok(#"{"id":88}"#))
        let client = try hey.client()
        _ = try await client.contacts.get(contactId: 88, options: GetContactOptions(page: "older-threads"))
        _ = try await client.contacts.get(contactId: 88)
        XCTAssertEqual(hey.requests[0].query("page"), "older-threads")
        XCTAssertNil(hey.requests[1].query("page"))
        XCTAssertEqual(hey.requests[1].path, "/contacts/88.json")
    }

    func testABodyIsEncodedAsTheModelSaysAndNilsAreLeftOff() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().postings.markSeen(body: MarkPostingsRequestContent(postingIds: [1, 2]))
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.header("Content-Type"), "application/json")
        let body = try jsonObject(request.body)
        XCTAssertEqual(body["posting_ids"] as? [Int], [1, 2])
        XCTAssertEqual(Set(body.keys), ["posting_ids"])
    }

    func testLargeIdsKeepTheirPrecision() async throws {
        let hey = mockHey(ok(#"{"id":9007199254740993,"kind":"imbox","name":"Large"}"#))
        let box = try await hey.client().boxes.get(boxId: 9_007_199_254_740_993)
        XCTAssertEqual(box.value.id, 9_007_199_254_740_993)
        XCTAssertEqual(hey.requests.first?.path, "/boxes/9007199254740993.json")
    }

    func testAMissingRequiredMemberIsAnAPIError() async throws {
        let hey = mockHey(ok(#"{"id":12345,"title":"Imbox"}"#))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client().boxes.get(boxId: 12345))
        XCTAssertEqual(error?.isRetryable, false)
        XCTAssertEqual(error?.hint, "body does not decode: missing required field 'kind'")
    }

    func testAnEmptyOnStatusAnswersNilAndOthersStillFail() async throws {
        let hey = mockHey(
            status(404, #"{"error":"Not found"}"#),
            ok(#"{"id":123,"type":"Calendar::TimeTrack","starts_at":"2026-03-04T10:00:00Z"}"#),
            status(500, #"{"error":"boom"}"#))
        let client = try hey.client { $0.enableRetry = false }
        let none = try await client.timeTracks.getOngoing()
        let some = try await client.timeTracks.getOngoing()
        XCTAssertNil(none)
        XCTAssertNotNil(some)
        let error = await assertThrows(HeyError.codeAPI, try await client.timeTracks.getOngoing())
        XCTAssertEqual(error?.httpStatus, 500)
    }

    func testABodyThatWillNotDecodeIsAnAPIErrorNamingTheOperation() async throws {
        let hey = mockHey(ok(#"{"id":"not a number"}"#))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client().boxes.get(boxId: 1))
        XCTAssertEqual(error?.httpStatus, 200)
        XCTAssertEqual(error?.isRetryable, false)
        XCTAssertEqual(error?.message, "GetBox: unexpected JSON in the response")
        XCTAssertFalse(String(describing: error).contains("not a number"), "the body is not quoted")
    }

    func testABodyPastTheCapIsRefused() async throws {
        let hey = mockHey(ok("[" + String(repeating: "1,", count: 2000) + "1]"))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client { $0.maxResponseBodyBytes = 100 }.boxes.list())
        XCTAssertEqual(error?.responseTooLarge, true)
    }

    func testARawPathGetsTheSameTreatment() async throws {
        let hey = mockHey(ok(#"{"ok":true}"#))
        let client = try hey.client()
        var operation = client.request(.get, "/anything?x=1")
        operation.query("y", "2")
        let response = try await client.execute(operation)
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.path, "/anything.json")
        XCTAssertEqual(request.query("x"), "1")
        XCTAssertEqual(request.query("y"), "2")
        XCTAssertEqual(response.status, 200)
    }

    func testARawPathsQueryGoesOutAsWritten() async throws {
        let hey = mockHey(ok(#"{"ok":true}"#))
        let client = try hey.client()
        try await client.execute(client.request(.get, "/search?q=a%26b&plus=1%2B1"))
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.encodedQuery, "q=a%26b&plus=1%2B1")
        XCTAssertEqual(request.query("q"), "a&b")
        XCTAssertEqual(request.query("plus"), "1+1")
    }

    func testAQueryValueIsEncodedWhole() async throws {
        let hey = mockHey(ok(#"{"ok":true}"#))
        let client = try hey.client()
        var operation = client.request(.get, "/search")
        operation.query("q", "a b&c=d+e/f")
        try await client.execute(operation)
        XCTAssertEqual(hey.requests.first?.encodedQuery, "q=a%20b%26c%3Dd%2Be%2Ff")
        XCTAssertEqual(hey.requests.first?.query("q"), "a b&c=d+e/f")
    }

    func testADocumentIsHeldToTheConfiguredCapWhateverTheAcceptListSays() async throws {
        XCTAssertTrue(isParsed("text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"))
        XCTAssertTrue(isParsed("application/vnd.api+json"))
        XCTAssertTrue(isParsed(""))
        XCTAssertFalse(isParsed("image/png"))
        XCTAssertFalse(isParsed("*/*"))

        let hey = mockHey(ok("<html>" + String(repeating: "x", count: 2000) + "</html>", [("Content-Type", "text/html")]))
        let client = try hey.client { $0.maxResponseBodyBytes = 100 }
        let error = await assertThrows(HeyError.codeAPI, try await client.sendForm(client.form(.get, "/workflows/new")))
        XCTAssertEqual(error?.responseTooLarge, true)
    }

    func testARecordingIsRecognisedByEitherSpellingOfItsType() throws {
        let decoder = JSONDecoder()
        let direct = try decoder.decode(Recording.self, from: Data(#"{"id":1,"type":"CalendarEvent"}"#.utf8))
        let namespaced = try decoder.decode(Recording.self, from: Data(#"{"id":1,"type":"Calendar::Event"}"#.utf8))
        XCTAssertTrue(direct.isCalendarEvent)
        XCTAssertTrue(namespaced.isCalendarEvent)
        XCTAssertFalse(direct.isCalendarTodo)
        XCTAssertTrue(try decoder.decode(Recording.self, from: Data(#"{"id":2,"type":"CalendarTodo"}"#.utf8)).isCalendarTodo)
    }

    func testARecordingHoldsItsParentRecording() throws {
        let body = #"{"id":2,"type":"CalendarTodo","parent":{"id":1,"type":"Calendar::Event","parent":{"id":0,"type":"Calendar::Event"}}}"#
        let recording = try JSONDecoder().decode(Recording.self, from: Data(body.utf8))
        XCTAssertEqual(recording.parent?.id, 1)
        XCTAssertEqual(recording.parent?.parent?.id, 0)
        XCTAssertNil(recording.parent?.parent?.parent)
        let again = try JSONDecoder().decode(Recording.self, from: try heyJSONEncoder().encode(recording))
        XCTAssertEqual(again, recording, "the parent travels back out as the member it came in as")
        var edited = recording
        edited.parent = nil
        XCTAssertNil(edited.parent)
        XCTAssertFalse(String(decoding: try heyJSONEncoder().encode(edited), as: UTF8.self).contains("parent\""))
    }

    func testAStatusTakenForNothingThereIsNotReadPastTheCap() async throws {
        let big = String(repeating: "x", count: 2000)
        let hey = mockHey(status(404, #"{"error":"\#(big)"}"#), status(302, "<html>\(big)</html>", [("Location", "/workflows/7")]))
        let client = try hey.client { $0.maxResponseBodyBytes = 100 }
        let none = try await client.timeTracks.getOngoing()
        XCTAssertNil(none, "an oversized 404 the route takes for nothing there is still nothing there")
        let answer = try await client.sendForm(client.form(.post, "/workflows"))
        XCTAssertEqual(answer.location, "/workflows/7", "and an oversized redirect a form takes for its answer still answers")
    }

    func testAnOperationPrintsWhereItGoesAndNotWhatItCarries() throws {
        let client = try mockHey().client()
        var operation = client.request(.post, "/search?token=secret")
        operation.query("q", "private words")
        operation.form([("password", "hunter2")])
        let printed = operation.description
        XCTAssertFalse(printed.contains("secret") || printed.contains("private") || printed.contains("hunter2"), printed)
        XCTAssertTrue(printed.contains("/search") && printed.contains("q"), printed)
    }
}
