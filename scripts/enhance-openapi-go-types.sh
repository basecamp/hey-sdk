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

# Pass 2: Optional booleans in RequestContent schemas → *bool
jq '
  (.components.schemas // {}) |= with_entries(
    if (.key | test("RequestContent$")) then
      .value |= (
        if .properties then
          .properties |= with_entries(
            if .value.type == "boolean" then
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

echo "Enhanced $OPENAPI_FILE with Go type annotations"
