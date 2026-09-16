import Foundation
import XCTest

@testable import Hey

final class ClientTests: XCTestCase {
    private func client(_ configure: (inout HeyConfig) -> Void) throws -> HeyClient {
        var config = HeyConfig()
        configure(&config)
        return try HeyClient(accessToken: "token", config: config, transport: MockHey([]))
    }

    func testPlainHTTPIsRefusedOffThisMachine() {
        let error = assertThrowsSync(HeyError.codeUsage) { try client { $0.baseURL = "http://evil.example.com" } }
        XCTAssertTrue(error?.message.contains("must use HTTPS") == true)
    }

    func testPlainHTTPIsAllowedOnLocalhost() throws {
        for base in ["http://localhost:3000", "http://127.0.0.1:8080", "http://app.localhost", "https://app.hey.com"] {
            let made = try client { $0.baseURL = base }
            XCTAssertEqual(made.baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")), base)
        }
    }

    func testABlankTokenIsAUsageError() {
        // An unset environment variable hands over "", and the failure is the one every other
        // mistake in the setup is.
        for blank in ["", "   ", "\t\n"] {
            let error = assertThrowsSync(HeyError.codeUsage) { try HeyClient(accessToken: blank, transport: MockHey([])) }
            XCTAssertEqual(error?.message, "Access token must not be blank")
            let direct = assertThrowsSync(HeyError.codeUsage) { try StaticTokenProvider(blank) }
            XCTAssertEqual(direct?.message, "Access token must not be blank")
        }
    }

    func testTheSettingsAreChecked() {
        assertThrowsSync(HeyError.codeUsage) { try client { $0.maxPages = 0 } }
        assertThrowsSync(HeyError.codeUsage) { try client { $0.maxRetries = -1 } }
        assertThrowsSync(HeyError.codeUsage) { try client { $0.timeout = .zero } }
        assertThrowsSync(HeyError.codeUsage) { try client { $0.baseURL = "not a url" } }
        assertThrowsSync(HeyError.codeUsage) { try client { $0.maxRetryDelay = .seconds(-1) } }
        assertThrowsSync(HeyError.codeUsage) { try client { $0.maxRetryJitter = .seconds(-1) } }
        assertThrowsSync(HeyError.codeUsage) { try client { $0.baseRetryDelay = .seconds(-1) } }
        XCTAssertNoThrow(try client { $0.maxRetryJitter = .zero })
        XCTAssertNoThrow(try client { $0.timeout = nil })
    }

    func testATimeoutBelowAMillisecondIsRefusedAsAUsageError() {
        let error = assertThrowsSync(HeyError.codeUsage) { try client { $0.timeout = .microseconds(500) } }
        XCTAssertTrue(error?.message.contains("millisecond") == true)
        XCTAssertNoThrow(try client { $0.timeout = .milliseconds(1) })
    }

    func testTheUserAgentNamesTheSDKAndTheAPIContract() {
        XCTAssertEqual(HeyConfig.defaultUserAgent, "hey-sdk-swift/\(HeyConfig.version) (api:\(HeyConfig.apiVersion))")
        XCTAssertNotNil(HeyConfig.apiVersion.range(of: #"^\d{4}-\d{2}-\d{2}$"#, options: .regularExpression))
    }

    func testABaseURLIsAnOriginAndAPathPrefixAndNothingMore() {
        for base in ["https://app.hey.com?filtered_account_id=42", "https://user:pass@app.hey.com", "https://app.hey.com/#fragment"] {
            let error = assertThrowsSync(HeyError.codeUsage) { try client { $0.baseURL = base } }
            let message = error?.message ?? ""
            XCTAssertFalse(message.contains("pass") || message.contains("42") || message.contains("#fragment"), message)
        }
        XCTAssertNoThrow(try client { $0.baseURL = "https://app.hey.com/prefix" })
    }

    func testClosingADerivedClientLeavesTheTransportToTheRoot() async throws {
        let hey = mockHey(ok(identityJSON), ok(identityJSON), ok("[]"), ok("[]"))
        let root = try hey.client()
        let work = try await root.forAccount(42)
        let other = try await root.forAccount(42)
        work.close()
        _ = try await other.boxes.list()
        _ = try await root.boxes.list()
        XCTAssertEqual(hey.requests.count, 4, "the root and a sibling still send after a derived client is closed")
        root.close()
        let error = await assertThrows(HeyError.codeUsage, try await root.boxes.list())
        XCTAssertEqual(error?.message, "client is closed")
        await assertThrows(HeyError.codeUsage, try await other.boxes.list())
    }

    func testAnAccountTheIdentityCannotReachIsRefused() async throws {
        let hey = mockHey(ok(identityJSON), ok(identityJSON))
        let client = try hey.client()
        await assertThrows(HeyError.codeNotFound, try await client.forAccount(43))
        await assertThrows(HeyError.codeNotFound, try await client.forAccount(99))
        await assertThrows(HeyError.codeUsage, try await client.forAccount(0))
    }

    func testAScopedClientChecksTheAccountAndFiltersEveryRequest() async throws {
        let hey = mockHey(ok(identityJSON), ok("[]", [("Link", #"</boxes.json?page=2>; rel="next""#)]), ok("[]"))
        let work = try await hey.client().forAccount(42)
        let page = try await work.boxes.list()
        _ = try await work.nextPage(page)

        XCTAssertEqual(hey.requests[0].path, "/identity.json")
        XCTAssertNil(hey.requests[0].query("filtered_account_id"))
        XCTAssertEqual(hey.requests[1].query("filtered_account_id"), "42")
        XCTAssertEqual(hey.requests[2].query("filtered_account_id"), "42", "the next page is filtered too")
        XCTAssertEqual(hey.requests[2].query("page"), "2")
        XCTAssertEqual(work.accountId, 42)
    }

    func testTheDefaultSenderFollowsTheScope() async throws {
        let hey = mockHey(ok(identityJSON), ok(identityJSON), ok(identityJSON))
        let client = try hey.client()
        let first = try await client.defaultSenderId()
        let again = try await client.defaultSenderId()
        XCTAssertEqual(first, 100, "the identity's default sender")
        XCTAssertEqual(again, 100, "read once")
        let work = try await client.forAccount(42)
        let workSender = try await work.defaultSenderId()
        let workUser = try await work.accountUserId()
        XCTAssertEqual(workSender, 100)
        XCTAssertEqual(workUser, 1000)
        XCTAssertEqual(hey.requests.count, 2)
        await assertThrows(HeyError.codeUsage, try await client.accountUserId())
    }

    func testWithoutADefaultSenderTheFirstOneOrThePrimaryContactStandsIn() async throws {
        let first = try await mockHey(ok(#"{"id":1,"primary_contact":{"id":9},"senders":[{"id":5},{"id":6}]}"#)).client().defaultSenderId()
        let primary = try await mockHey(ok(#"{"id":1,"primary_contact":{"id":9}}"#)).client().defaultSenderId()
        XCTAssertEqual(first, 5)
        XCTAssertEqual(primary, 9)
    }

    func testASensitiveStringPrintsRedactedAndTravelsAsAString() throws {
        let sender = try JSONDecoder().decode(Sender.self, from: Data(#"{"id":1,"email_address":"jane@example.com"}"#.utf8))
        XCTAssertEqual(sender.emailAddress?.expose(), "jane@example.com")
        XCTAssertEqual(sender.emailAddress?.description, "[REDACTED]")
        XCTAssertFalse(String(describing: sender).contains("jane@example.com"))
        XCTAssertFalse(String(reflecting: sender).contains("jane@example.com"))
        let encoded = String(decoding: try heyJSONEncoder().encode(sender), as: UTF8.self)
        XCTAssertEqual(encoded, #"{"email_address":"jane@example.com","id":1}"#)
        XCTAssertEqual(String(describing: try StaticTokenProvider("secret")), "StaticTokenProvider([REDACTED])")
    }
}
