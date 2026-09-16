import Foundation

/// A partial revision of an event, and partial only in what it names: the six fields here are
/// sent when set and HEY leaves the title, dates and times alone otherwise. The notes, location,
/// link, attached entry, attendees, reminders and countdown a caller says nothing about are
/// cleared, because HEY reads every calendar write as a form and defaults each of those to
/// nothing. A revision that keeps any of them goes through ``UpdateCalendarEventParams`` and
/// ``CalendarEventsService/updateEvent(eventId:params:)``, which take them all.
///
/// Clock times belong to a timed event, so an all-day revision leaves them off however they are
/// set here.
public struct CalendarEventUpdate: Sendable, Equatable {
    /// The event's title, HEY's summary.
    public var title: String?
    /// `YYYY-MM-DD`.
    public var startsAt: String?
    /// `YYYY-MM-DD`.
    public var endsAt: String?
    /// Whether the event takes the whole day rather than a clock time.
    public var allDay: Bool?
    /// `HH:MM`.
    public var startTime: String?
    /// `HH:MM`.
    public var endTime: String?

    /// A revision naming only the fields given.
    public init(
        title: String? = nil, startsAt: String? = nil, endsAt: String? = nil, allDay: Bool? = nil, startTime: String? = nil,
        endTime: String? = nil
    ) {
        self.title = title
        self.startsAt = startsAt
        self.endsAt = endsAt
        self.allDay = allDay
        self.startTime = startTime
        self.endTime = endTime
    }
}

/// An event's content, which is a replacement rather than a patch.
///
/// HEY reads these four out of the submitted parameters and then defaults every one of them to
/// nothing, so a write that says nothing about a field clears it. There is no way to send a
/// subset: the fields left empty here are the fields the event loses. An update therefore has to
/// read the event first and pass back whatever it means to keep.
///
/// The title is not in here, and that is not an oversight: HEY leaves the summary alone when it
/// is not submitted, so it stays a partial field like the rest of a partial write.
public struct EventContent: Sendable, Equatable {
    /// HEY's `calendar_event[description]` — Trix rich text, so HTML going in. It does not
    /// round-trip: HEY serves the notes back as plain text, so keeping formatted notes through an
    /// update means holding the HTML the caller sent, not the text HEY answered.
    public var notes: String
    /// A plain string. HEY truncates it at 3900 characters rather than refusing it.
    public var location: String
    /// Validated as a URL and capped at 2500 characters, so a malformed one is a 422 rather than
    /// a silent drop.
    public var link: String?
    /// The email attached to the event, HEY's `calendar_event[entry_id]`. A read serves it back as
    /// `attached_entry`.
    public var entryId: Int?

    /// Content with the fields given, and nothing in the rest.
    public init(notes: String = "", location: String = "", link: String? = nil, entryId: Int? = nil) {
        self.notes = notes
        self.location = location
        self.link = link
        self.entryId = entryId
    }
}

/// A countdown's unit, written as the number of seconds HEY's own form submits and the only form
/// it reads.
public enum CountdownUnit: Sendable, Equatable, CaseIterable {
    /// A day: 86,400 seconds.
    case days
    /// A week: 604,800 seconds.
    case weeks
    /// A month as HEY averages one: 2,629,746 seconds.
    case months

    /// The unit as the number of seconds HEY reads it as.
    public var seconds: Int {
        switch self {
        case .days: return 86_400
        case .weeks: return 604_800
        case .months: return 2_629_746
        }
    }
}

/// The countdown HEY runs up to an event. Like ``EventContent`` it is resend-or-lose-it: HEY reads
/// the pair on every editable write and a missing value deletes the countdown, so a zero
/// ``value`` means the event has none once the write lands. A countdown is a child recording
/// rather than a field on the event, so it cannot be read back from one. 1 through 30 is what the
/// web app offers.
public struct Countdown: Sendable, Equatable {
    /// How many units. Zero is no countdown.
    public var value: Int
    /// What the value counts in.
    public var unit: CountdownUnit

    /// A countdown of `value` units; the default is none.
    public init(value: Int = 0, unit: CountdownUnit = .days) {
        self.value = value
        self.unit = unit
    }
}

/// How often an event repeats. HEY has no day-of-week parameter, so ``everyWeekday`` — a
/// hardcoded Monday to Friday — is the only weekday set that can be expressed.
public enum RepeatFrequency: String, Sendable, Equatable, CaseIterable {
    /// Every day.
    case everyDay = "every_day"
    /// Monday to Friday.
    case everyWeekday = "every_weekday"
    /// Every week, on the event's day.
    case everyWeek = "every_week"
    /// Every other week, on the event's day.
    case everyOtherWeek = "every_other_week"
    /// Every month, on the event's day of the month.
    case everyDayOfMonth = "every_day_of_month"
    /// Every year, on the event's date.
    case everyYear = "every_year"
    /// Keeps whatever schedule the event already has instead of naming a new one.
    case custom = "custom"
}

/// When a recurrence stops.
public enum RepeatUntil: String, Sendable, Equatable, CaseIterable {
    /// The event never stops repeating.
    case forever = "forever"
    /// It stops after ``Repeat/untilDate``.
    case date = "date"
    /// It stops after ``Repeat/count`` occurrences.
    case count = "count"
}

/// An event's recurrence. The default is ``RepeatFrequency/custom`` with no end, which says "keep
/// the schedule the event already has". On an occurrence update that makes a default `Repeat` and
/// a nil one mean the same thing, since
/// ``CalendarEventsService/updateOccurrence(occurrence:scope:params:)`` sends `custom` for a nil
/// one anyway. On a whole-event update a nil writes no recurrence field at all.
public struct Repeat: Sendable, Equatable {
    /// How often the event repeats.
    public var frequency: RepeatFrequency
    /// When it stops. Nil says nothing about an end.
    public var until: RepeatUntil?
    /// `YYYY-MM-DD`, read only when ``until`` is ``RepeatUntil/date``.
    public var untilDate: String?
    /// Read only when ``until`` is ``RepeatUntil/count``.
    public var count: Int?

    /// A recurrence; the default keeps the schedule the event already has.
    public init(frequency: RepeatFrequency = .custom, until: RepeatUntil? = nil, untilDate: String? = nil, count: Int? = nil) {
        self.frequency = frequency
        self.until = until
        self.untilDate = untilDate
        self.count = count
    }
}

/// A new calendar event. Nothing exists to lose on a create, so the resend-or-lose-it fields of an
/// update — the content, reminders and countdown — default to an event with none of them.
public struct CreateCalendarEventParams: Sendable, Equatable {
    /// The calendar the event is filed on.
    public var calendarId: Int
    /// The event's title, HEY's summary.
    public var title: String
    /// `YYYY-MM-DD`.
    public var startsAt: String
    /// `YYYY-MM-DD`. Defaults to ``startsAt``.
    public var endsAt: String?
    /// Whether the event takes the whole day rather than a clock time.
    public var allDay: Bool
    /// `HH:MM`, required unless the event is all-day.
    public var startTime: String?
    /// `HH:MM`, required unless the event is all-day.
    public var endTime: String?
    /// The IANA name of the zone the start is written in — `Europe/Zagreb`, `America/New_York`.
    /// Leave both zones nil and the times are read in UTC, which is the zone HEY parses an API
    /// request in. HEY keeps a zone per end, as its own form offers, so an event can start in one
    /// and finish in another; one zone named stands in for the other.
    public var startTimeZone: String?
    /// The zone the end is written in, read as ``startTimeZone`` is.
    public var endTimeZone: String?
    /// How long before the event each reminder goes out. HEY takes several in one write and
    /// de-duplicates them, and accepts any duration rather than only the presets the web app
    /// offers. Only the list matching ``allDay`` is read, and an empty list is an event with no
    /// reminders.
    public var reminders: [Duration]
    /// The notes, location, link and attached entry.
    public var content: EventContent
    /// The guest list. Submitting one makes the caller the organizer and sends invitations.
    public var attendees: [String]?
    /// Circles the event. HEY reads it only when it is submitted, so nil is "not circled" on a
    /// create.
    public var highlighted: Bool?
    /// Counts down to the event. The default creates none.
    public var countdown: Countdown
    /// Makes the event recurring. Nil is a one-off.
    public var `repeat`: Repeat?

    /// A new event on a calendar, with a title and a day.
    public init(
        calendarId: Int, title: String, startsAt: String, endsAt: String? = nil, allDay: Bool = false, startTime: String? = nil,
        endTime: String? = nil, startTimeZone: String? = nil, endTimeZone: String? = nil, reminders: [Duration] = [],
        content: EventContent = EventContent(), attendees: [String]? = nil, highlighted: Bool? = nil,
        countdown: Countdown = Countdown(), repeat: Repeat? = nil
    ) {
        self.calendarId = calendarId
        self.title = title
        self.startsAt = startsAt
        self.endsAt = endsAt
        self.allDay = allDay
        self.startTime = startTime
        self.endTime = endTime
        self.startTimeZone = startTimeZone
        self.endTimeZone = endTimeZone
        self.reminders = reminders
        self.content = content
        self.attendees = attendees
        self.highlighted = highlighted
        self.countdown = countdown
        self.repeat = `repeat`
    }
}

/// A revision of a calendar event, whole. The optional fields are a partial update: only the ones
/// named are sent, and HEY leaves the rest as they are.
///
/// The rest are not, and the reason is on HEY's side. It reads the zones, the content, the
/// reminders and the countdown out of the submitted parameters on every write and defaults each of
/// them to nothing, so an update saying nothing about one clears it. A caller keeping any of them
/// reads the event and sends them back.
public struct UpdateCalendarEventParams: Sendable, Equatable {
    /// Moves the event to another calendar. It has to be one the identity can file on — owned or
    /// shared, not a subscription; the personal calendar answers 404 all the same.
    public var calendarId: Int?
    /// The event's title, HEY's summary.
    public var title: String?
    /// `YYYY-MM-DD`.
    public var startsAt: String?
    /// `YYYY-MM-DD`.
    public var endsAt: String?
    /// Whether the event takes the whole day rather than a clock time.
    public var allDay: Bool?
    /// `HH:MM`. Clock times belong to a timed event, so an all-day revision leaves them off
    /// however they are set here.
    public var startTime: String?
    /// `HH:MM`.
    public var endTime: String?
    /// The IANA zone the start is written in, as on a create. An empty string says the time is UTC
    /// and clears the zone the event was saved with; nil leaves it out of the request, which HEY
    /// also reads as clearing it.
    public var startTimeZone: String?
    /// The zone the end is written in, read as ``startTimeZone`` is.
    public var endTimeZone: String?
    /// Resend-or-lose-it, like the zones: HEY reads the list on every write and unschedules
    /// everything when it is empty. Several go in one write and HEY de-duplicates them; only the
    /// list matching the event's all-day flag is read.
    public var reminders: [Duration]
    /// The notes, location, link and attached entry, a replacement rather than a patch. Read
    /// ``EventContent`` before using it.
    public var content: EventContent
    /// Replaces the guest list. Nil leaves it alone; an empty list removes every guest.
    public var attendees: [String]?
    /// Circles or uncircles the event. Nil leaves it as it is.
    public var highlighted: Bool?
    /// Resend-or-lose-it too: a zero value deletes the event's countdown.
    public var countdown: Countdown
    /// Changes the recurrence. Nil leaves it untouched on a whole-event update, and keeps the
    /// series' schedule on an occurrence update.
    public var `repeat`: Repeat?

    /// A revision naming the fields given, and clearing the resend-or-lose-it ones left out.
    public init(
        calendarId: Int? = nil, title: String? = nil, startsAt: String? = nil, endsAt: String? = nil, allDay: Bool? = nil,
        startTime: String? = nil, endTime: String? = nil, startTimeZone: String? = nil, endTimeZone: String? = nil,
        reminders: [Duration] = [], content: EventContent = EventContent(), attendees: [String]? = nil,
        highlighted: Bool? = nil, countdown: Countdown = Countdown(), repeat: Repeat? = nil
    ) {
        self.calendarId = calendarId
        self.title = title
        self.startsAt = startsAt
        self.endsAt = endsAt
        self.allDay = allDay
        self.startTime = startTime
        self.endTime = endTime
        self.startTimeZone = startTimeZone
        self.endTimeZone = endTimeZone
        self.reminders = reminders
        self.content = content
        self.attendees = attendees
        self.highlighted = highlighted
        self.countdown = countdown
        self.repeat = `repeat`
    }
}

/// One day of a repeating event, addressed by the series it belongs to plus the day it falls on,
/// as HEY's `occurrence_id` (`<event id>_<YYYY-MM-DD>`) names it.
public struct OccurrenceId: Sendable, Equatable, Hashable, CustomStringConvertible {
    /// The event the series is.
    public let eventId: Int
    /// The day the occurrence falls on, `YYYY-MM-DD`.
    public let date: String

    /// An occurrence of an event on a day.
    ///
    /// - Throws: ``HeyError/usage(message:hint:)`` when the event id is not positive or the date
    ///   is not a day the calendar has: both go into an authenticated write's path.
    public init(eventId: Int, date: String) throws {
        // Both parts go into an authenticated write's path, so both are checked here, whichever
        // way the id was made: a positive event, and a day the calendar actually has.
        guard eventId > 0 else { throw HeyError.usage(message: "occurrence id names no event: \(eventId)") }
        guard Self.isCalendarDate(date) else {
            throw HeyError.usage(message: "occurrence id names no date: \"\(date)\" is not a YYYY-MM-DD the calendar has")
        }
        self.eventId = eventId
        self.date = date
    }

    /// Reads HEY's `occurrence_id`.
    public static func parse(_ source: String) throws -> OccurrenceId {
        guard let separator = source.firstIndex(of: "_") else {
            throw HeyError.usage(message: "occurrence id \"\(source)\" is not <event id>_<YYYY-MM-DD>")
        }
        guard let eventId = Int(source[..<separator]), eventId > 0 else {
            throw HeyError.usage(message: "occurrence id \"\(source)\" names no event")
        }
        return try OccurrenceId(eventId: eventId, date: String(source[source.index(after: separator)...]))
    }

    /// Whether the text is a `YYYY-MM-DD` the calendar has: a month of 1 to 12 and a day that
    /// month has, February 29 in leap years only.
    static func isCalendarDate(_ date: String) -> Bool {
        let parts = date.split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0].count == 4, parts[1].count == 2, parts[2].count == 2,
              parts.allSatisfy({ $0.unicodeScalars.allSatisfy { $0.value >= 0x30 && $0.value <= 0x39 } }),
              let year = Int(parts[0]), let month = Int(parts[1]), let day = Int(parts[2])
        else { return false }
        guard (1...12).contains(month), day >= 1 else { return false }
        let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
        let days: Int
        switch month {
        case 2: days = leap ? 29 : 28
        case 4, 6, 9, 11: days = 30
        default: days = 31
        }
        return day <= days
    }

    /// The id as HEY writes it: `<event id>_<YYYY-MM-DD>`.
    public var description: String { "\(eventId)_\(date)" }
}

/// How much of a repeating event a write to one of its occurrences reaches.
public enum OccurrenceScope: String, Sendable, Equatable, CaseIterable {
    /// The named day alone.
    case thisOnly = "this_event"
    /// The named day and every one after it.
    case thisAndFollowing = "this_and_following"

    /// Reads the scope as HEY writes it.
    public static func parse(_ source: String) throws -> OccurrenceScope {
        guard let scope = OccurrenceScope(rawValue: source) else {
            throw HeyError.usage(message: "occurrence scope \"\(source)\" is neither this_event nor this_and_following")
        }
        return scope
    }
}

/// Calendar events with the form-backed writes on top of the generated surface (`delete`,
/// `deleteOccurrence`). HEY's calendar writes are Rails form posts rather than JSON, so a create or
/// an update goes out form-encoded under `calendar_event[...]`.
/// ``updateEvent(eventId:params:)`` and ``updateOccurrence(occurrence:scope:params:)`` take the
/// whole of an event, as Go's and Rust's do; ``update(eventId:update:)`` names the six fields a
/// revision usually means, and clears the rest — read it before using it.
extension CalendarEventsService {
    /// Creates an event and answers it as a recording. An event needs a title and a day, and a
    /// timed one both clock times; those are refused here rather than sent for HEY to refuse.
    public func create(params: CreateCalendarEventParams) async throws -> Recording {
        if params.title.isEmpty { throw HeyError.usage(message: "a calendar event needs a title") }
        if params.startsAt.isEmpty { throw HeyError.usage(message: "a calendar event needs a day: startsAt is required") }
        if !params.allDay && ((params.startTime ?? "").isEmpty || (params.endTime ?? "").isEmpty) {
            throw HeyError.usage(message: "a timed calendar event needs a start and an end time; all-day events take neither")
        }
        return try await calendarWrite(
            .post, "/calendar/events.json",
            writeInfo(service: "CalendarEvents", operation: "CreateCalendarEvent", resourceType: "calendar_event"),
            createEventFields(params))
    }

    /// Revises an event's title, dates and times, and nothing else — partial only in what it
    /// names. HEY reads a calendar write out of submitted form parameters and defaults the notes,
    /// location, link, attached entry, attendees, reminders and countdown to nothing, so every one
    /// of those the event had is cleared by this call. A revision that keeps any of them goes
    /// through ``updateEvent(eventId:params:)``, which takes them all; read the event first and
    /// send back what it should keep.
    public func update(eventId: Int, update: CalendarEventUpdate) async throws {
        var operation = client.request(.patch, "/calendar/events/\(eventId)")
        operation.info = writeInfo(service: "CalendarEvents", operation: "UpdateCalendarEvent", resourceType: "calendar_event", resourceId: eventId)
        operation.form(eventUpdateFields(update))
        operation.accept = "application/json"
        try await client.sendVoid(operation)
    }

    /// Revises an event from the whole of `params` and answers it as a recording.
    public func updateEvent(eventId: Int, params: UpdateCalendarEventParams) async throws -> Recording {
        try await calendarWrite(
            .patch, "/calendar/events/\(eventId).json",
            writeInfo(service: "CalendarEvents", operation: "UpdateCalendarEvent", resourceType: "calendar_event", resourceId: eventId),
            updateEventFields(params))
    }

    /// Revises one day of a repeating event and answers it as a recording. A date that is not an
    /// occurrence of that series is a 404, as is an event the caller cannot edit. A nil
    /// ``UpdateCalendarEventParams/repeat`` keeps the series' schedule: HEY would read silence
    /// here as "stop repeating".
    public func updateOccurrence(occurrence: OccurrenceId, scope: OccurrenceScope, params: UpdateCalendarEventParams) async throws -> Recording {
        var fields = updateEventFields(params)
        fields.append(("apply_to_future", checkbox(scope == .thisAndFollowing)))
        if params.repeat == nil { fields.append(("repeat_frequency", RepeatFrequency.custom.rawValue)) }
        return try await calendarWrite(
            .patch, "/calendar/events/\(occurrence.eventId)/occurrences/\(occurrence.date).json",
            writeInfo(
                service: "CalendarEvents", operation: "UpdateCalendarEventOccurrence", resourceType: "calendar_event",
                resourceId: occurrence.eventId),
            fields)
    }

    /// Removes one day of a repeating event, or that day and every one after it. The generated
    /// ``deleteOccurrence(eventId:occurrence:options:)`` takes the same request in its parts.
    public func deleteOccurrenceScoped(occurrence: OccurrenceId, scope: OccurrenceScope) async throws {
        try await deleteOccurrence(
            eventId: occurrence.eventId, occurrence: occurrence.date,
            options: DeleteCalendarEventOccurrenceOptions(applyToFuture: scope == .thisAndFollowing))
    }

    /// Posts a calendar form to a `.json` path and reads the recording it answers, inside the
    /// operation the hooks hear. An older server answers a redirect instead, whose URL still names
    /// the recording's id; the type is not in it, so it stays empty as Go's and Rust's do.
    private func calendarWrite(_ method: HTTPMethod, _ path: String, _ info: OperationInfo, _ fields: [(String, String)]) async throws -> Recording {
        var operation = client.form(method, path)
        operation.info = info
        operation.form(fields)
        return try await client.execute(operation) { response in
            let answered = FormResponse.of(response)
            if answered.body.isEmpty { return Recording(id: try answered.extractId(), type: "") }
            return try response.json(Recording.self)
        }
    }
}

/// Form-encodes a partial revision.
func eventUpdateFields(_ update: CalendarEventUpdate) -> [(String, String)] {
    var fields: [(String, String)] = []
    if let title = update.title { fields.append(("calendar_event[summary]", title)) }
    if let startsAt = update.startsAt { fields.append(("calendar_event[starts_at]", startsAt)) }
    if let endsAt = update.endsAt { fields.append(("calendar_event[ends_at]", endsAt)) }
    if let allDay = update.allDay { fields.append(("calendar_event[all_day]", checkbox(allDay))) }
    if update.allDay != true {
        if let startTime = update.startTime { fields.append(("calendar_event[starts_at_time]", "\(startTime):00")) }
        if let endTime = update.endTime { fields.append(("calendar_event[ends_at_time]", "\(endTime):00")) }
    }
    return fields
}

private let attendeesField = "calendar_event[attendance_email_addresses][]"
private let allDayRemindersField = "all_day_reminder_durations[]"
private let timedRemindersField = "timed_reminder_durations[]"

private func checkbox(_ value: Bool) -> String { value ? "1" : "0" }

/// Form-encodes a new event.
func createEventFields(_ params: CreateCalendarEventParams) -> [(String, String)] {
    var fields: [(String, String)] = []
    fields.append(("calendar_event[calendar_id]", String(params.calendarId)))
    fields.append(("calendar_event[summary]", params.title))
    fields.append(("calendar_event[starts_at]", params.startsAt))
    fields.append(("calendar_event[ends_at]", params.endsAt.flatMap { $0.isEmpty ? nil : $0 } ?? params.startsAt))
    addContent(&fields, params.content)
    addAttendees(&fields, params.attendees)
    addHighlighted(&fields, params.highlighted)
    addCountdown(&fields, params.countdown)
    addRepeat(&fields, params.repeat)
    if params.allDay {
        fields.append(("calendar_event[all_day]", checkbox(true)))
        addReminders(&fields, allDayRemindersField, params.reminders)
    } else {
        fields.append(("calendar_event[all_day]", checkbox(false)))
        fields.append(("calendar_event[starts_at_time]", "\(params.startTime ?? ""):00"))
        fields.append(("calendar_event[ends_at_time]", "\(params.endTime ?? ""):00"))
        // A create always says what it means about the zones: naming none is "read the times in UTC".
        addTimeZones(&fields, params.startTimeZone ?? "", params.endTimeZone ?? "")
        addReminders(&fields, timedRemindersField, params.reminders)
    }
    return fields
}

/// Form-encodes a whole-event update; the occurrence update builds on it.
func updateEventFields(_ params: UpdateCalendarEventParams) -> [(String, String)] {
    var fields: [(String, String)] = []
    if let title = params.title { fields.append(("calendar_event[summary]", title)) }
    if let startsAt = params.startsAt { fields.append(("calendar_event[starts_at]", startsAt)) }
    if let endsAt = params.endsAt { fields.append(("calendar_event[ends_at]", endsAt)) }
    if let allDay = params.allDay { fields.append(("calendar_event[all_day]", checkbox(allDay))) }
    if params.allDay != true {
        if let startTime = params.startTime { fields.append(("calendar_event[starts_at_time]", "\(startTime):00")) }
        if let endTime = params.endTime { fields.append(("calendar_event[ends_at_time]", "\(endTime):00")) }
    }
    if let calendarId = params.calendarId { fields.append(("calendar_event[calendar_id]", String(calendarId))) }
    addContent(&fields, params.content)
    addAttendees(&fields, params.attendees)
    addHighlighted(&fields, params.highlighted)
    addCountdown(&fields, params.countdown)
    addRepeat(&fields, params.repeat)

    // Naming no zone on an update says nothing about zones; naming one, or an empty one, is an
    // answer, and the empty answer is "convert to UTC".
    if params.startTimeZone != nil || params.endTimeZone != nil {
        addTimeZones(&fields, params.startTimeZone ?? "", params.endTimeZone ?? "")
    }

    // HEY reads the list matching the event's all-day flag as it stands after the write. An update
    // that leaves the flag alone cannot know which that is, so it sends both lists: the one HEY
    // does not read is ignored, and the one it does keeps the reminders scheduled.
    let reminderFields: [String]
    switch params.allDay {
    case true?: reminderFields = [allDayRemindersField]
    case false?: reminderFields = [timedRemindersField]
    case nil: reminderFields = [allDayRemindersField, timedRemindersField]
    }
    for name in reminderFields { addReminders(&fields, name, params.reminders) }
    return fields
}

/// The content goes out whole every time, because HEY clears what it is not sent.
private func addContent(_ fields: inout [(String, String)], _ content: EventContent) {
    fields.append(("calendar_event[description]", content.notes))
    fields.append(("calendar_event[location]", content.location))
    fields.append(("calendar_event[url]", content.link ?? ""))
    // A zero id names no entry, so it goes out blank rather than as "0", which HEY would look up
    // and refuse.
    fields.append(("calendar_event[entry_id]", content.entryId.flatMap { $0 == 0 ? nil : String($0) } ?? ""))
}

/// The guest list is replaced wholesale when it is submitted at all; an empty list needs a blank
/// value on the wire to say so, since a form carries no empty array.
private func addAttendees(_ fields: inout [(String, String)], _ attendees: [String]?) {
    guard let addresses = attendees else { return }
    if addresses.isEmpty {
        fields.append((attendeesField, ""))
    } else {
        for address in addresses { fields.append((attendeesField, address)) }
    }
}

/// The empty highlight_id is what makes "off" mean off: HEY destroys the existing highlight when
/// the key is there and empty, and builds one when the flag is on.
private func addHighlighted(_ fields: inout [(String, String)], _ highlighted: Bool?) {
    guard let circled = highlighted else { return }
    fields.append(("calendar_event[highlighted]", checkbox(circled)))
    fields.append(("calendar_event[highlight_id]", ""))
}

/// A zero countdown sends no value at all, which is how HEY is told to delete it.
private func addCountdown(_ fields: inout [(String, String)], _ countdown: Countdown) {
    guard countdown.value > 0 else { return }
    fields.append(("countdown_interval_duration_value", String(countdown.value)))
    fields.append(("countdown_interval_duration_unit", String(countdown.unit.seconds)))
}

private func addRepeat(_ fields: inout [(String, String)], _ repeat: Repeat?) {
    guard let recurrence = `repeat` else { return }
    fields.append(("repeat_frequency", recurrence.frequency.rawValue))
    if let until = recurrence.until { fields.append(("calendar_recurrence_schedule[recurs_until_type]", until.rawValue)) }
    if recurrence.until == .date { fields.append(("calendar_recurrence_schedule[recurs_until_date]", recurrence.untilDate ?? "")) }
    if recurrence.until == .count { fields.append(("calendar_recurrence_schedule[recurs_count]", String(recurrence.count ?? 0))) }
}

/// The zones, and the flag that makes HEY honour them: without it both names are dropped and the
/// times are read in UTC. Naming none is a complete answer — convert to UTC — and one zone named
/// stands in for the other.
private func addTimeZones(_ fields: inout [(String, String)], _ start: String, _ end: String) {
    if start.isEmpty && end.isEmpty {
        fields.append(("calendar_event[set_time_zone]", checkbox(false)))
        return
    }
    fields.append(("calendar_event[set_time_zone]", checkbox(true)))
    fields.append(("calendar_event[starts_at_time_zone_name]", start.isEmpty ? end : start))
    fields.append(("calendar_event[ends_at_time_zone_name]", end.isEmpty ? start : end))
}

/// Each reminder in whole seconds, as HEY's form submits it.
private func addReminders(_ fields: inout [(String, String)], _ name: String, _ reminders: [Duration]) {
    for reminder in reminders { fields.append((name, String(reminder.components.seconds))) }
}
