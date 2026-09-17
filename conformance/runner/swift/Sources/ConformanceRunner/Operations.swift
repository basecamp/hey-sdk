import ConformanceSupport
import Foundation
import Hey

private let conformanceToken = "conformance-test-token"
private let refreshedToken = "conformance-refreshed-token"

/// Builds the client the case asks for and runs its operation as many times as `repeatOperation`
/// says, answering what the last run did. A case at the generated layer goes on past a run that
/// failed with a status HEY answered, as the Go runner's generated client does; any other failure
/// ends the case.
func executeCase(_ testCase: TestCase, _ baseURL: String) async -> Result<Outcome, any Error> {
    let client: HeyClient
    switch await capture({ try await clientFor(testCase, baseURL) }) {
    case let .success(built): client = built
    case let .failure(error): return .failure(error)
    }
    var outcome: Result<Outcome, any Error> = .success(.unit)
    for _ in 0..<testCase.runs {
        outcome = await capture { try await execute(client, testCase) }
        if case let .failure(error) = outcome {
            let answered = !testCase.isHeyLayer && (error as? HeyError)?.httpStatus != nil
            if !answered { return outcome }
        }
    }
    return outcome
}

/// The client layer the case exercises.
func clientFor(_ testCase: TestCase, _ baseURL: String) async throws -> HeyClient {
    let overrides = testCase.configOverrides
    let credentials = ConformanceCredentials(refreshable: overrides.refreshableCredentials)
    if testCase.isHeyLayer {
        var config = HeyConfig(baseURL: baseURL, enableCache: overrides.cacheEnabled, maxRetries: overrides.maxRetries ?? 0)
        if let delay = overrides.baseDelayMs { config.baseRetryDelay = .milliseconds(delay) }
        return try HeyClient(tokenProvider: credentials, config: config)
    }
    if let accountId = overrides.accountId {
        return try await HeyClient(tokenProvider: credentials, config: HeyConfig(baseURL: baseURL, maxRetries: 0))
            .forAccount(accountId)
    }
    return try HeyClient(
        tokenProvider: credentials,
        config: HeyConfig(baseURL: baseURL, maxRetries: 3, baseRetryDelay: .seconds(1), maxRetryDelay: .seconds(30)))
}

/// The token every request goes out with. A refreshable one is swapped for the refreshed token on a 401.
private final class ConformanceCredentials: TokenProvider, @unchecked Sendable {
    private let refreshable: Bool
    private let lock = NSLock()
    private var token = conformanceToken

    init(refreshable: Bool) {
        self.refreshable = refreshable
    }

    func accessToken() async throws -> String {
        lock.withLock { token }
    }

    func refresh() async throws -> Bool {
        guard refreshable else { return false }
        lock.withLock { token = refreshedToken }
        return true
    }
}

private func execute(_ client: HeyClient, _ testCase: TestCase) async throws -> Outcome {
    if testCase.isHeyLayer { return try await executeHeyOperation(client, testCase) }
    if testCase.configOverrides.accountId != nil { return try await executeAccountScopedOperation(client, testCase) }
    return try await executeOperation(client, testCase)
}

/// The value as the assertions read it: encoded as the SDK would send it, and parsed back.
private func asJSON<T: Encodable>(_ value: T) throws -> Outcome {
    .json(try encoded(value))
}

private func encoded<T: Encodable>(_ value: T) throws -> FixtureJSON {
    try FixtureJSON.parse(String(decoding: JSONEncoder().encode(value), as: UTF8.self))
}

private func page<T: Encodable>(_ client: HeyClient, _ result: Page<T>, _ follow: Bool) async throws -> Outcome {
    let check: Result<Void, any Error>? = follow ? await capture { _ = try await client.nextPage(result) } : nil
    return .page(value: try encoded(result.value), nextPage: result.nextPage, totalCount: result.totalCount, nextURLCheck: check)
}

private func unknown(_ operation: String) -> HeyError {
    .usage(message: "unknown operation: \(operation)")
}

private func executeOperation(_ client: HeyClient, _ testCase: TestCase) async throws -> Outcome {
    let path = testCase.pathParams
    let query = testCase.queryParams
    let body = testCase.requestBody
    let follow = testCase.followsNextPage

    switch testCase.operation {
    case "GetIdentity": return try asJSON(try await client.identity.get())
    case "GetNavigation": return try asJSON(try await client.identity.getNavigation())

    case "ListBoxes": return try await page(client, try await client.boxes.list(), follow)
    case "GetBox": return try await page(client, try await client.boxes.get(boxId: path.int("boxId")), follow)
    case "GetBoxPostingChanges":
        return try await page(
            client,
            try await client.postings.getBoxChanges(
                boxId: path.int("boxId"), since: query.string("since"),
                options: GetBoxPostingChangesOptions(v: query.nonEmptyString("v"), page: query.nonEmptyString("page"))),
            follow)
    case "GetImbox": return try asJSON(try await client.boxes.getImbox())
    case "GetFeedbox": return try asJSON(try await client.boxes.getFeedbox())
    case "GetTrailbox": return try asJSON(try await client.boxes.getTrailbox())
    case "GetAsidebox": return try asJSON(try await client.boxes.getAsidebox())
    case "GetLaterbox": return try asJSON(try await client.boxes.getLaterbox())
    case "GetBubblebox": return try asJSON(try await client.boxes.getBubblebox())

    case "GetTopic": return try asJSON(try await client.topics.get(topicId: path.int("topicId")))
    case "GetTopicEntries": return try await page(client, try await client.topics.getEntries(topicId: path.int("topicId")), follow)
    case "GetSentTopics": return try await page(client, try await client.topics.getSent(), follow)
    case "GetSpamTopics": return try await page(client, try await client.topics.getSpam(), follow)
    case "GetTrashTopics": return try await page(client, try await client.topics.getTrash(), follow)
    case "GetEverythingTopics": return try await page(client, try await client.topics.getEverything(), follow)

    case "GetMessage": return try asJSON(try await client.messages.get(messageId: path.int("messageId")))
    case "CreateMessage":
        try await client.messages.create(body: messageBody(body))
        return .unit
    case "UpdateMessage":
        try await client.messages.update(messageId: path.int("messageId"), body: messageBody(body))
        return .unit
    case "GetMessageEdit": return try asJSON(try await client.messages.getEdit(messageId: path.int("messageId")))
    case "CreateDirectUpload":
        return try asJSON(
            try await client.attachments.createDirectUpload(
                body: CreateDirectUploadRequestContent(
                    blob: DirectUploadBlob(
                        filename: body.string("filename"), byteSize: body.int("byte_size"), checksum: body.string("checksum"),
                        contentType: body.string("content_type")))))
    case "ListDrafts": return try await page(client, try await client.entries.listDrafts(), follow)
    case "DeleteDraft":
        try await client.entries.deleteDraft(entryId: path.int("entryId"))
        return .unit
    case "NewEntryReply": return try asJSON(try await client.entries.newReply(entryId: path.int("entryId")))
    case "CreateReply":
        try await client.entries.createReply(
            entryId: path.int("entryId"),
            body: CreateReplyRequestContent(
                actingSenderId: body.int("acting_sender_id"),
                message: ReplyMessagePayload(content: body.string("content"), subject: body.string("subject"))))
        return .unit

    case "ListContacts": return try await page(client, try await client.contacts.list(), follow)
    case "GetContact":
        return try await page(
            client,
            try await client.contacts.get(contactId: path.int("contactId"), options: GetContactOptions(page: query.stringOrNil("page"))),
            follow)

    case "ListCalendars": return try asJSON(try await client.calendars.list())
    case "GetCalendarRecordings":
        return try await page(
            client,
            try await client.calendars.getRecordings(
                calendarId: path.int("calendarId"),
                options: GetCalendarRecordingsOptions(
                    startsOn: query.stringOrNil("starts_on"), endsOn: query.stringOrNil("ends_on"), page: query.stringOrNil("page"))),
            follow)

    case "CreateCalendarTodo":
        return try asJSON(
            try await client.calendarTodos.create(
                body: CreateCalendarTodoRequestContent(calendarTodo: CalendarTodoPayload(title: body.string("title")))))
    case "CompleteCalendarTodo": return try asJSON(try await client.calendarTodos.complete(todoId: path.int("todoId")))
    case "UncompleteCalendarTodo": return try asJSON(try await client.calendarTodos.uncomplete(todoId: path.int("todoId")))
    case "DeleteCalendarTodo":
        try await client.calendarTodos.delete(todoId: path.int("todoId"))
        return .unit

    case "CompleteHabit": return try asJSON(try await client.habits.complete(day: path.string("day"), habitId: path.int("habitId")))
    case "UncompleteHabit": return try asJSON(try await client.habits.uncomplete(day: path.string("day"), habitId: path.int("habitId")))

    case "GetOngoingTimeTrack":
        guard let track = try await client.timeTracks.getOngoing() else { return .unit }
        return try asJSON(track)
    case "StartTimeTrack": return try asJSON(try await client.timeTracks.start())
    case "UpdateTimeTrack":
        return try asJSON(
            try await client.timeTracks.update(
                timeTrackId: path.int("timeTrackId"), body: UpdateTimeTrackRequestContent(calendarTimeTrack: UpdateTimeTrackPayload())))

    case "ListJournalEntries": return try await page(client, try await client.journal.listEntries(), follow)
    case "GetJournalEntry": return try asJSON(try await client.journal.getEntry(day: path.string("day")))
    case "UpdateJournalEntry":
        return try asJSON(
            try await client.journal.updateEntry(
                day: path.string("day"),
                body: UpdateJournalEntryRequestContent(calendarJournalEntry: JournalEntryPayload(content: body.string("body")))))

    case "GetAdvancedSearchFilters": return try asJSON(try await client.search.getAdvancedFilters())
    case "AdvancedSearch":
        return try await page(
            client,
            try await client.search.advanced(
                options: AdvancedSearchOptions(q: query.nonEmptyString("q"), refineFrom: query.nonEmptyString("refine[from]"))),
            follow)

    case "ListClips": return try await page(client, try await client.clips.list(), follow)
    case "ListSnippets": return try asJSON(try await client.snippets.list())
    case "GetWorkflow": return try asJSON(try await client.workflows.get(workflowId: path.int("workflowId")))
    case "CreateWorkflowStaging":
        try await client.workflows.createStaging(topicId: path.int("topicId"), workflowId: path.int("workflowId"))
        return .unit
    case "MoveWorkflowStaging":
        try await client.workflows.moveStaging(
            topicId: path.int("topicId"), workflowId: path.int("workflowId"),
            body: MoveWorkflowStagingRequestContent(
                workflowStaging: WorkflowStagingPayload(workflowStageId: body.int("workflow_stage_id"))))
        return .unit
    case "ListTimeTracks": return try await page(client, try await client.timeTracks.list(), follow)
    case "ListTimeTrackCategories": return try asJSON(try await client.timeTracks.listCategories())
    case "GetTopicPublication": return try asJSON(try await client.publications.get(topicId: path.int("topicId")))

    case "MarkPostingsSeen":
        try await client.postings.markSeen(body: postingIdsBody(body))
        return .unit
    case "MarkPostingsUnseen":
        try await client.postings.markUnseen(body: postingIdsBody(body))
        return .unit
    case "MovePostings":
        try await client.postings.movePostings(
            body: MovePostingsRequestContent(postingIds: body.intList("posting_ids"), boxId: body.int("box_id")))
        return .unit
    case "TrashPostings":
        try await client.postings.trash(body: TrashPostingsRequestContent(postingIds: body.intList("posting_ids")))
        return .unit
    case "MutePostings":
        try await client.postings.mute(body: postingIdsBody(body))
        return .unit
    case "UnmutePostings":
        try await client.postings.unmute(postingIds: query.string("posting_ids"))
        return .unit
    case "GetBundleUnseenPostings":
        return try await page(
            client,
            try await client.postings.getBundleUnseen(
                postingId: path.int("postingId"), options: GetBundleUnseenPostingsOptions(page: query.stringOrNil("page"))),
            follow)
    case "MarkPostingsSpam":
        try await client.postings.markSpam(body: postingIdsBody(body))
        return .unit
    case "AddPostingsToBoxGroup":
        try await client.postings.addToBoxGroup(
            body: AddPostingsToBoxGroupRequestContent(
                postingIds: body.intList("posting_ids"), boxId: body.int("box_id"), boxGroupId: body.int("box_group_id")))
        return .unit
    case "RemovePostingsFromBoxGroup":
        try await client.postings.removeFromBoxGroup(postingIds: query.string("posting_ids"))
        return .unit
    case "FilePostings":
        try await client.postings.file(
            body: FilePostingsRequestContent(postingIds: body.intList("posting_ids"), folderId: body.int("folder_id")))
        return .unit
    case "UnfilePostings":
        try await client.postings.unfile(
            postingIds: query.string("posting_ids"), options: UnfilePostingsOptions(folderId: query.gatedInt("folder_id")))
        return .unit
    case "CreateFolderForPostings":
        try await client.postings.createFolder(
            body: CreateFolderForPostingsRequestContent(
                postingIds: body.intList("posting_ids"), folder: FolderPayload(name: body.string("name"))))
        return .unit
    case "CancelPostingsBubbleUp":
        try await client.postings.cancelBubbleUp(postingIds: query.string("posting_ids"))
        return .unit
    case "SchedulePostingsBubbleUp":
        try await client.postings.scheduleBubbleUp(
            body: SchedulePostingsBubbleUpRequestContent(
                postingIds: body.intList("posting_ids"), slot: body.string("slot"), date: body.nonEmptyString("date")))
        return .unit
    case "BubbleUpPostingsNow":
        try await client.postings.bubbleUpNow(body: postingIdsBody(body))
        return .unit

    case "TrashTopic":
        try await client.topics.trash(
            topicId: path.int("topicId"), options: TrashTopicOptions(confirmDestroy: query.gatedString("confirm_destroy")))
        return .unit
    case "RestoreTopic":
        try await client.topics.restore(topicId: path.int("topicId"))
        return .unit
    case "MarkTopicHam":
        try await client.topics.markHam(topicId: path.int("topicId"))
        return .unit
    case "EmptyTrash":
        try await client.topics.emptyTrash()
        return .unit
    case "EmptySpam":
        try await client.topics.emptySpam()
        return .unit
    case "MoveTopic":
        try await client.topics.moveTopic(topicId: path.int("topicId"), body: MoveTopicRequestContent(boxId: body.int("box_id")))
        return .unit

    case "UpdateTopic":
        try await client.topics.update(topicId: path.int("topicId"), body: UpdateTopicRequestContent(name: body.string("name")))
        return .unit

    case "MarkEntrySpam":
        try await client.entries.markSpam(entryId: path.int("entryId"))
        return .unit
    case "NewEntryForward": return try asJSON(try await client.entries.newForward(entryId: path.int("entryId")))

    case "NewBulkReply": return try asJSON(try await client.bulkReplies.newBulkReply(postingIds: query.string("posting_ids")))
    case "CreateBulkReply":
        return try asJSON(
            try await client.bulkReplies.create(
                body: BulkReplyRequestContent(
                    entryIds: body.intList("entry_ids"), message: BulkReplyMessagePayload(content: body.string("content")))))

    case "BundleContact":
        try await client.contacts.bundle(contactId: path.int("contactId"))
        return .unit
    case "UnbundleContact":
        try await client.contacts.unbundle(contactId: path.int("contactId"))
        return .unit
    case "UpdateContactClearance":
        try await client.contacts.updateClearance(
            contactId: path.int("contactId"), body: UpdateContactClearanceRequestContent(status: body.string("status")))
        return .unit
    case "GetClearances": return try await page(client, try await client.clearances.get(), follow)
    case "UpdateClearance":
        return try asJSON(
            try await client.clearances.update(
                clearanceId: path.int("clearanceId"), body: UpdateClearanceRequestContent(status: body.string("status"))))
    case "BulkUpdateClearances":
        return try asJSON(
            try await client.clearances.bulkUpdate(
                body: BulkUpdateClearancesRequestContent(ids: body.string("ids"), status: body.string("status"))))
    case "PuntClearances":
        try await client.clearances.punt()
        return .unit
    case "GetMyClearances": return try await page(client, try await client.clearances.getMy(), follow)
    case "UpdateMyClearance":
        return try asJSON(
            try await client.clearances.updateMy(
                clearanceId: path.int("clearanceId"), body: UpdateMyClearanceRequestContent(status: body.string("status"))))

    case "CreateContact":
        return try asJSON(
            try await client.contacts.create(
                body: CreateContactRequestContent(contact: contactPayload(body), actingUserId: body.gatedInt("acting_user_id"))))
    case "UpdateContact":
        return try asJSON(
            try await client.contacts.update(contactId: path.int("contactId"), body: ContactRequestContent(contact: contactPayload(body))))
    case "HideContact":
        try await client.contacts.hide(contactId: path.int("contactId"))
        return .unit
    case "RevealContact": return try asJSON(try await client.contacts.reveal(contactId: path.int("contactId")))
    case "GetContactNote": return try asJSON(try await client.contacts.getNote(contactId: path.int("contactId")))
    case "UpdateContactNote":
        return try asJSON(
            try await client.contacts.updateNote(
                contactId: path.int("contactId"),
                body: ContactNoteRequestContent(contact: ContactNotePayload(note: body.string("note")))))
    case "DeleteContactNote":
        try await client.contacts.deleteNote(contactId: path.int("contactId"))
        return .unit

    case "CreateBoxDesignation":
        try await client.designations.create(
            boxId: path.int("boxId"), body: CreateBoxDesignationRequestContent(contactId: body.int("contact_id")))
        return .unit
    case "DeleteBoxDesignation":
        try await client.designations.delete(boxId: path.int("boxId"), designationId: path.int("designationId"))
        return .unit
    case "ListBoxGroups": return try asJSON(try await client.boxes.listGroups(boxId: path.int("boxId")))
    case "GetBoxGroup":
        return try await page(client, try await client.boxes.getGroup(boxId: path.int("boxId"), groupId: path.int("groupId")), follow)
    case "CreateBoxGroup":
        return try asJSON(
            try await client.boxes.createGroup(
                boxId: path.int("boxId"), body: CreateBoxGroupRequestContent(postingIds: body.intList("posting_ids"))))
    case "DeleteBoxGroup":
        try await client.boxes.deleteGroup(boxId: path.int("boxId"), groupId: path.int("groupId"))
        return .unit
    case "MarkBoxSeen":
        try await client.boxes.markSeen(boxId: path.int("boxId"))
        return .unit

    case "GetFolder": return try await page(client, try await client.folders.get(folderId: path.int("folderId")), follow)

    case "ListCollections": return try asJSON(try await client.collections.list())
    case "GetCollection":
        return try await page(
            client,
            try await client.collections.get(
                collectionId: path.int("collectionId"), options: GetCollectionOptions(page: query.gatedString("page"))),
            follow)
    case "UpdateCollection":
        try await client.collections.update(
            collectionId: path.int("collectionId"),
            body: UpdateCollectionRequestContent(
                collection: CollectionPayload(name: body.nonEmptyString("name"), summary: body.nonEmptyString("summary"))))
        return .unit

    case "ListStickies":
        return try asJSON(try await client.stickies.list(options: ListStickiesOptions(limit: query.gatedInt("limit").map { Int32($0) })))
    case "CreateSticky": return try asJSON(try await client.stickies.create(body: stickyBody(body)))
    case "UpdateSticky": return try asJSON(try await client.stickies.update(stickyId: path.int("stickyId"), body: stickyBody(body)))
    case "DeleteSticky":
        try await client.stickies.delete(stickyId: path.int("stickyId"))
        return .unit
    case "MoveSticky":
        try await client.stickies.moveSticky(
            body: MoveStickyRequestContent(id: body.int("id"), position: Int32(clamping: body.int("position"))))
        return .unit

    case "CreateTimeTrack":
        return try asJSON(
            try await client.timeTracks.create(
                body: TimeTrackRequestContent(
                    startsAt: body.string("starts_at"), endsAt: body.string("ends_at"),
                    categoryTitle: body.nonEmptyString("category_title"), notes: body.nonEmptyString("notes"))))
    case "DeleteTimeTrack":
        try await client.timeTracks.delete(timeTrackId: path.int("timeTrackId"))
        return .unit

    case "CreateHabit": return try asJSON(try await client.habits.create(body: habitBody(body)))
    case "UpdateHabit": return try asJSON(try await client.habits.update(habitId: path.int("habitId"), body: habitBody(body)))
    case "DeleteHabit":
        try await client.habits.delete(habitId: path.int("habitId"))
        return .unit
    case "StopHabit":
        try await client.habits.stop(habitId: path.int("habitId"))
        return .unit
    case "ResumeHabit":
        try await client.habits.resume(habitId: path.int("habitId"))
        return .unit

    default:
        throw unknown(testCase.operation)
    }
}

/// A HEY-layer operation, the hand-written conveniences the generated methods sit under.
private func executeHeyOperation(_ client: HeyClient, _ testCase: TestCase) async throws -> Outcome {
    let path = testCase.pathParams
    let body = testCase.requestBody
    switch testCase.operation {
    case "ListBoxes": return try await page(client, try await client.boxes.list(), testCase.followsNextPage)
    case "GetWorkflowStage":
        return try asJSON(try await client.workflows.stage(workflowId: path.int("workflowId"), stageId: path.int("stageId")))
    case "UpdateCalendarEvent":
        try await client.calendarEvents.update(eventId: path.int("eventId"), update: calendarEventUpdate(body))
        return .unit
    case "DeleteCalendarEvent":
        try await client.calendarEvents.delete(eventId: path.int("eventId"))
        return .unit
    case "DeleteCalendarEventOccurrence":
        try await client.calendarEvents.deleteOccurrenceScoped(
            occurrence: try OccurrenceId.parse(path.string("occurrenceId")), scope: try OccurrenceScope.parse(body.string("scope")))
        return .unit
    case "DeleteExtenzion":
        try await client.extenzions.delete(accountId: path.int("accountId"), extenzionId: path.int("extenzionId"))
        return .unit
    case "CreateReply":
        try await client.entries.reply(entryId: path.int("entryId"), reply: replyContent(body))
        return .unit
    case "CreateReplyDraft":
        _ = try await client.entries.replyDraft(entryId: path.int("entryId"), reply: replyContent(body))
        return .unit
    case "CreateDraft":
        _ = try await client.messages.createDraft(draftContent(body))
        return .unit
    case "UpdateDraft":
        try await client.messages.updateDraft(entryId: path.int("entryId"), draft: draftContent(body))
        return .unit
    case "SendDraft":
        try await client.messages.sendDraft(entryId: path.int("entryId"), draft: draftContent(body))
        return .unit
    default:
        throw unknown(testCase.operation)
    }
}

private func executeAccountScopedOperation(_ client: HeyClient, _ testCase: TestCase) async throws -> Outcome {
    switch testCase.operation {
    case "ListBoxes": return try await page(client, try await client.boxes.list(), testCase.followsNextPage)
    default: throw unknown(testCase.operation)
    }
}

private func messageBody(_ body: FixtureJSON) -> CreateMessageRequestContent {
    CreateMessageRequestContent(
        actingSenderId: body.int("acting_sender_id"),
        message: MessagePayload(subject: body.string("subject"), content: body.string("content")))
}

private func postingIdsBody(_ body: FixtureJSON) -> MarkPostingsRequestContent {
    MarkPostingsRequestContent(postingIds: body.intList("posting_ids"))
}

private func contactPayload(_ body: FixtureJSON) -> ContactPayload {
    ContactPayload(
        name: body.string("name"), emailAddress: body.nonEmptyString("email_address").map { SensitiveString($0) },
        aliasEmailAddresses: body.stringListOrNil("alias_email_addresses"))
}

private func stickyBody(_ body: FixtureJSON) -> StickyRequestContent {
    StickyRequestContent(sticky: StickyPayload(body: body.nonEmptyString("body"), size: body.nonEmptyString("size")))
}

private func habitBody(_ body: FixtureJSON) -> HabitRequestContent {
    HabitRequestContent(
        calendarHabit: HabitPayload(
            name: body.nonEmptyString("name"), icon: body.nonEmptyString("icon"), color: body.nonEmptyString("color"),
            days: body.intListOrNil("days")?.map { Int32(clamping: $0) }))
}

private func calendarEventUpdate(_ body: FixtureJSON) -> CalendarEventUpdate {
    CalendarEventUpdate(
        title: body.stringOrNil("title"), startsAt: body.stringOrNil("starts_at"), endsAt: body.stringOrNil("ends_at"),
        allDay: body.boolOrNil("all_day"), startTime: body.stringOrNil("start_time"), endTime: body.stringOrNil("end_time"))
}

private func replyContent(_ body: FixtureJSON) -> ReplyContent {
    ReplyContent(
        actingSenderId: body.int("acting_sender_id"), subject: body.string("subject"), content: body.string("content"),
        to: body.stringList("to"), cc: body.stringList("cc"), bcc: body.stringList("bcc"))
}

/// The draft a lifecycle case sends, acting sender included. A case that names no sender leaves it
/// to the client to resolve.
private func draftContent(_ body: FixtureJSON) -> DraftContent {
    DraftContent(
        subject: body.string("subject"), content: body.string("content"), to: body.stringList("to"), cc: body.stringList("cc"),
        bcc: body.stringList("bcc"), actingSenderId: body.gatedInt("acting_sender_id"))
}
