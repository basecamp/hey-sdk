# HEY SDK -- Agent Instructions

Go, Rust, TypeScript and Kotlin clients for the HEY API, generated from the Smithy spec in `spec/`.

**Shipped SDKs: Go, Rust, TypeScript and Kotlin.** Run `make ts-install` once (`npm ci`),
have a JDK 17 on hand (`.mise.toml` pins one), then `make check` for all four languages.
Ruby/Swift Makefile targets remain inherited placeholders; do not enable `make check-full`,
which invokes missing SDKs. TypeScript
operations, types, routes and metadata are generated; do not hand-edit
`typescript/src/generated`. See `typescript/README.md` and `TYPESCRIPT_RELEASE.md`.

## Hard rules

1. **Never hand-write API methods.** Operations are generated from the Smithy spec.
2. **Never construct URL paths manually.** Use the generated route table -- no
   `fmt.Sprintf` or `format!` for paths.
3. **Every new operation needs tests.** Go, Rust, TypeScript and Kotlin unit/operation-coverage
   tests, plus a conformance test when the change is behavioral.
4. **Run `make check` before committing.**

## Pipeline

```
spec/hey.smithy -> openapi.json -> oapi-codegen -> go/pkg/generated/client.gen.go
                                                            |
                                    hand-written wrappers in go/pkg/hey call into it
```

`openapi.json` and `go/pkg/generated/client.gen.go` are rebuilt from the spec; editing
either by hand loses the change on the next generate. `go/pkg/hey` is **not** generated
— those service wrappers are hand-written and you update them yourself.

oapi-codegen sees the OpenAPI document alone, so `go generate` first runs
`scripts/merge-behavior-model`, which decides each operation's idempotency — the
`x-hey-idempotent` override first, otherwise `readonly || idempotent` from
`behavior-model.json`, otherwise the verb, as the other generators decide it — and
writes it onto the operation as `x-go-idempotent` in `go/openapi.behavior.json`, a
build intermediate the template reads and git ignores.

`go-check-drift` does not re-derive anything from the spec. It extracts the operations
present in the checked-in `client.gen.go` and compares them with the `.gen.*WithResponse`
calls in `go/pkg/hey`, failing when a wrapper calls an operation that no longer exists.
So it catches wrappers left behind by a regenerate, not spec-vs-OpenAPI drift.

### Rust

```
openapi.json + behavior-model.json -> rust/generator -> rust/hey-sdk/src/generated/
                                                              |
                                  hand-written conveniences in rust/hey-sdk/src/services
```

`rust/generator` is a small Rust binary that reads `openapi.json` and `behavior-model.json`
and writes `types.rs` (every schema), `routes.rs` (one `Route` static per operation, with
idempotency, empty-on statuses, pagination style and the retry policy) and
`services/*.rs` (one struct per service, one async method per operation). A route's
`idempotent` — what lets the retry loop resend it — is the spec's explicit
`x-hey-idempotent.natural` when there is one, otherwise the behavior model's own `readonly`
or `idempotent` (which is how a PATCH earns a resend), and only when the model says neither
the verb (GET, HEAD, PUT, DELETE); the Kotlin and TypeScript generators read it the same
way. The hand-written
core in `rust/hey-sdk/src` (client, retries, cache, auth, pagination, account scope) knows
nothing about individual operations; everything operation-specific comes from the model. The
retry loop reads each route's policy — `max` is Smithy's `maxAttempts`, the sends in all —
and the client's own settings only lower the count and, but for `max_delay`, lengthen the
waits (`max_delay` caps the policy's wait like any other, which is how a test suite winds the
backoff down), so changing a policy in the model and regenerating is what changes what goes
over the wire.

Services are named from tags and methods from operation ids with the service's noun
removed (`ListBoxes` -> `boxes().list()`). The generator fails when two operations
collapse to one name or a name is a Rust keyword; `rust/generator/names.toml` is where
those are settled, and where an operation is moved to a different service than its tag —
every SDK files an operation under the same service, so those overrides follow Go's own
placement. The generator also names the record each operation acts on: the id in the
path's last segment, or the outermost parent's when the path ends in a collection, which
is what Go writes by hand.

`names.toml` also carries a `[type_names]` table for a schema whose Smithy name collides
with something Rust already has — `Box` is emitted as `Mailbox`. The rename reaches every
place the generator writes the type (the struct, the aliases built from it, the service
signatures) and nowhere else: a type name never goes on the wire, so Go and the fixtures
are untouched.

An operation whose 2xx body is `text/html` rather than JSON — a workflow stage — becomes a
method that asks for the page as HEY serves it (no `.json` suffix, `Accept: text/html`) and
answers the body as a `String`. Reading anything out of that page is a hand-written
convenience's job, as `go/pkg/hey` does for the same route.

A 2xx in any other representation — an image, a CSV, two representations on one status, a
schema that is not a `$ref` — fails generation naming the operation. A method the crate
cannot call is worse than no method, and one that quietly answered `()` for a page is how
the gate went red the first time an HTML route arrived.

`rs-check-drift` runs the generator in `--check` mode, so stale generated code fails the
gate. The generator also has fixtures of its own (`rust/generator/src/fixtures.rs`): small
models put through `Model::build` and the emitters whole, since the drift check only proves
the checked-in output matches the generator, not that the generator is right.

`rs-check` is every step CI's `test-rust` job runs before it, in order — fmt, clippy,
tests and docs with all features, clippy and tests again with `--no-default-features`, the
examples, `cargo deny` over both workspaces, and `cargo package` — so a green
`make rs-check rs-check-drift` locally is a green job. CI also builds the library on `rust-version` (`msrv-rust`), the docs on nightly as
docs.rs would (`docs-rust`), and the public API against the pull request's base
(`api-compat-rust`, `cargo semver-checks`; the `breaking` label skips it). The toolchain
everything else runs on is pinned in `rust-toolchain.toml` at the repository root.

`rust/hey-sdk/examples/*.rs` are the crate README's snippets as whole programs; they compile in
the gate, so a change to the public API that breaks one shows up there.

#### Lints

`rust/hey-sdk/Cargo.toml` and `rust/generator/Cargo.toml` carry the fleet's `[lints]` table:
`unsafe_code` forbidden, `missing_docs` and `unreachable_pub` on, clippy `all` and `pedantic`
on with a four-entry allowlist that each carry a reason, and CI turns warnings into errors.
`unwrap`/`expect` are denied in library code outside tests (`lib.rs`); a lock is read with
`unwrap_or_else(PoisonError::into_inner)`, and the rare provably-infallible call carries a
targeted `#[allow]` with its reason on the line. Generated code is not lint-gated: every
generated file starts with `#![allow(missing_docs, unreachable_pub, clippy::all,
clippy::pedantic)]`, because the model chooses its names and shapes. Hand-written code gets
no such allowance.

#### How the generated surface evolves

Which side of the wire a type sits on decides how it may change; the policy and what it
costs a release are in [rust/hey-sdk/README.md](rust/hey-sdk/README.md#versioning). The
generator applies it by reachability. A struct schema reachable from any request body — the
body type itself or anything it mentions, however deep — is request-side: public fields,
`Default`, literal construction, never `#[non_exhaustive]`. Every other struct schema is
response-side and is emitted `#[non_exhaustive]`. The `*Params` structs are request-side.
An enum is `#[non_exhaustive]` on either side; its declared variants stay constructible, so
a request that carries one is still a caller's to build. `Route`, `RouteParam` and `Retry`
are request-side too: a caller may write a route for a path the model lacks and hand it to
`Client::operation`, which is what the retry-policy tests do, so a field added to them —
`html` was one — is a `0.MINOR`.

When the model declares an enum, the generator emits a `#[non_exhaustive]` enum with one
variant per declared value and `Unknown(String)` for any value HEY sends that the model did
not declare, carried unchanged so a read-modify-write sends back what it read —
`#[serde(other)]` drops the value and cannot. Until the model declares one, the generator
invents none: a `kind` stays a `String`.

`rust/hey-sdk/src/services/*.rs` are hand-written, like `go/pkg/hey` — you update them
yourself. Each one re-exports the generated service it extends and adds conveniences as
extra `impl` blocks on it: `attachments`, `boxes`, `bulk_replies`, `calendar_changes`,
`calendar_events`, `calendar_periods`, `calendar_todos`, `calendars`, `clearances`,
`clips`, `collections`, `contacts`, `designations`, `entries`, `extenzions`, `habits`,
`identity`, `journal`, `messages`, `postings`, `publications`, `search`, `snippets`,
`stickies`, `time_tracks`, `topics`, `workflows`, `world` (`world` has no generated
counterpart and declares its own struct and `Client::world`).

The naming convention is in `services/mod.rs`: a hand-written method keeps its plain name
where the generated service leaves it free, and takes the name the model gives the
operation it sends where the generated method already holds it — `mark_postings_seen(&[a])`
alongside the generated `mark_seen(&body)`, `create_box_designation(box_id, contact_id)`
alongside `create(box_id, &body)`. A convenience that changes the shape of the call rather
than only its arguments — a different result, or a refusal the generated method leaves to
HEY — may take a descriptive name instead: `TimeTracks::start_tracking`,
`Calendars::toggle_selection`, `Messages::send`, `Identity::set_first_week_day`.

The parts of HEY with no JSON surface go through `Client::form`, which builds the browser
form request (path as written, browser `Accept`, redirect captured rather than followed,
retried only after a refreshed 401), and `Client::send_form`. Say what such a call means with
`Operation::info` and `services::write_info`, since the model describes none of those
paths. `Operation::quiet` skips the operation hooks while still firing the request ones,
for a read-back made inside another operation — `Publications::publish` and
`Workflows::stage_topic` use it so the hooks see one operation, as Go's do. A convenience
whose answer is not what HEY answered — the changes feeds turning a 409 into
`full_sync_required`, `TimeTracks::start_tracking` and the contact writes rewording a
refusal — sends quietly inside `Client::as_operation`, which runs the send and the
conversion as one operation under the operation's own info: the gate, the span, the
start, and an end that carries what the caller gets rather than what HEY answered. A
catch after `execute` returns is too late for the hooks, which have already heard the
operation fail.

### Kotlin

```
openapi.json + behavior-model.json -> kotlin/generator -> kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/generated/
                                                                    |
                          hand-written subclasses in kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/services
```

The Kotlin library follows the conventions of the company-wide
[basecamp-sdk](https://github.com/basecamp/basecamp-sdk) Kotlin SDK rather than the Rust
crate's, so a reader of one is a reader of the other: a Kotlin Multiplatform build with a JVM
target (`commonMain`/`jvmMain`, `expect`/`actual` for the platform bits in `Platform.kt`),
`HeyConfig` carrying `VERSION` and `API_VERSION`, a builder DSL (`HeyClient { accessToken(..) }`)
with `enableCache`/`enableRetry`/`maxRetries`/`maxPages`/`timeout`/`hooks`, generated models
under `generated/models` and services under `generated/services`, `<Operation>Options` data
classes for optional query parameters, `ServiceAccessors.kt` extension properties on the
client (`client.boxes`, imported from `com.basecamp.hey.generated`), a `BaseService` every
generated service extends, hand-written subclasses of the generated services for the
conveniences (`com.basecamp.hey.services.MessagesService` extends
`com.basecamp.hey.generated.services.MessagesService`), a sealed `HeyException` with string
`code`s and `exitCode`, `HeyHooks` with `onRetry(info, attempt, error, delayMs)`, and
`consoleHooks()`/`chainHooks()`.

Two deliberate departures from basecamp-sdk's builder: there is no `httpClient` option, only
`engine`, since a plugin on a caller's client (a retry, a default request, redirect following,
response validation) would run ahead of the retry policy, the credential handling on
redirects, the error mapping and the timeout the client is responsible for; and the operation
the hooks hear runs until the answer is decoded or parsed, so an answer that will not read
ends the operation with the error the caller gets rather than as a success.

`kotlin/generator` is a small Kotlin program, run through the Gradle build under `kotlin/`,
that reads `openapi.json`, `behavior-model.json` and `kotlin/generator/names.toml` and
writes `models/<Schema>.kt` (one `@Serializable` data class or typealias per schema),
`Routes.kt` (one `Route` per operation, with idempotency, empty-on statuses, pagination style
and the retry policy), `services/<Service>Service.kt` (one class per service, one suspend
method per operation) and `ServiceAccessors.kt`. The hand-written core in
`kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey` (client, retries, cache, auth,
pagination, account scope) knows nothing about individual operations; everything
operation-specific comes from the model, and the retry loop reads each route's policy as the
Rust one does.

`names.toml` is the Kotlin twin of `rust/generator/names.toml`: `[services]` (tag to
service), `[operation_services]` (an operation filed under another service, following Go's
placement), `[operation_methods]` (a method name the derivation gets wrong), `[type_names]`
(a schema renamed away from a Kotlin collision; none today), `[resource_types]` and
`[operation_resource_types]` (the noun the hooks report), and `[hand_written_services]` (the
subclass the client's accessor constructs, which makes the generated class `open`). Method
names are camelCase of the Rust ones: `ListBoxes` is `client.boxes.list()`,
`GetBoxPostingChanges` is `client.postings.getBoxChanges(..)`. Service classes take a
`Service` suffix (`BoxesService`), as basecamp-sdk's do, and are reached as camelCase
properties of the client (`client.timeTracks`).

Models follow basecamp-sdk's strictness: a required member has no default, so a body that
leaves one out fails to decode as a non-retryable `api_error` rather than reading as a
fabricated zero (Go and Rust read the zero); an optional member is nullable and null. That
makes Kotlin the strictest reader of a conformance mock body, which is why several fixtures
that Go and Rust accepted had to be brought to the model's shape. Request bodies are the
model's own `@Serializable` types (basecamp-sdk flattens its bodies into generated `Body`
classes, which HEY's nested payloads do not suit). `x-hey-sensitive` strings are
`SensitiveString`s that print as `[REDACTED]`; dates and timestamps stay strings. An
operation whose 2xx body is `text/html` becomes a method that asks for the page as HEY
serves it and answers a `String`; any other representation, two on one status, or a schema
that is not a `$ref` fails generation naming the operation. A paginated read answers a `Page`
with the cursor and `X-Total-Count`, walked with `nextPage`/`eachPage`/`pages`, rather than
basecamp-sdk's auto-aggregated `ListResult`: HEY's paginated responses are objects with
geared cursors, so there is nothing generic to aggregate.

`kt-check-drift` runs the generator in `--check` mode, so stale generated code fails the
gate. `kt-check` is what CI's `test-kotlin` job runs: the library's build and tests with
every warning an error, the generator's own tests (naming and a small model put through
`Model.build` and the emitters) and the conformance runner's. `kt-consumer-check` publishes
the library to a scratch repository and compiles a consumer of it with the oldest Kotlin
`kotlin/README.md` promises (the script reads the number from the README), since the
library's and Ktor's metadata set that floor and a Kotlin bump moves it. `HeyConfig.API_VERSION`
is kept in step with `openapi.json` by `scripts/sync-api-version.sh`, like Go's `APIVersion`.

The hand-written services in `kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/services`
are the Kotlin twins of `rust/hey-sdk/src/services/*.rs`, one per service the Rust crate
writes conveniences for (26 subclasses of the generated services, registered in
`kotlin/generator/names.toml` under `[hand_written_services]`, plus `WorldService` and its
`HeyClient.world` accessor, which has no generated counterpart), ported method for method
with the same wire fields. A convenience Rust or Go gains is a convenience Kotlin gains too.
The naming convention is Rust's: a hand-written method keeps its plain name where the generated service leaves it
free, and takes the name the model gives the operation it sends where the generated method
already holds it -- `markPostingsSeen(ids)` alongside the generated `markSeen(body)`. A
form-backed write goes through `HeyClient.form`/`sendForm` with `writeInfo` saying what it
means; a page HEY serves as HTML is read by `WorkflowStageView.parse`, with the rules Go and
Rust read it by.

## Adding an operation

1. Edit `spec/hey.smithy`
2. `make smithy-build` -- regenerates `openapi.json`
3. Refresh the three artifacts `smithy-build` leaves behind:

   ```bash
   make url-routes                      # go/pkg/hey/url-routes.json
   ./scripts/generate-shape-fingerprint # spec/shape-fingerprint.json
   ./scripts/generate-route-coverage    # spec/route-coverage.json
   ```

   Only the first has a make target; the other two are standalone scripts. All three
   are verified by `make check` (`url-routes-check`, `drift-check-shape` and
   `drift-check-coverage`), so forgetting any of them fails the build loudly.
4. `make go-generate` -- regenerates `go/pkg/generated/client.gen.go` via oapi-codegen.
   Note the name: this repo has no `go-generate-services` target, unlike the seed's
   vocabulary, and this step does not touch `go/pkg/hey`.
5. Add or update the hand-written wrapper in `go/pkg/hey` so the operation is reachable
6. `make rs-generate` -- regenerates `rust/hey-sdk/src/generated`. If the generator
   refuses a method name, add an override to `rust/generator/names.toml`.
7. Run `make ts-generate` to refresh all TypeScript generated artifacts.
8. `make kt-generate` -- regenerates the Kotlin generated tree. If the generator refuses a
   method name, add an override to `kotlin/generator/names.toml`.
9. Add Go and TypeScript unit tests, Rust and Kotlin tests where the change touches
   hand-written code in those SDKs, and a conformance case under `conformance/tests/` for
   behavioral changes. A conformance case also needs a dispatch arm in
   `conformance/runner/go/main.go`, `conformance/runner/rust/src/operations.rs` and
   `conformance/runner/kotlin/src/main/kotlin/com/basecamp/hey/conformance/Operations.kt`.
   Kotlin is the strictest reader of a mock body -- a required member left out, or an
   object-typed response mocked as an array, fails there and nowhere else -- so a mock body
   has to be the shape the model says.
10. `make check`

`make check` resolves to `check-mvp`: `smithy-check`, `behavior-model-check`,
`drift-check-mvp`, `url-routes-check`, `go-check`, `go-check-drift`, `rs-check`,
`rs-check-drift`, `ts-check`, `kt-check`, `kt-check-drift`, `sync-api-version-check` and
`conformance-mvp` (Go, Rust, TypeScript and Kotlin). `drift-check-mvp` is coverage freshness + forward (every modelled
route exists in `spec/route-snapshot.json`) + reverse (every JSON-capable snapshot
route is modelled or listed in `spec/excluded-routes.json` with a reason) + shape
fingerprint. Adding an operation for a route that haystack does not serve, or
forgetting to regenerate coverage, fails the gate.

The snapshot comes from a haystack checkout: `make drift-regen HAYSTACK_DIR=…` then
`./scripts/sync-provenance HAYSTACK_DIR` to record the pinned SHA in
`spec/api-provenance.json`.

Move the service's `version` in `spec/hey.smithy` to that same date when the snapshot
moves, and run `./scripts/sync-api-version.sh` so `APIVersion` follows. It is the date of
the API the SDK was built against, and it goes out in the User-Agent alongside the SDK's
own version — that string is how HEY sees which contract a client is working from, so a
stale date misreports it. The Rust crate's `API_VERSION` is generated into
`rust/hey-sdk/src/generated/mod.rs`, so `make rs-generate` is what moves it. The Kotlin
library's `HeyConfig.API_VERSION` moves with Go's, through `./scripts/sync-api-version.sh`.
