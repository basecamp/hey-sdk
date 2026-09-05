mod support;

use std::time::Duration;

use hey_sdk::services::{
    CalendarEventUpdate, Countdown, CountdownUnit, CreateCalendarEventParams, EventContent,
    OccurrenceId, OccurrenceScope, Repeat, RepeatFrequency, RepeatUntil, UpdateCalendarEventParams,
    UpdateOccurrenceParams,
};
use hey_sdk::types::Date;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

#[tokio::test]
async fn creating_a_timed_event_carries_the_clock_times_and_the_zone_they_are_written_in() {
    let server = MockServer::start().await;
    mock_create(&server, ResponseTemplate::new(201).set_body_json(event())).await;

    let recording = client(&server)
        .calendar_events()
        .create(&CreateCalendarEventParams {
            calendar_id: 1,
            title: "Meeting".to_string(),
            starts_at: "2026-04-06".to_string(),
            start_time: "10:00".to_string(),
            end_time: "11:00".to_string(),
            start_time_zone: "America/New_York".to_string(),
            end_time_zone: "America/New_York".to_string(),
            ..CreateCalendarEventParams::default()
        })
        .await
        .unwrap();

    assert_eq!(recording.id, 99);
    assert_eq!(recording.title.as_deref(), Some("Meeting"));
    assert_eq!(recording.r#type, "Calendar::Event");
    assert_eq!(recording.calendar.unwrap().id, 1);
    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[calendar_id]=1",
            "calendar_event[summary]=Meeting",
            "calendar_event[starts_at]=2026-04-06",
            "calendar_event[ends_at]=2026-04-06",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "calendar_event[all_day]=0",
            "calendar_event[starts_at_time]=10:00:00",
            "calendar_event[ends_at_time]=11:00:00",
            "calendar_event[set_time_zone]=1",
            "calendar_event[starts_at_time_zone_name]=America/New_York",
            "calendar_event[ends_at_time_zone_name]=America/New_York",
        ]
    );
}

/// A server without the JSON create branch redirects to the event, which still names its id.
#[tokio::test]
async fn a_create_that_only_redirects_answers_the_id_the_redirect_names() {
    let server = MockServer::start().await;
    mock_create(
        &server,
        ResponseTemplate::new(302).insert_header("Location", "/calendar/events/99"),
    )
    .await;

    let recording = client(&server)
        .calendar_events()
        .create(&CreateCalendarEventParams {
            calendar_id: 1,
            title: "Meeting".to_string(),
            starts_at: "2026-04-06".to_string(),
            start_time: "10:00".to_string(),
            end_time: "11:00".to_string(),
            ..CreateCalendarEventParams::default()
        })
        .await
        .unwrap();

    assert_eq!(recording.id, 99);
    assert_eq!(recording.title, None);
}

#[tokio::test]
async fn an_all_day_create_carries_no_clock_times_and_files_its_reminders_as_all_day_ones() {
    let server = MockServer::start().await;
    mock_create(
        &server,
        ResponseTemplate::new(201).set_body_json(
            json!({ "id": 100, "title": "Holiday", "type": "Calendar::Event", "all_day": true }),
        ),
    )
    .await;

    let recording = client(&server)
        .calendar_events()
        .create(&CreateCalendarEventParams {
            calendar_id: 1,
            title: "Holiday".to_string(),
            starts_at: "2026-04-06".to_string(),
            all_day: true,
            reminders: vec![Duration::from_secs(24 * 60 * 60)],
            ..CreateCalendarEventParams::default()
        })
        .await
        .unwrap();

    assert_eq!(recording.id, 100);
    assert_eq!(recording.all_day, Some(true));
    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[calendar_id]=1",
            "calendar_event[summary]=Holiday",
            "calendar_event[starts_at]=2026-04-06",
            "calendar_event[ends_at]=2026-04-06",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "calendar_event[all_day]=1",
            "all_day_reminder_durations[]=86400",
        ]
    );
}

/// The deprecated single time zone names both ends, and carries the flag that makes HEY read
/// it.
#[tokio::test]
async fn one_time_zone_stands_in_for_both_ends() {
    let server = MockServer::start().await;
    mock_create(&server, ResponseTemplate::new(201).set_body_json(event())).await;

    client(&server)
        .calendar_events()
        .create(&CreateCalendarEventParams {
            calendar_id: 1,
            title: "Meeting".to_string(),
            starts_at: "2026-04-06".to_string(),
            start_time: "10:00".to_string(),
            end_time: "11:00".to_string(),
            time_zone: "America/New_York".to_string(),
            ..CreateCalendarEventParams::default()
        })
        .await
        .unwrap();

    let form = sent_form(&server).await;
    assert!(form.contains(&"calendar_event[set_time_zone]=1".to_string()));
    assert!(
        form.contains(&"calendar_event[starts_at_time_zone_name]=America/New_York".to_string())
    );
    assert!(form.contains(&"calendar_event[ends_at_time_zone_name]=America/New_York".to_string()));
}

#[tokio::test]
async fn a_create_carries_its_content_guests_countdown_and_recurrence() {
    let server = MockServer::start().await;
    mock_create(&server, ResponseTemplate::new(201).set_body_json(event())).await;

    client(&server)
        .calendar_events()
        .create(&CreateCalendarEventParams {
            calendar_id: 1,
            title: "Quarterly roadmap review".to_string(),
            starts_at: "2026-09-10".to_string(),
            all_day: true,
            content: EventContent {
                notes: "<div>Bring the <strong>roadmap</strong>.</div>".to_string(),
                location: "Meeting Room 2".to_string(),
                link: Some("https://meet.google.com/abc-defg-hij".to_string()),
                entry_id: Some(884213),
            },
            attendees: Some(vec![
                "marta.kowalska@example.com".to_string(),
                "yusuf.demir@example.org".to_string(),
            ]),
            highlighted: Some(true),
            countdown: Countdown {
                value: 2,
                unit: CountdownUnit::Weeks,
            },
            repeat: Some(Repeat {
                frequency: RepeatFrequency::EveryOtherWeek,
                until: Some(RepeatUntil::Date),
                until_date: Date::new(2026, 12, 18),
                count: Some(9),
            }),
            ..CreateCalendarEventParams::default()
        })
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[calendar_id]=1",
            "calendar_event[summary]=Quarterly roadmap review",
            "calendar_event[starts_at]=2026-09-10",
            "calendar_event[ends_at]=2026-09-10",
            "calendar_event[description]=<div>Bring the <strong>roadmap</strong>.</div>",
            "calendar_event[location]=Meeting Room 2",
            "calendar_event[url]=https://meet.google.com/abc-defg-hij",
            "calendar_event[entry_id]=884213",
            "calendar_event[attendance_email_addresses][]=marta.kowalska@example.com",
            "calendar_event[attendance_email_addresses][]=yusuf.demir@example.org",
            "calendar_event[highlighted]=1",
            "calendar_event[highlight_id]=",
            "countdown_interval_duration_value=2",
            "countdown_interval_duration_unit=604800",
            "repeat_frequency=every_other_week",
            "calendar_recurrence_schedule[recurs_until_type]=date",
            "calendar_recurrence_schedule[recurs_until_date]=2026-12-18",
            "calendar_event[all_day]=1",
        ]
    );
}

#[tokio::test]
async fn an_update_answers_the_recording_it_wrote() {
    let server = MockServer::start().await;
    mock_update(
        &server,
        ResponseTemplate::new(200).set_body_json(
            json!({ "id": 99, "title": "Updated Meeting", "type": "Calendar::Event" }),
        ),
    )
    .await;

    let recording = client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                title: Some("Updated Meeting".to_string()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(recording.id, 99);
    assert_eq!(recording.title.as_deref(), Some("Updated Meeting"));
    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[summary]=Updated Meeting",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
        ]
    );
}

/// A server without the JSON update branch redirects to the event, which still names its id.
#[tokio::test]
async fn an_update_that_only_redirects_answers_the_id_the_redirect_names() {
    let server = MockServer::start().await;
    mock_update(
        &server,
        ResponseTemplate::new(302).insert_header("Location", "/calendar/events/99"),
    )
    .await;

    let recording = client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                title: Some("Updated Meeting".to_string()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(recording.id, 99);
}

/// HEY defaults all four content fields to nothing on every write, so an update that only
/// means to move an event still has to say what its notes are.
#[tokio::test]
async fn an_update_sends_the_whole_content_every_time() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                starts_at: Some("2026-09-11".to_string()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[starts_at]=2026-09-11",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
        ]
    );
}

/// An all-day update carries dates without clock times, matching an all-day create.
#[tokio::test]
async fn an_all_day_update_omits_the_clock_times() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                title: Some("Sarah's birthday".to_string()),
                starts_at: Some("2026-09-02".to_string()),
                ends_at: Some("2026-09-02".to_string()),
                all_day: Some(true),
                start_time: Some(String::new()),
                end_time: Some(String::new()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[summary]=Sarah's birthday",
            "calendar_event[starts_at]=2026-09-02",
            "calendar_event[ends_at]=2026-09-02",
            "calendar_event[all_day]=1",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
        ]
    );
}

/// An event can start in one zone and finish in another, which is what a flight is.
#[tokio::test]
async fn an_event_can_start_in_one_zone_and_finish_in_another() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                start_time_zone: Some("Europe/Zagreb".to_string()),
                end_time_zone: Some("America/New_York".to_string()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    let form = sent_form(&server).await;
    assert!(form.contains(&"calendar_event[set_time_zone]=1".to_string()));
    assert!(form.contains(&"calendar_event[starts_at_time_zone_name]=Europe/Zagreb".to_string()));
    assert!(form.contains(&"calendar_event[ends_at_time_zone_name]=America/New_York".to_string()));
}

/// Empty zones say the times are UTC, and put the event back to having no zones of its own.
#[tokio::test]
async fn clearing_the_zones_sends_the_flag_off_and_names_neither_end() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                start_time_zone: Some(String::new()),
                end_time_zone: Some(String::new()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "calendar_event[set_time_zone]=0",
        ]
    );
}

/// Turning the circle off needs the empty highlight_id: without it HEY reads highlighted=0
/// as a request to build a highlight, which turns the circle on.
#[tokio::test]
async fn circling_and_uncircling_both_carry_the_empty_highlight_id() {
    for (circled, flag) in [(true, "1"), (false, "0")] {
        let server = MockServer::start().await;
        mock_update(&server, updated()).await;

        client(&server)
            .calendar_events()
            .update_event(
                99,
                &UpdateCalendarEventParams {
                    highlighted: Some(circled),
                    ..UpdateCalendarEventParams::default()
                },
            )
            .await
            .unwrap();

        let form = sent_form(&server).await;
        assert!(form.contains(&format!("calendar_event[highlighted]={flag}")));
        assert!(form.contains(&"calendar_event[highlight_id]=".to_string()));
    }
}

/// Nothing about the circle, the countdown, the guests or the recurrence goes out when
/// nobody asked: HEY reads each of those only when it is submitted, and an update that says
/// nothing must not change them.
#[tokio::test]
async fn an_update_that_asks_for_nothing_sends_only_the_content_it_has_to() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(99, &UpdateCalendarEventParams::default())
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
        ]
    );
}

/// A countdown with no unit named is counted in days, which is the web app's own default.
#[tokio::test]
async fn a_countdown_with_no_unit_is_counted_in_days() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                countdown: Countdown {
                    value: 10,
                    ..Countdown::default()
                },
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    let form = sent_form(&server).await;
    assert!(form.contains(&"countdown_interval_duration_value=10".to_string()));
    assert!(form.contains(&"countdown_interval_duration_unit=86400".to_string()));
}

/// An empty guest list clears the roster, and needs a blank value on the wire to say so
/// since a form cannot carry an empty array.
#[tokio::test]
async fn an_empty_guest_list_goes_out_as_one_blank_address() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                attendees: Some(Vec::new()),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "calendar_event[attendance_email_addresses][]=",
        ]
    );
}

/// A count-limited recurrence sends the count and no until-date, since HEY reads only the
/// one matching the type.
#[tokio::test]
async fn a_count_limited_recurrence_names_no_until_date() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update_event(
            99,
            &UpdateCalendarEventParams {
                repeat: Some(Repeat {
                    frequency: RepeatFrequency::EveryWeekday,
                    until: Some(RepeatUntil::Count),
                    count: Some(12),
                    ..Repeat::default()
                }),
                ..UpdateCalendarEventParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "repeat_frequency=every_weekday",
            "calendar_recurrence_schedule[recurs_until_type]=count",
            "calendar_recurrence_schedule[recurs_count]=12",
        ]
    );
}

#[tokio::test]
async fn an_occurrence_update_names_the_day_and_keeps_the_series_repeating() {
    let server = MockServer::start().await;
    mock_occurrence_update(&server).await;
    let occurrence: OccurrenceId = "153688907_2026-08-21".parse().unwrap();

    let recording = client(&server)
        .calendar_events()
        .update_occurrence(
            &occurrence,
            OccurrenceScope::ThisOnly,
            &UpdateOccurrenceParams {
                title: Some("Summer Friday".to_string()),
                ..UpdateOccurrenceParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(recording.id, 153688908);
    let requests = server.received_requests().await.unwrap();
    assert_eq!(
        requests[0].url.path(),
        "/calendar/events/153688907/occurrences/2026-08-21.json"
    );
    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[summary]=Summer Friday",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "apply_to_future=0",
            "repeat_frequency=custom",
        ]
    );
}

/// Naming a recurrence on an occurrence update changes the series' schedule on purpose, and
/// the wider scope splits the series at this day.
#[tokio::test]
async fn an_occurrence_update_can_reschedule_the_series_from_this_day_on() {
    let server = MockServer::start().await;
    mock_occurrence_update(&server).await;
    let occurrence: OccurrenceId = "153688907_2026-08-21".parse().unwrap();

    client(&server)
        .calendar_events()
        .update_occurrence(
            &occurrence,
            OccurrenceScope::ThisAndFollowing,
            &UpdateOccurrenceParams {
                start_time: Some("10:30".to_string()),
                repeat: Some(Repeat {
                    frequency: RepeatFrequency::EveryWeek,
                    ..Repeat::default()
                }),
                ..UpdateOccurrenceParams::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[starts_at_time]=10:30:00",
            "calendar_event[description]=",
            "calendar_event[location]=",
            "calendar_event[url]=",
            "calendar_event[entry_id]=",
            "repeat_frequency=every_week",
            "apply_to_future=1",
        ]
    );
}

/// The delete asks for the JSON representation HEY serves it in — the `.json` path with a
/// JSON `Accept` — rather than the redirect its web form answers with.
#[tokio::test]
async fn deleting_an_event_asks_for_json() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path("/calendar/events/99.json"))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;

    client(&server).calendar_events().delete(99).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].headers["accept"], "application/json");
}

/// A zero entry id names no entry, so it goes out blank rather than as `0` — which HEY
/// would look up and refuse.
#[tokio::test]
async fn a_zero_entry_id_attaches_nothing_rather_than_entry_zero() {
    let server = MockServer::start().await;
    mock_create(&server, ResponseTemplate::new(201).set_body_json(event())).await;

    client(&server)
        .calendar_events()
        .create(&CreateCalendarEventParams {
            calendar_id: 21,
            title: "Standup".to_string(),
            starts_at: "2026-09-02".to_string(),
            all_day: true,
            content: EventContent {
                entry_id: Some(0),
                ..EventContent::default()
            },
            ..CreateCalendarEventParams::default()
        })
        .await
        .unwrap();

    let sent = sent_form(&server).await;
    assert!(
        sent.contains(&"calendar_event[entry_id]=".to_string()),
        "{sent:?}"
    );
}

#[tokio::test]
async fn only_the_wider_scope_asks_for_the_days_after_an_occurrence() {
    let server = MockServer::start().await;
    Mock::given(method("DELETE"))
        .and(path(
            "/calendar/events/153688907/occurrences/2026-08-21.json",
        ))
        .respond_with(ResponseTemplate::new(204))
        .mount(&server)
        .await;
    let occurrence: OccurrenceId = "153688907_2026-08-21".parse().unwrap();
    let client = client(&server);

    client
        .calendar_events()
        .delete_occurrence_scoped(&occurrence, OccurrenceScope::ThisOnly)
        .await
        .unwrap();
    client
        .calendar_events()
        .delete_occurrence_scoped(&occurrence, OccurrenceScope::ThisAndFollowing)
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.query(), Some("apply_to_future=false"));
    assert_eq!(requests[1].url.query(), Some("apply_to_future=true"));
}

#[tokio::test]
async fn the_narrow_revision_carries_dates_without_clock_times_when_it_is_all_day() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update(
            99,
            &CalendarEventUpdate {
                title: Some("Sarah's birthday".to_string()),
                starts_at: Some("2026-09-02".to_string()),
                ends_at: Some("2026-09-02".to_string()),
                all_day: Some(true),
                start_time: Some("09:00".to_string()),
                end_time: Some("10:00".to_string()),
            },
        )
        .await
        .unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[0].url.path(), "/calendar/events/99.json");
    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[summary]=Sarah's birthday",
            "calendar_event[starts_at]=2026-09-02",
            "calendar_event[ends_at]=2026-09-02",
            "calendar_event[all_day]=1",
        ]
    );
}

#[tokio::test]
async fn the_narrow_revision_carries_the_clock_times_to_the_second() {
    let server = MockServer::start().await;
    mock_update(&server, updated()).await;

    client(&server)
        .calendar_events()
        .update(
            99,
            &CalendarEventUpdate {
                all_day: Some(false),
                start_time: Some("09:00".to_string()),
                end_time: Some("10:30".to_string()),
                ..CalendarEventUpdate::default()
            },
        )
        .await
        .unwrap();

    assert_eq!(
        sent_form(&server).await,
        [
            "calendar_event[all_day]=0",
            "calendar_event[starts_at_time]=09:00:00",
            "calendar_event[ends_at_time]=10:30:00",
        ]
    );
}

#[test]
fn occurrence_ids_round_trip() {
    let occurrence: OccurrenceId = "153688907_2026-08-21".parse().unwrap();

    assert_eq!(occurrence.event_id, 153688907);
    assert_eq!(occurrence.date, Date::new(2026, 8, 21).unwrap());
    assert_eq!(occurrence.to_string(), "153688907_2026-08-21");
}

#[test]
fn an_unreadable_occurrence_id_is_refused() {
    for source in [
        "",
        "153688907",
        "153688907_",
        "153688907_2026-08",
        "153688907_2026-13-40",
        "153688907_20260821",
        "_2026-08-21",
        "0_2026-08-21",
        "summer_2026-08-21",
        "153688907_2026-08-21_extra",
    ] {
        assert!(
            source.parse::<OccurrenceId>().is_err(),
            "{source:?} was accepted as an occurrence id"
        );
    }
}

#[test]
fn occurrence_scopes_read_back_from_their_names() {
    assert_eq!(
        "this_event".parse::<OccurrenceScope>().unwrap(),
        OccurrenceScope::ThisOnly
    );
    assert_eq!(
        "this_and_following".parse::<OccurrenceScope>().unwrap(),
        OccurrenceScope::ThisAndFollowing
    );
    assert!("everything".parse::<OccurrenceScope>().is_err());
}

async fn mock_create(server: &MockServer, answer: ResponseTemplate) {
    Mock::given(method("POST"))
        .and(path("/calendar/events.json"))
        .respond_with(answer)
        .mount(server)
        .await;
}

async fn mock_update(server: &MockServer, answer: ResponseTemplate) {
    Mock::given(method("PATCH"))
        .and(path("/calendar/events/99.json"))
        .respond_with(answer)
        .mount(server)
        .await;
}

async fn mock_occurrence_update(server: &MockServer) {
    Mock::given(method("PATCH"))
        .and(path(
            "/calendar/events/153688907/occurrences/2026-08-21.json",
        ))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!({ "id": 153688908, "type": "Calendar::Event" })),
        )
        .mount(server)
        .await;
}

fn updated() -> ResponseTemplate {
    ResponseTemplate::new(200).set_body_json(json!({ "id": 99, "type": "Calendar::Event" }))
}

fn event() -> Value {
    json!({
        "id": 99,
        "title": "Meeting",
        "type": "Calendar::Event",
        "all_day": false,
        "starts_at": "2026-04-06T14:00:00Z",
        "ends_at": "2026-04-06T15:00:00Z",
        "calendar": { "id": 1, "name": "David Heinemeier Hansson" }
    })
}

/// The form the one request carried, as `name=value` in the order it was written: the
/// repeated keys and the empty values both matter to HEY, and neither survives a map.
async fn sent_form(server: &MockServer) -> Vec<String> {
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);
    url::form_urlencoded::parse(&requests[0].body)
        .map(|(name, value)| format!("{name}={value}"))
        .collect()
}
