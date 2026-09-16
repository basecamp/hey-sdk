import Foundation
import XCTest

@testable import Hey

final class CollectionsServiceTests: XCTestCase {
    func testACollectionIsMadeWithWhatItWasGiven() async throws {
        let redirect = status(302, nil, [("Location", "/collections")])
        let hey = mockHey(redirect, redirect)
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.collections.create(params: CreateCollectionParams(name: "Reading", summary: "Long reads for the weekend", accountId: 77))
        try await client.collections.create(params: CreateCollectionParams(name: "Reading", summary: ""))
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/collections")
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)
        XCTAssertEqual(fields(request.body), ["collection[name]=Reading", "collection[summary]=Long reads for the weekend", "account_id=77"])
        XCTAssertEqual(fields(hey.requests[1].body), ["collection[name]=Reading"], "an empty summary and no account are left off the wire")
        XCTAssertEqual(log.started[0], "Collections.CreateCollection:collection:true:nil")
    }

    func testARevisionSendsOnlyWhatItNamesAsTheGeneratedUpdateDoes() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().collections.updateCollection(collectionId: 5, params: UpdateCollectionParams(name: "Renamed", summary: ""))
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.path, "/collections/5.json")
        let collection = try XCTUnwrap(try jsonObject(request.body)["collection"] as? [String: Any])
        XCTAssertEqual(collection["name"] as? String, "Renamed")
        XCTAssertTrue(collection["summary"] == nil || collection["summary"] is NSNull, "an empty summary is no summary, and is left as it was")
    }

    func testATopicIsFiledIntoAndTakenOutOfACollectionByTheQuery() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/topics/4471829")]), status(302, nil, [("Location", "/topics/4471829")]))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.collections.addTopic(topicId: 4471829, collectionId: 5)
        try await client.collections.removeTopic(topicId: 4471829, collectionId: 5)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/topics/4471829/collecting")
        XCTAssertEqual(hey.requests[0].query("collection_id"), "5")
        XCTAssertEqual(hey.requests[0].header("Content-Type"), "application/x-www-form-urlencoded", "an empty form is still a form")
        XCTAssertEqual(hey.requests[0].body, "")
        XCTAssertEqual(hey.requests[1].method, "DELETE")
        XCTAssertEqual(hey.requests[1].path, "/topics/4471829/collecting")
        XCTAssertEqual(hey.requests[1].query("collection_id"), "5")
        XCTAssertEqual(log.started, [
            "Collections.CreateTopicCollecting:collecting:true:4471829",
            "Collections.DeleteTopicCollecting:collecting:true:4471829",
        ])
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
