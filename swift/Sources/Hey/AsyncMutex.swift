import Foundation

/// A lock a task can hold across `await`, taken in the order it was asked for. Swift's actors
/// are re-entrant, so they cannot keep one signing from interleaving with a refresh; this can.
final class AsyncMutex: @unchecked Sendable {
    private let state = NSLock()
    private var held = false
    private var waiters: [CheckedContinuation<Void, Never>] = []

    /// Waits for the lock and takes it.
    func acquire() async {
        await withCheckedContinuation { (continuation: CheckedContinuation<Void, Never>) in
            if tryAcquire(orWait: continuation) { continuation.resume() }
        }
    }

    /// Takes the lock when it is free, and otherwise queues the continuation to be handed it.
    private func tryAcquire(orWait continuation: CheckedContinuation<Void, Never>) -> Bool {
        state.lock()
        defer { state.unlock() }
        if !held {
            held = true
            return true
        }
        waiters.append(continuation)
        return false
    }

    /// Gives the lock up, handing it straight to the longest waiter when there is one.
    func release() {
        state.lock()
        let next: CheckedContinuation<Void, Never>?
        if waiters.isEmpty {
            held = false
            next = nil
        } else {
            next = waiters.removeFirst()
        }
        state.unlock()
        next?.resume()
    }

    /// Runs `body` holding the lock, and gives it up however `body` ends.
    func withLock<T>(_ body: () async throws -> T) async rethrows -> T {
        await acquire()
        do {
            let value = try await body()
            release()
            return value
        } catch {
            release()
            throw error
        }
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
