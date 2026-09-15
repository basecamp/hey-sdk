package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Operation
import com.basecamp.hey.SensitiveString
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.ConflictErrorResponseContent
import com.basecamp.hey.generated.models.Contact
import com.basecamp.hey.generated.models.ContactDetail
import com.basecamp.hey.generated.models.ContactNote
import com.basecamp.hey.generated.models.ContactNotePayload
import com.basecamp.hey.generated.models.ContactNoteRequestContent
import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.ContactRequestContent
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.generated.models.UpdateContactClearanceRequestContent
import com.basecamp.hey.heyJson
import com.basecamp.hey.json
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.serializer
import com.basecamp.hey.generated.services.ContactsService as GeneratedContactsService

/** A contact, as its writes take it. */
data class ContactParams(
    /** What the contact is called. */
    val name: String = "",
    /** The contact's main address, the one HEY files them under. */
    val emailAddress: String = "",
    /**
     * The other addresses that belong to the same person. Sending the list replaces it, so an
     * address left out stops being an alias; null leaves the current aliases alone.
     */
    val aliasEmailAddresses: List<String>? = null,
    /**
     * The account to file the contact under, on a create. One identity can hold several
     * accounts, each with its own contacts; this is the identity's user on the one meant,
     * which the identity's `all_users` carries alongside its `account_id`. Left unset, HEY
     * files the contact under the first account. An update ignores it.
     */
    val accountUserId: Long? = null,
)

/**
 * The contacts a refused write clashed with: HEY's web sends you to a merge form at this
 * point, and these are the contacts it would have offered to merge with. A create that
 * clashes still creates the contact — the merge happens afterwards — so [contactId] is the
 * contact that was written, not one that failed to be.
 *
 * It is read out of the [HeyException.Conflict] a contact write throws with [fromError], so
 * a caller who only cares that the write was refused can ignore it.
 */
data class ContactConflict(
    /** The contact the write was for — on a create, the one it made. */
    val contactId: Long,
    /** The contacts already holding one of the addresses. */
    val conflictingContactIds: List<Long> = emptyList(),
) {
    override fun toString(): String =
        if (conflictingContactIds.isEmpty()) {
            "contact $contactId conflicts with one that already exists"
        } else {
            "contact $contactId conflicts with ${conflictingContactIds.joinToString(", ")}"
        }

    companion object {
        /**
         * The conflict a refused contact write carries, when the error is one: a 409 whose body
         * names the contact that was written. Any other error, a 409 from elsewhere included,
         * carries none.
         */
        fun fromError(error: HeyException): ContactConflict? {
            if (error.httpStatus != 409) return null
            val payload = decodeBody<ConflictErrorResponseContent>(error) ?: return null
            val contactId = payload.contactId ?: return null
            return ContactConflict(contactId, payload.conflictingContactIds.orEmpty())
        }
    }
}

/**
 * Contacts service with the writes on top of the generated surface (`list`, `get`, `hide`,
 * `reveal`, `bundle`, `getNote`, ...), and the two refusals a contact write answers with
 * named: an address that belongs to someone else, and a contact the model itself rejected.
 */
class ContactsService(client: HeyClient) : GeneratedContactsService(client) {
    /**
     * Adds a contact and answers it. On a client scoped to an account the contact is filed
     * under that account, and an [ContactParams.accountUserId] naming another one is refused
     * rather than quietly overruled.
     */
    suspend fun createContact(params: ContactParams): Contact {
        val body = CreateContactRequestContent(contact = contactPayload(params), actingUserId = actingUserId(params.accountUserId))
        val operation = client.operation(Routes.CREATE_CONTACT, emptyList())
        operation.json(body)
        return write(operation, serializer())
    }

    /**
     * Edits a contact and answers it. Fields left empty are kept, as are the aliases when
     * [ContactParams.aliasEmailAddresses] is null.
     *
     * HEY's update is a full replacement — it rewrites the name and address and removes any
     * alias not submitted — so the contact is read first and the unset fields are filled in
     * from it before the write. That read-then-write is not atomic: a change made to the
     * contact in between is overwritten with what was read. Pass every field explicitly when
     * that matters.
     *
     * The contact that comes back is not always the one addressed: giving a contact one of
     * its own aliases as the main address promotes the alias, and the alias is what is
     * answered.
     */
    suspend fun updateContact(contactId: Long, params: ContactParams): Contact {
        val current = get(contactId).value
        val operation = client.operation(Routes.UPDATE_CONTACT, listOf(contactId))
        operation.resourceId(contactId)
        operation.json(ContactRequestContent(contact = mergedPayload(params, current)))
        return write(operation, serializer())
    }

    /** Answers the Screener for a contact. */
    suspend fun screen(contactId: Long, status: ClearanceStatus) =
        updateClearance(contactId, UpdateContactClearanceRequestContent(status = status.wire))

    /** Writes the private note kept on a contact, replacing whatever was there, and answers the note as it now reads. */
    suspend fun setNote(contactId: Long, note: String): ContactNote {
        val operation = client.operation(Routes.UPDATE_CONTACT_NOTE, listOf(contactId))
        operation.resourceId(contactId)
        operation.json(ContactNoteRequestContent(contact = ContactNotePayload(note = note)))
        return write(operation, serializer())
    }

    private suspend fun actingUserId(chosen: Long?): Long? {
        val accountId = client.accountId ?: return chosen
        val accountUserId = client.accountUserId()
        if (chosen != null && chosen != accountUserId) {
            throw HeyException.Usage("account user $chosen does not belong to selected account $accountId")
        }
        return accountUserId
    }

    /**
     * Sends a contact write, rewording the two refusals it can answer with in HEY's own
     * words: a 409 says which addresses clash, a 422 what the model rejected. Both are still
     * failures, and the hooks are told so, with the reworded error the caller gets; the body
     * stays on it for [ContactConflict.fromError] to read.
     */
    private suspend fun <T> write(operation: Operation, deserializer: DeserializationStrategy<T>): T {
        operation.quiet()
        return client.asOperation(operation.info) {
            try {
                client.send(operation, deserializer)
            } catch (error: HeyException.Conflict) {
                throw HeyException.Conflict(conflictMessage(error), hint = error.hint, requestId = error.requestId, body = error.body)
            } catch (error: HeyException.Validation) {
                throw HeyException.Validation(rejectionMessage(error), hint = error.hint, requestId = error.requestId, body = error.body)
            }
        }
    }
}

private fun contactPayload(params: ContactParams): ContactPayload =
    ContactPayload(
        name = params.name,
        emailAddress = params.emailAddress.takeIf { it.isNotEmpty() }?.let { SensitiveString(it) },
        aliasEmailAddresses = params.aliasEmailAddresses,
    )

/**
 * The write HEY is sent: the caller's fields, with the contact's own filled in wherever the
 * caller named none. The alias list always goes out, filled in from the contact when the
 * caller left it unset, so an explicit empty list clears the aliases — the one thing HEY's
 * own full-replacement update is for.
 */
private fun mergedPayload(params: ContactParams, current: ContactDetail): ContactPayload {
    val payload = contactPayload(params)
    return payload.copy(
        name = payload.name.ifEmpty { current.name.orEmpty() },
        emailAddress = payload.emailAddress ?: current.emailAddress,
        aliasEmailAddresses = payload.aliasEmailAddresses ?: current.aliases.orEmpty().mapNotNull { it.emailAddress?.expose() },
    )
}

/**
 * The server's own words out of a 409. Contact writes answer the `errors` list the other
 * refusals use; elsewhere a 409 is a single message. A body neither of those still has to
 * read as something.
 */
private fun conflictMessage(error: HeyException): String {
    val payload = decodeBody<ConflictErrorResponseContent>(error)
    val messages = payload?.errors.orEmpty()
    val single = payload?.error
    return when {
        messages.isNotEmpty() -> messages.joinToString("; ")
        !single.isNullOrEmpty() -> single
        else -> "the contact conflicts with one that already exists"
    }
}

/** What the model said about a 422, as the SDK already read it into the hint; the code's own message otherwise. */
private fun rejectionMessage(error: HeyException): String = error.hint ?: error.message ?: "validation error"

/** The failure body an error kept, read as [T], or null when there is none or it is not that shape. */
private inline fun <reified T> decodeBody(error: HeyException): T? {
    val body = error.body?.takeIf { it.isNotEmpty() } ?: return null
    return runCatching { heyJson.decodeFromString(serializer<T>(), body.decodeToString()) }.getOrNull()
}
