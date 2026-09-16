import Foundation
import XCTest

@testable import Hey

final class WorldTests: XCTestCase {
    private let csv = "email_address\njane.dawson@example.com\n"

    func testPublishingAddressesTheWorldAndAnswersThePostToken() async throws {
        let hey = mockHey(ok(identityJSON), status(302, nil, [("Location", "/world/posts/a1b2c3d4")]))
        let log = OperationLog()
        let token = try await hey.client(hooks: log).world.publish(subject: "On writing less", content: "<div>Fewer words, more meaning.</div>")
        XCTAssertEqual(token, "a1b2c3d4")
        let request = hey.requests[1]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/messages")
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)
        XCTAssertEqual(fields(request.body), [
            "acting_sender_id=100",
            "message[subject]=On writing less",
            "message[content]=<div>Fewer words, more meaning.</div>",
            "entry[addressed][directly]=\(WorldService.worldAddress)",
            "entry[status]=active",
        ])
        XCTAssertEqual(log.started.last, "World.PublishWorldPost:world_post:true:nil")
    }

    func testAMessageThatDidNotBecomeAPostSaysWhereItLanded() async throws {
        let hey = mockHey(ok(identityJSON), status(302, nil, [("Location", "/topics/4471829")]))
        let log = OperationLog()
        let refused = await assertThrows(
            HeyError.codeAPI, try await hey.client(hooks: log).world.publish(subject: "On writing less", content: "<div>Fewer words.</div>"))
        XCTAssertEqual(refused?.message, "the message was sent but did not become a HEY World post (landed on \"/topics/4471829\")")
        XCTAssertEqual(log.ended.last, "World.PublishWorldPost:api_error", "the hooks hear the failure the caller gets")
    }

    func testAnEditLeavesOutWhatItWasNotGiven() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/world/posts/a1b2c3d4")]))
        let log = OperationLog()
        try await hey.client(hooks: log).world.updatePost(token: "a1b2c3d4", subject: "On writing even less", content: "")
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "PATCH")
        XCTAssertEqual(request.path, "/world/posts/a1b2c3d4")
        XCTAssertEqual(fields(request.body), ["world_post[subject]=On writing even less"])
        XCTAssertEqual(log.started, ["World.UpdateWorldPost:world_post:true:nil"])
    }

    func testDeletingAPostNamesItByItsToken() async throws {
        let hey = mockHey(status(303, nil, [("Location", "/world/lists/david@example.com")]))
        let log = OperationLog()
        try await hey.client(hooks: log).world.deletePost(token: "a1b2c3d4")
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "DELETE")
        XCTAssertEqual(request.path, "/world/posts/a1b2c3d4")
        XCTAssertEqual(request.body, "")
        XCTAssertEqual(log.started, ["World.DeleteWorldPost:world_post:true:nil"])
    }

    func testSubscribersExportAsTheCsvHeyStreamed() async throws {
        let hey = mockHey(Answer(status: 200, body: csv, headers: [("Content-Type", "text/csv")]))
        let log = OperationLog()
        let exported = try await hey.client(hooks: log).world.exportSubscribers(listEmailAddress: "david@example.com")
        XCTAssertEqual(String(decoding: exported, as: UTF8.self), csv)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "GET")
        XCTAssertEqual(request.path, "/world/lists/david%40example.com/export.csv", "the list is named by an address, escaped as a path parameter is")
        XCTAssertEqual(request.header("Accept"), "text/csv")
        XCTAssertEqual(log.started, ["World.ExportWorldSubscribers:world_list:false:nil"])
    }

    func testAnImportUploadsTheCsvAsThePartHeyReads() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/world/lists/david@example.com/imports/1")]))
        let log = OperationLog()
        try await hey.client(hooks: log).world.importSubscribers(listEmailAddress: "david@example.com", filename: "subscribers.csv", csv: Data(csv.utf8))
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/world/lists/david%40example.com/imports")
        let contentType = request.header("Content-Type") ?? ""
        let prefix = "multipart/form-data; boundary="
        XCTAssertTrue(contentType.hasPrefix(prefix), contentType)
        let boundary = String(contentType.dropFirst(prefix.count))
        XCTAssertEqual(
            request.body,
            "--\(boundary)\r\nContent-Disposition: form-data; name=\"world_list_import[source]\"; filename=\"subscribers.csv\"\r\nContent-Type: application/octet-stream\r\n\r\n\(csv)\r\n--\(boundary)--\r\n")
        XCTAssertEqual(log.started, ["World.ImportWorldSubscribers:world_list:true:nil"])
    }

    func testAnImportWithoutACsvNameGetsOne() throws {
        XCTAssertEqual(importFilename(""), "subscribers.csv")
        XCTAssertEqual(importFilename("people"), "people.csv")
        XCTAssertEqual(importFilename("people.csv"), "people.csv")
        XCTAssertEqual(importFilename("PEOPLE.CSV"), "PEOPLE.CSV.csv", "the suffix check is case-sensitive, as Go's is")
        let (_, body) = try subscriberImportBody(filename: "say \"hi\"\\now", csv: Data())
        XCTAssertTrue(
            String(decoding: body, as: UTF8.self).contains("filename=\"say \\\"hi\\\"\\\\now.csv\""),
            "a quote or backslash in the name is escaped rather than ending the field")
    }

    func testThePostTokenIsTheHexRunAfterTheFirstPostsPathThatHasOne() {
        XCTAssertEqual(postToken("https://app.hey.com/world/posts/a1b2c3d4?welcome=1"), "a1b2c3d4")
        XCTAssertEqual(postToken("/world/posts/beef/edit"), "beef")
        XCTAssertEqual(postToken("/world/posts/?/world/posts/beef"), "beef", "an empty run is skipped for a later one")
        XCTAssertNil(postToken("/world/posts/"))
        XCTAssertNil(postToken("/world/posts/ABCD"), "only lowercase hex is a token")
        XCTAssertNil(postToken("/topics/4471829"))
    }

    func testAnImportFilenameWithALineBreakIsRefusedBeforeAnythingIsSent() async throws {
        let hey = mockHey()
        for filename in ["a\r\nContent-Type: text/html\r\n", "a\nb.csv", "a\rb"] {
            await assertThrows(
                HeyError.codeUsage,
                try await hey.client().world.importSubscribers(listEmailAddress: "list@example.com", filename: filename, csv: Data("x".utf8)))
        }
        XCTAssertEqual(hey.requests.count, 0)
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
