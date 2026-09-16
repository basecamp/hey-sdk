import Foundation
import XCTest

@testable import Hey

final class ErrorMappingTests: XCTestCase {
    private func map(_ status: Int, _ body: String = "", method: Method = .get, _ headers: [(String, String)] = []) -> HeyError {
        HeyError.fromResponse(status: status, method: method, headers: HTTPHeaders(headers), body: Data(body.utf8))
    }

    func testStatusesMapToTheSharedVocabulary() {
        let auth = map(401, #"{"error":"Unauthorized"}"#)
        guard case .auth = auth else { return XCTFail("\(auth)") }
        XCTAssertEqual(auth.code, HeyError.codeAuth)
        XCTAssertEqual(auth.httpStatus, 401)
        XCTAssertFalse(auth.isRetryable)
        XCTAssertEqual(auth.hint, "Unauthorized")
        XCTAssertEqual(auth.exitCode, 3)

        XCTAssertEqual(map(403).code, HeyError.codeForbidden)
        XCTAssertEqual(map(403, method: .post).message, "Access denied: insufficient scope")
        XCTAssertEqual(map(403, method: .post).hint, "Re-authenticate with full scope")
        XCTAssertEqual(map(404).code, HeyError.codeNotFound)
        XCTAssertEqual(map(409).code, HeyError.codeConflict)
        XCTAssertEqual(map(409).exitCode, 9)
        let validation = map(422, #"{"error":"Subject can't be blank"}"#)
        XCTAssertEqual(validation.code, HeyError.codeValidation)
        XCTAssertEqual(validation.hint, "Subject can't be blank")
        let limited = map(429, "", [("Retry-After", "5")])
        guard case let .rateLimit(_, retryAfter, _) = limited else { return XCTFail("\(limited)") }
        XCTAssertEqual(retryAfter, 5)
        XCTAssertEqual(limited.hint, "Retry after 5 seconds")
        XCTAssertTrue(limited.isRetryable)
        let server = map(500, #"{"error":"Internal server error"}"#)
        XCTAssertEqual(server.code, HeyError.codeAPI)
        XCTAssertTrue(server.isRetryable)
        XCTAssertEqual(server.message, "API error: 500")
        XCTAssertFalse(map(400).isRetryable)
        XCTAssertEqual(map(503).exitCode, 7)
        XCTAssertEqual(HeyError.usage(message: "x").exitCode, 1)
        XCTAssertEqual(HeyError.ambiguous(resource: "box", matches: [], hint: nil).exitCode, 8)
    }

    func testTheRequestIdAndTheBodyAreKept() {
        let error = map(404, #"{"error":"Not found"}"#, [("X-Request-Id", "req-123")])
        XCTAssertEqual(error.requestId, "req-123")
        XCTAssertEqual(error.bodyText, #"{"error":"Not found"}"#)
    }

    func testAnHTMLPageIsNeverEchoedAndAListOfErrorsIsJoined() throws {
        XCTAssertNil(map(500, "<html><body>Oops</body></html>").hint)
        XCTAssertEqual(map(422, #"{"errors":["a","b"]}"#).hint, "a; b")
        XCTAssertEqual(map(422, #"{"message":"fallback"}"#).hint, "fallback")
        XCTAssertNil(map(422, #"{"error":{"nested":true}}"#).hint)
        let long = try XCTUnwrap(map(422, #"{"error":"\#(String(repeating: "x", count: 600))"}"#).hint)
        XCTAssertEqual(long.count, 500)
        XCTAssertTrue(long.hasSuffix("..."))
    }

    func testTheBodyIsBounded() {
        let error = map(500, String(repeating: "x", count: HeyError.maxErrorBodyBytes + 10))
        XCTAssertEqual(error.body?.count, HeyError.maxErrorBodyBytes)
    }

    func testRetryAfterIsReadAsSecondsOrAsADate() {
        XCTAssertEqual(retryAfterSeconds("2"), 2)
        XCTAssertEqual(retryAfterSeconds("0"), 0)
        XCTAssertNil(retryAfterSeconds(nil))
        XCTAssertNil(retryAfterSeconds("soon"))
        XCTAssertEqual(retryAfterSeconds("Sun, 06 Nov 1994 08:49:37 GMT"), 0)
    }

    func testAFailedCallCarriesItAll() async throws {
        let hey = mockHey(status(422, #"{"error":"Subject can't be blank"}"#, [("X-Request-Id", "req-draft-422")]))
        let body = CreateMessageRequestContent(actingSenderId: 1, message: MessagePayload(subject: "", content: "Final body"))
        let error = await assertThrows(HeyError.codeValidation, try await hey.client().messages.create(body: body))
        XCTAssertEqual(error?.httpStatus, 422)
        XCTAssertEqual(error?.requestId, "req-draft-422")
        XCTAssertEqual(error?.isRetryable, false)
        XCTAssertEqual(error?.description, "validation error: Subject can't be blank")
        XCTAssertEqual(error?.localizedDescription, "validation error: Subject can't be blank")
    }

    func testABodyThatWillNotDecodeIsNeverQuotedBack() async throws {
        let hey = mockHey(
            ok(#"{"email_address":"distinctive@example.com","id":}"#),
            ok(#"{"email_address":"distinctive@example.com","id":"x"}"#),
            ok(#"{"id":1,"email_address":"distinctive@example.com","entries":{"distinctive@example.com":"x"}}"#))
        let client = try hey.client()
        for _ in 1...2 {
            let error = await assertThrows(HeyError.codeAPI, try await client.contacts.get(contactId: 1))
            let everything = "\(String(describing: error)) \(String(reflecting: error)) \(error?.hint ?? "")"
            XCTAssertFalse(everything.contains("distinctive"), "the address leaked: \(everything)")
            XCTAssertNil(error?.detail?.cause, "the decoder's own error quotes the body, so it is not kept")
        }
        struct Keyed: Decodable {
            let entries: [String: Int]
        }
        do {
            _ = try Response(
                status: 200, headers: HTTPHeaders(), body: Data(#"{"entries":{"distinctive@example.com":"x"}}"#.utf8),
                url: URL(string: "https://app.hey.com")!, fromCache: false, empty: false
            ).json(Keyed.self)
            XCTFail("expected a decode failure")
        } catch let error as HeyError {
            XCTAssertEqual(error.hint, "body does not decode: at path $.entries", "a map key in the path is the body's, so the path stops before it")
        }
    }

    func testTheDecodeHintNamesWhereItStopped() throws {
        struct Inner: Decodable { let id: Int }
        struct Outer: Decodable { let entries: [Inner] }
        func hint<T: Decodable>(_ type: T.Type, _ body: String) -> String? {
            let response = Response(
                status: 200, headers: HTTPHeaders(), body: Data(body.utf8), url: URL(string: "https://app.hey.com")!,
                fromCache: false, empty: false)
            switch Result(catching: { try response.json(type) }) {
            case .success: return nil
            case let .failure(error): return (error as? HeyError)?.hint ?? "\(error)"
            }
        }
        XCTAssertEqual(hint(Outer.self, #"{"entries":[{"id":"x"}]}"#), "body does not decode: at path $.entries[0].id")
        XCTAssertEqual(hint(Outer.self, #"{"entries":[{}]}"#), "body does not decode: missing required field 'id', at path $.entries[0]")
        XCTAssertEqual(hint(Outer.self, #"{"#), "body does not decode")
    }

    func testAFailureOnAHopNeverQuotesWhereTheHopWent() async throws {
        let secret = "sig=distinctive-secret"
        let hey = mockHey(
            status(302, nil, [("Location", "https://files.example.com/export.json?\(secret)")]),
            failure(TransportFailure(description: "connect to https://files.example.com/export.json?\(secret) failed")))
        let transcript = Transcript()
        final class Seen: HeyHooks, @unchecked Sendable {
            let lock = NSLock()
            var errors: [String] = []
            func onRequestEnd(_ info: RequestInfo, result: RequestResult) {
                if let error = result.error { lock.withLock { errors.append("\(error) \((error as? HeyError)?.hint ?? "")") } }
            }
            func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
                if let error = result.error { lock.withLock { errors.append("\(error) \((error as? HeyError)?.hint ?? "")") } }
            }
        }
        let seen = Seen()
        let client = try hey.client(hooks: ChainHooks(transcript, seen)) { $0.enableRetry = false }
        let error = await assertThrows(HeyError.codeNetwork, try await client.boxes.list())
        let everything = ([String(describing: error), error?.hint ?? ""] + seen.errors).joined(separator: " ")
        XCTAssertFalse(everything.contains("distinctive"), "the signed target leaked: \(everything)")
        XCTAssertEqual(error?.hint, "connect to https://files.example.com failed")

        let downgrade = mockHey(status(302, nil, [("Location", "http://app.hey.com/export.json?\(secret)")]))
        let refused = await assertThrows(HeyError.codeUsage, try await downgrade.client().boxes.list())
        XCTAssertEqual(refused?.message, "http://app.hey.com must use HTTPS")
    }

    func testAURLInATransportMessageIsCutBackToItsOrigin() {
        XCTAssertEqual(redactURLs("connect to https://host:8443/path?sig=1 failed"), "connect to https://host:8443 failed")
        XCTAssertEqual(redactURLs("https://user:pw@files.example.com/x?sig=1 timed out"), "https://files.example.com timed out")
        XCTAssertEqual(redactURLs("https://[::1]:8443/x#frag"), "https://[::1]:8443")
        XCTAssertEqual(redactURLs("[url=https://a/b?c, request_timeout=30000 ms]"), "[url=https://a request_timeout=30000 ms]")
        XCTAssertEqual(redactURLs("no url here"), "no url here")
        for text in [
            "connect to https://host/path?safe=x,token=distinctive failed", "https://host/(a)b=distinctive failed",
            "see https://host/x?a=1]&t=distinctive.",
        ] {
            let redacted = redactURLs(text)
            XCTAssertFalse(redacted.contains("distinctive"), "\(text) -> \(redacted)")
        }
    }

    func testAnOversizedErrorBodyKeepsTheErrorItsStatusMeans() async throws {
        let big = #"{"errors":["\#(String(repeating: "x", count: 2000))"]}"#
        let hey = mockHey(
            status(422, big, [("X-Request-Id", "req-422")]),
            status(429, big, [("Retry-After", "3")]),
            status(404, big),
            status(500, big))
        let client = try hey.client {
            $0.maxResponseBodyBytes = 100
            $0.enableRetry = false
        }
        let validation = await assertThrows(HeyError.codeValidation, try await client.contacts.get(contactId: 1))
        XCTAssertEqual(validation?.httpStatus, 422)
        XCTAssertEqual(validation?.requestId, "req-422")
        XCTAssertNil(validation?.body, "the body the client refused is not kept")
        XCTAssertEqual(validation?.responseTooLarge, true)
        XCTAssertEqual(validation?.hint, "response body exceeds 100 bytes")
        let limited = await assertThrows(HeyError.codeRateLimit, try await client.contacts.get(contactId: 1))
        guard case let .rateLimit(_, retryAfter, _) = limited else { return XCTFail() }
        XCTAssertEqual(retryAfter, 3)
        await assertThrows(HeyError.codeNotFound, try await client.contacts.get(contactId: 1))
        let api = await assertThrows(HeyError.codeAPI, try await client.contacts.get(contactId: 1))
        XCTAssertEqual(api?.httpStatus, 500)
        XCTAssertEqual(api?.responseTooLarge, true)
    }

    func testA400IsAnAPIErrorAsItIsInTheOtherSDKs() async throws {
        let hey = mockHey(status(400, #"{"error":"unparsable timestamp"}"#))
        let error = await assertThrows(HeyError.codeAPI, try await hey.client().boxes.list())
        XCTAssertEqual(error?.httpStatus, 400)
        XCTAssertEqual(error?.isRetryable, false)
        XCTAssertEqual(error?.hint, "unparsable timestamp")
    }
}
