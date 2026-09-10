# Contributing to HEY SDK

## Prerequisites

- Smithy CLI
- Go 1.26+
- Rust 1.88+ (with `rustfmt` and `clippy`; `rust-toolchain.toml` picks the exact stable for rustup users)
- [`cargo-deny`](https://github.com/EmbarkStudios/cargo-deny) (`cargo install cargo-deny --locked`), for `make rs-check`
- [`cargo-semver-checks`](https://github.com/obi1kenobi/cargo-semver-checks) only if you want the
  API-compatibility check locally (`cargo semver-checks -p hey-sdk --baseline-rev origin/main`
  from `rust/`); CI runs it on every pull request
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
4. Run `make rs-generate` to regenerate the Rust crate
5. Add unit tests
6. Add conformance tests if the operation has behavioral requirements, with dispatch arms in
   the Go and Rust runners
7. Run `make check`

The full step-by-step, including how the drift gates work, is in [AGENTS.md](AGENTS.md).

## Versioning

The Go module and the Rust crate share one version and one `vX.Y.Z` tag. Pre-1.0, a breaking
change bumps the minor version and an additive one the patch. For Rust, what counts as
breaking is decided once, by the type policy in
[rust/hey-sdk/README.md](rust/hey-sdk/README.md#versioning): request-side types are literal
and exhaustive, so a new field there is a minor; response-side types and open enums are
`#[non_exhaustive]`, so a new field or variant there is a patch. Say which in the PR when a
change touches a public type.

The crate's minimum supported Rust version is 1.88 (`rust-version` in `rust/Cargo.toml`). It
moves only when a dependency or a language feature the crate needs requires it, as a minor
release with a line in the release notes.

## Release Process

Two steps, in this order.

```bash
make bump VERSION=x.y.z     # rewrites go/pkg/hey/version.go, rust/hey-sdk/Cargo.toml and both Cargo.lock files
# commit that, open a PR, merge it
make release VERSION=x.y.z  # runs the gate, then tags vx.y.z and go/vx.y.z
```

The bump has to land on main *before* the tag, because the release workflows check that
`version.go` and `Cargo.toml` match the tag they were pushed for and refuse to publish
otherwise. `make release` checks the same thing up front, so a forgotten bump fails
locally in a second rather than on GitHub after the tags are already pushed; its gate
includes `cargo publish --dry-run`, so a crate that would not package fails there too.

The `vx.y.z` tag runs three workflows: `release-go.yml` tags the module, `release-rust.yml`
publishes the crate to crates.io, and `release-github.yml` waits for both and then creates
the GitHub release. Both git tags matter: the plain one triggers the release and is what a
git-dependency on the crate pins (there is no `rust/vx.y.z` tag; Cargo does not resolve tags
by path), and the `go/` one is what `go get github.com/basecamp/hey-sdk/go` resolves, since
the module lives in a subdirectory.

`Version` is not decorative — it goes out on every request as part of the User-Agent,
alongside `APIVersion`, which is how HEY sees which SDK and which contract a client is
working from.

### Publishing the crate

`release-rust.yml` publishes with [crates.io trusted
publishing](https://crates.io/docs/trusted-publishing): the `publish` job trades its GitHub
OIDC identity for a short-lived token through
[`rust-lang/crates-io-auth-action`](https://github.com/rust-lang/crates-io-auth-action), runs
`cargo publish`, and the token is revoked when the job ends. There is no long-lived token in
the repository's secrets. The job runs only for a pushed `v*` tag, inside the `release-crates`
environment, which is restricted to `v*` tags (no required reviewers: `release-github.yml`
polls the run for thirty minutes, and the tag is already a deliberate `make release` from a
green gate); a `workflow_dispatch` run is a rehearsal that packages and dry-runs but never
publishes and never exercises the token exchange.

The job is safe to re-run. It publishes only when crates.io answers 404 for the version; a 200
means the version is there (an earlier run that failed after the upload, say) and the job
succeeds without publishing again, provided the checksum crates.io holds is the one this
commit packages to; a version published from anything else, or yanked, fails; any other
answer fails without trying. After a partial release, re-run the failed run
(`gh run rerun <run-id> --failed`, or the Re-run button); do not delete or move the tag.

Before publishing it packages the crate a second time and refuses unless the bytes match what
the `package` job built and verified from the same commit, and afterwards it compares the
checksum crates.io holds with what it packaged. `.cargo_vcs_info.json` inside the crate names
the commit and `rust/hey-sdk` as the path, which is the crate's provenance on crates.io.

#### The first publish, once

Trusted publishing is configured on an existing crate's settings page, so the first version
of `hey-sdk` on crates.io is published by hand, by a crates.io user who will own the crate,
and everything after it by the workflow. Until then `https://crates.io/api/v1/crates/hey-sdk`
answers 404 and the `publish` job fails pointing here, which holds the GitHub release for
that tag; finish these steps and re-run the failed run.

1. Create the `release-crates` environment on the repository (Settings → Environments) with
   deployment branches and tags restricted to the tag pattern `v*`, and no required
   reviewers. Do this before the first tag: a job that references an environment that does
   not exist creates one with no protection.
2. On a clean checkout of `main` at the commit about to be tagged, with `make check` green:
   mint a crates.io API token scoped to `publish-new` and `change-owners`, crate-name
   pattern `hey-sdk`, with a one-day expiry.
3. `cd rust && cargo publish -p hey-sdk --locked` with that token (`CARGO_REGISTRY_TOKEN`).
4. `cargo owner --add github:basecamp:cli hey-sdk`, so ownership is the GitHub team and not
   the person.
5. On the crate's settings page on crates.io, add a trusted publisher: repository owner
   `basecamp`, repository `hey-sdk`, workflow filename `release-rust.yml` (the exact
   basename; renaming the file breaks the exchange), environment `release-crates`.
6. Revoke the token.
7. `make release VERSION=x.y.z` from that same commit. The tag's `release-rust.yml` run finds
   the version already on crates.io and succeeds; the next tag is the first real exchange.
