#!/bin/bash
set -e

DB_NAME=$(basename "$PWD")
CODEQL_DIR="codeql"
SUITE="${1:-code-scanning}"

printf "=== Running CodeQL security analysis ===\n"
printf "Suite: %s\n\n" "$SUITE"

for lang in go python javascript; do
  if [ ! -d "$CODEQL_DIR/${DB_NAME}-${lang}" ]; then
    echo "Error: Database $CODEQL_DIR/${DB_NAME}-${lang} not found"
    echo "Run ./scripts/codeql-build-all.sh first"
    exit 1
  fi

done

for lang in go python javascript; do
  echo "Analyzing $lang..."

  SUITE_PATH=$(find "$CODEQL_DIR/packs/codeql/${lang}-queries" -name "${lang}-${SUITE}.qls" 2>/dev/null | head -1)

  if [ -z "$SUITE_PATH" ]; then
    echo "  [WARN] Suite $SUITE not found for $lang, using default"
    SUITE_PATH="$CODEQL_DIR/packs/codeql/${lang}-queries"
  fi

  codeql database analyze "$CODEQL_DIR/${DB_NAME}-${lang}" \
    "$SUITE_PATH" \
    --format=sarif-latest \
    --output="$CODEQL_DIR/${DB_NAME}-${lang}-${SUITE}.sarif" \
    2>&1 | grep -E "(Analyzing|Running|Compiling|Evaluating|Writing)" || true

  echo "  [OK] Results: $CODEQL_DIR/${DB_NAME}-${lang}-${SUITE}.sarif"
done

echo ""
echo "=== Findings Summary ===\n"
printf "%-12s %8s %8s %8s\n" "Language" "Total" "Error" "Warning"
printf "%-12s %8s %8s %8s\n" "--------" "-----" "-----" "-------"

for lang in go python javascript; do
  sarif="$CODEQL_DIR/${DB_NAME}-${lang}-${SUITE}.sarif"
  if [ -f "$sarif" ]; then
    total=$(jq '.runs[0].results | length' "$sarif" 2>/dev/null || echo "0")
    errors=$(jq '[.runs[0].results[] | select(.level == "error")] | length' "$sarif" 2>/dev/null || echo "0")
    warnings=$(jq '[.runs[0].results[] | select(.level == "warning")] | length' "$sarif" 2>/dev/null || echo "0")
    printf "%-12s %8s %8s %8s\n" "$lang" "$total" "$errors" "$warnings"
  else
    printf "%-12s %8s\n" "$lang" "N/A"
  fi
done

echo ""
echo "View results:"
echo "  - VS Code: Install SARIF Viewer extension"
echo "  - CLI: jq '.runs[0].results[]' ${CODEQL_DIR}/${DB_NAME}-<lang>-${SUITE}.sarif"
