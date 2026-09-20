package com.basecamp.hey.conformance

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Page
import com.basecamp.hey.SensitiveString
import com.basecamp.hey.TokenProvider
import com.basecamp.hey.generated.models.AddPostingsToBoxGroupRequestContent
import com.basecamp.hey.generated.models.BulkReplyMessagePayload
import com.basecamp.hey.generated.models.BulkReplyRequestContent
import com.basecamp.hey.generated.models.BulkUpdateClearancesRequestContent
import com.basecamp.hey.generated.models.CalendarTodoPayload
import com.basecamp.hey.generated.models.CollectionPayload
import com.basecamp.hey.generated.models.ContactNotePayload
import com.basecamp.hey.generated.models.ContactNoteRequestContent
import com.basecamp.hey.generated.models.ContactPayload
import com.basecamp.hey.generated.models.ContactRequestContent
import com.basecamp.hey.generated.models.CreateBoxDesignationRequestContent
import com.basecamp.hey.generated.models.CreateBoxGroupRequestContent
import com.basecamp.hey.generated.models.CreateCalendarTodoRequestContent
import com.basecamp.hey.generated.models.CreateContactRequestContent
import com.basecamp.hey.generated.models.CreateDirectUploadRequestContent
import com.basecamp.hey.generated.models.CreateFolderForPostingsRequestContent
import com.basecamp.hey.generated.models.CreateMessageRequestContent
import com.basecamp.hey.generated.models.CreateReplyRequestContent
import com.basecamp.hey.generated.models.DirectUploadBlob
import com.basecamp.hey.generated.models.FilePostingsRequestContent
import com.basecamp.hey.generated.models.FolderPayload
import com.basecamp.hey.generated.models.HabitPayload
import com.basecamp.hey.generated.models.HabitRequestContent
import com.basecamp.hey.generated.models.JournalEntryPayload
import com.basecamp.hey.generated.models.MarkPostingsRequestContent
import com.basecamp.hey.generated.models.MessagePayload
import com.basecamp.hey.generated.models.MoveStickyRequestContent
import com.basecamp.hey.generated.models.MoveTopicRequestContent
import com.basecamp.hey.generated.models.UpdateTopicRequestContent
import com.basecamp.hey.generated.models.MoveWorkflowStagingRequestContent
import com.basecamp.hey.generated.models.MovePostingsRequestContent
import com.basecamp.hey.generated.models.ReplyMessagePayload
import com.basecamp.hey.generated.models.SchedulePostingsBubbleUpRequestContent
import com.basecamp.hey.generated.models.StickyPayload
import com.basecamp.hey.generated.models.StickyRequestContent
import com.basecamp.hey.generated.models.TimeTrackRequestContent
import com.basecamp.hey.generated.models.TrashPostingsRequestContent
import com.basecamp.hey.generated.models.UpdateClearanceRequestContent
import com.basecamp.hey.generated.models.UpdateCollectionRequestContent
import com.basecamp.hey.generated.models.UpdateContactClearanceRequestContent
import com.basecamp.hey.generated.models.UpdateJournalEntryRequestContent
import com.basecamp.hey.generated.models.UpdateMyClearanceRequestContent
import com.basecamp.hey.generated.models.UpdateTimeTrackPayload
import com.basecamp.hey.generated.models.UpdateTimeTrackRequestContent
import com.basecamp.hey.generated.models.WorkflowStagingPayload
import com.basecamp.hey.generated.*
import com.basecamp.hey.nextPage
import com.basecamp.hey.generated.services.AdvancedSearchOptions
import com.basecamp.hey.services.CalendarEventUpdate
import com.basecamp.hey.services.DraftContent
import com.basecamp.hey.generated.services.GetBoxPostingChangesOptions
import com.basecamp.hey.generated.services.GetBundleUnseenPostingsOptions
import com.basecamp.hey.generated.services.GetCalendarRecordingsOptions
import com.basecamp.hey.generated.services.GetCollectionOptions
import com.basecamp.hey.generated.services.GetContactOptions
import com.basecamp.hey.generated.services.ListStickiesOptions
import com.basecamp.hey.services.OccurrenceId
import com.basecamp.hey.services.OccurrenceScope
import com.basecamp.hey.services.ReplyContent
import com.basecamp.hey.generated.services.TrashTopicOptions
import com.basecamp.hey.generated.services.UnfilePostingsOptions
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.serializer
import java.util.concurrent.atomic.AtomicReference
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

private const val CONFORMANCE_TOKEN = "conformance-test-token"
private const val REFRESHED_TOKEN = "conformance-refreshed-token"

private val json = Json { encodeDefaults = true; explicitNulls = false }

/** What an operation answered, in a shape the assertions can read: the parsed model as JSON, plus a page's metadata. */
sealed class Outcome {
    object Unit : Outcome()
    class Json(val value: JsonElement) : Outcome()
    class Page(val value: JsonElement, val nextPage: String?, val totalCount: Long?, val nextUrlCheck: Result<kotlin.Unit>?) : Outcome()

    fun body(): JsonElement? = when (this) {
        Unit -> null
        is Json -> value
        is Page -> value
    }
}

/**
 * Builds the client the case asks for and runs its operation as many times as
 * `repeatOperation` says, answering what the last run did. A case at the generated layer
 * goes on past a run that failed with a status HEY answered, as the Go runner's generated
 * client does; any other failure ends the case.
 */
suspend fun executeCase(case: TestCase, baseUrl: String): Result<Outcome> {
    val client = runCatching { clientFor(case, baseUrl) }.getOrElse { return Result.failure(it) }
    var outcome: Result<Outcome> = Result.success(Outcome.Unit)
    repeat(case.runs) {
        outcome = runCatching { execute(client, case) }
        val error = outcome.exceptionOrNull()
        if (error != null) {
            val answered = !case.isHeyLayer && (error as? HeyException)?.httpStatus != null
            if (!answered) return outcome
        }
    }
    return outcome
}

/** The client layer the case exercises. */
suspend fun clientFor(case: TestCase, baseUrl: String): HeyClient {
    val credentials = ConformanceCredentials(case.configOverrides.refreshableCredentials)
    return when {
        case.isHeyLayer -> HeyClient {
            this.baseUrl = baseUrl
            accessToken(credentials)
            maxRetries = case.configOverrides.maxRetries ?: 0
            case.configOverrides.baseDelayMs?.let { baseRetryDelay = it.milliseconds }
            enableCache = case.configOverrides.cacheEnabled
        }
        case.configOverrides.accountId != null -> HeyClient {
            this.baseUrl = baseUrl
            accessToken(credentials)
            maxRetries = 0
        }.forAccount(case.configOverrides.accountId)
        else -> HeyClient {
            this.baseUrl = baseUrl
            accessToken(credentials)
            maxRetries = 3
            baseRetryDelay = 1.seconds
            maxRetryDelay = 30.seconds
        }
    }
}

/** The token every request goes out with. A refreshable one is swapped for the refreshed token on a 401. */
private class ConformanceCredentials(private val refreshable: Boolean) : TokenProvider {
    private val token = AtomicReference(CONFORMANCE_TOKEN)

    override suspend fun accessToken(): String = token.get()

    override suspend fun refresh(): Boolean {
        if (!refreshable) return false
        token.set(REFRESHED_TOKEN)
        return true
    }
}

private suspend fun execute(client: HeyClient, case: TestCase): Outcome = when {
    case.isHeyLayer -> executeHeyOperation(client, case)
    case.configOverrides.accountId != null -> executeAccountScopedOperation(client, case)
    else -> executeOperation(client, case)
}

private inline fun <reified T> asJson(value: T): Outcome = Outcome.Json(json.encodeToJsonElement(serializer<T>(), value))

private suspend inline fun <reified T> page(client: HeyClient, result: Page<T>, follow: Boolean): Outcome {
    val check = if (follow) runCatching { client.nextPage(result); Unit } else null
    return Outcome.Page(json.encodeToJsonElement(serializer<T>(), result.value), result.nextPage, result.totalCount, check)
}

private fun unknown(operation: String): Nothing = throw HeyException.Usage("unknown operation: $operation")

private suspend fun executeOperation(client: HeyClient, case: TestCase): Outcome {
    val path = case.pathParams
    val query = case.queryParams
    val body = case.requestBody
    val follow = case.followsNextPage

    return when (case.operation) {
        "GetIdentity" -> asJson(client.identity.get())
        "GetNavigation" -> asJson(client.identity.getNavigation())

        "ListBoxes" -> page(client, client.boxes.list(), follow)
        "GetBox" -> page(client, client.boxes.get(path.int64("boxId")), follow)
        "GetBoxPostingChanges" -> page(
            client,
            client.postings.getBoxChanges(
                path.int64("boxId"),
                query.string("since"),
                GetBoxPostingChangesOptions(v = query.nonEmptyString("v"), page = query.nonEmptyString("page")),
            ),
            follow,
        )
        "GetImbox" -> asJson(client.boxes.getImbox())
        "GetFeedbox" -> asJson(client.boxes.getFeedbox())
        "GetTrailbox" -> asJson(client.boxes.getTrailbox())
        "GetAsidebox" -> asJson(client.boxes.getAsidebox())
        "GetLaterbox" -> asJson(client.boxes.getLaterbox())
        "GetBubblebox" -> asJson(client.boxes.getBubblebox())

        "GetTopic" -> asJson(client.topics.get(path.int64("topicId")))
        "GetTopicEntries" -> page(client, client.topics.getEntries(path.int64("topicId")), follow)
        "GetSentTopics" -> page(client, client.topics.getSent(), follow)
        "GetSpamTopics" -> page(client, client.topics.getSpam(), follow)
        "GetTrashTopics" -> page(client, client.topics.getTrash(), follow)
        "GetEverythingTopics" -> page(client, client.topics.getEverything(), follow)

        "GetMessage" -> asJson(client.messages.get(path.int64("messageId")))
        "CreateMessage" -> {
            client.messages.create(messageBody(body))
            Outcome.Unit
        }
        "UpdateMessage" -> {
            client.messages.update(path.int64("messageId"), messageBody(body))
            Outcome.Unit
        }
        "GetMessageEdit" -> asJson(client.messages.getEdit(path.int64("messageId")))
        "CreateDirectUpload" -> asJson(
            client.attachments.createDirectUpload(
                CreateDirectUploadRequestContent(
                    blob = DirectUploadBlob(
                        filename = body.string("filename"),
                        byteSize = body.int64("byte_size"),
                        checksum = body.string("checksum"),
                        contentType = body.string("content_type"),
                    ),
                ),
            ),
        )
        "ListDrafts" -> page(client, client.entries.listDrafts(), follow)
        "DeleteDraft" -> {
            client.entries.deleteDraft(path.int64("entryId"))
            Outcome.Unit
        }
        "NewEntryReply" -> asJson(client.entries.newReply(path.int64("entryId")))
        "CreateReply" -> {
            client.entries.createReply(
                path.int64("entryId"),
                CreateReplyRequestContent(
                    actingSenderId = body.int64("acting_sender_id"),
                    message = ReplyMessagePayload(subject = body.string("subject"), content = body.string("content")),
                ),
            )
            Outcome.Unit
        }

        "ListContacts" -> page(client, client.contacts.list(), follow)
        "GetContact" -> page(client, client.contacts.get(path.int64("contactId"), GetContactOptions(page = query.stringOrNull("page"))), follow)

        "ListCalendars" -> asJson(client.calendars.list())
        "GetCalendarRecordings" -> page(
            client,
            client.calendars.getRecordings(
                path.int64("calendarId"),
                GetCalendarRecordingsOptions(startsOn = query.stringOrNull("starts_on"), endsOn = query.stringOrNull("ends_on"), page = query.stringOrNull("page")),
            ),
            follow,
        )

        "CreateCalendarTodo" -> asJson(client.calendarTodos.create(CreateCalendarTodoRequestContent(calendarTodo = CalendarTodoPayload(title = body.string("title")))))
        "CompleteCalendarTodo" -> asJson(client.calendarTodos.complete(path.int64("todoId")))
        "UncompleteCalendarTodo" -> asJson(client.calendarTodos.uncomplete(path.int64("todoId")))
        "DeleteCalendarTodo" -> {
            client.calendarTodos.delete(path.int64("todoId"))
            Outcome.Unit
        }

        "CompleteHabit" -> asJson(client.habits.complete(path.string("day"), path.int64("habitId")))
        "UncompleteHabit" -> asJson(client.habits.uncomplete(path.string("day"), path.int64("habitId")))

        "GetOngoingTimeTrack" -> client.timeTracks.getOngoing()?.let { asJson(it) } ?: Outcome.Unit
        "StartTimeTrack" -> asJson(client.timeTracks.start())
        "UpdateTimeTrack" -> asJson(client.timeTracks.update(path.int64("timeTrackId"), UpdateTimeTrackRequestContent(calendarTimeTrack = UpdateTimeTrackPayload())))

        "ListJournalEntries" -> page(client, client.journal.listEntries(), follow)
        "GetJournalEntry" -> asJson(client.journal.getEntry(path.string("day")))
        "UpdateJournalEntry" -> asJson(
            client.journal.updateEntry(path.string("day"), UpdateJournalEntryRequestContent(calendarJournalEntry = JournalEntryPayload(content = body.string("body")))),
        )

        "GetAdvancedSearchFilters" -> asJson(client.search.getAdvancedFilters())
        "AdvancedSearch" -> page(client, client.search.advanced(AdvancedSearchOptions(q = query.nonEmptyString("q"), refineFrom = query.nonEmptyString("refine[from]"))), follow)

        "ListClips" -> page(client, client.clips.list(), follow)
        "ListSnippets" -> asJson(client.snippets.list())
        "GetWorkflow" -> asJson(client.workflows.get(path.int64("workflowId")))
        "CreateWorkflowStaging" -> {
            client.workflows.createStaging(path.int64("topicId"), path.int64("workflowId"))
            Outcome.Unit
        }
        "MoveWorkflowStaging" -> {
            client.workflows.moveStaging(
                path.int64("topicId"),
                path.int64("workflowId"),
                MoveWorkflowStagingRequestContent(workflowStaging = WorkflowStagingPayload(workflowStageId = body.int64("workflow_stage_id"))),
            )
            Outcome.Unit
        }
        "ListTimeTracks" -> page(client, client.timeTracks.list(), follow)
        "ListTimeTrackCategories" -> asJson(client.timeTracks.listCategories())
        "GetTopicPublication" -> asJson(client.publications.get(path.int64("topicId")))

        "MarkPostingsSeen" -> {
            client.postings.markSeen(postingIdsBody(body))
            Outcome.Unit
        }
        "MarkPostingsUnseen" -> {
            client.postings.markUnseen(postingIdsBody(body))
            Outcome.Unit
        }
        "MovePostings" -> {
            client.postings.movePostings(MovePostingsRequestContent(postingIds = body.int64List("posting_ids"), boxId = body.int64("box_id")))
            Outcome.Unit
        }
        "TrashPostings" -> {
            client.postings.trash(TrashPostingsRequestContent(postingIds = body.int64List("posting_ids")))
            Outcome.Unit
        }
        "MutePostings" -> {
            client.postings.mute(postingIdsBody(body))
            Outcome.Unit
        }
        "UnmutePostings" -> {
            client.postings.unmute(query.string("posting_ids"))
            Outcome.Unit
        }
        "GetBundleUnseenPostings" -> page(client, client.postings.getBundleUnseen(path.int64("postingId"), GetBundleUnseenPostingsOptions(page = query.stringOrNull("page"))), follow)
        "MarkPostingsSpam" -> {
            client.postings.markSpam(postingIdsBody(body))
            Outcome.Unit
        }
        "AddPostingsToBoxGroup" -> {
            client.postings.addToBoxGroup(
                AddPostingsToBoxGroupRequestContent(postingIds = body.int64List("posting_ids"), boxId = body.int64("box_id"), boxGroupId = body.int64("box_group_id")),
            )
            Outcome.Unit
        }
        "RemovePostingsFromBoxGroup" -> {
            client.postings.removeFromBoxGroup(query.string("posting_ids"))
            Outcome.Unit
        }
        "FilePostings" -> {
            client.postings.file(FilePostingsRequestContent(postingIds = body.int64List("posting_ids"), folderId = body.int64("folder_id")))
            Outcome.Unit
        }
        "UnfilePostings" -> {
            client.postings.unfile(query.string("posting_ids"), UnfilePostingsOptions(folderId = query.gatedInt64("folder_id")))
            Outcome.Unit
        }
        "CreateFolderForPostings" -> {
            client.postings.createFolder(
                CreateFolderForPostingsRequestContent(postingIds = body.int64List("posting_ids"), folder = FolderPayload(name = body.string("name"))),
            )
            Outcome.Unit
        }
        "CancelPostingsBubbleUp" -> {
            client.postings.cancelBubbleUp(query.string("posting_ids"))
            Outcome.Unit
        }
        "SchedulePostingsBubbleUp" -> {
            client.postings.scheduleBubbleUp(
                SchedulePostingsBubbleUpRequestContent(postingIds = body.int64List("posting_ids"), slot = body.string("slot"), date = body.nonEmptyString("date")),
            )
            Outcome.Unit
        }
        "BubbleUpPostingsNow" -> {
            client.postings.bubbleUpNow(postingIdsBody(body))
            Outcome.Unit
        }

        "TrashTopic" -> {
            client.topics.trash(path.int64("topicId"), TrashTopicOptions(confirmDestroy = query.gatedString("confirm_destroy")))
            Outcome.Unit
        }
        "RestoreTopic" -> {
            client.topics.restore(path.int64("topicId"))
            Outcome.Unit
        }
        "MarkTopicHam" -> {
            client.topics.markHam(path.int64("topicId"))
            Outcome.Unit
        }
        "EmptyTrash" -> {
            client.topics.emptyTrash()
            Outcome.Unit
        }
        "EmptySpam" -> {
            client.topics.emptySpam()
            Outcome.Unit
        }
        "MoveTopic" -> {
            client.topics.moveTopic(path.int64("topicId"), MoveTopicRequestContent(boxId = body.int64("box_id")))
            Outcome.Unit
        }

        "UpdateTopic" -> {
            client.topics.update(path.int64("topicId"), UpdateTopicRequestContent(name = body.string("name")))
            Outcome.Unit
        }

        "MarkEntrySpam" -> {
            client.entries.markSpam(path.int64("entryId"))
            Outcome.Unit
        }
        "NewEntryForward" -> asJson(client.entries.newForward(path.int64("entryId")))

        "NewBulkReply" -> asJson(client.bulkReplies.newBulkReply(query.string("posting_ids")))
        "CreateBulkReply" -> asJson(
            client.bulkReplies.create(BulkReplyRequestContent(entryIds = body.int64List("entry_ids"), message = BulkReplyMessagePayload(content = body.string("content")))),
        )

        "BundleContact" -> {
            client.contacts.bundle(path.int64("contactId"))
            Outcome.Unit
        }
        "UnbundleContact" -> {
            client.contacts.unbundle(path.int64("contactId"))
            Outcome.Unit
        }
        "UpdateContactClearance" -> {
            client.contacts.updateClearance(path.int64("contactId"), UpdateContactClearanceRequestContent(status = body.string("status")))
            Outcome.Unit
        }
        "GetClearances" -> page(client, client.clearances.get(), follow)
        "UpdateClearance" -> asJson(client.clearances.update(path.int64("clearanceId"), UpdateClearanceRequestContent(status = body.string("status"))))
        "BulkUpdateClearances" -> asJson(client.clearances.bulkUpdate(BulkUpdateClearancesRequestContent(ids = body.string("ids"), status = body.string("status"))))
        "PuntClearances" -> {
            client.clearances.punt()
            Outcome.Unit
        }
        "GetMyClearances" -> page(client, client.clearances.getMy(), follow)
        "UpdateMyClearance" -> asJson(client.clearances.updateMy(path.int64("clearanceId"), UpdateMyClearanceRequestContent(status = body.string("status"))))

        "CreateContact" -> asJson(client.contacts.create(CreateContactRequestContent(actingUserId = body.gatedInt64("acting_user_id"), contact = contactPayload(body))))
        "UpdateContact" -> asJson(client.contacts.update(path.int64("contactId"), ContactRequestContent(contact = contactPayload(body))))
        "HideContact" -> {
            client.contacts.hide(path.int64("contactId"))
            Outcome.Unit
        }
        "RevealContact" -> asJson(client.contacts.reveal(path.int64("contactId")))
        "GetContactNote" -> asJson(client.contacts.getNote(path.int64("contactId")))
        "UpdateContactNote" -> asJson(client.contacts.updateNote(path.int64("contactId"), ContactNoteRequestContent(contact = ContactNotePayload(note = body.string("note")))))
        "DeleteContactNote" -> {
            client.contacts.deleteNote(path.int64("contactId"))
            Outcome.Unit
        }

        "CreateBoxDesignation" -> {
            client.designations.create(path.int64("boxId"), CreateBoxDesignationRequestContent(contactId = body.int64("contact_id")))
            Outcome.Unit
        }
        "DeleteBoxDesignation" -> {
            client.designations.delete(path.int64("boxId"), path.int64("designationId"))
            Outcome.Unit
        }
        "ListBoxGroups" -> asJson(client.boxes.listGroups(path.int64("boxId")))
        "GetBoxGroup" -> page(client, client.boxes.getGroup(path.int64("boxId"), path.int64("groupId")), follow)
        "CreateBoxGroup" -> asJson(client.boxes.createGroup(path.int64("boxId"), CreateBoxGroupRequestContent(postingIds = body.int64List("posting_ids"))))
        "DeleteBoxGroup" -> {
            client.boxes.deleteGroup(path.int64("boxId"), path.int64("groupId"))
            Outcome.Unit
        }
        "MarkBoxSeen" -> {
            client.boxes.markSeen(path.int64("boxId"))
            Outcome.Unit
        }

        "GetFolder" -> page(client, client.folders.get(path.int64("folderId")), follow)

        "ListCollections" -> asJson(client.collections.list())
        "GetCollection" -> page(client, client.collections.get(path.int64("collectionId"), GetCollectionOptions(page = query.gatedString("page"))), follow)
        "UpdateCollection" -> {
            client.collections.update(
                path.int64("collectionId"),
                UpdateCollectionRequestContent(collection = CollectionPayload(name = body.nonEmptyString("name"), summary = body.nonEmptyString("summary"))),
            )
            Outcome.Unit
        }

        "ListStickies" -> asJson(client.stickies.list(ListStickiesOptions(limit = query.gatedInt32("limit"))))
        "CreateSticky" -> asJson(client.stickies.create(stickyBody(body)))
        "UpdateSticky" -> asJson(client.stickies.update(path.int64("stickyId"), stickyBody(body)))
        "DeleteSticky" -> {
            client.stickies.delete(path.int64("stickyId"))
            Outcome.Unit
        }
        "MoveSticky" -> {
            client.stickies.moveSticky(MoveStickyRequestContent(id = body.int64("id"), position = body.int32("position")))
            Outcome.Unit
        }

        "CreateTimeTrack" -> asJson(
            client.timeTracks.create(
                TimeTrackRequestContent(
                    startsAt = body.string("starts_at"),
                    endsAt = body.string("ends_at"),
                    categoryTitle = body.nonEmptyString("category_title"),
                    notes = body.nonEmptyString("notes"),
                ),
            ),
        )
        "DeleteTimeTrack" -> {
            client.timeTracks.delete(path.int64("timeTrackId"))
            Outcome.Unit
        }

        "CreateHabit" -> asJson(client.habits.create(habitBody(body)))
        "UpdateHabit" -> asJson(client.habits.update(path.int64("habitId"), habitBody(body)))
        "DeleteHabit" -> {
            client.habits.delete(path.int64("habitId"))
            Outcome.Unit
        }
        "StopHabit" -> {
            client.habits.stop(path.int64("habitId"))
            Outcome.Unit
        }
        "ResumeHabit" -> {
            client.habits.resume(path.int64("habitId"))
            Outcome.Unit
        }

        else -> unknown(case.operation)
    }
}

/** A HEY-layer operation, the hand-written conveniences the generated methods sit under. */
private suspend fun executeHeyOperation(client: HeyClient, case: TestCase): Outcome {
    val path = case.pathParams
    val body = case.requestBody
    return when (case.operation) {
        "ListBoxes" -> page(client, client.boxes.list(), case.followsNextPage)
        "GetWorkflowStage" -> asJson(client.workflows.stage(path.int64("workflowId"), path.int64("stageId")))
        "UpdateCalendarEvent" -> {
            client.calendarEvents.update(path.int64("eventId"), calendarEventUpdate(body))
            Outcome.Unit
        }
        "DeleteCalendarEvent" -> {
            client.calendarEvents.delete(path.int64("eventId"))
            Outcome.Unit
        }
        "DeleteCalendarEventOccurrence" -> {
            client.calendarEvents.deleteOccurrenceScoped(OccurrenceId.parse(path.string("occurrenceId")), OccurrenceScope.parse(body.string("scope")))
            Outcome.Unit
        }
        "DeleteExtenzion" -> {
            client.extenzions.delete(path.int64("accountId"), path.int64("extenzionId"))
            Outcome.Unit
        }
        "CreateReply" -> {
            client.entries.reply(path.int64("entryId"), replyContent(body))
            Outcome.Unit
        }
        "CreateReplyDraft" -> {
            client.entries.replyDraft(path.int64("entryId"), replyContent(body))
            Outcome.Unit
        }
        "CreateDraft" -> {
            client.messages.createDraft(draftContent(body))
            Outcome.Unit
        }
        "UpdateDraft" -> {
            client.messages.updateDraft(path.int64("entryId"), draftContent(body))
            Outcome.Unit
        }
        "SendDraft" -> {
            client.messages.sendDraft(path.int64("entryId"), draftContent(body))
            Outcome.Unit
        }
        else -> unknown(case.operation)
    }
}

private suspend fun executeAccountScopedOperation(client: HeyClient, case: TestCase): Outcome =
    when (case.operation) {
        "ListBoxes" -> page(client, client.boxes.list(), case.followsNextPage)
        else -> unknown(case.operation)
    }

private fun messageBody(body: JsonObject): CreateMessageRequestContent = CreateMessageRequestContent(
    actingSenderId = body.int64("acting_sender_id"),
    message = MessagePayload(subject = body.string("subject"), content = body.string("content")),
)

private fun postingIdsBody(body: JsonObject): MarkPostingsRequestContent = MarkPostingsRequestContent(postingIds = body.int64List("posting_ids"))

private fun contactPayload(body: JsonObject): ContactPayload = ContactPayload(
    name = body.string("name"),
    emailAddress = body.nonEmptyString("email_address")?.let(::SensitiveString),
    aliasEmailAddresses = body.stringListOrNull("alias_email_addresses"),
)

private fun stickyBody(body: JsonObject): StickyRequestContent =
    StickyRequestContent(sticky = StickyPayload(body = body.nonEmptyString("body"), size = body.nonEmptyString("size")))

private fun habitBody(body: JsonObject): HabitRequestContent = HabitRequestContent(
    calendarHabit = HabitPayload(
        name = body.nonEmptyString("name"),
        icon = body.nonEmptyString("icon"),
        color = body.nonEmptyString("color"),
        days = body.int32ListOrNull("days"),
    ),
)

private fun calendarEventUpdate(body: JsonObject): CalendarEventUpdate = CalendarEventUpdate(
    title = body.stringOrNull("title"),
    startsAt = body.stringOrNull("starts_at"),
    endsAt = body.stringOrNull("ends_at"),
    allDay = body.boolOrNull("all_day"),
    startTime = body.stringOrNull("start_time"),
    endTime = body.stringOrNull("end_time"),
)

private fun replyContent(body: JsonObject): ReplyContent = ReplyContent(
    actingSenderId = body.int64("acting_sender_id"),
    subject = body.string("subject"),
    content = body.string("content"),
    to = body.stringList("to"),
    cc = body.stringList("cc"),
    bcc = body.stringList("bcc"),
)

/** The draft a lifecycle case sends, acting sender included. A case that names no sender leaves it to the client to resolve. */
private fun draftContent(body: JsonObject): DraftContent = DraftContent(
    subject = body.string("subject"),
    content = body.string("content"),
    to = body.stringList("to"),
    cc = body.stringList("cc"),
    bcc = body.stringList("bcc"),
    actingSenderId = body.gatedInt64("acting_sender_id"),
)
