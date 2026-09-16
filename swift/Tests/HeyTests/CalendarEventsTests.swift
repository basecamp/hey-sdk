import Foundation
import XCTest

@testable import Hey

final class CalendarEventsTests: XCTestCase {
    func testACalendarEventUpdateIsAFormPostToTheJSONPath() async throws {
        let hey = mockHey(ok(""), ok(""))
        let client = try hey.client()
        try await client.calendarEvents.update(
            eventId: 99,
            update: CalendarEventUpdate(title: "Sarah's birthday", startsAt: "2026-09-02", endsAt: "2026-09-02", allDay: true, startTime: "", endTime: ""))
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "PATCH")
        XCTAssertEqual(request.path, "/calendar/events/99.json")
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(request.header("Accept"), "application/json")
        XCTAssertEqual(formValue(request.body, "calendar_event[summary]"), "Sarah's birthday")
        XCTAssertEqual(formValue(request.body, "calendar_event[all_day]"), "1")
        XCTAssertNil(formValue(request.body, "calendar_event[starts_at_time]"))

        try await client.calendarEvents.update(eventId: 99, update: CalendarEventUpdate(allDay: false, startTime: "09:30", endTime: "10:00"))
        let timed = hey.requests[1].body
        XCTAssertEqual(formValue(timed, "calendar_event[starts_at_time]"), "09:30:00")
        XCTAssertEqual(formValue(timed, "calendar_event[all_day]"), "0")
    }

    func testAPartialUpdateSendsOnlyWhatItNamesInOrder() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().calendarEvents.update(
            eventId: 5, update: CalendarEventUpdate(title: "T", startsAt: "2026-01-01", endsAt: "2026-01-02", startTime: "08:00", endTime: "09:00"))
        let pairs = formPairs(hey.requests[0].body)
        XCTAssertEqual(pairs.map(\.0), [
            "calendar_event[summary]", "calendar_event[starts_at]", "calendar_event[ends_at]",
            "calendar_event[starts_at_time]", "calendar_event[ends_at_time]",
        ])
        XCTAssertEqual(pairs.map(\.1), ["T", "2026-01-01", "2026-01-02", "08:00:00", "09:00:00"])
    }

    func testAWholeEventUpdateSendsEverythingHEYWouldOtherwiseClear() async throws {
        let hey = mockHey(ok(#"{"id":99,"type":"Calendar::Event","summary":"Standup"}"#), status(302, nil, [("Location", "/calendar/events/99")]))
        let client = try hey.client()
        let params = UpdateCalendarEventParams(
            title: "Standup",
            allDay: false,
            startTime: "09:30",
            endTime: "10:00",
            startTimeZone: "Europe/Zagreb",
            reminders: [.seconds(600), .seconds(3600)],
            content: EventContent(notes: "<p>Agenda</p>", location: "Room 4", link: "https://example.com/call", entryId: 5),
            attendees: [],
            highlighted: true,
            countdown: Countdown(value: 3, unit: .weeks),
            repeat: Repeat(frequency: .everyWeek, until: .count, count: 12))
        let recording = try await client.calendarEvents.updateEvent(eventId: 99, params: params)
        XCTAssertEqual(recording.id, 99)
        XCTAssertEqual(recording.summary, "Standup")

        let request = hey.requests[0]
        XCTAssertEqual(request.method, "PATCH")
        XCTAssertEqual(request.path, "/calendar/events/99.json")
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)
        let body = request.body
        XCTAssertEqual(formValue(body, "calendar_event[summary]"), "Standup")
        XCTAssertEqual(formValue(body, "calendar_event[starts_at_time]"), "09:30:00")
        XCTAssertEqual(formValue(body, "calendar_event[description]"), "<p>Agenda</p>")
        XCTAssertEqual(formValue(body, "calendar_event[location]"), "Room 4")
        XCTAssertEqual(formValue(body, "calendar_event[url]"), "https://example.com/call")
        XCTAssertEqual(formValue(body, "calendar_event[entry_id]"), "5")
        XCTAssertEqual(formValues(body, "calendar_event[attendance_email_addresses][]"), [""], "an empty guest list goes out as one blank value")
        XCTAssertEqual(formValue(body, "calendar_event[highlighted]"), "1")
        XCTAssertEqual(formValue(body, "calendar_event[highlight_id]"), "")
        XCTAssertEqual(formValue(body, "countdown_interval_duration_value"), "3")
        XCTAssertEqual(formValue(body, "countdown_interval_duration_unit"), "604800")
        XCTAssertEqual(formValue(body, "repeat_frequency"), "every_week")
        XCTAssertEqual(formValue(body, "calendar_recurrence_schedule[recurs_until_type]"), "count")
        XCTAssertEqual(formValue(body, "calendar_recurrence_schedule[recurs_count]"), "12")
        XCTAssertEqual(formValue(body, "calendar_event[set_time_zone]"), "1")
        XCTAssertEqual(formValue(body, "calendar_event[starts_at_time_zone_name]"), "Europe/Zagreb")
        XCTAssertEqual(formValue(body, "calendar_event[ends_at_time_zone_name]"), "Europe/Zagreb", "the one zone named stands in for the other")
        XCTAssertEqual(formValues(body, "timed_reminder_durations[]"), ["600", "3600"])

        let fallback = try await client.calendarEvents.updateEvent(eventId: 99, params: UpdateCalendarEventParams(title: "Renamed"))
        XCTAssertEqual(fallback.id, 99, "an older server answers a redirect, whose URL still names the recording")
        XCTAssertEqual(fallback.type, "", "and nothing else about it, so the type is not invented")
        let cleared = hey.requests[1].body
        XCTAssertEqual(formValue(cleared, "calendar_event[description]"), "", "what the params leave empty goes out empty, since HEY clears it either way")
        XCTAssertEqual(formValue(cleared, "calendar_event[entry_id]"), "")
        XCTAssertNil(formValue(cleared, "countdown_interval_duration_value"), "a zero countdown sends nothing, which deletes it")
        XCTAssertNil(formValue(cleared, "calendar_event[attendance_email_addresses][]"), "a nil guest list is left alone")
        XCTAssertNil(formValue(cleared, "calendar_event[set_time_zone]"), "no zone named says nothing about zones")
    }

    func testAnOccurrenceUpdateKeepsTheSeriesScheduleUnlessToldOtherwise() async throws {
        let hey = mockHey(ok(#"{"id":0,"type":"Calendar::Event","parent_id":153688907}"#), ok(#"{"id":0,"type":"Calendar::Event"}"#))
        let transcript = OperationLog()
        let client = try hey.client(hooks: transcript)
        _ = try await client.calendarEvents.updateOccurrence(
            occurrence: try OccurrenceId.parse("153688907_2026-08-21"), scope: .thisAndFollowing, params: UpdateCalendarEventParams(title: "Moved"))
        let request = hey.requests[0]
        XCTAssertEqual(request.path, "/calendar/events/153688907/occurrences/2026-08-21.json")
        XCTAssertEqual(formValue(request.body, "apply_to_future"), "1")
        XCTAssertEqual(formValue(request.body, "repeat_frequency"), "custom", "silence would end the series, so the schedule is kept in so many words")
        XCTAssertEqual(transcript.started, ["CalendarEvents.UpdateCalendarEventOccurrence:calendar_event:true:153688907"])

        _ = try await client.calendarEvents.updateOccurrence(
            occurrence: try OccurrenceId(eventId: 9, date: "2026-08-21"), scope: .thisOnly,
            params: UpdateCalendarEventParams(repeat: Repeat(frequency: .everyDay)))
        XCTAssertEqual(formValue(hey.requests[1].body, "apply_to_future"), "0")
        XCTAssertEqual(formValues(hey.requests[1].body, "repeat_frequency"), ["every_day"], "a named recurrence is sent once, as named")
    }

    func testAnOccurrenceDeleteCarriesTheScopeInTheQuery() async throws {
        let hey = mockHey(ok(""))
        try await hey.client().calendarEvents.deleteOccurrenceScoped(occurrence: try OccurrenceId.parse("153688907_2026-08-21"), scope: .thisAndFollowing)
        XCTAssertEqual(hey.requests.count, 1)
        XCTAssertEqual(hey.requests[0].path, "/calendar/events/153688907/occurrences/2026-08-21.json")
        XCTAssertEqual(hey.requests[0].query("apply_to_future"), "true")
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId.parse("nope") }
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceScope.parse("everything") }
        XCTAssertEqual(try OccurrenceScope.parse("this_event"), .thisOnly)
    }

    func testRemindersGoUnderTheListHEYWillRead() async throws {
        let recording = #"{"id":1,"type":"Calendar::Event"}"#
        let hey = mockHey(ok(recording), ok(recording), ok(recording))
        let client = try hey.client()
        _ = try await client.calendarEvents.updateEvent(eventId: 1, params: UpdateCalendarEventParams(allDay: true, reminders: [.seconds(3600)]))
        _ = try await client.calendarEvents.updateEvent(eventId: 1, params: UpdateCalendarEventParams(allDay: false, reminders: [.seconds(3600)]))
        _ = try await client.calendarEvents.updateEvent(eventId: 1, params: UpdateCalendarEventParams(reminders: [.seconds(3600)]))
        XCTAssertEqual(formValues(hey.requests[0].body, "all_day_reminder_durations[]"), ["3600"])
        XCTAssertEqual(formValues(hey.requests[0].body, "timed_reminder_durations[]"), [])
        XCTAssertEqual(formValues(hey.requests[1].body, "timed_reminder_durations[]"), ["3600"])
        XCTAssertEqual(formValues(hey.requests[1].body, "all_day_reminder_durations[]"), [])
        XCTAssertEqual(
            formValues(hey.requests[2].body, "all_day_reminder_durations[]"), ["3600"],
            "an update that leaves the flag alone cannot know which list HEY reads, so it sends both")
        XCTAssertEqual(formValues(hey.requests[2].body, "timed_reminder_durations[]"), ["3600"])
    }

    func testAnOccurrenceIdIsCheckedHoweverItIsMade() async throws {
        XCTAssertEqual(try OccurrenceId.parse("9_2024-02-29").date, "2024-02-29")
        XCTAssertEqual(try OccurrenceId.parse("9_2024-02-29").description, "9_2024-02-29")
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId.parse("9_2023-02-29") }
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId.parse("9_2026-02-30") }
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId.parse("9_2026-13-01") }
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId.parse("0_2026-01-01") }
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId(eventId: 9, date: "../../42") }
        assertThrowsSync(HeyError.codeUsage) { try OccurrenceId(eventId: 0, date: "2026-01-01") }
        let hey = mockHey()
        let client = try hey.client()
        await assertThrows(
            HeyError.codeUsage,
            try await client.calendarEvents.updateOccurrence(
                occurrence: try OccurrenceId.parse("9_2026-04-31"), scope: .thisOnly, params: UpdateCalendarEventParams()))
        XCTAssertEqual(hey.requests.count, 0)
    }

    func testACalendarEventIsCreatedFromAFormPostedToTheJSONPath() async throws {
        let hey = mockHey(ok(#"{"id":7,"type":"Calendar::Event","summary":"Standup"}"#), status(302, nil, [("Location", "/calendar/events/8")]))
        let transcript = AnnouncementLog()
        let client = try hey.client(hooks: transcript)
        let timed = CreateCalendarEventParams(
            calendarId: 3,
            title: "Standup",
            startsAt: "2026-09-15",
            startTime: "09:30",
            endTime: "10:00",
            endTimeZone: "Europe/Zagreb",
            reminders: [.seconds(600)],
            content: EventContent(notes: "<p>Agenda</p>", location: "Room 4"),
            attendees: ["a@example.com", "b@example.com"],
            highlighted: true,
            countdown: Countdown(value: 2, unit: .days),
            repeat: Repeat(frequency: .everyWeekday, until: .date, untilDate: "2026-12-31"))
        let created = try await client.calendarEvents.create(params: timed)
        XCTAssertEqual(created.id, 7)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/calendar/events.json")
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        let body = request.body
        XCTAssertEqual(formValue(body, "calendar_event[calendar_id]"), "3")
        XCTAssertEqual(formValue(body, "calendar_event[summary]"), "Standup")
        XCTAssertEqual(formValue(body, "calendar_event[starts_at]"), "2026-09-15")
        XCTAssertEqual(formValue(body, "calendar_event[ends_at]"), "2026-09-15", "the end defaults to the start")
        XCTAssertEqual(formValue(body, "calendar_event[all_day]"), "0")
        XCTAssertEqual(formValue(body, "calendar_event[starts_at_time]"), "09:30:00")
        XCTAssertEqual(formValue(body, "calendar_event[ends_at_time]"), "10:00:00")
        XCTAssertEqual(formValue(body, "calendar_event[set_time_zone]"), "1")
        XCTAssertEqual(formValue(body, "calendar_event[starts_at_time_zone_name]"), "Europe/Zagreb", "the one zone named stands in for the other")
        XCTAssertEqual(formValue(body, "calendar_event[ends_at_time_zone_name]"), "Europe/Zagreb")
        XCTAssertEqual(formValues(body, "timed_reminder_durations[]"), ["600"])
        XCTAssertEqual(formValues(body, "all_day_reminder_durations[]"), [])
        XCTAssertEqual(formValue(body, "calendar_event[description]"), "<p>Agenda</p>")
        XCTAssertEqual(formValue(body, "calendar_event[location]"), "Room 4")
        XCTAssertEqual(formValue(body, "calendar_event[url]"), "")
        XCTAssertEqual(formValue(body, "calendar_event[entry_id]"), "")
        XCTAssertEqual(formValues(body, "calendar_event[attendance_email_addresses][]"), ["a@example.com", "b@example.com"])
        XCTAssertEqual(formValue(body, "calendar_event[highlighted]"), "1")
        XCTAssertEqual(formValue(body, "calendar_event[highlight_id]"), "")
        XCTAssertEqual(formValue(body, "countdown_interval_duration_value"), "2")
        XCTAssertEqual(formValue(body, "countdown_interval_duration_unit"), "86400")
        XCTAssertEqual(formValue(body, "repeat_frequency"), "every_weekday")
        XCTAssertEqual(formValue(body, "calendar_recurrence_schedule[recurs_until_type]"), "date")
        XCTAssertEqual(formValue(body, "calendar_recurrence_schedule[recurs_until_date]"), "2026-12-31")
        XCTAssertEqual(transcript.operations.count, 1)
        XCTAssertEqual(transcript.operations.first?.operation, "CreateCalendarEvent")
        XCTAssertEqual(transcript.operations.first?.service, "CalendarEvents")
        XCTAssertNil(transcript.operations.first?.resourceId, "a create names no record yet")

        let allDay = try await client.calendarEvents.create(
            params: CreateCalendarEventParams(
                calendarId: 3, title: "Holiday", startsAt: "2026-12-24", endsAt: "2026-12-26", allDay: true, reminders: [.seconds(3600)]))
        XCTAssertEqual(allDay.id, 8, "an older server answers a redirect, whose URL still names the recording")
        XCTAssertEqual(allDay.type, "")
        let whole = hey.requests[1].body
        XCTAssertEqual(formValue(whole, "calendar_event[all_day]"), "1")
        XCTAssertEqual(formValue(whole, "calendar_event[ends_at]"), "2026-12-26")
        XCTAssertNil(formValue(whole, "calendar_event[starts_at_time]"), "an all-day event has no clock times")
        XCTAssertNil(formValue(whole, "calendar_event[set_time_zone]"), "and no zones to set them in")
        XCTAssertEqual(formValues(whole, "all_day_reminder_durations[]"), ["3600"])
        XCTAssertNil(formValue(whole, "countdown_interval_duration_value"))
        XCTAssertNil(formValue(whole, "repeat_frequency"))
        XCTAssertNil(formValue(whole, "calendar_event[highlighted]"), "unsaid on a create is not circled")
        XCTAssertNil(formValue(whole, "calendar_event[attendance_email_addresses][]"))
    }

    func testACreateSendsItsFieldsInTheOrderTheOtherSDKsDo() async throws {
        let hey = mockHey(ok(#"{"id":7,"type":"Calendar::Event"}"#))
        _ = try await hey.client().calendarEvents.create(
            params: CreateCalendarEventParams(calendarId: 3, title: "Call", startsAt: "2026-09-15", startTime: "09:00", endTime: "09:30"))
        XCTAssertEqual(formPairs(hey.requests[0].body).map(\.0), [
            "calendar_event[calendar_id]", "calendar_event[summary]", "calendar_event[starts_at]", "calendar_event[ends_at]",
            "calendar_event[description]", "calendar_event[location]", "calendar_event[url]", "calendar_event[entry_id]",
            "calendar_event[all_day]", "calendar_event[starts_at_time]", "calendar_event[ends_at_time]", "calendar_event[set_time_zone]",
        ])
    }

    func testATimedEventWithNoZoneIsReadInUTCAndOneWithoutTheEssentialsIsRefused() async throws {
        let hey = mockHey(ok(#"{"id":7,"type":"Calendar::Event"}"#))
        let client = try hey.client()
        _ = try await client.calendarEvents.create(
            params: CreateCalendarEventParams(calendarId: 3, title: "Call", startsAt: "2026-09-15", startTime: "09:00", endTime: "09:30"))
        let body = hey.requests[0].body
        XCTAssertEqual(formValue(body, "calendar_event[set_time_zone]"), "0", "a create always says what it means about the zones")
        XCTAssertNil(formValue(body, "calendar_event[starts_at_time_zone_name]"))

        await assertThrows(
            HeyError.codeUsage, try await client.calendarEvents.create(params: CreateCalendarEventParams(calendarId: 3, title: "", startsAt: "2026-09-15", allDay: true)))
        await assertThrows(
            HeyError.codeUsage, try await client.calendarEvents.create(params: CreateCalendarEventParams(calendarId: 3, title: "Call", startsAt: "", allDay: true)))
        await assertThrows(
            HeyError.codeUsage,
            try await client.calendarEvents.create(params: CreateCalendarEventParams(calendarId: 3, title: "Call", startsAt: "2026-09-15", startTime: "09:00")))
        XCTAssertEqual(hey.requests.count, 1, "a refusal sends nothing")
    }

    func testACalendarWriteThatAnswersSomethingOtherThanARecordingEndsTheOperationWithThatError() async throws {
        let hey = mockHey(ok(#"{"summary":"no id or type"}"#))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        await assertThrows(HeyError.codeAPI, try await client.calendarEvents.updateEvent(eventId: 99, params: UpdateCalendarEventParams(title: "x")))
        XCTAssertEqual(transcript.log.last, "end:UpdateCalendarEvent:api_error")
        XCTAssertEqual(transcript.log.filter { $0.hasPrefix("end:") }.count, 1)
    }

    func testAFormRequestIsRefreshedAndResentButNeverRetriedOtherwise() async throws {
        let hey = mockHey(status(401), ok(""))
        let credentials = ScriptedProvider { provider, _ in
            provider.token = "refreshed"
            return true
        }
        try await hey.client(auth: BearerAuth(tokenProvider: credentials)).calendarEvents.update(eventId: 99, update: CalendarEventUpdate(title: "After"))
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[1].header("Authorization"), "Bearer refreshed")
        XCTAssertEqual(hey.requests[1].body, "calendar_event%5Bsummary%5D=After")

        let failing = mockHey(status(503))
        await assertThrows(HeyError.codeAPI, try await failing.client().calendarEvents.update(eventId: 99, update: CalendarEventUpdate(title: "After")))
        XCTAssertEqual(failing.requests.count, 1)
    }
}
