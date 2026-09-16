import Foundation

/// Start and stop conveniences, the categories tracks are filed under and the export on top of
/// the generated surface (`start`, `update`, `getOngoing`, ...). HEY serves no JSON endpoint for a
/// category write or for the export, so those are browser form posts and a file read.
extension TimeTracksService {
    /// Starts a time track: ``start()`` with the one refusal it can meet named. A track already
    /// running answers 409, which arrives as ``HeyError/conflict(message:detail:)`` carrying HEY's
    /// own message, so a caller can branch on it. The rewording happens inside the operation, so
    /// the hooks hear the failure the caller gets.
    public func startTracking() async throws -> Recording {
        var operation = try client.operation(Routes.startTimeTrack, [])
        operation.quiet()
        return try await client.asOperation(operation.info) {
            do {
                return try await client.send(operation, as: Recording.self)
            } catch let error as HeyError where error.httpStatus == 409 {
                throw HeyError.conflict(
                    message: error.hint ?? "a time track is already running",
                    detail: ErrorDetail(requestId: error.requestId, body: error.body))
            }
        }
    }

    /// Stops the running time track by setting its end to now.
    public func stop(timeTrackId: Int) async throws {
        try await stopAndFile(timeTrackId: timeTrackId, categoryTitle: nil)
    }

    /// Stops a time track and files it under a category in the one request, creating the
    /// category if HEY has none by that name. It sends the same PUT
    /// ``update(timeTrackId:body:)`` does, but announces itself to the hooks as `StopTimeTrack`.
    public func stopAndFile(timeTrackId: Int, categoryTitle: String?) async throws {
        let body = UpdateTimeTrackRequestContent(
            calendarTimeTrack: UpdateTimeTrackPayload(
                categoryTitle: categoryTitle.flatMap { $0.isEmpty ? nil : $0 },
                endsAt: nowISO8601()))
        var operation = try client.operation(Routes.updateTimeTrack, [timeTrackId])
        operation.operationName("StopTimeTrack")
        operation.resourceId(timeTrackId)
        try operation.json(body)
        try await client.sendVoid(operation)
    }

    /// Adds a category to file tracks under.
    public func createCategory(title: String) async throws {
        var operation = client.form(.post, "/calendar/time_tracks/categories")
        operation.info = writeInfo(service: "TimeTracks", operation: "CreateTimeTrackCategory", resourceType: "category")
        operation.form([("category[title]", title)])
        try await client.sendVoid(operation)
    }

    /// Renames a category.
    public func updateCategory(categoryId: Int, title: String) async throws {
        var operation = client.form(.patch, "/calendar/time_tracks/categories/\(categoryId)")
        operation.info = writeInfo(
            service: "TimeTracks", operation: "UpdateTimeTrackCategory", resourceType: "category", resourceId: categoryId)
        operation.form([("category[title]", title)])
        try await client.sendVoid(operation)
    }

    /// Removes a category. The tracks filed under it stay, uncategorized.
    public func deleteCategory(categoryId: Int) async throws {
        var operation = client.form(.delete, "/calendar/time_tracks/categories/\(categoryId)")
        operation.info = writeInfo(
            service: "TimeTracks", operation: "DeleteTimeTrackCategory", resourceType: "category", resourceId: categoryId)
        try await client.sendVoid(operation)
    }

    /// Every completed time track as CSV, newest first, under the columns Start, End, Duration,
    /// Category and Notes. HEY streams this as a file rather than a document, from the bare path.
    public func export() async throws -> Data {
        var operation = client.request(.get, "/calendar/time_tracks/exports")
        operation.info = OperationInfo(service: "TimeTracks", operation: "ExportTimeTracks", resourceType: "time_track", isMutation: false)
        operation.withoutJSONSuffix()
        operation.accept = "text/csv"
        return try await client.execute(operation) { $0.body }
    }
}
