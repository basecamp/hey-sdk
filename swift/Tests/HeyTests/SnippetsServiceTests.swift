import Foundation
import XCTest

@testable import Hey

final class SnippetsServiceTests: XCTestCase {
    func testASnippetIsSavedEditedAndThrownAwayAsForms() async throws {
        let redirect = status(302, nil, [("Location", "/snippets")])
        let hey = mockHey(redirect, redirect, redirect)
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.snippets.create(name: "Sign-off", content: "<div>Cheers, Jane</div>")
        try await client.snippets.update(snippetId: 12, name: "", content: "<div>Best, Jane</div>")
        try await client.snippets.delete(snippetId: 12)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/snippets")
        XCTAssertEqual(hey.requests[0].header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(hey.requests[0].header("Accept"), browserAcceptHeader)
        XCTAssertEqual(fields(hey.requests[0].body), ["snippet[name]=Sign-off", "snippet[content]=<div>Cheers, Jane</div>"])
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, "/snippets/12")
        XCTAssertEqual(fields(hey.requests[1].body), ["snippet[content]=<div>Best, Jane</div>"], "an empty field is left out, and so left as it was")
        XCTAssertEqual(hey.requests[2].method, "DELETE")
        XCTAssertEqual(hey.requests[2].path, "/snippets/12")
        XCTAssertEqual(hey.requests[2].body, "")
        XCTAssertEqual(log.started, ["Snippets.CreateSnippet:snippet:true:nil", "Snippets.UpdateSnippet:snippet:true:12", "Snippets.DeleteSnippet:snippet:true:12"])
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
