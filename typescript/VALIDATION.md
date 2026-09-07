# TypeScript implementation evidence

Feature: first non-Go HEY SDK, proposed `@37signals/hey`. Based on
`origin/main` **764f371a979d367213f29aedef2e6ce611e9f402**, branch
`feature/typescript-sdk`. These are local, hermetic checks, **not GitHub CI results**.
No live HEY token, package publication, repository settings change or registry
provisioning was used. This packet accompanies the implementation commit; human
review and eventual current-head CI are still required.

## Acceptance map

| Criterion | Evidence |
|---|---|
| Full modeled API, types, routes | `src/generated/coverage.json`: 130 operations; `tests/generator.test.ts` invokes every generated method against Fetch and checks OpenAPI-derived method/path/query/body. Byte-for-byte regeneration checks schemas, operations, guards and coverage. No Smithy, OpenAPI or Go client edits. |
| HEY runtime behavior | 183 tests across 6 files: generation, precision/null/optional values, safe retries, no mutation replay, refresh, errors/body caps, URL safety, redirects, Link envelopes/window boundaries, account selection/isolation, signed upload isolation, OAuth forms/PKCE, conformance fail-closed contract and version scripts. |
| Shared conformance | Real loopback HTTP through generated SDK methods: **184 passed / 184 applicable**. Exactly 3 named unmodeled Go calendar-update form fixtures excluded, 187 total; explicit checked identity inventory and applicability reasons. Unknown assertions/operations/configuration, empty tests and stale exclusions fail. |
| Node support | Node **22.12.0**, **24.20.0**, **26.7.0** each built, typechecked, ran all 183 tests and all 184 applicable conformance fixtures, and isolated package smoke. |
| Installable artifact | 23-file npm tarball installed offline outside the repository; ESM root and OAuth subpath imports, real SDK int64 request/response and consumer TypeScript positive/negative typechecks passed. Exact packed tarball smoke also passed. |
| Existing Go preserved | `make check`: Go vet/lint/tests and 187 Go conformance cases, Smithy/route/shape freshness plus Go wrapper drift, TypeScript checks/conformance and API-version sync. |
| Delivery security | actionlint + shellcheck and zizmor passed; `npm audit` found zero vulnerabilities. Credential-free `npm publish --dry-run --provenance` passed. OIDC/registry ownership remain unverified and publishing disabled by default. |

## Exact commands and results

Executed from the feature worktree (unless a command changes directory):

```sh
npm --prefix typescript ci
# PASS: frozen lockfile installation

npx --yes --package=node@22.12.0 -c 'node --version && make ts-check ts-smoke conformance-ts'
# PASS: v22.12.0; 183 tests, 130 generated operations, package smoke,
#       184/184 applicable conformance; 3 explicit not-applicable cases

npx --yes --package=node@24 -c 'node --version && make ts-check ts-smoke conformance-ts'
# PASS: v24.20.0; same checks and counts

make ts-check ts-smoke conformance-ts
# PASS on v26.7.0: same checks and counts

env -u GOROOT GOWORK=off \
  PATH="/home/rzolkos/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.7.linux-amd64/bin:$PATH" \
  make check
# PASS: MVP gate passed (shipped Go + TypeScript)

actionlint -shellcheck /home/rzolkos/.local/share/mise/installs/shellcheck/0.11.0/shellcheck-v0.11.0/shellcheck
# PASS: no diagnostics
zizmor --no-progress .github/workflows
# PASS: no findings (3 existing ignored, 8 suppressed); no new audit suppressions
npm --prefix typescript audit --audit-level=high
# PASS: zero vulnerabilities

git diff --check
# PASS

(cd typescript && npm run build && TARBALL=$(npm pack --silent) && \
  npm run smoke -- "$TARBALL" && \
  env -u NODE_AUTH_TOKEN -u NPM_TOKEN NPM_CONFIG_USERCONFIG=/dev/null \
    npm publish "$TARBALL" --access public --tag latest --dry-run --provenance)
# PASS: exact tarball isolated install/typecheck; dry-run only, 23 package files.
# npm warned authentication is absent, as expected; nothing published.
```

The local host inherits a stale `GOROOT` and an unrelated parent `go.work` listing
Basecamp modules, not this worktree. Plain `make check` initially failed the existing
Go toolchain-coherence check, then workspace selection; the command above aligns the
installed Go 1.26.7 toolchain and disables that **unrelated** workspace. No repository
check was loosened, and no parent workspace/main checkout was changed. The host's
shellcheck shim had no selected version; actionlint was given the already-installed
binary explicitly, without changing global tool configuration.

Development regressions were fixed before the passing runs: `UpdateMessage`'s explicit
`natural: false` must override inferred PUT idempotency; lossless integer parsing must
retain safe integers as numbers; Node timers can wake 1 ms early, so retry waits now
use a monotonic minimum deadline rather than weakening fixture timing assertions.
Initial sister-SDK development versions included a Vitest advisory; the frozen graph
uses patched Vitest 4.1.11 and tsx 4.23.13 and audits cleanly.

## Reviewer orientation and limits

1. `scripts/generate.ts` and `src/generated/coverage.json`: model-derived 130-operation
   surface, explicit retry override, int64 types and trait guards.
2. `src/client.ts`, `src/security.ts`, `src/oauth.ts` and the focused tests: sensitive
   transport boundaries, immutable mail-account context, credentials and pagination.
3. `../conformance/runner/typescript/`: actual requests/assertions, flattened fixture
   input adapters, draft aliases, checked inventory and exactly three exclusions.
4. `../TYPESCRIPT_RELEASE.md`, release workflows, Makefile and version scripts: safe
   activation, Go release compatibility, frozen dependencies and package artifact flow.

This is **not full Go-wrapper parity**: unmodeled calendar event updates are unavailable;
HTML/form convenience services, Go resilience hooks/circuit breakers, persistent auth
storage and streaming downloads are not claimed. Pagination is explicit page-envelope
iteration rather than Go's implicit aggregation. The package is Node ESM only.

No screenshots are applicable to this non-visual SDK/operational feature. Generated
TypeScript sources and coverage inventory are committed representative outputs; npm
release jobs retain tested tarballs once actually run. There are no data migrations,
backfills or live service changes. The feature originates from the user's implementation
request; there is no external issue/design link. Human npm ownership/bootstrap, protected
environment and trusted-publisher provisioning remain prerequisites to **activation**,
not assumptions satisfied by the dry-run. Read the release setup checklist before enabling
`HEY_TYPESCRIPT_PUBLISH_ENABLED`; unset means npm **NOT published** and Go-only orchestration.
