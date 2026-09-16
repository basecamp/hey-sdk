import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// The transport the SDK ships, over `URLSession`. Redirects are never followed, the body is
/// read as it arrives and the task is cancelled on the first byte past the limit, and no
/// cookie is stored or sent: the client decides what goes on each request.
///
/// The timeout covers the whole exchange, from sending the request to the last byte of the body,
/// and is kept by the transport rather than by `URLSession`: on Linux, `URLSession` rounds its
/// intervals down to whole seconds, so half a second would time out at once.
public final class URLSessionTransport: Transport, @unchecked Sendable {
    private let session: URLSession
    private let delegate: Delegate
    private let timeout: Duration?

    /// A transport whose requests time out after `timeout`, or never for `nil`.
    public init(timeout: Duration? = HeyConfig.defaultTimeout) {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.httpCookieAcceptPolicy = .never
        configuration.httpShouldSetCookies = false
        configuration.httpCookieStorage = nil
        configuration.urlCache = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        // Long enough never to be the one that fires, and small enough for Linux to convert to
        // whole milliseconds without overflowing.
        let never: TimeInterval = 365 * 24 * 60 * 60
        configuration.timeoutIntervalForRequest = never
        configuration.timeoutIntervalForResource = never
        self.timeout = timeout
        let delegate = Delegate()
        self.delegate = delegate
        self.session = URLSession(configuration: configuration, delegate: delegate, delegateQueue: nil)
    }

    deinit {
        session.invalidateAndCancel()
    }

    public func send(
        _ request: HTTPRequest, bodyLimit: @escaping @Sendable (_ status: Int, _ headers: HTTPHeaders) -> Int?
    ) async throws -> HTTPResponse {
        var urlRequest = URLRequest(url: request.url)
        urlRequest.httpMethod = request.method
        urlRequest.httpShouldHandleCookies = false
        for (name, value) in request.headers {
            urlRequest.addValue(value, forHTTPHeaderField: name)
        }
        urlRequest.httpBody = request.body

        let task = session.dataTask(with: urlRequest)
        let state = TaskState(bodyLimit: bodyLimit)
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<HTTPResponse, Error>) in
                state.continuation = continuation
                delegate.register(task, state)
                if Task.isCancelled {
                    task.cancel()
                }
                if let timeout {
                    delegate.startDeadline(for: task, state, after: timeout)
                }
                task.resume()
            }
        } onCancel: {
            task.cancel()
        }
    }

    /// What one task has read so far, and where its answer goes.
    fileprivate final class TaskState: @unchecked Sendable {
        let bodyLimit: @Sendable (Int, HTTPHeaders) -> Int?
        var continuation: CheckedContinuation<HTTPResponse, Error>?
        var status = 0
        var headers = HTTPHeaders()
        var limit: Int?
        var readBody = true
        var body = Data()
        var exceeded = false
        /// Guarded by the delegate's lock: the deadline fires on a queue of its own.
        var timedOut = false
        var deadline: DispatchWorkItem?

        init(bodyLimit: @escaping @Sendable (Int, HTTPHeaders) -> Int?) {
            self.bodyLimit = bodyLimit
        }
    }

    fileprivate final class Delegate: NSObject, URLSessionDataDelegate, @unchecked Sendable {
        private let lock = NSLock()
        private var tasks: [Int: TaskState] = [:]

        func register(_ task: URLSessionTask, _ state: TaskState) {
            lock.lock()
            tasks[task.taskIdentifier] = state
            lock.unlock()
        }

        private func state(for task: URLSessionTask) -> TaskState? {
            lock.lock()
            defer { lock.unlock() }
            return tasks[task.taskIdentifier]
        }

        /// The state of a task that has finished, and whether its deadline had passed. The
        /// deadline is called off, so it cannot fire for a task that is no longer running.
        private func finish(_ task: URLSessionTask) -> (TaskState, Bool)? {
            lock.lock()
            defer { lock.unlock() }
            guard let state = tasks.removeValue(forKey: task.taskIdentifier) else { return nil }
            state.deadline?.cancel()
            state.deadline = nil
            return (state, state.timedOut)
        }

        func startDeadline(for task: URLSessionTask, _ state: TaskState, after timeout: Duration) {
            let item = DispatchWorkItem { [weak self, weak task] in
                guard let self, let task else { return }
                self.lock.lock()
                let running = self.tasks[task.taskIdentifier] === state
                if running { state.timedOut = true }
                self.lock.unlock()
                if running { task.cancel() }
            }
            lock.lock()
            state.deadline = item
            lock.unlock()
            let seconds = Double(timeout.components.seconds) + Double(timeout.components.attoseconds) / 1e18
            // Past a century there is nothing to schedule, and nothing Dispatch could represent.
            guard seconds < 100 * 365 * 24 * 60 * 60 else { return }
            DispatchQueue.global().asyncAfter(deadline: .now() + seconds, execute: item)
        }

        func urlSession(
            _ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
            newRequest request: URLRequest, completionHandler: @escaping @Sendable (URLRequest?) -> Void
        ) {
            // The client follows redirects itself.
            completionHandler(nil)
        }

        func urlSession(
            _ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
            completionHandler: @escaping @Sendable (URLSession.ResponseDisposition) -> Void
        ) {
            if let state = state(for: dataTask), let http = response as? HTTPURLResponse {
                state.status = http.statusCode
                var headers = HTTPHeaders()
                for (name, value) in http.allHeaderFields {
                    headers.add(String(describing: name), String(describing: value))
                }
                state.headers = headers
                state.limit = state.bodyLimit(http.statusCode, headers)
                state.readBody = state.limit != nil
                if !state.readBody {
                    // A body the client will not look at is let go rather than read.
                    completionHandler(.cancel)
                    return
                }
            }
            completionHandler(.allow)
        }

        func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
            guard let state = state(for: dataTask), state.readBody, !state.exceeded, let limit = state.limit else { return }
            let room = limit + 1 - state.body.count
            if data.count >= room {
                state.body.append(data.prefix(max(room, 0)))
                if state.body.count > limit {
                    state.exceeded = true
                    // Let the rest go here and now rather than read what will be refused.
                    dataTask.cancel()
                }
            } else {
                state.body.append(data)
            }
        }

        func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
            guard let (state, timedOut) = finish(task), let continuation = state.continuation else { return }
            state.continuation = nil
            if timedOut, !state.exceeded {
                continuation.resume(throwing: URLError(.timedOut))
                return
            }
            if state.status != 0, !state.readBody {
                continuation.resume(returning: HTTPResponse(status: state.status, headers: state.headers, body: Data()))
                return
            }
            if state.exceeded {
                continuation.resume(returning: HTTPResponse(
                    status: state.status, headers: state.headers, body: state.body, bodyExceeded: true))
                return
            }
            if let error {
                continuation.resume(throwing: error)
                return
            }
            continuation.resume(returning: HTTPResponse(
                status: state.status, headers: state.headers, body: state.readBody ? state.body : Data()))
        }
    }
}
