# Migrating

Breaking changes to the SDKs' public surface, by release, with what to change. Wire behavior
is not versioned here; the conformance fixtures under `conformance/tests/` hold that.

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
`UpdateMessageResponseContent`, `CreateReplyResponseContent`) in every SDK. Saving a draft
through them answers 204 with no body, which is not that shape — save a draft through the
draft conveniences (`CreateDraft`, `UpdateDraft`, `CreateReplyDraft` and their
equivalents), which read the `Location` instead.

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
