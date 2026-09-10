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

# The examples under hey-sdk/examples are the README's snippets as whole programs; they
# have to keep compiling.
.PHONY: examples
examples:
	$(CARGO) build -p hey-sdk --examples --all-features --locked

# RustSec advisories, licences and the shape of the dependency graph, for this workspace and
# the conformance runner's, against the one deny.toml. Needs cargo-deny installed.
.PHONY: deny
deny:
	@$(CARGO) deny --version >/dev/null 2>&1 || \
		{ echo "ERROR: cargo-deny is not installed. Run: cargo install cargo-deny --locked"; exit 1; }
	$(CARGO) deny --locked check
	cd ../conformance/runner/rust && $(CARGO) deny --locked --config ../../../rust/deny.toml check

# Everything `cargo publish` does short of the upload: package the crate, build it from the
# tarball, check the metadata, so a missing file, a path dependency or a metadata slip
# surfaces on the pull request rather than on the tag. --allow-dirty so it runs before a
# commit as well as after; CI's checkout is clean anyway.
# The dry run unpacks the crate under target/package to build it once more from the
# tarball. That copy carries the crate's tests/ directory, and rust-cache's cleanup probes
# every tests/ it finds in target/ for the nested target dirs that trybuild and macrotest
# make, reporting each miss as an error annotation on the job. The tarball is what matters;
# the unpacked copy goes as soon as it has been built.
.PHONY: publish-check
publish-check:
	$(CARGO) publish -p hey-sdk --dry-run --locked --allow-dirty
	rm -rf target/package/hey-sdk-*/

# Run all checks: the same steps as CI's test-rust job, in the same order
.PHONY: check
check: fmt-check lint test doc no-default-features examples deny publish-check

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
	@echo "  examples             Build the examples"
	@echo "  deny                 cargo deny over both workspaces (advisories, licences, bans)"
	@echo "  publish-check        cargo publish --dry-run: package, build from the tarball, check metadata"
	@echo "  check                Every step CI's test-rust job runs, in order"
	@echo "  clean                Remove build artifacts"
