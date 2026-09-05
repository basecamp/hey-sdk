#!/usr/bin/env bash
# Bumps the SDK version in every language: go/pkg/hey/version.go and rust/hey-sdk/Cargo.toml.
# Usage: scripts/bump-version.sh <version>
# Example: scripts/bump-version.sh 0.3.0
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>" >&2
  echo "Example: $0 0.3.0" >&2
  exit 1
fi

# Validate semver format (strict)
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "ERROR: Version must be semver (e.g., 0.3.0)" >&2
  exit 1
fi

echo "Bumping version to: $VERSION"

VERSION_FILE="$REPO_ROOT/go/pkg/hey/version.go"
sedi "s/^const Version = \".*\"/const Version = \"$VERSION\"/" "$VERSION_FILE"

if ! grep -Fq "const Version = \"$VERSION\"" "$VERSION_FILE"; then
  echo "ERROR: Version substitution did not match in $VERSION_FILE" >&2
  exit 1
fi

CARGO_FILE="$REPO_ROOT/rust/hey-sdk/Cargo.toml"
sedi "s/^version = \".*\"/version = \"$VERSION\"/" "$CARGO_FILE"

if ! grep -Fxq "version = \"$VERSION\"" "$CARGO_FILE"; then
  echo "ERROR: Version substitution did not match in $CARGO_FILE" >&2
  exit 1
fi

# Cargo.lock records the package version too; refresh it so a --locked build still agrees.
if command -v cargo >/dev/null 2>&1; then
  (cd "$REPO_ROOT/rust" && cargo update -q -w --offline) || true
  (cd "$REPO_ROOT/conformance/runner/rust" && cargo update -q -w --offline) || true
fi

echo "Done. Bumped 2 files to $VERSION."
