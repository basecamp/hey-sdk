# HEY Rust SDK Makefile

CARGO := cargo

.PHONY: all
all: check

# Regenerate rust/hey-sdk/src/generated from openapi.json and behavior-model.json
.PHONY: generate
generate:
	@echo "Generating Rust types, routes and services from openapi.json..."
	$(CARGO) run -q -p hey-sdk-generator

# Fail when the checked-in generated code differs from what the generator emits
.PHONY: generate-check
generate-check:
	$(CARGO) run -q -p hey-sdk-generator -- --check

.PHONY: fmt
fmt:
	$(CARGO) fmt --all

.PHONY: fmt-check
fmt-check:
	$(CARGO) fmt --all -- --check

.PHONY: lint
lint:
	$(CARGO) clippy --workspace --all-targets -- -D warnings

.PHONY: test
test:
	$(CARGO) test --workspace

# Broken intra-doc links fail the build
.PHONY: doc
doc:
	RUSTDOCFLAGS="-D warnings" $(CARGO) doc -p hey-sdk --no-deps

# Run all checks (local development gate)
.PHONY: check
check: fmt-check lint test doc

.PHONY: clean
clean:
	$(CARGO) clean

.PHONY: help
help:
	@echo "HEY Rust SDK Makefile targets:"
	@echo ""
	@echo "  generate        Regenerate src/generated from openapi.json"
	@echo "  generate-check  Verify src/generated is up to date"
	@echo "  fmt             Format code"
	@echo "  fmt-check       Check formatting"
	@echo "  lint            Run clippy with warnings denied"
	@echo "  test            Run all tests"
	@echo "  doc             Build the crate docs"
	@echo "  check           fmt-check + lint + test + doc"
	@echo "  clean           Remove build artifacts"
