import Foundation
import XCTest

@testable import Hey

final class MessagesTests: XCTestCase {
    private func entry(_ request: RecordedRequest) throws -> [String: Any] {
        try XCTUnwrap(try jsonObject(request.body)["entry"] as? [String: Any])
    }

    private func addressed(_ request: RecordedRequest) throws -> [String: Any] {
        try XCTUnwrap(try entry(request)["addressed"] as? [String: Any])
    }

    func testSendRefusesAMessageAddressedToNobody() async throws {
        let hey = mockHey()
        await assertThrows(HeyError.codeUsage, try await hey.client().messages.send(MessageContent(subject: "Hi", content: "Body")))
        XCTAssertTrue(hey.requests.isEmpty)
    }

    func testSendDeliversAsTheChosenSenderWithTheRecipientsThatNameSomebody() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().messages.send(
            MessageContent(subject: "Subject", content: "<div>Body</div>", to: ["someone@example.com"], actingSenderId: 314))
        XCTAssertEqual(hey.requests.count, 1)
        let sent = try jsonObject(hey.requests[0].body)
        XCTAssertEqual(sent["acting_sender_id"] as? Int, 314)
        XCTAssertEqual((sent["message"] as? [String: Any])?["subject"] as? String, "Subject")
        XCTAssertEqual(Set(try addressed(hey.requests[0]).keys), ["directly"])
        XCTAssertNil(try entry(hey.requests[0])["status"])
    }

    func testSendResolvesTheDefaultSenderWhenNoneIsChosen() async throws {
        let hey = mockHey(ok(identityJSON), ok(""))
        try await hey.client().messages.send(MessageContent(subject: "Subject", content: "Body", to: ["a@example.com"]))
        XCTAssertEqual(hey.requests[0].path, "/identity.json")
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["acting_sender_id"] as? Int, 100)
    }

    private let lunchMessage = MessageContent(
        subject: "Lunch on Friday", content: "Are you free at noon?", to: ["maria@example.com"], actingSenderId: 314)

    func testDeliveringAnswersTheEntryHEYDelivered() async throws {
        let sentNow = #"{"id":2201,"topic_id":880,"subject":"Lunch on Friday","delayed":false}"#
        let hey = mockHey(ok(sentNow), ok(sentNow), ok(sentNow))
        let client = try hey.client()
        let expected = SentMessage(id: 2201, topicId: 880, subject: "Lunch on Friday", delayed: false)
        let sent = try await client.messages.send(lunchMessage)
        XCTAssertEqual(sent, expected)
        let draftSent = try await client.messages.sendDraft(
            entryId: 2201,
            draft: DraftContent(subject: "Lunch on Friday", content: "Are you free at noon?", to: ["maria@example.com"], actingSenderId: 314))
        XCTAssertEqual(draftSent, expected)
        let replied = try await client.entries.reply(
            entryId: 1990, reply: ReplyContent(actingSenderId: 314, subject: "Lunch on Friday", content: "Noon works.", to: ["maria@example.com"]))
        XCTAssertEqual(replied, expected)
    }

    func testDeliveringAnswersAnUndoableDelivery() async throws {
        let hey = mockHey(ok(#"{"id":2201,"topic_id":880,"subject":"Lunch on Friday","delayed":true,"notice":"Message sent","undo_action":"https://app.hey.com/topics/880/undo_send","undo_timeout":12}"#))
        let sent = try await hey.client().messages.send(lunchMessage)
        XCTAssertEqual(
            sent,
            SentMessage(
                id: 2201, topicId: 880, subject: "Lunch on Friday", delayed: true, notice: "Message sent",
                undoAction: "https://app.hey.com/topics/880/undo_send", undoTimeout: 12))
    }

    func testAHEYThatServesNoIdsAnswersWithoutThemAndReadsTheDelayFromItsUndo() async throws {
        let hey = mockHey(ok("{}"), ok(#"{"notice":"Message sent","undo_action":"https://app.hey.com/topics/880/undo_send","undo_timeout":12}"#))
        let client = try hey.client()
        let plain = try await client.messages.send(lunchMessage)
        XCTAssertEqual(plain, SentMessage())
        let delayed = try await client.messages.send(lunchMessage)
        XCTAssertNil(delayed.id)
        XCTAssertNil(delayed.topicId)
        XCTAssertEqual(delayed.delayed, true)
    }

    /// The message has gone out by the time its answer is read, so an answer the SDK cannot read
    /// must not turn into an error: a caller told the send failed would send it again.
    func testAnUnreadableAnswerDoesNotFailADelivery() async throws {
        let hey = mockHey(ok(""), ok("<html>Message sent</html>"), ok(#"{"topic_id":[880]}"#), status(204, nil, []))
        let client = try hey.client()
        for _ in 0..<4 {
            let sent = try await client.messages.send(lunchMessage)
            XCTAssertEqual(sent, SentMessage())
        }
    }

    func testADraftIsSavedDraftedAndAnswersItsEntryId() async throws {
        let hey = mockHey(status(204, nil, [("Location", "https://app.hey.com/messages/777")]), ok(""), ok(""))
        let client = try hey.client()
        var draft = DraftContent(subject: "Subject", content: "Body", actingSenderId: 314)
        let entryId = try await client.messages.createDraft(draft)
        XCTAssertEqual(entryId, 777)
        XCTAssertEqual(try entry(hey.requests[0])["status"] as? String, "drafted")
        let saved = try addressed(hey.requests[0])
        XCTAssertEqual(Set(saved.keys), ["directly", "copied", "blindcopied"])
        XCTAssertEqual((saved["directly"] as? [String])?.count, 0)

        try await client.messages.updateDraft(entryId: 777, draft: draft)
        XCTAssertEqual(hey.requests[1].method, "PUT")
        XCTAssertEqual(hey.requests[1].path, "/messages/777.json")

        draft.to = ["someone@example.com"]
        try await client.messages.sendDraft(entryId: 777, draft: draft)
        XCTAssertNil(try entry(hey.requests[2])["status"])
        XCTAssertEqual((try addressed(hey.requests[2])["directly"] as? [String])?.first, "someone@example.com")
    }

    func testAScheduledDraftCarriesItsDeliveryAndARevisionWithoutOneClearsIt() async throws {
        let hey = mockHey(status(204, nil, [("Location", "/messages/777")]), ok(""))
        let client = try hey.client()
        var draft = DraftContent(subject: "S", content: "B", actingSenderId: 314, schedule: DeliverySchedule(date: "tomorrow", hour: 9))
        _ = try await client.messages.createDraft(draft)
        let scheduled = try entry(hey.requests[0])
        XCTAssertEqual(scheduled["scheduled_delivery"] as? String, "true")
        XCTAssertEqual(scheduled["scheduled_delivery_at_date"] as? String, "tomorrow")
        XCTAssertEqual(scheduled["scheduled_delivery_at_hour"] as? String, "9")
        draft.schedule = nil
        try await client.messages.updateDraft(entryId: 777, draft: draft)
        XCTAssertNil(try entry(hey.requests[1])["scheduled_delivery"])
    }

    func testSendingADraftIsNeverRetried() async throws {
        let hey = mockHey(status(503))
        await assertThrows(
            HeyError.codeAPI,
            try await hey.client().messages.sendDraft(entryId: 777, draft: DraftContent(subject: "S", content: "B", to: ["a@example.com"], actingSenderId: 1)))
        XCTAssertEqual(hey.requests.count, 1)
        await assertThrows(
            HeyError.codeUsage, try await hey.client().messages.sendDraft(entryId: 777, draft: DraftContent(subject: "S", content: "B", actingSenderId: 1)))
    }

    func testADraftSaveWhoseLocationNamesNoIdEndsTheOperationWithThatError() async throws {
        let hey = mockHey(status(204), status(204, nil, [("Location", "/messages/new")]))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        let draft = DraftContent(subject: "s", content: "c", actingSenderId: 100)
        for _ in 1...2 {
            transcript.clear()
            await assertThrows(HeyError.codeAPI, try await client.messages.createDraft(draft))
            XCTAssertEqual(transcript.log.last, "end:CreateMessage:api_error")
            XCTAssertEqual(transcript.log.filter { $0.hasPrefix("end:") }.count, 1)
        }
    }

    func testAnEmptyLocationNamesNoEntryId() async throws {
        let hey = mockHey(status(204, nil, [("Location", "")]))
        let client = try hey.client()
        let error = await assertThrows(
            HeyError.codeAPI, try await client.messages.createDraft(DraftContent(subject: "s", content: "c", actingSenderId: 100)))
        XCTAssertTrue(error?.message.contains("names no entry id") == true, error?.message ?? "")
    }

    func testALocationWithoutAnIdIsNeverQuotedWhole() async throws {
        let hey = mockHey(status(204, nil, [("Location", "/messages/new?sig=distinctive-secret")]))
        final class Seen: HeyHooks, @unchecked Sendable {
            let lock = NSLock()
            var errors: [String] = []
            func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
                if let error = result.error { lock.withLock { errors.append(String(describing: error)) } }
            }
        }
        let seen = Seen()
        let client = try hey.client(hooks: seen)
        let error = await assertThrows(
            HeyError.codeAPI,
            try await client.messages.createDraft(DraftContent(subject: "s", content: "c", to: ["a@example.com"], actingSenderId: 100)))
        let told = (error.map { "\($0.message) \($0)" } ?? "") + seen.lock.withLock { seen.errors.joined() }
        XCTAssertFalse(told.contains("distinctive"), told)
        XCTAssertEqual(seen.lock.withLock { seen.errors.count }, 1)
    }
}
