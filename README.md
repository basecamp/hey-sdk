# HEY SDK

The SDKs for the [HEY](https://www.hey.com) API, generated from a Smithy model of the API in
`spec/`, so what an SDK offers is what HEY actually serves.

The repository ships a Go module, `github.com/basecamp/hey-sdk/go`, which is the library behind
[hey-cli](https://github.com/basecamp/hey-cli), and a Rust crate, `hey-sdk` in `rust/`. The
Go walkthrough is first; [the Rust one](#rust) follows it, and the crate's own
[README](rust/hey-sdk/README.md) goes further.

TypeScript, Ruby, Swift and Kotlin SDKs will be added in future updates, generated from the
same Smithy model; the Makefile already reserves targets for them (`ts-`, `rb-`, `swift-`,
`kt-`), which fail until those SDKs exist.

## Install

```bash
go get github.com/basecamp/hey-sdk/go@latest
```

Requires Go 1.26 or newer. The Rust crate's install line is in [its section](#install-1).

## Authenticate

Static token (scripts, agents, anything that already holds a token):

```go
import hey "github.com/basecamp/hey-sdk/go/pkg/hey"

cfg := hey.DefaultConfig() // https://app.hey.com
client := hey.NewClient(cfg, &hey.StaticTokenProvider{Token: os.Getenv("HEY_TOKEN")})
```

OAuth 2.0 with PKCE (user-facing apps): `hey.NewAuthManager` handles the token lifecycle
and refresh, and the `oauth` subpackage provides discovery, PKCE and the code exchange.
Anything else can plug in with `hey.WithAuthStrategy`, which sets headers on each request —
hey-cli uses this to bridge its own credential store.

## Use it

```go
ctx := context.Background()

boxes, _ := client.Boxes().List(ctx)          // Imbox, The Feed, Paper Trail, ...
imbox, _ := client.Boxes().GetImbox(ctx, nil)   // postings in the Imbox

// Sending: recipients are required — HEY saves an unaddressed reply as a draft.
_ = client.Messages().Create(ctx, "Subject", "Body", []string{"someone@example.com"}, nil, nil)

// Replying: start from the NewReply prefill — it carries the reply's subject, its
// acting sender (0 = the account default) and the recipients HEY resolved.
prefill, err := client.Entries().NewReply(ctx, entryID)
if err != nil {
	return err // nothing to reply with
}
var to []string
for _, contact := range prefill.Addressed.Directly {
	to = append(to, contact.EmailAddress)
}
_ = client.Entries().CreateReply(ctx, entryID, prefill.Sender.Id, prefill.Subject, "Reply body", to, nil, nil)

// Postings are bulk operations, as they are in HEY.
_ = client.Postings().MoveToSetAside(ctx, postingID)
_ = client.Postings().MarkSeen(ctx, []int64{a, b})

// Calendar
rec, _ := client.TimeTracks().Start(ctx)
_ = client.TimeTracks().Stop(ctx, rec.Id)
```

Services on the client: `Identity`, `Boxes`, `Postings`, `Topics`, `Messages`, `Entries`,
`Contacts`, `Calendars`, `CalendarTodos`, `CalendarEvents`, `Habits`, `TimeTracks`,
`Journal`, `Search`, `Folders`, `Collections`, `Stickies`, `Clips`, `Snippets`, `Workflows`,
`Publications`, `Designations`, `Extenzions`, `World`.

### Linked accounts and separate identities

A root client represents one authenticated HEY identity and presents mail from All Accounts.
Derive an immutable client to present mail and choose acting users and senders for one linked
account:

```go
work, err := client.ForAccount(ctx, workAccountID)
if err != nil {
    return err
}
postings, _ := work.Boxes().GetImbox(ctx, nil)
_ = work.Messages().Create(ctx, "Subject", "Body", []string{"someone@example.com"}, nil, nil)
```

`ForAccount` verifies that the account is accessible to the authenticated identity when the
scoped client is derived, then adds HEY's `filtered_account_id` to same-origin API requests,
including pagination and retries. Long-lived applications can derive a fresh scoped client
after observing identity or account-membership changes. It never adds the filter to signed external upload or download URLs.
Account-scoped message sends resolve a sender from that account, and account-scoped contact
creation resolves the identity's user in that account. Both operations return an error when
the account has no matching sender or user rather than falling back to another account.

Account scope follows HEY's mail-filter semantics; it is not an authorization boundary.
Identity-owned services such as Calendar and Journal remain identity-wide. Use a client
derived for a thread's account when replying or forwarding that thread.

Separate, unlinked identities use separate root clients with separate token providers or auth
strategies. Each root can independently derive its own linked-account clients:

```go
personal := hey.NewClient(cfg, personalTokenProvider)
workIdentity := hey.NewClient(cfg, workTokenProvider)

personalMail, _ := personal.ForAccount(ctx, personalAccountID)
workMail, _ := workIdentity.ForAccount(ctx, workAccountID)
```

Every call reports itself to the client's `Hooks` (`hey.WithHooks`) as a named operation —
`Postings.MovePostings`, `TimeTracks.StopTimeTrack` — and a `GatingHooks` implementation
can refuse an operation before it runs. Circuit breaking, bulkheads and rate limits are
configured with `WithResilience`, `WithCircuitBreaker`, `WithBulkhead` and `WithRateLimit`;
HTTP caching with `WithCache`. Response caching is active for requests with an
`Authorization` header, which gives each authenticated identity a stable cache partition.

Every modelled operation carries its retry policy from the API contract: how many sends it
gets in all, which statuses earn another, and the wait before the first resend. The client
honours that policy on the first request and on every page read after it, and its own
settings only ever make it gentler: `WithMaxRetries` caps the sends (an operation modelled
with two sends gets two whatever the cap, and a cap of one resend holds an operation
modelled with three to two), `WithBaseDelay` is the least the client waits before the first
resend, and a status the policy does not name is the operation's answer. An operation that
is not idempotent is sent once, and so is one the contract gives no policy. Whatever the
count, a 401 that a credential refresh answered earns one more send. A GET on a path
the caller wrote (`Get`, `GetAll`) has no policy to bring and runs on the client's settings
alone, resent on 429, 502, 503 and 504.

JSON and HTML answers are capped in the transport at `WithMaxResponseBodyBytes` (16 MiB of
decompressed body by default; the cap can be raised but not removed), success and error
responses alike. A body past it fails with an error that `errors.Is(err,
hey.ErrResponseTooLarge)`, and is not retried; a refused error response still carries its
status in the `*hey.Error`. Buffered blobs and CSV exports (`GetBlob`, `GetCSV`) are bounded
by the 50 MiB `hey.MaxResponseBodyBytes` constant instead; only `DownloadBlob` streams
without a bound.

### Errors

Calls return `*hey.Error` with a stable `Code` (`hey.CodeNotFound`, `hey.CodeAuth`,
`hey.CodeForbidden`, `hey.CodeRateLimit`, `hey.CodeConflict`, `hey.CodeUsage`, ...), the
HTTP status, and — for auth and scope problems — a hint. `hey.AsError(err)` unwraps it.

### Pagination

Paged reads follow HEY's `Link` headers automatically, up to `WithMaxPages`.

## Rust

The crate is `hey-sdk`, at `rust/hey-sdk`, with the same generated surface as the Go module and
the same hand-written conveniences on top. It is an async client on tokio, sends over rustls
by default, and lets an application bring its own HTTP stack instead.

### Install

```toml
[dependencies]
hey-sdk = "0.30"
tokio = { version = "1", features = ["macros", "rt-multi-thread"] }
```

The crate is [`hey-sdk` on crates.io](https://crates.io/crates/hey-sdk), documented on
[docs.rs](https://docs.rs/hey-sdk). To track the repository instead, depend on it at a release
tag: `hey-sdk = { git = "https://github.com/basecamp/hey-sdk", tag = "v0.30.0" }`. Requires
Rust 1.88 or newer (`rust-version` in `rust/Cargo.toml`, built on exactly that in CI); the
crate's [Versioning](rust/hey-sdk/README.md#versioning) section says when that floor moves
and what a version bump means.

### Authenticate

A fixed token, for scripts and anything that already holds one:

```rust
use hey_sdk::{Client, Config, StaticTokenProvider};

let client = Client::new(Config::default(), StaticTokenProvider::new(std::env::var("HEY_TOKEN")?))?;
```

OAuth 2.0 with PKCE, for user-facing apps: `hey_sdk::oauth` speaks the protocol — the
authorization URL, the code exchange and refresh, with the `install_id` HEY wants on each.
The application keeps the tokens and hands the client a `TokenProvider` over them; the
client asks it to `refresh` once when HEY answers 401. Anything that wants the request
headers outright implements `AuthStrategy` instead.
[`examples/oauth_pkce.rs`](rust/hey-sdk/examples/oauth_pkce.rs) is the whole flow.

### Use it

```rust
use hey_sdk::services::MessageContent;

let boxes = client.boxes().list().await?;                       // Imbox, The Feed, Paper Trail, ...
let imbox = client.boxes().get_imbox(&Default::default()).await?;

// Sending: recipients are required — HEY saves an unaddressed message as a draft.
client.messages().send(&MessageContent {
    subject: "Subject".into(),
    content: "<div>Body</div>".into(),
    to: vec!["someone@example.com".into()],
    ..Default::default()
}).await?;

// Replying: start from the prefill — the subject, the acting sender, the recipients HEY resolved.
let prefill = client.entries().new_reply(entry_id).await?;

// Postings are bulk operations, as they are in HEY.
client.postings().move_to_set_aside(&[posting_id]).await?;
client.postings().mark_postings_seen(&[a, b]).await?;

// Calendar
let track = client.time_tracks().start_tracking().await?;
client.time_tracks().stop(track.id).await?;
```

Services on the client, one handle per resource: `attachments`, `boxes`, `bulk_replies`,
`calendar_events`, `calendar_periods`, `calendar_todos`, `calendars`, `clearances`, `clips`,
`collections`, `contacts`, `designations`, `entries`, `extenzions`, `folders`, `habits`,
`identity`, `journal`, `messages`, `postings`, `publications`, `search`, `snippets`,
`stickies`, `time_tracks`, `topics`, `workflows`, `world`. Every method the model describes
is generated, named for the operation with the service's noun dropped (`ListBoxes` is
`boxes().list()`); the hand-written ones in `rust/hey-sdk/src/services` take the arguments a
caller has and cover the parts of HEY the model cannot describe. Every route is data in
`hey_sdk::routes`, and `hey_sdk::url::router()` names the operation a pasted HEY URL refers to.

### Linked accounts

A root client presents mail from All Accounts. Derive one for a linked account to present that
account's mail and act as its user and default sender; it adds HEY's `filtered_account_id` to
every same-origin request, including pagination and retries, and never to signed external URLs:

```rust
let work = client.for_account(work_account_id).await?;
let postings = work.boxes().get_imbox(&Default::default()).await?;
```

### Errors

Every call answers `Result<_, hey_sdk::Error>`: a stable `ErrorCode` (`NotFound`, `Auth`,
`Forbidden`, `RateLimit`, `Validation`, `Api`, `Usage`, ...), the HTTP status, whether it is
worth retrying, HEY's `X-Request-Id`, a hint when there was one, and the body HEY answered
the failure with, for the endpoints that describe a refusal there.

### Pagination

Paged reads answer a `Page<T>`, which derefs to the response and carries the next cursor and
`X-Total-Count`; `next_page` reads on, `each_page` walks to the client's `max_pages`, and a
`Link` that points off the HEY origin is refused.
[`examples/pagination.rs`](rust/hey-sdk/examples/pagination.rs) shows both walks.

### Beyond the Go module

Everything the crate sends goes through one `HttpClient` trait, so a binary that already has
an HTTP stack — a mobile shell on the platform's own — does not get a second one
([`examples/custom_http_client.rs`](rust/hey-sdk/examples/custom_http_client.rs) runs on a
canned transport, without the network and without the `reqwest` feature). Hooks report every
operation, request and resend ([`examples/hooks.rs`](rust/hey-sdk/examples/hooks.rs)), and
a gate can refuse an operation before it is sent. Retries, a circuit breaker, a bulkhead, a
rate limit and an ETag response cache are on the builder. Secrets are `SensitiveString`s that
print as `[REDACTED]`, response bodies are capped, and HTTPS is enforced off localhost.

The examples under [`rust/hey-sdk/examples`](rust/hey-sdk/examples) compile in CI;
`HEY_TOKEN=... cargo run --example first_call` from `rust/` is the quickest first call.

## How the SDK is built

```
spec/hey.smithy ──► openapi.json ──► oapi-codegen ──► go/pkg/generated/client.gen.go
                        │                                       │
                        │                     hand-written services in go/pkg/hey call into it
                        │
                        └─────────► rust/generator ──► rust/hey-sdk/src/generated/
                                                                │
                                          hand-written conveniences in rust/hey-sdk/src/services
```

The Smithy model is the source of truth for routes and payloads. `openapi.json`,
`behavior-model.json`, `client.gen.go`, `go/pkg/hey/url-routes.json`, everything under
`rust/hey-sdk/src/generated/` and the files under `spec/` that describe coverage are all
regenerated from it — editing them by hand is lost on the next build. The services in
`go/pkg/hey` are written by hand and add the things a generated client cannot know: which
recipients a reply needs, that HEY answers a shared topic's trash request with a confirmation
page, that starting a time track takes no body. The Rust crate generates its service methods
too, and adds those same conveniences by hand in `rust/hey-sdk/src/services`.

`make check` verifies the model against a snapshot of HEY's own routes
(`spec/route-snapshot.json`, pinned in `spec/api-provenance.json`): every modelled route
must exist in HEY, and every JSON-capable HEY route must be either modelled or listed in
`spec/excluded-routes.json` with a reason. Generated ops that HEY does not serve cannot get
in unnoticed.

A handful of services (`Clips`, `Snippets`, `Workflows`, `Publications`, `World`,
`Extenzions`, `CalendarEvents`, and parts of `Contacts` and `Search`) still talk to HEY the
way the web UI does — form posts, and for a few reads, the HTML page — because those
endpoints have no JSON yet. Both SDKs cover them: Go through `PostForm` and its neighbours,
Rust through `Client::form`/`Client::send_form` and the same hand-written services. They are
marked as such in the code and are being replaced as HEY grows JSON for them.

## Develop

```bash
make check      # Smithy validate/build, drift gates, Go and Rust lint/tests, conformance
```

`make check` is the gate; see [AGENTS.md](AGENTS.md) for the pipeline, the exact steps for
adding an operation, and the hard rules (never hand-write an API path; every operation needs
tests). [CONTRIBUTING.md](CONTRIBUTING.md) covers the workflow and releases.
