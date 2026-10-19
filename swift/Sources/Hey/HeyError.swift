import Foundation

/// What HEY or the SDK said about a failure, beyond its message: the word on what to do, the
/// request HEY named, the body it answered with, and what caused it.
public struct ErrorDetail: Sendable {
    /// What to do about it, when there is a word to say: HEY's own message, or when to try again.
    public var hint: String?
    /// The request id HEY answered with, for looking the failure up.
    public var requestId: String?
    /// What HEY answered the failure with, up to ``HeyError/maxErrorBodyBytes``; nil when there
    /// was no body or the SDK raised the error itself.
    public var body: Data?
    /// The answer was longer than the client will hold, so ``body`` is missing and ``hint``
    /// says so.
    public var responseTooLarge: Bool
    /// What caused the failure, when something else did: what a token provider threw.
    public var cause: (any Error)?

    public init(hint: String? = nil, requestId: String? = nil, body: Data? = nil, responseTooLarge: Bool = false, cause: (any Error)? = nil) {
        self.hint = hint
        self.requestId = requestId
        self.body = body
        self.responseTooLarge = responseTooLarge
        self.cause = cause
    }
}

/// One reason HEY gave for refusing what was sent (422): the field it objected to, when it named
/// one, and what it said. See ``HeyError/fieldErrors``.
///
/// HEY answers a 422 with `{"errors": ["Subject can't be blank", ...]}`, the model's own full
/// messages, which name no field apart from the message, so `field` is nil for those; an entry
/// that is an object, `{"field": "subject", "message": "can't be blank"}` (`attribute` is read as
/// well), carries both.
public struct FieldError: Sendable, Equatable, CustomStringConvertible {
    /// The field HEY objected to, when it named one.
    public var field: String?
    /// What HEY said about it.
    public var message: String

    public init(field: String?, message: String) {
        self.field = field
        self.message = message
    }

    /// The message alone, or the field and the message when HEY named one.
    public var description: String {
        guard let field else { return message }
        return "\(field): \(message)"
    }
}

/// Every failure the SDK reports, for exhaustive `switch` matching.
///
/// ```swift
/// do {
///     let box = try await client.boxes.get(boxId: 5)
/// } catch let error as HeyError {
///     switch error {
///     case .auth: print("Token expired")
///     case .notFound: print("Not found")
///     case let .rateLimit(_, retryAfterSeconds, _): print("Retry in \(retryAfterSeconds ?? 0)s")
///     default: print(error.localizedDescription)
///     }
/// }
/// ```
public enum HeyError: Error, Sendable, LocalizedError, CustomStringConvertible {
    /// HEY refused the credentials (401).
    case auth(message: String, detail: ErrorDetail)
    /// HEY refused access (403).
    case forbidden(message: String, detail: ErrorDetail)
    /// There is no such record (404).
    case notFound(message: String, detail: ErrorDetail)
    /// Too many requests (429). Retryable, after the wait HEY named when it named one.
    case rateLimit(message: String, retryAfterSeconds: Int?, detail: ErrorDetail)
    /// The request got no answer: a connection failure, a timeout, a cancellation.
    case network(message: String, retryable: Bool, detail: ErrorDetail)
    /// Any other status HEY answered with, or an answer the SDK could not read.
    case api(message: String, httpStatus: Int?, retryable: Bool, detail: ErrorDetail)
    /// HEY rejected what was sent (422).
    case validation(message: String, httpStatus: Int, detail: ErrorDetail)
    /// The request conflicts with what HEY already holds (409), such as a time track already
    /// running.
    case conflict(message: String, detail: ErrorDetail)
    /// More than one record matches a name or identifier.
    case ambiguous(resource: String, matches: [String], hint: String?)
    /// The SDK was asked for something it cannot do: a bad argument, a bad configuration.
    case usage(message: String, hint: String?)

    public static let codeAuth = "auth_required"
    public static let codeForbidden = "forbidden"
    public static let codeNotFound = "not_found"
    public static let codeRateLimit = "rate_limit"
    public static let codeNetwork = "network"
    public static let codeAPI = "api_error"
    public static let codeValidation = "validation"
    public static let codeConflict = "conflict"
    public static let codeAmbiguous = "ambiguous"
    public static let codeUsage = "usage"

    /// The most of a failure's body an error keeps.
    public static let maxErrorBodyBytes = 1 << 20

    /// The longest message or hint an error carries.
    public static let maxErrorMessageLength = 500

    /// A usage error with no hint.
    public static func usage(message: String) -> HeyError { .usage(message: message, hint: nil) }

    /// The error category, as every HEY SDK names it.
    public var code: String {
        switch self {
        case .auth: return Self.codeAuth
        case .forbidden: return Self.codeForbidden
        case .notFound: return Self.codeNotFound
        case .rateLimit: return Self.codeRateLimit
        case .network: return Self.codeNetwork
        case .api: return Self.codeAPI
        case .validation: return Self.codeValidation
        case .conflict: return Self.codeConflict
        case .ambiguous: return Self.codeAmbiguous
        case .usage: return Self.codeUsage
        }
    }

    public var message: String {
        switch self {
        case let .auth(message, _), let .forbidden(message, _), let .notFound(message, _),
             let .rateLimit(message, _, _), let .network(message, _, _), let .api(message, _, _, _),
             let .validation(message, _, _), let .conflict(message, _), let .usage(message, _):
            return message
        case let .ambiguous(resource, _, _):
            return "Ambiguous \(resource)"
        }
    }

    public var detail: ErrorDetail? {
        switch self {
        case let .auth(_, detail), let .forbidden(_, detail), let .notFound(_, detail),
             let .rateLimit(_, _, detail), let .network(_, _, detail), let .api(_, _, _, detail),
             let .validation(_, _, detail), let .conflict(_, detail):
            return detail
        case .ambiguous, .usage:
            return nil
        }
    }

    public var hint: String? {
        switch self {
        case let .ambiguous(_, _, hint), let .usage(_, hint): return hint
        default: return detail?.hint
        }
    }

    /// The status HEY answered with, when the failure is one.
    public var httpStatus: Int? {
        switch self {
        case .auth: return 401
        case .forbidden: return 403
        case .notFound: return 404
        case .rateLimit: return 429
        case .conflict: return 409
        case let .validation(_, status, _): return status
        case let .api(_, status, _, _): return status
        case .network, .ambiguous, .usage: return nil
        }
    }

    /// Whether the request can be sent again.
    public var isRetryable: Bool {
        switch self {
        case .rateLimit: return true
        case let .network(_, retryable, _), let .api(_, _, retryable, _): return retryable
        default: return false
        }
    }

    public var requestId: String? { detail?.requestId }

    public var body: Data? { detail?.body }

    /// The failure body as text, when there is one.
    public var bodyText: String? { body.map { String(decoding: $0, as: UTF8.self) } }

    /// The answer was longer than the client will hold, so ``body`` is missing.
    public var responseTooLarge: Bool { detail?.responseTooLarge ?? false }

    /// Each reason HEY gave for a 422, in the order it gave them: the entries of the body's
    /// `errors` list, or its single `error` or `message` when it sent no list. The ``hint`` joins
    /// the same reasons. Empty for any other error, for a 422 whose body is missing or names no
    /// reason, and for a refusal the SDK raised itself.
    public var fieldErrors: [FieldError] {
        guard case let .validation(_, 422, detail) = self, let body = detail.body else { return [] }
        return Self.reasons(body)
    }

    /// The exit code a command-line tool leaves with, as every HEY SDK maps it.
    public var exitCode: Int { Self.exitCode(for: code) }

    /// Maps an error code to a command-line exit code. A conflict leaves as a validation
    /// failure, as it does in Go.
    public static func exitCode(for code: String) -> Int {
        switch code {
        case codeUsage: return 1
        case codeNotFound: return 2
        case codeAuth: return 3
        case codeForbidden: return 4
        case codeRateLimit: return 5
        case codeNetwork: return 6
        case codeAPI: return 7
        case codeAmbiguous: return 8
        case codeValidation, codeConflict: return 9
        default: return 7
        }
    }

    public var errorDescription: String? { description }

    public var description: String {
        guard let hint else { return message }
        return "\(message): \(hint)"
    }

    /// Truncates an error message to a safe length.
    static func truncate(_ text: String) -> String {
        text.count <= maxErrorMessageLength ? text : String(text.prefix(maxErrorMessageLength - 3)) + "..."
    }

    /// Maps a non-2xx answer onto the SDK's errors. The hint carries whatever message HEY put
    /// in the body when it sent one as JSON — an HTML error page is never echoed — and the body
    /// itself is kept on the error for a caller that needs more of it than a hint. A body the
    /// client refused to hold leaves the error the status maps to, told why its body is missing.
    static func fromResponse(status: Int, method: HTTPMethod, headers: HTTPHeaders, body: Data, refusal: HeyError? = nil) -> HeyError {
        let kept = body.count > maxErrorBodyBytes ? body.prefix(maxErrorBodyBytes) : body
        let serverMessage = Self.serverMessage(body) ?? refusal?.message
        let detail = ErrorDetail(
            hint: serverMessage, requestId: headers["X-Request-Id"],
            body: kept.isEmpty ? nil : Data(kept), responseTooLarge: refusal != nil)
        switch status {
        case 401:
            return .auth(message: "authentication required", detail: detail)
        case 403:
            if method == .get { return .forbidden(message: "access denied", detail: detail) }
            var scoped = detail
            scoped.hint = serverMessage ?? "Re-authenticate with full scope"
            return .forbidden(message: "Access denied: insufficient scope", detail: scoped)
        case 404:
            return .notFound(message: "resource not found", detail: detail)
        case 409:
            return .conflict(message: "conflict", detail: detail)
        case 422:
            return .validation(message: "validation error", httpStatus: 422, detail: detail)
        case 429:
            let retryAfter = retryAfterSeconds(headers["Retry-After"])
            var limited = detail
            limited.hint = serverMessage ?? retryHint(retryAfter)
            return .rateLimit(message: "rate limited - try again later", retryAfterSeconds: retryAfter, detail: limited)
        default:
            return .api(message: "API error: \(status)", httpStatus: status, retryable: (500...599).contains(status), detail: detail)
        }
    }

    static func retryHint(_ retryAfter: Int?) -> String {
        if let retryAfter, retryAfter > 0 { return "Retry after \(retryAfter) seconds" }
        return "Try again later"
    }

    /// The message a JSON failure body carries: `error`, then `message`, then an `errors` list
    /// joined. Anything that is not a JSON object — an HTML page, a bare string — carries none.
    static func serverMessage(_ body: Data) -> String? {
        guard let object = jsonObject(body) else { return nil }
        if let single = singleMessage(object) { return truncate(single) }
        let reasons = listedReasons(object)
        return reasons.isEmpty ? nil : truncate(reasons.map(\.description).joined(separator: "; "))
    }

    /// The reasons a 422's body gives: the `errors` list, or its single `error` or `message`.
    static func reasons(_ body: Data) -> [FieldError] {
        guard let object = jsonObject(body) else { return [] }
        let listed = listedReasons(object)
        if !listed.isEmpty { return listed }
        return singleMessage(object).map { [FieldError(field: nil, message: $0)] } ?? []
    }

    /// The entries of a body's `errors` list: a message, or an object naming the field and the
    /// message. An entry that is neither is skipped, so one malformed entry costs the caller
    /// none of the rest; an `errors` member that is not a list has no entries.
    private static func listedReasons(_ object: [String: Any]) -> [FieldError] {
        guard let errors = object["errors"] as? [Any] else { return [] }
        return errors.compactMap { entry in
            if let message = nonBlank(entry) { return FieldError(field: nil, message: message) }
            guard let fields = entry as? [String: Any], let message = nonBlank(fields["message"]) else { return nil }
            return FieldError(field: nonBlank(fields["field"]) ?? nonBlank(fields["attribute"]), message: message)
        }
    }

    private static func singleMessage(_ object: [String: Any]) -> String? {
        nonBlank(object["error"]) ?? nonBlank(object["message"])
    }

    private static func nonBlank(_ value: Any?) -> String? {
        guard let text = value as? String, !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return nil }
        return text
    }

    private static func jsonObject(_ body: Data) -> [String: Any]? {
        guard !body.isEmpty else { return nil }
        return (try? JSONSerialization.jsonObject(with: body)) as? [String: Any]
    }
}

/// The seconds a `Retry-After` header names: an integer, or an HTTP-date reduced to the time
/// left until it. Nil for a header that is missing or unreadable, or a negative number, which is
/// no wait HEY named.
func retryAfterSeconds(_ value: String?) -> Int? {
    guard let trimmed = value?.trimmingCharacters(in: .whitespaces), !trimmed.isEmpty else { return nil }
    if let seconds = Int(trimmed) { return seconds < 0 ? nil : seconds }
    let formatter = DateFormatter()
    formatter.locale = Locale(identifier: "en_US_POSIX")
    formatter.timeZone = TimeZone(identifier: "GMT")
    formatter.dateFormat = "EEE, dd MMM yyyy HH:mm:ss zzz"
    guard let date = formatter.date(from: trimmed) else { return nil }
    let remaining = date.timeIntervalSinceNow
    return remaining > 0 ? Int(remaining.rounded(.up)) : 0
}
