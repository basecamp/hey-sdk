#!/usr/bin/env bash
# enhance-openapi-go-types.sh — Post-process openapi.json with Go-specific type hints
#
# Adds x-go-type annotations for oapi-codegen:
#   - *_at fields → time.Time
#   - *_on fields → types.Date
#   - Optional booleans in request bodies → *bool (nil vs false distinction)
set -euo pipefail

OPENAPI_FILE="${1:-openapi.json}"

if ! command -v jq &> /dev/null; then
    echo "ERROR: jq is required but not installed."
    exit 1
fi

# Pass 1: Time and date type hints for all properties
jq '
  # Walk all properties in schemas
  (.components.schemas // {}) |= with_entries(
    .value |= (
      if .properties then
        .properties |= with_entries(
          # Timestamp fields: *_at → time.Time
          if (.key | test("_at$")) and (.value.type == "string") then
            .value += {"x-go-type": "time.Time", "x-go-type-import": {"path": "time"}}
          # Date fields: *_on → types.Date
          elif (.key | test("_on$")) and (.value.type == "string") then
            .value += {"x-go-type": "types.Date", "x-go-type-import": {"path": "github.com/basecamp/hey-sdk/go/pkg/types"}}
          else .
          end
        )
      else .
      end
    )
  )
' "$OPENAPI_FILE" > "${OPENAPI_FILE}.tmp" && mv "${OPENAPI_FILE}.tmp" "$OPENAPI_FILE"

# Pass 2: Optional booleans and timestamps in request schemas → pointers
# Without this, Go's JSON encoder sends zero-valued time.Time as "0001-01-01T00:00:00Z"
# and false booleans even when the caller didn't set them — `omitempty` does not omit a
# struct or a false bool. That is how a partial update (e.g. stopping a time track by
# sending only ends_at) ends up rewriting starts_at on the server.
#
# Applies to every component schema reachable by $ref from a request body
# (RequestContent, nested *Payload, and anything they reference), for every property
# that pass 1 or the format turns into time.Time / types.Date, plus booleans.
# Smithy always emits request bodies as $ref to a component schema, so inline
# requestBody schemas do not occur here and are not walked.
jq '
  def refname: sub("^#/components/schemas/"; "");
  . as $root
  | [ .paths[] | .[] | objects | .requestBody? // empty | .. | .["$ref"]? // empty | refname ] | unique as $seeds
  | def expand($set):
      ($set + [ $set[] | $root.components.schemas[.] | .. | .["$ref"]? // empty | refname ] | unique) as $n
      | if ($n | length) == ($set | length) then $set else expand($n) end;
    expand($seeds) as $request_schemas
  | $root
  | (.components.schemas // {}) |= with_entries(
      if (.key as $k | $request_schemas | index($k)) then
        .value |= (
          if .properties then
            .properties |= with_entries(
              if .value.type == "boolean" then
                .value += {"x-go-type-skip-optional-pointer": false}
              elif (.value.type == "string" and (.value.format == "date-time" or .value.format == "date")) then
                .value += {"x-go-type-skip-optional-pointer": false}
              elif (.value.type == "string" and (.key | test("_at$|_on$"))) then
                .value += {"x-go-type-skip-optional-pointer": false}
              else .
              end
            )
          else .
          end
        )
      else .
      end
    )
' "$OPENAPI_FILE" > "${OPENAPI_FILE}.tmp" && mv "${OPENAPI_FILE}.tmp" "$OPENAPI_FILE"

# Pass 4: Optional query parameters → pointers
# oapi-codegen emits every non-pointer query field unconditionally, so an optional
# param the caller did not set still goes on the wire as its zero value: page=,
# limit=0, folder_id=0, confirm_destroy=. The last two are not harmless (unfile from
# folder 0; skip the shared-topic trash confirmation). Pointers are omitted when nil.
jq '
  .paths |= with_entries(
    .value |= with_entries(
      if (.key | test("^(get|post|put|patch|delete)$")) and (((.value.parameters // []) | length) > 0) then
        .value.parameters |= map(
          if .in == "query" and ((.required // false) | not) then
            .schema += {"x-go-type-skip-optional-pointer": false}
          else .
          end
        )
      else .
      end
    )
  )
' "$OPENAPI_FILE" > "${OPENAPI_FILE}.tmp" && mv "${OPENAPI_FILE}.tmp" "$OPENAPI_FILE"

# Pass 3: Nothing here — self-referential types are fixed via post-codegen sed
# in go/Makefile (oapi-codegen ignores type overrides on bare $ref properties).

echo "Enhanced $OPENAPI_FILE with Go type annotations"
