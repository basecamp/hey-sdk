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
    version: "2026-03-04"
    operations: [
        // Identity (2 MVP)
        GetIdentity
        GetNavigation

        // Boxes (8 MVP)
        ListBoxes
        GetBox
        GetImbox
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
        CreateTopicMessage

        // Entries (2 MVP)
        ListDrafts
        CreateReply

        // Contacts (2 MVP)
        ListContacts
        GetContact

        // Calendars (2 MVP)
        ListCalendars
        GetCalendarRecordings

        // Calendar Todos (4 MVP)
        CreateCalendarTodo
        CompleteCalendarTodo
        UncompleteCalendarTodo
        DeleteCalendarTodo

        // Calendar Habits (4 MVP)
        CreateHabit
        CompleteHabit
        UncompleteHabit
        DeleteHabit

        // Calendar Time Tracks (3 MVP)
        GetOngoingTimeTrack
        StartTimeTrack
        UpdateTimeTrack

        // Calendar Journal (2 MVP)
        GetJournalEntry
        UpdateJournalEntry

        // Search (1 MVP)
        Search

        // Postings (8 MVP)
        MarkPostingsSeen
        MarkPostingsUnseen
        MovePostingToFeed
        MovePostingToSetAside
        MovePostingToReplyLater
        MovePostingToPaperTrail
        MovePostingToTrash
        IgnorePosting
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

@error("client")
@httpError(422)
structure UnprocessableEntityError {
    @required
    message: String
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
structure Clearance {
    @required
    id: Long
    status: String
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

/// Recording — polymorphic by `type` (CalendarEvent, CalendarTodo, etc.)
@heyPolymorphic(
    discriminator: "type"
    variants: {
        "CalendarEvent": ["edit_url", "summary", "url", "location",
                         "manage_attendance", "attendance_status", "organizer",
                         "attendances", "attendances_summary", "description",
                         "join_link", "attached_entry"]
        "CalendarTodo": ["position"]
        "CalendarJournalEntry": ["content"]
        "CalendarHabit": ["color", "icon", "days", "icon_url", "stopped_at"]
        "CalendarTimeTrack": ["notes", "category"]
        "CalendarCountdown": ["label"]
        "CalendarDayBackground": ["image_url"]
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

    /// Discriminator: CalendarEvent, CalendarTodo, etc.
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

/// SearchResult — topics from search
structure SearchResult {
    topics: TopicList
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

structure MessageEntryPayload {
    addressed: MessageAddressed
}

/// Recipients as comma-separated email addresses.
structure MessageAddressed {
    directly: String
    copied: String
    blindcopied: String
}

/// Reply to an existing topic
@http(method: "POST", uri: "/topics/{topicId}/entries.json")
@tags(["Messages"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateTopicMessage {
    input: CreateTopicMessageInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateTopicMessageInput {
    @httpLabel
    @required
    topicId: Long

    @httpPayload
    @required
    body: CreateTopicMessageRequestContent
}

/// Wire format: {acting_sender_id, message: {content}}
structure CreateTopicMessageRequestContent {
    @required
    acting_sender_id: Long

    @required
    message: TopicMessagePayload
}

structure TopicMessagePayload {
    @required
    content: String
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

/// Reply to an entry
@http(method: "POST", uri: "/entries/{entryId}/replies.json")
@tags(["Entries"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation CreateReply {
    input: CreateReplyInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure CreateReplyInput {
    @httpLabel
    @required
    entryId: Long

    @httpPayload
    @required
    body: CreateReplyRequestContent
}

/// Wire format: {acting_sender_id, message: {content}}
structure CreateReplyRequestContent {
    @required
    acting_sender_id: Long

    @required
    message: ReplyMessagePayload
}

structure ReplyMessagePayload {
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

/// Get a contact
@readonly
@http(method: "GET", uri: "/contacts/{contactId}")
@tags(["Contacts"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation GetContact {
    input: GetContactInput
    output: GetContactOutput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure GetContactInput {
    @httpLabel
    @required
    contactId: Long
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
}

structure GetContactOutput {
    @required
    contact: ContactDetail
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
}

structure GetCalendarRecordingsOutput {
    @required
    recordings: CalendarRecordingsResponse
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

/// Create a habit
@http(method: "POST", uri: "/calendar/habits.json")
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
    body: CreateHabitRequestContent
}

/// Wire format: {calendar_habit: {title, days}}
structure CreateHabitRequestContent {
    @required
    calendar_habit: HabitPayload
}

structure HabitPayload {
    @required
    title: String

    /// Days of week as integers (0=Sunday … 6=Saturday). Omit to accept server default.
    days: DaysList
}

structure CreateHabitOutput {
    @required
    recording: Recording
}

/// Delete a habit
@idempotent
@http(method: "DELETE", uri: "/calendar/habits/{habitId}")
@tags(["Calendar Habits"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation DeleteHabit {
    input: DeleteHabitInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

structure DeleteHabitInput {
    @httpLabel
    @required
    habitId: Long
}

// =============================================================================
// CALENDAR TIME TRACK OPERATIONS
// =============================================================================

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

/// Start a new time track
@http(method: "POST", uri: "/calendar/ongoing_time_track.json")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation StartTimeTrack {
    input: StartTimeTrackInput
    output: StartTimeTrackOutput
    errors: [UnauthorizedError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure StartTimeTrackInput {
    @httpPayload
    body: StartTimeTrackRequestContent
}

/// Wire format: {calendar_time_track: {title, notes, category}}
structure StartTimeTrackRequestContent {
    calendar_time_track: StartTimeTrackPayload
}

structure StartTimeTrackPayload {
    title: String
    notes: String
    category: String
}

structure StartTimeTrackOutput {
    @required
    recording: Recording
}

/// Update a time track (stop by setting ends_at to current time)
@idempotent
// NOTE: Live API path is /calendar/time_tracks/{id}.json but Smithy
// forbids literals after labels in URI segments. The Go SDK uses a
// hardcoded path; generated clients append .json via middleware.
@http(method: "PUT", uri: "/calendar/time_tracks/{timeTrackId}")
@tags(["Calendar Time Tracks"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateTimeTrack {
    input: UpdateTimeTrackInput
    output: UpdateTimeTrackOutput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
}

structure UpdateTimeTrackInput {
    @httpLabel
    @required
    timeTrackId: Long

    @httpPayload
    @required
    body: UpdateTimeTrackRequestContent
}

/// Wire format: {calendar_time_track: {title, notes, category, starts_at, ends_at}}
structure UpdateTimeTrackRequestContent {
    @required
    calendar_time_track: UpdateTimeTrackPayload
}

structure UpdateTimeTrackPayload {
    title: String
    notes: String
    category: String
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

/// Update journal entry for a day
@http(method: "PATCH", uri: "/calendar/days/{day}/journal_entry")
@tags(["Calendar Journal"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation UpdateJournalEntry {
    input: UpdateJournalEntryInput
    errors: [UnauthorizedError, NotFoundError, UnprocessableEntityError, InternalServerError, ServiceUnavailableError]
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

/// Search topics
@readonly
@http(method: "GET", uri: "/search.json")
@tags(["Search"])
@heyRetry(maxAttempts: 3, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
@heyPagination(style: "link", totalCountHeader: "X-Total-Count")
operation Search {
    input: SearchInput
    output: SearchOutput
    errors: [UnauthorizedError, InternalServerError, ServiceUnavailableError]
}

structure SearchInput {
    @httpQuery("q")
    @required
    q: String

    @httpQuery("page")
    page: String
}

structure SearchOutput {
    @required
    result: SearchResult
}

// =============================================================================
// POSTINGS — mark seen/unseen, move between boxes, trash, ignore
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

/// Move posting to The Feed
@http(method: "POST", uri: "/postings/{postingId}/move/feedbox.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MovePostingToFeed {
    input: PostingActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Move posting to Set Aside
@http(method: "POST", uri: "/postings/{postingId}/move/asidebox.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MovePostingToSetAside {
    input: PostingActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Move posting to Reply Later
@http(method: "POST", uri: "/postings/{postingId}/move/laterbox.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MovePostingToReplyLater {
    input: PostingActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Move posting to Paper Trail
@http(method: "POST", uri: "/postings/{postingId}/move/trailbox.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MovePostingToPaperTrail {
    input: PostingActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Move posting to trash
@http(method: "POST", uri: "/postings/{postingId}/trash.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation MovePostingToTrash {
    input: PostingActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Ignore a posting (stop notifications)
@http(method: "POST", uri: "/postings/{postingId}/ignore.json")
@tags(["Postings"])
@heyRetry(maxAttempts: 2, baseDelayMs: 1000, backoff: "exponential", retryOn: [429, 503])
operation IgnorePosting {
    input: PostingActionInput
    errors: [UnauthorizedError, NotFoundError, InternalServerError, ServiceUnavailableError]
}

/// Input for single-posting actions (move, trash, ignore)
structure PostingActionInput {
    @httpLabel
    @required
    postingId: Long
}
