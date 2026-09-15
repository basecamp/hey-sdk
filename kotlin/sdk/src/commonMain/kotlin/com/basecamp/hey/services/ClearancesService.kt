package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Page
import com.basecamp.hey.generated.models.BulkUpdateClearancesRequestContent
import com.basecamp.hey.generated.models.Clearance
import com.basecamp.hey.generated.models.ClearanceListResponse
import com.basecamp.hey.generated.models.ClearanceSummary
import com.basecamp.hey.generated.models.UpdateClearanceRequestContent
import com.basecamp.hey.generated.models.UpdateMyClearanceRequestContent
import com.basecamp.hey.generated.services.GetClearancesOptions
import com.basecamp.hey.generated.services.GetMyClearancesOptions
import com.basecamp.hey.generated.services.ClearancesService as GeneratedClearancesService

/** The two decisions the Screener takes. */
enum class ClearanceStatus(
    /** The decision as HEY's `status` parameter names it. */
    val wire: String,
) {
    /** Screened in: the sender's mail arrives. */
    APPROVED("approved"),

    /** Screened out: the sender's mail is kept away. */
    DENIED("denied"),
    ;

    companion object {
        /** Reads a decision as HEY writes it. */
        fun parse(source: String): ClearanceStatus =
            entries.firstOrNull { it.wire == source }
                ?: throw HeyException.Validation("clearance status must be \"approved\" or \"denied\", got \"$source\"")
    }
}

/**
 * What to do beyond setting the status. HEY reads each of these for truthiness, so one left
 * alone stays off the wire entirely rather than going out as a false.
 */
data class ScreenOptions(
    /** Files everything the sender sends into that box rather than the Imbox. */
    val designationBoxId: Long? = null,
    /** Marks the topics already waiting as spam and trains the filter on them. */
    val spam: Boolean = false,
    /** Screens the sender in without their waiting mail arriving unread. */
    val markTopicsAsSeen: Boolean = false,
)

/**
 * The Screener: who is waiting to be let in, and letting them in or turning them away, on top
 * of the generated surface (`get`, `getMy`, `update`, `updateMy`, `bulkUpdate`, `punt`).
 * Clearing the Screener is the generated [punt]. The work it starts is queued, so everyone
 * waiting is still pending when it answers; they are dropped and reexamined the next time
 * they write, so nothing is decided for them.
 */
class ClearancesService(client: HeyClient) : GeneratedClearancesService(client) {
    /**
     * How many senders are waiting, without fetching them. This is the cheap read HEY's own
     * apps sync for the Screener badge; [pending] is the senders themselves.
     */
    suspend fun pendingCount(): Int = summary().pendingClearancesCount ?: 0

    /**
     * Everything HEY says about the Screener without the queue itself: how many senders are
     * waiting, and the signed stream name to subscribe to on HEY's cable server to be told
     * when that changes.
     */
    suspend fun summary(): ClearanceSummary = get().value

    /**
     * The senders waiting to be screened, a page at a time. Each one carries the petitioner
     * and the most recent entry they sent, so a caller can show who is asking and what they
     * wrote without a second read.
     */
    suspend fun pending(page: String? = null): ClearanceSummary = pendingPage(page).value

    /** The same queue as [pending], keeping the cursor for the page after it so a caller walking the queue is told when it has reached the end. */
    suspend fun pendingPage(page: String? = null): Page<ClearanceSummary> =
        get(GetClearancesOptions(includeClearances = true, page = page))

    /** Answers the Screener for one sender. */
    suspend fun screen(clearanceId: Long, status: ClearanceStatus, options: ScreenOptions = ScreenOptions()): Clearance =
        update(
            clearanceId,
            UpdateClearanceRequestContent(
                status = status.wire,
                designationBoxId = options.designationBoxId,
                spam = flag(options.spam),
                markTopicsAsSeen = flag(options.markTopicsAsSeen),
            ),
        )

    /**
     * Screens several senders at once and answers the clearances it changed. HEY answers 404
     * when none of the ids belong to the caller. A partial match succeeds and answers only
     * what it touched, so compare the answer against what was sent.
     */
    suspend fun screenMany(clearanceIds: List<Long>, status: ClearanceStatus, spam: Boolean = false): List<Clearance> {
        if (clearanceIds.isEmpty()) throw HeyException.Usage("at least one clearance is required")
        val body = BulkUpdateClearancesRequestContent(ids = clearanceIds.joinToString(","), status = status.wire, spam = flag(spam))
        return bulkUpdate(body).clearances.orEmpty()
    }

    /** The senders already screened in or out, newest decision first, a page at a time. */
    suspend fun screened(page: String? = null): List<Clearance> = screenedPage(page).value.clearances.orEmpty()

    /** The same decisions as [screened], keeping the cursor for the page after it. */
    suspend fun screenedPage(page: String? = null): Page<ClearanceListResponse> = getMy(GetMyClearancesOptions(page = page))

    /** Changes its mind about a sender already screened in or out. This is the decided list, not the queue: [screen] is what answers a pending sender. */
    suspend fun rescreen(clearanceId: Long, status: ClearanceStatus): Clearance =
        updateMy(clearanceId, UpdateMyClearanceRequestContent(status = status.wire))

    /** A flag HEY reads for truthiness goes out only when it is on. */
    private fun flag(value: Boolean): Boolean? = if (value) true else null
}
