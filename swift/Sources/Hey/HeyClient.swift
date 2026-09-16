import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// The wait before the first resend of a path the model says nothing about; each one after
/// doubles it.
private let defaultBaseDelay: Duration = .seconds(1)

/// The statuses a request the model says nothing about — a path the caller wrote — is resent on.
private let retryableStatuses = [429, 500, 502, 503, 504]
private let accountFilterParameter = "filtered_account_id"
private let maxRedirects = 10
private let redirectStatuses: Set<Int> = [301, 302, 303, 307, 308]

/// The redirects a browser form answers a completed write with. A 301, 307 or 308 is a request
/// to send it again, not an answer.
private let formAnswerStatuses: Set<Int> = [302, 303]

/// The root client for the HEY API: one authenticated identity, presenting mail from All
/// Accounts unless derived for one linked account with ``forAccount(_:)``.
///
/// Every service the model describes is a property of the client — `client.boxes`,
/// `client.messages`, `client.timeTracks`.
///
/// ```swift
/// let client = try HeyClient(accessToken: "your-token")
/// let boxes = try await client.boxes.list()
/// ```
public final class HeyClient: Sendable {
    let shared: Shared
    /// The linked account this client presents, or nil for All Accounts.
    public let accountId: Int?
    let scope: ScopeState
    /// Whether this is the client the initializer made, which owns the transport; a client derived
    /// from it does not.
    private let root: Bool

    /// The configuration this client was built from.
    public var config: HeyConfig { shared.config }

    /// Where HEY is.
    public var baseURL: URL { shared.baseURL }

    /// The hooks this client reports to.
    public var hooks: any HeyHooks { shared.hooks }

    /// A client that signs every request with a fixed access token.
    public convenience init(
        accessToken: String, config: HeyConfig = HeyConfig(), hooks: (any HeyHooks)? = nil,
        transport: (any Transport)? = nil, cache: (any ResponseCache)? = nil
    ) throws {
        try self.init(
            tokenProvider: try StaticTokenProvider(accessToken), config: config, hooks: hooks, transport: transport,
            cache: cache)
    }

    /// A client that signs every request with a provider's token, and asks it to refresh after a
    /// 401.
    public convenience init(
        tokenProvider: any TokenProvider, config: HeyConfig = HeyConfig(), hooks: (any HeyHooks)? = nil,
        transport: (any Transport)? = nil, cache: (any ResponseCache)? = nil
    ) throws {
        try self.init(auth: BearerAuth(tokenProvider: tokenProvider), config: config, hooks: hooks, transport: transport, cache: cache)
    }

    /// A client that authenticates however `auth` does.
    ///
    /// The transport the SDK ships is used unless one is handed over; there is no way to hand
    /// over a configured `URLSession` that follows redirects itself, since that would run ahead of
    /// the credential handling on redirects the client is responsible for.
    public convenience init(
        auth: any AuthStrategy, config: HeyConfig = HeyConfig(), hooks: (any HeyHooks)? = nil,
        transport: (any Transport)? = nil, cache: (any ResponseCache)? = nil
    ) throws {
        try self.init(
            auth: auth, config: config, hooks: hooks, transport: transport, cache: cache,
            clock: ContinuousClock().monotonicNow, sleeper: { try await Task.sleep(for: $0) })
    }

    init(
        auth: any AuthStrategy, config: HeyConfig, hooks: (any HeyHooks)?, transport: (any Transport)?,
        cache: (any ResponseCache)?, clock: @escaping @Sendable () -> Duration,
        sleeper: @escaping @Sendable (Duration) async throws -> Void
    ) throws {
        try config.validate()
        guard let parsed = parseAbsoluteURL(config.baseURL) else {
            throw HeyError.usage(message: "Invalid base URL: \(config.baseURL)")
        }
        try requireSecureEndpoint(parsed)
        // A base URL is an origin and a path prefix, nothing more: a query would ride along on
        // every request, and a userinfo or fragment would ride into every URL the hooks and logs
        // see.
        if parsed.query != nil || parsed.user != nil || parsed.fragment != nil {
            throw HeyError.usage(message: "base URL must carry no query, credentials or fragment: \(originDescription(parsed))")
        }
        shared = Shared(
            config: config,
            baseURL: parsed,
            transport: transport ?? URLSessionTransport(timeout: config.timeout),
            auth: auth,
            cache: config.enableCache ? (cache ?? InMemoryCache()) : nil,
            hooks: hooks ?? NoopHooks(),
            clock: clock,
            sleeper: sleeper
        )
        accountId = nil
        scope = ScopeState()
        root = true
    }

    init(shared: Shared, accountId: Int?, scope: ScopeState, root: Bool) {
        self.shared = shared
        self.accountId = accountId
        self.scope = scope
        self.root = root
    }

    // MARK: - Building requests

    /// Starts a request for one of the modelled routes. Generated service methods call this.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when `params` is not exactly as long as the
    ///   route's parameters, rather than sending a path with a `{param}` left in it.
    public func operation(_ route: Route, _ params: [any CustomStringConvertible & Sendable]) throws -> HeyOperation {
        try HeyOperation.forRoute(route, params)
    }

    /// Starts a request for a path the model does not cover. The path is relative to the base URL
    /// and gets the same credentials, `.json` suffix, account scope and retry treatment as a
    /// modelled one.
    public func request(_ method: HTTPMethod, _ path: String) -> HeyOperation {
        HeyOperation.raw(method, path)
    }

    /// A request to one of the endpoints HEY serves only as a browser form: the path as the caller
    /// wrote it, a browser's `Accept`, and the redirect taken for the answer rather than followed.
    /// It is not retried, whatever its method; a 401 is the exception, and is answered by a
    /// credential refresh and one resend like every other request.
    ///
    /// The model describes none of these paths, so say what the call means by setting
    /// ``Operation/info`` with ``writeInfo(service:operation:resourceType:resourceId:)`` before
    /// sending it.
    public func form(_ method: HTTPMethod, _ path: String) -> HeyOperation {
        var operation = request(method, path)
        operation.formRepresentation()
        operation.captureRedirects()
        operation.idempotent = false
        return operation
    }

    // MARK: - Sending

    /// Sends an operation and decodes its JSON body.
    public func send<T: Decodable & Sendable>(_ operation: HeyOperation, as type: T.Type = T.self) async throws -> T {
        let label = operation.label
        return try await execute(operation) { try decode($0, type, label) }
    }

    /// Sends an operation whose answer carries no body worth reading.
    public func sendVoid(_ operation: HeyOperation) async throws {
        _ = try await execute(operation)
    }

    /// Sends an operation and reads its body as text: the HTML page a route serves no JSON for.
    public func sendText(_ operation: HeyOperation) async throws -> String {
        try await execute(operation) { $0.text() }
    }

    /// Sends an operation that answers a status meaning "nothing there" with nil.
    public func sendOptional<T: Decodable & Sendable>(_ operation: HeyOperation, as type: T.Type = T.self) async throws -> T? {
        let label = operation.label
        return try await execute(operation) { $0.empty ? nil : try decode($0, type, label) }
    }

    /// Sends a paginated read and keeps the cursor HEY answered with, and the route, so the next
    /// page is read under the same policy.
    public func sendPage<T: Decodable & Sendable>(_ operation: HeyOperation, as type: T.Type = T.self) async throws -> Page<T> {
        let label = operation.label
        let info = operation.info
        let route = operation.route
        let skipsCache = operation.skipsCache
        let base = shared.baseURL
        return try await execute(operation) { response in
            try Page.of(decode(response, type, label), response: response, baseURL: base, info: info, route: route, skipsCache: skipsCache)
        }
    }

    /// Sends a form request and reads the redirect it answered with.
    public func sendForm(_ operation: HeyOperation) async throws -> FormResponse {
        try await execute(operation) { FormResponse.of($0) }
    }

    private func decode<T: Decodable>(_ response: Response, _ type: T.Type, _ label: String) throws -> T {
        do {
            return try response.json(type)
        } catch let HeyError.api(message, status, _, detail) {
            throw HeyError.api(message: "\(label): \(message)", httpStatus: status, retryable: false, detail: detail)
        }
    }

    /// Sends an operation: applies credentials and account scope, retries transient failures when
    /// the operation is idempotent, resends once after a refreshed 401, and answers a cached body on
    /// 304. Non-2xx statuses become errors unless the operation treats them as empty.
    @discardableResult
    public func execute(_ operation: HeyOperation) async throws -> Response {
        try await execute(operation) { $0 }
    }

    /// Sends an operation and reads its answer with `transform` — a decode, a parse — inside the
    /// operation the hooks hear, so an answer that will not read ends the operation with the error
    /// the caller gets, and its duration counts the reading.
    public func execute<T>(_ operation: HeyOperation, transform: (Response) throws -> T) async throws -> T {
        guard !shared.isClosed else { throw HeyError.usage(message: "client is closed") }
        if operation.isQuiet { return try transform(try await dispatch(operation)) }
        return try await asOperation(operation.info) { try transform(try await dispatch(operation)) }
    }

    /// Runs `block` as one operation the hooks hear: started before it, ended after it with
    /// whatever it answered or threw. A convenience made of several requests — a form post and the
    /// read-back that answers it — sends each of them quiet inside this, so the hooks hear one
    /// operation that ends when the last request is in, with the error the caller gets when any of
    /// them fails.
    func asOperation<T>(_ info: OperationInfo, _ block: () async throws -> T) async throws -> T {
        guard !shared.isClosed else { throw HeyError.usage(message: "client is closed") }
        let hooks = shared.hooks
        let started = shared.clock()
        hooks.onOperationStart(info)
        do {
            let value = try await block()
            hooks.onOperationEnd(info, result: OperationResult(duration: shared.clock() - started))
            return value
        } catch {
            // Whatever ended the operation — an error from HEY, a cancellation, a token provider
            // that threw — the hooks hear the end of what they heard the start of.
            hooks.onOperationEnd(info, result: OperationResult(duration: shared.clock() - started, error: reported(error, "operation cancelled")))
            throw error
        }
    }

    /// What the hooks are told an operation or request failed with: a cancellation is named as one.
    private func reported(_ error: any Error, _ cancelled: String) -> any Error {
        isCancellation(error) ? HeyError.network(message: cancelled, retryable: false, detail: ErrorDetail()) : error
    }

    private func dispatch(_ operation: HeyOperation) async throws -> Response {
        let url = urlFor(operation)
        let answered = try await attempt(operation, url)
        let outcome = Result { try finish(operation, answered) }
        let fromCache = (try? outcome.get())?.fromCache ?? false
        var error: (any Error)?
        if case let .failure(failure) = outcome { error = failure }
        shared.hooks.onRequestEnd(
            answered.info,
            result: RequestResult(statusCode: answered.received.status, duration: answered.duration, fromCache: fromCache, error: error))
        return try outcome.get()
    }

    /// What the retry loop may spend on one operation. A modelled route brings its own policy from
    /// the model; the client's settings only make that gentler. A path the caller wrote runs on the
    /// client's settings alone. Whatever the policy, an operation that is not idempotent is sent
    /// once, and so is everything when retries are off.
    private func budget(_ operation: HeyOperation) -> Budget {
        let config = shared.config
        // One send plus the retries, without wrapping when the retries are as many as an Int holds.
        let ceiling = config.maxRetries == Int.max ? Int.max : config.maxRetries + 1
        let attempts: Int
        let retryOn: [Int]
        let delay: Duration
        if let policy = operation.route?.retry, policy.max > 0 {
            attempts = min(policy.max, ceiling)
            retryOn = policy.retryOn
            delay = max(.milliseconds(policy.baseDelayMs), config.baseRetryDelay ?? .zero)
        } else if operation.route != nil {
            attempts = 1
            retryOn = []
            delay = defaultBaseDelay
        } else {
            attempts = ceiling
            retryOn = retryableStatuses
            delay = config.baseRetryDelay ?? defaultBaseDelay
        }
        return Budget(
            attempts: operation.idempotent && config.enableRetry ? attempts : 1,
            retryOn: retryOn,
            delay: min(delay, config.maxRetryDelay))
    }

    private struct Budget {
        let attempts: Int
        let retryOn: [Int]
        let delay: Duration
    }

    private struct Answered {
        let received: Received
        let cached: (key: String, entry: CachedResponse)?
        let info: RequestInfo
        let duration: Duration
        /// The headers the auth strategy set on the request, lowercased: what an answer that echoes
        /// them must not leave in the cache.
        let credentialNames: Set<String>
    }

    /// An answer read whole while the connection was still live, which is all the SDK keeps of a
    /// response: the status and headers, the body up to its bound, and the refusal when the body ran
    /// past it.
    private struct Received {
        let url: URL
        let status: Int
        let headers: HTTPHeaders
        let body: Data
        let refusal: HeyError?
        /// Whether a redirect was followed to get here: the answer is then another resource's, not
        /// the one the cache entry was for.
        let redirected: Bool
        /// Whether the request this answers went out with the credentials: a hop to another origin
        /// drops them, and they do not come back.
        let authenticated: Bool
        /// The credentials the request this answers was signed under — the last signing, when a hop
        /// was signed again.
        let signedUnder: Generation
        /// The `Authorization` the request this answers carried, for a 401 to be checked against
        /// what the provider would sign with now.
        let bearer: String?
    }

    /// The auth strategy failed to sign a hop; the failure is passed on as the strategy threw it.
    private struct Unsigned: Error {
        let failure: any Error
    }

    /// Sends the operation as many times as its retry budget and HEY's answers call for, and hands
    /// back the answer it stopped on.
    private func attempt(_ operation: HeyOperation, _ url: URL) async throws -> Answered {
        let hooks = shared.hooks
        let budget = budget(operation)
        var attempts = budget.attempts
        var attempt = 1
        var delay = budget.delay
        var refreshed = false
        var cached: (key: String, entry: CachedResponse)?

        while true {
            let prepared = try await prepare(operation, url, cached)
            cached = prepared.cached
            // A URL that authenticates itself carries its signature in the open, so the hooks hear
            // its origin and nothing more.
            let info = RequestInfo(
                method: operation.method.rawValue,
                url: operation.isUnsigned ? redactURLs(url.absoluteString) : url.absoluteString,
                attempt: attempt)
            hooks.onRequestStart(info)
            let started = shared.clock()
            let received: Received
            do {
                received = try await transmit(operation, url, prepared)
            } catch let unsigned as Unsigned {
                // The strategy could not sign a hop: its failure, as it would be on the first
                // request, not a network failure a resend would repeat.
                hooks.onRequestEnd(info, result: RequestResult(statusCode: 0, duration: shared.clock() - started, error: unsigned.failure))
                throw unsigned.failure
            } catch {
                if isCancellation(error) {
                    hooks.onRequestEnd(
                        info, result: RequestResult(statusCode: 0, duration: shared.clock() - started, error: reported(error, "request cancelled")))
                    throw error
                }
                let failure = (error as? HeyError) ?? networkFailure(error)
                hooks.onRequestEnd(info, result: RequestResult(statusCode: 0, duration: shared.clock() - started, error: failure))
                if failure.isRetryable, attempt < attempts {
                    let wait = waitFor(delay)
                    hooks.onRetry(info, attempt: attempt + 1, error: failure, delay: wait)
                    try await shared.sleep(wait)
                    delay = nextDelay(delay)
                    attempt += 1
                    continue
                }
                throw failure
            }
            let duration = shared.clock() - started

            let status = received.status
            let retryable = budget.retryOn.contains(status)
            // A 401 from a hop that carried no credentials rejected none of HEY's: there is nothing
            // to refresh, and nothing a resend would change.
            if status == 401, received.authenticated, !refreshed {
                let renewed: Bool
                do {
                    renewed = try await refreshCredentials(received.signedUnder, received.bearer)
                } catch {
                    hooks.onRequestEnd(info, result: RequestResult(statusCode: status, duration: duration, error: reported(error, "request cancelled")))
                    throw error
                }
                if renewed {
                    let cause = HeyError.auth(message: "Token refreshed", detail: ErrorDetail())
                    hooks.onRequestEnd(info, result: RequestResult(statusCode: status, duration: duration, error: cause))
                    hooks.onRetry(info, attempt: attempt + 1, error: cause, delay: .zero)
                    refreshed = true
                    attempt += 1
                    attempts = max(attempts, attempt)
                    continue
                }
            }
            if retryable, attempt < attempts {
                let cause = HeyError.fromResponse(status: status, method: operation.method, headers: received.headers, body: Data())
                // A Retry-After on any status that earns a resend — a 503 says how long the outage
                // is expected to last as plainly as a 429 says how long to back off — honoured as
                // given, zero included: HEY saying "now" is not a reason to wait the backoff instead.
                let wait = retryAfterSeconds(received.headers["Retry-After"]).map { Duration.seconds($0) } ?? waitFor(delay)
                hooks.onRequestEnd(info, result: RequestResult(statusCode: status, duration: duration, error: cause))
                hooks.onRetry(info, attempt: attempt + 1, error: cause, delay: wait)
                try await shared.sleep(wait)
                delay = nextDelay(delay)
                attempt += 1
                continue
            }
            // An answer reached through a redirect is another resource's: the entry looked up for the
            // URL asked for neither satisfies its 304 nor takes its body.
            return Answered(
                received: received, cached: received.redirected ? nil : cached, info: info, duration: duration,
                credentialNames: prepared.credentials.names)
        }
    }

    /// A transport failure as the SDK reports it: what went wrong, with any URL the transport
    /// quoted cut back to its origin, since a redirect target can carry a signed query. The
    /// transport's own error is not kept, for the same reason. A timeout is as retryable as any
    /// other failure to get an answer; the operation's idempotency and its budget say whether it is
    /// resent.
    private func networkFailure(_ error: any Error) -> HeyError {
        .network(
            message: "Network error", retryable: true,
            detail: ErrorDetail(hint: HeyError.truncate(redactURLs(String(describing: error)))))
    }

    // MARK: - Credential refresh

    /// Answers a 401 with fresh credentials, once for all the requests the stale ones earned it
    /// on. Refreshes go one at a time, and a request that was signed before the last refresh is
    /// simply resent: the credentials it will pick up are already the new ones. A refresh that did
    /// not renew them is shared the same way: every request signed with the credentials it failed
    /// to renew gets its answer — not renewed, or what it threw — rather than a refresh of its own,
    /// so an outage at the token's issuer costs one call per set of credentials, not one per
    /// request. A request signed after that failure earns a fresh attempt, since its 401 is news. A
    /// failed refresh leaves the credentials as they were, so a later refresh of them is a refresh
    /// of every request's still signed under them: a 401 arriving while it runs joins it, and is
    /// resent if it renews them.
    ///
    /// The refresh runs in a task of its own rather than the request's, so a request cancelled
    /// while waiting for it leaves it running: a refresh half done is a rotated token nobody holds,
    /// and every other stale request is waiting on the same one.
    private func refreshCredentials(_ signedUnder: Generation, _ rejected: String?) async throws -> Bool {
        guard !shared.isClosed else { throw HeyError.usage(message: "client is closed") }
        let shared = self.shared
        enum Decision {
            case answer(Result<Bool, any Error>)
            case wait(Task<Bool, any Error>)
        }
        let decision: Decision = shared.refreshState.withLock { state in
            if state.refreshes != signedUnder.refreshes { return .answer(.success(true)) }
            if let inFlight = state.inFlight { return .wait(inFlight) }
            if state.runs != signedUnder.runs { return .answer(state.lastRefresh ?? .success(false)) }
            let task = Task.detached { try await shared.runRefresh(rejected) }
            state.inFlight = task
            return .wait(task)
        }
        let renewed: Bool
        switch decision {
        case let .answer(result):
            renewed = try result.get()
        case let .wait(task):
            // A request cancelled while it waits stops waiting; the refresh goes on for the rest.
            renewed = try await awaitValue(of: task)
        }
        try Task.checkCancellation()
        return renewed
    }

    // MARK: - URLs

    func urlFor(_ operation: HeyOperation) -> URL {
        var components: URLComponents
        if let url = operation.url, let parsed = URLComponents(url: url, resolvingAgainstBaseURL: true) {
            components = parsed
        } else {
            let parts = operation.path.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)
            let path = String(parts[0])
            let full = operation.appendsJSONSuffix ? withJSONExtension(path) : path
            components = URLComponents(url: shared.baseURL, resolvingAgainstBaseURL: true) ?? URLComponents()
            var basePath = components.percentEncodedPath
            while basePath.hasSuffix("/") { basePath.removeLast() }
            var relative = Substring(full)
            while relative.hasPrefix("/") { relative.removeFirst() }
            components.percentEncodedPath = basePath + "/" + relative
            // The caller's query goes out as written; decoding it here would turn a %26 into a
            // second parameter.
            components.percentEncodedQuery = parts.count > 1 && !parts[1].isEmpty ? String(parts[1]) : nil
        }
        var query = components.percentEncodedQuery.map { [$0] } ?? []
        for (name, value) in operation.query {
            query.append("\(percentEncodeComponent(name))=\(percentEncodeComponent(value))")
        }
        components.percentEncodedQuery = query.isEmpty ? nil : query.joined(separator: "&")
        let built = components.url ?? shared.baseURL
        // An unsigned request goes to a URL that authenticates itself, as built: the account scope
        // is HEY's parameter, not the storage service's, even when the two share an origin.
        return operation.isUnsigned ? built : scoped(built)
    }

    /// The URL with the client's account scope on it, when it has one and the URL is HEY's:
    /// whatever the URL carried for the filter already, the scope wins, on the first request and on
    /// every redirect that stays on the origin.
    private func scoped(_ url: URL) -> URL {
        guard let accountId, isSameOrigin(url, shared.baseURL),
              var components = URLComponents(url: url, resolvingAgainstBaseURL: true)
        else { return url }
        var pairs = (components.percentEncodedQuery ?? "").split(separator: "&").map(String.init)
        pairs.removeAll { pair in
            let name = pair.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)[0]
            return (String(name).removingPercentEncoding ?? String(name)) == accountFilterParameter
        }
        pairs.append("\(accountFilterParameter)=\(accountId)")
        components.percentEncodedQuery = pairs.joined(separator: "&")
        return components.url ?? url
    }

    // MARK: - Preparing and signing

    private struct Prepared {
        let request: HTTPRequest
        let cached: (key: String, entry: CachedResponse)?
        /// What the auth strategy put on the request: the headers a hop to another origin must not
        /// carry, and what partitions the cache.
        let credentials: Credentials
        /// The credentials the request was signed under, so a 401 knows whether they are already
        /// stale, or already known not to renew.
        let signedUnder: Generation
    }

    /// The headers an auth strategy added or changed on a request, by lowercased name, with their
    /// values.
    private struct Credentials {
        let headers: [String: [String]]

        var names: Set<String> { Set(headers.keys) }

        /// What partitions the cache: every credential header, canonically ordered, so a
        /// cookie-signed identity is kept apart from a bearer-signed one and two identities that
        /// share a bearer but differ in another header are kept apart too. Each name and each value
        /// goes in with its length in front, so two values can never read as one and one as two.
        /// Nil when the strategy set nothing, in which case nothing is cached.
        var partition: String? {
            let joined = headers.sorted { $0.key < $1.key }.map { name, values in
                "\(name.count):\(name)\(values.count):" + values.map { "\($0.count):\($0)" }.joined()
            }.joined()
            return joined.isEmpty ? nil : joined
        }
    }

    /// Signs a request with the auth strategy, under the refresh lock so no refresh lands between
    /// the signing and the count that says which credentials went out, and answers what the
    /// strategy put on it. A hop that stays on the origin is signed again for its own URL and
    /// method, once the previous signature is off; a hop to another origin is never signed.
    private func sign(_ request: inout HTTPRequest) async throws -> (Credentials, Generation) {
        let before = request.headers
        let shared = self.shared
        // A request cancelled while a refresh or another signing holds the turn stops waiting
        // here, unsigned and unsent.
        try await shared.refreshing.acquire()
        let signedUnder: Generation
        do {
            try await shared.auth.authenticate(&request)
            // A header value the wire cannot carry is refused here, without quoting it: a
            // credential is exactly what a strategy sets.
            for (name, value) in request.headers where !before.values(for: name).contains(value) {
                if value.unicodeScalars.contains(where: { ($0.value < 0x20 && $0 != "\t") || $0.value == 0x7F }) {
                    throw HeyError.auth(message: "auth strategy set a header that is not a valid header value", detail: ErrorDetail())
                }
            }
            let bearer = request.headers["Authorization"]
            signedUnder = shared.refreshState.withLock { state in
                if shared.auth is BearerAuth {
                    // A provider that hands over a new token of its own accord — renewing ahead of
                    // expiry, as OAuth libraries do — has refreshed the credentials as surely as a
                    // refresh would: a 401 on the token before it is answered by resending, not by
                    // refreshing the new one over the top. Only the SDK's own strategy is read this
                    // way; a strategy of the caller's may sign every request differently.
                    if let last = state.lastBearer, bearer != last { state.refreshes += 1 }
                    state.lastBearer = bearer
                }
                return Generation(refreshes: state.refreshes, runs: state.runs)
            }
            shared.refreshing.release()
        } catch {
            shared.refreshing.release()
            throw error
        }
        var added: [String: [String]] = [:]
        for name in request.headers.names where request.headers.values(for: name) != before.values(for: name) {
            added[name] = request.headers.values(for: name)
        }
        return (Credentials(headers: added), signedUnder)
    }

    /// Builds the request for one attempt, and looks the response cache up the first time it is
    /// asked for a key.
    private func prepare(_ operation: HeyOperation, _ url: URL, _ previous: (key: String, entry: CachedResponse)?) async throws -> Prepared {
        var request = HTTPRequest(method: operation.method.rawValue, url: url)
        request.headers.set("User-Agent", shared.config.userAgent)
        request.headers.set("Accept", operation.accept)
        for (name, value) in operation.headers { request.headers.add(name, value) }
        if let body = operation.body {
            request.headers.set("Content-Type", body.contentType)
            request.body = body.bytes
        }
        // An unsigned request goes out as built: no strategy touches it, so nothing partitions a
        // cache for it and a 401 it earns is about the credentials it carried of its own.
        let credentials: Credentials
        let signedUnder: Generation
        if operation.isUnsigned {
            credentials = Credentials(headers: [:])
            signedUnder = shared.generation()
        } else {
            (credentials, signedUnder) = try await sign(&request)
        }

        var cached = previous
        if let cache = cacheFor(operation), let partition = credentials.partition {
            let key = cacheKey(url: url.absoluteString, credentials: partition)
            if cached?.key != key { cached = lookUp(cache, key) }
        } else {
            cached = nil
        }
        if let etag = cached?.entry.etag, !etag.isEmpty {
            request.headers.set("If-None-Match", etag)
        }
        return Prepared(request: request, cached: cached, credentials: credentials, signedUnder: signedUnder)
    }

    private func lookUp(_ cache: any ResponseCache, _ key: String) -> (key: String, entry: CachedResponse) {
        guard let entry = cache.get(key) else { return (key, CachedResponse(etag: "", body: Data())) }
        if entry.body.count <= shared.config.effectiveMaxResponseBodyBytes { return (key, entry) }
        cache.invalidate(key)
        return (key, CachedResponse(etag: "", body: Data()))
    }

    /// The cache the operation reads and writes, when there is one to use. Only a JSON GET is cached.
    private func cacheFor(_ operation: HeyOperation) -> (any ResponseCache)? {
        !operation.skipsCache && operation.method == .get && operation.accept == "application/json" ? shared.cache : nil
    }

    // MARK: - Transmitting

    /// Sends one request and follows the redirects it is answered with, up to ten hops, unless the
    /// operation is one that takes the redirect for its answer. Credentials stay on the origin they
    /// were meant for: a hop to another origin goes out without the headers the auth strategy set,
    /// whatever it called them, and without the usual suspects. A hop that stays on HEY keeps the
    /// client's account scope, unless the request went out unsigned: that URL is the storage
    /// service's, wherever it lives, and its hops go as named.
    ///
    /// The transport reads each body only as far as its bound, so a body is refused while it is
    /// still arriving rather than after all of it has been held.
    private func transmit(_ operation: HeyOperation, _ start: URL, _ prepared: Prepared) async throws -> Received {
        var url = start
        var request = prepared.request
        var credentials = prepared.credentials
        var signedUnder = prepared.signedUnder
        var hops = 0
        var authenticated = !operation.isUnsigned
        let captures = operation.capturesRedirects
        let emptyOn = operation.emptyOn
        let bound = isParsed(operation.accept) ? shared.config.effectiveMaxResponseBodyBytes : HeyConfig.maxResponseBodyBytes

        while true {
            let from = url
            let response = try await shared.transport.send(request) { status, headers in
                // A redirect about to be followed, a 304, a status the operation takes for "nothing
                // there", and the redirect a form takes for its answer carry nothing the SDK reads,
                // so their bodies are not read at all — and so cannot be refused for their size.
                if !captures, redirectStatuses.contains(status), let location = headers["Location"],
                   resolveReference(from, location) != nil
                {
                    return nil
                }
                if status == 304 || emptyOn.contains(status) || (captures && formAnswerStatuses.contains(status)) {
                    return nil
                }
                return bound
            }
            if !captures, redirectStatuses.contains(response.status), let location = response.headers["Location"],
               let target = resolveReference(url, location)
            {
                guard hops < maxRedirects else {
                    throw HeyError.network(
                        message: "\(operation.label) redirected more than \(maxRedirects) times", retryable: false, detail: ErrorDetail())
                }
                // A hop of an unsigned request goes where it was sent, as the first request did: the
                // storage URL authenticates itself, and the account scope is not its.
                let next = operation.isUnsigned ? target : scoped(target)
                try requireSecureEndpoint(next)
                let sameOrigin = isSameOrigin(url, next)
                if !sameOrigin { authenticated = false }
                request = redirected(request, status: response.status, from: url, to: next, credentialNames: credentials.names)
                // The signature was for the URL and method the hop left behind; on the origin it is
                // made again for the ones it goes to, and off it never is. The count moves with it: a
                // 401 on the hop is about the credentials it carried.
                if sameOrigin, authenticated {
                    do {
                        (credentials, signedUnder) = try await sign(&request)
                    } catch {
                        if isCancellation(error) { throw error }
                        throw Unsigned(failure: error)
                    }
                }
                url = next
                hops += 1
                continue
            }
            let refusal: HeyError? = response.bodyExceeded
                ? .api(
                    message: "response body exceeds \(bound) bytes", httpStatus: nil, retryable: false,
                    detail: ErrorDetail(responseTooLarge: true))
                : nil
            return Received(
                url: url, status: response.status, headers: response.headers,
                body: response.bodyExceeded ? Data() : response.body, refusal: refusal, redirected: hops > 0,
                authenticated: authenticated, signedUnder: signedUnder, bearer: credentials.headers["authorization"]?.first)
        }
    }

    /// The request for the hop: the outgoing one's headers less the validator, less the strategy's
    /// own headers (a fresh signing puts them back when the hop stays on the origin), and less
    /// anything credential-like when it does not.
    private func redirected(_ outgoing: HTTPRequest, status: Int, from: URL, to: URL, credentialNames: Set<String>) -> HTTPRequest {
        // A 303 says fetch the answer, whatever the method; a 301 or 302 is only allowed to turn a
        // POST into a GET, and leaves a PUT, PATCH or DELETE as it was, since a GET in its place
        // would report a mutation done that never reached where it was sent; a 307 or 308 keeps
        // everything.
        let keepBody: Bool
        switch status {
        case 303: keepBody = outgoing.method == "GET" || outgoing.method == "HEAD"
        case 301, 302: keepBody = outgoing.method != "POST"
        default: keepBody = true
        }
        var request = HTTPRequest(method: keepBody ? outgoing.method : "GET", url: to)
        let sameOrigin = isSameOrigin(from, to)
        for (name, value) in outgoing.headers {
            // The validator was the resource asked for's; the one pointed to has its own.
            if name.caseInsensitiveCompare("If-None-Match") == .orderedSame { continue }
            if credentialNames.contains(name.lowercased()) { continue }
            if !sameOrigin, isSensitiveHeader(name) { continue }
            // A hop that drops the body drops everything that described it — the type, the length,
            // the checksum storage wanted — since a GET with a Content-MD5 and nothing to check it
            // against is a request the destination may well refuse.
            if !keepBody, isContentHeader(name) { continue }
            request.headers.add(name, value)
        }
        if keepBody { request.body = outgoing.body }
        return request
    }

    // MARK: - Finishing

    private func finish(_ operation: HeyOperation, _ answered: Answered) throws -> Response {
        let received = answered.received
        let status = received.status
        let headers = received.headers
        if status == 304 {
            guard let (key, entry) = answered.cached, !entry.etag.isEmpty else {
                throw HeyError.api(message: "304 received but no cached response available", httpStatus: 304, retryable: false, detail: ErrorDetail())
            }
            // The 304 answers with the cached body under the cached headers, updated by what it
            // carried, and the entry keeps that update: a validator or cursor HEY moved is what the
            // next read goes out with, and a no-store on the 304 ends the entry.
            let merged = entry.headersUpdated(by: headers)
            if let cache = shared.cache {
                if forbidsStoring(merged) {
                    cache.invalidate(key)
                } else {
                    cache.set(key, CachedResponse(
                        etag: merged["ETag"] ?? entry.etag, body: entry.body,
                        headers: storableHeaders(merged, answered.credentialNames)))
                }
            }
            return Response(status: 200, headers: merged, body: entry.body, url: received.url, fromCache: true, empty: false)
        }
        if let refusal = received.refusal {
            if (200...299).contains(status) { throw refusal }
            // The status is what matters about a failure, and a body the client would not hold is no
            // reason to lose it: the error is the one the status maps to, told why its body is missing.
            throw HeyError.fromResponse(status: status, method: operation.method, headers: headers, body: Data(), refusal: refusal)
        }
        let body = received.body
        if (200...299).contains(status) {
            if let key = answered.cached?.key, let cache = shared.cache {
                let etag = headers["ETag"]
                if forbidsStoring(headers) {
                    // An answer HEY says not to keep is not kept, and neither is what it replaced.
                    cache.invalidate(key)
                } else if let etag, !body.isEmpty {
                    cache.set(key, CachedResponse(etag: etag, body: body, headers: storableHeaders(headers, answered.credentialNames)))
                } else {
                    // A success that cannot be revalidated — no validator, or nothing to hold — has
                    // replaced what was held, so the old entry goes rather than being sent back as a
                    // validator for a body HEY has moved on from.
                    cache.invalidate(key)
                }
            }
            return Response(status: status, headers: headers, body: body, url: received.url, fromCache: false, empty: false)
        }
        if operation.emptyOn.contains(status) || (operation.capturesRedirects && formAnswerStatuses.contains(status)) {
            return Response(status: status, headers: headers, body: body, url: received.url, fromCache: false, empty: true)
        }
        if operation.capturesRedirects, redirectStatuses.contains(status) {
            throw HeyError.api(
                message: "\(operation.label) answered \(status); a form write completes with a 302 or 303, and a \(status) asks for the request again",
                httpStatus: status, retryable: false, detail: ErrorDetail(requestId: headers["X-Request-Id"]))
        }
        throw HeyError.fromResponse(status: status, method: operation.method, headers: headers, body: body)
    }

    private func waitFor(_ delay: Duration) -> Duration {
        let jitter = shared.config.maxRetryJitter
        let jitterMs = Int(jitter.components.seconds) * 1000 + Int(jitter.components.attoseconds / 1_000_000_000_000_000)
        let added: Duration = jitterMs > 0 ? .milliseconds(Int.random(in: 0...jitterMs)) : .zero
        return min(delay + added, shared.config.maxRetryDelay)
    }

    private func nextDelay(_ delay: Duration) -> Duration {
        min(delay * 2, shared.config.maxRetryDelay)
    }

    // MARK: - Account scope

    /// Derives a client that presents mail from one linked account and acts as that account's user
    /// and default sender. The account is checked against the identity first, so a stale or foreign
    /// id fails here rather than on the first read.
    ///
    /// Calendar, journal, habits and time tracking belong to the identity, so they read the same
    /// through a scoped client.
    public func forAccount(_ accountId: Int) async throws -> HeyClient {
        guard accountId > 0 else { throw HeyError.usage(message: "account id must be positive") }
        let unscoped = HeyClient(shared: shared, accountId: nil, scope: ScopeState(), root: false)
        let identity = try await unscoped.identity()
        let accessible = (identity.accounts ?? []).contains { $0.id == accountId && accountIsAccessible($0) }
        guard accessible else {
            throw HeyError.notFound(message: "accessible account not found: \(accountId)", detail: ErrorDetail())
        }
        let scope = ScopeState()
        scope.defaultSenderId = defaultSender(identity, accountId)
        scope.accountUserId = (identity.allUsers ?? []).first { $0.accountId == accountId }?.id
        return HeyClient(shared: shared, accountId: accountId, scope: scope, root: false)
    }

    /// The sender a message goes out as when the caller names none: the scoped account's default
    /// sender, or the identity's default sender for All Accounts.
    public func defaultSenderId() async throws -> Int {
        try await scope.lock.withLock {
            if let id = scope.defaultSenderId { return id }
            let identity = try await self.identity()
            guard let id = defaultSender(identity, accountId) ?? (accountId == nil ? identity.primaryContact?.id : nil) else {
                if let accountId {
                    throw HeyError.notFound(message: "sender for account not found: \(accountId)", detail: ErrorDetail())
                }
                throw HeyError.api(message: "no sender found in identity", httpStatus: nil, retryable: false, detail: ErrorDetail())
            }
            scope.defaultSenderId = id
            return id
        }
    }

    /// The identity's user in the scoped account, which is what a record is filed under.
    public func accountUserId() async throws -> Int {
        guard let accountId else { throw HeyError.usage(message: "account user id needs an account-scoped client") }
        return try await scope.lock.withLock {
            if let id = scope.accountUserId { return id }
            let identity = try await self.identity()
            guard let user = (identity.allUsers ?? []).first(where: { $0.accountId == accountId }) else {
                throw HeyError.notFound(message: "user for account not found: \(accountId)", detail: ErrorDetail())
            }
            scope.accountUserId = user.id
            return user.id
        }
    }

    /// The identity read the client makes for itself, sent as the `GetIdentity` operation.
    func identity() async throws -> Identity {
        try await send(try operation(Routes.getIdentity, []))
    }

    /// Stops the client. Only the client the initializer made does so: one derived with
    /// ``forAccount(_:)`` shares the transport with the client it came from and its siblings, and
    /// closing it closes nothing. A request after closing is refused as a usage error.
    public func close() {
        guard root else { return }
        let inFlight = shared.refreshState.withLock { state -> Task<Bool, any Error>? in
            state.closed = true
            return state.inFlight
        }
        inFlight?.cancel()
    }
}

// MARK: - Shared state

/// What every client derived from one root shares: the configuration, the transport, the
/// credentials and the state that keeps their refreshes one at a time.
final class Shared: Sendable {
    let config: HeyConfig
    let baseURL: URL
    let transport: any Transport
    let auth: any AuthStrategy
    let cache: (any ResponseCache)?
    let hooks: any HeyHooks
    /// The client's clock, which only runs forward.
    let clock: @Sendable () -> Duration
    let sleeper: @Sendable (Duration) async throws -> Void

    /// Signing and refreshing go one at a time, and never together. Held for a refresh's whole run.
    let refreshing = AsyncMutex()

    /// The counts, the refresh in flight and how the last one ended, read and written only for a
    /// moment at a time, so a stale request can find the refresh in flight while it runs.
    let refreshState = Locked(RefreshState())

    struct RefreshState {
        /// How many times the credentials have been renewed, so a 401 answered after someone else
        /// renewed them is resent rather than refreshed again.
        var refreshes = 0
        /// How many refreshes have run to an answer, renewed or not, so a 401 on credentials a
        /// refresh already failed to renew shares that answer.
        var runs = 0
        /// The refresh in flight, for every stale request to wait on; nil between refreshes.
        var inFlight: Task<Bool, any Error>?
        /// How the last refresh ended: renewed, not renewed, or with what it threw.
        var lastRefresh: Result<Bool, any Error>?
        /// The bearer the SDK's own strategy last put on a request, so a token the provider rotates
        /// of its own accord is seen for the renewal it is. Nil until a signing, and again after a
        /// refresh.
        var lastBearer: String?
        /// Set once the root client closes: a request after that is a mistake the caller is told
        /// about, not a cancellation.
        var closed = false
    }

    init(
        config: HeyConfig, baseURL: URL, transport: any Transport, auth: any AuthStrategy, cache: (any ResponseCache)?,
        hooks: any HeyHooks, clock: @escaping @Sendable () -> Duration, sleeper: @escaping @Sendable (Duration) async throws -> Void
    ) {
        self.config = config
        self.baseURL = baseURL
        self.transport = transport
        self.auth = auth
        self.cache = cache
        self.hooks = hooks
        self.clock = clock
        self.sleeper = sleeper
    }

    var isClosed: Bool { refreshState.withLock { $0.closed } }

    func generation() -> Generation {
        refreshState.withLock { Generation(refreshes: $0.refreshes, runs: $0.runs) }
    }

    /// Waits, with the longest wait the platform can sleep standing in for anything longer.
    func sleep(_ duration: Duration) async throws {
        try await sleeper(min(duration, .seconds(Int64.max / 1_000_000_000)))
    }

    /// One refresh, under the signing lock for its whole run so no request is signed while the
    /// credentials are changing hands, and the counts move with them. Before the SDK's own bearer
    /// strategy is asked to refresh, its provider is asked what it would sign with now: a token
    /// other than the rejected one is a renewal the provider already made, so the request is resent
    /// with it rather than the new token's refresh token being spent. A provider that cannot hand
    /// over a token at all is the refresh failing.
    func runRefresh(_ rejected: String?) async throws -> Bool {
        do {
            try await refreshing.acquire()
        } catch {
            // Cancelled before it had the turn: the client closed.
            refreshState.withLock { $0.inFlight = nil }
            throw error
        }
        defer { refreshing.release() }
        let outcome: Result<Bool, any Error>
        do {
            if let bearerAuth = auth as? BearerAuth, let rejected, try await bearerAuth.bearer() != rejected {
                outcome = .success(true)
            } else {
                outcome = .success(try await auth.refresh())
            }
        } catch {
            if Task.isCancelled {
                // The refresh itself was cancelled — the client closed — so there is no answer to
                // share, and nothing left in flight.
                refreshState.withLock { $0.inFlight = nil }
                throw error
            }
            // Anything else the strategy threw is its answer, a timeout of its own included: one
            // every request signed under these credentials gets, as an SDK failure.
            outcome = .failure(refreshFailed(error))
        }
        refreshState.withLock { state in
            if case .success(true) = outcome {
                state.refreshes += 1
                // The next signing carries the renewed token; that is this refresh, not another.
                state.lastBearer = nil
            }
            state.runs += 1
            state.lastRefresh = outcome
            state.inFlight = nil
        }
        return try outcome.get()
    }

    /// What a strategy threw from its refresh, as the failure the caller gets: an SDK error of its
    /// own passes through, anything else becomes an authentication error carrying it as the cause,
    /// with its description in the hint less any URL, since a token endpoint's may carry a
    /// credential.
    private func refreshFailed(_ error: any Error) -> HeyError {
        if let error = error as? HeyError { return error }
        return .auth(
            message: "credential refresh failed",
            detail: ErrorDetail(hint: HeyError.truncate(redactURLs(String(describing: error))), cause: error))
    }
}

/// What a client works out about the identity it presents and keeps for as long as it lives. A
/// client derived with ``HeyClient/forAccount(_:)`` starts an empty one of its own.
final class ScopeState: @unchecked Sendable {
    let lock = AsyncMutex()
    var defaultSenderId: Int?
    var accountUserId: Int?
    var boxKinds: [String: Int]?
}

/// The credentials a request went out with, by the counts at its signing: how many refreshes had
/// renewed them, and how many refreshes had run at all. The first says whether a 401 is already
/// answered by someone else's refresh; the second whether a refresh of these very credentials
/// already ran and failed, in which case its answer is this request's too.
struct Generation: Equatable {
    let refreshes: Int
    let runs: Int
}

// MARK: - Helpers

private func accountIsAccessible(_ account: Account) -> Bool {
    let status = account.status ?? ""
    let purpose = account.purpose ?? ""
    return status == "active" || (status == "inactive" && (purpose == "work" || purpose == "domains"))
}

private func defaultSender(_ identity: Identity, _ accountId: Int?) -> Int? {
    let senders = (identity.senders ?? []).filter { accountId == nil || $0.accountId == accountId }
    return (senders.first { $0.default == true } ?? senders.first)?.id
}

/// HEY answers JSON to paths ending in `.json`; a path whose last segment has no extension gets one.
func withJSONExtension(_ path: String) -> String {
    let lastSegment = path.split(separator: "/", omittingEmptySubsequences: false).last ?? ""
    return path.isEmpty || path.hasSuffix("/") || lastSegment.contains(".") ? path : "\(path).json"
}

/// Whether the answer carries `Cache-Control: no-store`, which forbids holding any part of it.
private func forbidsStoring(_ headers: HTTPHeaders) -> Bool {
    headers.values(for: "Cache-Control").contains { value in
        value.split(separator: ",").contains { directive in
            directive.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)[0]
                .trimmingCharacters(in: .whitespaces).caseInsensitiveCompare("no-store") == .orderedSame
        }
    }
}

/// The headers a cache entry keeps beside the body: everything HEY sent that is not a credential —
/// the usual suspects, and whatever the strategy signs with, should HEY echo it.
private func storableHeaders(_ headers: HTTPHeaders, _ credentialNames: Set<String>) -> HTTPHeaders {
    HTTPHeaders(headers.filter { !isSensitiveHeader($0.name) && !credentialNames.contains($0.name.lowercased()) }.map { ($0.name, $0.value) })
}

/// Whether the answer to a request that asked for this is a document the SDK holds whole and goes
/// on to parse — JSON, a `+json` type, or HTML, anywhere in the `Accept` list — and so is held to
/// the configured cap. Anything else, a blob or an export, is held to the fixed one.
func isParsed(_ accept: String) -> Bool {
    accept.isEmpty || accept.split(separator: ",").contains { part in
        let mediaType = part.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: false)[0].trimmingCharacters(in: .whitespaces)
        return mediaType == "application/json" || mediaType.hasSuffix("+json") || mediaType == "text/html"
    }
}

/// Whether an error is a cancellation rather than a failure: Swift's own, or the transport's.
func isCancellation(_ error: any Error) -> Bool {
    if error is CancellationError { return true }
    if let urlError = error as? URLError, urlError.code == .cancelled { return true }
    return false
}

extension ContinuousClock {
    /// How far the clock has run since an arbitrary start, for measuring with subtraction.
    var monotonicNow: @Sendable () -> Duration {
        let origin = now
        return { ContinuousClock.now - origin }
    }
}
