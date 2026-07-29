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
spec/hey.smithy -> openapi.json -> behavior-model.json -> Go generator -> go/pkg
```

`openapi.json`, `behavior-model.json` and the generated Go are all rebuilt from the
spec. Editing any of them by hand loses the change on the next generate, and
`go-check-drift` fails the build when they disagree with the spec.

## Adding an operation

1. Edit `spec/hey.smithy`
2. `make smithy-build` -- regenerates `openapi.json`
3. `make go-generate` -- regenerates the Go service layer. Note the name: this repo has
   no `go-generate-services` target, unlike the seed's vocabulary.
4. Add Go unit tests, and a conformance case under `conformance/tests/` for behavioral
   changes
5. `make check`

`make check` resolves to `check-mvp`: `smithy-check`, `behavior-model-check`,
`drift-check-mvp`, `url-routes-check`, `go-check`, `go-check-drift` and
`conformance-mvp`.
