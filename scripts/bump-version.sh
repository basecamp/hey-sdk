#!/usr/bin/env bash
# Bumps SDK version across Go implementation.
# Usage: scripts/bump-version.sh <version>
# Example: scripts/bump-version.sh 0.3.0
set -euo pipefail

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

# Portable in-place sed: use temp file instead of -i flag
sedi() {
  local expr="$1" file="$2"
  local tmp
  tmp=$(mktemp)
  sed "$expr" "$file" > "$tmp" && cat "$tmp" > "$file" && rm "$tmp"
}

echo "Bumping version to: $VERSION"

# Go
sedi "s/^const Version = \".*\"/const Version = \"$VERSION\"/" go/pkg/hey/version.go

echo "Done. Bumped 1 file to $VERSION."
