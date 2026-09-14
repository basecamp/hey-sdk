package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.models.UpdateTimeTrackPayload
import com.basecamp.hey.generated.models.UpdateTimeTrackRequestContent
import com.basecamp.hey.json
import com.basecamp.hey.nowIso8601
import com.basecamp.hey.generated.services.TimeTracksService as GeneratedTimeTracksService

/** Time tracks service with start and stop conveniences on top of the generated surface (`start`, `update`, `getOngoing`, ...). */
class TimeTracksService(client: HeyClient) : GeneratedTimeTracksService(client) {
    /**
     * Starts a time track: [start] with the one refusal it can meet named. A track already
     * running answers 409, which arrives as [HeyException.Conflict] carrying HEY's own
     * message, so a caller can branch on it.
     */
    suspend fun startTracking(): Recording =
        try {
            start()
        } catch (error: HeyException) {
            if (error.httpStatus == 409) {
                throw HeyException.Conflict(error.hint ?: "a time track is already running", requestId = error.requestId, body = error.body)
            }
            throw error
        }

    /** Stops the running time track by setting its end to now. */
    suspend fun stop(timeTrackId: Long) {
        stopAndFile(timeTrackId, null)
    }

    /**
     * Stops a time track and files it under a category in the one request, creating the
     * category if HEY has none by that name. It sends the same PUT [update] does, but
     * announces itself to the hooks as `StopTimeTrack`.
     */
    suspend fun stopAndFile(timeTrackId: Long, categoryTitle: String?) {
        val body = UpdateTimeTrackRequestContent(
            calendarTimeTrack = UpdateTimeTrackPayload(
                endsAt = nowIso8601(),
                categoryTitle = categoryTitle?.takeIf { it.isNotEmpty() },
            ),
        )
        val operation = client.operation(Routes.UPDATE_TIME_TRACK, listOf(timeTrackId))
        operation.operationName("StopTimeTrack")
        operation.resourceId(timeTrackId)
        operation.json(body)
        client.sendUnit(operation)
    }
}
