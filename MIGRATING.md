# Migrating

Breaking changes to the SDKs' public surface, by release, with what to change. Wire behavior
is not versioned here; the conformance fixtures under `conformance/tests/` hold that.

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
