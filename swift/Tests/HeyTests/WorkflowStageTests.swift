import Foundation
import XCTest

@testable import Hey

final class WorkflowStageTests: XCTestCase {
    private let page = #"<section id="container_workflow_stage_5512"><h2>Applied<span class="u-for-screen-reader"> stage</span></h2>"#
        + #"<div class="workflow__card" id="topic_4471829" data-identifier="91"><h3>Application<span class="u-for-screen-reader">Thread: Application</span></h3>"#
        + #"<p class="card__detail"><span class="u-for-screen-reader">,</span>3 emails</p></div>"#
        + #"<div class="workflow__card" id="topic_4471830" data-identifier="92"><div id="topic_1" data-identifier="93"><h3>Inner</h3></div></div>"#
        + #"<div id="topic_not-a-number" data-identifier="93"></div>"#
        + #"<div id="topic_5" data-identifier="94"><p class="card__detail">many emails</p></div></section>"#

    func testTheStageIsReadOutOfThePage() throws {
        let view = try WorkflowStageView.parse(page, stageId: 5512)
        XCTAssertEqual(view.id, 5512)
        XCTAssertEqual(view.name, "Applied")
        XCTAssertEqual(view.topics.count, 2)
        XCTAssertEqual(view.topics[0].topicId, 4471829)
        XCTAssertEqual(view.topics[0].stagingId, 91)
        XCTAssertEqual(view.topics[0].subject, "Application")
        XCTAssertEqual(view.topics[0].entryCount, 3)
        XCTAssertEqual(view.topics[1].topicId, 4471830)
        XCTAssertEqual(view.topics[1].subject, "Inner", "a card inside a card is that card's content")
        XCTAssertEqual(view.topics[1].entryCount, 0)
    }

    func testAPageWithoutTheStageIsNotFound() {
        assertThrowsSync(HeyError.codeNotFound) { try WorkflowStageView.parse("<html></html>", stageId: 1) }
    }

    func testTheServiceReadsThePageAsHtml() async throws {
        let hey = mockHey(ok(page, [("Content-Type", "text/html; charset=utf-8")]))
        let view = try await hey.client().workflows.stage(workflowId: 8801, stageId: 5512)
        XCTAssertEqual(view.name, "Applied")
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].path, "/workflows/8801/stages/5512")
    }

    func testAPageNestedBeyondAnyStackIsStillRead() throws {
        let depth = 200_000
        var page = #"<section id="container_workflow_stage_5"><h2>Applied</h2>"#
        page += String(repeating: "<div>", count: depth)
        page += #"<div id="topic_9" data-identifier="77"><h3>Deep</h3><p class="card__detail">2 messages</p></div>"#
        page += String(repeating: "</div>", count: depth)
        page += "</section>"
        let view = try WorkflowStageView.parse(page, stageId: 5)
        XCTAssertEqual(view.name, "Applied")
        XCTAssertEqual(view.topics.map(\.topicId), [9])
        XCTAssertEqual(view.topics.first?.subject, "Deep")
    }

    func testAHeadingThatIsItselfHiddenIsNotTheName() throws {
        for hidden in ["sr-only", "screen-reader-only", "u-for-screen-reader", "visually-hidden"] {
            let page = #"<section id="container_workflow_stage_5"><h2 class="\#(hidden)">Secret</h2>"#
                + #"<div id="topic_9" data-identifier="77"><h3 class="\#(hidden)">Hush</h3><p class="card__detail">1 message</p></div></section>"#
            let view = try WorkflowStageView.parse(page, stageId: 5)
            XCTAssertEqual(view.name, "", hidden)
            XCTAssertEqual(view.topics.count, 1, hidden)
            XCTAssertEqual(view.topics.first?.subject, "", hidden)
        }
    }

    func testAPageOfStrayCloseTagsIsReadInLinearTime() throws {
        let n = 200_000
        var page = #"<section id="container_workflow_stage_5"><h2>Applied</h2>"#
        page += String(repeating: "<div>", count: n)
        page += String(repeating: "</span>", count: n)
        page += #"<div id="topic_9" data-identifier="77"><h3>Deep</h3><p class="card__detail">2 messages</p></div></section>"#
        let clock = ContinuousClock()
        let started = clock.now
        XCTAssertEqual(try WorkflowStageView.parse(page, stageId: 5).topics.map(\.topicId), [9])
        let elapsed = clock.now - started
        XCTAssertLessThan(elapsed, .seconds(5), "stray close tags took \(elapsed)")
    }
}
