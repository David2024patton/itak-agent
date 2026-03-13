#!/bin/bash
set -e

DB_NAME=$(basename "$PWD")
CODEQL_DIR="codeql"
GO_SRC_ROOT="${GO_SRC_ROOT:-.}"
PYTHON_SRC_ROOT="${PYTHON_SRC_ROOT:-.}"
JS_SRC_ROOT="${JS_SRC_ROOT:-.}"

printf "=== Building CodeQL databases ===\n"
printf "Database name prefix: %s\n\n" "$DB_NAME"

mkdir -p "$CODEQL_DIR/packs"

# Build databases for each language
for lang in go python javascript; do
  src_root_var="${lang^^}_SRC_ROOT"
  src_root="${!src_root_var:-.}"
  
  echo "Building ${lang} database..."
  if codeql database create "$CODEQL_DIR/${DB_NAME}-${lang}" \
    --language="$lang" \
    --source-root="$src_root" \
    --command="${lang} build ./..." \
    --threads=0 \
    --overwrite 2>&1 | tail -3; then
    echo "  [OK] ${lang} database created"
  else
    echo "  [WARN] ${lang} build failed, trying buildless mode..."
    codeql database create "$CODEQL_DIR/${DB_NAME}-${lang}" \
      --language="$lang" \
      --source-root="$src_root" \
      --build-mode=none \
      --threads=0 \
      --overwrite 2>&1 | tail -3
    echo "  [OK] ${lang} database created (buildless)"
  fi
done

echo ""
echo "Downloading query packs..."
for lang in go python javascript; do
  codeql pack download --dir "$CODEQL_DIR/packs" codeql/${lang}-queries 2>&1 | tail -1
done
echo "  [OK] Query packs downloaded"

echo ""
echo "=== Running Security Analysis ==="

for lang in go python javascript; do
  if [ -d "$CODEQL_DIR/${DB_NAME}-${lang}" ]; then
    echo ""
    echo "Analyzing ${lang}..."
    codeql database analyze "$CODEQL_DIR/${DB_NAME}-${lang}" \
      --format=sarif-latest \
      --output="$CODEQL_DIR/${DB_NAME}-${lang}-security.sarif" \
      -- codeql/${lang}-queries:codeql-suites/${lang}-experimental.qls 2>&1 | grep -E "(Analyzing|Running|Compiling|Evaluating|Writing|Analyzing with)" || true
    echo "  [OK] Security results: $CODEQL_DIR/${DB_NAME}-${lang}-security.sarif"
  fi
done

echo ""
echo "=== Done! ==="
echo ""
echo "Databases created:"
for lang in go python javascript; do
  if [ -d "$CODEQL_DIR/${DB_NAME}-${lang}" ]; then
    echo "  [OK] $CODEQL_DIR/${DB_NAME}-${lang}"
  else
    echo "  [ERR] $CODEQL_DIR/${DB_NAME}-${lang} (failed)"
  fi
done

echo ""
echo "Next steps:"
echo "  1. Open VS Code and install the CodeQL extension"
echo "  2. In the extension view, click 'Choose Database from Folder'"
echo "  3. Select $CODEQL_DIR/${DB_NAME}-<lang> to analyze"
echo "  4. Query packs in $CODEQL_DIR/packs/ are auto-discovered"
