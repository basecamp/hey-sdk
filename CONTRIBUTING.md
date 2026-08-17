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

Releases are managed via `make release VERSION=x.y.z`.
