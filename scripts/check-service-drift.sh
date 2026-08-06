#!/usr/bin/env bash
# check-service-drift.sh
#
# Compares generated client operations against service layer usage.
# Detects drift between the OpenAPI spec and the Go service layer wrapper.
#
# Run after: make go-generate
# Exit codes:
#   0 = No drift detected
#   1 = Drift detected (service calls non-existent operations)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SDK_DIR="$(dirname "$SCRIPT_DIR")/go"

GENERATED_FILE="$SDK_DIR/pkg/generated/client.gen.go"
SERVICE_DIR="$SDK_DIR/pkg/hey"

if [ ! -f "$GENERATED_FILE" ]; then
  echo "ERROR: Generated client not found at $GENERATED_FILE"
  echo "Run 'make go-generate' first."
  exit 1
fi

GEN_OPS=$(mktemp)
SVC_OPS=$(mktemp)
GEN_FORM_OPS=$(mktemp)
SVC_FORM_OPS=$(mktemp)
WRAPPED_FORM_OPS=$(mktemp)
trap 'rm -f "$GEN_OPS" "$SVC_OPS" "$GEN_FORM_OPS" "$SVC_FORM_OPS" "$WRAPPED_FORM_OPS"' EXIT

# Normalize generated method variants to their Smithy operation name.
# e.g., CreateMessageWithBodyWithResponse -> CreateMessage
#       CreateReplyDraftWithFormdataBodyWithResponse -> CreateReplyDraft
#       ListBoxesWithResponse -> ListBoxes
normalize_operations() {
  sed -E 's/WithResponse$//' \
    | sed -E 's/With[A-Za-z0-9]*Body$//'
}

# Extract generated operations. Generic and typed body variants collapse to
# the same operation after normalization.
grep "^func (c \*ClientWithResponses)" "$GENERATED_FILE" 2>/dev/null \
  | sed -E 's/^func \(c \*ClientWithResponses\) ([A-Za-z0-9_]+).*/\1/' \
  | normalize_operations \
  | sort -u > "$GEN_OPS"

# Record operations with a typed form transport separately. Operation-level
# normalization remains useful for coverage, but service wrappers for these
# operations must not silently switch to the generic WithBody helper.
grep "^func (c \*ClientWithResponses)" "$GENERATED_FILE" 2>/dev/null \
  | sed -E 's/^func \(c \*ClientWithResponses\) ([A-Za-z0-9_]+).*/\1/' \
  | sed -nE 's/^([A-Za-z0-9_]+)WithFormdataBodyWithResponse$/\1/p' \
  | sort -u > "$GEN_FORM_OPS"

# Extract generated-client calls made by the service layer (excluding tests).
# Some form operations intentionally use the raw WithFormdataBody method so
# they can inspect 302/303 responses without the generated response parser.
for f in "$SERVICE_DIR"/*.go; do
  case "$f" in
    *_test.go) continue ;;
  esac
  grep -oE '\.gen\.[A-Za-z0-9_]+' "$f" 2>/dev/null || true
done | sed -E 's/^\.gen\.//' \
  | normalize_operations \
  | sort -u > "$SVC_OPS"

for f in "$SERVICE_DIR"/*.go; do
  case "$f" in
    *_test.go) continue ;;
  esac
  grep -oE '\.gen\.[A-Za-z0-9_]+' "$f" 2>/dev/null || true
done | sed -E 's/^\.gen\.//' \
  | sed -nE 's/^([A-Za-z0-9_]+)WithFormdataBody(WithResponse)?$/\1/p' \
  | sort -u > "$SVC_FORM_OPS"

comm -12 "$GEN_FORM_OPS" "$SVC_OPS" > "$WRAPPED_FORM_OPS"

GEN_COUNT=$(wc -l < "$GEN_OPS" | tr -d ' ')
SVC_COUNT=$(wc -l < "$SVC_OPS" | tr -d ' ')

echo "Generated client operations: $GEN_COUNT"
echo "Service layer wrapped operations: $SVC_COUNT"
echo ""

# Operations in generated but not wrapped (informational only)
UNWRAPPED=$(comm -23 "$GEN_OPS" "$SVC_OPS")
UNWRAPPED_COUNT=$(echo "$UNWRAPPED" | grep -c . || true)

# Service calls to non-existent operations (failure)
MISSING=$(comm -13 "$GEN_OPS" "$SVC_OPS")
MISSING_COUNT=$(echo "$MISSING" | grep -c . || true)

# Form operations wrapped by the service layer must use their typed form
# helper so browser media type and encoding cannot drift to generic bodies.
MISSING_FORM=$(comm -23 "$WRAPPED_FORM_OPS" "$SVC_FORM_OPS")
MISSING_FORM_COUNT=$(echo "$MISSING_FORM" | grep -c . || true)

HAS_DRIFT=0

if [ "$UNWRAPPED_COUNT" -gt 0 ]; then
  echo "=== Generated operations NOT YET wrapped by service layer ($UNWRAPPED_COUNT) ==="
  echo "$UNWRAPPED"
  echo ""
fi

if [ "$MISSING_COUNT" -gt 0 ]; then
  echo "=== ERROR: Service calls to NON-EXISTENT generated operations ($MISSING_COUNT) ==="
  echo "$MISSING"
  echo ""
  echo "These service methods call generated operations that don't exist."
  echo "Either the spec is missing these operations, or there's a typo in the service layer."
  HAS_DRIFT=1
fi

if [ "$MISSING_FORM_COUNT" -gt 0 ]; then
  echo "=== ERROR: FORM OPERATIONS USING NON-FORM SERVICE TRANSPORT ($MISSING_FORM_COUNT) ==="
  echo "$MISSING_FORM"
  echo ""
  echo "These service wrappers must call their generated WithFormdataBody helper."
  HAS_DRIFT=1
fi

if [ "$GEN_COUNT" -eq 0 ]; then
  echo "ERROR: No generated operations found. Check GENERATED_FILE path or parsing."
  exit 1
fi

COVERAGE_PCT=$((SVC_COUNT * 100 / GEN_COUNT))
echo "Coverage: $SVC_COUNT / $GEN_COUNT operations ($COVERAGE_PCT%)"

if [ "$HAS_DRIFT" -eq 1 ]; then
  echo ""
  echo "DRIFT DETECTED - Fix the issues above before proceeding."
  exit 1
fi

echo ""
echo "No critical drift detected."
exit 0
