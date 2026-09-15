package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.models.MoveStickyRequestContent
import com.basecamp.hey.generated.models.Sticky
import com.basecamp.hey.generated.models.StickyPayload
import com.basecamp.hey.generated.models.StickyRequestContent
import com.basecamp.hey.generated.services.ListStickiesOptions
import com.basecamp.hey.generated.services.StickiesService as GeneratedStickiesService

/** The largest page the stickies index answers with. The server clamps anything above it, so [StickiesService.listUpTo] clamps too rather than sending a number it knows is ignored. */
const val MAX_STICKIES_LIMIT: Int = 100

/** The highest board position [StickiesService.moveTo] accepts. The wire format carries the position as a 32-bit integer. */
const val MAX_STICKY_POSITION: Long = Int.MAX_VALUE.toLong()

/** How much room a sticky takes on the board. */
enum class StickySize(
    /** The size as HEY's `size` parameter names it. */
    val wire: String,
) {
    /** The smallest. */
    SMALL("small"),

    /** The middle size. */
    MEDIUM("medium"),

    /** The largest. */
    LARGE("large"),
    ;

    companion object {
        /** Reads a size as HEY writes it. */
        fun parse(source: String): StickySize =
            entries.firstOrNull { it.wire == source }
                ?: throw HeyException.Usage("sticky size \"$source\" is none of \"small\", \"medium\" or \"large\"")
    }
}

/** The stickies board, in the terms the board itself is kept in — a size, a limit and a position — on top of the generated surface (`list`, `create`, `update`, `delete`, `moveSticky`). */
class StickiesService(client: HeyClient) : GeneratedStickiesService(client) {
    /**
     * The stickies in board order, at most [limit] of them. Zero asks for the server default,
     * which is also its maximum of [MAX_STICKIES_LIMIT]; a limit of zero is left off the query
     * entirely, since `limit=0` is clamped to a single sticky rather than read as "no limit".
     */
    suspend fun listUpTo(limit: Int): List<Sticky> {
        if (limit < 0) throw HeyException.Usage("sticky limit must be at least 0, got $limit")
        return list(ListStickiesOptions(limit = limit.takeIf { it > 0 }?.coerceAtMost(MAX_STICKIES_LIMIT)))
    }

    /** Writes a new sticky. No size leaves the server default in place. */
    suspend fun createSticky(body: String, size: StickySize? = null): Sticky = create(stickyBody(body, size))

    /** Edits a sticky. An empty body and no size are left alone. */
    suspend fun updateSticky(stickyId: Long, body: String, size: StickySize? = null): Sticky = update(stickyId, stickyBody(body, size))

    /** Repositions a sticky on the board. Positions run from zero to [MAX_STICKY_POSITION]. */
    suspend fun moveTo(stickyId: Long, position: Long) {
        if (position < 0 || position > MAX_STICKY_POSITION) {
            throw HeyException.Usage("sticky position must be between 0 and $MAX_STICKY_POSITION, got $position")
        }
        moveSticky(MoveStickyRequestContent(id = stickyId, position = position.toInt()))
    }

    private fun stickyBody(body: String, size: StickySize?): StickyRequestContent =
        StickyRequestContent(StickyPayload(body = body.ifEmpty { null }, size = size?.wire))
}
