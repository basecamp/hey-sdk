import Foundation
import XCTest

@testable import Hey

final class ExtenzionsServiceTests: XCTestCase {
    private let sales = ExtenzionContact(id: 10, name: "sales", appUrl: "https://app.hey.com/contacts/10")
    private let navigation = #"{"items":[{"title":"Boxes","menu_items":[{"title":"Imbox","app_url":"https://app.hey.com/imbox"}]},"#
        + #"{"title":"Extensions","menu_items":[{"title":"All Extensions","app_url":"https://app.hey.com/accounts/1/domains/extenzions"},"#
        + #"{"title":"sales","app_url":"https://app.hey.com/contacts/10"},{"title":"support","app_url":"https://app.hey.com/contacts/11"}]}]}"#

    func testListingReadsTheExtensionsGroupOutOfNavigationAsItsOwnOperation() async throws {
        let hey = mockHey(ok(navigation))
        let log = OperationLog()
        let listed = try await hey.client(hooks: log).extenzions.list()
        XCTAssertEqual(
            listed, [sales, ExtenzionContact(id: 11, name: "support", appUrl: "https://app.hey.com/contacts/11")],
            "the group's own link names no contact and falls out")
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].path, "/my/navigation.json")
        XCTAssertEqual(log.started, ["Extenzions.ListExtenzions:extenzion:false:nil"])
    }

    func testCreatingAnswersTheExtenzionUnderItsContactId() async throws {
        let hey = mockHey(Answer(status: 201, body: #"{"id":55,"name":"sales","app_url":"https://app.hey.com/contacts/10"}"#))
        let log = OperationLog()
        let created = try await hey.client(hooks: log).extenzions.create(
            accountId: 1, params: CreateExtenzionParams(name: "sales", members: ["jane.dawson@example.com"]))
        XCTAssertEqual(created, sales, "the id is the contact's, not the record's")
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/accounts/1/domains/extenzions.json")
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(fields(request.body), ["extenzion[name]=sales", "extenzion[members][]=jane.dawson@example.com"])
        XCTAssertEqual(log.started, ["Extenzions.CreateExtenzion:extenzion:true:nil"])
    }

    func testAWriteThatOnlyRedirectsHandsNothingBack() async throws {
        let redirect = status(302, nil, [("Location", "/accounts/1/domains/extenzions")])
        let hey = mockHey(redirect, redirect)
        let client = try hey.client()
        let created = try await client.extenzions.create(accountId: 1, params: CreateExtenzionParams(name: "sales"))
        XCTAssertNil(created)
        let updated = try await client.extenzions.update(accountId: 1, extenzionId: 10, params: UpdateExtenzionParams(name: "support"))
        XCTAssertNil(updated)
    }

    func testARevisionNamesOnlyWhatItWasAskedToChange() async throws {
        let answer = #"{"id":55,"name":"support","app_url":"https://app.hey.com/contacts/10"}"#
        let hey = mockHey(ok(answer), ok(answer))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        let updated = try await client.extenzions.update(accountId: 1, extenzionId: 10, params: UpdateExtenzionParams(name: "support"))
        XCTAssertEqual(updated?.name, "support")
        XCTAssertEqual(hey.requests[0].method, "PATCH")
        XCTAssertEqual(hey.requests[0].path, "/accounts/1/domains/extenzions/10.json")
        XCTAssertEqual(fields(hey.requests[0].body), ["extenzion[name]=support"])
        _ = try await client.extenzions.update(
            accountId: 1, extenzionId: 10, params: UpdateExtenzionParams(name: "", members: ["a@example.com", "b@example.com"]))
        XCTAssertEqual(
            fields(hey.requests[1].body), ["extenzion[members][]=a@example.com", "extenzion[members][]=b@example.com"],
            "a membership replaces the whole of what the extenzion had, and an empty name is no name")
        XCTAssertEqual(log.started[0], "Extenzions.UpdateExtenzion:extenzion:true:10")
    }

    func testAContactUrlThatNamesNoReadableIdIsRefusedRatherThanSkipped() async throws {
        XCTAssertNil(try contactIdFromUrl("https://app.hey.com/accounts/1/domains/extenzions"))
        XCTAssertNil(try contactIdFromUrl("/contacts/"))
        XCTAssertEqual(try contactIdFromUrl("https://app.hey.com/contacts/4821/edit"), 4821)
        let refused = assertThrowsSync(HeyError.codeAPI) { try contactIdFromUrl("/contacts/99999999999999999999") }
        XCTAssertTrue(refused?.message.contains("is not a number") == true, refused?.message ?? "")

        let hey = mockHey(Answer(status: 201, body: #"{"id":55,"name":"sales","app_url":"https://app.hey.com/accounts/1"}"#))
        let unreadable = await assertThrows(
            HeyError.codeAPI, try await hey.client().extenzions.create(accountId: 1, params: CreateExtenzionParams(name: "sales")))
        XCTAssertTrue(unreadable?.message.hasPrefix("no contact id in") == true, unreadable?.message ?? "")
    }

    func testAnEmptyMembershipClearsAndANilOneIsLeftAlone() async throws {
        let redirect = status(302, nil, [("Location", "/accounts/1/domains/extenzions/7")])
        let hey = mockHey(redirect, redirect)
        let client = try hey.client()
        _ = try await client.extenzions.update(accountId: 1, extenzionId: 7, params: UpdateExtenzionParams(members: []))
        _ = try await client.extenzions.update(accountId: 1, extenzionId: 7, params: UpdateExtenzionParams(members: nil))
        XCTAssertEqual(
            formValues(hey.requests[0].body, "extenzion[members][]"), [""],
            "an empty list goes out as one blank value, which HEY reads as a membership of none")
        XCTAssertEqual(
            formValues(hey.requests[1].body, "extenzion[members][]"), [],
            "nil sends nothing, so the membership stays as it was")
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
