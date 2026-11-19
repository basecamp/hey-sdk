# Migrating

Breaking changes to the SDKs' public surface, by release, with what to change. Wire behavior
is not versioned here; the conformance fixtures under `conformance/tests/` hold that.

## Rust: the first release after 0.35.1

### Dates and times are jiff's, not chrono's

chrono's maintainer has wound it down, and RustSec now lists it as unmaintained
(RUSTSEC-2026-0543, recommending jiff or time). The crate moves to [jiff](https://docs.rs/jiff/1)
1.x, whose 1.0 commits to a stable public API, and re-exports it as `hey_sdk::jiff`. The wire
format does not change: instants still read RFC 3339 with any offset and write it in UTC with
`Z`, and dates are still `YYYY-MM-DD`.

| Was | Now |
|---|---|
| `DateTime` = `chrono::DateTime<Utc>` | `DateTime` = `jiff::Timestamp` |
| `Date(pub chrono::NaiveDate)`, `From<NaiveDate>` | `Date(pub jiff::civil::Date)`, `From<jiff::civil::Date>` |
| `<Date as FromStr>::Err` = `chrono::ParseError` | `jiff::Error` |
| `Date::from_datetime_in<Tz: chrono::TimeZone>(&moment, &zone)` | `Date::from_datetime_in(&moment, &jiff::tz::TimeZone)` |
| `Date::at_midnight_utc() -> DateTime` | `-> Option<DateTime>`, `None` only for 9999-12-31 |
| `Date::weekday() -> chrono::Weekday` | `-> jiff::civil::Weekday` |
| `Identity::set_first_week_day(chrono::Weekday) -> chrono::Weekday` | `jiff::civil::Weekday` both ways |
| `Token::expires_at: Option<DateTime<Utc>>` | `Option<jiff::Timestamp>` |

- `Utc::now()` is `jiff::Timestamp::now()`; `moment + TimeDelta::seconds(n)` is
  `moment.checked_add(jiff::SignedDuration::from_secs(n))`; `later - earlier` is a
  `jiff::SignedDuration`.
- jiff's weekdays are spelled out: `Weekday::Mon` is `Weekday::Monday`, and so on. jiff's
  `Weekday` has no `Display` or `FromStr`; match on it, or use `to_sunday_zero_offset()`.
- A zone for `from_datetime_in` is `jiff::tz::TimeZone::get("America/Chicago")?`,
  `TimeZone::system()`, or a fixed offset, `TimeZone::fixed(jiff::tz::offset(9))`.
  `Date::today()` still answers in this machine's zone, and now finds it the way jiff does:
  `TZ` if set, else the system's setting, read against the platform's time zone database (a
  copy bundled on Windows).
- `Date::new`, `Date::parse`, `add_days`, `add_months`, `add_years`, `days_since`, `year`,
  `month`, `day` and `Display` keep their signatures and answers, and `Date::default()` is
  still 1970-01-01.
- A token stored as JSON by an earlier release reads back unchanged: both crates write
  `expires_at` as RFC 3339 in UTC.
- Instants differ from chrono's only at the edges: a leap second (`23:59:60`) reads as
  `23:59:59`, and more than nine fractional digits is refused rather than truncated. jiff also
  reads a few forms chrono refused, such as a time without seconds and an RFC 9557 zone suffix
  (`[America/Chicago]`).
- Code that keeps chrono at its own boundary converts there:
  `jiff::Timestamp::new(dt.timestamp(), dt.timestamp_subsec_nanos() as i32)?` for an instant,
  `Date::new(d.year(), d.month(), d.day())` for a date, or the `jiff-chrono-conversions` crate
  for both directions.

## Rust, TypeScript, Kotlin and Swift: the first release after 0.34.0

### A workflow stage is read as JSON

HEY now answers `GET /workflows/{workflowId}/stages/{stageId}.json`, so `GetWorkflowStage` is
a JSON operation and the SDKs no longer read the stage's web page. The model's
`WorkflowStageDetail` is the stage, and each of its `topics` a `StagedTopic`: the topic as
`GetTopic` answers it, with `staging_id`, `emails_count` and `notes_count`.

The hand-written stage read keeps its signature and its answer everywhere:
`Workflows::stage` (Rust), `client.workflows.stage` (Kotlin, Swift) and Go's
`WorkflowsService.GetStage` still answer a `WorkflowStageView` of `WorkflowStageTopic`s,
`entry_count` still the thread's email count. What changes:

- The generated `get_stage` / `getStage` / `getWorkflowStage` answers the stage rather than
  the page: `WorkflowStageDetail` in Rust (`GetWorkflowStageResponseContent`), Kotlin and
  Swift, and the stage object as `data` in TypeScript, where it was a `string`.
  `GetWorkflowStageOutputPayload` is gone.
- `WorkflowStageView::parse(html, stage_id)` (Rust), `WorkflowStageView.parse(html, stageId)`
  (Kotlin) and `WorkflowStageView.parse(_:stageId:)` (Swift) are gone, with the HTML parsers
  behind them. Convert what `get_stage` answers instead:

```rust
// before
let view = WorkflowStageView::parse(&page, stage_id)?;

// after
let view = WorkflowStageView::from(client.workflows().get_stage(workflow_id, stage_id).await?);
```

  Kotlin's is `WorkflowStageView.from(stage)`, Swift's `WorkflowStageView(stage)`.
- A stage HEY does not have is its 404, `not_found` as before (it was also what a page
  without the stage read as).
- The Rust crate no longer depends on `scraper` or `ego-tree`, nor Go's module directly on
  `golang.org/x/net`.

## Go, TypeScript and Rust: the first release after 0.33.1

### A client with no credentials is refused when it is built

HEY's OAuth token endpoint is now a generated operation (`CreateToken`, `RevokeToken`) that goes
out unsigned. A client made only to call it holds no credentials and says so with
`NoCredentials`. A client given nothing at all is a forgotten token, so it is now refused when
it is built rather than failing at its first send:

- **Go.** `hey.NewClient` panics when given neither a token provider nor an auth strategy. Such a
  client already crashed on a nil pointer at its first send. Build a credential-less client with
  `hey.WithAuthStrategy(hey.NoCredentials{})`.
- **TypeScript.** `new HeyClient(...)` throws a `usage` `HeyError` when `token` is neither a string nor a
  `TokenProvider`. Before, it failed with a `TypeError` at the first send. Pass
  `token: NoCredentials`.

On a `NoCredentials` client, every signed operation fails `auth_required` before anything is
sent.

### Rust: `Route` gains `unsigned` and `no_redirect`

A `Route` written literally for `Client::operation` adds both fields:

```rust
// before
Route { id, service, method, /* … */ lenient_success: false, pagination, retry }

// after
Route { id, service, method, /* … */ lenient_success: false, unsigned: false, no_redirect: false, pagination, retry }
```

`false` for both keeps the route as it was: signed, and following redirects as before.

## Rust: the first release after 0.32.1

### `CachedResponse` keeps the headers that came with the body

A 304 need not repeat the `Link` and `X-Total-Count` of the 200 it confirms, so a cache
entry now holds the headers it was answered with, and a 304 is answered under them, updated
by whatever the 304 carried (RFC 9111 §4.3.4). `CachedResponse` gains a `headers: HeaderMap`
field. A `ResponseCache` of your own that builds one literally adds the field, and keeps the
headers it is handed in `set` to give them back from `get`:

```rust
// before
CachedResponse { etag, body }

// after
CachedResponse { etag, body, headers }
```

An entry whose headers a cache cannot give back can use `HeaderMap::new()`; a 304 is then
answered under its own headers alone, as before. `ResponseCache` also gains `revalidate`,
which a 304 calls to keep its validator and headers; its default reads the entry and sets it
again, so a cache of your own needs no change for it. Override it to skip writing the body
again, or to make the check and update a single step (the default can put back an older but
still consistent entry over one stored meanwhile, which the next read refetches). `FileCache` writes them to
`responses/<key>.headers`, the file the Go SDK reads, and takes an entry written without one
for a miss, so each page is fetched in full once after the upgrade.

## Every SDK: the first release after 0.31.1

### Delivering a message answers what HEY delivered

The calls that deliver a message used to answer nothing; they now answer a `SentMessage`,
HEY's answer for the delivery: the entry that went out (`id`), the thread it is on
(`topic_id`), its `subject`, and whether Undo Send is holding it back (`delayed`, with
`notice`, `undo_action` and `undo_timeout` while it is). A reply that breaks out into a
thread of its own on a Domains account names the new thread, not the one replied to.

| SDK | calls | was | now |
|---|---|---|---|
| Go | `Messages().Create`, `Messages().Send`, `Messages().SendDraft`, `Entries().CreateReply` | `error` | `(*generated.SentMessage, error)` |
| Rust | `messages().send`, `messages().send_draft`, `entries().reply` | `Result<(), Error>` | `Result<SentMessage, Error>` |
| Kotlin | `messages.send`, `messages.sendDraft`, `entries.reply` | `Unit` | `SentMessage` |
| Swift | `messages.send`, `messages.sendDraft`, `entries.reply` | `Void` | `SentMessage` (`@discardableResult`) |

In Go, a caller that only checked the error takes the answer too:

```go
// before
if err := client.Messages().Create(ctx, subject, body, to, nil, nil); err != nil {

// after
sent, err := client.Messages().Create(ctx, subject, body, to, nil, nil)
if err != nil {
```

Kotlin and Swift callers that ignore the answer need no change; Rust callers that `?` the
call need none either.

Every member is optional, because a HEY that predates the ids answers `{}` — or only the
undo members while the delivery is delayed. Check for a zero or absent `id` and `topic_id`
before using one. The conveniences above read `delayed` from `undo_action` when HEY does not
say. TypeScript has no delivery conveniences: its generated operations answer the
`SentMessage` exactly as HEY served it, so there `delayed` is absent wherever HEY leaves it
out.

The generated operations change with them: `CreateMessage`, `UpdateMessage` and
`CreateReply` now decode their 200 answer as `SentMessage` (`CreateMessageResponseContent`,
`UpdateMessageResponseContent`, `CreateReplyResponseContent`) in every SDK. They are
marked `@heyLenientSuccess` (ADR-005): a 2xx whose body is empty, cannot be read or does not
decode is an empty result rather than an error, since the message has gone out — Go's
`Parse*Response` leaves `JSON200` nil, TypeScript's `data` is `undefined`, and Rust, Kotlin and
Swift answer what `{}` decodes to. A refused delivery is still an error. Saving a draft
through them answers 204 with no body, which reads as that empty result; the draft's id is in
the `Location` header. In Go, Rust, Kotlin and Swift, save a draft through the draft
conveniences (`CreateDraft`, `UpdateDraft`, `CreateReplyDraft` and their equivalents), which
read that header. TypeScript has no draft conveniences: read the draft's id from the
response's `Location` header.

`Route` gains `lenient_success` in Rust (a struct literal needs it), and `lenientSuccess` in
Kotlin and Swift (defaulted to `false`).

## Rust: the first release after 0.30.0

### Response-side types and open enums are `#[non_exhaustive]`

Every type the crate decodes and hands back, and every enum whose set of values is HEY's to
grow, is now `#[non_exhaustive]`. The policy — which side a type is on and what a change to
it costs — is in [rust/hey-sdk/README.md](rust/hey-sdk/README.md#versioning); this is what
it changes for code outside the crate.

**Building one literally no longer compiles**, `..Default::default()` included. This reaches
tests and fixtures more than applications. The generated types and most hand-written results
keep `Default`, so build one and set what the test needs:

```rust
// before
let mailbox = Mailbox { name: "Imbox".into(), ..Default::default() };

// after
let mut mailbox = Mailbox::default();
mailbox.name = "Imbox".into();
```

A few carry no `Default`, and a fixture for one of those comes from where the crate itself
gets it: `Token`, `DeletedCalendar` and `DeletedRecording` deserialize from JSON;
`client::Response`, `FormResponse`, `RequestInfo` and `RequestResult` come out of a call
against a mock server (the crate's own tests use wiremock for this); `url::Match` comes from
`Router::recognize`.

**A `match` over an open enum needs a wildcard arm**, and a struct pattern needs `..`:

```rust
match error.code() {
    ErrorCode::NotFound => …,
    ErrorCode::RateLimit => …,
    _ => …,
}
```

Reading fields, calling methods, `Clone`, `PartialEq` and serde are untouched.

The types affected:

- Generated: every schema the model reads back and nothing the model sends. Request bodies
  (`*RequestContent`) and everything they mention (`MessagePayload`, `ContactPayload`,
  `StickyPayload`, …) stay literal; the generator decides by reachability from the request
  bodies, not by name.
- Hand-written results: `WorkflowSummary`, `CalendarChanges`, `RecordingChanges`,
  `DeletedRecording`, `DeletedCalendar`, `PostingChanges`, `SearchResults`,
  `ContactConflict`, `ListedCalendar`, `CalendarList`, `services::extenzions::Extenzion`,
  `url::Match`.
- Runtime: `client::Response`, `FormResponse`, `oauth::Token`, and what the hooks see —
  `RequestInfo` and `RequestResult`. `OperationInfo` stays literal: `Operation::info` takes
  one from a caller describing its own form-backed call. `Route`, `RouteParam` and `Retry`
  stay literal too: a caller may write a route for a path the model lacks and hand it to
  `Client::operation`.
- Open enums: `ErrorCode`, `route::Pagination`, `route::ParamKind`, `BoxKind`, `StickySize`,
  `BubbleUpSlot`, `RepeatFrequency`, `CountdownUnit`, `TimeFormat`.

Not affected, on purpose: `Config`, the resilience configs, `ExchangeRequest`,
`RefreshRequest`, `ServerMetadata`, `Pkce`, `CachedResponse`, `OperationInfo`, `Route`,
`RouteParam`, `Retry`, the `*Params` structs, the `*Cursor` structs, and the closed enums
`ClearanceStatus`, `OccurrenceScope`, `RepeatUntil` and `ParamRole`. A field added to one of
those is a `0.MINOR` release.
