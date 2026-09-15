package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.models.CreateBoxGroupRequestContent
import com.basecamp.hey.generated.models.CreateBoxGroupResponseContent
import kotlinx.coroutines.sync.withLock
import com.basecamp.hey.generated.services.BoxesService as GeneratedBoxesService

/** The kinds of box a HEY account has, as `BoxesService.list` reports them. */
enum class BoxKind(
    /** The kind as the box index names it — the `kind` a listed box carries. */
    val wire: String,
) {
    /** The Imbox, where screened-in mail lands. */
    IMBOX("imbox"),

    /** The Feed, for newsletters and the like. */
    FEED("feedbox"),

    /** Set Aside, for threads kept close to hand. */
    SET_ASIDE("asidebox"),

    /** Reply Later, for threads waiting on an answer. */
    REPLY_LATER("laterbox"),

    /** The Paper Trail, for receipts and confirmations. */
    PAPER_TRAIL("trailbox"),

    /** Bubble Up, holding postings until the day they resurface. */
    BUBBLE_UP("bubblebox"),
    ;

    companion object {
        /** The kind a box index names. */
        fun parse(source: String): BoxKind =
            entries.firstOrNull { it.wire == source }
                ?: throw HeyException.Usage("box kind \"$source\" is none of imbox, feedbox, asidebox, laterbox, trailbox, bubblebox")
    }
}

/** Boxes service with kind resolution on top of the generated surface (`list`, `get`, `getImbox`, ...). */
class BoxesService(client: HeyClient) : GeneratedBoxesService(client) {
    /**
     * The id of the box of a kind. The client reads the box index once and answers every kind
     * from that reading for as long as it lives; a client derived with [HeyClient.forAccount]
     * reads it again for the account it presents. Use [kinds] to read the index afresh.
     */
    suspend fun idByKind(kind: BoxKind): Long = client.scope.lock.withLock {
        val kinds = client.scope.boxKinds ?: kinds().also { client.scope.boxKinds = it }
        kinds[kind.wire] ?: throw HeyException.Api("no box of kind \"${kind.wire}\"", httpStatus = null, retryable = false)
    }

    /** Reads the box index and maps every box's kind to its id. This is the read itself, so it goes to HEY every time. */
    suspend fun kinds(): Map<String, Long> =
        list().value.filter { it.kind.isNotEmpty() }.associate { it.kind to it.id }

    /** Gathers a selection of postings into a new Set Aside group. The generated [createGroup] takes the same request as a body. */
    suspend fun createBoxGroup(boxId: Long, postingIds: List<Long>): CreateBoxGroupResponseContent =
        createGroup(boxId, CreateBoxGroupRequestContent(postingIds = postingIds))
}
