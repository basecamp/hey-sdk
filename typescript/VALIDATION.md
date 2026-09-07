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
| Full modeled API, types, routes | `src/generated/coverage.json`: 130 operations; `tests/generator.test.ts` invokes every generated method against Fetch and checks OpenAPI-derived method/path/query/body. Byte-for-byte regeneration checks schemas, operations, guards and coverage. The narrow source `@heyNullable` member trait represents HEY's explicit null time-track category and regenerates OpenAPI plus both SDKs. |
| HEY runtime behavior and artifact checks | 463 tests across 9 files: generation, precision/null/optional values, safe retries, no mutation replay, refresh, errors/body caps, URL safety, redirects, Link envelopes/window boundaries, account selection/isolation, signed upload isolation, OAuth forms/PKCE, release idempotency, conformance fail-closed contract and transactional version scripts. |
| Shared conformance | Real loopback HTTP through generated SDK methods: **184 passed / 184 applicable**. Exactly 3 named unmodeled Go calendar-update form fixtures excluded, 187 total; explicit checked identity inventory and applicability reasons. Unknown assertions/operations/configuration, empty tests and stale exclusions fail. |
| Node support | Node **22.12.0**, **24.20.0**, **26.7.0** each built, typechecked, ran all 463 tests and all 184 applicable conformance fixtures, and passed isolated package smoke. Renewed remote CI is required after commit. |
| Installable artifact | 23-file npm tarball installed offline outside the repository; ESM root and OAuth subpath imports, real SDK int64 request/response and consumer TypeScript positive/negative typechecks passed. Exact packed tarball smoke also passed. |
| Existing Go preserved | `make check`: Go vet/lint/tests and 187 Go conformance cases, Smithy/route/shape freshness plus Go wrapper drift, TypeScript checks/conformance and API-version sync. |
| Delivery security | actionlint + shellcheck and zizmor passed; `npm audit` found zero vulnerabilities. Credential-free `npm publish --dry-run --provenance` passed. OIDC/registry ownership remain unverified and publishing disabled by default. |

## Senior-maintainer review corrections after `2818e80`

A full base-to-head review found ten runtime, contract, conformance and release defects.
Two independent fresh-context reviews of the correction found four additional boundary
failures; those were corrected before the final gate. The resulting changes:

- charge a 401 refresh resend to the remaining retry budget; reject non-finite and
  timer-overflowing `Retry-After` values; validate operation timeouts before credentials;
- normalize Fetch and response-stream failures for generated calls, OAuth and signed
  uploads while preserving cancellation and bounded-response errors;
- parse RFC 8288 Link parameters without treating quoted text as relations, including
  optional whitespace before delimiters;
- represent HEY's explicit null uncategorized time-track response through a narrow
  Smithy member trait and regenerated OpenAPI/Go/TypeScript artifacts;
- validate complete OAuth token/discovery shapes and reject already-aborted calls before
  Fetch;
- make conformance body-absence assertions presence-aware, validate configuration values,
  and preserve the shared zero-repeat-means-once contract;
- make npm dry-run/publish idempotency use exact structured E404 and integrity checks,
  rejecting malformed or incidental registry diagnostics;
- require canonical SDK versions and stage every Go/TypeScript version replacement before
  transactional installation, with rollback and fail-closed source extraction; and
- remove generated trailing whitespace at the generator boundary.

Named regressions live in `tests/client.test.ts`, `tests/account-scope.test.ts`,
`tests/oauth.test.ts`, `tests/generator.test.ts`, `tests/conformance-contract.test.ts`,
`tests/release-workflow.test.ts` and `tests/version-scripts.test.ts`. Final local results:

```sh
npm --prefix typescript test                 # PASS: 463 tests / 9 files
npm --prefix typescript run typecheck         # PASS
npm --prefix typescript run conformance       # PASS: 184/184 applicable; 3 exclusions
make ts-smoke                                 # PASS: 23-file isolated artifact
node scripts/npm-publish-artifact.mjs --dry-run … # PASS: exact packed artifact; no publish
npm --prefix typescript audit --audit-level=high # PASS: 0 vulnerabilities
npx --yes --package=node@22.12.0 -c 'make ts-check ts-smoke conformance-ts' # PASS
npx --yes --package=node@24 -c 'make ts-check ts-smoke conformance-ts'       # PASS
# Native Node 26.7.0 passed the same checks through make check + make ts-smoke.
actionlint -shellcheck /home/rzolkos/.local/share/mise/installs/shellcheck/0.11.0/shellcheck-v0.11.0/shellcheck # PASS
zizmor --no-progress .github/workflows     # PASS: no findings

env -u GOROOT GOWORK=off PATH="/home/rzolkos/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.7.linux-amd64/bin:$PATH" make check
# PASS: complete Go + TypeScript gate; 187 Go fixtures, 184 TypeScript fixtures
git diff --check                              # PASS
```

The npm helper's E404 path was additionally exercised against the public registry using a
nonexistent tarball: the structured absence check reached dry-run and then failed locally
with ENOENT, so no publication was possible. Hermetic tests cover identical/conflicting
integrity, structured E404, E500 mentioning E404, malformed errors, dry-run and publish
command selection. Live ownership/OIDC publication remains unverified and inactive.

## PR #144 feedback corrections at `2818e80`

These focused corrections follow reviewed head
`4bb1bab98347863037cda15cc59065c0a5b78a61`. Earlier independent reviews and remote
checks do **not** cover this correction; renewed exact-head review and CI are required.
The originating feedback is [PR #144's review](https://github.com/basecamp/hey-sdk/pull/144)
and the inline discussions linked below. No Go behavior, generated sources, modeled
operations, retry budget, release workflow or applicability exclusions changed.

| Finding / acceptance criterion | Correction and named regression coverage | Failing before → passing after |
|---|---|---|
| [A: OAuth HTTP errors](https://github.com/basecamp/hey-sdk/pull/144#discussion_r3951430383) retain status mapping with empty/malformed bodies | `tests/oauth.test.ts`: `preserves HTTP error metadata` exercises public discovery, code exchange and refresh for 401/403/429/500/503 × empty/HTML/broken JSON. Checks code, status, retryability, request ID, hints and sanitized fallback message. `rejects malformed successful JSON` and `preserves structured error messages and bounds error bodies` pin success parsing, valid errors and the existing 1 MiB cap. Existing redirect/insecure-target tests remain. | With A tests added before the runtime edit: **45 failed / 12 passed**. Same file after A: **57 passed**. |
| B: successful grants require the public `token_type` string | `tests/oauth.test.ts`: both grant helpers `reject malformed token_type` for missing, empty, numeric, null, boolean and object values; `accept non-empty token_type` preserves Bearer and a custom scheme (no new scheme restriction). | With B tests added after A, before the token guard: **12 failed / 61 passed**. Same file after B: **73 passed**. |
| [C: assertion shapes fail closed](https://github.com/basecamp/hey-sdk/pull/144#discussion_r3951430307) | `tests/conformance-contract.test.ts`: `accepts legitimate assertion`, `rejects extra, missing and inherited fields`, `rejects malformed assertion` cover all 19 supported kinds, min/max delay variants, path-specific value types, null/empty-string/boolean/nested/lossless-bigint values, the reported `expectd` typo and required own fields. Existing unknown-kind/path/config/empty-list checks remain. Real runner passes all existing fixtures. | Before validator edit: **83 failed / 35 passed**. After: **118 passed**, plus **184/184** applicable fixtures. |
| [D: declared Node support matches documentation](https://github.com/basecamp/hey-sdk/pull/144#discussion_r3951430349) | `tests/node-engines.test.ts`: `declares support for Node` evaluates the actual manifest with pinned dev-only `semver@7.7.4` (`satisfies`); 19 boundary/major/prerelease cases plus SDK/runner manifest and frozen-lock equality. `tests/package-smoke.test.ts` preserves offline artifact graph and packed-engine consistency checks. Range: `^22.12.0 || ^24.0.0 || ^26.0.0`. No production dependency changed. | Before engine/lock correction: **8 failed / 12 passed**. After: **20 passed**, or **33 passed** including existing smoke tests. |

Focused commands (same command before/after each corresponding edit):

```sh
npm --prefix typescript test -- tests/oauth.test.ts                  # A, then B
npm --prefix typescript test -- tests/conformance-contract.test.ts  # C
npm --prefix typescript test -- tests/node-engines.test.ts          # D
npm --prefix typescript test -- tests/node-engines.test.ts tests/package-smoke.test.ts
npm --prefix typescript run typecheck
npm --prefix typescript run conformance
```

All post-fix commands passed. Failing-before runs used the new tests with the relevant
production fix still absent; failures were behavioral assertion failures, not tool or
compilation failures. The final combined suite is **410 passed across 8 files**.

### Frozen installs, supported matrix and artifact

Executed sequentially from this worktree. A new cache was populated only by frozen
installation before the offline smoke runs; SDK and runner lockfile SHA-256 sums were
identical before and after installation and the matrix.

```sh
sha256sum typescript/package-lock.json conformance/runner/typescript/package-lock.json > /tmp/hey-pr144-fixes/locks-before.sha256
CACHE=$(mktemp -d /tmp/hey-pr144-fixes/npm-cache-XXXXXX)
printf '%s\n' "$CACHE" > /tmp/hey-pr144-fixes/cache-path
npm_config_cache="$CACHE" make ts-install
npm_config_cache="$CACHE" npm --prefix conformance/runner/typescript ci
sha256sum -c /tmp/hey-pr144-fixes/locks-before.sha256
npm_config_cache="$CACHE" npm --prefix typescript audit --audit-level=high
# PASS: both frozen installs, unchanged locks, zero vulnerabilities.

npx --yes --package=node@22.12.0 -c 'node --version && npm_config_cache="$(cat /tmp/hey-pr144-fixes/cache-path)" make ts-check ts-smoke conformance-ts'
npx --yes --package=node@24.20.0 -c 'node --version && npm_config_cache="$(cat /tmp/hey-pr144-fixes/cache-path)" make ts-check ts-smoke conformance-ts'
(node --version && npm_config_cache="$(cat /tmp/hey-pr144-fixes/cache-path)" make ts-check ts-smoke conformance-ts)
# PASS on 22.12.0 / 24.20.0 / 26.7.0 respectively:
# 410 tests, source/test/runner typechecks, build, 130-operation generation freshness,
# 23-file offline package smoke, 184/184 applicable fixtures; exactly 3 exclusions.

(cd typescript && npm run build && TARBALL=$(npm pack --silent --pack-destination /tmp/hey-pr144-fixes) && \
  npm_config_cache="$(cat /tmp/hey-pr144-fixes/cache-path)" npm run smoke -- "/tmp/hey-pr144-fixes/$TARBALL" && \
  env -u NODE_AUTH_TOKEN -u NPM_TOKEN NPM_CONFIG_USERCONFIG=/dev/null \
    npm publish "/tmp/hey-pr144-fixes/$TARBALL" --access public --tag latest --dry-run --provenance)
# PASS: exact artifact offline installation/import/int64/consumer typecheck and
# credential-free dry-run. 23 files. Authentication warning expected; NOT published.
```

Combined pre-commit gate:

```sh
env -u GOROOT GOWORK=off \
  PATH="/home/rzolkos/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.7.linux-amd64/bin:$PATH" \
  make check
# PASS: MVP gate passed; Go vet/lint (0 issues)/tests, 187 Go fixtures,
# 410 TS tests, 184 applicable TS fixtures, generation/typechecks/build,
# shared Smithy/behavior/route/shape freshness, wrapper drift and version sync.
git diff --check
# PASS
```

The host-only Go environment correction remains the one documented below; no gate
was bypassed. The full gate is repeated after the evidence update immediately before
committing. Local results do not claim current-head GitHub CI or independent review.

The new semver dependency is **test-only**, used solely for real engine-range evaluation;
it is not shipped in the consumer's production graph. Registry ownership, OIDC and
remote workflow execution are still unverified here. npm activation remains disabled
by default. No visual artifacts apply to these non-visual SDK and harness corrections.

## Earlier review corrections and regression evidence

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
