package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.Recording
import com.basecamp.hey.generated.models.UpdateTimeTrackPayload
import com.basecamp.hey.generated.models.UpdateTimeTrackRequestContent
import com.basecamp.hey.json
import com.basecamp.hey.nowIso8601
import com.basecamp.hey.writeInfo
import com.basecamp.hey.generated.services.TimeTracksService as GeneratedTimeTracksService

/**
 * Time tracks service with start and stop conveniences, the categories tracks are filed
 * under and the export on top of the generated surface (`start`, `update`, `getOngoing`,
 * ...). HEY serves no JSON endpoint for a category write or for the export, so those are
 * browser form posts and a file read.
 */
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

    /** Adds a category to file tracks under. */
    suspend fun createCategory(title: String) {
        val operation = client.form(Method.POST, "/calendar/time_tracks/categories")
        operation.info(writeInfo("TimeTracks", "CreateTimeTrackCategory", "category"))
        operation.form(listOf("category[title]" to title))
        client.sendUnit(operation)
    }

    /** Renames a category. */
    suspend fun updateCategory(categoryId: Long, title: String) {
        val operation = client.form(Method.PATCH, "/calendar/time_tracks/categories/$categoryId")
        operation.info(writeInfo("TimeTracks", "UpdateTimeTrackCategory", "category", categoryId))
        operation.form(listOf("category[title]" to title))
        client.sendUnit(operation)
    }

    /** Removes a category. The tracks filed under it stay, uncategorized. */
    suspend fun deleteCategory(categoryId: Long) {
        val operation = client.form(Method.DELETE, "/calendar/time_tracks/categories/$categoryId")
        operation.info(writeInfo("TimeTracks", "DeleteTimeTrackCategory", "category", categoryId))
        client.sendUnit(operation)
    }

    /**
     * Every completed time track as CSV, newest first, under the columns Start, End,
     * Duration, Category and Notes. HEY streams this as a file rather than a document, from
     * the bare path.
     */
    suspend fun export(): ByteArray {
        val operation = client.request(Method.GET, "/calendar/time_tracks/exports")
        operation.info(OperationInfo(service = "TimeTracks", operation = "ExportTimeTracks", resourceType = "time_track", isMutation = false))
        operation.withoutJsonSuffix()
        operation.accept("text/csv")
        return client.execute(operation) { it.body }
    }
}
