#!/usr/bin/env bash
# enhance-openapi-go-types.sh — Post-process openapi.json with Go-specific type hints
#
# Adds x-go-type annotations for oapi-codegen:
#   - *_at fields → time.Time
#   - *_on fields → types.Date
#   - Optional booleans in request bodies → *bool (nil vs false distinction)
set -euo pipefail

OPENAPI_FILE="${1:-openapi.json}"
TMP_FILE="${OPENAPI_FILE}.tmp.$$"
trap 'rm -f "$TMP_FILE"' EXIT

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
' "$OPENAPI_FILE" > "$TMP_FILE"
chmod 0644 "$TMP_FILE"
mv "$TMP_FILE" "$OPENAPI_FILE"

# Pass 2: Optional booleans and timestamps in RequestContent schemas → pointers
# Without this, Go's JSON encoder sends zero-valued time.Time as "0001-01-01T00:00:00Z"
# and false booleans even when the caller didn't set them.
jq '
  (.components.schemas // {}) |= with_entries(
    if (.key | test("RequestContent$")) then
      .value |= (
        if .properties then
          .properties |= with_entries(
            if .value.type == "boolean" then
              .value += {"x-go-type-skip-optional-pointer": false}
            elif (.value.type == "string" and .value.format == "date-time") then
              .value += {"x-go-type-skip-optional-pointer": false}
            else .
            end
          )
        else .
        end
      ) |
      if (.key == "CreateReplyRequestContent" and (.value.properties.entry["$ref"] // "") != "") then
        .value.properties.entry = {
          "allOf": [{"$ref": .value.properties.entry["$ref"]}],
          "x-go-type-skip-optional-pointer": false
        }
      else .
      end
    else .
    end
  )
' "$OPENAPI_FILE" > "$TMP_FILE"
chmod 0644 "$TMP_FILE"
mv "$TMP_FILE" "$OPENAPI_FILE"

# Pass 3: Convert Smithy-modeled form operations from restJson1's JSON media
# type to application/x-www-form-urlencoded. Inline the top-level component so
# oapi-codegen emits form tags; the schema itself remains generated from Smithy.
jq '
  . as $root |
  (.paths // {}) |= with_entries(
    .value |= with_entries(
      if (.key | test("^(get|put|post|delete|options|head|patch|trace)$")) then
        .value |= (
          if (.["x-hey-form-urlencoded"] != null and .requestBody.content["application/json"] != null) then
            .requestBody.content = {
              "application/x-www-form-urlencoded": (
                .requestBody.content["application/json"] as $body |
                if ($body.schema["$ref"] // "") | startswith("#/components/schemas/") then
                  ($body.schema["$ref"] | split("/") | last) as $schema_name |
                  $body + {schema: $root.components.schemas[$schema_name]}
                else
                  $body
                end
              )
            }
          else .
          end |
          if (([.requestBody.content[]?.schema.properties[]? | has("x-hey-sensitive")] | any)
              or ([.parameters[]? | (has("x-hey-sensitive") or ((.schema // {}) | has("x-hey-sensitive")))] | any)) then
            . + {"x-hey-sensitive": true}
          else .
          end
        )
      else
        .
      end
    )
  )
' "$OPENAPI_FILE" > "$TMP_FILE"
chmod 0644 "$TMP_FILE"
mv "$TMP_FILE" "$OPENAPI_FILE"

# Pass 4: Nothing here — self-referential types are fixed via post-codegen sed
# in go/Makefile (oapi-codegen ignores type overrides on bare $ref properties).

trap - EXIT
echo "Enhanced $OPENAPI_FILE with Go type annotations"
