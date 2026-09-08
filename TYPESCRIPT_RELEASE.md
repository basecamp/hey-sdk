# TypeScript release setup (human-owned)

Proposed public package: **@37signals/hey**, following @37signals/basecamp.
Neither npm organization ownership nor trusted publishing has been verified. This
change does not configure live repository, npm, environments, credentials or settings.

## Safe activation boundary

Both `release-typescript.yml` and `release-github.yml` read
**`.github/typescript-publish-enabled` from the tagged commit**. The file contains
exactly `true` or `false`; its value is immutable for the complete release. `false`
means:

- Tag/manual TypeScript workflows still validate, test all supported Node versions,
  run conformance, build, smoke-test and pack an artifact, and dry-run publishing.
- **Nothing is published to npm.** The workflow emits an explicit notice.
- GitHub release orchestration waits only for Go, preserving existing Go releases.
  It does not claim a completed TypeScript publication.

Once enabled, tag pushes permit the protected publish job and GitHub orchestration
waits for `go,typescript`. Manual `workflow_dispatch` is **always dry-run**, even
when enabled, with no environment approval or OIDC permission. An npm failure after
activation intentionally blocks the combined release; Go module tags remain compatible.

## Provisioning checklist — maintainer action, not automation in this PR

1. Confirm the npm `37signals` organization controls/approves `@37signals/hey`, its
   public name, license and first version. The initial repository version is 0.29.0
   to align with Go; this does not assert that npm version exists.
2. Establish the package under that organization. First-package bootstrapping may
   require an authorized human publish before npm allows trusted-publisher setup;
   follow npm's current process, never invent an owner or embed a token in CI.
3. Configure npm trusted publishing for owner **basecamp**, repository **hey-sdk**,
   workflow filename **release-typescript.yml**, environment **release-npm**. Confirm
   npm's current OIDC/provenance requirements, public repository eligibility and
   workflow identity. Workflow uses Node 24's npm >=11.5.1, no long-lived npm token.
4. Create/protect GitHub environment `release-npm`: required human reviewers and
   deployment policies permitting only reviewed version tags on main. Verify fork
   PRs and arbitrary branch/manual dispatch cannot obtain publishing authority.
5. Run `Release TypeScript SDK` manually at the intended main commit. Inspect tests,
   conformance, package smoke, dry-run file list and downloadable tarball. This
   credential-free run cannot prove ownership, OIDC identity or provenance minting.
6. Only after verification, change `.github/typescript-publish-enabled` to `true`
   and replace the README's pending-provisioning notice in the same reviewed commit.
   Then release a **new**, synchronized version from main via the normal human-owned
   release procedure. Do not reuse an already-published bootstrap version with a
   different tarball. Observe npm provenance and actual installation after the first
   publish, and GitHub orchestration waiting for both SDK workflows.

## Versioning and release mechanics

`make bump VERSION=x.y.z` updates Go's constant, TypeScript's constant, package and
private conformance manifests, and both lockfile root versions. `make sync-api-version`
updates API constants from OpenAPI. `make check` verifies they match. Review/merge the
bump before tagging. `make release VERSION=x.y.z` retains the existing plain and Go
subdirectory tags, and validates shipped artifacts before tagging. Humans own tags,
pushes and merging; there is no auto-merge or package publish on main pushes.

The release workflow verifies tag ancestry on `origin/main`, strict package/version
agreement, generated freshness, supported Node matrix, and conformance. Packing has
no OIDC permission; publishing has no checkout, install, cache or build. The protected
job uploads the **exact tarball** built earlier, with provenance and least privileges.
An existing version is accepted only when npm's SHA-512 integrity matches that tarball;
registry errors other than explicit E404 fail closed. Re-run the failed **tag workflow
run** after fixing infrastructure (manual dispatch does not publish). Conflicting
published bytes require a new version, not overwrite/unpublish. The inherited shared
GitHub orchestrator is unchanged except its language set; its existing retry/manual
release semantics are not redesigned here.

## Monitoring and rollback

Monitor `Release TypeScript SDK`, `Release GitHub`, npm integrity/provenance and a real
consumer install after activation. To stop future publication, change
`.github/typescript-publish-enabled` to `false`, merge that reviewed commit before the
next tag, and disable pending deployments as a human administrator. The state recorded
in an existing tag remains authoritative for its release. Existing npm packages cannot
be rolled back by this workflow: consumers pin a prior good version, and fixes ship in
a new reviewed version. No migrations, backfills, data writers, scheduled app jobs,
registry secrets or live HEY API credentials are introduced. Security scanning and
Dependabot cover the new npm dependency graph.
