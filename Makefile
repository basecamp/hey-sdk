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
# Drift detection
#------------------------------------------------------------------------------

.PHONY: drift-check drift-check-mvp drift-check-full drift-regen

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

# Reverse: every JSON-capable route is either modeled or excluded (Phase 3)
drift-check-reverse:
	@echo "==> Drift check (reverse)..."
	@echo "TODO: Phase 3 — verify every JSON-capable route is modeled or excluded"
	@echo "SKIP: reverse drift check not yet implemented"

# MVP: forward + shape only
drift-check-mvp: drift-check-forward drift-check-shape

# Full: forward + reverse + shape
drift-check-full: drift-check-forward drift-check-reverse drift-check-shape

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
	@$(MAKE) check-full
	git tag "v$(VERSION)"
	git push origin "v$(VERSION)"

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
# TypeScript SDK
#------------------------------------------------------------------------------

.PHONY: ts-generate ts-generate-services ts-build ts-test ts-typecheck ts-check

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

ts-check: ts-build ts-test ts-typecheck

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

.PHONY: swift-generate swift-build swift-test swift-check

swift-generate:
	$(MAKE) -C swift generate

swift-build:
	$(MAKE) -C swift build

swift-test:
	$(MAKE) -C swift test

swift-check:
	$(MAKE) -C swift check

#------------------------------------------------------------------------------
# Kotlin SDK
#------------------------------------------------------------------------------

.PHONY: kt-generate-services kt-build kt-test kt-check kt-check-drift

kt-generate-services:
	cd kotlin && ./gradlew :generator:run --args="$(CURDIR)/openapi.json $(CURDIR)/behavior-model.json"

kt-build:
	cd kotlin && ./gradlew :sdk:build

kt-test:
	cd kotlin && ./gradlew :sdk:test

kt-check: kt-build kt-test

kt-check-drift:
	./scripts/check-kotlin-service-drift.sh

#------------------------------------------------------------------------------
# Conformance
#------------------------------------------------------------------------------

.PHONY: conformance conformance-mvp conformance-full conformance-go conformance-ts conformance-rb conformance-swift conformance-kt

conformance-go:
	cd conformance/runner/go && go run .

conformance-ts:
	cd conformance/runner/typescript && npm test

conformance-rb:
	cd conformance/runner/ruby && bundle exec ruby runner.rb

conformance-swift:
	$(MAKE) -C conformance/runner/swift test

conformance-kt:
	cd conformance/runner/kotlin && ./gradlew test

# MVP: behavioral tests only (conformance/tests/*.json)
conformance-mvp: conformance-go

# Full: MVP + full-surface tests (conformance/tests/ + conformance/tests/full/)
conformance-full: conformance-go conformance-ts conformance-rb conformance-swift conformance-kt

# Bare alias points to full
conformance: conformance-full

audit-check:
	@echo "==> Checking rubric audit..."
	@test -f rubric-audit.json || \
		{ echo "ERROR: rubric-audit.json not found."; exit 1; }

#------------------------------------------------------------------------------
# Progressive gates
#------------------------------------------------------------------------------

# Phase 0-1: Smithy model validation
check-mvp: smithy-check behavior-model-check drift-check-mvp \
           go-check go-check-drift conformance-mvp
	@echo "==> MVP gate passed"

# Phase 3: Full surface, all languages
check-full: smithy-check behavior-model-check drift-check-full \
            sync-api-version-check provenance-check \
            go-check-drift kt-check-drift \
            go-check ts-check rb-check swift-check kt-check \
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
	@echo "  check-mvp   Run MVP gate (Smithy + Go + conformance)"
	@echo "  check-full  Run full gate (all languages + conformance + audit)"
	@echo "  check       Alias for check-mvp"
	@echo "  smithy-build   Regenerate OpenAPI from Smithy"
	@echo "  drift-regen    Regenerate route snapshot from haystack"
	@echo "  clean          Remove build artifacts"
	@echo "  help           Show this help"
