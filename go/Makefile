# HEY Go SDK Makefile

# Go parameters
GOCMD := go
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := gofmt
GOMOD := $(GOCMD) mod
GOGEN := $(GOCMD) generate

# Default target
.PHONY: all
all: check

# Verify Go toolchain coherence: the `go` binary and GOROOT must be from the
# same installation. Mismatches (e.g. go1.25 binary with go1.24 GOROOT) cause
# cryptic compile errors.
.PHONY: toolchain-check
toolchain-check:
	@PATH_GO=$$(realpath "$$(which go)"); \
	ROOT_GO=$$(realpath "$$(go env GOROOT)/bin/go"); \
	if [ "$$PATH_GO" != "$$ROOT_GO" ]; then \
		echo "ERROR: Go toolchain mismatch detected."; \
		echo "  PATH go:  $$PATH_GO ($$(go version))"; \
		echo "  GOROOT go: $$ROOT_GO"; \
		echo ""; \
		echo "Ensure PATH and GOROOT point to the same Go installation."; \
		echo "If using mise: eval \"\$$(mise hook-env)\""; \
		exit 1; \
	fi

# Generate code from OpenAPI spec
.PHONY: generate
generate:
	@echo "Generating client code from OpenAPI spec..."
	$(GOGEN) ./...
	@echo "Fixing recursive types in generated code..."
	@sed -i.bak 's/Parent   Recording/Parent   *Recording/' pkg/generated/client.gen.go && rm pkg/generated/client.gen.go.bak
	@$(GOFMT) -s -w pkg/generated/client.gen.go

# Run tests
.PHONY: test
test:
	$(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run go vet
.PHONY: vet
vet:
	$(GOVET) ./...

# Format code
.PHONY: fmt
fmt:
	$(GOFMT) -s -w .

# Check formatting (for CI)
.PHONY: fmt-check
fmt-check:
	@test -z "$$($(GOFMT) -s -l . | tee /dev/stderr)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)

# Run linter (requires golangci-lint)
.PHONY: lint
lint:
	golangci-lint run ./...

# Tidy dependencies
.PHONY: tidy
tidy:
	$(GOMOD) tidy

# Verify dependencies
.PHONY: verify
verify:
	$(GOMOD) verify

# Clean build artifacts
.PHONY: clean
clean:
	rm -f coverage.out coverage.html
	rm -f pkg/generated/*.gen.go

# Run all checks (local development gate)
.PHONY: check
check: toolchain-check fmt-check vet lint test

# CI target (same as check, explicit for clarity)
.PHONY: ci
ci: check

# Show help
.PHONY: help
help:
	@echo "HEY Go SDK Makefile targets:"
	@echo ""
	@echo "Code Generation:"
	@echo "  generate       Generate client code from OpenAPI spec using oapi-codegen"
	@echo ""
	@echo "Testing:"
	@echo "  test           Run all tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo ""
	@echo "Code Quality:"
	@echo "  vet            Run go vet"
	@echo "  fmt            Format code"
	@echo "  fmt-check      Check code formatting"
	@echo "  lint           Run golangci-lint"
	@echo ""
	@echo "Dependencies:"
	@echo "  tidy           Tidy go.mod dependencies"
	@echo "  verify         Verify dependencies"
	@echo ""
	@echo "CI/Release:"
	@echo "  check          Run all checks (fmt-check, vet, lint, test)"
	@echo "  ci             Run CI checks (check)"
	@echo ""
	@echo "Other:"
	@echo "  clean          Remove generated artifacts"
	@echo "  help           Show this help"
