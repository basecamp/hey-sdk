#!/usr/bin/env bash
# Syncs API_VERSION constants from openapi.json info.version to the Go, TypeScript and Kotlin SDKs.
# Usage: scripts/sync-api-version.sh [--check] [openapi.json]
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

CHECK=false
OPENAPI="$REPO_ROOT/openapi.json"

for arg in "$@"; do
  case "$arg" in
    --check) CHECK=true ;;
    *) OPENAPI="$arg" ;;
  esac
done

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required" >&2
  exit 1
fi

API_VERSION=$(jq -r '.info.version' "$OPENAPI")
if [ -z "$API_VERSION" ] || [ "$API_VERSION" = "null" ]; then
  echo "ERROR: Could not read info.version from $OPENAPI" >&2
  exit 1
fi

VERSION_FILE="$REPO_ROOT/go/pkg/hey/version.go"
KOTLIN_FILE="$REPO_ROOT/kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/HeyConfig.kt"
KOTLIN_CURRENT=$(sed -n 's/^        const val API_VERSION = "\(.*\)"/\1/p' "$KOTLIN_FILE")

if [ "$CHECK" = true ]; then
  node "$REPO_ROOT/scripts/sync-typescript-versions.mjs" --check --api-version "$API_VERSION"
  if [ "$KOTLIN_CURRENT" != "$API_VERSION" ]; then
    echo "ERROR: API version mismatch. openapi.json=$API_VERSION, HeyConfig.kt=$KOTLIN_CURRENT" >&2
    echo "Run 'make sync-api-version' to fix." >&2
    exit 1
  fi
  echo "API version is in sync: $API_VERSION"
  exit 0
fi

echo "Syncing API version: $API_VERSION"

# One transaction updates the Go and TypeScript constants after every target
# has been read and validated.
node "$REPO_ROOT/scripts/sync-typescript-versions.mjs" --api-version "$API_VERSION"

if ! grep -Fq "const APIVersion = \"$API_VERSION\"" "$VERSION_FILE"; then
  echo "ERROR: API version synchronization did not update $VERSION_FILE" >&2
  exit 1
fi

ESCAPED_VERSION=$(printf '%s\n' "$API_VERSION" | sed 's/[&/\]/\\&/g')
sedi "s/^        const val API_VERSION = \".*\"/        const val API_VERSION = \"$ESCAPED_VERSION\"/" "$KOTLIN_FILE"
if ! grep -Fq "const val API_VERSION = \"$API_VERSION\"" "$KOTLIN_FILE"; then
  echo "ERROR: API version substitution did not match in $KOTLIN_FILE" >&2
  exit 1
fi

echo "Done."
