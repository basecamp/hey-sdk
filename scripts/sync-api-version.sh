#!/usr/bin/env bash
# Syncs API_VERSION constants from openapi.json info.version to Go SDK.
# Usage: scripts/sync-api-version.sh [--check] [openapi.json]
set -euo pipefail

CHECK=false
OPENAPI="openapi.json"

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

VERSION_FILE="go/pkg/hey/version.go"
CURRENT=$(sed -n 's/^const APIVersion = "\(.*\)"/\1/p' "$VERSION_FILE")

if [ "$CHECK" = true ]; then
  if [ "$CURRENT" = "$API_VERSION" ]; then
    echo "API version is in sync: $API_VERSION"
    exit 0
  else
    echo "ERROR: API version mismatch. openapi.json=$API_VERSION, version.go=$CURRENT" >&2
    echo "Run 'make sync-api-version' to fix." >&2
    exit 1
  fi
fi

# Portable in-place sed: use temp file instead of -i flag
sedi() {
  local expr="$1" file="$2"
  local tmp
  tmp=$(mktemp)
  sed "$expr" "$file" > "$tmp" && cat "$tmp" > "$file" && rm "$tmp"
}

echo "Syncing API version: $API_VERSION"

# Go
sedi "s/^const APIVersion = \".*\"/const APIVersion = \"$API_VERSION\"/" "$VERSION_FILE"

echo "Done."
