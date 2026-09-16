.PHONY: test check build clean

# Run every case under conformance/tests against the Swift SDK.
test:
	swift run ConformanceRunner ../../tests

# The runner's own tests: the assertions, the fixture reader and the mock, and the SDK's
# transport driven over a real loopback connection.
check:
	swift build --build-tests -Xswiftc -warnings-as-errors
	swift test -Xswiftc -warnings-as-errors

build:
	swift build

clean:
	swift package clean
