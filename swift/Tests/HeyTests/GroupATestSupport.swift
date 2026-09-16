import Foundation
import XCTest

@testable import Hey

/// The operations the hooks heard start, whole, and every request as its method and URL, for a
/// test of what a convenience announces itself as and where its requests went.
final class GroupAHooksLog: HeyHooks, @unchecked Sendable {
    private let lock = NSLock()
    private var operationLog: [OperationInfo] = []
    private var requestLog: [String] = []

    var operations: [OperationInfo] { lock.withLock { operationLog } }
    var requests: [String] { lock.withLock { requestLog } }

    func onOperationStart(_ info: OperationInfo) {
        lock.withLock { operationLog.append(info) }
    }

    func onRequestStart(_ info: RequestInfo) {
        lock.withLock { requestLog.append("\(info.method) \(info.url)") }
    }
}

/// A JSON body's member as an object, for asserting on a wrapped payload.
func groupAMember(_ body: String, _ name: String) throws -> [String: Any] {
    try XCTUnwrap(try jsonObject(body)[name] as? [String: Any])
}
