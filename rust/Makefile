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
	$(CARGO) run -q --locked -p hey-sdk-generator -- --check

.PHONY: fmt
fmt:
	$(CARGO) fmt --all

.PHONY: fmt-check
fmt-check:
	$(CARGO) fmt --all -- --check

.PHONY: lint
lint:
	$(CARGO) clippy --workspace --all-targets --all-features --locked -- -D warnings

.PHONY: test
test:
	$(CARGO) test --workspace --all-features --locked

# The crate with the shipped HTTP client off. The library has to build without it, and the
# test suite has to pass through a transport of the caller's own (tests/support), since that
# is what the feature promises. The doctests are left out: they show the shipped client.
.PHONY: no-default-features
no-default-features:
	$(CARGO) clippy -p hey-sdk --all-targets --no-default-features --locked -- -D warnings
	$(CARGO) test -p hey-sdk --lib --tests --no-default-features --locked

# Broken intra-doc links fail the build
.PHONY: doc
doc:
	RUSTDOCFLAGS="-D warnings" $(CARGO) doc -p hey-sdk --no-deps --all-features --locked

# RustSec advisories, licences and the shape of the dependency graph, for this workspace and
# the conformance runner's, against the one deny.toml. Needs cargo-deny installed.
.PHONY: deny
deny:
	@$(CARGO) deny --version >/dev/null 2>&1 || \
		{ echo "ERROR: cargo-deny is not installed. Run: cargo install cargo-deny --locked"; exit 1; }
	$(CARGO) deny --locked check
	cd ../conformance/runner/rust && $(CARGO) deny --locked --config ../../../rust/deny.toml check

# Run all checks: the same steps as CI's test-rust job, in the same order
.PHONY: check
check: fmt-check lint test no-default-features doc deny

.PHONY: clean
clean:
	$(CARGO) clean

.PHONY: help
help:
	@echo "HEY Rust SDK Makefile targets:"
	@echo ""
	@echo "  generate             Regenerate src/generated from openapi.json"
	@echo "  generate-check       Verify src/generated is up to date"
	@echo "  fmt                  Format code"
	@echo "  fmt-check            Check formatting"
	@echo "  lint                 Run clippy with warnings denied"
	@echo "  test                 Run all tests"
	@echo "  no-default-features  Lint and test the crate without the shipped HTTP client"
	@echo "  doc                  Build the crate docs"
	@echo "  deny                 cargo deny over both workspaces (advisories, licences, bans)"
	@echo "  check                fmt-check + lint + test + no-default-features + doc + deny"
	@echo "  clean                Remove build artifacts"
