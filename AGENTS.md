# HEY SDK -- Agent Instructions

Go client for the HEY API, generated from the Smithy spec in `spec/`.

**This repo ships Go only.** The Makefile still carries `ts-`, `rb-`, `swift-` and `kt-`
targets inherited from the shared SDK seed. They `cd` into `typescript/`, `ruby/`,
`swift/` and `kotlin/`, none of which exist here, so they fail immediately -- as does
`make check-full`, which invokes them. `make check` is the gate that works.

## Hard rules

1. **Never hand-write API methods.** Operations are generated from the Smithy spec.
2. **Never construct URL paths manually.** Use the generated route table -- no
   `fmt.Sprintf` for paths.
3. **Every new operation needs tests.** Go unit tests, plus a conformance test when the
   change is behavioral.
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
6. Add Go unit tests, and a conformance case under `conformance/tests/` for behavioral
   changes
7. `make check`

`make check` resolves to `check-mvp`: `smithy-check`, `behavior-model-check`,
`drift-check-mvp`, `url-routes-check`, `go-check`, `go-check-drift` and
`conformance-mvp`. `drift-check-mvp` is coverage freshness + forward (every modelled
route exists in `spec/route-snapshot.json`) + reverse (every JSON-capable snapshot
route is modelled or listed in `spec/excluded-routes.json` with a reason) + shape
fingerprint. Adding an operation for a route that haystack does not serve, or
forgetting to regenerate coverage, fails the gate.

The snapshot comes from a haystack checkout: `make drift-regen HAYSTACK_DIR=…` then
`./scripts/sync-provenance HAYSTACK_DIR` to record the pinned SHA in
`spec/api-provenance.json`.
