# TypeScript SDK validation

This document maps the TypeScript SDK's acceptance boundary to repeatable repository checks.
GitHub Actions remains the authority for the exact pull-request head and supported Node matrix.

## Acceptance map

| Capability | Evidence |
|---|---|
| Generated API coverage | `npm run generate:check` verifies 131 modeled operations, generated routes, schemas, guards and coverage metadata against `openapi.json` and `behavior-model.json`. |
| Runtime behavior | `npm test` runs 518 tests across transport, retries, cancellation, credentials, OAuth, pagination, account scope, response limits, package installation, generation and release integration. |
| Type safety | `npm run typecheck`, `npm run typecheck:tests` and the conformance-runner typecheck cover the public SDK, tests, examples and fixture adapter. |
| Package artifact | `npm run package:smoke` packs the package, installs the tarball in an isolated project and validates its exports without resolving repository source files. |
| Shared conformance | `make conformance-ts` executes 191 applicable fixtures through generated methods. Four named convenience-layer fixtures remain explicitly excluded in `conformance/runner/typescript/not-applicable.json`; stale exclusions and unknown fixture shapes fail closed. |
| Existing SDKs | `make check` runs the complete Smithy, drift, Go, Rust, TypeScript and conformance gate. Go and Rust each execute all 195 fixtures. |
| Release safety | Release tests validate inactive-by-default npm publication, tagged-state parsing, exact-artifact publication and transactional version synchronization across Go, Rust and TypeScript. |

## Current local result

The final review-fix working tree passed:

```text
node --version   # v26.7.0
npm --version    # 11.19.0
go version       # go1.26.7 linux/amd64
rustc --version  # rustc 1.98.1

make check
# PASS
# TypeScript tests: 518/518
# Go conformance: 195/195
# Rust conformance: 195/195
# TypeScript conformance: 191/191 applicable; 4 declared exclusions
```

Focused TypeScript verification is available with:

```bash
make ts-check ts-smoke conformance-ts
```

CI additionally runs generation, build, tests, typechecks, version synchronization and
package smoke on Node 22.12, 24 and 26.

## Review corrections

The regression suite covers the review findings that changed runtime or delivery behavior:

| Boundary | Guaranteed behavior |
|---|---|
| Retry timing | A retry waits for the greater of the modeled backoff and a valid `Retry-After` value. Safe transport failures retry within the modeled attempt budget; mutations are not replayed unless the model permits it. |
| Cancellation | Aborts during requests and backoff preserve the caller's abort reason through the SDK error boundary. |
| Credential refresh | Concurrent refresh is single-flight, stale generations cannot overwrite newer credentials, and provider failures surface as `auth_required` errors. |
| OAuth tokens | Successful grants require a public RFC 6750 Bearer token value; missing, malformed and unsupported token schemes are rejected. |
| Pagination URLs | Continuations stay on the configured origin. Malformed endpoints and pagination links surface as usage errors rather than native URL exceptions. |
| Recording variants | Generated TypeScript guards and Rust recording helpers recognize both direct and namespaced discriminator spellings declared by the shared model. |
| Nullable categories | TypeScript and Rust preserve nullable recording categories, while generated Go exposes `Recording.Category` as `*string`. |
| Release state | Tagging requires a valid TypeScript publication-state result. Parser failures and malformed state cannot silently fall back to an inactive release. |

## Delivery boundary

Merging the SDK publishes no npm package. `.github/typescript-publish-enabled` remains
`false`; Go and Rust releases continue while the TypeScript workflow builds, tests and
packs without requesting npm credentials. `TYPESCRIPT_RELEASE.md` defines the separately
reviewed provisioning and activation procedure.

The TypeScript runtime supports Node ESM on Node 22.12+, 24 and 26. Browser, CommonJS,
Deno, Bun, React Native, persistent credential storage, streaming downloads and handwritten
Go/Rust convenience parity remain outside this release.
