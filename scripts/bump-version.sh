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

# The READMEs show a consumer two ways in: the crates.io requirement, which names the minor
# (Cargo reads "0.31" as ">=0.31.0, <0.32.0"), and the git dependency pinned to the release
# tag, which names the whole version. Both move with the bump, and a README that no longer
# carries either line is a README the bump cannot keep true, so each substitution is checked.
MINOR="${VERSION%.*}"
for README in "$REPO_ROOT/README.md" "$REPO_ROOT/rust/hey-sdk/README.md"; do
  sedi "s|^hey-sdk = \"[0-9.]*\"$|hey-sdk = \"$MINOR\"|" "$README"
  if ! grep -Fxq "hey-sdk = \"$MINOR\"" "$README"; then
    echo "ERROR: crates.io requirement substitution did not match in $README" >&2
    exit 1
  fi
  sedi "s|\(hey-sdk = { git = \"https://github.com/basecamp/hey-sdk\", tag = \"v\)[0-9.]*\"|\1$VERSION\"|" "$README"
  if ! grep -Fq "hey-sdk = { git = \"https://github.com/basecamp/hey-sdk\", tag = \"v$VERSION\" }" "$README"; then
    echo "ERROR: git tag substitution did not match in $README" >&2
    exit 1
  fi
done

# Cargo.lock records the package version too, and both lockfiles are checked in, so a
# --locked build only agrees once they carry the new one. A bump that cannot refresh them
# is a bump that breaks the build, so cargo is required and neither update may fail.
if ! command -v cargo >/dev/null 2>&1; then
  echo "ERROR: cargo is required to refresh the Rust lockfiles" >&2
  exit 1
fi

(cd "$REPO_ROOT/rust" && cargo update -q -w --offline)
(cd "$REPO_ROOT/conformance/runner/rust" && cargo update -q -w --offline)

echo "Done. Bumped version.go, Cargo.toml and the READMEs to $VERSION."
