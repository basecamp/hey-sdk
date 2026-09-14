#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
state_file=${1:-"$repo_root/.github/kotlin-publish-enabled"}
state=$(cat "$state_file")

case "$state" in
  true|false) printf '%s\n' "$state" ;;
  *)
    echo "Kotlin publish state must be exactly true or false: $state_file" >&2
    exit 1
    ;;
esac
