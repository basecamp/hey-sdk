import Foundation
import XCTest

@testable import Hey

/// A 422 keeps the reasons HEY gave: ``HeyError/fieldErrors``, and the hint that joins them.
final class ValidationErrorTests: XCTestCase {
    /// The body HEY answers a refused habit with: the model's full messages (`habit.errors.full_messages`).
    private let habitRefusal = #"{"errors":["Title can't be blank","Days must include at least one day"]}"#

    private let habitReasons = [
        FieldError(field: nil, message: "Title can't be blank"),
        FieldError(field: nil, message: "Days must include at least one day"),
    ]

    private func refused(_ body: String) -> HeyError {
        HeyError.fromResponse(status: 422, method: .post, headers: HTTPHeaders([]), body: Data(body.utf8))
    }

    func testARefusedHabitKeepsEachReasonHeyGave() async throws {
        let hey = mockHey(status(422, habitRefusal, [("X-Request-Id", "req-422")]))
        let client = try hey.client()

        do {
            _ = try await client.habits.createHabit(params: HabitParams())
            XCTFail("expected a validation error")
        } catch let error as HeyError {
            XCTAssertEqual(error.code, HeyError.codeValidation)
            XCTAssertEqual(error.httpStatus, 422)
            XCTAssertFalse(error.isRetryable)
            XCTAssertEqual(error.requestId, "req-422")
            XCTAssertEqual(error.fieldErrors, habitReasons)
            XCTAssertEqual(error.hint, "Title can't be blank; Days must include at least one day")
        }
        XCTAssertEqual(hey.requests.count, 1)
    }

    func testARawWriteRefusedKeepsEachReasonHeyGave() async throws {
        let hey = mockHey(status(422, habitRefusal))
        let client = try hey.client()

        do {
            _ = try await client.execute(client.request(.post, "/calendar/habits.json"))
            XCTFail("expected a validation error")
        } catch let error as HeyError {
            XCTAssertEqual(error.code, HeyError.codeValidation)
            XCTAssertEqual(error.fieldErrors, habitReasons)
        }
    }

    func testEveryShapeOfReasonReads() {
        let cases: [(String, [FieldError])] = [
            (
                #"{"errors":[{"field":"subject","message":"is too long"},{"attribute":"content","message":"can't be blank"}]}"#,
                [FieldError(field: "subject", message: "is too long"), FieldError(field: "content", message: "can't be blank")]
            ),
            (#"{"errors":["Title can't be blank", 7, {"field":"days"}, "", null]}"#, [FieldError(field: nil, message: "Title can't be blank")]),
            (#"{"error":"Subject can't be blank"}"#, [FieldError(field: nil, message: "Subject can't be blank")]),
            (#"{"errors":{"subject":["is too long"]},"message":"Invalid message"}"#, [FieldError(field: nil, message: "Invalid message")]),
            ("", []),
            ("<html>Unprocessable</html>", []),
            (#"{"errors":[]}"#, []),
        ]
        for (body, expected) in cases {
            XCTAssertEqual(refused(body).fieldErrors, expected, body)
        }
    }

    func testAReasonNamingItsFieldReadsWithIt() {
        let error = refused(#"{"errors":[{"field":"subject","message":"is too long"}]}"#)

        XCTAssertEqual(error.hint, "subject: is too long")
        XCTAssertEqual(error.fieldErrors.first?.description, "subject: is too long")
    }

    func testOnlyA422HasFieldErrors() {
        let conflict = HeyError.fromResponse(
            status: 409, method: .post, headers: HTTPHeaders([]), body: Data(#"{"errors":["Email address is taken"]}"#.utf8))

        XCTAssertTrue(conflict.fieldErrors.isEmpty)
    }
}
