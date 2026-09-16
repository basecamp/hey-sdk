import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// The transport the SDK ships, over `URLSession`. Redirects are never followed, the body is
/// read as it arrives and the task is cancelled on the first byte past the limit, and no
/// cookie is stored or sent: the client decides what goes on each request.
public final class URLSessionTransport: Transport, @unchecked Sendable {
    private let session: URLSession
    private let delegate: Delegate

    /// A transport whose requests time out after `timeout`, or never for `nil`.
    public init(timeout: Duration? = HeyConfig.defaultTimeout) {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.httpCookieAcceptPolicy = .never
        configuration.httpShouldSetCookies = false
        configuration.httpCookieStorage = nil
        configuration.urlCache = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        if let timeout {
            let seconds = Double(timeout.components.seconds) + Double(timeout.components.attoseconds) / 1e18
            configuration.timeoutIntervalForRequest = seconds
            configuration.timeoutIntervalForResource = seconds
        } else {
            configuration.timeoutIntervalForRequest = .greatestFiniteMagnitude
            configuration.timeoutIntervalForResource = .greatestFiniteMagnitude
        }
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

        private func finish(_ task: URLSessionTask) -> TaskState? {
            lock.lock()
            defer { lock.unlock() }
            return tasks.removeValue(forKey: task.taskIdentifier)
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
            guard let state = finish(task), let continuation = state.continuation else { return }
            state.continuation = nil
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
