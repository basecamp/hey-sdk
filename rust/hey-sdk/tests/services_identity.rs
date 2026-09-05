mod support;

use chrono::Weekday;
use hey_sdk::services::TimeFormat;
use serde_json::{Value, json};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

use support::client;

// HEY takes the day by name and answers it as the index the identity carries, where 0 is
// Sunday.
#[tokio::test]
async fn the_first_week_day_goes_out_as_a_name_and_comes_back_as_a_day() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/calendar/identity/first_week_day.json"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "first_week_day": 1 })))
        .mount(&server)
        .await;

    let stored = client(&server)
        .identity()
        .set_first_week_day(Weekday::Mon)
        .await
        .unwrap();

    assert_eq!(stored, Weekday::Mon);
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "identity_preference": { "first_week_day": "monday" } })
    );
}

#[tokio::test]
async fn the_time_format_goes_out_as_the_web_toggles_it() {
    let server = MockServer::start().await;
    Mock::given(method("PUT"))
        .and(path("/identity/time_format.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "time_format": "twenty_four_hour" })),
        )
        .up_to_n_times(1)
        .mount(&server)
        .await;
    Mock::given(method("PUT"))
        .and(path("/identity/time_format.json"))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(json!({ "time_format": "twelve_hour" })),
        )
        .mount(&server)
        .await;
    let client = client(&server);

    let twenty_four = client
        .identity()
        .set_time_format(TimeFormat::TwentyFourHour)
        .await
        .unwrap();
    let twelve = client
        .identity()
        .set_time_format(TimeFormat::TwelveHour)
        .await
        .unwrap();

    assert_eq!(twenty_four, TimeFormat::TwentyFourHour);
    assert_eq!(twelve, TimeFormat::TwelveHour);
    assert_eq!(
        sent_json(&server, 0).await,
        json!({ "twenty_four_hour_time_format": true })
    );
    assert_eq!(
        sent_json(&server, 1).await,
        json!({ "twenty_four_hour_time_format": false })
    );
}

#[tokio::test]
async fn a_time_format_reads_back_from_its_name() {
    assert_eq!(
        "twelve_hour".parse::<TimeFormat>().unwrap(),
        TimeFormat::TwelveHour
    );
    assert_eq!(
        "twenty_four_hour".parse::<TimeFormat>().unwrap(),
        TimeFormat::TwentyFourHour
    );
    assert!("military".parse::<TimeFormat>().is_err());
}

async fn sent_json(server: &MockServer, index: usize) -> Value {
    let requests = server.received_requests().await.unwrap();
    serde_json::from_slice(&requests[index].body).unwrap()
}
