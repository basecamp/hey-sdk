import Foundation

/// A response the cache holds for a URL: the validator HEY sent with it, the body it
/// validates, and the headers that came with the body, so a 304 answers the page as it was
/// first read — `Link` and `X-Total-Count` included, which a 304 need not repeat.
public struct CachedResponse: Sendable, Equatable {
    /// The `ETag` HEY sent with the body, sent back as `If-None-Match` on the next read.
    public var etag: String
    /// The body as HEY answered it.
    public var body: Data
    /// The headers HEY answered the body with, less any credential.
    public var headers: HTTPHeaders

    public init(etag: String, body: Data, headers: HTTPHeaders = HTTPHeaders()) {
        self.etag = etag
        self.body = body
        self.headers = headers
    }

    /// The cached headers, with each one a 304 carried replacing what was held under its name.
    func headersUpdated(by fresh: HTTPHeaders) -> HTTPHeaders {
        var merged = headers
        for name in fresh.names { merged.remove(name) }
        for (name, value) in fresh { merged.add(name, value) }
        return merged
    }
}

/// Stores JSON responses by `ETag`, so a repeated read can be answered from a 304.
public protocol ResponseCache: Sendable {
    /// What is held under `key`, if anything.
    func get(_ key: String) -> CachedResponse?
    /// Holds a response under `key`, replacing whatever was there.
    func set(_ key: String, _ response: CachedResponse)
    /// Forgets what is held under `key`.
    func invalidate(_ key: String)
    /// Forgets everything.
    func clear()
}

/// Keeps cached responses for the life of the process. What ``HeyConfig/enableCache`` turns on
/// when no cache of your own is handed over.
public final class InMemoryCache: ResponseCache, @unchecked Sendable {
    private let lock = NSLock()
    private var entries: [String: CachedResponse] = [:]

    public init() {}

    public func get(_ key: String) -> CachedResponse? {
        lock.lock()
        defer { lock.unlock() }
        return entries[key]
    }

    public func set(_ key: String, _ response: CachedResponse) {
        lock.lock()
        defer { lock.unlock() }
        entries[key] = response
    }

    public func invalidate(_ key: String) {
        lock.lock()
        defer { lock.unlock() }
        entries.removeValue(forKey: key)
    }

    public func clear() {
        lock.lock()
        defer { lock.unlock() }
        entries.removeAll()
    }

    /// How many responses are held.
    public var count: Int {
        lock.lock()
        defer { lock.unlock() }
        return entries.count
    }
}

/// The key a read is cached under: the URL and the credentials it went out with — every header
/// the auth strategy set, not only `Authorization` — hashed, so one identity's reading is never
/// answered to another and no token sits in a key.
func cacheKey(url: String, credentials: String) -> String {
    sha256Hex(Data("\(credentials)\n\(url)".utf8))
}
