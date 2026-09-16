import Foundation

/// The title HEY gives the extensions group in the navigation payload.
private let navigationGroup = "Extensions"

/// What a contact URL puts the contact's id after, as in `/contacts/4821`.
private let contactPath = "/contacts/"

/// One email extenzion. The id is the extenzion's *contact* id — the one every write endpoint
/// takes, and the one its `appUrl` carries. The id a JSON write answers with belongs to the
/// Extenzion record instead, which no endpoint takes.
///
/// Kotlin and Rust call this `Extenzion`; in Swift that name is the generated model's, which
/// carries the record's id, so this one is named for the id it carries instead.
public struct ExtenzionContact: Sendable, Equatable {
    /// The extenzion's contact id.
    public var id: Int
    /// The part before the `@`, as in `sales`.
    public var name: String
    /// The contact's page in HEY's web app.
    public var appUrl: String

    public init(id: Int, name: String, appUrl: String) {
        self.id = id
        self.name = name
        self.appUrl = appUrl
    }
}

/// A new extenzion.
public struct CreateExtenzionParams: Sendable, Equatable {
    /// The extenzion name: "sales" becomes `sales@yourdomain.com`.
    public var name: String
    /// The member email addresses.
    public var members: [String]

    public init(name: String, members: [String] = []) {
        self.name = name
        self.members = members
    }
}

/// A partial revision: nil leaves a field as it is.
public struct UpdateExtenzionParams: Sendable, Equatable {
    /// An empty name is no name, and is left off the wire like a nil one.
    public var name: String?
    /// The whole membership, which replaces what the extenzion had rather than adding to it. Nil
    /// leaves it alone; an empty list removes every member.
    public var members: [String]?

    public init(name: String? = nil, members: [String]? = nil) {
        self.name = name
        self.members = members
    }
}

/// The custom addresses a custom-domain account carries, such as `sales@yourdomain.com`, on top
/// of the generated surface (`delete`). ``create(accountId:params:)`` and
/// ``update(accountId:extenzionId:params:)`` post a form to the `.json` path, so a current server
/// answers the written extenzion while one without the JSON branch redirects and hands nothing
/// back.
extension ExtenzionsService {
    /// The extenzions on the account. This reads the navigation payload rather than scraping the
    /// extenzions page, so it carries only what navigation carries: each extenzion's name and its
    /// contact URL. It is one operation, `Extenzions.ListExtenzions`, rather than the identity
    /// read it happens to be built on, which is why it sends the navigation route itself instead
    /// of going through ``IdentityService/getNavigation()``.
    public func list() async throws -> [ExtenzionContact] {
        var operation = try client.operation(Routes.getNavigation, [])
        operation.info = OperationInfo(service: "Extenzions", operation: "ListExtenzions", resourceType: "extenzion", isMutation: false)
        return try await client.execute(operation) { try extenzionsFromNavigation($0.json(NavigationResponse.self)) }
    }

    /// Creates an extenzion and answers it. A server without the JSON create branch hands nothing
    /// back, and the answer is then nil.
    public func create(accountId: Int, params: CreateExtenzionParams) async throws -> ExtenzionContact? {
        var fields = [("extenzion[name]", params.name)]
        for member in params.members { fields.append(("extenzion[members][]", member)) }
        var operation = client.form(.post, "/accounts/\(accountId)/domains/extenzions.json")
        operation.info = writeInfo(service: "Extenzions", operation: "CreateExtenzion", resourceType: "extenzion")
        operation.form(fields)
        return try await client.execute(operation, transform: extenzionFromFormResponse)
    }

    /// Revises an extenzion and answers it. The id is the extenzion's contact id. A server without
    /// the JSON update branch hands nothing back, and the answer is then nil.
    public func update(accountId: Int, extenzionId: Int, params: UpdateExtenzionParams) async throws -> ExtenzionContact? {
        var fields: [(String, String)] = []
        if let name = params.name, !name.isEmpty { fields.append(("extenzion[name]", name)) }
        // The membership is replaced when the field is present at all, so an empty list has to be
        // on the wire as one blank value — a form carries no empty array — while nil, which leaves
        // the membership alone, sends nothing.
        if let members = params.members {
            if members.isEmpty {
                fields.append(("extenzion[members][]", ""))
            } else {
                for member in members { fields.append(("extenzion[members][]", member)) }
            }
        }
        var operation = client.form(.patch, "/accounts/\(accountId)/domains/extenzions/\(extenzionId).json")
        operation.info = writeInfo(service: "Extenzions", operation: "UpdateExtenzion", resourceType: "extenzion", resourceId: extenzionId)
        operation.form(fields)
        return try await client.execute(operation, transform: extenzionFromFormResponse)
    }
}

/// The extenzions in navigation's "Extensions" group. The group leads with the "All Extensions"
/// link, which names no contact and so falls out on its own.
func extenzionsFromNavigation(_ navigation: NavigationResponse) throws -> [ExtenzionContact] {
    try (navigation.items ?? [])
        .filter { $0.title == navigationGroup }
        .flatMap { $0.menuItems ?? [] }
        .compactMap(listedExtenzion)
}

private func listedExtenzion(_ entry: NavigationItem) throws -> ExtenzionContact? {
    let appUrl = entry.appUrl ?? ""
    guard let id = try contactIdFromUrl(appUrl) else { return nil }
    return ExtenzionContact(id: id, name: entry.title ?? "", appUrl: appUrl)
}

/// The extenzion a JSON write answered with. A server without the JSON branch redirects to the
/// extenzions page instead, which leaves nothing to read.
private func extenzionFromFormResponse(_ answered: Response) throws -> ExtenzionContact? {
    if FormResponse.of(answered).body.isEmpty { return nil }
    let payload = try answered.json(Extenzion.self)
    let appUrl = payload.appUrl ?? ""
    guard let id = try contactIdFromUrl(appUrl) else {
        throw HeyError.api(message: "no contact id in \"\(appUrl)\"", httpStatus: answered.status, retryable: false, detail: ErrorDetail())
    }
    return ExtenzionContact(id: id, name: payload.name ?? "", appUrl: appUrl)
}

/// The contact id a contact URL carries, as Go's `/contacts/(\d+)` reads it. A URL naming no
/// contact at all answers nil — navigation's "All Extensions" link is one, and it is meant to
/// fall out of the list. A URL that names one the SDK cannot read is a failure instead of another
/// such link: dropping it would hide an extenzion the caller would then never hear about.
func contactIdFromUrl(_ contactUrl: String) throws -> Int? {
    var searchFrom = contactUrl.startIndex
    while let found = contactUrl.range(of: contactPath, range: searchFrom..<contactUrl.endIndex) {
        let digits = contactUrl[found.upperBound...].prefix { $0.asciiValue.map { (0x30...0x39).contains($0) } ?? false }
        if !digits.isEmpty {
            guard let id = Int(digits) else {
                throw HeyError.api(
                    message: "contact id in \"\(contactUrl)\" is not a number: \(digits) is out of range", httpStatus: nil,
                    retryable: false, detail: ErrorDetail())
            }
            return id
        }
        searchFrom = contactUrl.index(after: found.lowerBound)
    }
    return nil
}
