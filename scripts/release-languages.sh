#!/usr/bin/env bash
set -euo pipefail

repo_root=${1:-.}
event_name=${2:-${GITHUB_EVENT_NAME:-}}
state_file="$repo_root/.github/typescript-publish-enabled"
state_helper="$repo_root/scripts/typescript-publish-state.sh"

# The Kotlin library joins the list only when the target's own switch says so, read the
# same way as TypeScript's: a target from before the switch existed has no Kotlin release
# to wait for, and a switch without its helper, or a helper without its switch, is a
# broken target rather than a quiet one.
kotlin_state_file="$repo_root/.github/kotlin-publish-enabled"
kotlin_state_helper="$repo_root/scripts/kotlin-publish-state.sh"
kotlin=""
if [[ -f "$kotlin_state_file" && -f "$kotlin_state_helper" ]]; then
  kotlin_state=$(bash "$kotlin_state_helper" "$kotlin_state_file")
  case "$kotlin_state" in
    true) kotlin=",kotlin" ;;
    false) kotlin="" ;;
    *)
      echo "Kotlin publish helper must print exactly true or false" >&2
      exit 1
      ;;
  esac
elif [[ -e "$kotlin_state_file" || -e "$kotlin_state_helper" ]]; then
  echo "Kotlin publish state and helper must both be present" >&2
  exit 1
fi

# Swift has no registry and no switch: a tag is the release, so the Swift workflow is waited
# for whenever the target has one. A target from before it existed has none to wait for.
swift=""
if [[ -f "$repo_root/.github/workflows/release-swift.yml" ]]; then
  swift=",swift"
fi

if [[ -f "$state_file" && -f "$state_helper" ]]; then
  state=$(bash "$state_helper" "$state_file")
  case "$state" in
    true) printf '%s\n' "go,rust,typescript$kotlin$swift" ;;
    false) printf '%s\n' "go,rust$kotlin$swift" ;;
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
