# HEY SDK

The Go SDK for the [HEY](https://www.hey.com) API. It is the library behind
[hey-cli](https://github.com/basecamp/hey-cli), and it is generated from a Smithy model of
the API in `spec/`, so what the SDK offers is what HEY actually serves.

Today the repository ships a single Go module, `github.com/basecamp/hey-sdk/go`.

TypeScript, Ruby, Swift and Kotlin SDKs will be added in future updates, generated from the
same Smithy model; the Makefile already reserves targets for them (`ts-`, `rb-`, `swift-`,
`kt-`), which fail until those SDKs exist.

## Install

```bash
go get github.com/basecamp/hey-sdk/go@latest
```

Requires Go 1.26 or newer.

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
// A reply's acting sender comes from the NewReply prefill (0 = the account default).
_ = client.Messages().Create(ctx, "Subject", "Body", []string{"someone@example.com"}, nil, nil)
prefill, _ := client.Entries().NewReply(ctx, entryID)
_ = client.Entries().CreateReply(ctx, entryID, prefill.Sender.Id, prefill.Subject, "Reply body", []string{"someone@example.com"}, nil, nil)

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
can refuse an operation before it runs. Retries, circuit breaking, bulkheads and rate limits
are configured with `WithResilience`, `WithCircuitBreaker`, `WithBulkhead` and
`WithRateLimit`; HTTP caching with `WithCache`. Response caching is active for requests with
an `Authorization` header, which gives each authenticated identity a stable cache partition.

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

## How the SDK is built

```
spec/hey.smithy ──► openapi.json ──► oapi-codegen ──► go/pkg/generated/client.gen.go
                                                                │
                              hand-written services in go/pkg/hey call into it
```

The Smithy model is the source of truth for routes and payloads. `openapi.json`,
`behavior-model.json`, `client.gen.go`, `go/pkg/hey/url-routes.json` and the files under
`spec/` that describe coverage are all regenerated from it — editing them by hand is lost on
the next build. The services in `go/pkg/hey` are written by hand and add the things a
generated client cannot know: which recipients a reply needs, that HEY answers a shared
topic's trash request with a confirmation page, that starting a time track takes no body.

`make check` verifies the model against a snapshot of HEY's own routes
(`spec/route-snapshot.json`, pinned in `spec/api-provenance.json`): every modelled route
must exist in HEY, and every JSON-capable HEY route must be either modelled or listed in
`spec/excluded-routes.json` with a reason. Generated ops that HEY does not serve cannot get
in unnoticed.

A handful of services (`Clips`, `Snippets`, `Workflows`, `Publications`, `World`,
`Extenzions`, `CalendarEvents`, and parts of `Contacts` and `Search`) still talk to HEY the
way the web UI does — form posts, and for a few reads, the HTML page — because those
endpoints have no JSON yet. They are marked as such in the code and are being replaced as
HEY grows JSON for them.

## Develop

```bash
make check      # Smithy validate/build, drift gates, Go vet/lint/tests, conformance
```

`make check` is the gate; see [AGENTS.md](AGENTS.md) for the pipeline, the exact steps for
adding an operation, and the hard rules (never hand-write an API path; every operation needs
tests). [CONTRIBUTING.md](CONTRIBUTING.md) covers the workflow and releases.
