# HEY SDK Makefile

.PHONY: all check check-mvp check-full clean help

all: check-mvp

#------------------------------------------------------------------------------
# Smithy targets
#------------------------------------------------------------------------------

.PHONY: smithy-validate smithy-build smithy-check smithy-mapper smithy-clean

smithy-validate:
	@echo "==> Validating Smithy spec..."
	cd spec && smithy validate

smithy-mapper:
	@echo "==> Building smithy-bare-arrays mapper..."
	@test -f spec/lib/com/basecamp/smithy-bare-arrays/1.0.0/smithy-bare-arrays-1.0.0.jar || \
		{ echo "ERROR: Vendored JAR not found. Build with: cd spec/smithy-bare-arrays && ./gradlew publishToMavenLocal"; exit 1; }

smithy-build: smithy-mapper
	@echo "==> Building OpenAPI from Smithy..."
	cd spec && smithy build
	cp spec/build/smithy/openapi/openapi/HEY.openapi.json openapi.json
	./scripts/enhance-openapi-go-types.sh
	@$(MAKE) behavior-model

smithy-check: smithy-validate smithy-mapper
	@echo "==> Checking OpenAPI freshness..."
	@cd spec && smithy build
	@cp spec/build/smithy/openapi/openapi/HEY.openapi.json /tmp/hey-openapi-check.json
	@./scripts/enhance-openapi-go-types.sh /tmp/hey-openapi-check.json > /dev/null 2>&1
	@diff -q openapi.json /tmp/hey-openapi-check.json > /dev/null 2>&1 || \
		{ echo "ERROR: openapi.json is out of date. Run 'make smithy-build'"; exit 1; }
	@rm -f /tmp/hey-openapi-check.json

smithy-clean:
	rm -rf spec/build

#------------------------------------------------------------------------------
# Behavior model
#------------------------------------------------------------------------------

.PHONY: behavior-model behavior-model-check

behavior-model:
	@echo "==> Generating behavior model..."
	./scripts/generate-behavior-model

behavior-model-check:
	@echo "==> Checking behavior model freshness..."
	@./scripts/generate-behavior-model --check

#------------------------------------------------------------------------------
# URL routes
#------------------------------------------------------------------------------

.PHONY: url-routes url-routes-check

url-routes:
	@echo "==> Generating url-routes.json..."
	./scripts/generate-url-routes

url-routes-check:
	@echo "==> Checking url-routes.json freshness..."
	@tmpfile=$$(mktemp) && \
	./scripts/generate-url-routes openapi.json "$$tmpfile" > /dev/null && \
	diff -q go/pkg/hey/url-routes.json "$$tmpfile" > /dev/null 2>&1 || \
		{ rm -f "$$tmpfile"; echo "ERROR: url-routes.json is out of date. Run 'make url-routes'"; exit 1; }; \
	rm -f "$$tmpfile"

#------------------------------------------------------------------------------
# Drift detection
#------------------------------------------------------------------------------

.PHONY: drift-check drift-check-mvp drift-check-full drift-check-coverage drift-check-forward drift-check-reverse drift-check-shape drift-regen

# route-coverage.json is derived from openapi.json and read by both drift checks;
# a stale file means the gate never sees new operations. Refuse that.
drift-check-coverage:
	@echo "==> Drift check (route coverage freshness)..."
	@./scripts/generate-route-coverage --check

# Forward-only: every modeled operation has a matching route
drift-check-forward:
	@echo "==> Drift check (forward)..."
	@if jq -e 'length == 0' spec/route-snapshot.json > /dev/null 2>&1; then \
		echo "ERROR: route-snapshot.json is empty. Run 'make drift-regen HAYSTACK_DIR=~/Work/basecamp/haystack' to populate."; \
		exit 1; \
	fi
	@./scripts/drift-check-forward

# Shape fingerprint unchanged
drift-check-shape:
	@echo "==> Drift check (shape fingerprints)..."
	@./scripts/generate-shape-fingerprint --check

# Reverse: every JSON-capable route is either modeled or excluded
drift-check-reverse:
	@echo "==> Drift check (reverse)..."
	@./scripts/drift-check-reverse

# MVP: coverage freshness + forward + reverse + shape
drift-check-mvp: drift-check-coverage drift-check-forward drift-check-reverse drift-check-shape

# Full: same as MVP (kept as an alias for check-full)
drift-check-full: drift-check-mvp

# Convenience alias
drift-check: drift-check-mvp

# Regenerate route snapshot from local haystack checkout
drift-regen:
	@echo "==> Regenerating route snapshot from haystack..."
	@test -d $(HAYSTACK_DIR) || \
		{ echo "ERROR: Set HAYSTACK_DIR to your local haystack checkout."; exit 1; }
	./scripts/generate-route-snapshot $(HAYSTACK_DIR)

HAYSTACK_DIR ?= $(HOME)/Work/basecamp/haystack

#------------------------------------------------------------------------------
# Provenance
#------------------------------------------------------------------------------

.PHONY: provenance-check provenance-sync

provenance-check:
	@echo "==> Checking API provenance..."
	@test -f spec/api-provenance.json || \
		{ echo "ERROR: spec/api-provenance.json not found."; exit 1; }
	@source=$$(jq -r '.source' spec/api-provenance.json); \
		test "$$source" = "haystack" || \
		{ echo "ERROR: API provenance source is '$$source', want 'haystack'."; exit 1; }
	@sha=$$(jq -r '.sha' spec/api-provenance.json); \
		echo "$$sha" | grep -Eq '^[0-9a-f]{40}$$' || \
		{ echo "ERROR: API provenance SHA is not a full Git commit SHA."; exit 1; }
	@api_version=$$(sed -n 's/^[[:space:]]*version: "\([^"]*\)"/\1/p' spec/hey.smithy | head -1); \
		provenance_date=$$(jq -r '.date' spec/api-provenance.json); \
		test "$$api_version" = "$$provenance_date" || \
		{ echo "ERROR: API version $$api_version does not match provenance date $$provenance_date."; exit 1; }

provenance-sync:
	@echo "==> Syncing provenance from haystack..."
	./scripts/sync-provenance

#------------------------------------------------------------------------------
# Version management
#------------------------------------------------------------------------------

.PHONY: bump release sync-api-version sync-api-version-check

bump:
ifndef VERSION
	$(error VERSION is required. Usage: make bump VERSION=x.y.z)
endif
	@echo "Bumping to v$(VERSION)..."
	./scripts/bump-version.sh $(VERSION)

sync-api-version:
	@echo "==> Syncing API version across languages..."
	./scripts/sync-api-version.sh

sync-api-version-check:
	@echo "==> Checking API version sync..."
	./scripts/sync-api-version.sh --check

release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=x.y.z)
endif
	@echo "Releasing v$(VERSION)..."
	@git diff --quiet && git diff --cached --quiet || \
		{ echo "ERROR: Working tree has uncommitted changes."; exit 1; }
	@bash ./scripts/typescript-publish-state.sh > /dev/null
	@bash ./scripts/kotlin-publish-state.sh > /dev/null
	@grep -Fxq 'const Version = "$(VERSION)"' go/pkg/hey/version.go || \
		{ echo "ERROR: go/pkg/hey/version.go does not say $(VERSION):"; \
		  grep 'const Version' go/pkg/hey/version.go; \
		  echo "       The release workflow checks this, so the tag would fail to publish."; \
		  echo "       Run: make bump VERSION=$(VERSION) — then commit and merge that first."; \
		  exit 1; }
	@grep -Fxq 'version = "$(VERSION)"' rust/hey-sdk/Cargo.toml || \
		{ echo "ERROR: rust/hey-sdk/Cargo.toml does not say $(VERSION):"; \
		  grep '^version' rust/hey-sdk/Cargo.toml; \
		  echo "       Run: make bump VERSION=$(VERSION) — then commit and merge that first."; \
		  exit 1; }
	@node scripts/sync-typescript-versions.mjs --check
	@grep -Fxq 'version = "$(VERSION)"' kotlin/sdk/build.gradle.kts || \
		{ echo "ERROR: kotlin/sdk/build.gradle.kts does not say $(VERSION):"; \
		  grep '^version' kotlin/sdk/build.gradle.kts; \
		  echo "       Run: make bump VERSION=$(VERSION) — then commit and merge that first."; \
		  exit 1; }
	@grep -Fq 'const val VERSION = "$(VERSION)"' kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/HeyConfig.kt || \
		{ echo "ERROR: kotlin HeyConfig.kt does not say $(VERSION). Run: make bump VERSION=$(VERSION)"; exit 1; }
	@grep -Fq 'public static let version = "$(VERSION)"' swift/Sources/Hey/HeyConfig.swift || \
		{ echo "ERROR: swift HeyConfig.swift does not say $(VERSION). Run: make bump VERSION=$(VERSION)"; exit 1; }
	@$(MAKE) check-mvp
	git tag "v$(VERSION)"
	git tag "go/v$(VERSION)"
	git push origin "v$(VERSION)" "go/v$(VERSION)"

#------------------------------------------------------------------------------
# Go SDK
#------------------------------------------------------------------------------

.PHONY: go-check go-check-drift go-test go-lint go-generate

go-generate:
	$(MAKE) -C go generate

go-test:
	$(MAKE) -C go test

go-lint:
	$(MAKE) -C go lint

go-check:
	$(MAKE) -C go check

go-check-drift:
	./scripts/check-service-drift.sh

#------------------------------------------------------------------------------
# Rust SDK
#------------------------------------------------------------------------------

.PHONY: rs-generate rs-check-drift rs-test rs-lint rs-examples rs-deny rs-publish-check rs-check rs-consumer-check

# Types, routes and services are all generated; there is no hand-written wrapper layer
# to drift, so the drift check is the generator's own --check.
rs-generate:
	$(MAKE) -C rust generate

rs-check-drift:
	$(MAKE) -C rust generate-check

rs-test:
	$(MAKE) -C rust test

rs-lint:
	$(MAKE) -C rust lint

rs-examples:
	$(MAKE) -C rust examples

rs-deny:
	$(MAKE) -C rust deny

rs-publish-check:
	$(MAKE) -C rust publish-check

rs-check:
	$(MAKE) -C rust check

# Build the packaged crate from a throwaway consumer, with fresh resolution and no
# dev-dependencies, for the default features and with none
rs-consumer-check:
	./scripts/rs-consumer-check

#------------------------------------------------------------------------------
# TypeScript SDK
#------------------------------------------------------------------------------

.PHONY: ts-install ts-generate ts-generate-services ts-build ts-test ts-typecheck ts-check ts-check-drift ts-smoke

ts-install:
	cd typescript && npm ci

ts-check-drift:
	cd typescript && npm run check:generated

ts-smoke: ts-build
	cd typescript && npm run smoke

ts-generate:
	cd typescript && npm run generate

ts-generate-services:
	cd typescript && npm run generate-services

ts-build:
	cd typescript && npm run build

ts-test:
	cd typescript && npm test

ts-typecheck:
	cd typescript && npm run typecheck

ts-check: ts-check-drift ts-build ts-test ts-typecheck
	node scripts/sync-typescript-versions.mjs --check

#------------------------------------------------------------------------------
# Ruby SDK
#------------------------------------------------------------------------------

.PHONY: rb-generate rb-generate-services rb-build rb-test rb-check

rb-generate:
	cd ruby && bundle exec ruby scripts/generate-metadata.rb
	cd ruby && bundle exec ruby scripts/generate-types.rb

rb-generate-services:
	cd ruby && bundle exec ruby scripts/generate-services.rb

rb-build:
	cd ruby && bundle exec rake build

rb-test:
	cd ruby && bundle exec rake test

rb-check: rb-build rb-test

#------------------------------------------------------------------------------
# Swift SDK
#------------------------------------------------------------------------------

.PHONY: swift-generate swift-build swift-test swift-check swift-check-drift swift-consumer-check

# Regenerate swift/Sources/Hey/Generated from openapi.json and behavior-model.json. The
# generator reads swift/names.toml for its naming overrides.
swift-generate:
	$(MAKE) -C swift generate

swift-build:
	$(MAKE) -C swift build

swift-test:
	$(MAKE) -C swift test

# Every step CI's test-swift job runs before the conformance suite: the library's and the
# generator's build and tests with warnings as errors, and the conformance runner's own tests.
swift-check:
	$(MAKE) -C swift check
	$(MAKE) -C conformance/runner/swift check

# Regenerate into memory and compare with the checked-in tree; stale generated code fails.
swift-check-drift:
	$(MAKE) -C swift check-drift

# Resolve the package from a git tag the way an app does and compile a consumer against it,
# so the root Package.swift ships what the README promises.
swift-consumer-check:
	./scripts/swift-consumer-check

#------------------------------------------------------------------------------
# Kotlin SDK
#------------------------------------------------------------------------------

.PHONY: kt-generate kt-generate-services kt-build kt-test kt-check kt-check-drift kt-consumer-check

# The Gradle build under kotlin/ wants JDK 17; .mise.toml pins one for mise users.
GRADLE := cd kotlin && ./gradlew --quiet

# Regenerate kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/generated from openapi.json and
# behavior-model.json. The generator runs from the repository root and reads
# kotlin/generator/names.toml for its naming overrides.
kt-generate:
	$(GRADLE) :generator:run

# Kept as an alias for the name the shared SDK seed uses.
kt-generate-services: kt-generate

kt-build:
	$(GRADLE) :hey-sdk:build -x allTests

kt-test:
	$(GRADLE) :hey-sdk:allTests

# Every step CI's test-kotlin job runs before the conformance suite: the library's build and
# tests with warnings as errors, the generator's own tests, and the conformance runner's.
kt-check:
	$(GRADLE) :hey-sdk:check :generator:test :conformance:test

# Regenerate into memory and compare with the checked-in tree; stale generated code fails.
kt-check-drift:
	$(GRADLE) :generator:run --args="--check"

# Publish the library to a scratch repository and compile a consumer against it with the
# oldest Kotlin kotlin/README.md promises, so the promise is one the artifact keeps.
kt-consumer-check:
	./scripts/kt-consumer-check

#------------------------------------------------------------------------------
# Conformance
#------------------------------------------------------------------------------

.PHONY: conformance conformance-mvp conformance-full conformance-go conformance-rs conformance-ts conformance-rb conformance-swift conformance-kt

conformance-go:
	cd conformance/runner/go && go run .

conformance-rs:
	cd conformance/runner/rust && cargo run -q --locked

conformance-ts:
	cd conformance/runner/typescript && npm test

conformance-rb:
	cd conformance/runner/ruby && bundle exec ruby runner.rb

conformance-swift:
	$(MAKE) -C conformance/runner/swift test

# The Kotlin runner lives under conformance/runner/kotlin with the other runners and is a
# subproject of the kotlin/ Gradle build, so it runs through that build's wrapper.
conformance-kt:
	$(GRADLE) :conformance:run

# Shipped SDKs: behavioral tests (conformance/tests/*.json)
conformance-mvp: conformance-go conformance-rs conformance-ts conformance-kt conformance-swift

# Full: MVP + full-surface tests (conformance/tests/ + conformance/tests/full/)
conformance-full: conformance-go conformance-rs conformance-ts conformance-kt conformance-rb conformance-swift

# Bare alias covers shipped SDKs only
conformance: conformance-mvp

audit-check:
	@echo "==> Checking rubric audit..."
	@test -f rubric-audit.json || \
		{ echo "ERROR: rubric-audit.json not found."; exit 1; }

#------------------------------------------------------------------------------
# Progressive gates
#------------------------------------------------------------------------------

# Supported gate: Smithy + shipped Go, Rust, TypeScript, Kotlin and Swift SDKs
check-mvp: smithy-check behavior-model-check drift-check-mvp \
           url-routes-check go-check go-check-drift rs-check rs-check-drift \
           ts-check kt-check kt-check-drift swift-check swift-check-drift \
           sync-api-version-check provenance-check conformance-mvp
	@echo "==> MVP gate passed"

# Phase 3: Full surface, all languages
check-full: smithy-check behavior-model-check drift-check-full \
            sync-api-version-check provenance-check \
            go-check-drift rs-check-drift kt-check-drift swift-check-drift \
            go-check rs-check ts-check rb-check swift-check kt-check \
            conformance-full audit-check
	@echo "==> Full gate passed"

check: check-mvp

#------------------------------------------------------------------------------
# Housekeeping
#------------------------------------------------------------------------------

clean: smithy-clean
	rm -rf spec/build

help:
	@echo "HEY SDK Makefile"
	@echo ""
	@echo "  check-mvp   Run MVP gate (Smithy + Go + Rust + TypeScript + Kotlin + Swift + conformance)"
	@echo "  check-full  Run full gate (all languages + conformance + audit)"
	@echo "  check       Alias for check-mvp"
	@echo "  smithy-build   Regenerate OpenAPI from Smithy"
	@echo "  drift-regen    Regenerate route snapshot from haystack"
	@echo "  go-check       Go build, vet, tests, lint"
	@echo "  go-generate    Regenerate go/pkg/generated from openapi.json"
	@echo "  rs-check       What CI runs: fmt, clippy, tests, docs; clippy + tests again with"
	@echo "                 --no-default-features; examples; cargo deny; cargo package"
	@echo "  rs-check-drift Fail if rust/hey-sdk/src/generated is stale"
	@echo "  rs-generate    Regenerate rust/hey-sdk/src/generated from openapi.json"
	@echo "  rs-test        Rust tests only; rs-lint, rs-examples, rs-deny, rs-publish-check likewise"
	@echo "  conformance-rs Run the shared conformance fixtures against the Rust crate"
	@echo "  kt-check       What CI runs: the Kotlin library's build and tests, the generator's"
	@echo "                 tests and the conformance runner's"
	@echo "  kt-check-drift Fail if kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/generated is stale"
	@echo "  kt-consumer-check Compile a consumer of the published library with the oldest Kotlin the README promises"
	@echo "  kt-generate    Regenerate the Kotlin generated tree from openapi.json"
	@echo "  conformance-kt Run the shared conformance fixtures against the Kotlin library"
	@echo "  swift-check    What CI runs: the Swift library's and generator's build and tests with"
	@echo "                 warnings as errors, and the conformance runner's own tests"
	@echo "  swift-check-drift Fail if swift/Sources/Hey/Generated is stale"
	@echo "  swift-consumer-check Resolve the package from a git tag and compile a consumer against it"
	@echo "  swift-generate Regenerate the Swift generated tree from openapi.json"
	@echo "  conformance-swift Run the shared conformance fixtures against the Swift library"
	@echo "  clean          Remove build artifacts"
	@echo "  help           Show this help"
