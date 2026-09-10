# HEY Rust SDK

The Rust client for the [HEY](https://www.hey.com) API. Types, routes and service methods are
generated from the Smithy model in the repository's `spec/` directory, so what the crate offers
is what HEY serves.

```toml
[dependencies]
hey-sdk = "0.30"
tokio = { version = "1", features = ["macros", "rt-multi-thread"] }
```

The crate is on [crates.io](https://crates.io/crates/hey-sdk) and documented on
[docs.rs](https://docs.rs/hey-sdk). To track the repository instead, depend on it at a release
tag — the repository's `vX.Y.Z` tags are the crate's releases, and there is no `rust/vX.Y.Z`
tag to look for:

```toml
hey-sdk = { git = "https://github.com/basecamp/hey-sdk", tag = "v0.30.0" }
```

Requires Rust 1.88 or newer; see [Versioning](#versioning).

The [examples](examples) are this page's snippets as whole programs, and CI compiles them:

```sh
HEY_TOKEN=... cargo run --example first_call      # identity and boxes
HEY_TOKEN=... cargo run --example pagination      # page by page, and to the end
HEY_TOKEN=... cargo run --example hooks           # every operation, request and resend
cargo run --example oauth_pkce                    # the whole PKCE login, then a call
cargo run --example custom_http_client --no-default-features   # a transport of your own, offline
```

## Authenticate

A fixed token, for scripts and anything that already holds one:

```rust
use hey_sdk::{Client, Config, StaticTokenProvider};

let client = Client::new(Config::default(), StaticTokenProvider::new(std::env::var("HEY_TOKEN")?))?;
```

OAuth 2.0 with PKCE, for user-facing apps: `hey_sdk::oauth` speaks the protocol — discovery,
PKCE, the authorization URL, the code exchange and refresh. HEY wants an `install_id` on all
three requests, a stable identifier the application mints once per installation, so every
call takes one.

```rust
use hey_sdk::oauth::{self, ExchangeRequest, OAuthClient, ServerMetadata};

let oauth = OAuthClient::default();
let metadata = ServerMetadata::for_hey(&config.base_url);   // HEY publishes no well-known document
let pkce = oauth::generate_pkce();
let state = oauth::generate_state();
let url = oauth::authorization_url(&metadata, &config.oauth_client_id, redirect_uri, None, &state, &pkce, install_id)?;
// Send the person to `url`, receive the code on `redirect_uri`, then:
let token = oauth.exchange(&ExchangeRequest {
    token_endpoint: metadata.token_endpoint,
    code,
    redirect_uri: redirect_uri.to_string(),
    client_id: config.oauth_client_id.clone(),
    client_secret: None,
    code_verifier: pkce.verifier,
    install_id: install_id.to_string(),
}).await?;
```

Where the tokens live between runs, and when to refresh them, is the application's business:
it implements `TokenProvider` over its own store and hands that to `Client::new`. Anything
that wants the request headers outright implements `AuthStrategy` instead.

## Use it

```rust
use hey_sdk::services::{BoxKind, MessageContent, ReplyContent};

let boxes = client.boxes().list().await?;   // a Page: derefs to the response it wraps
for mailbox in boxes.iter() {
    println!("{} ({})", mailbox.name, mailbox.kind);   // Imbox, The Feed, Paper Trail, ...
}
let imbox = client.boxes().get_imbox(&Default::default()).await?;
println!("{} postings", imbox.postings.unwrap_or_default().len());

// Sending: recipients are required. HEY saves an unaddressed message as a draft.
client.messages().send(&MessageContent {
    subject: "Subject".into(),
    content: "<div>Body</div>".into(),
    to: vec!["someone@example.com".into()],
    ..Default::default()
}).await?;

// Replying: start from the prefill. It carries the subject, the acting sender and the
// recipients HEY resolved, which differ from the account default on shared addresses.
let prefill = client.entries().new_reply(entry_id).await?;
client.entries().reply(entry_id, &ReplyContent {
    acting_sender_id: prefill.sender.as_ref().map(|sender| sender.id).unwrap_or_default(),
    subject: prefill.subject.clone().unwrap_or_default(),
    content: "<div>Reply</div>".into(),
    to: prefill
        .addressed
        .iter()
        .flat_map(|addressed| addressed.directly.iter().flatten())
        .filter_map(|contact| contact.email_address.as_ref())
        .map(|address| address.expose().to_string())
        .collect(),
    ..Default::default()
}).await?;

// Postings are bulk operations, as they are in HEY. Moving by kind resolves the box index
// once per client.
client.postings().mark_postings_seen(&[a, b]).await?;
client.postings().move_to_set_aside(&[a]).await?;
let trail = client.boxes().id_by_kind(BoxKind::PaperTrail).await?;

// Calendar
let track = client.time_tracks().start_tracking().await?;
let ongoing = client.time_tracks().get_ongoing().await?;   // Option: None when nothing is running
```

### Services

One handle per resource, all off the client: `attachments`, `boxes`, `bulk_replies`,
`calendar_events`, `calendar_periods`, `calendar_todos`, `calendars`, `clearances`, `clips`,
`collections`, `contacts`, `designations`, `entries`, `extenzions`, `folders`, `habits`,
`identity`, `journal`, `messages`, `postings`, `publications`, `search`, `snippets`,
`stickies`, `time_tracks`, `topics`, `workflows`, `world`.

Every method the model describes is generated, and named for the operation with the service's
noun dropped: `ListBoxes` is `boxes().list()`, `GetBoxPostingChanges` is
`postings().get_box_changes(..)`. Operation ids, methods and paths are all in `hey_sdk::routes`.

On top of those, `src/services/*.rs` are hand-written: they take the arguments a caller has
rather than a request body, and cover the parts of HEY the model cannot describe. A
hand-written method keeps the plain name where the generated service leaves it free, and takes
the model's own name for the operation where it does not — `postings().mark_postings_seen(&[a])`
alongside the generated `mark_seen(&body)`. One that changes the shape of the call rather than
only its arguments may take a descriptive name instead: `time_tracks().start_tracking()` names
the conflict a running track answers with, `calendars().toggle_selection(..)` answers the
selection rather than the payload holding it.

Schema names are the model's own, with one exception: `Box` is `hey_sdk::models::Mailbox`, so
it does not shadow `std`'s. The rename is Rust's alone — a type name never goes on the wire —
and lives in `rust/generator/names.toml`.

```rust
use hey_sdk::services::{BubbleUpSlot, ClearanceStatus, ContactParams, PostingChangesCursor};

// The Screener
for waiting in client.clearances().pending(None).await?.clearances.unwrap_or_default() {
    client.clearances().screen(waiting.id, ClearanceStatus::Approved, &Default::default()).await?;
}
client.contacts().screen(contact_id, ClearanceStatus::Denied).await?;
client.designations().create_box_designation(box_id, contact_id).await?;

// Contacts: update reads the contact first, since HEY's write is a full replacement
client.contacts().create_contact(&ContactParams {
    name: "Jane Dawson".into(),
    email_address: "jane@example.com".into(),
    ..Default::default()
}).await?;

// Bubble Up, and the incremental sync feed the mail clients follow
client.postings().schedule_postings_bubble_up(BubbleUpSlot::NextWeek, &[a]).await?;
let cursor = PostingChangesCursor::from_url(mailbox.posting_changes_url.as_deref().unwrap_or_default())?;
let changes = client.postings().all_changes(mailbox.id, &cursor).await?;
if changes.full_sync_required {
    // the cursor fell too far behind — read the box again
}
```

### Form-backed writes

Workflows, collections, snippets, clips, publications, HEY World and the calendar writes have
no JSON surface: HEY serves them only as browser forms that answer a redirect. The services
cover them, so a caller does not have to know:

```rust
use hey_sdk::services::CreateCollectionParams;

client.workflows().create("Launch", None).await?;
client.workflows().stage_topic(topic_id, workflow_id, stage_id).await?;
client.collections().create(&CreateCollectionParams {
    name: "Launch".into(),
    ..Default::default()
}).await?;
let publication = client.publications().publish(topic_id).await?;   // .url is the public link
let token = client.world().publish("Subject", "<div>Body</div>").await?;
```

For a form endpoint nothing covers, `Client::form` builds the request — the path as written, a
browser's `Accept`, the redirect captured rather than followed, and never retried — and
`Client::send_form` sends it:

```rust
use hey_sdk::http::Method;
use hey_sdk::services::write_info;

let mut operation = client.form(Method::POST, "/workflows")?;
operation.info(write_info("Workflows", "CreateWorkflow", "workflow", None));
operation.form(&[("workflow[name]", "Launch")]);
let created = client.send_form(operation).await?;
let workflow_id = created.extract_id()?;         // out of the redirect's Location
```

The model describes none of these paths, so `write_info` is what the call tells the hooks it
is; without it they only hear that something raw went out.

`post_form`, `patch_form`, `delete_form` and `post_multipart` are those two together for the
common shapes. Unlike Go, a form failure keeps the code, hint and request id HEY answered with
rather than being flattened to "Form request failed (HTTP 503)".

### Beyond the model

`client.request(method, path)` builds an `Operation` for a path the model does not cover, with
the same credentials, `.json` suffix, account scope and retry treatment. The raw verbs are that
plus a send: `get`, `get_html`, `get_csv`, `get_blob`, `download_blob`, `post`, `put`, `patch`,
`delete`, their `_mutation` variants for endpoints that answer something other than JSON, and
`get_all` to walk a paginated path to its end. `hey_sdk::url::router()` recognizes pasted HEY
URLs and names the operation and ids they refer to.

An `Operation` marked `quiet` skips the operation hooks while still firing the request ones,
for a read-back made inside another operation — which is how publishing a thread reports one
operation rather than two.

### Pages

Reads HEY paginates answer a `Page<T>`. It derefs to the response, and carries the next
cursor and `X-Total-Count`:

```rust
let mut page = client.contacts().list(&Default::default()).await?;
while let Some(next) = client.next_page(&page).await? {
    page = next;
}
```

`next_page` refuses a `Link` header that points off the HEY origin. `each_page` walks pages
up to the client's `max_pages`.

### Linked accounts

A root client presents mail from All Accounts. Derive one for a linked account to present that
account's mail and act as its user and default sender:

```rust
let work = client.for_account(work_account_id).await?;
let postings = work.boxes().get_imbox(&Default::default()).await?;
```

`for_account` checks the account against the identity first, then adds HEY's
`filtered_account_id` to every same-origin request. Calendar, journal, habits and time tracking
belong to the identity and read the same through a scoped client.

### Errors

Every call answers `Result<_, hey_sdk::Error>`. The error carries a stable `ErrorCode`
(`NotFound`, `Auth`, `Forbidden`, `RateLimit`, `Validation`, `Api`, `Usage`, ...), the HTTP
status, whether it is worth retrying, HEY's `X-Request-Id`, and a hint when the server or the
SDK had one. It also keeps the body HEY answered the failure with, for the endpoints that
describe a refusal there rather than in the status:

```rust
match client.contacts().create_contact(&params).await {
    Err(error) if error.http_status() == Some(409) => {
        let clash = error.body_json::<serde_json::Value>();
        // ContactConflict::from_error(&error) reads that same body, typed
    }
    other => { other?; }
}
```

### Hooks

`hooks` on the builder reports every operation and every request the client makes: what the
call means (`Boxes.ListBoxes`, the record it names, whether it changes anything), each attempt
and how it turned out, and every resend before it is made. Every callback does nothing by
default, so an implementation says only what it cares about, and several sets go on as one with
`ChainHooks`:

```rust
use hey_sdk::observability::{Hooks, RequestInfo, RequestResult};

struct Log;

impl Hooks for Log {
    fn on_request_end(&self, info: &RequestInfo, result: &RequestResult<'_>) {
        println!("{} {} -> {:?} in {:?}", info.method, info.url, result.status, result.duration);
    }
}

let client = Client::builder(Config::default()).token_provider(provider).hooks(Log).build()?;
```

The `tracing` feature, on by default, opens one `tracing` span per operation — `hey.operation`,
with `operation`, `service`, and once HEY has answered `http.status` and `request_id` — and a
`hey.attempt` child span per send, numbered the way the hooks number attempts, with the status
each one got. Spans are put on the futures with `Instrument`, so concurrent calls keep their
own, and a call the caller drops closes its span with no status. Nothing the caller passed is
recorded: no path, no query, no body — a request for a path the caller wrote is named by its
method alone, and the hooks are where its URL goes. A quiet send — a read-back inside another operation —
opens no span of its own and runs in whichever span its caller is in. Any `tracing-subscriber`
sees them; with `default-features = false` (plus `reqwest` if wanted) the crate depends on
`tracing` for nothing and emits nothing. The hooks stay the place for a policy or a metric:
they carry the whole `RequestResult`, and they run whether or not `tracing` is on.

`on_operation_gate` is the one callback that can refuse a call before it is sent, and the only
one that may wait — which is how the bulkhead below holds a call back rather than turning it
away.

### Retries, refresh and caching

Every modelled operation carries its retry policy on its route (`routes::LIST_BOXES.retry`):
`max`, the sends it gets in all; `retry_on`, the statuses that earn another; and the first
wait between them. The client honours that policy on the first request and on every page
`next_page` and `each_page` read after it, and only ever makes it gentler: the sends are
`min(max, max_retries + 1)`, `base_delay` is the least the client waits before the first
resend, `max_jitter` is added to every wait, and `max_delay` is the most it waits between any
two, jitter included — the one setting that can shorten the policy's own wait, and the way a
test suite winds the backoff down. A positive `Retry-After` on a 429 the policy names is honoured as given,
as a count of seconds or as an HTTP-date, above `max_delay` if need be. A route the model
gives no policy is sent once, and so is any operation that is not idempotent, whatever its
policy says. A path the caller wrote has no policy to bring, so an idempotent one runs on the
client's settings alone and is resent on 429, 500, 502, 503 and 504; `get_all` and
`follow_pagination` read every page that way. Any operation is resent once after a 401 that
the token provider's `refresh` could answer, even with its sends spent. With a `ResponseCache` (`InMemoryCache`, `FileCache`, or
`config.cache_enabled`), JSON reads revalidate with `If-None-Match` and a 304 is answered from
the cache. Response bodies are capped at `max_response_body_bytes` (16 MiB by default).

### Bring your own HTTP client

Everything the SDK sends — API calls, OAuth token requests, the attachment bytes that go to
the storage service — goes out through one `http::HttpClient`. The `reqwest` feature, on by
default, ships `ReqwestClient` over rustls and HTTP/2, and that is what `Client::new` and
`OAuthClient::default()` use. To configure it, build it from reqwest's own builder:

```rust
use hey_sdk::http::ReqwestClient;

let http = ReqwestClient::from_builder(reqwest::Client::builder().proxy(proxy))?;
let client = Client::builder(config).token_provider(provider).http_client(http).build()?;
```

To replace it — a mobile shell on the platform's own stack, a test on canned answers —
implement the trait. It is one method over the `http` crate's types, re-exported at
`hey_sdk::http`:

```rust
use async_trait::async_trait;
use bytes::Bytes;
use hey_sdk::http::{Body, HttpClient, Request, Response};

struct PlatformHttp;

#[async_trait]
impl HttpClient for PlatformHttp {
    async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, hey_sdk::Error> {
        // hand the request to the platform, answer with its status, headers and a Body
        // built from the byte stream it gives back
    }
}
```

An implementation must not follow redirects: the SDK follows them itself, dropping the
credentials on a hop off the HEY origin and giving up after ten, and a form request's redirect
is its answer. Timeouts are the implementation's to enforce. With `default-features = false`
there is no shipped client, and `ClientBuilder::http_client` is the only way to build a
`Client`. The Go SDK takes an `*http.Client` instead; the trait is a deliberate divergence, so
the SDK never forces a second HTTP stack into a binary that already has one.

### Resilience

`circuit_breaker`, `bulkhead` and `rate_limit` on the builder — or `resilience` for all three
— keep a struggling HEY from taking the caller down with it. Each keeps its counters per
operation (`Boxes.ListBoxes`), so one failing operation does not shut down the reads beside
it: the breaker gives up on an operation that keeps failing, the bulkhead caps how many calls
of one kind run at once, and the limiter holds the client to a budget of its own and to any
`Retry-After` HEY sends back. A refused call answers `CircuitOpen`, `BulkheadFull` or
`RateLimit` without sending anything. Hooks installed before them still hear every operation.

## Versioning

The crate follows [Cargo's reading of semver](https://doc.rust-lang.org/cargo/reference/semver.html)
before 1.0: a change that breaks the public API moves the minor version (`0.29` → `0.30`), an
additive one moves the patch. CI runs `cargo semver-checks` on every pull request against the
base branch, so an accidental break is caught before it is tagged; a deliberate one carries the
`breaking` label and a version bump. What the API guarantees on the wire is the conformance
suite's business, not semver's.

`rust-version` is 1.88, and CI builds the library on exactly that toolchain. It moves only when
a dependency or a feature the crate needs requires it, and a move is a minor release with a
line in the release notes, never a patch. Development and CI otherwise run on the exact stable
that `rust-toolchain.toml` at the repository root pins, so rustfmt and clippy agree on every
machine; a new stable arrives as a bump to that file.

### How types evolve

Every public type is on one of two sides, and the side decides what a change to it costs.

- **Request-side types are built literally and are never `#[non_exhaustive]`.** Request bodies
  (`CreateMessageRequestContent`), the `*Params` structs, `Config`, the resilience configs,
  `ExchangeRequest` and `RefreshRequest`: plain structs with public fields, and `Default`
  wherever every field has one, written as `GetBoxParams { page: Some(2), ..Default::default() }`.
  Adding a field to one is a breaking change — a literal without `..Default::default()` stops
  compiling — and ships as `0.MINOR`.
- **Response-side types and open enums are `#[non_exhaustive]`.** Everything the SDK decodes
  and hands back (`Mailbox`, `Posting`, `Token`, `Route`, what the hooks see) and every enum
  whose variant set is HEY's to extend (`ErrorCode`, `BoxKind`, `Pagination`): read them,
  match them with a `..` or `_` arm, never build them. They keep `Default`, so
  `Mailbox::default()` still works where a test wants one. Adding a field or a variant is
  additive and ships as `0.x.PATCH` — provided what HEY already sends still decodes: a new
  required field with no default, a `DateTime` say, would refuse yesterday's payload, and
  that is a break whatever the attribute says.
- A type on both sides — sent in a body and read back — is request-side.
- Closed enums stay exhaustive. `ClearanceStatus`, `OccurrenceScope` and `RepeatUntil` name a
  choice the SDK defines, not a set HEY grows, so a `match` over them may stay exhaustive.
- An open enum is `#[non_exhaustive]` whichever side it sits on: its declared variants are
  still there to build a request with, and only a `match` over it has to leave room.

The generator applies the same rule by reachability: a schema reachable from any request body
is request-side, the rest are response-side. So a field the model adds to a response schema is
a patch, and one it adds to a request schema is a minor.

When the model declares an enum, the generator emits a `#[non_exhaustive]` enum with one
variant per declared value and `Unknown(String)` for any value HEY sends that the model did
not declare, carried unchanged so a read-modify-write sends back what it read. Until the model
declares one, the generator invents none: a `kind` is a `String`.

## Develop

```bash
make rs-check               # every step CI runs before the drift check, in order: fmt, clippy,
                            #   tests, docs; clippy + tests again with --no-default-features;
                            #   examples, cargo deny, cargo package
make rs-check-drift         # fail if src/generated is stale; with rs-check, the whole CI job
make rs-generate            # regenerate src/generated from openapi.json
make conformance-rs         # run the cross-language conformance fixtures
```

`make -C rust help` lists the steps one by one. `rs-check` needs
[`cargo-deny`](https://github.com/EmbarkStudios/cargo-deny) installed; the formatter's options
are in `rust/rustfmt.toml`, stable ones only. Everything under `src/generated/` is written by
`rust/generator`; edit the generator or the Smithy model, never those files. Method and
service names that the generator's rule gets wrong are settled in `rust/generator/names.toml`.
