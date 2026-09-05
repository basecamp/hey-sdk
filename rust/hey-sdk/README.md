# HEY Rust SDK

The Rust client for the [HEY](https://www.hey.com) API. Types, routes and service methods are
generated from the Smithy model in the repository's `spec/` directory, so what the crate offers
is what HEY serves.

The crate is not on crates.io. Depend on it from the repository:

```toml
[dependencies]
hey-sdk = { git = "https://github.com/basecamp/hey-sdk", version = "0.29" }
tokio = { version = "1", features = ["macros", "rt-multi-thread"] }
```

Requires Rust 1.88 or newer.

## Authenticate

A fixed token, for scripts and anything that already holds one:

```rust
use hey_sdk::{Client, Config, StaticTokenProvider};

let client = Client::new(Config::default(), StaticTokenProvider::new(std::env::var("HEY_TOKEN")?))?;
```

OAuth 2.0 with PKCE, for user-facing apps: `hey_sdk::oauth` does discovery, PKCE, the code
exchange and refresh, and `hey_sdk::credentials::AuthManager` keeps the resulting credentials in
a `CredentialStore` and refreshes them as needed. `AuthManager` is a `TokenProvider`, so it
plugs straight into the client:

```rust
use std::sync::Arc;

use hey_sdk::credentials::{AuthManager, DefaultCredentialStore};

let store = Arc::new(DefaultCredentialStore::default_location());
let auth = AuthManager::new(Config::default(), reqwest::Client::new(), store);
let client = Client::new(Config::default(), auth)?;
```

`DefaultCredentialStore` uses the platform's secret store where it answers and a private JSON
file under the config directory where it does not; `KeyringCredentialStore`,
`FileCredentialStore` and `InMemoryCredentialStore` pick one outright. Anything else implements
`TokenProvider`, or `AuthStrategy` to control the request headers outright.

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
use hey_sdk::services::write_info;
use reqwest::Method;

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

`on_operation_gate` is the one callback that can refuse a call before it is sent, and the only
one that may wait — which is how the bulkhead below holds a call back rather than turning it
away.

### Retries, refresh and caching

Idempotent operations, as the model marks them, are resent on 429, 500, 502, 503 and 504 with
exponential backoff (`max_retries`, `base_delay`, `max_delay`, `max_jitter` on the builder),
honouring `Retry-After` on a 429. Any operation is resent once after a 401 that the token
provider's `refresh` could answer. With a `ResponseCache` (`InMemoryCache`, `FileCache`, or
`config.cache_enabled`), JSON reads revalidate with `If-None-Match` and a 304 is answered from
the cache. Response bodies are capped at `max_response_body_bytes` (16 MiB by default).

### Resilience

`circuit_breaker`, `bulkhead` and `rate_limit` on the builder — or `resilience` for all three
— keep a struggling HEY from taking the caller down with it. Each keeps its counters per
operation (`Boxes.ListBoxes`), so one failing operation does not shut down the reads beside
it: the breaker gives up on an operation that keeps failing, the bulkhead caps how many calls
of one kind run at once, and the limiter holds the client to a budget of its own and to any
`Retry-After` HEY sends back. A refused call answers `CircuitOpen`, `BulkheadFull` or
`RateLimit` without sending anything. Hooks installed before them still hear every operation.

## Develop

```bash
make -C rust check          # fmt, clippy, tests
make rs-generate            # regenerate src/generated from openapi.json
make rs-check-drift         # fail if src/generated is stale
make conformance-rs         # run the cross-language conformance fixtures
```

Everything under `src/generated/` is written by `rust/generator`; edit the generator or the
Smithy model, never those files. Method and service names that the generator's rule gets wrong
are settled in `rust/generator/names.toml`.
