import Foundation

/// A contact, as its writes take it.
public struct ContactParams: Sendable, Equatable {
    /// What the contact is called.
    public var name: String
    /// The contact's main address, the one HEY files them under.
    public var emailAddress: String
    /// The other addresses that belong to the same person. Sending the list replaces it, so an
    /// address left out stops being an alias; nil leaves the current aliases alone.
    public var aliasEmailAddresses: [String]?
    /// The account to file the contact under, on a create. One identity can hold several accounts,
    /// each with its own contacts; this is the identity's user on the one meant, which the
    /// identity's `all_users` carries alongside its `account_id`. Left unset, HEY files the contact
    /// under the first account. An update ignores it.
    public var accountUserId: Int?

    /// A contact from its parts.
    public init(name: String = "", emailAddress: String = "", aliasEmailAddresses: [String]? = nil, accountUserId: Int? = nil) {
        self.name = name
        self.emailAddress = emailAddress
        self.aliasEmailAddresses = aliasEmailAddresses
        self.accountUserId = accountUserId
    }
}

/// The contacts a refused write clashed with: HEY's web sends you to a merge form at this point,
/// and these are the contacts it would have offered to merge with. A create that clashes still
/// creates the contact — the merge happens afterwards — so ``contactId`` is the contact that was
/// written, not one that failed to be.
///
/// It is read out of the ``HeyError/conflict(message:detail:)`` a contact write throws with
/// ``fromError(_:)``, so a caller who only cares that the write was refused can ignore it.
public struct ContactConflict: Sendable, Equatable, CustomStringConvertible {
    /// The contact the write was for — on a create, the one it made.
    public var contactId: Int
    /// The contacts already holding one of the addresses.
    public var conflictingContactIds: [Int]

    /// A conflict from its parts.
    public init(contactId: Int, conflictingContactIds: [Int] = []) {
        self.contactId = contactId
        self.conflictingContactIds = conflictingContactIds
    }

    /// The conflict in words: the contact written, and the contacts it clashed with.
    public var description: String {
        conflictingContactIds.isEmpty
            ? "contact \(contactId) conflicts with one that already exists"
            : "contact \(contactId) conflicts with \(conflictingContactIds.map(String.init).joined(separator: ", "))"
    }

    /// The conflict a refused contact write carries, when the error is one: a 409 whose body names
    /// the contact that was written. Any other error, a 409 from elsewhere included, carries none.
    public static func fromError(_ error: HeyError) -> ContactConflict? {
        guard error.httpStatus == 409,
              let payload = decodeContactFailure(ConflictErrorResponseContent.self, error),
              let contactId = payload.contactId
        else { return nil }
        return ContactConflict(contactId: contactId, conflictingContactIds: payload.conflictingContactIds ?? [])
    }
}

/// The writes on top of the generated surface (`list`, `get`, `hide`, `reveal`, `bundle`,
/// `getNote`, ...), and the two refusals a contact write answers with named: an address that
/// belongs to someone else, and a contact the model itself rejected.
extension ContactsService {
    /// Adds a contact and answers it. On a client scoped to an account the contact is filed under
    /// that account, and a ``ContactParams/accountUserId`` naming another one is refused rather
    /// than quietly overruled.
    ///
    /// - Throws: ``HeyError/conflict(message:detail:)`` in HEY's words when an address belongs to
    ///   another contact (see ``ContactConflict``), ``HeyError/validation(message:httpStatus:detail:)``
    ///   in the model's when it rejects the contact, and ``HeyError/usage(message:hint:)`` for an
    ///   account user of another account.
    public func createContact(_ params: ContactParams) async throws -> Contact {
        let body = CreateContactRequestContent(contact: contactPayload(params), actingUserId: try await actingUserId(params.accountUserId))
        var operation = try client.operation(Routes.createContact, [])
        try operation.json(body)
        return try await write(operation, as: CreateContactResponseContent.self)
    }

    /// Edits a contact and answers it. Fields left empty are kept, as are the aliases when
    /// ``ContactParams/aliasEmailAddresses`` is nil.
    ///
    /// HEY's update is a full replacement — it rewrites the name and address and removes any alias
    /// not submitted — so the contact is read first and the unset fields are filled in from it
    /// before the write. That read-then-write is not atomic: a change made to the contact in
    /// between is overwritten with what was read. Pass every field explicitly when that matters.
    ///
    /// The contact that comes back is not always the one addressed: giving a contact one of its own
    /// aliases as the main address promotes the alias, and the alias is what is answered.
    public func updateContact(contactId: Int, params: ContactParams) async throws -> Contact {
        let current = try await get(contactId: contactId).value
        var operation = try client.operation(Routes.updateContact, [contactId])
        operation.resourceId(contactId)
        try operation.json(ContactRequestContent(contact: mergedPayload(params, current)))
        return try await write(operation, as: UpdateContactResponseContent.self)
    }

    /// Answers the Screener for a contact.
    public func screen(contactId: Int, status: ClearanceStatus) async throws {
        try await updateClearance(contactId: contactId, body: UpdateContactClearanceRequestContent(status: status.wire))
    }

    /// Writes the private note kept on a contact, replacing whatever was there, and answers the
    /// note as it now reads.
    public func setNote(contactId: Int, note: String) async throws -> ContactNote {
        var operation = try client.operation(Routes.updateContactNote, [contactId])
        operation.resourceId(contactId)
        try operation.json(ContactNoteRequestContent(contact: ContactNotePayload(note: note)))
        return try await write(operation, as: UpdateContactNoteResponseContent.self)
    }

    private func actingUserId(_ chosen: Int?) async throws -> Int? {
        guard let accountId = client.accountId else { return chosen }
        let accountUserId = try await client.accountUserId()
        if let chosen, chosen != accountUserId {
            throw HeyError.usage(message: "account user \(chosen) does not belong to selected account \(accountId)")
        }
        return accountUserId
    }

    /// Sends a contact write, rewording the two refusals it can answer with in HEY's own words: a
    /// 409 says which addresses clash, a 422 what the model rejected. Both are still failures, and
    /// the hooks are told so, with the reworded error the caller gets; the body stays on it for
    /// ``ContactConflict/fromError(_:)`` to read.
    private func write<T: Decodable & Sendable>(_ operation: HeyOperation, as type: T.Type) async throws -> T {
        var request = operation
        request.quiet()
        let client = self.client
        let quiet = request
        return try await client.asOperation(operation.info) {
            do {
                return try await client.send(quiet, as: type)
            } catch let HeyError.conflict(message, detail) {
                throw HeyError.conflict(message: contactConflictMessage(message, detail), detail: detail)
            } catch let HeyError.validation(message, status, detail) {
                throw HeyError.validation(message: detail.hint ?? message, httpStatus: status, detail: detail)
            }
        }
    }

    private func contactPayload(_ params: ContactParams) -> ContactPayload {
        ContactPayload(
            name: params.name,
            emailAddress: params.emailAddress.isEmpty ? nil : SensitiveString(params.emailAddress),
            aliasEmailAddresses: params.aliasEmailAddresses)
    }

    /// The write HEY is sent: the caller's fields, with the contact's own filled in wherever the
    /// caller named none. The alias list always goes out, filled in from the contact when the
    /// caller left it unset, so an explicit empty list clears the aliases — the one thing HEY's own
    /// full-replacement update is for.
    private func mergedPayload(_ params: ContactParams, _ current: ContactDetail) -> ContactPayload {
        var payload = contactPayload(params)
        if payload.name.isEmpty { payload.name = current.name ?? "" }
        if payload.emailAddress == nil { payload.emailAddress = current.emailAddress }
        if payload.aliasEmailAddresses == nil {
            payload.aliasEmailAddresses = (current.aliases ?? []).compactMap { $0.emailAddress?.expose() }
        }
        return payload
    }
}

/// The server's own words out of a 409. Contact writes answer the `errors` list the other
/// refusals use; elsewhere a 409 is a single message. A body neither of those still has to read as
/// something.
private func contactConflictMessage(_ message: String, _ detail: ErrorDetail) -> String {
    let payload = decodeContactFailure(ConflictErrorResponseContent.self, .conflict(message: message, detail: detail))
    if let messages = payload?.errors, !messages.isEmpty { return messages.joined(separator: "; ") }
    if let single = payload?.error, !single.isEmpty { return single }
    return "the contact conflicts with one that already exists"
}

/// The failure body an error kept, read as `T`, or nil when there is none or it is not that shape.
private func decodeContactFailure<T: Decodable>(_ type: T.Type, _ error: HeyError) -> T? {
    guard let body = error.body, !body.isEmpty else { return nil }
    return try? JSONDecoder().decode(type, from: body)
}
