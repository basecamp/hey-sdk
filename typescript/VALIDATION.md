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
| HEY runtime behavior and artifact checks | 203 tests across 7 files: generation, precision/null/optional values, safe retries, no mutation replay, refresh, errors/body caps, URL safety, redirects, Link envelopes/window boundaries, account selection/isolation, signed upload isolation, OAuth forms/PKCE, conformance fail-closed contract and version scripts. |
| Shared conformance | Real loopback HTTP through generated SDK methods: **184 passed / 184 applicable**. Exactly 3 named unmodeled Go calendar-update form fixtures excluded, 187 total; explicit checked identity inventory and applicability reasons. Unknown assertions/operations/configuration, empty tests and stale exclusions fail. |
| Node support | Node **22.12.0**, **24.20.0**, **26.7.0** each built, typechecked, ran all 203 tests and all 184 applicable conformance fixtures, and isolated package smoke. |
| Installable artifact | 23-file npm tarball installed offline outside the repository; ESM root and OAuth subpath imports, real SDK int64 request/response and consumer TypeScript positive/negative typechecks passed. Exact packed tarball smoke also passed. |
| Existing Go preserved | `make check`: Go vet/lint/tests and 187 Go conformance cases, Smithy/route/shape freshness plus Go wrapper drift, TypeScript checks/conformance and API-version sync. |
| Delivery security | actionlint + shellcheck and zizmor passed; `npm audit` found zero vulnerabilities. Credential-free `npm publish --dry-run --provenance` passed. OIDC/registry ownership remain unverified and publishing disabled by default. |

## Review corrections and regression evidence

All three recovered independent review findings are addressed, with no scope or Go API
changes. The original prose review artifacts were empty, **not clean reviews**; the
supervisor recovered their `needs_fixes` findings from native structured outputs.

| Finding | Correction | Regression evidence in `tests/client.test.ts` |
|---|---|---|
| P1: timeout/caller cancellation hung on `getToken()` or shared `refresh()` | Per-request abort-aware credential waits remove their abort listener on settlement; the underlying provider/shared refresh is not cancelled. Cancellation is rechecked before Fetch and renewal. | Four hanging-provider tests cover timeout and caller abort for both methods; shared-refresh test proves cancelling one waiter leaves the other successful with one rotation and no cancelled replay; token-acquisition/cancellation race test proves no dispatch. |
| P2: delayed old-token read was stamped with the post-refresh generation | Capture generation before awaiting the provider read. | Deferred old read resolves after another request has completed renewal; old-token 401 resends with the new token and refresh remains exactly once. |
| P2: transport User-Agent test pinned an API date outside version synchronization | Assert the complete literal SDK identifier plus imported `VERSION` and `API_VERSION`, not a fixed date. | Updated authorization/Accept/User-Agent test and unchanged `tests/version-scripts.test.ts` both pass. |

### Failing before / passing after

With all seven new regressions present but **before modifying `src/client.ts`**, ran:

```sh
npm --prefix typescript test -- tests/client.test.ts tests/version-scripts.test.ts
# FAIL: 7 failed, 41 passed (2 files).
# - Both hanging getToken cases and both hanging refresh cases timed out at 1000ms.
# - Cancelling one shared-refresh waiter also timed out at 1000ms.
# - Token acquisition cancellation dispatched anyway (TypeError, not AbortError).
# - Delayed old token triggered 2 refresh calls; expected 1.
```

After the runtime fix, the **same command passed all 48 tests** (2 files).
The updated User-Agent assertion was already present in the red run; that finding is
source-proven by the removed fixed-date literal and the separate synchronization test,
not one of the seven failing runtime regressions.

### Post-fix checks

Executed from the feature worktree; no checks were loosened and no publication occurred:

```sh
npm --prefix typescript test -- tests/client.test.ts tests/version-scripts.test.ts
# PASS: 48 tests (2 files)
npm --prefix typescript run typecheck
# PASS: source, tests, conformance runner

npx --yes --package=node@22.12.0 -c 'node --version && make ts-check ts-smoke conformance-ts'
npx --yes --package=node@24 -c 'node --version && make ts-check ts-smoke conformance-ts'
(node --version && make ts-check ts-smoke conformance-ts)
# Each PASS on v22.12.0 / v24.20.0 / v26.7.0 respectively:
# 190 tests, 130-operation generation freshness, build/typecheck, package smoke,
# 184/184 applicable conformance cases; 3 explicitly not applicable.

env -u GOROOT GOWORK=off \
  PATH="/home/rzolkos/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.7.linux-amd64/bin:$PATH" \
  make check
# PASS before correction commit: MVP gate passed; Go lint 0 issues,
# Go tests and 187/187 Go fixtures; 190 TS tests, 184/184 applicable TS fixtures.

actionlint -shellcheck /home/rzolkos/.local/share/mise/installs/shellcheck/0.11.0/shellcheck-v0.11.0/shellcheck
# PASS: no diagnostics
zizmor --no-progress .github/workflows
# PASS: no findings (3 ignored, 8 suppressed; unchanged suppressions)
npm --prefix typescript audit --audit-level=high
# PASS: zero vulnerabilities

(cd typescript && npm run build && TARBALL=$(npm pack --silent) && \
  npm run smoke -- "$TARBALL" && \
  env -u NODE_AUTH_TOKEN -u NPM_TOKEN NPM_CONFIG_USERCONFIG=/dev/null \
    npm publish "$TARBALL" --access public --tag latest --dry-run --provenance)
# PASS: isolated exact-artifact smoke and credential-free dry-run, 23 package files.
# Authentication warning expected; npm NOT published.
```

Provider work cannot be forcibly stopped through the existing no-signal provider API;
request cancellation now settles independently and intentionally leaves shared renewal
available to other callers. A never-settling renewal remains shared, but every request
can now time out/cancel rather than hang. Live npm/OIDC and remote workflow execution
remain human prerequisites, not claims established by these local checks.

## PR CI correction: offline smoke with a cold npm cache

The first pushed head `fe546990e450a986529b7be0367d60875adb6d0d` passed all
190 tests/build/typechecks remotely, but all three Node jobs failed package smoke:
[initial GitHub Test run](https://github.com/basecamp/hey-sdk/actions/runs/34141520489).
`npm ci` caches lockfile tarballs, not registry packument metadata; the isolated
`npm install --offline` incorrectly depended on a developer's warmed metadata cache.
Aggregate conformance correctly skipped because its upstream matrix failed.

The smoke script now constructs an isolated consumer lock from the frozen production
entries plus the exact tested SDK tarball's SHA-512 integrity, verifies the packed
manifest's dependencies match the frozen root, and uses `npm ci --offline`. No runtime,
API, package dependency, workflow or gate changed. The artifact still installs outside
the repository with no network, lifecycle scripts or local-source resolution.

Reproduction and passing-after commands (executed locally):

```sh
CACHE=$(mktemp -d /tmp/hey-ts-cold-cache-XXXXXX)
echo "$CACHE" > /tmp/hey-ts-cold-cache-path
npm_config_cache="$CACHE" npm --prefix typescript ci
npm_config_cache="$CACHE" make ts-smoke
# Before script correction: FAIL, ENOTCACHED for lossless-json registry metadata.
# After correction, same cache and command: PASS, 23-file isolated package smoke.

npx --yes --package=node@22.12.0 -c 'node --version && npm_config_cache="$(cat /tmp/hey-ts-cold-cache-path)" make ts-check ts-smoke conformance-ts'
npx --yes --package=node@24 -c 'node --version && npm_config_cache="$(cat /tmp/hey-ts-cold-cache-path)" make ts-check ts-smoke conformance-ts'
(node --version && npm_config_cache="$(cat /tmp/hey-ts-cold-cache-path)" make ts-check ts-smoke conformance-ts)
# Each PASS: Node 22.12.0 / 24.20.0 / 26.7.0, 190 tests, generated freshness,
# build/typecheck, offline package smoke and 184/184 applicable fixtures.

(cd typescript && npm run build && TARBALL=$(npm pack --silent) && \
  npm_config_cache="$(cat /tmp/hey-ts-cold-cache-path)" npm run smoke -- "$TARBALL" && \
  env -u NODE_AUTH_TOKEN -u NPM_TOKEN NPM_CONFIG_USERCONFIG=/dev/null \
    npm publish "$TARBALL" --access public --tag latest --dry-run --provenance)
# PASS: exact-artifact offline smoke and dry-run only; npm NOT published.
```

Additional manual negative probes passed: repacking the artifact with
`lossless-json: 0.0.0` fails the manifest/lock assertion before installation;
using a wholly empty npm cache (no dependency tarball) fails with `ENOTCACHED`,
proving no online fallback. An initial synthetic repack included directory entries
and was rejected by the existing file whitelist; repeating with only package files
exercised the intended dependency assertion. No test or whitelist was loosened.

Pre-commit `make check` **passed**, using the same coherent Go invocation documented
above: MVP gate passed, 190 TS tests, 184 applicable TS fixtures and 187 Go fixtures,
plus all shared freshness/lint/version gates. The prior independent review does not
cover this new smoke-script correction; renewed review is required.

### Renewed review: reject unsupported artifact install semantics

The first CI correction (`eb5931e002a9e55b82cb45813cd2b25d7bf72503`) passed
[remote Test CI](https://github.com/basecamp/hey-sdk/actions/runs/34142233197), but
renewed independent review demonstrated a false-positive: adding an optional override
of `lossless-json` or a nonexistent required peer still passed, because the synthetic
consumer lock omitted those manifest semantics. Green CI did not establish correctness.

The smoke now explicitly rejects optional/peer dependencies and peer metadata,
both bundled-dependency spellings, overrides, workspaces and OS/CPU/libc restrictions
in the artifact or frozen root. These are unsupported by this smoke fixture, not
unsupported by npm; HEY currently uses none of them. It verifies engine equality
against the frozen Node contract and preserves engines in the consumer lock. Future
adoption of these manifest features must extend the fixture deliberately; it cannot
silently smoke a different installation graph. The ordinary dependency equality,
SHA-512 integrity, offline install, lifecycle-script refusal, package whitelist,
isolated imports/int64 behavior and consumer typechecks remain intact.

New automated `tests/package-smoke.test.ts` builds/packs the real package, confirms
the unmodified artifact passes, and performs file-only adversarial repacks changing
only the manifest. Executed with the same cold tarball-only cache:

```sh
npm_config_cache="$(cat /tmp/hey-ts-cold-cache-path)" \
  npm --prefix typescript test -- tests/package-smoke.test.ts
# Before runtime-script correction: 12 failed / 1 passed.
# Ten unsupported-field artifacts plus an impossible Node engine falsely passed.
# Dependency-drift artifact already failed, but lacked the new precise diagnostic.
# After correction, same command: 13/13 passed.
```

Re-executed the exact three Node matrix commands and exact-artifact dry-run command
in the preceding section: **all passed**, now **203 tests across 7 files**, generation,
build/typecheck, 23-file offline smoke, and **184 applicable fixtures** on each of
22.12.0 / 24.20.0 / 26.7.0. npm was not published. No API/runtime code, dependency
versions, workflow settings or coverage exclusions changed. The same coherent-Go
`make check` command also **passed before commit**, now with 203 TS tests, 184 TS
fixtures, 187 Go fixtures and all shared gates (`MVP gate passed`).

## Initial implementation commands and results

The following historical runs preceded the review corrections (183 tests). Post-fix
results above supersede them. Executed from the feature worktree (unless a command changes directory):

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
