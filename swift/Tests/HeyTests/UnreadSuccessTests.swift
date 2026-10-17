import Foundation
import XCTest

@testable import Hey

/// A write to a path the caller wrote has taken effect once HEY answers a success, so a body lost
/// after the status, or one past the size limit, must not fail it: a caller told a send failed
/// would send it again.
final class UnreadSuccessTests: XCTestCase {
    private let lost = URLError(.networkConnectionLost)

    func testARawWriteWhoseSuccessIsCutShortIsTheSuccessItWas() async throws {
        for method in [HTTPMethod.post, .put, .patch, .delete] {
            for code in [200, 201] {
                let hey = mockHey(interrupted(code, #"{"id":2201,"topic_id":"#, lost), ok())
                let client = try hey.client { $0.maxRetries = 2 }
                var operation = client.request(method, "/messages")
                if method != .delete { try operation.json(["subject": "Lunch on Friday"]) }

                let response = try await client.execute(operation)
                XCTAssertEqual(response.status, code, "\(method)")
                XCTAssertTrue(response.body.isEmpty, "\(method)")
                let bodyError = try XCTUnwrap(response.bodyError, "\(method) \(code)")
                XCTAssertEqual(bodyError.code, HeyError.codeNetwork)
                XCTAssertEqual(hey.requests.count, 1, "\(method): a write that took effect is never resent")
            }
        }
    }

    func testSendVoidSucceedsAndSendThrowsTheBodyError() async throws {
        let hey = mockHey(interrupted(200, #"{"id":"#, lost), interrupted(200, #"{"id":"#, lost))
        let client = try hey.client()
        try await client.sendVoid(client.request(.put, "/messages/1"))

        struct Sent: Decodable, Sendable { let id: Int }
        await assertThrows(HeyError.codeNetwork, try await client.send(client.request(.post, "/messages"), as: Sent.self))
        XCTAssertEqual(hey.requests.count, 2)
    }

    func testJSONThrowsTheBodyErrorRatherThanADecodeFailure() async throws {
        let hey = mockHey(interrupted(200, #"{"id":"#, lost))
        let client = try hey.client()
        let response = try await client.execute(client.request(.post, "/messages"))
        struct Sent: Decodable { let id: Int }
        assertThrowsSync(HeyError.codeNetwork) { try response.json(Sent.self) }
    }

    func testARawWritePastTheBodyLimitIsTheSuccessItWas() async throws {
        let hey = mockHey(ok(String(repeating: " ", count: 4096)))
        let client = try hey.client { $0.maxResponseBodyBytes = 1024 }
        let response = try await client.execute(client.request(.post, "/messages"))
        XCTAssertEqual(response.status, 200)
        XCTAssertTrue(response.body.isEmpty)
        XCTAssertEqual(response.bodyError?.responseTooLarge, true)
    }

    /// A GET changes nothing, so a GET cut short is the failure it was and is resent as before.
    func testARawReadCutShortIsStillAFailure() async throws {
        let hey = mockHey(interrupted(200, #"{"id":"#, lost))
        let client = try hey.client { $0.maxRetries = 0 }
        await assertThrows(HeyError.codeNetwork, try await client.execute(client.request(.get, "/topics/1")))
    }

    /// The caller's own cancellation is never absorbed: a cancelled send is not reported delivered.
    func testACancelledReadOfARawWriteIsStillTheCancellation() async throws {
        let hey = mockHey(interrupted(200, #"{"id":"#, URLError(.cancelled)))
        let client = try hey.client()
        do {
            try await client.execute(client.request(.post, "/messages"))
            XCTFail("a cancelled read must not be reported as a success")
        } catch {
            XCTAssertTrue(isCancellation(error), "\(error)")
        }
    }
}
