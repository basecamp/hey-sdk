# Shared helpers for hey-sdk scripts.
# Source this file; do not execute directly.

# Resolve repo root from the sourcing script's location.
# BASH_SOURCE[1] is the caller when sourced; fall back to BASH_SOURCE[0] (this file).
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[1]:-${BASH_SOURCE[0]}}")/.." && pwd)"

# Portable in-place sed: use temp file instead of -i flag.
sedi() {
  local expr="$1" file="$2"
  local tmp
  tmp=$(mktemp)
  trap 'rm -f "$tmp"' RETURN
  sed "$expr" "$file" > "$tmp" && cat "$tmp" > "$file"
}
