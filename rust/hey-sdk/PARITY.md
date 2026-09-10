# Go ↔ Rust parity

What the Go SDK's hand-written service methods (`go/pkg/hey/*.go`) map to in the Rust crate,
where the Rust half is a generated method (`src/generated/services/*.rs`, one per modelled
operation) or a hand-written convenience (`src/services/*.rs`). Parity is semantic — the same
request with the same behaviour — not one method name per method name: Rust types what Go
passes as strings and zero values, and answers `Page<T>` where Go unwraps the payload.

Counts at the time of writing: 202 Go methods; 131 modelled operations, each a Rust route and
a generated method; 122 Rust conveniences. Every Go method has a Rust route to the same
request except the waivers at the end. Both runners dispatch the whole shared conformance
suite.

## Behaviour the model declares, and who honours it

| contract | Go | Rust |
|---|---|---|
| idempotency | per-operation literal in the generated client; a form post is sent once, plus the one resend after a refreshed 401 | `Route::idempotent` → `Operation` → the retry budget; `Client::form` sends once, plus that same resend |
| empty-on statuses | by hand at the one call site (`GetOngoing`) | `Route::empty_on` → `send_optional` → `Option` |
| pagination style | by hand per `*Page` method | `Route::pagination` → generated methods answer `Page<T>`; `next_page`/`each_page` keep the route's policy |
| retry policy | model policy under the client ceiling (#164) | model policy under the client ceiling (#154) |
| HTML representation | by hand (`GetStage` strips `.json`, asks `text/html`) | `Route::html` → `Operation` asks as written |

## Go method → Rust

Unqualified Rust names are conveniences; `gen` marks a generated method.

| Go | sends | Rust | notes |
|---|---|---|---|
| Postings.MarkSeen / MarkUnseen | MarkPostingsSeen / MarkPostingsUnseen | `mark_postings_seen` / `mark_postings_unseen` | every bulk method refuses an empty selection, as Go's `bulkAction` does |
| Postings.Move(boxID, ids) | MovePostings | `move_to_box(box_id, ids)` | |
| Postings.MoveToBox(kind) | ListBoxes + MovePostings | `move_to_kind(BoxKind)` | Go re-reads the box index on every miss; Rust reads once per client |
| Postings.MoveToImbox … MoveToPaperTrail | as above | `move_to_imbox` … `move_to_paper_trail` | |
| Postings.MoveToTrash / TrashForEveryone | TrashPostings | `move_to_trash` / `trash_for_everyone` | |
| Postings.Mute / Unmute / MarkSpam | MutePostings / UnmutePostings / MarkPostingsSpam | `mute_postings` / `unmute_postings` / `mark_postings_spam` | |
| Postings.AddToBoxGroup / RemoveFromBoxGroup | AddPostingsToBoxGroup / RemovePostingsFromBoxGroup | `add_postings_to_box_group` / `remove_postings_from_box_group` | |
| Postings.File / Unfile / CreateFolder | FilePostings / UnfilePostings / CreateFolderForPostings | `file_postings` / `unfile_postings` / `create_folder_for_postings` | `Unfile` omits `folder_id` when zero in both |
| Postings.CancelBubbleUp / BubbleUpNow | CancelPostingsBubbleUp / BubbleUpPostingsNow | `cancel_postings_bubble_up` / `bubble_up_postings_now` | |
| Postings.ScheduleBubbleUp(date) / ScheduleBubbleUpFor(slot) | SchedulePostingsBubbleUp | `schedule_postings_bubble_up(BubbleUpSlot, ids)` | one method, typed slot |
| Postings.BundleUnseenPage | GetBundleUnseenPostings | gen `get_bundle_unseen` → `Page` | |
| Postings.AllChanges / Changes | GetBoxPostingChanges | `all_changes` / `changes` | 409 → `full_sync_required`; both bypass the cache |
| Boxes.List / Get / GetPage | ListBoxes / GetBox | gen `list` / `get` → `Page` | |
| Boxes.GetImbox … GetBubblebox | GetImbox … GetBubblebox | gen `get_imbox` … `get_bubblebox` | |
| Boxes.ListGroups / GetGroup / GetGroupPage / DeleteGroup / MarkSeen | ListBoxGroups / GetBoxGroup / DeleteBoxGroup / MarkBoxSeen | gen | |
| Boxes.CreateGroup(boxID, ids) | CreateBoxGroup | `create_box_group` | |
| Topics.Get / GetEntries / GetEntriesPage | GetTopic / GetTopicEntries | gen `get` / `get_entries` → `Page` | |
| Topics.GetSent / GetSpam / GetTrash / GetEverything | same ids | gen → `Page` | |
| Topics.Trash(id, confirm) | TrashTopic | `trash_topic` | both turn the removal redirect into a usage error |
| Topics.Restore / MarkHam / EmptyTrash / EmptySpam | same ids | gen | |
| Topics.Move(topicID, boxID) | MoveTopic | `move_to_box(topic_id, box_id)` | added with this file; gen `move_topic` takes the body |
| Contacts.List / Get / ThreadsPage | ListContacts / GetContact | gen `list` / `get` → `Page` | |
| Contacts.Bundle / Unbundle / Hide / Reveal / Note / DeleteNote | same ids | gen | |
| Contacts.Screen(id, status) | UpdateContactClearance | `screen(id, ClearanceStatus)` | |
| Contacts.Create / Update / SetNote | CreateContact / UpdateContact / UpdateContactNote | `create_contact` / `update_contact` / `set_note` | Rust always sends the alias list, so `Some(vec![])` clears it, which Go's `omitempty` cannot |
| Clearances.PendingCount / Summary / Pending / PendingPage | GetClearances | `pending_count` / `summary` / `pending` / `pending_page` | |
| Clearances.Screen / ScreenMany / Punt | UpdateClearance / BulkUpdateClearances / PuntClearances | `screen` / `screen_many` / gen `punt` | |
| Clearances.Screened / ScreenedPage / Rescreen | GetMyClearances / UpdateMyClearance | `screened` / `screened_page` / `rescreen` | |
| Messages.Get / GetEdit | GetMessage / GetMessageEdit | gen | |
| Messages.Create | CreateMessage | `send(&MessageContent)` | Rust adds an optional `acting_sender_id` |
| Messages.CreateDraft / UpdateDraft / SendDraft | CreateMessage / UpdateMessage | `create_draft` / `update_draft` / `send_draft` | neither resends; Go parses 422 `errors[]` itself, Rust maps 422 to `Validation` with the server message |
| Entries.ListDrafts / ListDraftsPage | ListDrafts | gen `list_drafts` → `Page` | |
| Entries.CreateReply / CreateReplyDraft | CreateReply | `reply` / `reply_draft` | a reply refuses no recipients in both; a draft may have none |
| Entries.MarkSpam / DeleteDraft / NewReply / NewForward | same ids | gen | |
| Calendars.List / GetRecordings / GetRecordingsPage | ListCalendars / GetCalendarRecordings | gen | |
| Calendars.Toggle | ToggleCalendar | `toggle_selection` | |
| Calendars.ListWithChanges | GET `/calendars.json` | `list_with_changes` | |
| Calendars.AllCalendarChanges / CalendarChanges / AllRecordingChanges / RecordingChanges | unmodelled change feeds | same names | |
| CalendarEvents.Create / Update / UpdateOccurrence | forms | `create` / `update_event` / `update_occurrence` | Rust also has a narrower `update` |
| CalendarEvents.Delete / DeleteOccurrence | DeleteCalendarEvent / DeleteCalendarEventOccurrence | gen `delete` / `delete_occurrence_scoped` (hand-written over gen `delete_occurrence`) | |
| CalendarPeriods.Day / Days / Week / Weeks / Year | GetCalendarDay … GetCalendarYear | `day` / `days` / `week` / `weeks` / `year` | |
| CalendarTodos.Create / Update | CreateCalendarTodo / UpdateCalendarTodo | `create_todo(title, Option<Date>)` / `update_todo` | both bypass the generated timestamp payload for a bare date |
| CalendarTodos.Complete / Uncomplete / Delete | same ids | gen | |
| Habits.Create / Update | CreateHabit / UpdateHabit | `create_habit` / `update_habit` | |
| Habits.Complete / Uncomplete / Delete / Stop / Resume | same ids | gen | |
| Journal.ListPage | ListJournalEntries | gen `list_entries` → `Page` | |
| Journal.Get / GetContent / Update | GetJournalEntry / UpdateJournalEntry | `entry` / `get_content` / `update_content` → `Option` | a 204 is "nothing there" in both |
| Identity.GetIdentity / GetNavigation | GetIdentity / GetNavigation | gen | |
| Identity.UpdateFirstWeekDay / UpdateTimeFormat | same ids | `set_first_week_day(Weekday)` / `set_time_format` | |
| TimeTracks.List / ListPage / Update / Create / Delete / Categories | same ids | gen | |
| TimeTracks.GetOngoing | GetOngoingTimeTrack | gen `get_ongoing` → `Option` | from the model's `empty_on` |
| TimeTracks.Start | StartTimeTrack | `start_tracking` | 409 → conflict |
| TimeTracks.Stop / StopAndFile | UpdateTimeTrack as `StopTimeTrack` | `stop` / `stop_and_file` | |
| TimeTracks.CreateCategory / UpdateCategory / DeleteCategory / Export | forms, CSV | same names | |
| Workflows.List | autocomplete GET | `list` → `Vec<WorkflowSummary>` | |
| Workflows.Get / Stages | GetWorkflow | gen `get` / `stages` | |
| Workflows.GetStage → WorkflowStageView | GetWorkflowStage (HTML) | `stage` → `WorkflowStageView`; gen `get_stage` → the page | added with this file, same parser rules as Go; a page without the stage is `NotFound` in Rust and a plain error in Go |
| Workflows.Create / Update / Delete / CreateStage / UpdateStage / DeleteStage / UnstageTopic | forms | same names | |
| Workflows.StageTopic / MoveTopic | CreateWorkflowStaging + MoveWorkflowStaging (form) | `stage_topic` / `move_topic_to_stage` | the read-back is `quiet()` in Rust; hooks hear one operation in both |
| World.Publish / Update / Delete / ExportSubscribers / ImportSubscribers | forms, CSV, multipart | `publish` / `update_post` / `delete_post` / `export_subscribers` / `import_subscribers` | |
| Search.Search / SearchPage / Filters | AdvancedSearch / GetAdvancedSearchFilters | `search` / `search_page` / gen `get_advanced_filters` | |
| Publications.Get / Create / Delete | GetTopicPublication, forms | gen `get` / `publish` / `unpublish` | |
| Extenzions.List | GetNavigation | `list` | Go announces a nested `GetNavigation`; Rust announces `ListExtenzions` only |
| Extenzions.Create / Update / Delete | forms, DeleteExtenzion | `create` / `update` → `Option<Extenzion>` / gen `delete` | |
| Attachments.CreateDirectUpload / Upload | CreateDirectUpload, storage PUT | gen `create_direct_upload` / `upload` | Go refuses an empty reservation at the single call; Rust inside `upload` |
| BulkReplies.Draft / Send / Undo | NewBulkReply / CreateBulkReply, form | `draft` / `send` / `undo` | |
| Collections.List / Get / GetPage | ListCollections / GetCollection | gen | |
| Collections.Update / Create / AddTopic / RemoveTopic | UpdateCollection, forms | `update_collection` / `create` / `add_topic` / `remove_topic` | |
| Folders.Get / GetPage | GetFolder | gen `get` → `Page` | |
| Clips.List / Create / Delete | ListClips, forms | gen `list` → `Page` / `create` / `delete` | |
| Designations.Create / Destroy | CreateBoxDesignation / DeleteBoxDesignation | `create_box_designation` / gen `delete` | |
| Snippets.List / Create / Update / Delete | ListSnippets, forms | gen `list` / `create` / `update` / `delete` | |
| Stickies.List(limit) / Create / Update / Delete / Move | ListStickies … MoveSticky | `list_up_to` / `create_sticky` / `update_sticky` / gen `delete` / `move_to` | both clamp the limit to 100 |

## Rust-only

`CalendarEvents::update` (a six-field partial revision), `Boxes::kinds`, `FromStr` for the
typed enums, `ContactConflict::from_error`, `services::write_info`, and every generated
method Go reaches only through its generated client (`messages.create`, `topics.trash` with
params, `clearances.get`, …).

## Waived

- `Contacts.Clearances` — deprecated in Go; `clearances().summary()` is the same read.
- `Client.BoxIDByKind` lives on Go's client; Rust's `Boxes::id_by_kind` is the counterpart.
