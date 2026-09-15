package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.generated.models.CreateBoxDesignationRequestContent
import com.basecamp.hey.generated.services.DesignationsService as GeneratedDesignationsService

/**
 * Screening a contact into a box, so everything they send lands there, on top of the
 * generated surface (`create`, `delete`). HEY models a designation under the box that holds
 * it, so these are `/boxes/{id}` paths; every SDK files them under the name the feature goes
 * by, which is this service.
 */
class DesignationsService(client: HeyClient) : GeneratedDesignationsService(client) {
    /**
     * Designates a contact to a box. The generated [create] takes the same request as a body.
     * HEY designates the contact's primary, so a contact's aliases fold into the one
     * designation — whose id cannot be worked out from [contactId]. Read the box back if you
     * need it.
     */
    suspend fun createBoxDesignation(boxId: Long, contactId: Long) = create(boxId, CreateBoxDesignationRequestContent(contactId = contactId))
}
