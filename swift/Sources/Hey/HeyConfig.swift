import Foundation

/// Configuration for a ``HeyClient``. Every setting has a default, so `HeyConfig()` is a
/// client of HEY itself with retries on and no cache.
public struct HeyConfig: Sendable, Equatable {
    /// The SDK's own version. `make bump VERSION=x.y.z` moves it with the other SDKs'.
    public static let version = "0.31.0"

    /// The HEY API version this SDK targets; `scripts/sync-api-version.sh` moves it with the spec.
    public static let apiVersion = "2026-09-16"

    public static let defaultBaseURL = "https://app.hey.com"

    /// What the client calls itself: the SDK and the API contract it was built against.
    public static let defaultUserAgent = "hey-sdk-swift/\(version) (api:\(apiVersion))"
    public static let defaultMaxRetries = 3
    public static let defaultMaxPages = 10_000
    public static let defaultMaxResponseBodyBytes = 16 << 20

    /// The most the client buffers of an answer the configurable cap leaves alone: a blob,
    /// whatever a form request answered.
    public static let maxResponseBodyBytes = 50 << 20
    public static let defaultTimeout: Duration = .seconds(30)
    public static let defaultMaxRetryDelay: Duration = .seconds(30)
    public static let defaultMaxRetryJitter: Duration = .milliseconds(100)

    /// Where HEY is. Plain HTTP is refused anywhere but this machine.
    public var baseURL: String
    /// The `User-Agent` every request carries.
    public var userAgent: String
    /// Keep JSON reads by `ETag`, so a repeated read is answered from a 304.
    public var enableCache: Bool
    /// Resend transient failures of operations that can be sent again.
    public var enableRetry: Bool
    /// How long the transport the SDK ships gives an answer to arrive, or `nil` for no limit. A
    /// transport of the caller's own keeps its own timeout.
    public var timeout: Duration?
    /// The most times any operation is resent after a transient failure. A modelled route is
    /// resent as many times as its own policy allows and no more; this only lowers that.
    public var maxRetries: Int
    /// The most pages a walk reads before it stops.
    public var maxPages: Int
    /// The least the client waits before the first resend; a route's own policy holds it up
    /// when longer.
    public var baseRetryDelay: Duration?
    /// The most the client waits between attempts, jitter included. The wait a `Retry-After`
    /// names is honoured as given.
    public var maxRetryDelay: Duration
    /// The most added at random to each wait, so resends from many clients do not land together.
    public var maxRetryJitter: Duration
    /// The most a JSON or HTML answer may deliver before the client refuses to hold it. Zero or
    /// less is the default.
    public var maxResponseBodyBytes: Int

    public init(
        baseURL: String = HeyConfig.defaultBaseURL,
        userAgent: String = HeyConfig.defaultUserAgent,
        enableCache: Bool = false,
        enableRetry: Bool = true,
        timeout: Duration? = HeyConfig.defaultTimeout,
        maxRetries: Int = HeyConfig.defaultMaxRetries,
        maxPages: Int = HeyConfig.defaultMaxPages,
        baseRetryDelay: Duration? = nil,
        maxRetryDelay: Duration = HeyConfig.defaultMaxRetryDelay,
        maxRetryJitter: Duration = HeyConfig.defaultMaxRetryJitter,
        maxResponseBodyBytes: Int = HeyConfig.defaultMaxResponseBodyBytes
    ) {
        self.baseURL = baseURL
        self.userAgent = userAgent
        self.enableCache = enableCache
        self.enableRetry = enableRetry
        self.timeout = timeout
        self.maxRetries = maxRetries
        self.maxPages = maxPages
        self.baseRetryDelay = baseRetryDelay
        self.maxRetryDelay = maxRetryDelay
        self.maxRetryJitter = maxRetryJitter
        self.maxResponseBodyBytes = maxResponseBodyBytes
    }

    /// Refuses a setting the client cannot run on, as a usage error.
    func validate() throws {
        if maxPages <= 0 { throw HeyError.usage(message: "maxPages must be > 0, got: \(maxPages)") }
        if maxRetries < 0 { throw HeyError.usage(message: "maxRetries must be >= 0, got: \(maxRetries)") }
        if let timeout, timeout < .milliseconds(1) {
            throw HeyError.usage(message: "timeout must be at least one millisecond or nil, got: \(timeout)")
        }
        // A wait has to be one the client can wait: not negative.
        if let baseRetryDelay, baseRetryDelay < .zero {
            throw HeyError.usage(message: "baseRetryDelay must be >= 0, got: \(baseRetryDelay)")
        }
        if maxRetryDelay < .zero { throw HeyError.usage(message: "maxRetryDelay must be >= 0, got: \(maxRetryDelay)") }
        if maxRetryJitter < .zero { throw HeyError.usage(message: "maxRetryJitter must be >= 0, got: \(maxRetryJitter)") }
    }

    /// The body cap the client holds a parsed answer to.
    var effectiveMaxResponseBodyBytes: Int {
        maxResponseBodyBytes <= 0 ? HeyConfig.defaultMaxResponseBodyBytes : maxResponseBodyBytes
    }
}
