# Analysis Commands

## VS Code Extension

The VS Code extension provides interactive analysis:
1. Open the CodeQL extension view
2. Select a database from the "DATABASES" section
3. Select queries from the "QUERY SERVER" section
4. Click the play button to run

## CLI Analysis

List packs:

```
codeql resolve qlpacks
```

Security suite (SARIF):

```
codeql database analyze codeql.db \
  --format=sarif-latest \
  --output=results.sarif \
  -- codeql/python-queries:codeql-suites/python-security-extended.qls
```

CSV output:

```
codeql database analyze codeql.db \
  --format=csv \
  --output=results.csv \
  -- codeql/javascript-queries
```

With Trail of Bits queries:

```
codeql database analyze codeql.db \
  --format=sarif-latest \
  --output=results.sarif \
  -- trailofbits/go-queries
```

## Batch Analysis

Use `codeql/scripts/codeql-analyze-all.sh` for batch analysis across languages:

```
./scripts/codeql-analyze-all.sh security-and-quality
```

