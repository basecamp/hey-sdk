# Contributing to HEY SDK

## Prerequisites

- Smithy CLI
- Go 1.26+
- Make
- jq

## Development Workflow

1. Fork and clone the repository
2. Create a feature branch: `git checkout -b my-feature`
3. Make changes following the patterns in AGENTS.md
4. Run checks: `make check`
5. Commit and push
6. Open a pull request

## Adding a New API Operation

1. Add the operation to the Smithy spec in `spec/`
2. Run `make smithy-build` to regenerate OpenAPI, then refresh the derived files
   (`make url-routes`, `./scripts/generate-shape-fingerprint`, `./scripts/generate-route-coverage`)
3. Run `make go-generate` to regenerate the Go client, then add or update the hand-written
   service in `go/pkg/hey`
4. Add unit tests
5. Add conformance tests if the operation has behavioral requirements
6. Run `make check`

The full step-by-step, including how the drift gates work, is in [AGENTS.md](AGENTS.md).

## Release Process

Two steps, in this order.

```bash
make bump VERSION=x.y.z     # rewrites go/pkg/hey/version.go
# commit that, open a PR, merge it
make release VERSION=x.y.z  # runs the gate, then tags vx.y.z and go/vx.y.z
```

The bump has to land on main *before* the tag, because the release workflow checks that
`version.go` matches the tag it was pushed for and refuses to publish otherwise.
`make release` checks the same thing up front, so a forgotten bump fails locally in a
second rather than on GitHub after the tags are already pushed.

Both tags matter: the plain one triggers the release, and the `go/` one is what
`go get github.com/basecamp/hey-sdk/go` resolves, since the module lives in a
subdirectory.

`Version` is not decorative — it goes out on every request as part of the User-Agent,
alongside `APIVersion`, which is how HEY sees which SDK and which contract a client is
working from.
