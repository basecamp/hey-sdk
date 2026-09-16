import Foundation
import XCTest

@testable import Hey

final class ClipsServiceTests: XCTestCase {
    func testAClipIsSavedAndThrownAwayAsForms() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/topics/4471829")]), status(302, nil, [("Location", "/clips")]))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.clips.create(entryId: 4471829, content: "<div>The part worth keeping.</div>")
        try await client.clips.delete(clipId: 7)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/clips")
        XCTAssertEqual(hey.requests[0].header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(hey.requests[0].header("Accept"), browserAcceptHeader)
        XCTAssertEqual(fields(hey.requests[0].body), ["clip[entry_id]=4471829", "clip[content]=<div>The part worth keeping.</div>"])
        XCTAssertEqual(hey.requests[1].method, "DELETE")
        XCTAssertEqual(hey.requests[1].path, "/clips/7")
        XCTAssertEqual(hey.requests[1].body, "")
        XCTAssertEqual(
            log.started, ["Clips.CreateClip:clip:true:4471829", "Clips.DeleteClip:clip:true:7"],
            "a new clip names the entry it comes from, a deleted one the clip")
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
