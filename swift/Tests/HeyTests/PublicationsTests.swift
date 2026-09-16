import Foundation
import XCTest

@testable import Hey

final class PublicationsTests: XCTestCase {
    func testPublishingReadsThePublicLinkBackUnderTheOperationThatAskedForIt() async throws {
        let hey = mockHey(
            status(302, nil, [("Location", "/topics/5/sharing")]),
            ok(#"{"published":true,"url":"https://app.hey.com/p/abc"}"#),
            status(302, nil, [("Location", "/topics/5")]))
        let transcript = GroupAHooksLog()
        let client = try hey.client(hooks: transcript)
        let publication = try await client.publications.publish(topicId: 5)
        XCTAssertTrue(publication.published)
        XCTAssertEqual(publication.url, "https://app.hey.com/p/abc")
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/topics/5/publication", "a form post goes to the path as written")
        XCTAssertEqual(hey.requests[0].header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(hey.requests[0].header("Accept"), browserAcceptHeader)
        XCTAssertEqual(hey.requests[0].body, "")
        XCTAssertEqual(hey.requests[1].method, "GET")
        XCTAssertEqual(hey.requests[1].path, "/topics/5/publication.json")
        XCTAssertEqual(transcript.operations.map(\.operation), ["CreateTopicPublication"], "the read-back is quiet: one operation for two requests")
        XCTAssertEqual(transcript.operations.first?.resourceType, "publication")
        XCTAssertEqual(transcript.operations.first?.resourceId, 5)
        XCTAssertEqual(transcript.requests.count, 2, "while the request hooks hear both")

        try await client.publications.unpublish(topicId: 5)
        XCTAssertEqual(hey.requests[2].method, "DELETE")
        XCTAssertEqual(hey.requests[2].path, "/topics/5/publication")
        XCTAssertEqual(hey.requests[2].body, "")
        XCTAssertEqual(transcript.operations[1].operation, "DeleteTopicPublication")
        XCTAssertEqual(transcript.operations[1].resourceId, 5)
    }

    func testAThreadThatMayNotBePublishedIsRefusedWithoutAReadBack() async throws {
        let hey = mockHey(status(403, #"{"error":"not eligible"}"#))
        await assertThrows(HeyError.codeForbidden, try await hey.client().publications.publish(topicId: 5))
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testAPublishWhoseReadBackFailsEndsItsOneOperationWithThatFailure() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/topics/5/sharing")]), status(404, #"{"error":"gone"}"#))
        let transcript = Transcript()
        await assertThrows(HeyError.codeNotFound, try await hey.client(hooks: transcript).publications.publish(topicId: 5))
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("start:") }.count, 1)
        XCTAssertEqual(transcript.log.last, "end:CreateTopicPublication:not_found")
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("response:") }, ["response:302:nil", "response:404:not_found"])
    }
}
