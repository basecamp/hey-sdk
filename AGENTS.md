# HEY SDK -- Agent Instructions

Go and Rust clients for the HEY API, generated from the Smithy spec in `spec/`.

**This repo ships Go and Rust.** The Makefile still carries `ts-`, `rb-`, `swift-` and `kt-`
targets inherited from the shared SDK seed. They `cd` into `typescript/`, `ruby/`,
`swift/` and `kotlin/`, none of which exist here, so they fail immediately -- as does
`make check-full`, which invokes them. `make check` is the gate that works.

## Hard rules

1. **Never hand-write API methods.** Operations are generated from the Smithy spec.
2. **Never construct URL paths manually.** Use the generated route table -- no
   `fmt.Sprintf` or `format!` for paths.
3. **Every new operation needs tests.** Go and Rust unit tests, plus a conformance test
   when the change is behavioral.
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
`services/*.rs` (one struct per service, one async method per operation). The hand-written
core in `rust/hey-sdk/src` (client, retries, cache, auth, pagination, account scope) knows
nothing about individual operations; everything operation-specific comes from the model.

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

`rs-check-drift` runs the generator in `--check` mode, so stale generated code fails the
gate.

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
`Workflows::stage_topic` use it so the hooks see one operation, as Go's do.

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
7. Add Go unit tests, Rust tests where the change touches hand-written Rust, and a
   conformance case under `conformance/tests/` for behavioral changes. A conformance case
   also needs a dispatch arm in both `conformance/runner/go/main.go` and
   `conformance/runner/rust/src/operations.rs`.
8. `make check`

`make check` resolves to `check-mvp`: `smithy-check`, `behavior-model-check`,
`drift-check-mvp`, `url-routes-check`, `go-check`, `go-check-drift`, `rs-check`,
`rs-check-drift` and `conformance-mvp` (the Go and Rust runners). `drift-check-mvp` is coverage freshness + forward (every modelled
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
`rust/hey-sdk/src/generated/mod.rs`, so `make rs-generate` is what moves it.
