import Foundation
import XCTest

@testable import Hey

final class RouteTests: XCTestCase {
    func testEveryOperationHasOneRoute() {
        XCTAssertEqual(Routes.all.count, 131)
        XCTAssertEqual(Set(Routes.all.map(\.id)).count, Routes.all.count)
        XCTAssertTrue(Routes.all.allSatisfy { $0.retry.max >= 0 })
        XCTAssertTrue(Routes.getWorkflowStage.html)
        XCTAssertEqual(Routes.getOngoingTimeTrack.emptyOn, [404])
        XCTAssertEqual(Routes.listBoxes.pagination, .link)
        XCTAssertFalse(Routes.updateMessage.idempotent)
        XCTAssertTrue(Routes.completeCalendarTodo.idempotent)
        XCTAssertEqual(Routes.getClearances.service, "Clearances")
        XCTAssertEqual(Routes.getClearances.resource, "Contacts")
    }

    func testFillingAndRecognizingAPath() throws {
        let route = Routes.getBoxGroup
        XCTAssertEqual(try route.fill([1, 2]), "/boxes/1/groups/2")
        assertThrowsSync(HeyError.codeUsage) { try route.fill([1]) }
        XCTAssertEqual(route.recognize("/boxes/1/groups/2")?.map { "\($0.0)=\($0.1)" }, ["boxId=1", "groupId=2"])
        XCTAssertNil(route.recognize("/boxes/1/groups"))
        XCTAssertNil(route.recognize("/boxes//groups/2"))
        XCTAssertEqual(route.params.map(\.role), [.parent, .recording])
        XCTAssertEqual(try Routes.completeHabit.fill(["2026-03-04", 7]), "/calendar/days/2026-03-04/habits/7/completions")
        XCTAssertEqual(percentEncodeComponent("a b/c?d"), "a%20b%2Fc%3Fd")
        XCTAssertEqual(percentEncodeComponent("café"), "caf%C3%A9")
    }

    func testAPastedURLIsRecognisedWhateverItCarries() throws {
        let router = Router.shared
        let topic = try XCTUnwrap(router.recognize("https://app.hey.com/topics/456?x=1#y"))
        XCTAssertEqual(topic.operation, "GetTopic")
        XCTAssertEqual(topic.resourceId, "456")
        XCTAssertEqual(topic.params, [RouteMatch.Param(name: "topicId", value: "456")])
        XCTAssertEqual(router.recognize("/boxes.json")?.operation, "ListBoxes")
        XCTAssertEqual(router.recognize("/boxes/123.json")?.operation, "GetBox")
        XCTAssertEqual(router.recognize("/boxes/123.json/")?.resourceId, "123")
        let group = try XCTUnwrap(router.recognize("https://app.hey.com/boxes/123/groups/7.json?page=2"))
        XCTAssertEqual(group.params.map(\.value), ["123", "7"])
        XCTAssertEqual(group.resourceId, "7")
        XCTAssertEqual(router.recognize("/boxes/1")?.resource, "Boxes")
        XCTAssertNotNil(router.recognize("/boxes/1")?.operations[.get])
        XCTAssertNil(router.recognize("/nothing/like/this"))
        XCTAssertNil(router.recognize("https://app.hey.com/"))
    }

    func testTheDigestsMatchTheirKnownAnswers() {
        XCTAssertEqual(sha256Hex(Data()), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
        XCTAssertEqual(sha256Hex(Data("abc".utf8)), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
        XCTAssertEqual(
            sha256Hex(Data("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq".utf8)),
            "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1")
        XCTAssertEqual(sha256Hex(Data(repeating: 0x61, count: 1000)), "41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3")
        let hex = { (bytes: [UInt8]) in bytes.map { String(format: "%02x", $0) }.joined() }
        XCTAssertEqual(hex(md5(Data())), "d41d8cd98f00b204e9800998ecf8427e")
        XCTAssertEqual(hex(md5(Data("abc".utf8))), "900150983cd24fb0d6963f7d28e17f72")
        XCTAssertEqual(hex(md5(Data("The quick brown fox jumps over the lazy dog".utf8))), "9e107d9d372bb6826bd81d3542a419d6")
        XCTAssertEqual(Data(md5(Data("hello".utf8))).base64EncodedString(), "XUFAKrxLKna5cZ2REBfFkg==")
    }

    func testAFormAnswerNamesTheRightmostNumberInItsLocation() throws {
        func form(_ status: Int, _ location: String?) -> FormResponse {
            FormResponse.of(Response(
                status: status, headers: HTTPHeaders(location.map { [("Location", $0)] } ?? []), body: Data(),
                url: URL(string: "https://app.hey.com/x")!, fromCache: false, empty: true))
        }
        XCTAssertEqual(try form(302, "/calendar/events/42").extractId(), 42)
        XCTAssertEqual(try form(303, "https://app.hey.com/calendar/events/42/edit?x=1").extractId(), 42)
        XCTAssertEqual(try form(302, "/calendar/events/42/edit#top").extractId(), 42)
        assertThrowsSync(HeyError.codeAPI) { try form(302, "/calendar/events/new").extractId() }
        assertThrowsSync(HeyError.codeAPI) { try form(302, nil).extractId() }

        for location in [
            "/done?sig=distinctive-secret", "https://app.hey.com/done?sig=distinctive-secret#f",
            "https://u:distinctive-secret@app.hey.com/done", "//u:distinctive-secret@app.hey.com/done?sig=x",
            "::not a url::?sig=distinctive-secret",
        ] {
            let error = assertThrowsSync(HeyError.codeAPI) { try form(302, location).extractId() }
            XCTAssertFalse(String(describing: error).contains("distinctive"), String(describing: error))
        }
        XCTAssertEqual(redactLocation("/done?sig=x#y"), "/done")
        XCTAssertEqual(redactLocation("https://u:p@app.hey.com/done?sig=x"), "https://app.hey.com/done")
        XCTAssertEqual(redactLocation("//u:p@app.hey.com/done?sig=x"), "//app.hey.com/done")
    }

    func testAFormRequestCapturesTheRedirectAndIsNotRetried() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/workflows/7")]), status(503))
        let client = try hey.client()
        var operation = client.form(.post, "/workflows")
        operation.info = writeInfo(service: "Workflows", operation: "CreateWorkflow", resourceType: "workflow")
        operation.form([("workflow[name]", "Launch")])
        let response = try await client.sendForm(operation)
        XCTAssertEqual(response.status, 302)
        XCTAssertEqual(response.location, "/workflows/7")
        XCTAssertEqual(try response.extractId(), 7)
        let request = try XCTUnwrap(hey.requests.first)
        XCTAssertEqual(request.path, "/workflows", "a form path is sent as written")
        XCTAssertEqual(request.body, "workflow%5Bname%5D=Launch")
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)

        await assertThrows(HeyError.codeAPI, try await client.sendForm(client.form(.post, "/workflows")))
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testOnlyA302Or303CompletesAFormWrite() async throws {
        for accepted in [302, 303] {
            let hey = mockHey(status(accepted, nil, [("Location", "/workflows/7")]))
            let client = try hey.client()
            let response = try await client.sendForm(client.form(.post, "/workflows"))
            XCTAssertEqual(response.status, accepted)
            XCTAssertEqual(try response.extractId(), 7)
        }
        for refused in [301, 307, 308] {
            let hey = mockHey(status(refused, nil, [("Location", "/workflows/7")]))
            let client = try hey.client()
            let error = await assertThrows(HeyError.codeAPI, try await client.sendForm(client.form(.post, "/workflows")))
            XCTAssertEqual(error?.httpStatus, refused)
            XCTAssertEqual(hey.requests.count, 1, "a \(refused) is not followed by a form request either")
        }
    }
}
