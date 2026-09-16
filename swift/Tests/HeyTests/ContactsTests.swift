import Foundation
import XCTest

@testable import Hey

final class ContactsTests: XCTestCase {
    private func contact(_ request: RecordedRequest) throws -> [String: Any] {
        try XCTUnwrap(try jsonObject(request.body)["contact"] as? [String: Any])
    }

    func testACreateSendsTheContactAndTheAccountUserItIsFiledUnder() async throws {
        let hey = mockHey(ok(#"{"id":77,"name":"Ann"}"#), ok(#"{"id":78,"name":"Bob"}"#))
        let client = try hey.client()
        let created = try await client.contacts.createContact(
            ContactParams(name: "Ann", emailAddress: "ann@example.com", aliasEmailAddresses: ["a@example.com"], accountUserId: 1000))
        XCTAssertEqual(created.id, 77)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/contacts.json")
        XCTAssertEqual(try jsonObject(hey.requests[0].body)["acting_user_id"] as? Int, 1000)
        let sent = try contact(hey.requests[0])
        XCTAssertEqual(sent["name"] as? String, "Ann")
        XCTAssertEqual(sent["email_address"] as? String, "ann@example.com")
        XCTAssertEqual(sent["alias_email_addresses"] as? [String], ["a@example.com"])

        _ = try await client.contacts.createContact(ContactParams(name: "Bob"))
        XCTAssertNil(try jsonObject(hey.requests[1].body)["acting_user_id"], "left unset, HEY files the contact under the first account")
        XCTAssertNil(try contact(hey.requests[1])["email_address"], "an empty address is not sent as one")
        XCTAssertNil(try contact(hey.requests[1])["alias_email_addresses"])
    }

    func testAScopedClientFilesTheContactUnderItsAccountAndRefusesAnother() async throws {
        let hey = mockHey(ok(identityJSON), ok(#"{"id":77,"name":"Ann"}"#))
        let scoped = try await hey.client().forAccount(42)
        _ = try await scoped.contacts.createContact(ContactParams(name: "Ann", emailAddress: "ann@example.com"))
        XCTAssertEqual(try jsonObject(hey.requests[1].body)["acting_user_id"] as? Int, 1000, "the identity's user on the scoped account")
        let error = await assertThrows(HeyError.codeUsage, try await scoped.contacts.createContact(ContactParams(name: "Ann", accountUserId: 7000)))
        XCTAssertEqual(error?.message, "account user 7000 does not belong to selected account 42")
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testAnUpdateReadsTheContactFirstAndFillsInWhatTheCallerLeftUnset() async throws {
        let current = #"{"id":77,"name":"Ann","email_address":"ann@example.com","aliases":[{"id":78,"email_address":"a@example.com"},{"id":79,"email_address":"annie@example.com"}]}"#
        let hey = mockHey(ok(current), ok(#"{"id":77,"name":"Ann Smith"}"#), ok(current), ok(#"{"id":77,"name":"Ann"}"#))
        let client = try hey.client()
        let updated = try await client.contacts.updateContact(contactId: 77, params: ContactParams(name: "Ann Smith"))
        XCTAssertEqual(updated.name, "Ann Smith")
        XCTAssertEqual(hey.requests[0].method, "GET")
        XCTAssertEqual(hey.requests[0].path, "/contacts/77.json")
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, "/contacts/77.json")
        let merged = try contact(hey.requests[1])
        XCTAssertEqual(merged["name"] as? String, "Ann Smith")
        XCTAssertEqual(merged["email_address"] as? String, "ann@example.com", "the address is kept from the contact")
        XCTAssertEqual(
            merged["alias_email_addresses"] as? [String], ["a@example.com", "annie@example.com"],
            "and so are the aliases, since HEY removes any not submitted")

        _ = try await client.contacts.updateContact(contactId: 77, params: ContactParams(aliasEmailAddresses: []))
        let cleared = try contact(hey.requests[3])
        XCTAssertEqual(cleared["name"] as? String, "Ann")
        XCTAssertEqual((cleared["alias_email_addresses"] as? [String])?.count, 0, "an explicit empty list clears the aliases")
    }

    func testAClashIsAConflictInHeysWordsThatNamesTheContactsItWouldMergeWith() async throws {
        let hey = mockHey(status(409, #"{"errors":["Email address has already been taken"],"contact_id":80,"conflicting_contact_ids":[77,78]}"#))
        let error = try await XCTUnwrapAsync(
            await assertThrows(HeyError.codeConflict, try await hey.client().contacts.createContact(ContactParams(name: "Ann", emailAddress: "ann@example.com"))))
        XCTAssertEqual(error.message, "Email address has already been taken")
        let conflict = try XCTUnwrap(ContactConflict.fromError(error))
        XCTAssertEqual(conflict.contactId, 80, "a create that clashes still creates the contact")
        XCTAssertEqual(conflict.conflictingContactIds, [77, 78])
        XCTAssertEqual(conflict.description, "contact 80 conflicts with 77, 78")
        XCTAssertEqual(ContactConflict(contactId: 80).description, "contact 80 conflicts with one that already exists")
    }

    func testAConflictFromElsewhereCarriesNoContactConflict() async throws {
        let hey = mockHey(status(409, #"{"error":"A time track is already running"}"#), status(409))
        let client = try hey.client()
        let worded = try await XCTUnwrapAsync(await assertThrows(HeyError.codeConflict, try await client.contacts.setNote(contactId: 77, note: "note")))
        XCTAssertEqual(worded.message, "A time track is already running", "a single message is read too")
        XCTAssertNil(ContactConflict.fromError(worded))
        let bare = try await XCTUnwrapAsync(await assertThrows(HeyError.codeConflict, try await client.contacts.setNote(contactId: 77, note: "note")))
        XCTAssertEqual(bare.message, "the contact conflicts with one that already exists", "a 409 without a readable body still has to read as something")
        XCTAssertNil(ContactConflict.fromError(bare))
        XCTAssertNil(ContactConflict.fromError(.notFound(message: "resource not found", detail: ErrorDetail())))
    }

    func testARejectionIsAValidationInTheModelsWords() async throws {
        let hey = mockHey(status(422, #"{"errors":["Name can't be blank","Email address is invalid"]}"#))
        let error = await assertThrows(HeyError.codeValidation, try await hey.client().contacts.createContact(ContactParams()))
        XCTAssertEqual(error?.message, "Name can't be blank; Email address is invalid")
    }

    func testARewordedRefusalIsWhatTheHooksHearTheWriteEndWith() async throws {
        let hey = mockHey(status(409, #"{"errors":["Email address has already been taken"],"contact_id":80}"#))
        final class Ends: HeyHooks, @unchecked Sendable {
            let lock = NSLock()
            var errors: [String] = []
            func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
                lock.withLock { errors.append("\(info.operation):\((result.error as? HeyError)?.message ?? "nil")") }
            }
        }
        let ends = Ends()
        await assertThrows(
            HeyError.codeConflict, try await hey.client(hooks: ends).contacts.createContact(ContactParams(name: "Ann", emailAddress: "ann@example.com")))
        XCTAssertEqual(ends.lock.withLock { ends.errors }, ["CreateContact:Email address has already been taken"])
    }

    func testScreeningAndTheNoteGoToTheContactsOwnEndpoints() async throws {
        let hey = mockHey(ok(""), ok(#"{"contact_id":77,"note":"Met at the conference","note_html":"<div>Met at the conference</div>"}"#))
        let client = try hey.client()
        try await client.contacts.screen(contactId: 77, status: .denied)
        XCTAssertEqual(hey.requests[0].path, "/contacts/77/clearance.json")
        XCTAssertEqual(hey.requests[0].body, #"{"status":"denied"}"#)

        let note = try await client.contacts.setNote(contactId: 77, note: "Met at the conference")
        XCTAssertEqual(note.note, "Met at the conference")
        XCTAssertEqual(hey.requests[1].path, "/contacts/77/note.json")
        XCTAssertEqual(try contact(hey.requests[1])["note"] as? String, "Met at the conference")
    }
}

/// Unwraps what an async assertion handed back, failing the test when it handed back nothing.
private func XCTUnwrapAsync<T>(_ value: T?, file: StaticString = #filePath, line: UInt = #line) async throws -> T {
    try XCTUnwrap(value, file: file, line: line)
}
