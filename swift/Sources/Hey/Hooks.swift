import Foundation

/// Describes a semantic SDK operation: the service and operation as the model names them, the
/// kind of record they touch, and whether they change anything. A hand-written convenience is
/// free to report itself as something other than the route it sends.
public struct OperationInfo: Sendable, Equatable {
    /// Logical service: `Boxes`, `TimeTracks`.
    public var service: String
    /// The operation as the model names it: `ListBoxes`.
    public var operation: String
    /// HEY resource type in `snake_case`: `box`, `time_track`.
    public var resourceType: String
    /// Whether the operation changes anything.
    public var isMutation: Bool
    /// The record the operation acts on, when it names one.
    public var resourceId: Int?

    public init(service: String, operation: String, resourceType: String, isMutation: Bool, resourceId: Int? = nil) {
        self.service = service
        self.operation = operation
        self.resourceType = resourceType
        self.isMutation = isMutation
        self.resourceId = resourceId
    }
}

/// How an operation ended.
public struct OperationResult: Sendable {
    /// How long the operation took, on a monotonic clock.
    public var duration: Duration
    /// The error it failed with, or nil when it succeeded.
    public var error: (any Error)?

    public init(duration: Duration, error: (any Error)? = nil) {
        self.duration = duration
        self.error = error
    }
}

/// One HTTP request, as the hooks hear of it.
public struct RequestInfo: Sendable, Equatable {
    /// The HTTP method.
    public var method: String
    /// The URL. One that authenticates itself — a storage upload — is cut back to its origin.
    public var url: String
    /// The attempt, from 1, counted across the whole operation.
    public var attempt: Int

    public init(method: String, url: String, attempt: Int) {
        self.method = method
        self.url = url
        self.attempt = attempt
    }
}

/// How one HTTP request ended.
public struct RequestResult: Sendable {
    /// The status HEY answered with, or 0 when there was no answer.
    public var statusCode: Int
    /// How long the request took, on a monotonic clock.
    public var duration: Duration
    /// Whether the answer came out of the response cache.
    public var fromCache: Bool
    /// The error the request failed with, whether or not the client went on to resend it.
    public var error: (any Error)?

    public init(statusCode: Int, duration: Duration, fromCache: Bool = false, error: (any Error)? = nil) {
        self.statusCode = statusCode
        self.duration = duration
        self.fromCache = fromCache
        self.error = error
    }
}

/// Observability hooks, at two levels: an operation (`Boxes.ListBoxes`) and each HTTP request
/// it sends, retries included. Every method has a default that does nothing, so implement only
/// what you need.
///
/// ```swift
/// struct Logging: HeyHooks {
///     func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
///         print("\(info.service).\(info.operation) took \(result.duration)")
///     }
/// }
/// ```
public protocol HeyHooks: Sendable {
    /// An operation starts.
    func onOperationStart(_ info: OperationInfo)
    /// An operation ends, however it ends. Always called for one that started.
    func onOperationEnd(_ info: OperationInfo, result: OperationResult)
    /// A request goes out, including every resend.
    func onRequestStart(_ info: RequestInfo)
    /// A request is answered or fails, including every resend.
    func onRequestEnd(_ info: RequestInfo, result: RequestResult)
    /// A request is about to be sent again: the attempt about to be made, and the wait before it.
    func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration)
}

extension HeyHooks {
    public func onOperationStart(_ info: OperationInfo) {}
    public func onOperationEnd(_ info: OperationInfo, result: OperationResult) {}
    public func onRequestStart(_ info: RequestInfo) {}
    public func onRequestEnd(_ info: RequestInfo, result: RequestResult) {}
    public func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration) {}
}

/// Hooks that do nothing.
public struct NoopHooks: HeyHooks {
    public init() {}
}

/// Several hooks as one. Start events reach them in order and end events in reverse, so spans
/// and traces nest.
public struct ChainHooks: HeyHooks {
    private let hooks: [any HeyHooks]

    public init(_ hooks: any HeyHooks...) {
        self.hooks = hooks
    }

    public init(_ hooks: [any HeyHooks]) {
        self.hooks = hooks
    }

    public func onOperationStart(_ info: OperationInfo) {
        for hook in hooks { hook.onOperationStart(info) }
    }

    public func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
        for hook in hooks.reversed() { hook.onOperationEnd(info, result: result) }
    }

    public func onRequestStart(_ info: RequestInfo) {
        for hook in hooks { hook.onRequestStart(info) }
    }

    public func onRequestEnd(_ info: RequestInfo, result: RequestResult) {
        for hook in hooks.reversed() { hook.onRequestEnd(info, result: result) }
    }

    public func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration) {
        for hook in hooks { hook.onRetry(info, attempt: attempt, error: error, delay: delay) }
    }
}

/// Hooks that print operations, and optionally requests and retries, for debugging.
public struct ConsoleHooks: HeyHooks {
    public var logOperations: Bool
    public var logRequests: Bool
    public var logRetries: Bool

    public init(logOperations: Bool = true, logRequests: Bool = false, logRetries: Bool = true) {
        self.logOperations = logOperations
        self.logRequests = logRequests
        self.logRetries = logRetries
    }

    public func onOperationStart(_ info: OperationInfo) {
        guard logOperations else { return }
        let mutation = info.isMutation ? " [mutation]" : ""
        let resource = info.resourceId.map { " #\($0)" } ?? ""
        print("[HEY] \(info.service).\(info.operation)\(resource)\(mutation)")
    }

    public func onOperationEnd(_ info: OperationInfo, result: OperationResult) {
        guard logOperations else { return }
        if let error = result.error {
            print("[HEY] \(info.service).\(info.operation) failed (\(result.duration)): \(error)")
        } else {
            print("[HEY] \(info.service).\(info.operation) completed (\(result.duration))")
        }
    }

    public func onRequestStart(_ info: RequestInfo) {
        guard logRequests else { return }
        let retry = info.attempt > 1 ? " (attempt \(info.attempt))" : ""
        print("[HEY] -> \(info.method) \(info.url)\(retry)")
    }

    public func onRequestEnd(_ info: RequestInfo, result: RequestResult) {
        guard logRequests else { return }
        let cache = result.fromCache ? " (cached)" : ""
        print("[HEY] <- \(info.method) \(info.url) \(result.statusCode) (\(result.duration))\(cache)")
    }

    public func onRetry(_ info: RequestInfo, attempt: Int, error: any Error, delay: Duration) {
        guard logRetries else { return }
        print("[HEY] Retrying \(info.method) \(info.url) (attempt \(attempt), waiting \(delay)): \(error)")
    }
}
