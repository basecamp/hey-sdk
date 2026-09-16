import Foundation

/// Provides access tokens for the HEY API.
///
/// ```swift
/// struct Tokens: TokenProvider {
///     let store: TokenStore
///     func accessToken() async throws -> String { try await store.token() }
///     func refresh() async throws -> Bool { try await store.renew() }
/// }
/// ```
///
/// The client asks for the token under the same lock it refreshes under, so a request is
/// signed with credentials a refresh cannot change halfway: ``accessToken()`` and
/// ``refresh()`` are called one at a time, and neither may use the client itself, which would
/// wait on that lock for good. A refresh belongs to the client: the request that earned it
/// being cancelled does not cancel it, and every stale request waits on the same one.
public protocol TokenProvider: Sendable {
    /// The current access token.
    func accessToken() async throws -> String

    /// Asked once when a request is answered with 401. Answer `true` when the next
    /// ``accessToken()`` will hand over renewed credentials, and the request is sent again. A
    /// refresh that answers `false` or throws is not repeated for the requests already signed
    /// with the credentials it could not renew: each of them fails with that answer, what was
    /// thrown reaching them as the cause of a ``HeyError/auth(message:detail:)``. Only a
    /// request signed after the failure asks again. A provider that renews of its own accord —
    /// ``accessToken()`` handing over a new token ahead of the old one's expiry — has refreshed
    /// as surely as this would: a 401 on the old token is answered by resending with the new
    /// one, and this is not asked.
    func refresh() async throws -> Bool
}

extension TokenProvider {
    public func refresh() async throws -> Bool { false }
}

/// A ``TokenProvider`` that always hands over the same token. It prints as `[REDACTED]`.
public struct StaticTokenProvider: TokenProvider, CustomStringConvertible {
    private let token: SensitiveString

    /// A provider of `token`. A blank token — what an unset environment variable hands over —
    /// is refused as a usage error, the failure every other mistake in the client's setup is.
    public init(_ token: String) throws {
        guard !token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw HeyError.usage(message: "Access token must not be blank")
        }
        self.token = SensitiveString(token)
    }

    public func accessToken() async throws -> String { token.expose() }

    public var description: String { "StaticTokenProvider(\(token))" }
}

/// How a request is authenticated. The default is ``BearerAuth``, which puts a
/// ``TokenProvider``'s token in `Authorization`; a strategy of your own can sign any other way.
/// ``authenticate(_:)`` and ``refresh()`` are called one at a time, under the client's refresh
/// lock, and neither may send a request through the client itself.
public protocol AuthStrategy: Sendable {
    /// Puts the credentials on a request. Called before every request, and again for a
    /// redirect that stays on HEY.
    func authenticate(_ request: inout HTTPRequest) async throws

    /// Asked once when a request is answered with 401; see ``TokenProvider/refresh()``.
    func refresh() async throws -> Bool
}

extension AuthStrategy {
    public func refresh() async throws -> Bool { false }
}

/// Sends a ``TokenProvider``'s token as `Authorization: Bearer`, which is how HEY takes one.
public struct BearerAuth: AuthStrategy {
    let tokenProvider: any TokenProvider

    public init(tokenProvider: any TokenProvider) {
        self.tokenProvider = tokenProvider
    }

    public func authenticate(_ request: inout HTTPRequest) async throws {
        request.headers.set("Authorization", try await bearer())
    }

    public func refresh() async throws -> Bool {
        try await tokenProvider.refresh()
    }

    /// What this would sign with now, as the header value: asked before a refresh, so a token
    /// the provider has already renewed is not renewed again.
    func bearer() async throws -> String {
        let token = try await tokenProvider.accessToken()
        // The whole of what a header value may not carry, not only the line breaks: the
        // transport would refuse the rest too, and might quote the token doing it.
        if token.unicodeScalars.contains(where: { ($0.value < 0x20 && $0 != "\t") || $0.value == 0x7F }) {
            throw HeyError.auth(message: "access token is not a valid header value", detail: ErrorDetail())
        }
        return "Bearer \(token)"
    }
}
