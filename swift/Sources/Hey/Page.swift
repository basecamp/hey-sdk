import Foundation

/// One page of a paginated read, with the cursor HEY handed out for the next one.
///
/// ```swift
/// let first = try await client.boxes.list()
/// try await client.eachPage(first) { page in print(page.value); return true }
/// ```
public struct Page<Value: Decodable & Sendable>: Sendable {
    /// The page's value: the response as HEY answered it.
    public let value: Value
    /// The URL of the page after this one, as HEY's `Link` header named it.
    public let nextURL: URL?
    /// The opaque cursor for the page after this one, to pass as `page` on the same read.
    public let nextPage: String?
    /// Where to read next once the pages run out, when HEY named one: a change feed's last page
    /// links to the cursor to poll from later rather than to another page. A walk stops here;
    /// the URL is the caller's to come back to.
    public let nextCursor: URL?
    /// The `X-Total-Count` header, when the read carried one.
    public let totalCount: Int?

    let info: OperationInfo
    let route: Route?
    /// Whether the read that produced this page left the cache alone, so the reads that walk on
    /// from it do too.
    let skipsCache: Bool

    /// Whether HEY named a page after this one.
    public var hasNext: Bool { nextURL != nil }

    static func of(_ value: Value, response: Response, baseURL: URL, info: OperationInfo, route: Route?, skipsCache: Bool) throws -> Page {
        let linked = response.header("Link").flatMap(nextLink).flatMap { resolveReference(response.url, $0) }
        // A Link that names a further page carries the page parameter; one that does not is where
        // to poll next — a change feed's last page says so — and no page at all. A cursor off
        // HEY's origin is refused here, as a page off it is refused when followed: the header is
        // the server's to write, and a caller would take the URL on trust.
        let pageParameter = route?.pageParameter ?? "page"
        let nextPage = linked.flatMap { queryValue($0, pageParameter) }
        let nextURL = nextPage == nil ? nil : linked
        let nextCursor = nextPage == nil ? linked : nil
        if let nextCursor, !isSameOrigin(nextCursor, baseURL) {
            throw HeyError.usage(message: "pagination Link header points to a different origin: \(originDescription(nextCursor))")
        }
        let totalCount = response.header("X-Total-Count").flatMap { Int($0.trimmingCharacters(in: .whitespaces)) }
        return Page(
            value: value, nextURL: nextURL, nextPage: nextPage, nextCursor: nextCursor, totalCount: totalCount,
            info: info, route: route, skipsCache: skipsCache)
    }
}

/// The first value a URL's query carries under `name`, decoded.
func queryValue(_ url: URL, _ name: String) -> String? {
    URLComponents(url: url, resolvingAgainstBaseURL: true)?.queryItems?.first { $0.name == name }?.value
}

/// The target of the `rel="next"` link in a `Link` header, or nil when the header names none.
public func nextLink(_ header: String) -> String? {
    var remaining = Substring(header)
    while true {
        guard let start = remaining.firstIndex(of: "<") else { return nil }
        let afterStart = remaining[remaining.index(after: start)...]
        guard let end = afterStart.firstIndex(of: ">") else { return nil }
        let target = String(afterStart[..<end])
        let rest = afterStart[afterStart.index(after: end)...]
        let paramsEnd = rest.firstIndex(of: "<") ?? rest.endIndex
        // The comma that separates this link value from the next is not part of its last
        // parameter: `rel="next", <...>` names next, not `next",`.
        var params = String(rest[..<paramsEnd])
        while let last = params.last, last.isWhitespace { params.removeLast() }
        if params.hasSuffix(",") { params.removeLast() }
        if linkIsNext(params) { return target }
        remaining = rest[paramsEnd...]
    }
}

private func linkIsNext(_ params: String) -> Bool {
    params.split(separator: ";", omittingEmptySubsequences: false).contains { param in
        let parts = param.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
        let name = parts[0].trimmingCharacters(in: .whitespaces)
        let value = parts.count > 1
            ? parts[1].trimmingCharacters(in: .whitespaces).trimmingCharacters(in: CharacterSet(charactersIn: "\""))
            : ""
        return name.caseInsensitiveCompare("rel") == .orderedSame
            && value.split(separator: " ").contains { $0.caseInsensitiveCompare("next") == .orderedSame }
    }
}

extension HeyClient {
    /// Reads the page after the given one, or nil when HEY named no next page. A `Link` header
    /// pointing off the HEY origin is refused rather than followed. The read announces itself as
    /// the operation the first page came from, and is resent under that operation's retry policy.
    public func nextPage<T>(_ page: Page<T>) async throws -> Page<T>? {
        guard let next = page.nextURL else { return nil }
        guard isSameOrigin(next, baseURL) else {
            throw HeyError.usage(message: "pagination Link header points to a different origin: \(originDescription(next))")
        }
        var operation = HeyOperation.at(.get, next, route: page.route)
        operation.info = page.info
        if page.skipsCache { operation.noCache() }
        return try await sendPage(operation)
    }

    /// Reads every page after the first, calling `visit` with each one, until it answers false
    /// or the pages run out. A walk that reaches the client's page limit with pages still to read
    /// stops there and fails, rather than answering a shorter list that looks complete.
    public func eachPage<T>(_ first: Page<T>, _ visit: (Page<T>) async throws -> Bool) async throws {
        var page = first
        var count = 1
        while try await visit(page) {
            guard page.hasNext else { break }
            guard count < config.maxPages else {
                throw HeyError.api(
                    message: "pagination stopped after \(config.maxPages) pages with more to read",
                    httpStatus: nil, retryable: false, detail: ErrorDetail())
            }
            guard let next = try await nextPage(page) else { break }
            page = next
            count += 1
        }
    }

    /// Every page from the first on, read as each one is asked for.
    public func pages<T>(from first: Page<T>) -> AsyncThrowingStream<Page<T>, any Error> {
        AsyncThrowingStream { continuation in
            let task = Task {
                do {
                    try await self.eachPage(first) { page in
                        continuation.yield(page)
                        return !Task.isCancelled
                    }
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }
}
