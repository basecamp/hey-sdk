import Foundation

/// Parses an absolute URL: a scheme, `://`, and a host. Nil — fail closed — for anything
/// malformed or relative, so a guard never reads a host the transport would not dial.
func parseAbsoluteURL(_ string: String) -> URL? {
    guard let components = URLComponents(string: string),
          let scheme = components.scheme, !scheme.isEmpty,
          let host = components.host, !host.isEmpty,
          string.lowercased().hasPrefix("\(scheme.lowercased())://"),
          let url = components.url
    else { return nil }
    return url
}

/// A URL reference HEY sent — a `Location`, a `Link` target — resolved against the URL it came
/// in, as RFC 3986 resolves one. An absolute reference stands on its own; a relative one is
/// resolved against the base's path, and never takes the query the request went out with, so
/// that never rides along to where the answer points. A reference that names nothing — empty,
/// blank — is nil, as is one that will not parse: a redirect to nowhere is not followed.
func resolveReference(_ base: URL, _ reference: String) -> URL? {
    let trimmed = reference.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else { return nil }
    if let absolute = parseAbsoluteURL(trimmed) { return absolute }
    guard var components = URLComponents(url: base, resolvingAgainstBaseURL: true) else { return nil }
    components.percentEncodedQuery = nil
    components.percentEncodedFragment = nil
    if trimmed.hasPrefix("?") || trimmed.hasPrefix("#") {
        // RFC 3986 keeps the base's path in front of a reference that starts at the query or
        // the fragment.
        guard let stripped = components.string, let url = URL(string: stripped + trimmed) else { return nil }
        return url
    }
    guard let stripped = components.url,
          let resolved = URL(string: trimmed, relativeTo: stripped)?.absoluteURL,
          resolved.host != nil
    else { return nil }
    return resolved
}

/// Whether a host is this machine: `localhost`, a `.localhost` name, or a loopback address.
func isLocalhostHost(_ host: String) -> Bool {
    var lowered = host.lowercased()
    if lowered.hasPrefix("["), lowered.hasSuffix("]") { lowered = String(lowered.dropFirst().dropLast()) }
    return lowered == "localhost" || lowered == "127.0.0.1" || lowered == "::1" || lowered.hasSuffix(".localhost")
}

/// The port a URL goes to, its scheme's default when it names none.
func effectivePort(_ url: URL) -> Int? {
    if let port = url.port { return port }
    switch url.scheme?.lowercased() {
    case "https": return 443
    case "http": return 80
    default: return nil
    }
}

/// Whether two URLs share a scheme, host and port, a default port equal to its explicit form.
func isSameOrigin(_ a: URL, _ b: URL) -> Bool {
    (a.scheme ?? "").lowercased() == (b.scheme ?? "").lowercased()
        && (a.host ?? "").lowercased() == (b.host ?? "").lowercased()
        && effectivePort(a) == effectivePort(b)
}

/// An origin as an error may name it: scheme and host.
func originDescription(_ url: URL) -> String {
    "\(url.scheme ?? "")://\(url.host ?? "")"
}

/// Refuses an endpoint that would carry credentials over plain HTTP, unless it is on this
/// machine.
func requireSecureEndpoint(_ url: URL) throws {
    let scheme = (url.scheme ?? "").lowercased()
    if scheme == "https" || (scheme == "http" && isLocalhostHost(url.host ?? "")) { return }
    throw HeyError.usage(message: "\(originDescription(url)) must use HTTPS")
}

/// A URL in prose: scheme, an optional userinfo, a host (an IPv6 one in brackets) with its
/// port, and then everything up to whitespace or a quote. A comma or a bracket is legal in a
/// query, so a secret could sit past one; the whole token goes rather than the part before some
/// punctuation, at the cost of a trailing comma or bracket the prose meant.
private let urlInText = try! NSRegularExpression(
    pattern: #"([a-zA-Z][a-zA-Z0-9+.-]*://)(?:[^/?#\s"'<>@]*@)?(\[[^\]\s]*\](?::\d+)?|[^/?#\s"'<>]+)[^\s"'<>]*"#)

/// The text with every URL in it cut back to its origin: a path or a query carries a signed
/// credential, and a userinfo a password.
func redactURLs(_ text: String) -> String {
    let range = NSRange(text.startIndex..., in: text)
    return urlInText.stringByReplacingMatches(in: text, range: range, withTemplate: "$1$2")
}

/// A `Location` as an error may name it: the path alone, with no userinfo, query or fragment,
/// since a signed query is what a redirect target is likeliest to carry.
func redactLocation(_ location: String) -> String {
    if let absolute = parseAbsoluteURL(location) {
        return "\(absolute.scheme ?? "")://\(absolute.host ?? "")\(absolute.path)"
    }
    // A network-path reference carries an authority, and so can carry a userinfo, without a scheme.
    if location.hasPrefix("//") {
        if let parsed = parseAbsoluteURL("https:\(location)") { return "//\(parsed.host ?? "")\(parsed.path)" }
        return "//"
    }
    return String(location.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)[0]
        .split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false)[0])
}

private let sensitiveHeaders: Set<String> = ["authorization", "proxy-authorization", "cookie", "set-cookie", "x-csrf-token"]

/// The headers that describe a request's body without saying so in their name: what RFC 9110
/// has a client drop, along with every `Content-*` header, when a redirect turns the request
/// into a GET, since they would describe a body that is no longer sent.
private let bodyHeaders: Set<String> = ["digest", "repr-digest", "last-modified"]

/// Whether a header describes the body a request carries, and so goes with it when a hop drops
/// the body: any `Content-*` header — the type, the length, the checksum storage wanted, the
/// disposition HEY named — and the few that describe it under another name.
func isContentHeader(_ name: String) -> Bool {
    let lowered = name.lowercased()
    return lowered.hasPrefix("content-") || bodyHeaders.contains(lowered)
}

/// Whether a header carries credentials and must be neither logged nor sent to another origin.
func isSensitiveHeader(_ name: String) -> Bool {
    sensitiveHeaders.contains(name.lowercased())
}
