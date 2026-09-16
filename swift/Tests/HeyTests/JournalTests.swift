import Foundation
import XCTest

@testable import Hey

final class JournalTests: XCTestCase {
    func testAJournalEntryIsReadAndWrittenAsItsContent() async throws {
        let hey = mockHey(
            status(204),
            ok(#"{"id":1,"type":"Calendar::JournalEntry","content":"plain","content_html":"<p>rich</p>"}"#),
            ok(#"{"id":1,"type":"Calendar::JournalEntry","content":"plain","content_html":""}"#),
            ok(#"{"id":1,"type":"Calendar::JournalEntry","content":"written"}"#),
            status(204))
        let transcript = GroupAHooksLog()
        let client = try hey.client(hooks: transcript)
        let missing = try await client.journal.entry(day: "2026-09-15")
        XCTAssertNil(missing, "a day without an entry answers nothing, which is nil rather than a body that will not decode")
        XCTAssertEqual(hey.requests[0].path, "/calendar/days/2026-09-15/journal_entry.json")
        let rich = try await client.journal.getContent(day: "2026-09-15")
        XCTAssertEqual(rich, "<p>rich</p>")
        let plain = try await client.journal.getContent(day: "2026-09-15")
        XCTAssertEqual(plain, "plain", "a blank rendered body is not the entry")
        let written = try await client.journal.updateContent(day: "2026-09-15", content: "written")
        XCTAssertEqual(written?.content, "written")
        XCTAssertEqual(hey.requests[3].method, "PATCH")
        XCTAssertEqual(try groupAMember(hey.requests[3].body, "calendar_journal_entry")["content"] as? String, "written")
        let removed = try await client.journal.updateContent(day: "2026-09-15", content: "")
        XCTAssertNil(removed, "empty content removes the entry, which HEY answers with nothing")
        XCTAssertEqual(
            transcript.operations.map(\.operation),
            ["GetJournalEntry", "GetJournalContent", "GetJournalContent", "UpdateJournalEntry", "UpdateJournalEntry"])
    }
}
