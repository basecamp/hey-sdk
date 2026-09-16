import Foundation
import XCTest

@testable import Hey

final class DesignationsTests: XCTestCase {
    func testADesignationIsWrittenUnderTheBoxThatHoldsIt() async throws {
        let hey = mockHey(ok(""))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.designations.createBoxDesignation(boxId: 3, contactId: 77)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/boxes/3/designations.json")
        XCTAssertEqual(request.body, #"{"contact_id":77}"#)
        XCTAssertEqual(log.started, ["Designations.CreateBoxDesignation:designation:true:3"])
    }
}
