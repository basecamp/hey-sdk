import Foundation

/// A lock a task can hold across `await`, taken in the order it was asked for. Swift's actors
/// are re-entrant, so they cannot keep one signing from interleaving with a refresh; this can.
/// A task cancelled while it waits stops waiting and is told so, rather than holding its place
/// in the queue until the lock comes round to it.
final class AsyncMutex: @unchecked Sendable {
    private let state = NSLock()
    private var held = false
    private var waiters: [(id: UInt64, continuation: CheckedContinuation<Void, any Error>)] = []
    private var nextId: UInt64 = 0

    /// Waits for the lock and takes it.
    ///
    /// - Throws: `CancellationError` when the task is cancelled before the lock is its.
    func acquire() async throws {
        let id = reserveId()
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, any Error>) in
                switch enqueue(id, continuation) {
                case .acquired: continuation.resume()
                case .cancelled: continuation.resume(throwing: CancellationError())
                case .waiting: break
                }
            }
        } onCancel: {
            cancel(id)
        }
    }

    /// Gives the lock up, handing it straight to the longest waiter when there is one.
    func release() {
        state.lock()
        let next: CheckedContinuation<Void, any Error>?
        if waiters.isEmpty {
            held = false
            next = nil
        } else {
            next = waiters.removeFirst().continuation
        }
        state.unlock()
        next?.resume()
    }

    /// Runs `body` holding the lock, and gives it up however `body` ends.
    func withLock<T>(_ body: () async throws -> T) async throws -> T {
        try await acquire()
        do {
            let value = try await body()
            release()
            return value
        } catch {
            release()
            throw error
        }
    }

    private enum Enqueued { case acquired, waiting, cancelled }

    /// Waits that have been given an id but not yet joined the queue, and those of them cancelled
    /// before they could.
    private var pendingIds: Set<UInt64> = []
    private var cancelledIds: Set<UInt64> = []

    private func reserveId() -> UInt64 {
        state.lock()
        defer { state.unlock() }
        nextId += 1
        pendingIds.insert(nextId)
        return nextId
    }

    private func enqueue(_ id: UInt64, _ continuation: CheckedContinuation<Void, any Error>) -> Enqueued {
        state.lock()
        defer { state.unlock() }
        pendingIds.remove(id)
        if cancelledIds.remove(id) != nil { return .cancelled }
        if !held {
            held = true
            return .acquired
        }
        waiters.append((id, continuation))
        return .waiting
    }

    private func cancel(_ id: UInt64) {
        state.lock()
        if let index = waiters.firstIndex(where: { $0.id == id }) {
            let waiter = waiters.remove(at: index)
            state.unlock()
            waiter.continuation.resume(throwing: CancellationError())
            return
        }
        // Not queued yet — the cancellation came before the wait began — so the wait is refused
        // when it arrives; or already handed the lock, in which case the holder releases it as usual.
        if pendingIds.contains(id) { cancelledIds.insert(id) }
        state.unlock()
    }
}

/// A value read and written under a lock that is never held across `await`.
final class Locked<Value>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: Value

    init(_ value: Value) {
        self.value = value
    }

    func withLock<T>(_ body: (inout Value) throws -> T) rethrows -> T {
        lock.lock()
        defer { lock.unlock() }
        return try body(&value)
    }
}

/// Waits for a task's value, and stops waiting — without cancelling the task — when the waiting
/// task is cancelled: a refresh another request is sharing goes on for the requests still waiting.
func awaitValue<T: Sendable>(of task: Task<T, any Error>) async throws -> T {
    let resumed = Locked<CheckedContinuation<T, any Error>?>(nil)
    let finished = Locked(false)
    return try await withTaskCancellationHandler {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<T, any Error>) in
            let alreadyCancelled = finished.withLock { done -> Bool in
                if done { return true }
                resumed.withLock { $0 = continuation }
                return false
            }
            if alreadyCancelled {
                continuation.resume(throwing: CancellationError())
                return
            }
            Task {
                let result = await task.result
                let waiting = finished.withLock { done -> CheckedContinuation<T, any Error>? in
                    if done { return nil }
                    done = true
                    return resumed.withLock { current in
                        defer { current = nil }
                        return current
                    }
                }
                waiting?.resume(with: result)
            }
        }
    } onCancel: {
        let waiting = finished.withLock { done -> CheckedContinuation<T, any Error>? in
            if done { return nil }
            done = true
            return resumed.withLock { current in
                defer { current = nil }
                return current
            }
        }
        waiting?.resume(throwing: CancellationError())
    }
}
