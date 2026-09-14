#!/usr/bin/env bash
set -euo pipefail

repo_root=${1:-.}
event_name=${2:-${GITHUB_EVENT_NAME:-}}
state_file="$repo_root/.github/typescript-publish-enabled"
state_helper="$repo_root/scripts/typescript-publish-state.sh"

# The Kotlin library publishes to GitHub Packages with the workflow's own token, so it
# joins the list whenever its release workflow is in the target.
kotlin=""
if [[ -f "$repo_root/.github/workflows/release-kotlin.yml" ]]; then
  kotlin=",kotlin"
fi

if [[ -f "$state_file" && -f "$state_helper" ]]; then
  state=$(bash "$state_helper" "$state_file")
  case "$state" in
    true) printf '%s\n' "go,rust,typescript$kotlin" ;;
    false) printf '%s\n' "go,rust$kotlin" ;;
    *)
      echo "TypeScript publish helper must print exactly true or false" >&2
      exit 1
      ;;
  esac
elif [[ ! -e "$state_file" && ! -e "$state_helper" && "$event_name" == "workflow_dispatch" ]]; then
  if [[ -f "$repo_root/.github/workflows/release-rust.yml" ]]; then
    printf '%s\n' 'go,rust'
  else
    printf '%s\n' 'go'
  fi
else
  echo "TypeScript publish state and helper must both be present" >&2
  exit 1
fi
