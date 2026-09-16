.PHONY: build test check check-drift generate clean help

# Build the library, the generator and the tests.
build:
	swift build --build-tests

# Run the library's and the generator's tests.
test:
	swift test

# Everything CI's Swift job runs: the build with warnings as errors, then the tests.
check:
	swift build --build-tests -Xswiftc -warnings-as-errors
	swift test -Xswiftc -warnings-as-errors

# Regenerate Sources/Hey/Generated from openapi.json, behavior-model.json and names.toml.
generate:
	swift run HeyGenerator --root ..

# Fail when Sources/Hey/Generated is not what the generator would write.
check-drift:
	swift run HeyGenerator --root .. --check

clean:
	swift package clean

help:
	@echo "  build        Build the library, generator and tests"
	@echo "  test         Run the tests"
	@echo "  check        Build with warnings as errors and run the tests"
	@echo "  generate     Regenerate Sources/Hey/Generated"
	@echo "  check-drift  Fail when the generated code is stale"
