// Generated from openapi.json and behavior-model.json. Do not edit.
export interface paths {
    "/accounts/{accountId}/domains/extenzions/{extenzionId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * @description Delete an extenzion. The id is the extenzion's contact id, the one its app_url
         *     carries. Answers 204; forbidden when the caller cannot edit the extenzion.
         */
        delete: operations["DeleteExtenzion"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/advanced_search.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description Get the options the advanced search refine form offers.
         *
         *     Advanced search: message matches grouped by topic as the search page shows them —
         *     the topic, its posting id, and the matching entries as summaries (no bodies; read a
         *     message with GetMessage). Refinements are the same query parameters the page uses.
         *     The next page, if any, is a Link header.
         */
        get: operations["AdvancedSearch"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/advanced_search_filters.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description The advanced search refine form's options: boxes, date ranges, labels and attachment kinds. */
        get: operations["GetAdvancedSearchFilters"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List all boxes */
        get: operations["ListBoxes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get a specific box */
        get: operations["GetBox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}/designations.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Designate a contact to a box, so everything they send lands there */
        post: operations["CreateBoxDesignation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}/designations/{designationId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Remove a designation from a box. The id is the designation's, not the contact's. */
        delete: operations["DeleteBoxDesignation"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}/groups.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List the Set Aside groups in a box */
        get: operations["ListBoxGroups"];
        put?: never;
        /**
         * @description Create a Set Aside group out of a selection of postings.
         *
         *     This endpoint does not split a comma-joined posting_ids string — send an array.
         */
        post: operations["CreateBoxGroup"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}/groups/{groupId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description Read one Set Aside group with the postings in it.
         *
         *     The postings are paged like a folder's: newest observed first, 30 to a page, with the
         *     next page in the Link header and the total in X-Total-Count.
         */
        get: operations["GetBoxGroup"];
        put?: never;
        post?: never;
        /** @description Break up a Set Aside group, moving its postings back to Previously Seen */
        delete: operations["DeleteBoxGroup"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}/observation.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Mark everything in a box as seen. The work is queued, so the effect is eventually consistent. */
        post: operations["MarkBoxSeen"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/boxes/{boxId}/postings/changes.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description Read what changed among a box's postings since a point in time.
         *
         *     This is the incremental sync feed the mail clients follow rather than re-reading a
         *     box. `since` is an ISO 8601 timestamp with milliseconds and is exclusive, and `v` is
         *     the client's contract version — the server answers 409 when the caller is too far
         *     behind for an increment to carry the difference, which means read the box in full
         *     instead. A box's own `posting_changes_url` carries the `since` and `v` to start from,
         *     and the `Link` header names the next page while one remains and the next `since`
         *     cursor on the last page.
         */
        get: operations["GetBoxPostingChanges"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/bubble_up.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Bubble Up box */
        get: operations["GetBubblebox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/bulk_replies.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Send one reply to every entry. Answers what was sent, not the replies themselves:
         *     delivery is queued, and delayed while undo is still possible.
         */
        post: operations["CreateBulkReply"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/bulk_replies/new.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description Work out which entries a bulk reply would answer. HEY replies to the last replyable
         *     entry of each thread, skipping threads with no reply address, so the postings you hold
         *     are not the entries you send to — this resolves them.
         */
        get: operations["NewBulkReply"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/days.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description List the days from a date onwards. The server picks how many, so this is a window
         *     rather than a page: read the next one by asking from the last day's date.
         */
        get: operations["ListCalendarDays"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/days/{day}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get one day */
        get: operations["GetCalendarDay"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/days/{day}/habits/{habitId}/completions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Complete a habit for a day */
        post: operations["CompleteHabit"];
        /** @description Uncomplete a habit for a day */
        delete: operations["UncompleteHabit"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/days/{day}/journal_entry": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get journal entry for a day */
        get: operations["GetJournalEntry"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * @description Update the journal entry for a day: writes it, creating it if the day has none, and
         *     answers the entry as a recording. Empty content removes the entry instead, and HEY then
         *     answers 204 with no body — which is not this shape, so send that through the SDK's own
         *     journal wrapper rather than here.
         */
        patch: operations["UpdateJournalEntry"];
        trace?: never;
    };
    "/calendar/events/{eventId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Delete a calendar event, cancelling it for every attendee. Answers 204. */
        delete: operations["DeleteCalendarEvent"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/events/{eventId}/occurrences/{occurrence}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * @description Delete one day of a repeating event, or that day and every one after it.
         *     Answers 204. A single day becomes an exception in the series' schedule; with
         *     apply_to_future the series is truncated at the day before, or destroyed if this
         *     was its first day.
         */
        delete: operations["DeleteCalendarEventOccurrence"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/habits.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Start a new habit. Answers the created habit as a recording. */
        post: operations["CreateHabit"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/habits/{habitId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Delete a habit. habitId is the recording's id. */
        delete: operations["DeleteHabit"];
        options?: never;
        head?: never;
        /** @description Edit a habit. habitId is the recording's id. */
        patch: operations["UpdateHabit"];
        trace?: never;
    };
    "/calendar/habits/{habitId}/stop.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Pause a habit, so it stops appearing on the calendar */
        post: operations["StopHabit"];
        /** @description Resume a paused habit */
        delete: operations["ResumeHabit"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/identity/first_week_day": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * @description Set which day the identity's calendar weeks start on. Answers the stored
         *     preference. The write reaches every HEY client — web, mobile and this SDK
         *     read the same identity preference.
         */
        put: operations["UpdateFirstWeekDay"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/journal_entries": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description List journal entries newest first. The next page, if any, is a Link header.
         *     Pass q to search journal entry content.
         */
        get: operations["ListJournalEntries"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/ongoing_time_track.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the ongoing time track (404 = no active track; see ADR-004) */
        get: operations["GetOngoingTimeTrack"];
        put?: never;
        /**
         * @description Start a new time track. Takes no body: haystack's
         *     Calendar::OngoingTimeTracksController#create ignores request parameters and
         *     starts a track with defaults; use UpdateTimeTrack to set notes and category_title,
         *     which also stops the track.
         */
        post: operations["StartTimeTrack"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/time_tracks.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description List tracked time — completed tracks only, newest-ended first.
         *
         *     A running track is not here; read that with GetOngoingTimeTrack. The next page, if
         *     any, is a Link header, and the last page carries none, so a nil Link is the end of
         *     the list rather than an error.
         *
         *     category_id narrows the list to one category and 404s if the calendar has no
         *     category by that id.
         *
         *     The calendar's categories come back alongside the tracks, so showing or applying the
         *     filter does not need ListTimeTrackCategories as well.
         */
        get: operations["ListTimeTracks"];
        put?: never;
        /**
         * @description Record a finished stretch of time.
         *
         *     JSON callers send the fields flat; Rails wraps them into calendar_time_track itself.
         */
        post: operations["CreateTimeTrack"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/time_tracks/categories.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List the calendar's time track categories, alphabetically */
        get: operations["ListTimeTrackCategories"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/time_tracks/{timeTrackId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * @description Update a time track (stop by setting ends_at to current time).
         *
         *     Every update completes the track, whether or not ends_at is sent, so this cannot
         *     be used to adjust a running track: it stops it.
         *
         *     Only the fields sent are written, so a partial update leaves the rest of the track
         *     alone. A starts_at or ends_at the server cannot parse is a 400, not a 422.
         */
        put: operations["UpdateTimeTrack"];
        post?: never;
        /** @description Delete a time track. The id is the recording's. */
        delete: operations["DeleteTimeTrack"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/todos.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Create a calendar todo */
        post: operations["CreateCalendarTodo"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/todos/{todoId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Delete a calendar todo */
        delete: operations["DeleteCalendarTodo"];
        options?: never;
        head?: never;
        /**
         * @description Edit a calendar todo. todoId is the recording's id, and every field of the payload
         *     is optional: haystack's `wrap_parameters` accepts title, focused and starts_at, and
         *     changes only what is sent.
         */
        patch: operations["UpdateCalendarTodo"];
        trace?: never;
    };
    "/calendar/todos/{todoId}/completions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Complete a calendar todo */
        post: operations["CompleteCalendarTodo"];
        /** @description Uncomplete a calendar todo */
        delete: operations["UncompleteCalendarTodo"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/weeks.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List the weeks around a date — nine of them, centered on it. */
        get: operations["ListCalendarWeeks"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/weeks/{week}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get one week */
        get: operations["GetCalendarWeek"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendar/years/{year}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get one year as the grid it is drawn as */
        get: operations["GetCalendarYear"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendars.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List calendars */
        get: operations["ListCalendars"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendars/{calendarId}/recordings": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get recordings for a calendar */
        get: operations["GetCalendarRecordings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/calendars/{calendarId}/toggle": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Switch a calendar in or out of the reader's selection, and answer the selection it
         *     left behind. The selection is what every period read is scoped to, so a toggle is how
         *     a client changes which calendars a day, week or year is drawn from.
         */
        post: operations["ToggleCalendar"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/clearances.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Screener — the pending count, and the senders waiting when asked for them */
        get: operations["GetClearances"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/clearances/bulk.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** @description Screen several senders out at once. ids is a comma-separated list. */
        patch: operations["BulkUpdateClearances"];
        trace?: never;
    };
    "/clearances/punt.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Clear the Screener — every pending sender is punted and reexamined on their next email */
        post: operations["PuntClearances"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/clearances/{clearanceId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * @description Screen a sender in or out of the Screener
         *
         *     designation_box_id files everything they send into that box instead of the Imbox.
         *     spam marks what is already waiting as spam and trains the filter on it.
         */
        patch: operations["UpdateClearance"];
        trace?: never;
    };
    "/clips.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List clips, newest first */
        get: operations["ListClips"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/collections.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List collections */
        get: operations["ListCollections"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/collections/{collectionId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get a collection and one page of its active, accessible threads */
        get: operations["GetCollection"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** @description Rename a collection or change its summary */
        patch: operations["UpdateCollection"];
        trace?: never;
    };
    "/contacts.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List contacts */
        get: operations["ListContacts"];
        put?: never;
        /** @description Add a contact. Answers the contact that was created. */
        post: operations["CreateContact"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/contacts/{contactId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get a contact, with a page of the threads they are on */
        get: operations["GetContact"];
        put?: never;
        post?: never;
        /** @description Hide a contact. Nothing is deleted — RevealContact brings them back. */
        delete: operations["HideContact"];
        options?: never;
        head?: never;
        /**
         * @description Edit a contact. HEY rewrites the whole contact, so send every field: a name,
         *     address or alias left out is cleared. Answers the contact, which is not always
         *     the one addressed — promoting an alias makes the alias primary.
         */
        patch: operations["UpdateContact"];
        trace?: never;
    };
    "/contacts/{contactId}/bundle.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Bundle a contact so their mail arrives grouped */
        post: operations["BundleContact"];
        /** @description Stop bundling a contact's mail */
        delete: operations["UnbundleContact"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/contacts/{contactId}/clearance.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** @description Screen a contact in or out. Status is "approved" or "denied". */
        patch: operations["UpdateContactClearance"];
        trace?: never;
    };
    "/contacts/{contactId}/note.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Read the private note kept on a contact */
        get: operations["GetContactNote"];
        put?: never;
        post?: never;
        /** @description Clear the private note on a contact */
        delete: operations["DeleteContactNote"];
        options?: never;
        head?: never;
        /** @description Write the private note on a contact, replacing whatever was there */
        patch: operations["UpdateContactNote"];
        trace?: never;
    };
    "/contacts/{contactId}/reveal.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Put a hidden contact back in the contact list */
        post: operations["RevealContact"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entries/drafts.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List draft messages */
        get: operations["ListDrafts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entries/drafts/{entryId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * @description Trash a draft (Entries::DraftsController#destroy). The id is the draft's entry id,
         *     as ListDrafts reports it.
         */
        delete: operations["DeleteDraft"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entries/{entryId}/forwards/new.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description Get a prefilled forward of an entry: subject, quoted body and blank recipients.
         *     Send it with CreateMessage once the recipients are filled in.
         */
        get: operations["NewEntryForward"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entries/{entryId}/replies.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Reply to an entry */
        post: operations["CreateReply"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entries/{entryId}/replies/new.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description Get a prefilled reply to an entry: the quoted body and, in addressed, the
         *     participating contacts a reply goes to as HEY computes them — the sender moved onto
         *     the To line and the acting user's own addresses, aliases and catch-alls excluded.
         */
        get: operations["NewEntryReply"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entries/{entryId}/status/spam.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** @description Mark an entry as spam. Denies the sender when every thread from them is already spam. */
        put: operations["MarkEntrySpam"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/feedbox.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Feed */
        get: operations["GetFeedbox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/folders/{folderId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get a folder (label) and the postings filed in it */
        get: operations["GetFolder"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/identity.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the current identity (authenticated user profile) */
        get: operations["GetIdentity"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/identity/time_format": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * @description Set whether HEY renders times on a 12-hour or a 24-hour clock. Answers the
         *     stored preference. The parameter is the web toggle's, said honestly: true
         *     for the 24-hour clock, false for the 12-hour one.
         */
        put: operations["UpdateTimeFormat"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/imbox.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Imbox */
        get: operations["GetImbox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/imbox/seen.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Imbox's Previously Seen postings */
        get: operations["GetImboxSeen"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/messages.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Create a new message (start a new topic).
         *     The acting sender ID must be included; the Go SDK resolves this automatically.
         *     Every message is created drafted on HEY's side; without entry.status the server
         *     delivers it, while entry.status "drafted" leaves it as a draft and answers
         *     204 with a Location header naming /messages/{entry_id}.
         */
        post: operations["CreateMessage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/messages/{messageId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get a message */
        get: operations["GetMessage"];
        /**
         * @description Revise a message entry (MessagesController#update). With entry.status "drafted" the
         *     entry is saved as a draft (204 + Location, like CreateMessage); without it a draft is
         *     delivered through the undo-delay window. A trashed draft is silently restored first.
         *     The revision is not a patch: subject, content and any scheduled delivery are rewritten
         *     from this request (an omitted scheduled delivery clears one), while recipients are
         *     replaced only when entry.addressed is present.
         *
         *     Not naturally idempotent despite the PUT: without the drafted status this request
         *     *delivers*, so a transparent retry after an ambiguous first attempt could send the
         *     message again. The client must not retry it.
         */
        put: operations["UpdateMessage"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/messages/{messageId}/edit.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description A draft's editable state: content, recipients and scheduled delivery as the
         *     composer would load them (GET /messages/{id}/edit).
         */
        get: operations["GetMessageEdit"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/my/clearances.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description The senders already screened in or out */
        get: operations["GetMyClearances"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/my/clearances/{clearanceId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** @description Rescreen a sender who was already screened in or out */
        patch: operations["UpdateMyClearance"];
        trace?: never;
    };
    "/my/navigation.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get navigation items */
        get: operations["GetNavigation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/paper_trail.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Paper Trail */
        get: operations["GetTrailbox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/box_groups.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Add a selection of postings to a Set Aside group */
        post: operations["AddPostingsToBoxGroup"];
        /** @description Remove a selection of postings from their Set Aside group */
        delete: operations["RemovePostingsFromBoxGroup"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/bubble_up.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Schedule a selection of postings to bubble up.
         *
         *     HEY's scheduler takes a `slot` — today, tomorrow, weekend, next_week, surprise_me
         *     or custom — and a custom slot also carries the `date` (YYYY-MM-DD) to bubble up on,
         *     at HEY's morning hour. The today slot lands at HEY's evening hour of the current
         *     day instead, and both hours are UTC over JSON. An unknown slot, or a custom slot
         *     without a date, is a server error rather than a validation response, so callers
         *     check both first. Responds 201 Created.
         */
        post: operations["SchedulePostingsBubbleUp"];
        /** @description Cancel a scheduled bubble up for a selection of postings */
        delete: operations["CancelPostingsBubbleUp"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/bulk_bubble_up_now.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Bubble a selection of postings up right now */
        post: operations["BubbleUpPostingsNow"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/filings.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description File a selection of postings into an existing folder (label) */
        post: operations["FilePostings"];
        /** @description Remove a selection of postings from a folder, or from every folder when folder_id is omitted */
        delete: operations["UnfilePostings"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/folders.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Create a folder (label) and file a selection of postings into it */
        post: operations["CreateFolderForPostings"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/moves.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Move postings to a box (bulk).
         *     Mirrors HEY's Postings::MovesController: `posting_ids` plus the target `box_id`
         *     (an ID from ListBoxes; the box `kind` field identifies imbox, feedbox, asidebox,
         *     laterbox, trailbox). Responds 204 No Content.
         */
        post: operations["MovePostings"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/mutings.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Mute postings (bulk) — stop notifications for their threads.
         *     Mirrors HEY's Postings::MutingsController#create. Responds 201 Created.
         */
        post: operations["MutePostings"];
        /**
         * @description Unmute postings (bulk).
         *     Mirrors HEY's Postings::MutingsController#destroy. `posting_ids` is sent as a
         *     comma-separated query string because DELETE carries no body. Responds 201 Created.
         */
        delete: operations["UnmutePostings"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/seen.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Mark postings as seen */
        post: operations["MarkPostingsSeen"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/spam.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Mark a selection of postings as spam.
         *
         *     Over ten postings the server hands the work to a background job, so the effect is
         *     eventually consistent.
         */
        post: operations["MarkPostingsSpam"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/trash.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Move postings to the trash (bulk).
         *     Mirrors HEY's Postings::TrashController. For JSON requests the server treats
         *     the removal decision as made (shared topics: your access is removed).
         *     Responds 204 No Content.
         */
        post: operations["TrashPostings"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/unseen.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Mark postings as unseen */
        post: operations["MarkPostingsUnseen"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/postings/{postingId}/bundles/unseen.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * @description List the unseen postings inside a bundle posting.
         *
         *     A bundle posting groups one contact's unseen mail; this is its contents — the member
         *     postings, newest first, paged by cursor like a box. The posting must be a bundle.
         */
        get: operations["GetBundleUnseenPostings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/rails/active_storage/direct_uploads.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Create an Active Storage direct upload for an outgoing attachment.
         *     The returned URL is self-authenticating and accepts the raw file bytes via PUT.
         */
        post: operations["CreateDirectUpload"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/reply_later.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Reply Later box */
        get: operations["GetLaterbox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/set_aside.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get the Set Aside box */
        get: operations["GetAsidebox"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/snippets.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List snippets, alphabetically */
        get: operations["ListSnippets"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/stickies.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description List stickies, newest position first */
        get: operations["ListStickies"];
        put?: never;
        /** @description Write a new sticky */
        post: operations["CreateSticky"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/stickies/moves.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Reposition a sticky on the board */
        post: operations["MoveSticky"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/stickies/{stickyId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Throw a sticky away */
        delete: operations["DeleteSticky"];
        options?: never;
        head?: never;
        /** @description Edit a sticky */
        patch: operations["UpdateSticky"];
        trace?: never;
    };
    "/topics/everything.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get all topics (everything view) */
        get: operations["GetEverythingTopics"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/sent.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get sent topics */
        get: operations["GetSentTopics"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/spam.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get spam topics */
        get: operations["GetSpamTopics"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/spam/all.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Empty the spam box. Runs synchronously, so it can take a while on a large mailbox. */
        delete: operations["EmptySpam"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/trash.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get trash topics */
        get: operations["GetTrashTopics"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/trash/all.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** @description Empty the trash. Runs synchronously, so it can take a while on a large mailbox. */
        delete: operations["EmptyTrash"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get a topic */
        get: operations["GetTopic"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/entries": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Get entries for a topic */
        get: operations["GetTopicEntries"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/moves.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * @description Move a topic to another box.
         *
         *     Answers 204 without moving anything when the acting user has no posting for the topic.
         */
        post: operations["MoveTopic"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/publication.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description Whether a thread is shared with a public link, and the link */
        get: operations["GetTopicPublication"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/status/active.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** @description Restore a topic from the trash or the catch-all */
        put: operations["RestoreTopic"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/status/ham.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** @description Mark a spam topic as ham. Every other spam topic from the same sender is hammed too. */
        put: operations["MarkTopicHam"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/status/trashed.json": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * @description Trash a topic.
         *
         *     A shared topic redirects to the removal confirmation page unless confirm_destroy is set,
         *     so always pass it when trashing something that might be shared.
         */
        put: operations["TrashTopic"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/topics/{topicId}/workflows/{workflowId}/stagings": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** @description Add a topic to a workflow. HEY places it in the first stage. */
        post: operations["CreateWorkflowStaging"];
        delete?: never;
        options?: never;
        head?: never;
        /** @description Move a staged topic to a workflow stage. */
        patch: operations["MoveWorkflowStaging"];
        trace?: never;
    };
    "/workflows/{workflowId}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** @description A workflow with its stages */
        get: operations["GetWorkflow"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        /** @description Account — a HEY account */
        Account: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            domain?: string;
            status?: string;
            purpose?: string;
            trial?: boolean;
            trial_ends_on?: string;
            burner?: boolean;
            readonly?: boolean;
        };
        AddPostingsToBoxGroupRequestContent: {
            posting_ids: (number | bigint)[];
            /** Format: int64 */
            box_id: number | bigint;
            /** Format: int64 */
            box_group_id: number | bigint;
        };
        /** @description Addressed recipients */
        Addressed: {
            directly?: components["schemas"]["Contact"][];
            copied?: components["schemas"]["Contact"][];
            blindcopied?: components["schemas"]["Contact"][];
        };
        /** @description AddressedSender — sender context */
        AddressedSender: {
            directly?: components["schemas"]["Contact"][];
        };
        /** @description AdvancedSearchFilters — the options the advanced search refine form offers */
        AdvancedSearchFilters: {
            refine_in?: components["schemas"]["SearchFilterItem"][];
            refine_dates?: components["schemas"]["SearchFilterItem"][];
            refine_labels?: components["schemas"]["SearchFilterItem"][];
            refine_attachments?: components["schemas"]["SearchFilterItem"][];
        };
        AdvancedSearchResponseContent: components["schemas"]["AdvancedSearchResult"];
        AdvancedSearchResult: {
            matches: components["schemas"]["SearchMatch"][];
        };
        /** @description AttachedEntry — entry reference on a calendar event */
        AttachedEntry: {
            /** Format: int64 */
            id: number | bigint;
            kind?: string;
            title?: string;
            app_url?: string;
        };
        /** @description Attendance — calendar event attendee */
        Attendance: {
            /** Format: int64 */
            id: number | bigint;
            email_address?: string;
            status?: string;
            name?: string;
        };
        BadRequestErrorResponseContent: {
            message: string;
        };
        /** @description Box — a HEY mailbox */
        Box: {
            /** Format: int64 */
            id: number | bigint;
            kind: string;
            name: string;
            app_url?: string;
            url?: string;
            signed_stream_name?: string;
            posting_changes_url?: string;
            updates_channels?: components["schemas"]["UpdatesChannel"][];
        };
        /** @description BoxGroup — a Set Aside group. The API only ever returns the id. */
        BoxGroup: {
            /** Format: int64 */
            id: number | bigint;
        };
        /** @description BoxGroupWithPostings — a Set Aside group with one page of the postings in it */
        BoxGroupWithPostings: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            box_id?: number | bigint;
            postings?: components["schemas"]["Posting"][];
        };
        /** @description BoxGroupsResponse — the wrapper the groups index answers with */
        BoxGroupsResponse: {
            box_groups?: components["schemas"]["BoxGroup"][];
        };
        /**
         * @description BoxShowResponse — box detail with postings.
         *     The API can return fields at root level or nested under a `box` key.
         *     SDK response decoders normalize the nested variant to flat before decoding.
         */
        BoxShowResponse: {
            /** Format: int64 */
            id: number | bigint;
            kind: string;
            name: string;
            app_url?: string;
            url?: string;
            signed_stream_name?: string;
            posting_changes_url?: string;
            updates_channels?: components["schemas"]["UpdatesChannel"][];
            next_history_url?: string;
            next_incremental_sync_url?: string;
            postings?: components["schemas"]["Posting"][];
        };
        /** @description BubbleUpSchedule */
        BubbleUpSchedule: {
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            bubble_up_at?: string;
            surprise_me?: boolean;
        };
        BulkReplyDelivery: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int32 */
            entries_count: number;
            /** @description True while the send is held open for undo. */
            delayed: boolean;
            /** @description Where to POST to call the replies back, present only while delayed. */
            undo_send_url?: string;
        };
        /** @description The reply as HEY would send it: the prefilled content and the entries it goes to. */
        BulkReplyDraft: {
            /** @description The prefilled body — the name tag when every thread is on the same account. */
            content: string;
            entries: components["schemas"]["BulkReplyEntry"][];
        };
        /** @description One thread a bulk reply answers, with the recipients that thread's reply goes to. */
        BulkReplyEntry: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            topic_id: number | bigint;
            topic_name: string;
            addressed: components["schemas"]["Addressed"];
        };
        BulkReplyMessagePayload: {
            content: string;
        };
        /** @description Wire format: {entry_ids: [...], message: {content}} */
        BulkReplyRequestContent: {
            entry_ids: (number | bigint)[];
            message: components["schemas"]["BulkReplyMessagePayload"];
        };
        BulkUpdateClearancesRequestContent: {
            ids: string;
            status: string;
            spam?: boolean;
        };
        BulkUpdateClearancesResponseContent: components["schemas"]["ClearanceListResponse"];
        /** @description Calendar */
        Calendar: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            kind?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            owned?: boolean;
            color?: string;
            personal?: boolean;
            external?: boolean;
            url?: string;
            recordings_url?: string;
            occurrences_url?: string;
            owner_email_address?: string;
        };
        CalendarDayListPayload: {
            days: components["schemas"]["CalendarPeriod"][];
        };
        /** @description CalendarListPayload */
        CalendarListPayload: {
            calendars?: components["schemas"]["CalendarWithRecordingChangesUrl"][];
            calendar_changes_url?: string;
            /**
             * @description The calendars every period read is drawn from. ToggleCalendar changes this and
             *     answers the new one, so a client reads it here once — to open on what is already
             *     on — and takes it from the toggle after that.
             */
            selected_calendar_ids?: (number | bigint)[];
        };
        /**
         * @description CalendarPeriod — a day or a week: its bounds and everything in it, grouped by type.
         *     Recurring events arrive expanded into the occurrences that fall inside the window,
         *     which is what makes this a different answer than the recordings a calendar lists.
         */
        CalendarPeriod: {
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            starts_at: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            ends_at: string;
            /** @description "day" or "week" */
            kind: string;
            recordings: components["schemas"]["CalendarRecordingsResponse"];
        };
        /** @description CalendarRecordingsResponse — recordings grouped by type */
        CalendarRecordingsResponse: {
            [key: string]: components["schemas"]["Recording"][];
        };
        /** @description CalendarSelection — the calendars a toggle left switched on */
        CalendarSelection: {
            selected_calendar_ids: (number | bigint)[];
        };
        /** @description Nothing here is required: a rename sends a title and leaves the day alone. */
        CalendarTodoChanges: {
            title?: string;
            /** @description Date string (YYYY-MM-DD). The day the todo is filed on. */
            starts_at?: string;
            focused?: boolean;
        };
        CalendarTodoPayload: {
            title: string;
            /** @description Date string (YYYY-MM-DD). Defaults to today if omitted. */
            starts_at?: string;
        };
        CalendarWeekListPayload: {
            weeks: components["schemas"]["CalendarPeriod"][];
        };
        /** @description CalendarWithRecordingChangesUrl — wraps calendar with sync URL */
        CalendarWithRecordingChangesUrl: {
            calendar?: components["schemas"]["Calendar"];
            recording_changes_url?: string;
        };
        /**
         * @description CalendarYear — the grid a year is drawn as. A year carries one entry per day plus the
         *     events that span more than one, not every recording it holds: a year's worth of
         *     expanded occurrences is not something a client asks for by opening a year.
         */
        CalendarYear: {
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            starts_at: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            ends_at: string;
            /** @description "year" */
            kind: string;
            /**
             * Format: int32
             * @description Days between the reader's week start and January 1st, so the grid lines up
             */
            padding_days_count: number;
            days: components["schemas"]["CalendarYearDay"][];
            /** @description All-day and multi-day events, oldest first */
            spanned_events: components["schemas"]["Recording"][];
        };
        CalendarYearDay: {
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            starts_at: string;
            backgrounded: boolean;
        };
        /**
         * @description Clearance — screening status for a contact
         *
         *     petitioner and most_recent_entry are only filled in by the Screener reads. The
         *     contact reads answer a clearance with nothing but its id and status.
         */
        Clearance: {
            /** Format: int64 */
            id: number | bigint;
            status?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            petitioner?: components["schemas"]["Contact"];
            most_recent_entry?: components["schemas"]["Entry"];
        };
        /** @description ClearanceListResponse — wire format: {clearances: [...]} */
        ClearanceListResponse: {
            clearances?: components["schemas"]["Clearance"][];
        };
        /**
         * @description ClearanceSummary — the Screener's pending count, and the queue itself when asked for
         *
         *     clearances is only present when the read passes include_clearances. Without it HEY
         *     answers the count alone, which is what its own apps sync.
         */
        ClearanceSummary: {
            /** Format: int32 */
            pending_clearances_count?: number;
            signed_stream_name?: string;
            clearances?: components["schemas"]["Clearance"][];
        };
        Clip: {
            /** Format: int64 */
            id: number | bigint;
            content?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            /** Format: int64 */
            entry_id?: number | bigint;
            topic?: components["schemas"]["ClipTopic"];
        };
        /** @description The topic a clip was taken from */
        ClipTopic: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            app_url?: string;
        };
        /** @description Collection — email collection/label */
        Collection: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            app_url?: string;
        };
        CollectionPayload: {
            name?: string;
            summary?: string;
        };
        /** @description CollectionWithPostings — collection detail with its threads as posting objects */
        CollectionWithPostings: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            app_url?: string;
            postings?: components["schemas"]["Posting"][];
        };
        CompleteCalendarTodoResponseContent: components["schemas"]["Recording"];
        CompleteHabitResponseContent: components["schemas"]["Recording"];
        /**
         * @description The request conflicts with current state, e.g. starting a time track while one is
         *     already ongoing. Time tracks answer {"error": "..."}; contact writes answer the
         *     {"errors": [...]} list every other error path uses.
         */
        ConflictErrorResponseContent: {
            error?: string;
            errors?: string[];
            /**
             * Format: int64
             * @description Contact writes only: the contact that was written -- a create that clashes
             *     still creates the contact -- and the contacts already holding the email
             *     addresses that were sent, so a client can offer the merge the web offers.
             */
            contact_id?: number | bigint;
            conflicting_contact_ids?: (number | bigint)[];
        };
        /** @description Contact — the identity of someone in HEY */
        Contact: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            account_id?: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            name?: string;
            email_address?: string;
            avatar_url?: string;
            initials?: string;
            avatar_background_color?: string;
            contactable_type?: string;
            name_tag?: string;
        };
        /** @description ContactDetail — extended contact with additional show fields */
        ContactDetail: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            account_id?: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            name?: string;
            email_address?: string;
            avatar_url?: string;
            initials?: string;
            avatar_background_color?: string;
            contactable_type?: string;
            name_tag?: string;
            edit_app_url?: string;
            clearance?: components["schemas"]["Clearance"];
            aliases?: components["schemas"]["Contact"][];
            domain?: components["schemas"]["Domain"];
            /** @description The heading HEY gives the thread list, e.g. "All threads with GitHub" */
            entries_title?: string;
            /** @description One page of the threads this contact is on, newest first */
            postings?: components["schemas"]["Posting"][];
        };
        /** @description A contact's private note. Empty strings when there is no note. */
        ContactNote: {
            /** Format: int64 */
            contact_id: number | bigint;
            note: string;
            /** @description The note as editor HTML, the same markup the web hands Trix. */
            note_html: string;
        };
        ContactNotePayload: {
            note: string;
        };
        /** @description Wire format: {contact: {note: "..."}} */
        ContactNoteRequestContent: {
            contact: components["schemas"]["ContactNotePayload"];
        };
        ContactPayload: {
            name: string;
            email_address?: string;
            /** @description Sending the list replaces it: an address left out stops being an alias. */
            alias_email_addresses?: string[];
        };
        /** @description Wire format: {contact: {name, email_address, alias_email_addresses: [...]}} */
        ContactRequestContent: {
            contact: components["schemas"]["ContactPayload"];
        };
        CreateBoxDesignationRequestContent: {
            /** Format: int64 */
            contact_id: number | bigint;
        };
        CreateBoxGroupRequestContent: {
            posting_ids: (number | bigint)[];
        };
        CreateBoxGroupResponseContent: components["schemas"]["BoxGroup"];
        CreateBulkReplyResponseContent: components["schemas"]["BulkReplyDelivery"];
        /** @description Wire format: {calendar_todo: {title, starts_at}} */
        CreateCalendarTodoRequestContent: {
            calendar_todo: components["schemas"]["CalendarTodoPayload"];
        };
        CreateCalendarTodoResponseContent: components["schemas"]["Recording"];
        /**
         * @description Wire format: {acting_user_id, contact: {...}} — creating also has to say which account
         *     the contact belongs to, since one identity can hold several.
         */
        CreateContactRequestContent: {
            /**
             * Format: int64
             * @description The identity's user on the account the contact should be filed under; Identity's
             *     all_users carries one per account. Left out, HEY files it under the first account.
             */
            acting_user_id?: number | bigint;
            contact: components["schemas"]["ContactPayload"];
        };
        CreateContactResponseContent: components["schemas"]["Contact"];
        CreateDirectUploadRequestContent: {
            blob: components["schemas"]["DirectUploadBlob"];
        };
        /** @description Wire format: {posting_ids: [...], folder: {name, status}} */
        CreateFolderForPostingsRequestContent: {
            posting_ids: (number | bigint)[];
            folder: components["schemas"]["FolderPayload"];
        };
        CreateHabitResponseContent: components["schemas"]["Recording"];
        /** @description Wire format: {acting_sender_id, message: {subject, content}, entry: {addressed: {directly: "..."}}} */
        CreateMessageRequestContent: {
            /** Format: int64 */
            acting_sender_id: number | bigint;
            message: components["schemas"]["MessagePayload"];
            entry?: components["schemas"]["MessageEntryPayload"];
        };
        /**
         * @description Wire format: {acting_sender_id, message: {subject, content}, entry: {addressed: {directly: [...]}}}
         *     entry.addressed is optional on the wire but a reply posted without it is saved as a
         *     draft rather than delivered — HEY does not reply-all for the caller. Resolve the
         *     thread's recipients first and always send them.
         */
        CreateReplyRequestContent: {
            /** Format: int64 */
            acting_sender_id: number | bigint;
            message: components["schemas"]["ReplyMessagePayload"];
            entry?: components["schemas"]["MessageEntryPayload"];
        };
        CreateStickyResponseContent: components["schemas"]["Sticky"];
        CreateTimeTrackResponseContent: components["schemas"]["Recording"];
        /** @description DeletedPosting — the stub the changes feed answers with for a posting that is gone */
        DeletedPosting: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            box_id?: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            deleted_at?: string;
        };
        DirectUpload: {
            signed_id: string;
            attachable_sgid: string;
            direct_upload: components["schemas"]["DirectUploadTarget"];
        };
        DirectUploadBlob: {
            filename: string;
            /** Format: int64 */
            byte_size: number | bigint;
            checksum: string;
            content_type: string;
        };
        DirectUploadHeaders: {
            [key: string]: string;
        };
        DirectUploadTarget: {
            url: string;
            headers?: components["schemas"]["DirectUploadHeaders"];
        };
        /** @description Domain — email domain */
        Domain: {
            /** Format: int64 */
            id: number | bigint;
            address?: string;
            app_url?: string;
            avatar_url?: string;
        };
        /** @description DraftMessage — a draft entry */
        DraftMessage: {
            /** Format: int64 */
            id: number | bigint;
            subject?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            creator?: components["schemas"]["Contact"];
            /** Format: int64 */
            account_id?: number | bigint;
            summary?: string;
            url?: string;
            app_url?: string;
            edit_url?: string;
            addressed_contacts?: components["schemas"]["Contact"][];
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            scheduled_delivery_at?: string;
        };
        /** @description Entry — a message entry within a topic */
        Entry: {
            /** Format: int64 */
            id: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            creator?: components["schemas"]["Contact"];
            alternative_sender_name?: string;
            summary?: string;
            kind?: string;
            app_url?: string;
            subject?: string;
            /** Format: int64 */
            topic_id?: number | bigint;
        };
        /** @description Extenzion — external account extension */
        Extenzion: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            app_url?: string;
        };
        ExternalAccount: {
            /** Format: int64 */
            id: number | bigint;
            contact?: components["schemas"]["Contact"];
        };
        FilePostingsRequestContent: {
            posting_ids: (number | bigint)[];
            /** Format: int64 */
            folder_id: number | bigint;
        };
        FirstWeekDayParams: {
            /** @description Lowercase day name, sunday through saturday. */
            first_week_day: string;
        };
        FirstWeekDayPreference: {
            /**
             * Format: int32
             * @description 0 is Sunday through 6 Saturday, as GetIdentity serves it.
             */
            first_week_day: number;
        };
        /** @description Folder — email folder */
        Folder: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            app_url?: string;
        };
        FolderPayload: {
            name: string;
            status?: string;
        };
        /** @description FolderWithPostings — folder detail with the postings filed in it */
        FolderWithPostings: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            app_url?: string;
            postings?: components["schemas"]["Posting"][];
        };
        ForbiddenErrorResponseContent: {
            message: string;
        };
        GetAdvancedSearchFiltersResponseContent: components["schemas"]["AdvancedSearchFilters"];
        GetAsideboxResponseContent: components["schemas"]["BoxShowResponse"];
        GetBoxGroupResponseContent: components["schemas"]["BoxGroupWithPostings"];
        GetBoxPostingChangesResponseContent: {
            added?: components["schemas"]["Posting"][];
            updated?: components["schemas"]["Posting"][];
            deleted?: components["schemas"]["DeletedPosting"][];
        };
        GetBoxResponseContent: components["schemas"]["BoxShowResponse"];
        GetBubbleboxResponseContent: components["schemas"]["BoxShowResponse"];
        GetBundleUnseenPostingsResponseContent: {
            contact: components["schemas"]["Contact"];
            postings: components["schemas"]["Posting"][];
        };
        GetCalendarDayResponseContent: components["schemas"]["CalendarPeriod"];
        GetCalendarRecordingsResponseContent: components["schemas"]["CalendarRecordingsResponse"];
        GetCalendarWeekResponseContent: components["schemas"]["CalendarPeriod"];
        GetCalendarYearResponseContent: components["schemas"]["CalendarYear"];
        GetClearancesResponseContent: components["schemas"]["ClearanceSummary"];
        GetCollectionResponseContent: components["schemas"]["CollectionWithPostings"];
        GetContactNoteResponseContent: components["schemas"]["ContactNote"];
        GetContactResponseContent: components["schemas"]["ContactDetail"];
        GetEverythingTopicsResponseContent: components["schemas"]["TopicListResponse"];
        GetFeedboxResponseContent: components["schemas"]["BoxShowResponse"];
        GetFolderResponseContent: components["schemas"]["FolderWithPostings"];
        GetIdentityResponseContent: components["schemas"]["Identity"];
        GetImboxResponseContent: components["schemas"]["BoxShowResponse"];
        GetImboxSeenResponseContent: components["schemas"]["BoxShowResponse"];
        GetJournalEntryResponseContent: components["schemas"]["Recording"];
        GetLaterboxResponseContent: components["schemas"]["BoxShowResponse"];
        GetMessageEditResponseContent: components["schemas"]["MessageEditState"];
        GetMessageResponseContent: components["schemas"]["Message"];
        GetMyClearancesResponseContent: components["schemas"]["ClearanceListResponse"];
        GetNavigationResponseContent: components["schemas"]["NavigationResponse"];
        GetOngoingTimeTrackResponseContent: components["schemas"]["Recording"];
        GetSentTopicsResponseContent: components["schemas"]["TopicListResponse"];
        GetSpamTopicsResponseContent: components["schemas"]["TopicListResponse"];
        GetTopicEntriesResponseContent: components["schemas"]["Entry"][];
        GetTopicPublicationResponseContent: components["schemas"]["TopicPublication"];
        GetTopicResponseContent: components["schemas"]["Topic"];
        GetTrailboxResponseContent: components["schemas"]["BoxShowResponse"];
        GetTrashTopicsResponseContent: components["schemas"]["TopicListResponse"];
        GetWorkflowResponseContent: components["schemas"]["Workflow"];
        HabitPayload: {
            name?: string;
            icon?: string;
            color?: string;
            /** @description Days of the week the habit runs on, 0 for Sunday through 6 for Saturday */
            days?: number[];
        };
        /** @description Wire format: {calendar_habit: {name, icon, color, days: [0..6]}} */
        HabitRequestContent: {
            calendar_habit: components["schemas"]["HabitPayload"];
        };
        Identity: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            avatar_url?: string;
            icon_url?: string;
            time_zone?: string;
            time_zone_name?: string;
            /** Format: int32 */
            time_zone_offset?: number;
            auto_time_zone?: boolean;
            /** Format: int32 */
            first_week_day?: number;
            time_format?: string;
            primary_contact?: components["schemas"]["Contact"];
            all_users?: components["schemas"]["User"][];
            accounts?: components["schemas"]["Account"][];
            senders?: components["schemas"]["Sender"][];
        };
        InternalServerErrorResponseContent: {
            message: string;
        };
        /** @description JoinLink — video/meeting join link */
        JoinLink: {
            title?: string;
            url?: string;
        };
        JournalEntryPayload: {
            content: string;
        };
        ListBoxGroupsResponseContent: components["schemas"]["BoxGroupsResponse"];
        ListBoxesResponseContent: components["schemas"]["Box"][];
        ListCalendarDaysResponseContent: components["schemas"]["CalendarDayListPayload"];
        ListCalendarWeeksResponseContent: components["schemas"]["CalendarWeekListPayload"];
        ListCalendarsResponseContent: components["schemas"]["CalendarListPayload"];
        ListClipsResponseContent: components["schemas"]["Clip"][];
        ListCollectionsResponseContent: components["schemas"]["Collection"][];
        ListContactsResponseContent: components["schemas"]["Contact"][];
        ListDraftsResponseContent: components["schemas"]["DraftMessage"][];
        ListJournalEntriesResponseContent: components["schemas"]["Recording"][];
        ListSnippetsResponseContent: components["schemas"]["Snippet"][];
        ListStickiesResponseContent: components["schemas"]["Sticky"][];
        ListTimeTrackCategoriesResponseContent: components["schemas"]["TimeTrackCategory"][];
        ListTimeTracksResponseContent: components["schemas"]["TrackedTime"];
        MarkPostingsRequestContent: {
            posting_ids: (number | bigint)[];
        };
        /** @description Message — full message detail */
        Message: {
            /** Format: int64 */
            id: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            url?: string;
            creator?: components["schemas"]["Contact"];
            sender?: components["schemas"]["Contact"];
            is_reply?: boolean;
            subject?: string;
            content?: string;
            addressed?: components["schemas"]["Addressed"];
            show_addressed_selector?: boolean;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            scheduled_delivery_at?: string;
            posting?: components["schemas"]["MessagePostingContext"];
            addressed_sender?: components["schemas"]["AddressedSender"];
        };
        /**
         * @description Recipients per kind, each a list of email addresses.
         *     haystack applies Array() to each kind, so a JSON array is the correct wire format
         *     (a bare string would be treated as a single address, not split on commas).
         */
        MessageAddressed: {
            directly?: string[];
            copied?: string[];
            blindcopied?: string[];
        };
        /** @description MessageDraft — a prefilled compose payload (forward, reply). Unsent, so it has no id. */
        MessageDraft: {
            url?: string;
            creator?: components["schemas"]["Contact"];
            sender?: components["schemas"]["Contact"];
            is_reply?: boolean;
            subject?: string;
            content?: string;
            addressed?: components["schemas"]["Addressed"];
            show_addressed_selector?: boolean;
            posting?: components["schemas"]["MessagePostingContext"];
            addressed_sender?: components["schemas"]["AddressedSender"];
        };
        /**
         * @description MessageEditState — a saved draft as the editor sees it. The same compose fields as
         *     MessageDraft, plus the identity and scheduling a saved entry carries.
         */
        MessageEditState: {
            /** Format: int64 */
            id: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            url?: string;
            creator?: components["schemas"]["Contact"];
            sender?: components["schemas"]["Contact"];
            is_reply?: boolean;
            subject?: string;
            content?: string;
            addressed?: components["schemas"]["Addressed"];
            show_addressed_selector?: boolean;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            scheduled_delivery_at?: string;
            posting?: components["schemas"]["MessagePostingContext"];
            addressed_sender?: components["schemas"]["AddressedSender"];
        };
        MessageEntryPayload: {
            addressed?: components["schemas"]["MessageAddressed"];
            /**
             * @description "drafted" saves the entry as a draft instead of delivering it. Any other value
             *     (or omitting it) delivers through the undo-delay window.
             */
            status?: string;
            /**
             * @description "true" schedules delivery for the date and hour below; the entry stays drafted
             *     with a scheduled_delivery_at until then. On an update, omitting it clears an
             *     existing scheduled delivery.
             */
            scheduled_delivery?: string;
            /**
             * @description The delivery date: YYYY-MM-DD, "today" or "tomorrow", read in the identity's
             *     time zone.
             */
            scheduled_delivery_at_date?: string;
            /**
             * @description The delivery hour, "0" through "23" — a string so that midnight survives
             *     omitempty. HEY schedules to the hour.
             */
            scheduled_delivery_at_hour?: string;
        };
        MessagePayload: {
            subject: string;
            content: string;
        };
        /** @description MessagePostingContext — posting context for a message */
        MessagePostingContext: {
            box?: string;
        };
        MovePostingsRequestContent: {
            posting_ids: (number | bigint)[];
            /** Format: int64 */
            box_id: number | bigint;
        };
        /** @description Wire format: {id, position} — both at the top level. */
        MoveStickyRequestContent: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int32 */
            position: number;
        };
        MoveTopicRequestContent: {
            /** Format: int64 */
            box_id: number | bigint;
        };
        MoveWorkflowStagingRequestContent: {
            workflow_staging: components["schemas"]["WorkflowStagingPayload"];
        };
        /** @description NavigationIcon */
        NavigationIcon: {
            name?: string;
            android_url?: string;
            ios_url?: string;
        };
        /** @description NavigationItem */
        NavigationItem: {
            title?: string;
            app_url?: string;
            platform?: string;
            hotkey?: string;
            highlighted?: boolean;
            icon?: components["schemas"]["NavigationIcon"];
            menu_items?: components["schemas"]["NavigationItem"][];
        };
        /** @description NavigationResponse */
        NavigationResponse: {
            items?: components["schemas"]["NavigationItem"][];
            hotkeys?: components["schemas"]["NavigationItem"][];
        };
        NewBulkReplyResponseContent: components["schemas"]["BulkReplyDraft"];
        NewEntryForwardResponseContent: components["schemas"]["MessageDraft"];
        NewEntryReplyResponseContent: components["schemas"]["MessageDraft"];
        NotFoundErrorResponseContent: {
            message: string;
        };
        /** @description Organizer — calendar event organizer */
        Organizer: {
            email_address?: string;
            name?: string;
        };
        /** @description Posting — polymorphic by `kind` (topic, bundle, entry) */
        Posting: {
            /** Format: int64 */
            id: number | bigint;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            observed_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            active_at?: string;
            /** Format: int64 */
            box_id?: number | bigint;
            /** Format: int64 */
            account_id?: number | bigint;
            /** @description Discriminator: "topic", "bundle", or "entry" */
            kind: string;
            seen?: boolean;
            bundled?: boolean;
            muted?: boolean;
            note?: components["schemas"]["PostingNote"];
            preapproved_clearance?: boolean;
            /** Format: int64 */
            box_group_id?: number | bigint;
            includes_attachments?: boolean;
            includes_calendar_invites?: boolean;
            bubbled_up?: boolean;
            bubble_up_waiting_on?: boolean;
            bubble_up_schedule?: components["schemas"]["BubbleUpSchedule"];
            creator?: components["schemas"]["Contact"];
            app_url?: string;
            summary?: string;
            alternative_sender_name?: string;
            name?: string;
            blocked_trackers?: boolean;
            contacts?: components["schemas"]["Contact"][];
            extenzions?: components["schemas"]["Extenzion"][];
            folders?: components["schemas"]["Folder"][];
            collections?: components["schemas"]["Collection"][];
            workflows?: components["schemas"]["Workflow"][];
            /** Format: int32 */
            visible_entry_count?: number;
            entry_kind?: string;
            addressed_contacts?: components["schemas"]["Contact"][];
            app_bundle_url?: string;
        };
        /** @description Note — a posting note */
        PostingNote: {
            /** Format: int64 */
            id: number | bigint;
            content?: string;
        };
        /** @description Recording — polymorphic by `type`, with direct and namespaced calendar wire values */
        Recording: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            parent_id?: number | bigint;
            title?: string;
            all_day?: boolean;
            recurring?: boolean;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            starts_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            ends_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            /** @description Discriminator with direct (`CalendarTodo`) and namespaced (`Calendar::Todo`) values. */
            type: string;
            parent?: components["schemas"]["Recording"];
            starts_at_time_zone?: string;
            ends_at_time_zone?: string;
            reminders_label?: string;
            reminders?: components["schemas"]["Reminder"][];
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            completed_at?: string;
            highlighted?: boolean;
            recurrence_schedule?: components["schemas"]["RecurrenceSchedule"];
            occurrences_url?: string;
            occurrence_id?: string;
            calendar?: components["schemas"]["Calendar"];
            edit_url?: string;
            summary?: string;
            url?: string;
            location?: string;
            manage_attendance?: boolean;
            attendance_status?: string;
            organizer?: components["schemas"]["Organizer"];
            attendances?: components["schemas"]["Attendance"][];
            attendances_summary?: string;
            description?: string;
            join_link?: components["schemas"]["JoinLink"];
            attached_entry?: components["schemas"]["AttachedEntry"];
            /** Format: int32 */
            position?: number;
            content?: string;
            /**
             * @description Full rich-text HTML of a journal entry (GetJournalEntry / UpdateJournalEntry only;
             *     listings carry a truncated plain-text `content` instead).
             */
            content_html?: string;
            color?: string;
            icon?: string;
            days?: number[];
            icon_url?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            stopped_at?: string;
            notes?: string;
            /** @description HEY emits explicit JSON null when a time track has no category. */
            category?: string | null;
            label?: string;
            image_url?: string;
        };
        /** @description RecurrenceSchedule */
        RecurrenceSchedule: {
            kind?: string;
            description?: string;
            preset?: boolean;
        };
        /** @description Reminder */
        Reminder: {
            /** Format: int64 */
            id: number | bigint;
            summary?: string;
            /** Format: int32 */
            duration?: number;
            default_duration?: boolean;
            iso8601_duration?: string;
            delivered?: boolean;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            remind_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            label?: string;
        };
        /**
         * @description HEY does not derive a subject for a reply: a reply draft saved without message.subject
         *     reads "No subject" in Drafts. NewEntryReply hands back the prefilled subject ("Re: …") —
         *     send it here. Content is the caller's reply body alone: the server appends the quoted
         *     original at delivery (auto_quoting defaults on), so the prefill's quoted content must
         *     not be echoed back.
         */
        ReplyMessagePayload: {
            subject?: string;
            content: string;
        };
        RevealContactResponseContent: components["schemas"]["Contact"];
        SchedulePostingsBubbleUpRequestContent: {
            posting_ids: (number | bigint)[];
            slot: string;
            date?: string;
        };
        /** @description SearchFilterItem — one option offered by the advanced search refine form */
        SearchFilterItem: {
            title?: string;
            value?: string;
        };
        /** @description One matching topic: the topic, your posting of it (if any), and the entries that matched. */
        SearchMatch: {
            topic: components["schemas"]["Topic"];
            /** Format: int64 */
            posting_id?: number | bigint;
            entries?: components["schemas"]["Entry"][];
        };
        /** @description Sender — a contact with default flag */
        Sender: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            account_id?: number | bigint;
            name?: string;
            email_address?: string;
            avatar_url?: string;
            initials?: string;
            avatar_background_color?: string;
            contactable_type?: string;
            name_tag?: string;
            default?: boolean;
        };
        ServiceUnavailableErrorResponseContent: {
            message: string;
        };
        Snippet: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /** @description Plain text */
            content?: string;
            /** @description Rich-text HTML */
            content_html?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
        };
        StartTimeTrackResponseContent: components["schemas"]["Recording"];
        /** @description Sticky — a note on the stickies board */
        Sticky: {
            /** Format: int64 */
            id: number | bigint;
            body?: string;
            size?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
        };
        StickyPayload: {
            body?: string;
            size?: string;
        };
        /** @description Wire format: {sticky: {body, size}}. Size is "small", "medium" or "large". */
        StickyRequestContent: {
            sticky: components["schemas"]["StickyPayload"];
        };
        TimeFormatPreference: {
            /** @description "twelve_hour" or "twenty_four_hour", as GetIdentity serves it. */
            time_format: string;
        };
        TimeTrackCategory: {
            /** Format: int64 */
            id: number | bigint;
            title?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
        };
        TimeTrackRequestContent: {
            starts_at: string;
            ends_at: string;
            category_title?: string;
            notes?: string;
        };
        ToggleCalendarResponseContent: components["schemas"]["CalendarSelection"];
        /** @description Topic detail */
        Topic: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            active_at?: string;
            status?: string;
            /** Format: int64 */
            account_id?: number | bigint;
            app_url?: string;
            creator?: components["schemas"]["Contact"];
            contacts?: components["schemas"]["Contact"][];
            extenzions?: components["schemas"]["Extenzion"][];
            collections?: components["schemas"]["Collection"][];
            is_forged_sender?: boolean;
            latest_entry?: components["schemas"]["Entry"];
            /**
             * @description The topic's first page of entries (summaries, no bodies). Present on GetTopic; use
             *     GetTopicEntries for the rest and GetMessage for a body.
             */
            entries?: components["schemas"]["Entry"][];
        };
        /** @description TopicListResponse — wrapped topic list (sent, spam, trash, everything) */
        TopicListResponse: {
            title?: string;
            description?: string;
            topics?: components["schemas"]["Topic"][];
        };
        TopicPublication: {
            published: boolean;
            /** @description The public link, when published */
            url?: string;
        };
        /**
         * @description The tracked-time index: a page of completed tracks, and every category they can be
         *     filed under.
         */
        TrackedTime: {
            time_tracks?: components["schemas"]["Recording"][];
            categories?: components["schemas"]["TimeTrackCategory"][];
        };
        TrashPostingsRequestContent: {
            posting_ids: (number | bigint)[];
            /**
             * @description Omitted, JSON requests default to removing only your own access from shared topics.
             *     "false" trashes them for everyone instead.
             */
            remove_access?: string;
        };
        UnauthorizedErrorResponseContent: {
            message: string;
        };
        UncompleteCalendarTodoResponseContent: components["schemas"]["Recording"];
        UncompleteHabitResponseContent: components["schemas"]["Recording"];
        /**
         * @description The server rejected what was sent. HEY answers {"errors": ["..."]} — the messages
         *     the model itself produced — so a client can show them as they are.
         */
        UnprocessableEntityErrorResponseContent: {
            errors?: string[];
            message?: string;
        };
        /** @description Wire format: {calendar_todo: {title, starts_at, focused}} */
        UpdateCalendarTodoRequestContent: {
            calendar_todo: components["schemas"]["CalendarTodoChanges"];
        };
        UpdateCalendarTodoResponseContent: components["schemas"]["Recording"];
        /** @description Wire format: {status: "approved"|"denied"} — top level, not nested under a clearance key. */
        UpdateClearanceRequestContent: {
            status: string;
            /** Format: int64 */
            designation_box_id?: number | bigint;
            spam?: boolean;
            mark_topics_as_seen?: boolean;
        };
        UpdateClearanceResponseContent: components["schemas"]["Clearance"];
        /** @description Wire format: {collection: {name, summary}} */
        UpdateCollectionRequestContent: {
            collection: components["schemas"]["CollectionPayload"];
        };
        /** @description Wire format: {status: "approved"|"denied"} — top level, not nested under a clearance key. */
        UpdateContactClearanceRequestContent: {
            status: string;
        };
        UpdateContactNoteResponseContent: components["schemas"]["ContactNote"];
        UpdateContactResponseContent: components["schemas"]["Contact"];
        /** @description Wire format: {identity_preference: {first_week_day: "monday"}} */
        UpdateFirstWeekDayRequestContent: {
            identity_preference: components["schemas"]["FirstWeekDayParams"];
        };
        UpdateFirstWeekDayResponseContent: components["schemas"]["FirstWeekDayPreference"];
        UpdateHabitResponseContent: components["schemas"]["Recording"];
        /** @description Wire format: {calendar_journal_entry: {content}} */
        UpdateJournalEntryRequestContent: {
            calendar_journal_entry: components["schemas"]["JournalEntryPayload"];
        };
        UpdateJournalEntryResponseContent: components["schemas"]["Recording"];
        UpdateMyClearanceRequestContent: {
            status: string;
        };
        UpdateMyClearanceResponseContent: components["schemas"]["Clearance"];
        UpdateStickyResponseContent: components["schemas"]["Sticky"];
        UpdateTimeFormatRequestContent: {
            twenty_four_hour_time_format: boolean;
        };
        UpdateTimeFormatResponseContent: components["schemas"]["TimeFormatPreference"];
        UpdateTimeTrackPayload: {
            /**
             * @description Ignored by the server. A time track's title is the constant "Time Track";
             *     HEY dropped per-track titles in 2023. Kept for compatibility only.
             */
            title?: string;
            notes?: string;
            /**
             * @description Ignored by the server, which reads category_title instead. Kept for
             *     compatibility only.
             */
            category?: string;
            /**
             * @description Files the track under this category, creating the category if HEY does not
             *     have one by that name. Blank is a no-op, not a way to clear the category:
             *     once filed, a track can only be moved to another category, or left where it
             *     is by deleting the category itself.
             */
            category_title?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            starts_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            ends_at?: string;
        };
        /** @description Wire format: {calendar_time_track: {notes, category_title, starts_at, ends_at}} */
        UpdateTimeTrackRequestContent: {
            calendar_time_track: components["schemas"]["UpdateTimeTrackPayload"];
        };
        UpdateTimeTrackResponseContent: components["schemas"]["Recording"];
        /** @description UpdatesChannel — streaming channel for a box */
        UpdatesChannel: {
            signed_stream_name?: string;
        };
        /** @description User — a user within an account */
        User: {
            /** Format: int64 */
            id: number | bigint;
            /** Format: int64 */
            account_id?: number | bigint;
            account_purpose_icon_url?: string;
            contact?: components["schemas"]["Contact"];
            external_accounts?: components["schemas"]["ExternalAccount"][];
            auto_responder?: boolean;
        };
        /** @description Workflow — email workflow/label */
        Workflow: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            created_at?: string;
            /**
             * Format: date-time
             * @description ISO 8601 date-time timestamp (overrides restJson1 epoch-seconds default)
             */
            updated_at?: string;
            app_url?: string;
            /** @description The workflow's stages in position order. Present on GetWorkflow. */
            stages?: components["schemas"]["WorkflowStage"][];
        };
        WorkflowStage: {
            /** Format: int64 */
            id: number | bigint;
            name?: string;
        };
        WorkflowStagingPayload: {
            /** Format: int64 */
            workflow_stage_id: number | bigint;
        };
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    DeleteExtenzion: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                accountId: number | bigint;
                extenzionId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteExtenzion 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    AdvancedSearch: {
        parameters: {
            query?: {
                /** @description The words to search for */
                q?: string;
                page?: string;
                /**
                 * @description Refinements, e.g. refine[from], refine[to], refine[subject], refine[exact_phrase],
                 *     refine[required], refine[any], refine[none], refine[date], refine[in], refine[label],
                 *     refine[attachment] — passed through as the page sends them.
                 */
                "refine[from]"?: string;
                "refine[to]"?: string;
                "refine[subject]"?: string;
                "refine[exact_phrase]"?: string;
                "refine[required]"?: string;
                "refine[any]"?: string;
                "refine[none]"?: string;
                "refine[date]"?: string;
                "refine[in]"?: string;
                "refine[label]"?: string;
                "refine[attachment]"?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description AdvancedSearch 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AdvancedSearchResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetAdvancedSearchFilters: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetAdvancedSearchFilters 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetAdvancedSearchFiltersResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListBoxes: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListBoxes 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListBoxesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetBox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                boxId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetBox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetBoxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateBoxDesignation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                boxId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateBoxDesignationRequestContent"];
            };
        };
        responses: {
            /** @description CreateBoxDesignation 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteBoxDesignation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                boxId: number | bigint;
                designationId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteBoxDesignation 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListBoxGroups: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                boxId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListBoxGroups 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListBoxGroupsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateBoxGroup: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                boxId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateBoxGroupRequestContent"];
            };
        };
        responses: {
            /** @description CreateBoxGroup 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateBoxGroupResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetBoxGroup: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                boxId: number | bigint;
                groupId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetBoxGroup 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetBoxGroupResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteBoxGroup: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                boxId: number | bigint;
                groupId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteBoxGroup 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MarkBoxSeen: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                boxId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description MarkBoxSeen 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetBoxPostingChanges: {
        parameters: {
            query: {
                since: string;
                v?: string;
                page?: string;
                per_page?: string;
            };
            header?: never;
            path: {
                boxId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetBoxPostingChanges 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetBoxPostingChangesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description ConflictError 409 response */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConflictErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetBubblebox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetBubblebox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetBubbleboxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateBulkReply: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["BulkReplyRequestContent"];
            };
        };
        responses: {
            /** @description CreateBulkReply 201 response */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateBulkReplyResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    NewBulkReply: {
        parameters: {
            query: {
                /** @description The postings to reply to, comma separated. */
                posting_ids: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description NewBulkReply 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NewBulkReplyResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListCalendarDays: {
        parameters: {
            query?: {
                /** @description Date (YYYY-MM-DD) to start from. Defaults to today. */
                starts_at?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListCalendarDays 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListCalendarDaysResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetCalendarDay: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                day: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetCalendarDay 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetCalendarDayResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CompleteHabit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                day: string;
                habitId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description CompleteHabit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CompleteHabitResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UncompleteHabit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                day: string;
                habitId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description UncompleteHabit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UncompleteHabitResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetJournalEntry: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                day: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetJournalEntry 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetJournalEntryResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateJournalEntry: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                day: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateJournalEntryRequestContent"];
            };
        };
        responses: {
            /** @description UpdateJournalEntry 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateJournalEntryResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteCalendarEvent: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                eventId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteCalendarEvent 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteCalendarEventOccurrence: {
        parameters: {
            query?: {
                /** @description Remove this day and every one after it. Off, only this day is removed. */
                apply_to_future?: boolean;
            };
            header?: never;
            path: {
                eventId: number | bigint;
                /** @description The day the occurrence falls on, as YYYY-MM-DD. */
                occurrence: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteCalendarEventOccurrence 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateHabit: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HabitRequestContent"];
            };
        };
        responses: {
            /** @description CreateHabit 201 response */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateHabitResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteHabit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                habitId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteHabit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateHabit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                habitId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HabitRequestContent"];
            };
        };
        responses: {
            /** @description UpdateHabit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateHabitResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    StopHabit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                habitId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description StopHabit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ResumeHabit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                habitId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ResumeHabit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateFirstWeekDay: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateFirstWeekDayRequestContent"];
            };
        };
        responses: {
            /** @description UpdateFirstWeekDay 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateFirstWeekDayResponseContent"];
                };
            };
            /** @description BadRequestError 400 response */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BadRequestErrorResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListJournalEntries: {
        parameters: {
            query?: {
                page?: string;
                q?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListJournalEntries 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListJournalEntriesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetOngoingTimeTrack: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetOngoingTimeTrack 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetOngoingTimeTrackResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    StartTimeTrack: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description StartTimeTrack 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["StartTimeTrackResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ConflictError 409 response */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConflictErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListTimeTracks: {
        parameters: {
            query?: {
                page?: string;
                category_id?: number | bigint;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListTimeTracks 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListTimeTracksResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateTimeTrack: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TimeTrackRequestContent"];
            };
        };
        responses: {
            /** @description CreateTimeTrack 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateTimeTrackResponseContent"];
                };
            };
            /** @description BadRequestError 400 response */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BadRequestErrorResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListTimeTrackCategories: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListTimeTrackCategories 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListTimeTrackCategoriesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateTimeTrack: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                timeTrackId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateTimeTrackRequestContent"];
            };
        };
        responses: {
            /** @description UpdateTimeTrack 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateTimeTrackResponseContent"];
                };
            };
            /** @description BadRequestError 400 response */
            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BadRequestErrorResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteTimeTrack: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                timeTrackId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteTimeTrack 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateCalendarTodo: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateCalendarTodoRequestContent"];
            };
        };
        responses: {
            /** @description CreateCalendarTodo 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateCalendarTodoResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteCalendarTodo: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                todoId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteCalendarTodo 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateCalendarTodo: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                todoId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateCalendarTodoRequestContent"];
            };
        };
        responses: {
            /** @description UpdateCalendarTodo 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateCalendarTodoResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CompleteCalendarTodo: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                todoId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description CompleteCalendarTodo 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CompleteCalendarTodoResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UncompleteCalendarTodo: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                todoId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description UncompleteCalendarTodo 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UncompleteCalendarTodoResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListCalendarWeeks: {
        parameters: {
            query?: {
                /** @description Date (YYYY-MM-DD) of the first week. Takes precedence over centered_at. */
                starts_at?: string;
                /** @description Date (YYYY-MM-DD) to center the nine weeks on. Defaults to today. */
                centered_at?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListCalendarWeeks 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListCalendarWeeksResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetCalendarWeek: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description Any date in the week (YYYY-MM-DD) */
                week: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetCalendarWeek 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetCalendarWeekResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetCalendarYear: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description Any date in the year (YYYY-MM-DD) */
                year: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetCalendarYear 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetCalendarYearResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListCalendars: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListCalendars 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListCalendarsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetCalendarRecordings: {
        parameters: {
            query?: {
                starts_on?: string;
                ends_on?: string;
                page?: string;
            };
            header?: never;
            path: {
                calendarId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetCalendarRecordings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetCalendarRecordingsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ToggleCalendar: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                calendarId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ToggleCalendar 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ToggleCalendarResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetClearances: {
        parameters: {
            query?: {
                include_clearances?: boolean;
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetClearances 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetClearancesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    BulkUpdateClearances: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["BulkUpdateClearancesRequestContent"];
            };
        };
        responses: {
            /** @description BulkUpdateClearances 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BulkUpdateClearancesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    PuntClearances: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description PuntClearances 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateClearance: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                clearanceId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateClearanceRequestContent"];
            };
        };
        responses: {
            /** @description UpdateClearance 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateClearanceResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListClips: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListClips 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListClipsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListCollections: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListCollections 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListCollectionsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetCollection: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                collectionId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetCollection 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetCollectionResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateCollection: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                collectionId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateCollectionRequestContent"];
            };
        };
        responses: {
            /** @description UpdateCollection 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListContacts: {
        parameters: {
            query?: {
                page?: string;
                q?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListContacts 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListContactsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateContact: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateContactRequestContent"];
            };
        };
        responses: {
            /** @description CreateContact 201 response */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateContactResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ConflictError 409 response */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConflictErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetContact: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetContact 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetContactResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    HideContact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description HideContact 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateContact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ContactRequestContent"];
            };
        };
        responses: {
            /** @description UpdateContact 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateContactResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description ConflictError 409 response */
            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConflictErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    BundleContact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description BundleContact 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UnbundleContact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description UnbundleContact 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateContactClearance: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateContactClearanceRequestContent"];
            };
        };
        responses: {
            /** @description UpdateContactClearance 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetContactNote: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetContactNote 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetContactNoteResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteContactNote: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteContactNote 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateContactNote: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ContactNoteRequestContent"];
            };
        };
        responses: {
            /** @description UpdateContactNote 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateContactNoteResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    RevealContact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                contactId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description RevealContact 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RevealContactResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListDrafts: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListDrafts 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListDraftsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteDraft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                entryId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteDraft 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    NewEntryForward: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                entryId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description NewEntryForward 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NewEntryForwardResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateReply: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                entryId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateReplyRequestContent"];
            };
        };
        responses: {
            /** @description CreateReply 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    NewEntryReply: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                entryId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description NewEntryReply 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NewEntryReplyResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MarkEntrySpam: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                entryId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description MarkEntrySpam 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetFeedbox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetFeedbox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetFeedboxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetFolder: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                folderId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetFolder 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetFolderResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetIdentity: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetIdentity 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetIdentityResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateTimeFormat: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateTimeFormatRequestContent"];
            };
        };
        responses: {
            /** @description UpdateTimeFormat 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateTimeFormatResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetImbox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetImbox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetImboxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetImboxSeen: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetImboxSeen 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetImboxSeenResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateMessage: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateMessageRequestContent"];
            };
        };
        responses: {
            /** @description CreateMessage 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetMessage: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                messageId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetMessage 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetMessageResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateMessage: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                messageId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateMessageRequestContent"];
            };
        };
        responses: {
            /** @description UpdateMessage 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetMessageEdit: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                messageId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetMessageEdit 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetMessageEditResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetMyClearances: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetMyClearances 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetMyClearancesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateMyClearance: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                clearanceId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateMyClearanceRequestContent"];
            };
        };
        responses: {
            /** @description UpdateMyClearance 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateMyClearanceResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetNavigation: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetNavigation 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetNavigationResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetTrailbox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetTrailbox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetTrailboxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    AddPostingsToBoxGroup: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AddPostingsToBoxGroupRequestContent"];
            };
        };
        responses: {
            /** @description AddPostingsToBoxGroup 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    RemovePostingsFromBoxGroup: {
        parameters: {
            query: {
                /** @description Posting ids as a comma-joined string, for verbs that carry no body */
                posting_ids: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description RemovePostingsFromBoxGroup 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    SchedulePostingsBubbleUp: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["SchedulePostingsBubbleUpRequestContent"];
            };
        };
        responses: {
            /** @description SchedulePostingsBubbleUp 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CancelPostingsBubbleUp: {
        parameters: {
            query: {
                /** @description Posting ids as a comma-joined string, for verbs that carry no body */
                posting_ids: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description CancelPostingsBubbleUp 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    BubbleUpPostingsNow: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MarkPostingsRequestContent"];
            };
        };
        responses: {
            /** @description BubbleUpPostingsNow 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    FilePostings: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["FilePostingsRequestContent"];
            };
        };
        responses: {
            /** @description FilePostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UnfilePostings: {
        parameters: {
            query: {
                /** @description Posting ids as a comma-joined string, for verbs that carry no body */
                posting_ids: string;
                folder_id?: number | bigint;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description UnfilePostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateFolderForPostings: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateFolderForPostingsRequestContent"];
            };
        };
        responses: {
            /** @description CreateFolderForPostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MovePostings: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MovePostingsRequestContent"];
            };
        };
        responses: {
            /** @description MovePostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MutePostings: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MarkPostingsRequestContent"];
            };
        };
        responses: {
            /** @description MutePostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UnmutePostings: {
        parameters: {
            query: {
                /** @description Comma-separated posting IDs, e.g. "123,456" */
                posting_ids: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description UnmutePostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MarkPostingsSeen: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MarkPostingsRequestContent"];
            };
        };
        responses: {
            /** @description MarkPostingsSeen 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MarkPostingsSpam: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MarkPostingsRequestContent"];
            };
        };
        responses: {
            /** @description MarkPostingsSpam 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    TrashPostings: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TrashPostingsRequestContent"];
            };
        };
        responses: {
            /** @description TrashPostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MarkPostingsUnseen: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MarkPostingsRequestContent"];
            };
        };
        responses: {
            /** @description MarkPostingsUnseen 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetBundleUnseenPostings: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                postingId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetBundleUnseenPostings 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetBundleUnseenPostingsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateDirectUpload: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateDirectUploadRequestContent"];
            };
        };
        responses: {
            /** @description CreateDirectUpload 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DirectUpload"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetLaterbox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetLaterbox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetLaterboxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetAsidebox: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetAsidebox 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetAsideboxResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListSnippets: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListSnippets 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListSnippetsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    ListStickies: {
        parameters: {
            query?: {
                /** @description Clamped server-side to 1..100 */
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description ListStickies 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ListStickiesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateSticky: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["StickyRequestContent"];
            };
        };
        responses: {
            /** @description CreateSticky 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreateStickyResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MoveSticky: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MoveStickyRequestContent"];
            };
        };
        responses: {
            /** @description MoveSticky 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    DeleteSticky: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                stickyId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description DeleteSticky 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    UpdateSticky: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                stickyId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["StickyRequestContent"];
            };
        };
        responses: {
            /** @description UpdateSticky 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UpdateStickyResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetEverythingTopics: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetEverythingTopics 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetEverythingTopicsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetSentTopics: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetSentTopics 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetSentTopicsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetSpamTopics: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetSpamTopics 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetSpamTopicsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    EmptySpam: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description EmptySpam 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetTrashTopics: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetTrashTopics 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetTrashTopicsResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    EmptyTrash: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description EmptyTrash 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetTopic: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetTopic 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetTopicResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetTopicEntries: {
        parameters: {
            query?: {
                page?: string;
            };
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetTopicEntries 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetTopicEntriesResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MoveTopic: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MoveTopicRequestContent"];
            };
        };
        responses: {
            /** @description MoveTopic 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetTopicPublication: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetTopicPublication 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetTopicPublicationResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    RestoreTopic: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description RestoreTopic 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MarkTopicHam: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description MarkTopicHam 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    TrashTopic: {
        parameters: {
            query?: {
                confirm_destroy?: string;
            };
            header?: never;
            path: {
                topicId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description TrashTopic 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    CreateWorkflowStaging: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
                workflowId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description CreateWorkflowStaging 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    MoveWorkflowStaging: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                topicId: number | bigint;
                workflowId: number | bigint;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MoveWorkflowStagingRequestContent"];
            };
        };
        responses: {
            /** @description MoveWorkflowStaging 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description ForbiddenError 403 response */
            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ForbiddenErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description UnprocessableEntityError 422 response */
            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnprocessableEntityErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
    GetWorkflow: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                workflowId: number | bigint;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description GetWorkflow 200 response */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GetWorkflowResponseContent"];
                };
            };
            /** @description UnauthorizedError 401 response */
            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UnauthorizedErrorResponseContent"];
                };
            };
            /** @description NotFoundError 404 response */
            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["NotFoundErrorResponseContent"];
                };
            };
            /** @description InternalServerError 500 response */
            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InternalServerErrorResponseContent"];
                };
            };
            /** @description ServiceUnavailableError 503 response */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ServiceUnavailableErrorResponseContent"];
                };
            };
        };
    };
}
