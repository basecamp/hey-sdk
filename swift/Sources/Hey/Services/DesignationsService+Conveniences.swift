/// Screening a contact into a box, so everything they send lands there, on top of the generated
/// surface (`create`, `delete`). HEY models a designation under the box that holds it, so these
/// are `/boxes/{id}` paths; every SDK files them under the name the feature goes by, which is this
/// service.
extension DesignationsService {
    /// Designates a contact to a box. The generated ``create(boxId:body:)`` takes the same request
    /// as a body. HEY designates the contact's primary, so a contact's aliases fold into the one
    /// designation — whose id cannot be worked out from `contactId`. Read the box back if you need
    /// it.
    public func createBoxDesignation(boxId: Int, contactId: Int) async throws {
        try await create(boxId: boxId, body: CreateBoxDesignationRequestContent(contactId: contactId))
    }
}
