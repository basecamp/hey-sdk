use std::sync::Mutex;
use std::time::Duration;

use async_trait::async_trait;
use hey_sdk::cache::InMemoryCache;
use hey_sdk::services::{
    CalendarEventUpdate, DraftContent, OccurrenceId, OccurrenceScope, ReplyContent,
};
use hey_sdk::services::{
    boxes, calendars, clearances, collections, contacts, entries, folders, journal, postings,
    search, stickies, time_tracks, topics,
};
use hey_sdk::{Client, Config, Error, Page, SensitiveString, TokenProvider, models};
use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::fixtures::{
    Params, TestCase, bool_ptr_param, date_time_param, gated_int32_param, gated_int64_param,
    gated_string_param, int32_list_param, int32_param, int64_list_param, int64_param,
    non_empty_string_param, optional_string_list_param, string_list_param, string_param,
    string_ptr_param,
};

const CONFORMANCE_TOKEN: &str = "conformance-test-token";
const REFRESHED_TOKEN: &str = "conformance-refreshed-token";

/// What an operation answered, in a shape the assertions can read: the parsed model as
/// JSON, plus the pagination metadata a `Page` carries.
pub enum Outcome {
    Unit,
    Json(Value),
    Page {
        value: Value,
        next_page: Option<String>,
        total_count: Option<u64>,
        next_url_check: Option<Result<(), Error>>,
    },
}

impl Outcome {
    pub fn body(&self) -> Option<&Value> {
        match self {
            Outcome::Unit => None,
            Outcome::Json(value) => Some(value),
            Outcome::Page { value, .. } => Some(value),
        }
    }
}

/// Builds the client the case asks for and runs its operation, as many times as
/// `repeatOperation` says, stopping at the first error.
pub async fn execute_case(case: &TestCase, base_url: &str) -> Result<Outcome, Error> {
    let client = client_for(case, base_url).await?;
    let mut outcome = Outcome::Unit;
    for _ in 0..case.runs() {
        outcome = execute(&client, case).await?;
    }
    Ok(outcome)
}

/// The client layer the case exercises. An account-scoped client checks the account
/// against the identity as it is derived, so that read is the case's first request and its
/// failure is the case's error.
async fn client_for(case: &TestCase, base_url: &str) -> Result<Client, Error> {
    let config = Config::default().with_base_url(base_url);
    let credentials = ConformanceCredentials::new(case.config_overrides.refreshable_credentials);
    if case.is_hey_layer() {
        let mut builder = Client::builder(config)
            .token_provider(credentials)
            .max_retries(0);
        if case.config_overrides.cache_enabled {
            builder = builder.cache(InMemoryCache::new());
        }
        builder.build()
    } else if let Some(account_id) = case.config_overrides.account_id {
        let client = Client::builder(config)
            .token_provider(credentials)
            .max_retries(0)
            .build()?;
        client.for_account(account_id).await
    } else {
        Client::builder(config)
            .token_provider(credentials)
            .max_retries(3)
            .base_delay(Duration::from_secs(1))
            .max_delay(Duration::from_secs(30))
            .build()
    }
}

/// The token every request goes out with. A case that marks its credentials refreshable
/// has a 401 answered by swapping in the refreshed token; for any other case the refresh
/// fails and the 401 stands.
struct ConformanceCredentials {
    token: Mutex<String>,
    refreshable: bool,
}

impl ConformanceCredentials {
    fn new(refreshable: bool) -> ConformanceCredentials {
        ConformanceCredentials {
            token: Mutex::new(CONFORMANCE_TOKEN.to_string()),
            refreshable,
        }
    }
}

#[async_trait]
impl TokenProvider for ConformanceCredentials {
    async fn access_token(&self) -> Result<String, Error> {
        let token = self.token.lock().unwrap().clone();
        Ok(token)
    }

    async fn refresh(&self) -> bool {
        if self.refreshable {
            REFRESHED_TOKEN.clone_into(&mut self.token.lock().unwrap());
            true
        } else {
            false
        }
    }
}

async fn execute(client: &Client, case: &TestCase) -> Result<Outcome, Error> {
    if case.is_hey_layer() {
        execute_hey_operation(client, case).await
    } else if case.config_overrides.account_id.is_some() {
        execute_account_scoped_operation(client, case).await
    } else {
        execute_operation(client, case).await
    }
}

async fn execute_operation(client: &Client, case: &TestCase) -> Result<Outcome, Error> {
    let path = &case.path_params;
    let query = &case.query_params;
    let body = &case.request_body;
    let follow = case.follows_next_page();

    match case.operation.as_str() {
        "GetIdentity" => json(client.identity().get().await?),
        "GetNavigation" => json(client.identity().get_navigation().await?),

        "ListBoxes" => page(client, client.boxes().list().await?, follow).await,
        "GetBox" => {
            let result = client
                .boxes()
                .get(int64_param(path, "boxId"), &boxes::GetBoxParams::default())
                .await?;
            page(client, result, follow).await
        }
        "GetBoxPostingChanges" => {
            let params = postings::GetBoxPostingChangesParams {
                v: non_empty_string_param(query, "v"),
                page: non_empty_string_param(query, "page"),
                per_page: None,
            };
            let result = client
                .postings()
                .get_box_changes(
                    int64_param(path, "boxId"),
                    &string_param(query, "since"),
                    &params,
                )
                .await?;
            page(client, result, follow).await
        }
        "GetImbox" => json(
            client
                .boxes()
                .get_imbox(&boxes::GetImboxParams::default())
                .await?,
        ),
        "GetFeedbox" => json(
            client
                .boxes()
                .get_feedbox(&boxes::GetFeedboxParams::default())
                .await?,
        ),
        "GetTrailbox" => json(
            client
                .boxes()
                .get_trailbox(&boxes::GetTrailboxParams::default())
                .await?,
        ),
        "GetAsidebox" => json(
            client
                .boxes()
                .get_asidebox(&boxes::GetAsideboxParams::default())
                .await?,
        ),
        "GetLaterbox" => json(
            client
                .boxes()
                .get_laterbox(&boxes::GetLaterboxParams::default())
                .await?,
        ),
        "GetBubblebox" => json(
            client
                .boxes()
                .get_bubblebox(&boxes::GetBubbleboxParams::default())
                .await?,
        ),

        "GetTopic" => json(client.topics().get(int64_param(path, "topicId")).await?),
        "GetTopicEntries" => {
            let result = client
                .topics()
                .get_entries(
                    int64_param(path, "topicId"),
                    &topics::GetTopicEntriesParams::default(),
                )
                .await?;
            page(client, result, follow).await
        }
        "GetSentTopics" => {
            page(
                client,
                client
                    .topics()
                    .get_sent(&topics::GetSentTopicsParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "GetSpamTopics" => {
            page(
                client,
                client
                    .topics()
                    .get_spam(&topics::GetSpamTopicsParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "GetTrashTopics" => {
            page(
                client,
                client
                    .topics()
                    .get_trash(&topics::GetTrashTopicsParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "GetEverythingTopics" => {
            page(
                client,
                client
                    .topics()
                    .get_everything(&topics::GetEverythingTopicsParams::default())
                    .await?,
                follow,
            )
            .await
        }

        "GetMessage" => json(
            client
                .messages()
                .get(int64_param(path, "messageId"))
                .await?,
        ),
        "CreateMessage" => {
            client.messages().create(&message_body(body)).await?;
            Ok(Outcome::Unit)
        }
        "UpdateMessage" => {
            client
                .messages()
                .update(int64_param(path, "messageId"), &message_body(body))
                .await?;
            Ok(Outcome::Unit)
        }
        "GetMessageEdit" => json(
            client
                .messages()
                .get_edit(int64_param(path, "messageId"))
                .await?,
        ),
        "CreateDirectUpload" => {
            let request = models::CreateDirectUploadRequestContent {
                blob: models::DirectUploadBlob {
                    filename: string_param(body, "filename"),
                    byte_size: int64_param(body, "byte_size"),
                    checksum: string_param(body, "checksum"),
                    content_type: string_param(body, "content_type"),
                },
            };
            json(client.attachments().create_direct_upload(&request).await?)
        }
        "ListDrafts" => {
            page(
                client,
                client
                    .entries()
                    .list_drafts(&entries::ListDraftsParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "DeleteDraft" => {
            client
                .entries()
                .delete_draft(int64_param(path, "entryId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "NewEntryReply" => json(
            client
                .entries()
                .new_reply(int64_param(path, "entryId"))
                .await?,
        ),
        "CreateReply" => {
            let request = models::CreateReplyRequestContent {
                acting_sender_id: int64_param(body, "acting_sender_id"),
                message: models::ReplyMessagePayload {
                    subject: Some(string_param(body, "subject")),
                    content: string_param(body, "content"),
                },
                entry: None,
            };
            client
                .entries()
                .create_reply(int64_param(path, "entryId"), &request)
                .await?;
            Ok(Outcome::Unit)
        }

        "ListContacts" => {
            page(
                client,
                client
                    .contacts()
                    .list(&contacts::ListContactsParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "GetContact" => {
            let params = contacts::GetContactParams {
                page: string_ptr_param(query, "page"),
            };
            page(
                client,
                client
                    .contacts()
                    .get(int64_param(path, "contactId"), &params)
                    .await?,
                follow,
            )
            .await
        }

        "ListCalendars" => json(client.calendars().list().await?),
        "GetCalendarRecordings" => {
            let params = calendars::GetCalendarRecordingsParams {
                starts_on: string_ptr_param(query, "starts_on"),
                ends_on: string_ptr_param(query, "ends_on"),
                page: string_ptr_param(query, "page"),
            };
            let result = client
                .calendars()
                .get_recordings(int64_param(path, "calendarId"), &params)
                .await?;
            page(client, result, follow).await
        }

        "CreateCalendarTodo" => {
            let request = models::CreateCalendarTodoRequestContent {
                calendar_todo: models::CalendarTodoPayload {
                    title: string_param(body, "title"),
                    starts_at: None,
                },
            };
            json(client.calendar_todos().create(&request).await?)
        }
        "CompleteCalendarTodo" => json(
            client
                .calendar_todos()
                .complete(int64_param(path, "todoId"))
                .await?,
        ),
        "UncompleteCalendarTodo" => json(
            client
                .calendar_todos()
                .uncomplete(int64_param(path, "todoId"))
                .await?,
        ),
        "DeleteCalendarTodo" => {
            client
                .calendar_todos()
                .delete(int64_param(path, "todoId"))
                .await?;
            Ok(Outcome::Unit)
        }

        "CompleteHabit" => json(
            client
                .habits()
                .complete(&string_param(path, "day"), int64_param(path, "habitId"))
                .await?,
        ),
        "UncompleteHabit" => json(
            client
                .habits()
                .uncomplete(&string_param(path, "day"), int64_param(path, "habitId"))
                .await?,
        ),

        "GetOngoingTimeTrack" => match client.time_tracks().get_ongoing().await? {
            Some(track) => json(track),
            None => Ok(Outcome::Unit),
        },
        "StartTimeTrack" => json(client.time_tracks().start().await?),
        "UpdateTimeTrack" => {
            let request = models::UpdateTimeTrackRequestContent {
                calendar_time_track: models::UpdateTimeTrackPayload::default(),
            };
            json(
                client
                    .time_tracks()
                    .update(int64_param(path, "timeTrackId"), &request)
                    .await?,
            )
        }

        "ListJournalEntries" => {
            page(
                client,
                client
                    .journal()
                    .list_entries(&journal::ListJournalEntriesParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "GetJournalEntry" => json(
            client
                .journal()
                .get_entry(&string_param(path, "day"))
                .await?,
        ),
        "UpdateJournalEntry" => {
            let request = models::UpdateJournalEntryRequestContent {
                calendar_journal_entry: models::JournalEntryPayload {
                    content: string_param(body, "body"),
                },
            };
            json(
                client
                    .journal()
                    .update_entry(&string_param(path, "day"), &request)
                    .await?,
            )
        }

        "GetAdvancedSearchFilters" => json(client.search().get_advanced_filters().await?),
        "AdvancedSearch" => {
            let params = search::AdvancedSearchParams {
                q: non_empty_string_param(query, "q"),
                refine_from: non_empty_string_param(query, "refine[from]"),
                ..Default::default()
            };
            page(client, client.search().advanced(&params).await?, follow).await
        }

        "ListClips" => {
            page(
                client,
                client.clips().list(&Default::default()).await?,
                follow,
            )
            .await
        }
        "ListSnippets" => json(client.snippets().list().await?),
        "GetWorkflow" => json(
            client
                .workflows()
                .get(int64_param(path, "workflowId"))
                .await?,
        ),
        "CreateWorkflowStaging" => {
            client
                .workflows()
                .create_staging(
                    int64_param(path, "topicId"),
                    int64_param(path, "workflowId"),
                )
                .await?;
            Ok(Outcome::Unit)
        }
        "MoveWorkflowStaging" => {
            let request = models::MoveWorkflowStagingRequestContent {
                workflow_staging: models::WorkflowStagingPayload {
                    workflow_stage_id: int64_param(body, "workflow_stage_id"),
                },
            };
            client
                .workflows()
                .move_staging(
                    int64_param(path, "topicId"),
                    int64_param(path, "workflowId"),
                    &request,
                )
                .await?;
            Ok(Outcome::Unit)
        }
        "ListTimeTracks" => {
            page(
                client,
                client
                    .time_tracks()
                    .list(&time_tracks::ListTimeTracksParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "ListTimeTrackCategories" => json(client.time_tracks().list_categories().await?),
        "GetTopicPublication" => json(
            client
                .publications()
                .get(int64_param(path, "topicId"))
                .await?,
        ),

        "MarkPostingsSeen" => {
            client.postings().mark_seen(&posting_ids_body(body)).await?;
            Ok(Outcome::Unit)
        }
        "MarkPostingsUnseen" => {
            client
                .postings()
                .mark_unseen(&posting_ids_body(body))
                .await?;
            Ok(Outcome::Unit)
        }
        "MovePostings" => {
            let request = models::MovePostingsRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
                box_id: int64_param(body, "box_id"),
            };
            client.postings().move_postings(&request).await?;
            Ok(Outcome::Unit)
        }
        "TrashPostings" => {
            let request = models::TrashPostingsRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
                remove_access: None,
            };
            client.postings().trash(&request).await?;
            Ok(Outcome::Unit)
        }
        "MutePostings" => {
            client.postings().mute(&posting_ids_body(body)).await?;
            Ok(Outcome::Unit)
        }
        "UnmutePostings" => {
            client
                .postings()
                .unmute(&string_param(query, "posting_ids"))
                .await?;
            Ok(Outcome::Unit)
        }
        "GetBundleUnseenPostings" => {
            let params = postings::GetBundleUnseenPostingsParams {
                page: string_ptr_param(query, "page"),
            };
            let result = client
                .postings()
                .get_bundle_unseen(int64_param(path, "postingId"), &params)
                .await?;
            page(client, result, follow).await
        }
        "MarkPostingsSpam" => {
            client.postings().mark_spam(&posting_ids_body(body)).await?;
            Ok(Outcome::Unit)
        }
        "AddPostingsToBoxGroup" => {
            let request = models::AddPostingsToBoxGroupRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
                box_id: int64_param(body, "box_id"),
                box_group_id: int64_param(body, "box_group_id"),
            };
            client.postings().add_to_box_group(&request).await?;
            Ok(Outcome::Unit)
        }
        "RemovePostingsFromBoxGroup" => {
            client
                .postings()
                .remove_from_box_group(&string_param(query, "posting_ids"))
                .await?;
            Ok(Outcome::Unit)
        }
        "FilePostings" => {
            let request = models::FilePostingsRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
                folder_id: int64_param(body, "folder_id"),
            };
            client.postings().file(&request).await?;
            Ok(Outcome::Unit)
        }
        "UnfilePostings" => {
            let params = postings::UnfilePostingsParams {
                folder_id: gated_int64_param(query, "folder_id"),
            };
            client
                .postings()
                .unfile(&string_param(query, "posting_ids"), &params)
                .await?;
            Ok(Outcome::Unit)
        }
        "CreateFolderForPostings" => {
            let request = models::CreateFolderForPostingsRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
                folder: models::FolderPayload {
                    name: string_param(body, "name"),
                    status: None,
                },
            };
            client.postings().create_folder(&request).await?;
            Ok(Outcome::Unit)
        }
        "CancelPostingsBubbleUp" => {
            client
                .postings()
                .cancel_bubble_up(&string_param(query, "posting_ids"))
                .await?;
            Ok(Outcome::Unit)
        }
        "SchedulePostingsBubbleUp" => {
            let request = models::SchedulePostingsBubbleUpRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
                slot: string_param(body, "slot"),
                date: non_empty_string_param(body, "date"),
            };
            client.postings().schedule_bubble_up(&request).await?;
            Ok(Outcome::Unit)
        }
        "BubbleUpPostingsNow" => {
            client
                .postings()
                .bubble_up_now(&posting_ids_body(body))
                .await?;
            Ok(Outcome::Unit)
        }

        "TrashTopic" => {
            let params = topics::TrashTopicParams {
                confirm_destroy: gated_string_param(query, "confirm_destroy"),
            };
            client
                .topics()
                .trash(int64_param(path, "topicId"), &params)
                .await?;
            Ok(Outcome::Unit)
        }
        "RestoreTopic" => {
            client
                .topics()
                .restore(int64_param(path, "topicId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "MarkTopicHam" => {
            client
                .topics()
                .mark_ham(int64_param(path, "topicId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "EmptyTrash" => {
            client.topics().empty_trash().await?;
            Ok(Outcome::Unit)
        }
        "EmptySpam" => {
            client.topics().empty_spam().await?;
            Ok(Outcome::Unit)
        }
        "MoveTopic" => {
            let request = models::MoveTopicRequestContent {
                box_id: int64_param(body, "box_id"),
            };
            client
                .topics()
                .move_topic(int64_param(path, "topicId"), &request)
                .await?;
            Ok(Outcome::Unit)
        }

        "MarkEntrySpam" => {
            client
                .entries()
                .mark_spam(int64_param(path, "entryId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "NewEntryForward" => json(
            client
                .entries()
                .new_forward(int64_param(path, "entryId"))
                .await?,
        ),

        "NewBulkReply" => json(
            client
                .bulk_replies()
                .new_bulk_reply(&string_param(query, "posting_ids"))
                .await?,
        ),
        "CreateBulkReply" => {
            let request = models::BulkReplyRequestContent {
                entry_ids: int64_list_param(body, "entry_ids"),
                message: models::BulkReplyMessagePayload {
                    content: string_param(body, "content"),
                },
            };
            json(client.bulk_replies().create(&request).await?)
        }

        "BundleContact" => {
            client
                .contacts()
                .bundle(int64_param(path, "contactId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "UnbundleContact" => {
            client
                .contacts()
                .unbundle(int64_param(path, "contactId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "UpdateContactClearance" => {
            let request = models::UpdateContactClearanceRequestContent {
                status: string_param(body, "status"),
            };
            client
                .contacts()
                .update_clearance(int64_param(path, "contactId"), &request)
                .await?;
            Ok(Outcome::Unit)
        }
        "GetClearances" => {
            page(
                client,
                client
                    .clearances()
                    .get(&clearances::GetClearancesParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "UpdateClearance" => {
            let request = models::UpdateClearanceRequestContent {
                status: string_param(body, "status"),
                ..Default::default()
            };
            json(
                client
                    .clearances()
                    .update(int64_param(path, "clearanceId"), &request)
                    .await?,
            )
        }
        "BulkUpdateClearances" => {
            let request = models::BulkUpdateClearancesRequestContent {
                ids: string_param(body, "ids"),
                status: string_param(body, "status"),
                spam: None,
            };
            json(client.clearances().bulk_update(&request).await?)
        }
        "PuntClearances" => {
            client.clearances().punt().await?;
            Ok(Outcome::Unit)
        }
        "GetMyClearances" => {
            page(
                client,
                client
                    .clearances()
                    .get_my(&clearances::GetMyClearancesParams::default())
                    .await?,
                follow,
            )
            .await
        }
        "UpdateMyClearance" => {
            let request = models::UpdateMyClearanceRequestContent {
                status: string_param(body, "status"),
            };
            json(
                client
                    .clearances()
                    .update_my(int64_param(path, "clearanceId"), &request)
                    .await?,
            )
        }

        "CreateContact" => {
            let request = models::CreateContactRequestContent {
                acting_user_id: gated_int64_param(body, "acting_user_id"),
                contact: contact_payload(body),
            };
            json(client.contacts().create(&request).await?)
        }
        "UpdateContact" => {
            let request = models::ContactRequestContent {
                contact: contact_payload(body),
            };
            json(
                client
                    .contacts()
                    .update(int64_param(path, "contactId"), &request)
                    .await?,
            )
        }
        "HideContact" => {
            client
                .contacts()
                .hide(int64_param(path, "contactId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "RevealContact" => json(
            client
                .contacts()
                .reveal(int64_param(path, "contactId"))
                .await?,
        ),
        "GetContactNote" => json(
            client
                .contacts()
                .get_note(int64_param(path, "contactId"))
                .await?,
        ),
        "UpdateContactNote" => {
            let request = models::ContactNoteRequestContent {
                contact: models::ContactNotePayload {
                    note: string_param(body, "note"),
                },
            };
            json(
                client
                    .contacts()
                    .update_note(int64_param(path, "contactId"), &request)
                    .await?,
            )
        }
        "DeleteContactNote" => {
            client
                .contacts()
                .delete_note(int64_param(path, "contactId"))
                .await?;
            Ok(Outcome::Unit)
        }

        "CreateBoxDesignation" => {
            let request = models::CreateBoxDesignationRequestContent {
                contact_id: int64_param(body, "contact_id"),
            };
            client
                .designations()
                .create(int64_param(path, "boxId"), &request)
                .await?;
            Ok(Outcome::Unit)
        }
        "DeleteBoxDesignation" => {
            client
                .designations()
                .delete(
                    int64_param(path, "boxId"),
                    int64_param(path, "designationId"),
                )
                .await?;
            Ok(Outcome::Unit)
        }
        "ListBoxGroups" => json(
            client
                .boxes()
                .list_groups(int64_param(path, "boxId"))
                .await?,
        ),
        "GetBoxGroup" => {
            let params = boxes::GetBoxGroupParams::default();
            let result = client
                .boxes()
                .get_group(
                    int64_param(path, "boxId"),
                    int64_param(path, "groupId"),
                    &params,
                )
                .await?;
            page(client, result, follow).await
        }
        "CreateBoxGroup" => {
            let request = models::CreateBoxGroupRequestContent {
                posting_ids: int64_list_param(body, "posting_ids"),
            };
            json(
                client
                    .boxes()
                    .create_group(int64_param(path, "boxId"), &request)
                    .await?,
            )
        }
        "DeleteBoxGroup" => {
            client
                .boxes()
                .delete_group(int64_param(path, "boxId"), int64_param(path, "groupId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "MarkBoxSeen" => {
            client.boxes().mark_seen(int64_param(path, "boxId")).await?;
            Ok(Outcome::Unit)
        }

        "GetFolder" => {
            let params = folders::GetFolderParams::default();
            page(
                client,
                client
                    .folders()
                    .get(int64_param(path, "folderId"), &params)
                    .await?,
                follow,
            )
            .await
        }

        "ListCollections" => json(client.collections().list().await?),
        "GetCollection" => {
            let params = collections::GetCollectionParams {
                page: gated_string_param(query, "page"),
            };
            page(
                client,
                client
                    .collections()
                    .get(int64_param(path, "collectionId"), &params)
                    .await?,
                follow,
            )
            .await
        }
        "UpdateCollection" => {
            let request = models::UpdateCollectionRequestContent {
                collection: models::CollectionPayload {
                    name: non_empty_string_param(body, "name"),
                    summary: non_empty_string_param(body, "summary"),
                },
            };
            client
                .collections()
                .update(int64_param(path, "collectionId"), &request)
                .await?;
            Ok(Outcome::Unit)
        }

        "ListStickies" => {
            let params = stickies::ListStickiesParams {
                limit: gated_int32_param(query, "limit"),
            };
            json(client.stickies().list(&params).await?)
        }
        "CreateSticky" => json(client.stickies().create(&sticky_body(body)).await?),
        "UpdateSticky" => json(
            client
                .stickies()
                .update(int64_param(path, "stickyId"), &sticky_body(body))
                .await?,
        ),
        "DeleteSticky" => {
            client
                .stickies()
                .delete(int64_param(path, "stickyId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "MoveSticky" => {
            let request = models::MoveStickyRequestContent {
                id: int64_param(body, "id"),
                position: int32_param(body, "position"),
            };
            client.stickies().move_sticky(&request).await?;
            Ok(Outcome::Unit)
        }

        "CreateTimeTrack" => {
            let request = models::TimeTrackRequestContent {
                starts_at: date_time_param(body, "starts_at"),
                ends_at: date_time_param(body, "ends_at"),
                category_title: non_empty_string_param(body, "category_title"),
                notes: non_empty_string_param(body, "notes"),
            };
            json(client.time_tracks().create(&request).await?)
        }
        "DeleteTimeTrack" => {
            client
                .time_tracks()
                .delete(int64_param(path, "timeTrackId"))
                .await?;
            Ok(Outcome::Unit)
        }

        "CreateHabit" => json(client.habits().create(&habit_body(body)).await?),
        "UpdateHabit" => json(
            client
                .habits()
                .update(int64_param(path, "habitId"), &habit_body(body))
                .await?,
        ),
        "DeleteHabit" => {
            client.habits().delete(int64_param(path, "habitId")).await?;
            Ok(Outcome::Unit)
        }
        "StopHabit" => {
            client.habits().stop(int64_param(path, "habitId")).await?;
            Ok(Outcome::Unit)
        }
        "ResumeHabit" => {
            client.habits().resume(int64_param(path, "habitId")).await?;
            Ok(Outcome::Unit)
        }

        operation => Err(Error::usage(format!("unknown operation: {operation}"))),
    }
}

/// A HEY-layer operation, the hand-written conveniences the generated methods sit under.
async fn execute_hey_operation(client: &Client, case: &TestCase) -> Result<Outcome, Error> {
    let path = &case.path_params;
    let body = &case.request_body;

    match case.operation.as_str() {
        "ListBoxes" => {
            page(
                client,
                client.boxes().list().await?,
                case.follows_next_page(),
            )
            .await
        }
        "UpdateCalendarEvent" => {
            client
                .calendar_events()
                .update(int64_param(path, "eventId"), &calendar_event_update(body))
                .await?;
            Ok(Outcome::Unit)
        }
        "DeleteCalendarEvent" => {
            client
                .calendar_events()
                .delete(int64_param(path, "eventId"))
                .await?;
            Ok(Outcome::Unit)
        }
        "DeleteCalendarEventOccurrence" => {
            let occurrence: OccurrenceId = string_param(path, "occurrenceId").parse()?;
            let scope: OccurrenceScope = string_param(body, "scope").parse()?;
            client
                .calendar_events()
                .delete_occurrence_scoped(&occurrence, scope)
                .await?;
            Ok(Outcome::Unit)
        }
        "DeleteExtenzion" => {
            client
                .extenzions()
                .delete(
                    int64_param(path, "accountId"),
                    int64_param(path, "extenzionId"),
                )
                .await?;
            Ok(Outcome::Unit)
        }
        "CreateReply" => {
            client
                .entries()
                .reply(int64_param(path, "entryId"), &reply_content(body))
                .await?;
            Ok(Outcome::Unit)
        }
        "CreateReplyDraft" => {
            client
                .entries()
                .reply_draft(int64_param(path, "entryId"), &reply_content(body))
                .await?;
            Ok(Outcome::Unit)
        }
        "CreateDraft" => {
            client.messages().create_draft(&draft_content(body)).await?;
            Ok(Outcome::Unit)
        }
        "UpdateDraft" => {
            client
                .messages()
                .update_draft(int64_param(path, "entryId"), &draft_content(body))
                .await?;
            Ok(Outcome::Unit)
        }
        "SendDraft" => {
            client
                .messages()
                .send_draft(int64_param(path, "entryId"), &draft_content(body))
                .await?;
            Ok(Outcome::Unit)
        }
        operation => Err(Error::usage(format!("unknown operation: {operation}"))),
    }
}

async fn execute_account_scoped_operation(
    client: &Client,
    case: &TestCase,
) -> Result<Outcome, Error> {
    match case.operation.as_str() {
        "ListBoxes" => {
            page(
                client,
                client.boxes().list().await?,
                case.follows_next_page(),
            )
            .await
        }
        operation => Err(Error::usage(format!("unknown operation: {operation}"))),
    }
}

fn json(value: impl Serialize) -> Result<Outcome, Error> {
    Ok(Outcome::Json(serde_json::to_value(value)?))
}

/// A page's parsed body, its cursor and total count. The next page is only read when the
/// case asks what the SDK does with the `Link` header.
async fn page<T: Serialize + DeserializeOwned>(
    client: &Client,
    result: Page<T>,
    follow: bool,
) -> Result<Outcome, Error> {
    let next_page = result.next_page().map(str::to_string);
    let total_count = result.total_count();
    let next_url_check = if follow {
        Some(client.next_page(&result).await.map(|_| ()))
    } else {
        None
    };
    Ok(Outcome::Page {
        value: serde_json::to_value(result.value())?,
        next_page,
        total_count,
        next_url_check,
    })
}

fn message_body(body: &Params) -> models::CreateMessageRequestContent {
    models::CreateMessageRequestContent {
        acting_sender_id: int64_param(body, "acting_sender_id"),
        message: models::MessagePayload {
            subject: string_param(body, "subject"),
            content: string_param(body, "content"),
        },
        entry: None,
    }
}

fn posting_ids_body(body: &Params) -> models::MarkPostingsRequestContent {
    models::MarkPostingsRequestContent {
        posting_ids: int64_list_param(body, "posting_ids"),
    }
}

fn contact_payload(body: &Params) -> models::ContactPayload {
    models::ContactPayload {
        name: string_param(body, "name"),
        email_address: non_empty_string_param(body, "email_address").map(SensitiveString::new),
        alias_email_addresses: optional_string_list_param(body, "alias_email_addresses"),
    }
}

fn sticky_body(body: &Params) -> models::StickyRequestContent {
    models::StickyRequestContent {
        sticky: models::StickyPayload {
            body: non_empty_string_param(body, "body"),
            size: non_empty_string_param(body, "size"),
        },
    }
}

fn habit_body(body: &Params) -> models::HabitRequestContent {
    models::HabitRequestContent {
        calendar_habit: models::HabitPayload {
            name: non_empty_string_param(body, "name"),
            icon: non_empty_string_param(body, "icon"),
            color: non_empty_string_param(body, "color"),
            days: int32_list_param(body, "days"),
        },
    }
}

fn calendar_event_update(body: &Params) -> CalendarEventUpdate {
    CalendarEventUpdate {
        title: string_ptr_param(body, "title"),
        starts_at: string_ptr_param(body, "starts_at"),
        ends_at: string_ptr_param(body, "ends_at"),
        all_day: bool_ptr_param(body, "all_day"),
        start_time: string_ptr_param(body, "start_time"),
        end_time: string_ptr_param(body, "end_time"),
    }
}

fn reply_content(body: &Params) -> ReplyContent {
    ReplyContent {
        acting_sender_id: int64_param(body, "acting_sender_id"),
        subject: string_param(body, "subject"),
        content: string_param(body, "content"),
        to: string_list_param(body, "to"),
        cc: string_list_param(body, "cc"),
        bcc: string_list_param(body, "bcc"),
    }
}

/// The draft a lifecycle case sends, acting sender included — the wire behavior the
/// HEY-layer draft cases exist to pin down. A case that names no sender leaves it to the
/// client to resolve.
fn draft_content(body: &Params) -> DraftContent {
    DraftContent {
        acting_sender_id: gated_int64_param(body, "acting_sender_id"),
        subject: string_param(body, "subject"),
        content: string_param(body, "content"),
        to: string_list_param(body, "to"),
        cc: string_list_param(body, "cc"),
        bcc: string_list_param(body, "bcc"),
        schedule: None,
    }
}
