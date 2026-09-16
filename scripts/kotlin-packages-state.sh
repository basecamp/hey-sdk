#!/usr/bin/env bash
# Whether a Kotlin release is on GitHub Packages, file by file.
# Usage: scripts/kotlin-packages-state.sh <staging-repo> <repository-url>
#
# A Maven version is several files — two artifacts, each with a jar, a pom, module
# metadata and sources — and GitHub Packages refuses to overwrite any of them, so a run
# that failed part-way leaves a version that publishing again can neither finish nor
# replace. Given the publication as Gradle laid it out locally, this fetches each of its
# files from the remote and prints `absent` when none is there (publish), `published` when
# every one is there byte for byte (a re-run of a finished release: nothing to do), and
# fails naming the files otherwise.
set -euo pipefail

staging=${1:?usage: $0 <staging-repo> <repository-url>}
repository=${2:?usage: $0 <staging-repo> <repository-url>}
: "${GITHUB_USER:?GITHUB_USER is required}"
: "${GITHUB_ACCESS_TOKEN:?GITHUB_ACCESS_TOKEN is required}"

# The files the version is made of: everything staged except the checksum sidecars and the
# per-artifact maven-metadata.xml, which the repository writes for itself.
files=$(cd "$staging" && find . -type f \
  ! -name '*.md5' ! -name '*.sha1' ! -name '*.sha256' ! -name '*.sha512' \
  ! -name 'maven-metadata.xml' | sed 's|^\./||' | sort)
if [ -z "$files" ]; then
  echo "ERROR: nothing staged under $staging" >&2
  exit 1
fi

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

total=0 present=0 missing=0
report=""
while IFS= read -r file; do
  total=$((total + 1))
  url="${repository%/}/$file"
  # curl drops the credentials on a redirect to another host, which is where GitHub
  # Packages sends a download; --location-trusted would be wrong here.
  status=$(curl -sS -L -o "$scratch/remote" -w '%{http_code}' \
    -u "$GITHUB_USER:$GITHUB_ACCESS_TOKEN" "$url")
  case "$status" in
    200)
      if cmp -s "$staging/$file" "$scratch/remote"; then
        present=$((present + 1))
        report="$report  present    $file"$'\n'
      else
        report="$report  different  $file"$'\n'
      fi
      ;;
    404)
      missing=$((missing + 1))
      report="$report  missing    $file"$'\n'
      ;;
    *)
      echo "ERROR: $url answered $status; not publishing on a guess" >&2
      exit 1
      ;;
  esac
done <<< "$files"

if [ "$missing" -eq "$total" ]; then
  echo absent
elif [ "$present" -eq "$total" ]; then
  echo published
else
  {
    echo "ERROR: GitHub Packages holds part of this version, or holds it differently:"
    printf '%s' "$report"
    echo "A Maven version there can neither be finished nor overwritten. Delete the version from"
    echo "the com.basecamp.hey-sdk and com.basecamp.hey-sdk-jvm packages on GitHub, then re-run."
  } >&2
  exit 1
fi
