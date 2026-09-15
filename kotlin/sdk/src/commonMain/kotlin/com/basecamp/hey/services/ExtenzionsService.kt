package com.basecamp.hey.services

import com.basecamp.hey.FormResponse
import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.Response
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.NavigationItem
import com.basecamp.hey.generated.models.NavigationResponse
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.models.Extenzion as ExtenzionPayload
import com.basecamp.hey.generated.services.ExtenzionsService as GeneratedExtenzionsService

/** The title HEY gives the extensions group in the navigation payload. */
private const val NAVIGATION_GROUP = "Extensions"

/** What a contact URL puts the contact's id after, as in `/contacts/4821`. */
private const val CONTACT_PATH = "/contacts/"

/**
 * One email extenzion. The id is the extenzion's *contact* id — the one every write endpoint
 * takes, and the one its `appUrl` carries. The id a JSON write answers with belongs to the
 * Extenzion record instead, which no endpoint takes.
 */
data class Extenzion(
    /** The extenzion's contact id. */
    val id: Long,
    /** The part before the `@`, as in `sales`. */
    val name: String,
    /** The contact's page in HEY's web app. */
    val appUrl: String,
)

/** A new extenzion. */
data class CreateExtenzionParams(
    /** The extenzion name: "sales" becomes `sales@yourdomain.com`. */
    val name: String,
    /** The member email addresses. */
    val members: List<String> = emptyList(),
)

/** A partial revision: null leaves a field as it is. */
data class UpdateExtenzionParams(
    /** An empty name is no name, and is left off the wire like a null one. */
    val name: String? = null,
    /** The whole membership, which replaces what the extenzion had rather than adding to it. */
    val members: List<String>? = null,
)

/**
 * Extenzions service — the custom addresses a custom-domain account carries, such as
 * `sales@yourdomain.com` — on top of the generated surface (`delete`). [create] and [update]
 * post a form to the `.json` path, so a current server answers the written extenzion while
 * one without the JSON branch redirects and hands nothing back.
 */
class ExtenzionsService(client: HeyClient) : GeneratedExtenzionsService(client) {
    /**
     * The extenzions on the account. This reads the navigation payload rather than scraping
     * the extenzions page, so it carries only what navigation carries: each extenzion's name
     * and its contact URL. It is one operation, `Extenzions.ListExtenzions`, rather than the
     * identity read it happens to be built on, which is why it sends the navigation route
     * itself instead of going through `IdentityService.getNavigation`.
     */
    suspend fun list(): List<Extenzion> {
        val operation = client.operation(Routes.GET_NAVIGATION, emptyList())
        operation.info(OperationInfo(service = "Extenzions", operation = "ListExtenzions", resourceType = "extenzion", isMutation = false))
        return client.execute(operation) { response -> extenzionsFromNavigation(response.json(NavigationResponse.serializer())) }
    }

    /** Creates an extenzion and answers it. A server without the JSON create branch hands nothing back, and the answer is then null. */
    suspend fun create(accountId: Long, params: CreateExtenzionParams): Extenzion? {
        val fields = mutableListOf("extenzion[name]" to params.name)
        for (member in params.members) fields += "extenzion[members][]" to member
        val operation = client.form(Method.POST, "/accounts/$accountId/domains/extenzions.json")
        operation.info(writeInfo("Extenzions", "CreateExtenzion", "extenzion"))
        operation.form(fields)
        return client.execute(operation, ::extenzionFromFormResponse)
    }

    /**
     * Revises an extenzion and answers it. The id is the extenzion's contact id. A server
     * without the JSON update branch hands nothing back, and the answer is then null.
     */
    suspend fun update(accountId: Long, extenzionId: Long, params: UpdateExtenzionParams): Extenzion? {
        val fields = mutableListOf<Pair<String, String>>()
        params.name?.takeIf { it.isNotEmpty() }?.let { fields += "extenzion[name]" to it }
        params.members?.forEach { fields += "extenzion[members][]" to it }
        val operation = client.form(Method.PATCH, "/accounts/$accountId/domains/extenzions/$extenzionId.json")
        operation.info(writeInfo("Extenzions", "UpdateExtenzion", "extenzion", extenzionId))
        operation.form(fields)
        return client.execute(operation, ::extenzionFromFormResponse)
    }
}

/** The extenzions in navigation's "Extensions" group. The group leads with the "All Extensions" link, which names no contact and so falls out on its own. */
internal fun extenzionsFromNavigation(navigation: NavigationResponse): List<Extenzion> =
    navigation.items.orEmpty()
        .filter { it.title == NAVIGATION_GROUP }
        .flatMap { group -> group.menuItems.orEmpty() }
        .mapNotNull(::listedExtenzion)

private fun listedExtenzion(entry: NavigationItem): Extenzion? {
    val appUrl = entry.appUrl.orEmpty()
    val id = contactIdFromUrl(appUrl) ?: return null
    return Extenzion(id = id, name = entry.title.orEmpty(), appUrl = appUrl)
}

/** The extenzion a JSON write answered with. A server without the JSON branch redirects to the extenzions page instead, which leaves nothing to read. */
private fun extenzionFromFormResponse(answered: Response): Extenzion? {
    if (FormResponse.of(answered).body.isEmpty()) return null
    val payload = answered.json(ExtenzionPayload.serializer())
    val appUrl = payload.appUrl.orEmpty()
    val id = contactIdFromUrl(appUrl) ?: throw HeyException.Api("no contact id in \"$appUrl\"", httpStatus = answered.status, retryable = false)
    return Extenzion(id = id, name = payload.name.orEmpty(), appUrl = appUrl)
}

/**
 * The contact id a contact URL carries, as Go's `/contacts/(\d+)` reads it. A URL naming no
 * contact at all answers null — navigation's "All Extensions" link is one, and it is meant to
 * fall out of the list. A URL that names one the SDK cannot read is a failure instead of
 * another such link: dropping it would hide an extenzion the caller would then never hear
 * about.
 */
internal fun contactIdFromUrl(contactUrl: String): Long? {
    var at = contactUrl.indexOf(CONTACT_PATH)
    while (at >= 0) {
        val digits = contactUrl.substring(at + CONTACT_PATH.length).takeWhile { it in '0'..'9' }
        if (digits.isNotEmpty()) {
            return digits.toLongOrNull()
                ?: throw HeyException.Api("contact id in \"$contactUrl\" is not a number: $digits is out of range", httpStatus = null, retryable = false)
        }
        at = contactUrl.indexOf(CONTACT_PATH, at + 1)
    }
    return null
}
