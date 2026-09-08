$version: "2"

// =============================================================================
// ARCHITECTURAL NOTE: Response Format Mappers
// =============================================================================
// The HEY API returns bare values — arrays for list endpoints and objects for
// single-entity endpoints. Smithy's AWS restJson1 protocol requires outputs to
// be modeled as wrapped structures because @httpPayload only supports string,
// blob, structure, union, and document types — not arrays or bare references.
//
// Two custom OpenApiMappers transform schemas during OpenAPI generation:
//   * BareArrayResponseMapper: List*ResponseContent → bare arrays
//   * BareObjectResponseMapper: Get*ResponseContent (single property) → bare $ref
//
// Multi-field responses (e.g., BoxShowResponse) are left wrapped.
// =============================================================================

// =============================================================================
// SOURCE-OF-TRUTH POLICY
// =============================================================================
// 1. haystack/config/routes.rb — canonical for endpoints
// 2. haystack/app/views/**/*.jbuilder — canonical for response shapes
// 3. iOS/Android clients — discovery aid (must confirm in routes.rb)
// 4. Live behavior — tiebreaker (broken endpoints excluded)
//
// Exception: Rails engine-mounted routes (e.g., ActiveStorage direct_uploads)
// are as canonical as routes.rb entries. See ADR on engine routes.
// =============================================================================

namespace hey

use smithy.api#documentation
use smithy.api#http
use smithy.api#httpLabel
use smithy.api#httpQuery
use smithy.api#httpPayload
use smithy.api#required
use smithy.api#readonly
use smithy.api#idempotent
use smithy.api#error
use smithy.api#httpError
use smithy.api#retryable
use smithy.api#sensitive
use smithy.api#tags
use smithy.api#timestampFormat
use aws.protocols#restJson1

use hey.traits#heyRetry
use hey.traits#heyPagination
use hey.traits#heyIdempotent
use hey.traits#heySensitive
use hey.traits#heyPolymorphic
use hey.traits#heyEmptyOn

/// ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
@timestampFormat("date-time")
timestamp DateTime

/// HEY API
@restJson1
service HEY {
    version: "2026-08-21"
    operations: [
        // Identity (4 MVP)
        GetIdentity
        GetNavigation
        UpdateFirstWeekDay
        UpdateTimeFormat

        // Boxes (8 MVP)
        ListBoxes
        GetBox
        GetImbox
        GetImboxSeen
        GetFeedbox
        GetTrailbox
        GetAsidebox
        GetLaterbox
        GetBubblebox

        // Topics (6 MVP)
        GetTopic
        GetTopicEntries
        GetSentTopics
        GetSpamTopics
        GetTrashTopics
        GetEverythingTopics

        // Messages (3 MVP)
        GetMessage
        CreateMessage
        UpdateMessage
        GetMessageEdit

        // Attachments
        CreateDirectUpload

        // Entries (2 MVP)
        ListDrafts
        DeleteDraft
        CreateReply
        NewEntryReply

        // Contacts (2 MVP)
        ListContacts
        GetContact

        // Calendars (2 MVP)
        ListCalendars
        GetCalendarRecordings
        ToggleCalendar

        // Calendar periods
        GetCalendarDay
        ListCalendarDays
        GetCalendarWeek
        ListCalendarWeeks
        GetCalendarYear

        // Calendar Todos (5 MVP)
        CreateCalendarTodo
        UpdateCalendarTodo
        CompleteCalendarTodo
        UncompleteCalendarTodo
        DeleteCalendarTodo

        // Calendar Habits (2 MVP)
        CompleteHabit
        UncompleteHabit

        // Calendar Habits — CRUD
        CreateHabit
        UpdateHabit
        DeleteHabit
        StopHabit
        ResumeHabit

        // Calendar Time Tracks (3 MVP)
        GetOngoingTimeTrack
        StartTimeTrack
        UpdateTimeTrack

        // Calendar Journal (3 MVP)
        ListJournalEntries
        GetJournalEntry
        UpdateJournalEntry

        // Search
        AdvancedSearch
        GetAdvancedSearchFilters

        // Postings (6 MVP)
        MarkPostingsSeen
        MarkPostingsUnseen
        MovePostings
        TrashPostings
        MutePostings
        UnmutePostings

        // Postings — bundles
        GetBundleUnseenPostings

        // Postings — bulk
        MarkPostingsSpam
        AddPostingsToBoxGroup
        RemovePostingsFromBoxGroup
        FilePostings
        UnfilePostings
        CreateFolderForPostings
        CancelPostingsBubbleUp
        BubbleUpPostingsNow
        SchedulePostingsBubbleUp

        // Topics — status and moves
        TrashTopic
        RestoreTopic
        MarkTopicHam
        EmptyTrash
        EmptySpam
        MoveTopic

        // Entries
        MarkEntrySpam
        NewEntryForward

        // Bulk reply
        NewBulkReply
        CreateBulkReply

        // Contacts — bundles and screening
        BundleContact
        UnbundleContact
        UpdateContactClearance
        GetClearances
        UpdateClearance
        BulkUpdateClearances
        PuntClearances
        GetMyClearances
        UpdateMyClearance

        // Contacts — writing and notes
        CreateContact
        UpdateContact
        HideContact
        RevealContact
        GetContactNote
        UpdateContactNote
        DeleteContactNote

        // Boxes — posting sync
        GetBoxPostingChanges

        // Boxes — designations, groups, observation
        CreateBoxDesignation
        DeleteBoxDesignation
        ListBoxGroups
        GetBoxGroup
        CreateBoxGroup
        DeleteBoxGroup
        MarkBoxSeen

        // Folders
        GetFolder

        // Collections
        ListCollections
        GetCollection
        UpdateCollection

        // Stickies
        ListStickies
        CreateSticky
        UpdateSticky
        DeleteSticky
        MoveSticky

        // Calendar Time Tracks — write
        CreateTimeTrack
        DeleteTimeTrack

        // Calendar Time Tracks — read
        ListTimeTracks

        // Calendar Time Track categories
        ListTimeTrackCategories

        // Clips, Snippets, Workflows, Publications — reads
        ListClips
        ListSnippets
        GetWorkflow
        CreateWorkflowStaging
        MoveWorkflowStaging
        GetTopicPublication

        // Calendar Events
        DeleteCalendarEvent
        DeleteCalendarEventOccurrence

        // Extenzions
        DeleteExtenzion
    ]
}

// =============================================================================
// ERRORS
// =============================================================================

@error("client")
@httpError(400)
structure BadRequestError {
    @required
    message: String
}

@error("client")
@httpError(401)
structure UnauthorizedError {
    @required
    message: String
}

@error("client")
@httpError(403)
structure ForbiddenError {
    @required
    message: String
}

@error("client")
@httpError(404)
structure NotFoundError {
    @required
    message: String
}

/// The server rejected what was sent. HEY answers {"errors": ["..."]} — the messages
/// the model itself produced — so a client can show them as they are.
@error("client")
@httpError(422)
structure UnprocessableEntityError {
    errors: MessageList

    message: String
}

list MessageList {
    member: String
}

/// The request conflicts with current state, e.g. starting a time track while one is
/// already ongoing. Time tracks answer {"error": "..."}; contact writes answer the
/// {"errors": [...]} list every other error path uses.
@error("client")
@httpError(409)
structure ConflictError {
    error: String

    errors: MessageList

    /// Contact writes only: the contact that was written -- a create that clashes
    /// still creates the contact -- and the contacts already holding the email
    /// addresses that were sent, so a client can offer the merge the web offers.
    contact_id: Long

    conflicting_contact_ids: ContactIdList
}

@error("client")
@httpError(429)
@retryable(throttling: true)
structure TooManyRequestsError {
    @required
    message: String
}

@error("server")
@httpError(500)
@retryable
structure InternalServerError {
    @required
    message: String
}

@error("server")
@httpError(503)
@retryable
structure ServiceUnavailableError {
    @required
    message: String
}

// =============================================================================
// SHARED SHAPES
// =============================================================================

/// Contact — the identity of someone in HEY
structure Contact {
    @required
    id: Long

    account_id: Long

    updated_at: DateTime

    name: String

    @heySensitive(category: "pii")
    email_address: String

    avatar_url: String

    initials: String

    avatar_background_color: String

    contactable_type: String

    name_tag: String
}

list ContactIdList {
    member: Long
}

list ContactList {
    member: Contact
}

/// Extenzion — external account extension
structure Extenzion {
    @required
    id: Long
    name: String
    app_url: String
}

list ExtenzionList {
    member: Extenzion
}

/// Collection — email collection/label
structure Collection {
    @required
    id: Long
    name: String
    created_at: DateTime
    updated_at: DateTime
    app_url: String
}

list CollectionList {
    member: Collection
}

/// CollectionWithPostings — collection detail with its threads as posting objects
structure CollectionWithPostings {
    @required
    id: Long
    name: String
    created_at: DateTime
    updated_at: DateTime
    app_url: String
    postings: PostingList
}

/// Folder — email folder
structure Folder {
    @required
    id: Long
    name: String
    created_at: DateTime
    updated_at: DateTime
    app_url: String
}

list FolderList {
    member: Folder
}

/// Workflow — email workflow/label
structure Workflow {
    @required
    id: Long
    name: String
    created_at: DateTime
    updated_at: DateTime
    app_url: String
    /// The workflow's stages in position order. Present on GetWorkflow.
    stages: WorkflowStageList
}

structure WorkflowStage {
    @required
    id: Long
    name: String
}

list WorkflowStageList {
    member: WorkflowStage
}

list WorkflowList {
    member: Workflow
}

list PostingIdList {
    member: Long
}

/// Domain — email domain
structure Domain {
    @required
    id: Long
    address: String
    app_url: String
    avatar_url: String
}

/// Clearance — screening status for a contact
///
/// petitioner and most_recent_entry are only filled in by the Screener reads. The
/// contact reads answer a clearance with nothing but its id and status.
structure Clearance {
    @required
    id: Long
    status: String
    created_at: DateTime
    updated_at: DateTime
    petitioner: Contact
    most_recent_entry: Entry
}

list ClearanceList {
    member: Clearance
}

/// ClearanceListResponse — wire format: {clearances: [...]}
structure ClearanceListResponse {
    clearances: ClearanceList
}

/// Account — a HEY account
structure Account {
    @required
    id: Long
    name: String
    domain: String
    status: String
    purpose: String
    trial: Boolean
    trial_ends_on: String
    burner: Boolean
    readonly: Boolean
}

list AccountList {
    member: Account
}

/// User — a user within an account
structure User {
    @required
    id: Long
    account_id: Long
    account_purpose_icon_url: String
    contact: Contact
    external_accounts: ExternalAccountList
    auto_responder: Boolean
}

structure ExternalAccount {
    @required
    id: Long
    contact: Contact
}

list ExternalAccountList {
    member: ExternalAccount
}

list UserList {
    member: User
}

/// Sender — a contact with default flag
structure Sender {
    @required
    id: Long
    account_id: Long
    name: String
    @heySensitive(category: "pii")
    email_address: String
    avatar_url: String
    initials: String
    avatar_background_color: String
    contactable_type: String
    name_tag: String
    default: Boolean
}

list SenderList {
    member: Sender
}

/// Entry — a message entry within a topic
structure Entry {
    @required
    id: Long
    created_at: DateTime
    updated_at: DateTime
    creator: Contact
    alternative_sender_name: String
    summary: String
    kind: String
    app_url: String
    subject: String
    topic_id: Long
}

list EntryList {
    member: Entry
}

/// Note — a posting note
structure PostingNote {
    @required
    id: Long
    content: String
}

/// BubbleUpSchedule
structure BubbleUpSchedule {
    bubble_up_at: DateTime
    surprise_me: Boolean
}

/// Posting — polymorphic by `kind` (topic, bundle, entry)
@heyPolymorphic(
    discriminator: "kind"
    variants: {
        "topic": ["name", "blocked_trackers", "contacts", "extenzions", "folders",
                  "collections", "workflows", "visible_entry_count"]
        "bundle": ["name", "blocked_trackers", "app_bundle_url"]
        "entry": ["entry_kind", "addressed_contacts"]
    }
)
structure Posting {
    @required
    id: Long

    created_at: DateTime
    updated_at: DateTime
    observed_at: DateTime
    active_at: DateTime
    box_id: Long
    account_id: Long

    /// Discriminator: "topic", "bundle", or "entry"
    @required
    kind: String

    seen: Boolean
    bundled: Boolean
    muted: Boolean
    note: PostingNote
    preapproved_clearance: Boolean
    box_group_id: Long
    includes_attachments: Boolean
    includes_calendar_invites: Boolean
    bubbled_up: Boolean
    bubble_up_waiting_on: Boolean
    bubble_up_schedule: BubbleUpSchedule

    // Shared across kinds
    creator: Contact
    app_url: String
    summary: String
    alternative_sender_name: String

    // Topic-kind fields
    name: String
    blocked_trackers: Boolean
    contacts: ContactList
    extenzions: ExtenzionList
    folders: FolderList
    collections: CollectionList
    workflows: WorkflowList
    visible_entry_count: Integer

    // Entry-kind fields
    entry_kind: String
    addressed_contacts: ContactList

    // Bundle-kind fields
    app_bundle_url: String
}

list PostingList {
    member: Posting
}

/// UpdatesChannel — streaming channel for a box
structure UpdatesChannel {
    signed_stream_name: String
}

list UpdatesChannelList {
    member: UpdatesChannel
}

/// Box — a HEY mailbox
structure Box {
    @required
    id: Long

    @required
    kind: String

    @required
    name: String

    app_url: String
    url: String
    signed_stream_name: String
    posting_changes_url: String
    updates_channels: UpdatesChannelList
}

list BoxList {
    member: Box
}

/// BoxShowResponse — box detail with postings.
/// The API can return fields at root level or nested under a `box` key.
/// SDK response decoders normalize the nested variant to flat before decoding.
structure BoxShowResponse {
    @required
    id: Long

    @required
    kind: String

    @required
    name: String

    app_url: String
    url: String
    signed_stream_name: String
    posting_changes_url: String
    updates_channels: UpdatesChannelList

    next_history_url: String
    next_incremental_sync_url: String
    postings: PostingList
}

/// Topic detail
structure Topic {
    @required
    id: Long

    name: String
    created_at: DateTime
    updated_at: DateTime
    active_at: DateTime
    status: String
    account_id: Long
    app_url: String
    creator: Contact
    contacts: ContactList
    extenzions: ExtenzionList
    collections: CollectionList
    is_forged_sender: Boolean
    latest_entry: Entry

    /// The topic's first page of entries (summaries, no bodies). Present on GetTopic; use
    /// GetTopicEntries for the rest and GetMessage for a body.
    entries: EntryList
}

list TopicList {
    member: Topic
}

/// TopicListResponse — wrapped topic list (sent, spam, trash, everything)
structure TopicListResponse {
    title: String
    description: String
    topics: TopicList
}

/// Addressed recipients
structure Addressed {
    directly: ContactList
    copied: ContactList
    blindcopied: ContactList
}

/// MessagePostingContext — posting context for a message
structure MessagePostingContext {
    box: String
}

/// AddressedSender — sender context
structure AddressedSender {
    directly: ContactList
}

/// Message — full message detail
structure Message {
    @required
    id: Long

    created_at: DateTime
    updated_at: DateTime
    url: String
    creator: Contact
    sender: Contact
    is_reply: Boolean
    subject: String
    content: String
    addressed: Addressed
    show_addressed_selector: Boolean
    scheduled_delivery_at: DateTime
    posting: MessagePostingContext
    addressed_sender: AddressedSender
}

/// DraftMessage — a draft entry
structure DraftMessage {
    @required
    id: Long

    subject: String
    updated_at: DateTime
    creator: Contact
    account_id: Long
    summary: String
    url: String
    app_url: String
    edit_url: String
    addressed_contacts: ContactList
    scheduled_delivery_at: DateTime
}

list DraftMessageList {
    member: DraftMessage
}

/// Calendar
structure Calendar {
    @required
    id: Long

    name: String
    kind: String
    created_at: DateTime
    updated_at: DateTime
    owned: Boolean
    color: String
    personal: Boolean
    external: Boolean
    url: String
    recordings_url: String
    occurrences_url: String
    owner_email_address: String
}

list CalendarList {
    member: Calendar
}

/// CalendarWithRecordingChangesUrl — wraps calendar with sync URL
structure CalendarWithRecordingChangesUrl {
    calendar: Calendar
    recording_changes_url: String
}

list CalendarWithRecordingChangesUrlList {
    member: CalendarWithRecordingChangesUrl
}

/// CalendarListPayload
structure CalendarListPayload {
    calendars: CalendarWithRecordingChangesUrlList
    calendar_changes_url: String

    /// The calendars every period read is drawn from. ToggleCalendar changes this and
    /// answers the new one, so a client reads it here once — to open on what is already
    /// on — and takes it from the toggle after that.
    selected_calendar_ids: CalendarIdList
}

/// RecurrenceSchedule
structure RecurrenceSchedule {
    kind: String
    description: String
    preset: Boolean
}

/// Reminder
structure Reminder {
    @required
    id: Long

    summary: String
    duration: Integer
    default_duration: Boolean
    iso8601_duration: String
    delivered: Boolean
    remind_at: DateTime
    created_at: DateTime
    updated_at: DateTime
    label: String
}

list ReminderList {
    member: Reminder
}

/// Attendance — calendar event attendee
structure Attendance {
    @required
    id: Long

    email_address: String
    status: String
    name: String
}

list AttendanceList {
    member: Attendance
}

/// Organizer — calendar event organizer
structure Organizer {
    email_address: String
    name: String
}

/// JoinLink — video/meeting join link
structure JoinLink {
    title: String
    url: String
}

/// AttachedEntry — entry reference on a calendar event
structure AttachedEntry {
    @required
    id: Long

    kind: String
    title: String
    app_url: String
}

/// Recording — polymorphic by `type` (Calendar::Event, Calendar::Todo, etc.)
@heyPolymorphic(
    discriminator: "type"
    variants: {
        "Calendar::Event": ["edit_url", "summary", "url", "location",
                            "manage_attendance", "attendance_status", "organizer",
                            "attendances", "attendances_summary", "description",
                            "join_link", "attached_entry"]
        "Calendar::Todo": ["position"]
        "Calendar::JournalEntry": ["content"]
        "Calendar::Habit": ["color", "icon", "days", "icon_url", "stopped_at"]
        "Calendar::TimeTrack": ["notes", "category"]
        "Calendar::Countdown": ["label"]
        "Calendar::DayBackground": ["image_url"]
    }
)
structure Recording {
    @required
    id: Long

    parent_id: Long
    title: String
    all_day: Boolean
    recurring: Boolean
    starts_at: DateTime
    ends_at: DateTime
    created_at: DateTime
    updated_at: DateTime

    /// Discriminator — the recordable's Ruby class name: Calendar::Event, Calendar::Todo,
    /// Calendar::JournalEntry, Calendar::Habit, Calendar::TimeTrack, Calendar::Countdown,
    /// Calendar::DayBackground, Calendar::DayTitle or Calendar::Habit::Completion.
    @required
    type: String

    parent: Recording
    starts_at_time_zone: String
    ends_at_time_zone: String
    reminders_label: String
    reminders: ReminderList
    completed_at: DateTime
    highlighted: Boolean
    recurrence_schedule: RecurrenceSchedule
    occurrences_url: String
    occurrence_id: String
    calendar: Calendar

    // CalendarEvent fields
    edit_url: String
    summary: String
    url: String
    location: String
    manage_attendance: Boolean
    attendance_status: String
    organizer: Organizer
    attendances: AttendanceList
    attendances_summary: String
    description: String
    join_link: JoinLink
    attached_entry: AttachedEntry

    // CalendarTodo fields
    position: Integer

    // CalendarJournalEntry fields
    content: String
    /// Full rich-text HTML of a journal entry (GetJournalEntry / UpdateJournalEntry only;
    /// listings carry a truncated plain-text `content` instead).
    content_html: String

    // CalendarHabit fields
    color: String
    icon: String
    days: DaysList
    icon_url: String
    stopped_at: DateTime

    // CalendarTimeTrack fields
    notes: String
    category: String

    // CalendarCountdown fields
    label: String

    // CalendarDayBackground fields
    image_url: String
}

list DaysList {
    member: Integer
}

list RecordingList {
    member: Recording
}

/// CalendarRecordingsResponse — recordings grouped by type
map CalendarRecordingsResponse {
    key: String
    value: RecordingList
}

/// CalendarPeriod — a day or a week: its bounds and everything in it, grouped by type.
/// Recurring events arrive expanded into the occurrences that fall inside the window,
/// which is what makes this a different answer than the recordings a calendar lists.
structure CalendarPeriod {
    @required
    starts_at: DateTime

    @required
    ends_at: DateTime

    /// "day" or "week"
    @required
    kind: String

    @required
    recordings: CalendarRecordingsResponse
}

list CalendarPeriodList {
    member: CalendarPeriod
}

structure CalendarDayListPayload {
    @required
    days: CalendarPeriodList
}

structure CalendarWeekListPayload {
    @required
    weeks: CalendarPeriodList
}

/// CalendarYear — the grid a year is drawn as. A year carries one entry per day plus the
/// events that span more than one, not every recording it holds: a year's worth of
/// expanded occurrences is not something a client asks for by opening a year.
structure CalendarYear {
    @required
    starts_at: DateTime

    @required
    ends_at: DateTime

    /// "year"
    @required
    kind: String

    /// Days between the reader's week start and January 1st, so the grid lines up
    @required
    padding_days_count: Integer

    @required
    days: CalendarYearDayList

    /// All-day and multi-day events, oldest first
    @required
    spanned_events: RecordingList
}

structure CalendarYearDay {
    @required
    starts_at: DateTime

    @required
    backgrounded: Boolean
}

list CalendarYearDayList {
    member: CalendarYearDay
}

list CalendarIdList {
    member: Long
}

/// CalendarSelection — the calendars a toggle left switched on
structure CalendarSelection {
    @required
    selected_calendar_ids: CalendarIdList
}

/// NavigationIcon
structure NavigationIcon {
    name: String
    android_url: String
    ios_url: String
}

/// NavigationItem
structure NavigationItem {
    title: String
    app_url: String
    platform: String
    hotkey: String
    highlighted: Boolean
    icon: NavigationIcon
    menu_items: NavigationItemList
}

list NavigationItemList {
    member: NavigationItem
}

/// NavigationResponse
structure NavigationResponse {
    items: NavigationItemList
    hotkeys: NavigationItemList
}

/// SearchFilterItem — one option offered by the advanced search refine form
structure SearchFilterItem {
    title: String
    value: String
}

list SearchFilterItemList {
    member: SearchFilterItem
}

/// AdvancedSearchFilters — the options the advanced search refine form offers
structure AdvancedSearchFilters {
    refine_in: SearchFilterItemList
    refine_dates: SearchFilterItemList
    refine_labels: SearchFilterItemList
    refine_attachments: SearchFilterItemList
}

// =============================================================================
// IDENTITY OPERATIONS
// =============================================================================

/// Get the current identity (authenticated user profile)
@readonly
@http(method: "GET", uri: "/identity.json")
@tags(["Identity"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetIdentity {
    output: GetIdentityOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetIdentityOutput {
    @required
    identity: Identity
}

structure Identity {
    @required
    id: Long

    name: String
    avatar_url: String
    icon_url: String
    time_zone: String
    time_zone_name: String
    time_zone_offset: Integer
    auto_time_zone: Boolean
    first_week_day: Integer
    time_format: String
    primary_contact: Contact
    all_users: UserList
    accounts: AccountList
    senders: SenderList
}

/// Get navigation items
@readonly
@http(method: "GET", uri: "/my/navigation.json")
@tags(["Identity"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetNavigation {
    output: GetNavigationOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetNavigationOutput {
    @required
    navigation: NavigationResponse
}

/// Set which day the identity's calendar weeks start on. Answers the stored
/// preference. The write reaches every HEY client — web, mobile and this SDK
/// read the same identity preference.
@idempotent
@http(method: "PUT", uri: "/calendar/identity/first_week_day")
@tags(["Identity"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateFirstWeekDay {
    input: UpdateFirstWeekDayInput
    output: UpdateFirstWeekDayOutput
    errors: [UnauthorizedError, BadRequestError, InternalServerError, ServiceUnavailableError]
}

structure UpdateFirstWeekDayInput {
    @httpPayload
    @required
    body: UpdateFirstWeekDayRequestContent
}

/// Wire format: {identity_preference: {first_week_day: "monday"}}
structure UpdateFirstWeekDayRequestContent {
    @required
    identity_preference: FirstWeekDayParams
}

structure FirstWeekDayParams {
    /// Lowercase day name, sunday through saturday.
    @required
    first_week_day: String
}

structure UpdateFirstWeekDayOutput {
    @required
    preference: FirstWeekDayPreference
}

structure FirstWeekDayPreference {
    /// 0 is Sunday through 6 Saturday, as GetIdentity serves it.
    @required
    first_week_day: Integer
}

/// Set whether HEY renders times on a 12-hour or a 24-hour clock. Answers the
/// stored preference. The parameter is the web toggle's, said honestly: true
/// for the 24-hour clock, false for the 12-hour one.
@idempotent
@http(method: "PUT", uri: "/identity/time_format")
@tags(["Identity"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateTimeFormat {
    input: UpdateTimeFormatInput
    output: UpdateTimeFormatOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure UpdateTimeFormatInput {
    @httpPayload
    @required
    body: UpdateTimeFormatRequestContent
}

structure UpdateTimeFormatRequestContent {
    @required
    twenty_four_hour_time_format: Boolean
}

structure UpdateTimeFormatOutput {
    @required
    preference: TimeFormatPreference
}

structure TimeFormatPreference {
    /// "twelve_hour" or "twenty_four_hour", as GetIdentity serves it.
    @required
    time_format: String
}

// =============================================================================
// BOX OPERATIONS
// =============================================================================

/// List all boxes
@readonly
@http(method: "GET", uri: "/boxes.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation ListBoxes {
    output: ListBoxesOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListBoxesOutput {
    @required
    boxes: BoxList
}

/// Get a specific box
@readonly
@http(method: "GET", uri: "/boxes/{boxId}")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetBox {
    input: GetBoxInput
    output: GetBoxOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetBoxInput {
    @httpLabel
    @required
    boxId: Long

    @httpQuery("page")
    page: String
}

structure GetBoxOutput {
    @required
    box: BoxShowResponse
}

/// Get the Imbox
@readonly
@http(method: "GET", uri: "/imbox.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetImbox {
    input: GetNamedBoxInput
    output: GetNamedBoxOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetNamedBoxInput {
    @httpQuery("page")
    page: String
}

structure GetNamedBoxOutput {
    @required
    box: BoxShowResponse
}

/// Get the Imbox's Previously Seen postings
@readonly
@http(method: "GET", uri: "/imbox/seen.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetImboxSeen {
    input: GetNamedBoxInput
    output: GetImboxSeenOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetImboxSeenOutput {
    @required
    box: BoxShowResponse
}

/// Get the Feed
@readonly
@http(method: "GET", uri: "/feedbox.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetFeedbox {
    input: GetNamedBoxInput
    output: GetFeedboxOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetFeedboxOutput {
    @required
    box: BoxShowResponse
}

/// Get the Paper Trail
@readonly
@http(method: "GET", uri: "/paper_trail.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetTrailbox {
    input: GetNamedBoxInput
    output: GetTrailboxOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetTrailboxOutput {
    @required
    box: BoxShowResponse
}

/// Get the Set Aside box
@readonly
@http(method: "GET", uri: "/set_aside.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetAsidebox {
    input: GetNamedBoxInput
    output: GetAsideboxOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetAsideboxOutput {
    @required
    box: BoxShowResponse
}

/// Get the Reply Later box
@readonly
@http(method: "GET", uri: "/reply_later.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetLaterbox {
    input: GetNamedBoxInput
    output: GetLaterboxOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetLaterboxOutput {
    @required
    box: BoxShowResponse
}

/// Get the Bubble Up box
@readonly
@http(method: "GET", uri: "/bubble_up.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetBubblebox {
    input: GetNamedBoxInput
    output: GetBubbleboxOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetBubbleboxOutput {
    @required
    box: BoxShowResponse
}

// =============================================================================
// TOPIC OPERATIONS
// =============================================================================

/// Get a topic
@readonly
@http(method: "GET", uri: "/topics/{topicId}")
@tags(["Topics"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetTopic {
    input: GetTopicInput
    output: GetTopicOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetTopicInput {
    @httpLabel
    @required
    topicId: Long
}

structure GetTopicOutput {
    @required
    topic: Topic
}

/// Get entries for a topic
@readonly
@http(method: "GET", uri: "/topics/{topicId}/entries")
@tags(["Topics"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetTopicEntries {
    input: GetTopicEntriesInput
    output: GetTopicEntriesOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetTopicEntriesInput {
    @httpLabel
    @required
    topicId: Long

    @httpQuery("page")
    page: String
}

structure GetTopicEntriesOutput {
    @required
    entries: EntryList
}

/// Get sent topics
@readonly
@http(method: "GET", uri: "/topics/sent.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetSentTopics {
    input: PagedInput
    output: GetSentTopicsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure PagedInput {
    @httpQuery("page")
    page: String
}

structure GetSentTopicsOutput {
    @required
    response: TopicListResponse
}

/// Get spam topics
@readonly
@http(method: "GET", uri: "/topics/spam.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetSpamTopics {
    input: PagedInput
    output: GetSpamTopicsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetSpamTopicsOutput {
    @required
    response: TopicListResponse
}

/// Get trash topics
@readonly
@http(method: "GET", uri: "/topics/trash.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetTrashTopics {
    input: PagedInput
    output: GetTrashTopicsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetTrashTopicsOutput {
    @required
    response: TopicListResponse
}

/// Get all topics (everything view)
@readonly
@http(method: "GET", uri: "/topics/everything.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetEverythingTopics {
    input: PagedInput
    output: GetEverythingTopicsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetEverythingTopicsOutput {
    @required
    response: TopicListResponse
}

// =============================================================================
// MESSAGE OPERATIONS
// =============================================================================

/// Get a message
@readonly
@http(method: "GET", uri: "/messages/{messageId}")
@tags(["Messages"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetMessage {
    input: GetMessageInput
    output: GetMessageOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetMessageInput {
    @httpLabel
    @required
    messageId: Long
}

structure GetMessageOutput {
    @required
    message: Message
}

/// Create a new message (start a new topic).
/// The acting sender ID must be included; the Go SDK resolves this automatically.
/// Every message is created drafted on HEY's side; without entry.status the server
/// delivers it, while entry.status "drafted" leaves it as a draft and answers
/// 204 with a Location header naming /messages/{entry_id}.
@http(method: "POST", uri: "/messages.json")
@tags(["Messages"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateMessage {
    input: CreateMessageInput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateMessageInput {
    @httpPayload
    @required
    body: CreateMessageRequestContent
}

/// Wire format: {acting_sender_id, message: {subject, content}, entry: {addressed: {directly: "..."}}}
structure CreateMessageRequestContent {
    @required
    acting_sender_id: Long

    @required
    message: MessagePayload

    entry: MessageEntryPayload
}

structure MessagePayload {
    @required
    subject: String

    @required
    content: String
}

/// Revise a message entry (MessagesController#update). With entry.status "drafted" the
/// entry is saved as a draft (204 + Location, like CreateMessage); without it a draft is
/// delivered through the undo-delay window. A trashed draft is silently restored first.
/// The revision is not a patch: subject, content and any scheduled delivery are rewritten
/// from this request (an omitted scheduled delivery clears one), while recipients are
/// replaced only when entry.addressed is present.
///
/// Not naturally idempotent despite the PUT: without the drafted status this request
/// *delivers*, so a transparent retry after an ambiguous first attempt could send the
/// message again. The client must not retry it.
@http(method: "PUT", uri: "/messages/{messageId}")
@tags(["Messages"])
@heyIdempotent(natural: false)
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateMessage {
    input: UpdateMessageInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateMessageInput {
    @httpLabel
    @required
    messageId: Long

    @httpPayload
    @required
    body: CreateMessageRequestContent
}

/// A draft's editable state: content, recipients and scheduled delivery as the
/// composer would load them (GET /messages/{id}/edit).
@readonly
@http(method: "GET", uri: "/messages/{messageId}/edit.json")
@tags(["Messages"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetMessageEdit {
    input: GetMessageEditInput
    output: GetMessageEditOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetMessageEditInput {
    @httpLabel
    @required
    messageId: Long
}

structure GetMessageEditOutput {
    @required
    message: MessageEditState
}

/// MessageEditState — a saved draft as the editor sees it. The same compose fields as
/// MessageDraft, plus the identity and scheduling a saved entry carries.
structure MessageEditState {
    @required
    id: Long

    created_at: DateTime
    updated_at: DateTime
    url: String
    creator: Contact
    sender: Contact
    is_reply: Boolean
    subject: String
    content: String
    addressed: Addressed
    show_addressed_selector: Boolean
    scheduled_delivery_at: DateTime
    posting: MessagePostingContext
    addressed_sender: AddressedSender
}

// =============================================================================
// ATTACHMENT OPERATIONS
// =============================================================================

/// Create an Active Storage direct upload for an outgoing attachment.
/// The returned URL is self-authenticating and accepts the raw file bytes via PUT.
@http(method: "POST", uri: "/rails/active_storage/direct_uploads.json")
@tags(["Attachments"])
operation CreateDirectUpload {
    input: CreateDirectUploadInput
    output: CreateDirectUploadOutput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateDirectUploadInput {
    @httpPayload
    @required
    body: CreateDirectUploadRequestContent
}

structure CreateDirectUploadRequestContent {
    @required
    blob: DirectUploadBlob
}

structure DirectUploadBlob {
    @required
    filename: String

    @required
    byte_size: Long

    @required
    checksum: String

    @required
    content_type: String
}

map DirectUploadHeaders {
    key: String
    value: String
}

structure DirectUploadTarget {
    @required
    url: String

    headers: DirectUploadHeaders
}

structure DirectUpload {
    @required
    signed_id: String

    @required
    attachable_sgid: String

    @required
    direct_upload: DirectUploadTarget
}

structure CreateDirectUploadOutput {
    @httpPayload
    @required
    upload: DirectUpload
}

structure MessageEntryPayload {
    addressed: MessageAddressed

    /// "drafted" saves the entry as a draft instead of delivering it. Any other value
    /// (or omitting it) delivers through the undo-delay window.
    status: String

    /// "true" schedules delivery for the date and hour below; the entry stays drafted
    /// with a scheduled_delivery_at until then. On an update, omitting it clears an
    /// existing scheduled delivery.
    scheduled_delivery: String

    /// The delivery date: YYYY-MM-DD, "today" or "tomorrow", read in the identity's
    /// time zone.
    scheduled_delivery_at_date: String

    /// The delivery hour, "0" through "23" — a string so that midnight survives
    /// omitempty. HEY schedules to the hour.
    scheduled_delivery_at_hour: String
}

/// Recipients per kind, each a list of email addresses.
/// haystack applies Array() to each kind, so a JSON array is the correct wire format
/// (a bare string would be treated as a single address, not split on commas).
structure MessageAddressed {
    directly: EmailAddressList
    copied: EmailAddressList
    blindcopied: EmailAddressList
}

list EmailAddressList {
    member: String
}

// =============================================================================
// ENTRY OPERATIONS
// =============================================================================

/// List draft messages
@readonly
@http(method: "GET", uri: "/entries/drafts.json")
@tags(["Entries"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation ListDrafts {
    input: PagedInput
    output: ListDraftsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListDraftsOutput {
    @required
    drafts: DraftMessageList
}

/// Trash a draft (Entries::DraftsController#destroy). The id is the draft's entry id,
/// as ListDrafts reports it.
@idempotent
@http(method: "DELETE", uri: "/entries/drafts/{entryId}")
@tags(["Entries"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteDraft {
    input: DeleteDraftInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteDraftInput {
    @httpLabel
    @required
    entryId: Long
}

/// Reply to an entry
@http(method: "POST", uri: "/entries/{entryId}/replies.json")
@tags(["Entries"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateReply {
    input: CreateReplyInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

/// Get a prefilled reply to an entry: the quoted body and, in addressed, the
/// participating contacts a reply goes to as HEY computes them — the sender moved onto
/// the To line and the acting user's own addresses, aliases and catch-alls excluded.
@readonly
@http(method: "GET", uri: "/entries/{entryId}/replies/new.json")
@tags(["Entries"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation NewEntryReply {
    input: EntryStatusInput
    output: NewEntryReplyOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure NewEntryReplyOutput {
    @required
    reply: MessageDraft
}

structure CreateReplyInput {
    @httpLabel
    @required
    entryId: Long

    @httpPayload
    @required
    body: CreateReplyRequestContent
}

/// Wire format: {acting_sender_id, message: {subject, content}, entry: {addressed: {directly: [...]}}}
/// entry.addressed is optional on the wire but a reply posted without it is saved as a
/// draft rather than delivered — HEY does not reply-all for the caller. Resolve the
/// thread's recipients first and always send them.
structure CreateReplyRequestContent {
    @required
    acting_sender_id: Long

    @required
    message: ReplyMessagePayload

    entry: MessageEntryPayload
}

/// HEY does not derive a subject for a reply: a reply draft saved without message.subject
/// reads "No subject" in Drafts. NewEntryReply hands back the prefilled subject ("Re: …") —
/// send it here. Content is the caller's reply body alone: the server appends the quoted
/// original at delivery (auto_quoting defaults on), so the prefill's quoted content must
/// not be echoed back.
structure ReplyMessagePayload {
    subject: String

    @required
    content: String
}

// =============================================================================
// CONTACT OPERATIONS
// =============================================================================

/// List contacts
@readonly
@http(method: "GET", uri: "/contacts.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation ListContacts {
    input: ListContactsInput
    output: ListContactsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListContactsInput {
    @httpQuery("page")
    page: String

    @httpQuery("q")
    q: String
}

structure ListContactsOutput {
    @required
    contacts: ContactList
}

/// Get a contact, with a page of the threads they are on
@readonly
@http(method: "GET", uri: "/contacts/{contactId}")
@tags(["Contacts"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetContact {
    input: GetContactInput
    output: GetContactOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetContactInput {
    @httpLabel
    @required
    contactId: Long

    @httpQuery("page")
    page: String
}

/// ContactDetail — extended contact with additional show fields
structure ContactDetail {
    @required
    id: Long
    account_id: Long
    updated_at: DateTime
    name: String
    @heySensitive(category: "pii")
    email_address: String
    avatar_url: String
    initials: String
    avatar_background_color: String
    contactable_type: String
    name_tag: String
    edit_app_url: String
    clearance: Clearance
    aliases: ContactList
    domain: Domain

    /// The heading HEY gives the thread list, e.g. "All threads with GitHub"
    entries_title: String

    /// One page of the threads this contact is on, newest first
    postings: PostingList
}

structure GetContactOutput {
    @required
    contact: ContactDetail
}

/// Add a contact. Answers the contact that was created.
@http(method: "POST", uri: "/contacts.json", code: 201)
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateContact {
    input: CreateContactInput
    output: ContactWriteOutput
    errors: [UnauthorizedError, ConflictError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateContactInput {
    @httpPayload
    @required
    body: CreateContactRequestContent
}

/// Edit a contact. HEY rewrites the whole contact, so send every field: a name,
/// address or alias left out is cleared. Answers the contact, which is not always
/// the one addressed — promoting an alias makes the alias primary.
@idempotent
@http(method: "PATCH", uri: "/contacts/{contactId}")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateContact {
    input: UpdateContactInput
    output: ContactWriteOutput
    errors: [UnauthorizedError, NotFoundError, ConflictError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateContactInput {
    @httpLabel
    @required
    contactId: Long

    @httpPayload
    @required
    body: ContactRequestContent
}

/// Hide a contact. Nothing is deleted — RevealContact brings them back.
@idempotent
@http(method: "DELETE", uri: "/contacts/{contactId}")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation HideContact {
    input: ContactActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Put a hidden contact back in the contact list
@http(method: "POST", uri: "/contacts/{contactId}/reveal.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation RevealContact {
    input: ContactActionInput
    output: ContactWriteOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Wire format: {contact: {name, email_address, alias_email_addresses: [...]}}
structure ContactRequestContent {
    @required
    contact: ContactPayload
}

/// Wire format: {acting_user_id, contact: {...}} — creating also has to say which account
/// the contact belongs to, since one identity can hold several.
structure CreateContactRequestContent {
    /// The identity's user on the account the contact should be filed under; Identity's
    /// all_users carries one per account. Left out, HEY files it under the first account.
    acting_user_id: Long

    @required
    contact: ContactPayload
}

structure ContactPayload {
    @required
    name: String

    @heySensitive(category: "pii")
    email_address: String

    /// Sending the list replaces it: an address left out stops being an alias.
    alias_email_addresses: EmailAddressList
}

/// The written contact, as the contact list renders it.
structure ContactWriteOutput {
    @required
    contact: Contact
}

/// Read the private note kept on a contact
@readonly
@http(method: "GET", uri: "/contacts/{contactId}/note.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetContactNote {
    input: ContactActionInput
    output: GetContactNoteOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetContactNoteOutput {
    @required
    note: ContactNote
}

/// A contact's private note. Empty strings when there is no note.
structure ContactNote {
    @required
    contact_id: Long

    @required
    note: String

    /// The note as editor HTML, the same markup the web hands Trix.
    @required
    note_html: String
}

/// Write the private note on a contact, replacing whatever was there
@idempotent
@http(method: "PATCH", uri: "/contacts/{contactId}/note.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateContactNote {
    input: UpdateContactNoteInput
    output: GetContactNoteOutput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateContactNoteInput {
    @httpLabel
    @required
    contactId: Long

    @httpPayload
    @required
    body: ContactNoteRequestContent
}

/// Wire format: {contact: {note: "..."}}
structure ContactNoteRequestContent {
    @required
    contact: ContactNotePayload
}

structure ContactNotePayload {
    @required
    note: String
}

/// Clear the private note on a contact
@idempotent
@http(method: "DELETE", uri: "/contacts/{contactId}/note.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteContactNote {
    input: ContactActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

// =============================================================================
// EXTENZION OPERATIONS
// =============================================================================

/// Delete an extenzion. The id is the extenzion's contact id, the one its app_url
/// carries. Answers 204; forbidden when the caller cannot edit the extenzion.
@idempotent
@http(method: "DELETE", uri: "/accounts/{accountId}/domains/extenzions/{extenzionId}")
@tags(["Extenzions"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteExtenzion {
    input: DeleteExtenzionInput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteExtenzionInput {
    @httpLabel
    @required
    accountId: Long

    @httpLabel
    @required
    extenzionId: Long
}

// =============================================================================
// CALENDAR OPERATIONS
// =============================================================================

/// List calendars
@readonly
@http(method: "GET", uri: "/calendars.json")
@tags(["Calendars"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListCalendars {
    output: ListCalendarsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListCalendarsOutput {
    @required
    response: CalendarListPayload
}

/// Get recordings for a calendar
@readonly
@http(method: "GET", uri: "/calendars/{calendarId}/recordings")
@tags(["Calendars"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "window")
operation GetCalendarRecordings {
    input: GetCalendarRecordingsInput
    output: GetCalendarRecordingsOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetCalendarRecordingsInput {
    @httpLabel
    @required
    calendarId: Long

    @httpQuery("starts_on")
    starts_on: String

    @httpQuery("ends_on")
    ends_on: String

    @httpQuery("page")
    page: String
}

structure GetCalendarRecordingsOutput {
    @required
    recordings: CalendarRecordingsResponse
}

/// Switch a calendar in or out of the reader's selection, and answer the selection it
/// left behind. The selection is what every period read is scoped to, so a toggle is how
/// a client changes which calendars a day, week or year is drawn from.
@http(method: "POST", uri: "/calendars/{calendarId}/toggle")
@tags(["Calendars"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ToggleCalendar {
    input: ToggleCalendarInput
    output: ToggleCalendarOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure ToggleCalendarInput {
    @httpLabel
    @required
    calendarId: Long
}

structure ToggleCalendarOutput {
    @required
    selection: CalendarSelection
}

// =============================================================================
// CALENDAR EVENT OPERATIONS
// =============================================================================

/// Delete a calendar event, cancelling it for every attendee. Answers 204.
@idempotent
@http(method: "DELETE", uri: "/calendar/events/{eventId}")
@tags(["Calendar Events"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteCalendarEvent {
    input: DeleteCalendarEventInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteCalendarEventInput {
    @httpLabel
    @required
    eventId: Long
}

/// Delete one day of a repeating event, or that day and every one after it.
/// Answers 204. A single day becomes an exception in the series' schedule; with
/// apply_to_future the series is truncated at the day before, or destroyed if this
/// was its first day.
@idempotent
@http(method: "DELETE", uri: "/calendar/events/{eventId}/occurrences/{occurrence}")
@tags(["Calendar Events"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteCalendarEventOccurrence {
    input: DeleteCalendarEventOccurrenceInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteCalendarEventOccurrenceInput {
    @httpLabel
    @required
    eventId: Long

    /// The day the occurrence falls on, as YYYY-MM-DD.
    @httpLabel
    @required
    occurrence: String

    /// Remove this day and every one after it. Off, only this day is removed.
    @httpQuery("apply_to_future")
    applyToFuture: Boolean
}

// =============================================================================
// CALENDAR PERIOD OPERATIONS
//
// A day, a week and a year, each scoped to the reader's calendar selection. `day`,
// `week` and `year` are dates (YYYY-MM-DD); a day also takes the literal `now`.
// =============================================================================

/// Get one day
@readonly
@http(method: "GET", uri: "/calendar/days/{day}")
@tags(["Calendar Periods"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetCalendarDay {
    input: GetCalendarDayInput
    output: GetCalendarDayOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetCalendarDayInput {
    @httpLabel
    @required
    day: String
}

structure GetCalendarDayOutput {
    @required
    day: CalendarPeriod
}

/// List the days from a date onwards. The server picks how many, so this is a window
/// rather than a page: read the next one by asking from the last day's date.
@readonly
@http(method: "GET", uri: "/calendar/days.json")
@tags(["Calendar Periods"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListCalendarDays {
    input: ListCalendarDaysInput
    output: ListCalendarDaysOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListCalendarDaysInput {
    /// Date (YYYY-MM-DD) to start from. Defaults to today.
    @httpQuery("starts_at")
    starts_at: String
}

structure ListCalendarDaysOutput {
    @required
    response: CalendarDayListPayload
}

/// Get one week
@readonly
@http(method: "GET", uri: "/calendar/weeks/{week}")
@tags(["Calendar Periods"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetCalendarWeek {
    input: GetCalendarWeekInput
    output: GetCalendarWeekOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetCalendarWeekInput {
    /// Any date in the week (YYYY-MM-DD)
    @httpLabel
    @required
    week: String
}

structure GetCalendarWeekOutput {
    @required
    week: CalendarPeriod
}

/// List the weeks around a date — nine of them, centered on it.
@readonly
@http(method: "GET", uri: "/calendar/weeks.json")
@tags(["Calendar Periods"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListCalendarWeeks {
    input: ListCalendarWeeksInput
    output: ListCalendarWeeksOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListCalendarWeeksInput {
    /// Date (YYYY-MM-DD) of the first week. Takes precedence over centered_at.
    @httpQuery("starts_at")
    starts_at: String

    /// Date (YYYY-MM-DD) to center the nine weeks on. Defaults to today.
    @httpQuery("centered_at")
    centered_at: String
}

structure ListCalendarWeeksOutput {
    @required
    response: CalendarWeekListPayload
}

/// Get one year as the grid it is drawn as
@readonly
@http(method: "GET", uri: "/calendar/years/{year}")
@tags(["Calendar Periods"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetCalendarYear {
    input: GetCalendarYearInput
    output: GetCalendarYearOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetCalendarYearInput {
    /// Any date in the year (YYYY-MM-DD)
    @httpLabel
    @required
    year: String
}

structure GetCalendarYearOutput {
    @required
    year: CalendarYear
}

// =============================================================================
// CALENDAR TODO OPERATIONS
// =============================================================================

/// Create a calendar todo
@http(method: "POST", uri: "/calendar/todos.json")
@tags(["Calendar Todos"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateCalendarTodo {
    input: CreateCalendarTodoInput
    output: CreateCalendarTodoOutput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateCalendarTodoInput {
    @httpPayload
    @required
    body: CreateCalendarTodoRequestContent
}

/// Wire format: {calendar_todo: {title, starts_at}}
structure CreateCalendarTodoRequestContent {
    @required
    calendar_todo: CalendarTodoPayload
}

structure CalendarTodoPayload {
    @required
    title: String

    /// Date string (YYYY-MM-DD). Defaults to today if omitted.
    starts_at: String
}

structure CreateCalendarTodoOutput {
    @required
    recording: Recording
}

/// Edit a calendar todo. todoId is the recording's id, and every field of the payload
/// is optional: haystack's `wrap_parameters` accepts title, focused and starts_at, and
/// changes only what is sent.
@idempotent
@http(method: "PATCH", uri: "/calendar/todos/{todoId}")
@tags(["Calendar Todos"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateCalendarTodo {
    input: UpdateCalendarTodoInput
    output: UpdateCalendarTodoOutput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateCalendarTodoInput {
    @httpLabel
    @required
    todoId: Long

    @httpPayload
    @required
    body: UpdateCalendarTodoRequestContent
}

/// Wire format: {calendar_todo: {title, starts_at, focused}}
structure UpdateCalendarTodoRequestContent {
    @required
    calendar_todo: CalendarTodoChanges
}

/// Nothing here is required: a rename sends a title and leaves the day alone.
structure CalendarTodoChanges {
    title: String

    /// Date string (YYYY-MM-DD). The day the todo is filed on.
    starts_at: String

    focused: Boolean
}

/// The edited todo as a recording (haystack renders calendar/recordings/_recording).
structure UpdateCalendarTodoOutput {
    @required
    recording: Recording
}

/// Complete a calendar todo
@http(method: "POST", uri: "/calendar/todos/{todoId}/completions")
@tags(["Calendar Todos"])
@heyIdempotent(natural: true)
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CompleteCalendarTodo {
    input: CalendarTodoCompletionInput
    output: CalendarTodoCompletionOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure CalendarTodoCompletionInput {
    @httpLabel
    @required
    todoId: Long
}

structure CalendarTodoCompletionOutput {
    @required
    recording: Recording
}

/// Uncomplete a calendar todo
@idempotent
@http(method: "DELETE", uri: "/calendar/todos/{todoId}/completions")
@tags(["Calendar Todos"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UncompleteCalendarTodo {
    input: CalendarTodoCompletionInput
    output: CalendarTodoCompletionOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Delete a calendar todo
@idempotent
@http(method: "DELETE", uri: "/calendar/todos/{todoId}")
@tags(["Calendar Todos"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteCalendarTodo {
    input: DeleteCalendarTodoInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteCalendarTodoInput {
    @httpLabel
    @required
    todoId: Long
}

// =============================================================================
// CALENDAR HABIT OPERATIONS
// =============================================================================

/// Complete a habit for a day
@http(method: "POST", uri: "/calendar/days/{day}/habits/{habitId}/completions")
@tags(["Calendar Habits"])
@heyIdempotent(natural: true)
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CompleteHabit {
    input: HabitCompletionInput
    output: HabitCompletionOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure HabitCompletionInput {
    @httpLabel
    @required
    day: String

    @httpLabel
    @required
    habitId: Long
}

structure HabitCompletionOutput {
    @required
    recording: Recording
}

/// Uncomplete a habit for a day
@idempotent
@http(method: "DELETE", uri: "/calendar/days/{day}/habits/{habitId}/completions")
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UncompleteHabit {
    input: HabitCompletionInput
    output: HabitCompletionOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

// =============================================================================
// CALENDAR TIME TRACK OPERATIONS
// =============================================================================

/// List tracked time — completed tracks only, newest-ended first.
///
/// A running track is not here; read that with GetOngoingTimeTrack. The next page, if
/// any, is a Link header, and the last page carries none, so a nil Link is the end of
/// the list rather than an error.
///
/// category_id narrows the list to one category and 404s if the calendar has no
/// category by that id.
///
/// The calendar's categories come back alongside the tracks, so showing or applying the
/// filter does not need ListTimeTrackCategories as well.
@readonly
@http(method: "GET", uri: "/calendar/time_tracks.json")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link")
operation ListTimeTracks {
    input: ListTimeTracksInput
    output: ListTimeTracksOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure ListTimeTracksInput {
    @httpQuery("page")
    page: String

    @httpQuery("category_id")
    category_id: Long
}

structure ListTimeTracksOutput {
    @required
    tracked_time: TrackedTime
}

/// The tracked-time index: a page of completed tracks, and every category they can be
/// filed under.
structure TrackedTime {
    time_tracks: RecordingList
    categories: TimeTrackCategoryList
}

/// Get the ongoing time track (404 = no active track; see ADR-004)
@readonly
@http(method: "GET", uri: "/calendar/ongoing_time_track.json")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyEmptyOn(statusCodes: [404])
operation GetOngoingTimeTrack {
    output: GetOngoingTimeTrackOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetOngoingTimeTrackOutput {
    @required
    recording: Recording
}

/// Start a new time track. Takes no body: haystack's
/// Calendar::OngoingTimeTracksController#create ignores request parameters and
/// starts a track with defaults; use UpdateTimeTrack to set notes and category_title,
/// which also stops the track.
@http(method: "POST", uri: "/calendar/ongoing_time_track.json")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation StartTimeTrack {
    output: StartTimeTrackOutput
    errors: [UnauthorizedError, ConflictError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure StartTimeTrackOutput {
    @required
    recording: Recording
}

/// Update a time track (stop by setting ends_at to current time).
///
/// Every update completes the track, whether or not ends_at is sent, so this cannot
/// be used to adjust a running track: it stops it.
///
/// Only the fields sent are written, so a partial update leaves the rest of the track
/// alone. A starts_at or ends_at the server cannot parse is a 400, not a 422.
@idempotent
// NOTE: The live path is /calendar/time_tracks/{id}.json, but Smithy forbids a
// literal after a label inside one segment. The generated client appends .json to
// extension-less paths at request time (see initGeneratedClient), so URIs that end
// in a label are written without it here.
@http(method: "PUT", uri: "/calendar/time_tracks/{timeTrackId}")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateTimeTrack {
    input: UpdateTimeTrackInput
    output: UpdateTimeTrackOutput
    errors: [UnauthorizedError, BadRequestError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateTimeTrackInput {
    @httpLabel
    @required
    timeTrackId: Long

    @httpPayload
    @required
    body: UpdateTimeTrackRequestContent
}

/// Wire format: {calendar_time_track: {notes, category_title, starts_at, ends_at}}
structure UpdateTimeTrackRequestContent {
    @required
    calendar_time_track: UpdateTimeTrackPayload
}

structure UpdateTimeTrackPayload {
    /// Ignored by the server. A time track's title is the constant "Time Track";
    /// HEY dropped per-track titles in 2023. Kept for compatibility only.
    title: String

    notes: String

    /// Ignored by the server, which reads category_title instead. Kept for
    /// compatibility only.
    category: String

    /// Files the track under this category, creating the category if HEY does not
    /// have one by that name. Blank is a no-op, not a way to clear the category:
    /// once filed, a track can only be moved to another category, or left where it
    /// is by deleting the category itself.
    category_title: String

    starts_at: DateTime
    ends_at: DateTime
}

structure UpdateTimeTrackOutput {
    @required
    recording: Recording
}

// =============================================================================
// CALENDAR JOURNAL OPERATIONS
// =============================================================================

/// List journal entries newest first. The next page, if any, is a Link header.
/// Pass q to search journal entry content.
@readonly
@http(method: "GET", uri: "/calendar/journal_entries")
@tags(["Calendar Journal"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link")
operation ListJournalEntries {
    input: ListJournalEntriesInput
    output: ListJournalEntriesOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListJournalEntriesInput {
    @httpQuery("page")
    page: String

    @httpQuery("q")
    q: String
}

structure ListJournalEntriesOutput {
    @required
    entries: RecordingList
}

/// Get journal entry for a day
@readonly
@http(method: "GET", uri: "/calendar/days/{day}/journal_entry")
@tags(["Calendar Journal"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetJournalEntry {
    input: JournalEntryInput
    output: GetJournalEntryOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure JournalEntryInput {
    @httpLabel
    @required
    day: String
}

structure GetJournalEntryOutput {
    @required
    recording: Recording
}

/// Update the journal entry for a day: writes it, creating it if the day has none, and
/// answers the entry as a recording. Empty content removes the entry instead, and HEY then
/// answers 204 with no body — which is not this shape, so send that through the SDK's own
/// journal wrapper rather than here.
@http(method: "PATCH", uri: "/calendar/days/{day}/journal_entry")
@tags(["Calendar Journal"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateJournalEntry {
    input: UpdateJournalEntryInput
    output: UpdateJournalEntryOutput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateJournalEntryOutput {
    @required
    recording: Recording
}

structure UpdateJournalEntryInput {
    @httpLabel
    @required
    day: String

    @httpPayload
    @required
    body: UpdateJournalEntryRequestContent
}

/// Wire format: {calendar_journal_entry: {content}}
structure UpdateJournalEntryRequestContent {
    @required
    calendar_journal_entry: JournalEntryPayload
}

structure JournalEntryPayload {
    @required
    content: String
}

// =============================================================================
// SEARCH OPERATIONS
// =============================================================================

/// Get the options the advanced search refine form offers.
///
/// Advanced search: message matches grouped by topic as the search page shows them —
/// the topic, its posting id, and the matching entries as summaries (no bodies; read a
/// message with GetMessage). Refinements are the same query parameters the page uses.
/// The next page, if any, is a Link header.
@readonly
@http(method: "GET", uri: "/advanced_search.json")
@tags(["Search"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link")
operation AdvancedSearch {
    input: AdvancedSearchInput
    output: AdvancedSearchOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure AdvancedSearchInput {
    /// The words to search for
    @httpQuery("q")
    q: String

    @httpQuery("page")
    page: String

    /// Refinements, e.g. refine[from], refine[to], refine[subject], refine[exact_phrase],
    /// refine[required], refine[any], refine[none], refine[date], refine[in], refine[label],
    /// refine[attachment] — passed through as the page sends them.
    @httpQuery("refine[from]")
    from: String

    @httpQuery("refine[to]")
    to: String

    @httpQuery("refine[subject]")
    subject: String

    @httpQuery("refine[exact_phrase]")
    exact_phrase: String

    @httpQuery("refine[required]")
    required: String

    @httpQuery("refine[any]")
    any: String

    @httpQuery("refine[none]")
    none: String

    @httpQuery("refine[date]")
    date: String

    @httpQuery("refine[in]")
    in: String

    @httpQuery("refine[label]")
    label: String

    @httpQuery("refine[attachment]")
    attachment: String
}

structure AdvancedSearchOutput {
    @required
    result: AdvancedSearchResult
}

structure AdvancedSearchResult {
    @required
    matches: SearchMatchList
}

/// One matching topic: the topic, your posting of it (if any), and the entries that matched.
structure SearchMatch {
    @required
    topic: Topic
    posting_id: Long
    entries: EntryList
}

list SearchMatchList {
    member: SearchMatch
}

/// The advanced search refine form's options: boxes, date ranges, labels and attachment kinds.
@readonly
@http(method: "GET", uri: "/advanced_search_filters.json")
@tags(["Search"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetAdvancedSearchFilters {
    output: GetAdvancedSearchFiltersOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetAdvancedSearchFiltersOutput {
    @required
    filters: AdvancedSearchFilters
}

// =============================================================================
// POSTINGS — mark seen/unseen, move between boxes, trash, mute (all bulk)
// =============================================================================

/// Mark postings as seen
@http(method: "POST", uri: "/postings/seen.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MarkPostingsSeen {
    input: MarkPostingsInput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

/// Mark postings as unseen
@http(method: "POST", uri: "/postings/unseen.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MarkPostingsUnseen {
    input: MarkPostingsInput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure MarkPostingsInput {
    @httpPayload
    @required
    body: MarkPostingsRequestContent
}

structure MarkPostingsRequestContent {
    @required
    posting_ids: PostingIdList
}

/// Move postings to a box (bulk).
/// Mirrors HEY's Postings::MovesController: `posting_ids` plus the target `box_id`
/// (an ID from ListBoxes; the box `kind` field identifies imbox, feedbox, asidebox,
/// laterbox, trailbox). Responds 204 No Content.
@http(method: "POST", uri: "/postings/moves.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MovePostings {
    input: MovePostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure MovePostingsInput {
    @httpPayload
    @required
    body: MovePostingsRequestContent
}

structure MovePostingsRequestContent {
    @required
    posting_ids: PostingIdList

    @required
    box_id: Long
}

/// Move postings to the trash (bulk).
/// Mirrors HEY's Postings::TrashController. For JSON requests the server treats
/// the removal decision as made (shared topics: your access is removed).
/// Responds 204 No Content.
@http(method: "POST", uri: "/postings/trash.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation TrashPostings {
    input: TrashPostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure TrashPostingsInput {
    @httpPayload
    @required
    body: TrashPostingsRequestContent
}

structure TrashPostingsRequestContent {
    @required
    posting_ids: PostingIdList

    /// Omitted, JSON requests default to removing only your own access from shared topics.
    /// "false" trashes them for everyone instead.
    remove_access: String
}

/// Mute postings (bulk) — stop notifications for their threads.
/// Mirrors HEY's Postings::MutingsController#create. Responds 201 Created.
@http(method: "POST", uri: "/postings/mutings.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MutePostings {
    input: MarkPostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Unmute postings (bulk).
/// Mirrors HEY's Postings::MutingsController#destroy. `posting_ids` is sent as a
/// comma-separated query string because DELETE carries no body. Responds 201 Created.
@idempotent
@http(method: "DELETE", uri: "/postings/mutings.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UnmutePostings {
    input: UnmutePostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure UnmutePostingsInput {
    /// Comma-separated posting IDs, e.g. "123,456"
    @httpQuery("posting_ids")
    @required
    posting_ids: String
}

// =============================================================================
// SHARED SHAPES — added for the 1.0 coverage pass
// =============================================================================

/// Sticky — a note on the stickies board
structure Sticky {
    @required
    id: Long

    body: String
    size: String
    created_at: DateTime
    updated_at: DateTime
}

list StickyList {
    member: Sticky
}

/// BoxGroup — a Set Aside group. The API only ever returns the id.
structure BoxGroup {
    @required
    id: Long
}

list BoxGroupList {
    member: BoxGroup
}

/// BoxGroupsResponse — the wrapper the groups index answers with
structure BoxGroupsResponse {
    box_groups: BoxGroupList
}

/// BoxGroupWithPostings — a Set Aside group with one page of the postings in it
structure BoxGroupWithPostings {
    @required
    id: Long

    box_id: Long
    postings: PostingList
}

/// FolderWithPostings — folder detail with the postings filed in it
structure FolderWithPostings {
    @required
    id: Long

    name: String
    created_at: DateTime
    updated_at: DateTime
    app_url: String
    postings: PostingList
}

/// ClearanceSummary — the Screener's pending count, and the queue itself when asked for
///
/// clearances is only present when the read passes include_clearances. Without it HEY
/// answers the count alone, which is what its own apps sync.
structure ClearanceSummary {
    pending_clearances_count: Integer
    signed_stream_name: String
    clearances: ClearanceList
}

/// MessageDraft — a prefilled compose payload (forward, reply). Unsent, so it has no id.
structure MessageDraft {
    url: String
    creator: Contact
    sender: Contact
    is_reply: Boolean
    subject: String
    content: String
    addressed: Addressed
    show_addressed_selector: Boolean
    posting: MessagePostingContext
    addressed_sender: AddressedSender
}

/// Posting ids as a comma-joined string, for verbs that carry no body
string PostingIdsParam

/// List the unseen postings inside a bundle posting.
///
/// A bundle posting groups one contact's unseen mail; this is its contents — the member
/// postings, newest first, paged by cursor like a box. The posting must be a bundle.
@readonly
@http(method: "GET", uri: "/postings/{postingId}/bundles/unseen.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetBundleUnseenPostings {
    input: GetBundleUnseenPostingsInput
    output: GetBundleUnseenPostingsOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetBundleUnseenPostingsInput {
    @httpLabel
    @required
    postingId: Long

    @httpQuery("page")
    page: String
}

structure GetBundleUnseenPostingsOutput {
    @required
    contact: Contact

    @required
    postings: PostingList
}

// =============================================================================
// POSTINGS — bulk actions across a selection
// =============================================================================

/// Mark a selection of postings as spam.
///
/// Over ten postings the server hands the work to a background job, so the effect is
/// eventually consistent.
@http(method: "POST", uri: "/postings/spam.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MarkPostingsSpam {
    input: MarkPostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// A posting selection carried in the query string, for verbs that send no body
structure PostingSelectionInput {
    @httpQuery("posting_ids")
    @required
    posting_ids: PostingIdsParam
}

/// Add a selection of postings to a Set Aside group
@http(method: "POST", uri: "/postings/box_groups.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation AddPostingsToBoxGroup {
    input: AddPostingsToBoxGroupInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure AddPostingsToBoxGroupInput {
    @httpPayload
    @required
    body: AddPostingsToBoxGroupRequestContent
}

structure AddPostingsToBoxGroupRequestContent {
    @required
    posting_ids: PostingIdList

    @required
    box_id: Long

    @required
    box_group_id: Long
}

/// Remove a selection of postings from their Set Aside group
@idempotent
@http(method: "DELETE", uri: "/postings/box_groups.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation RemovePostingsFromBoxGroup {
    input: PostingSelectionInput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

/// File a selection of postings into an existing folder (label)
@http(method: "POST", uri: "/postings/filings.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation FilePostings {
    input: FilePostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure FilePostingsInput {
    @httpPayload
    @required
    body: FilePostingsRequestContent
}

structure FilePostingsRequestContent {
    @required
    posting_ids: PostingIdList

    @required
    folder_id: Long
}

/// Remove a selection of postings from a folder, or from every folder when folder_id is omitted
@idempotent
@http(method: "DELETE", uri: "/postings/filings.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UnfilePostings {
    input: UnfilePostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure UnfilePostingsInput {
    @httpQuery("posting_ids")
    @required
    posting_ids: PostingIdsParam

    @httpQuery("folder_id")
    folder_id: Long
}

/// Create a folder (label) and file a selection of postings into it
@http(method: "POST", uri: "/postings/folders.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateFolderForPostings {
    input: CreateFolderForPostingsInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateFolderForPostingsInput {
    @httpPayload
    @required
    body: CreateFolderForPostingsRequestContent
}

/// Wire format: {posting_ids: [...], folder: {name, status}}
structure CreateFolderForPostingsRequestContent {
    @required
    posting_ids: PostingIdList

    @required
    folder: FolderPayload
}

structure FolderPayload {
    @required
    name: String

    status: String
}

/// Cancel a scheduled bubble up for a selection of postings
@idempotent
@http(method: "DELETE", uri: "/postings/bubble_up.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CancelPostingsBubbleUp {
    input: PostingSelectionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Bubble a selection of postings up right now
@http(method: "POST", uri: "/postings/bulk_bubble_up_now.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation BubbleUpPostingsNow {
    input: MarkPostingsInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Schedule a selection of postings to bubble up.
///
/// HEY's scheduler takes a `slot` — today, tomorrow, weekend, next_week, surprise_me
/// or custom — and a custom slot also carries the `date` (YYYY-MM-DD) to bubble up on,
/// at HEY's morning hour. The today slot lands at HEY's evening hour of the current
/// day instead, and both hours are UTC over JSON. An unknown slot, or a custom slot
/// without a date, is a server error rather than a validation response, so callers
/// check both first. Responds 201 Created.
@http(method: "POST", uri: "/postings/bubble_up.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation SchedulePostingsBubbleUp {
    input: SchedulePostingsBubbleUpInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure SchedulePostingsBubbleUpInput {
    @httpPayload
    @required
    body: SchedulePostingsBubbleUpRequestContent
}

structure SchedulePostingsBubbleUpRequestContent {
    @required
    posting_ids: PostingIdList

    @required
    slot: String

    date: String
}

// =============================================================================
// TOPIC STATUS AND MOVES
// =============================================================================

/// Trash a topic.
///
/// A shared topic redirects to the removal confirmation page unless confirm_destroy is set,
/// so always pass it when trashing something that might be shared.
@idempotent
@http(method: "PUT", uri: "/topics/{topicId}/status/trashed.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation TrashTopic {
    input: TrashTopicInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure TrashTopicInput {
    @httpLabel
    @required
    topicId: Long

    @httpQuery("confirm_destroy")
    confirm_destroy: String
}

/// Restore a topic from the trash or the catch-all
@idempotent
@http(method: "PUT", uri: "/topics/{topicId}/status/active.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation RestoreTopic {
    input: TopicStatusInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Mark a spam topic as ham. Every other spam topic from the same sender is hammed too.
@idempotent
@http(method: "PUT", uri: "/topics/{topicId}/status/ham.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MarkTopicHam {
    input: TopicStatusInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure TopicStatusInput {
    @httpLabel
    @required
    topicId: Long
}

/// Empty the trash. Runs synchronously, so it can take a while on a large mailbox.
@idempotent
@http(method: "DELETE", uri: "/topics/trash/all.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation EmptyTrash {
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

/// Empty the spam box. Runs synchronously, so it can take a while on a large mailbox.
@idempotent
@http(method: "DELETE", uri: "/topics/spam/all.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation EmptySpam {
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

/// Move a topic to another box.
///
/// Answers 204 without moving anything when the acting user has no posting for the topic.
@http(method: "POST", uri: "/topics/{topicId}/moves.json")
@tags(["Topics"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MoveTopic {
    input: MoveTopicInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure MoveTopicInput {
    @httpLabel
    @required
    topicId: Long

    @httpPayload
    @required
    body: MoveTopicRequestContent
}

structure MoveTopicRequestContent {
    @required
    box_id: Long
}

// =============================================================================
// ENTRY STATUS AND FORWARDS
// =============================================================================

/// Mark an entry as spam. Denies the sender when every thread from them is already spam.
@idempotent
@http(method: "PUT", uri: "/entries/{entryId}/status/spam.json")
@tags(["Entries"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MarkEntrySpam {
    input: EntryStatusInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure EntryStatusInput {
    @httpLabel
    @required
    entryId: Long
}

/// Get a prefilled forward of an entry: subject, quoted body and blank recipients.
/// Send it with CreateMessage once the recipients are filled in.
@readonly
@http(method: "GET", uri: "/entries/{entryId}/forwards/new.json")
@tags(["Entries"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation NewEntryForward {
    input: EntryStatusInput
    output: NewEntryForwardOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure NewEntryForwardOutput {
    @required
    forward: MessageDraft
}

// =============================================================================
// BULK REPLY
// =============================================================================

/// Work out which entries a bulk reply would answer. HEY replies to the last replyable
/// entry of each thread, skipping threads with no reply address, so the postings you hold
/// are not the entries you send to — this resolves them.
@readonly
@http(method: "GET", uri: "/bulk_replies/new.json")
@tags(["Bulk Reply"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation NewBulkReply {
    input: NewBulkReplyInput
    output: NewBulkReplyOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure NewBulkReplyInput {
    /// The postings to reply to, comma separated.
    @httpQuery("posting_ids")
    @required
    posting_ids: String
}

structure NewBulkReplyOutput {
    @required
    draft: BulkReplyDraft
}

/// The reply as HEY would send it: the prefilled content and the entries it goes to.
structure BulkReplyDraft {
    /// The prefilled body — the name tag when every thread is on the same account.
    @required
    content: String

    @required
    entries: BulkReplyEntryList
}

list BulkReplyEntryList {
    member: BulkReplyEntry
}

/// One thread a bulk reply answers, with the recipients that thread's reply goes to.
structure BulkReplyEntry {
    @required
    id: Long

    @required
    topic_id: Long

    @required
    topic_name: String

    @required
    addressed: Addressed
}

/// Send one reply to every entry. Answers what was sent, not the replies themselves:
/// delivery is queued, and delayed while undo is still possible.
@http(method: "POST", uri: "/bulk_replies.json", code: 201)
@tags(["Bulk Reply"])
operation CreateBulkReply {
    input: CreateBulkReplyInput
    output: CreateBulkReplyOutput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateBulkReplyInput {
    @httpPayload
    @required
    body: BulkReplyRequestContent
}

/// Wire format: {entry_ids: [...], message: {content}}
structure BulkReplyRequestContent {
    @required
    entry_ids: PostingIdList

    @required
    message: BulkReplyMessagePayload
}

structure BulkReplyMessagePayload {
    @required
    content: String
}

structure CreateBulkReplyOutput {
    @required
    delivery: BulkReplyDelivery
}

structure BulkReplyDelivery {
    @required
    id: Long

    @required
    entries_count: Integer

    /// True while the send is held open for undo.
    @required
    delayed: Boolean

    /// Where to POST to call the replies back, present only while delayed.
    undo_send_url: String
}

// =============================================================================
// CONTACT BUNDLES AND SCREENING
// =============================================================================

/// Bundle a contact so their mail arrives grouped
@http(method: "POST", uri: "/contacts/{contactId}/bundle.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation BundleContact {
    input: ContactActionInput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Stop bundling a contact's mail
@idempotent
@http(method: "DELETE", uri: "/contacts/{contactId}/bundle.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UnbundleContact {
    input: ContactActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure ContactActionInput {
    @httpLabel
    @required
    contactId: Long
}

/// Screen a contact in or out. Status is "approved" or "denied".
@idempotent
@http(method: "PATCH", uri: "/contacts/{contactId}/clearance.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateContactClearance {
    input: UpdateContactClearanceInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateContactClearanceInput {
    @httpLabel
    @required
    contactId: Long

    @httpPayload
    @required
    body: UpdateContactClearanceRequestContent
}

/// Wire format: {status: "approved"|"denied"} — top level, not nested under a clearance key.
structure UpdateContactClearanceRequestContent {
    @required
    status: String
}

/// Get the Screener — the pending count, and the senders waiting when asked for them
@readonly
@http(method: "GET", uri: "/clearances.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetClearances {
    input: GetClearancesInput
    output: GetClearancesOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetClearancesInput {
    @httpQuery("include_clearances")
    include_clearances: Boolean

    @httpQuery("page")
    page: String
}

structure GetClearancesOutput {
    @required
    summary: ClearanceSummary
}

/// Screen a sender in or out of the Screener
///
/// designation_box_id files everything they send into that box instead of the Imbox.
/// spam marks what is already waiting as spam and trains the filter on it.
@idempotent
@http(method: "PATCH", uri: "/clearances/{clearanceId}")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateClearance {
    input: UpdateClearanceInput
    output: UpdateClearanceOutput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure UpdateClearanceInput {
    @httpLabel
    @required
    clearanceId: Long

    @httpPayload
    @required
    body: UpdateClearanceRequestContent
}

/// Wire format: {status: "approved"|"denied"} — top level, not nested under a clearance key.
structure UpdateClearanceRequestContent {
    @required
    status: String

    designation_box_id: Long
    spam: Boolean
    mark_topics_as_seen: Boolean
}

structure UpdateClearanceOutput {
    @required
    clearance: Clearance
}

/// Screen several senders out at once. ids is a comma-separated list.
@idempotent
@http(method: "PATCH", uri: "/clearances/bulk.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation BulkUpdateClearances {
    input: BulkUpdateClearancesInput
    output: BulkUpdateClearancesOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure BulkUpdateClearancesInput {
    @httpPayload
    @required
    body: BulkUpdateClearancesRequestContent
}

structure BulkUpdateClearancesRequestContent {
    @required
    ids: String

    @required
    status: String

    spam: Boolean
}

structure BulkUpdateClearancesOutput {
    @required
    response: ClearanceListResponse
}

/// Clear the Screener — every pending sender is punted and reexamined on their next email
@http(method: "POST", uri: "/clearances/punt.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation PuntClearances {
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

/// The senders already screened in or out
@readonly
@http(method: "GET", uri: "/my/clearances.json")
@tags(["Contacts"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetMyClearances {
    input: PagedInput
    output: GetMyClearancesOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure GetMyClearancesOutput {
    @required
    response: ClearanceListResponse
}

/// Rescreen a sender who was already screened in or out
@idempotent
@http(method: "PATCH", uri: "/my/clearances/{clearanceId}")
@tags(["Contacts"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateMyClearance {
    input: UpdateMyClearanceInput
    output: UpdateMyClearanceOutput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure UpdateMyClearanceInput {
    @httpLabel
    @required
    clearanceId: Long

    @httpPayload
    @required
    body: UpdateMyClearanceRequestContent
}

structure UpdateMyClearanceRequestContent {
    @required
    status: String
}

structure UpdateMyClearanceOutput {
    @required
    clearance: Clearance
}

// =============================================================================
// BOX DESIGNATIONS, GROUPS AND OBSERVATION
// =============================================================================

/// Designate a contact to a box, so everything they send lands there
@http(method: "POST", uri: "/boxes/{boxId}/designations.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateBoxDesignation {
    input: CreateBoxDesignationInput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure CreateBoxDesignationInput {
    @httpLabel
    @required
    boxId: Long

    @httpPayload
    @required
    body: CreateBoxDesignationRequestContent
}

structure CreateBoxDesignationRequestContent {
    @required
    contact_id: Long
}

/// Remove a designation from a box. The id is the designation's, not the contact's.
@idempotent
@http(method: "DELETE", uri: "/boxes/{boxId}/designations/{designationId}")
@tags(["Boxes"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteBoxDesignation {
    input: DeleteBoxDesignationInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteBoxDesignationInput {
    @httpLabel
    @required
    boxId: Long

    @httpLabel
    @required
    designationId: Long
}

/// Read what changed among a box's postings since a point in time.
///
/// This is the incremental sync feed the mail clients follow rather than re-reading a
/// box. `since` is an ISO 8601 timestamp with milliseconds and is exclusive, and `v` is
/// the client's contract version — the server answers 409 when the caller is too far
/// behind for an increment to carry the difference, which means read the box in full
/// instead. A box's own `posting_changes_url` carries the `since` and `v` to start from,
/// and the `Link` header names the next page while one remains and the next `since`
/// cursor on the last page.
@readonly
@http(method: "GET", uri: "/boxes/{boxId}/postings/changes.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetBoxPostingChanges {
    input: GetBoxPostingChangesInput
    output: GetBoxPostingChangesOutput
    errors: [UnauthorizedError, NotFoundError, ConflictError, InternalServerError, ServiceUnavailableError]
}

structure GetBoxPostingChangesInput {
    @httpLabel
    @required
    boxId: Long

    @httpQuery("since")
    @required
    since: String

    @httpQuery("v")
    v: String

    @httpQuery("page")
    page: String

    @httpQuery("per_page")
    per_page: String
}

structure GetBoxPostingChangesOutput {
    added: PostingList

    updated: PostingList

    deleted: DeletedPostingList
}

/// DeletedPosting — the stub the changes feed answers with for a posting that is gone
structure DeletedPosting {
    @required
    id: Long

    box_id: Long
    deleted_at: DateTime
}

list DeletedPostingList {
    member: DeletedPosting
}

/// List the Set Aside groups in a box
@readonly
@http(method: "GET", uri: "/boxes/{boxId}/groups.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListBoxGroups {
    input: BoxGroupsInput
    output: ListBoxGroupsOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure BoxGroupsInput {
    @httpLabel
    @required
    boxId: Long
}

structure ListBoxGroupsOutput {
    @required
    response: BoxGroupsResponse
}

/// Read one Set Aside group with the postings in it.
///
/// The postings are paged like a folder's: newest observed first, 30 to a page, with the
/// next page in the Link header and the total in X-Total-Count.
@readonly
@http(method: "GET", uri: "/boxes/{boxId}/groups/{groupId}")
@tags(["Boxes"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetBoxGroup {
    input: GetBoxGroupInput
    output: GetBoxGroupOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetBoxGroupInput {
    @httpLabel
    @required
    boxId: Long

    @httpLabel
    @required
    groupId: Long

    @httpQuery("page")
    page: String
}

structure GetBoxGroupOutput {
    @required
    group: BoxGroupWithPostings
}

/// Create a Set Aside group out of a selection of postings.
///
/// This endpoint does not split a comma-joined posting_ids string — send an array.
@http(method: "POST", uri: "/boxes/{boxId}/groups.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateBoxGroup {
    input: CreateBoxGroupInput
    output: CreateBoxGroupOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure CreateBoxGroupInput {
    @httpLabel
    @required
    boxId: Long

    @httpPayload
    @required
    body: CreateBoxGroupRequestContent
}

structure CreateBoxGroupRequestContent {
    @required
    posting_ids: PostingIdList
}

structure CreateBoxGroupOutput {
    @required
    group: BoxGroup
}

/// Break up a Set Aside group, moving its postings back to Previously Seen
@idempotent
@http(method: "DELETE", uri: "/boxes/{boxId}/groups/{groupId}")
@tags(["Boxes"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteBoxGroup {
    input: DeleteBoxGroupInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteBoxGroupInput {
    @httpLabel
    @required
    boxId: Long

    @httpLabel
    @required
    groupId: Long
}

/// Mark everything in a box as seen. The work is queued, so the effect is eventually consistent.
@http(method: "POST", uri: "/boxes/{boxId}/observation.json")
@tags(["Boxes"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MarkBoxSeen {
    input: MarkBoxSeenInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure MarkBoxSeenInput {
    @httpLabel
    @required
    boxId: Long
}

// =============================================================================
// FOLDERS
// =============================================================================

/// Get a folder (label) and the postings filed in it
@readonly
@http(method: "GET", uri: "/folders/{folderId}")
@tags(["Folders"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetFolder {
    input: GetFolderInput
    output: GetFolderOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetFolderInput {
    @httpLabel
    @required
    folderId: Long

    @httpQuery("page")
    page: String
}

structure GetFolderOutput {
    @required
    folder: FolderWithPostings
}

// =============================================================================
// COLLECTIONS
// =============================================================================

/// List collections
@readonly
@http(method: "GET", uri: "/collections.json")
@tags(["Collections"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListCollections {
    output: ListCollectionsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListCollectionsOutput {
    @required
    collections: CollectionList
}

/// Get a collection and one page of its active, accessible threads
@readonly
@http(method: "GET", uri: "/collections/{collectionId}")
@tags(["Collections"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation GetCollection {
    input: GetCollectionInput
    output: GetCollectionOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetCollectionInput {
    @httpLabel
    @required
    collectionId: Long

    @httpQuery("page")
    page: String
}

structure GetCollectionOutput {
    @required
    collection: CollectionWithPostings
}

/// Rename a collection or change its summary
@idempotent
@http(method: "PATCH", uri: "/collections/{collectionId}")
@tags(["Collections"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateCollection {
    input: UpdateCollectionInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateCollectionInput {
    @httpLabel
    @required
    collectionId: Long

    @httpPayload
    @required
    body: UpdateCollectionRequestContent
}

/// Wire format: {collection: {name, summary}}
structure UpdateCollectionRequestContent {
    @required
    collection: CollectionPayload
}

structure CollectionPayload {
    name: String
    summary: String
}

// =============================================================================
// STICKIES
// =============================================================================

/// List stickies, newest position first
@readonly
@http(method: "GET", uri: "/stickies.json")
@tags(["Stickies"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListStickies {
    input: ListStickiesInput
    output: ListStickiesOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListStickiesInput {
    /// Clamped server-side to 1..100
    @httpQuery("limit")
    limit: Integer
}

structure ListStickiesOutput {
    @required
    stickies: StickyList
}

/// Write a new sticky
@http(method: "POST", uri: "/stickies.json")
@tags(["Stickies"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateSticky {
    input: CreateStickyInput
    output: StickyOutput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateStickyInput {
    @httpPayload
    @required
    body: StickyRequestContent
}

/// Wire format: {sticky: {body, size}}. Size is "small", "medium" or "large".
structure StickyRequestContent {
    @required
    sticky: StickyPayload
}

structure StickyPayload {
    body: String
    size: String
}

structure StickyOutput {
    @required
    sticky: Sticky
}

/// Edit a sticky
@idempotent
@http(method: "PATCH", uri: "/stickies/{stickyId}")
@tags(["Stickies"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateSticky {
    input: UpdateStickyInput
    output: StickyOutput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateStickyInput {
    @httpLabel
    @required
    stickyId: Long

    @httpPayload
    @required
    body: StickyRequestContent
}

/// Throw a sticky away
@idempotent
@http(method: "DELETE", uri: "/stickies/{stickyId}")
@tags(["Stickies"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteSticky {
    input: DeleteStickyInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteStickyInput {
    @httpLabel
    @required
    stickyId: Long
}

/// Reposition a sticky on the board
@http(method: "POST", uri: "/stickies/moves.json")
@tags(["Stickies"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MoveSticky {
    input: MoveStickyInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure MoveStickyInput {
    @httpPayload
    @required
    body: MoveStickyRequestContent
}

/// Wire format: {id, position} — both at the top level.
structure MoveStickyRequestContent {
    @required
    id: Long

    @required
    position: Integer
}

// =============================================================================
// CALENDAR TIME TRACK WRITES
// =============================================================================

/// Record a finished stretch of time.
///
/// JSON callers send the fields flat; Rails wraps them into calendar_time_track itself.
@http(method: "POST", uri: "/calendar/time_tracks.json")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateTimeTrack {
    input: CreateTimeTrackInput
    output: CreateTimeTrackOutput
    errors: [UnauthorizedError, BadRequestError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateTimeTrackInput {
    @httpPayload
    @required
    body: TimeTrackRequestContent
}

structure TimeTrackRequestContent {
    @required
    starts_at: String

    @required
    ends_at: String

    category_title: String
    notes: String
}

structure CreateTimeTrackOutput {
    @required
    recording: Recording
}

/// Delete a time track. The id is the recording's.
@idempotent
@http(method: "DELETE", uri: "/calendar/time_tracks/{timeTrackId}")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteTimeTrack {
    input: DeleteTimeTrackInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteTimeTrackInput {
    @httpLabel
    @required
    timeTrackId: Long
}

// =============================================================================
// CALENDAR HABIT CRUD
// =============================================================================

/// Start a new habit. Answers the created habit as a recording.
@http(method: "POST", uri: "/calendar/habits.json", code: 201)
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateHabit {
    input: CreateHabitInput
    output: CreateHabitOutput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateHabitInput {
    @httpPayload
    @required
    body: HabitRequestContent
}

structure CreateHabitOutput {
    @required
    recording: Recording
}

/// Wire format: {calendar_habit: {name, icon, color, days: [0..6]}}
structure HabitRequestContent {
    @required
    calendar_habit: HabitPayload
}

structure HabitPayload {
    name: String
    icon: String
    color: String

    /// Days of the week the habit runs on, 0 for Sunday through 6 for Saturday
    days: DaysList
}

/// Edit a habit. habitId is the recording's id.
@idempotent
@http(method: "PATCH", uri: "/calendar/habits/{habitId}")
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateHabit {
    input: UpdateHabitInput
    output: UpdateHabitOutput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

/// The edited habit as a recording (haystack renders calendar/recordings/_recording).
structure UpdateHabitOutput {
    @required
    recording: Recording
}

structure UpdateHabitInput {
    @httpLabel
    @required
    habitId: Long

    @httpPayload
    @required
    body: HabitRequestContent
}

/// Delete a habit. habitId is the recording's id.
@idempotent
@http(method: "DELETE", uri: "/calendar/habits/{habitId}")
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteHabit {
    input: HabitInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure HabitInput {
    @httpLabel
    @required
    habitId: Long
}

/// Pause a habit, so it stops appearing on the calendar
@http(method: "POST", uri: "/calendar/habits/{habitId}/stop.json")
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation StopHabit {
    input: HabitInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Resume a paused habit
@idempotent
@http(method: "DELETE", uri: "/calendar/habits/{habitId}/stop.json")
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ResumeHabit {
    input: HabitInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

// =============================================================================
// TIME TRACK CATEGORIES, CLIPS, SNIPPETS, WORKFLOWS, PUBLICATIONS — reads
// =============================================================================

/// List the calendar's time track categories, alphabetically
@readonly
@http(method: "GET", uri: "/calendar/time_tracks/categories.json")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListTimeTrackCategories {
    output: ListTimeTrackCategoriesOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListTimeTrackCategoriesOutput {
    @required
    categories: TimeTrackCategoryList
}

structure TimeTrackCategory {
    @required
    id: Long
    title: String
    created_at: DateTime
    updated_at: DateTime
}

list TimeTrackCategoryList {
    member: TimeTrackCategory
}

/// List clips, newest first
@readonly
@http(method: "GET", uri: "/clips.json")
@tags(["Clips"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation ListClips {
    input: PagedInput
    output: ListClipsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListClipsOutput {
    @required
    clips: ClipList
}

structure Clip {
    @required
    id: Long
    content: String
    created_at: DateTime
    updated_at: DateTime
    entry_id: Long
    topic: ClipTopic
}

/// The topic a clip was taken from
structure ClipTopic {
    @required
    id: Long
    name: String
    app_url: String
}

list ClipList {
    member: Clip
}

/// List snippets, alphabetically
@readonly
@http(method: "GET", uri: "/snippets.json")
@tags(["Snippets"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation ListSnippets {
    output: ListSnippetsOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure ListSnippetsOutput {
    @required
    snippets: SnippetList
}

structure Snippet {
    @required
    id: Long
    name: String
    /// Plain text
    content: String
    /// Rich-text HTML
    content_html: String
    created_at: DateTime
    updated_at: DateTime
}

list SnippetList {
    member: Snippet
}

/// A workflow with its stages
@readonly
@http(method: "GET", uri: "/workflows/{workflowId}")
@tags(["Workflows"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetWorkflow {
    input: GetWorkflowInput
    output: GetWorkflowOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetWorkflowInput {
    @httpLabel
    @required
    workflowId: Long
}

structure GetWorkflowOutput {
    @required
    workflow: Workflow
}

/// Add a topic to a workflow. HEY places it in the first stage.
@http(method: "POST", uri: "/topics/{topicId}/workflows/{workflowId}/stagings")
@tags(["Workflows"])
operation CreateWorkflowStaging {
    input: CreateWorkflowStagingInput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateWorkflowStagingInput {
    @httpLabel
    @required
    topicId: Long

    @httpLabel
    @required
    workflowId: Long
}

/// Move a staged topic to a workflow stage.
@http(method: "PATCH", uri: "/topics/{topicId}/workflows/{workflowId}/stagings")
@tags(["Workflows"])
operation MoveWorkflowStaging {
    input: MoveWorkflowStagingInput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure MoveWorkflowStagingInput {
    @httpLabel
    @required
    topicId: Long

    @httpLabel
    @required
    workflowId: Long

    @httpPayload
    @required
    body: MoveWorkflowStagingRequestContent
}

structure MoveWorkflowStagingRequestContent {
    @required
    workflow_staging: WorkflowStagingPayload
}

structure WorkflowStagingPayload {
    @required
    workflow_stage_id: Long
}

/// Whether a thread is shared with a public link, and the link
@readonly
@http(method: "GET", uri: "/topics/{topicId}/publication.json")
@tags(["Publications"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetTopicPublication {
    input: GetTopicPublicationInput
    output: GetTopicPublicationOutput
    errors: [UnauthorizedError, ForbiddenError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetTopicPublicationInput {
    @httpLabel
    @required
    topicId: Long
}

structure GetTopicPublicationOutput {
    @required
    publication: TopicPublication
}

structure TopicPublication {
    @required
    published: Boolean
    /// The public link, when published
    url: String
}
