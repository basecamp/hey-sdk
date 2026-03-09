#!/usr/bin/env bash
# Syncs API_VERSION constants from openapi.json info.version to Go SDK.
# Usage: scripts/sync-api-version.sh [--check] [openapi.json]
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

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

VERSION_FILE="$REPO_ROOT/go/pkg/hey/version.go"
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

echo "Syncing API version: $API_VERSION"

ESCAPED_VERSION=$(printf '%s\n' "$API_VERSION" | sed 's/[&/\]/\\&/g')
sedi "s/^const APIVersion = \".*\"/const APIVersion = \"$ESCAPED_VERSION\"/" "$VERSION_FILE"

if ! grep -q "const APIVersion = \"$API_VERSION\"" "$VERSION_FILE"; then
  echo "ERROR: API version substitution did not match in $VERSION_FILE" >&2
  exit 1
fi

echo "Done."
