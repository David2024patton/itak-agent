---
name: codeql
description: Use when writing, testing, and running CodeQL queries in VS Code, or setting up workspace configuration for the CodeQL extension.
---

# CodeQL

## Goal

Write, test, and run CodeQL queries with VS Code integration. The extension automatically discovers query packs in your workspace.

## Guardrails

- The VS Code extension discovers query packs automatically from `.codeql` or `codeql` directories.
- Database analysis is manual in VS Code; scripts are provided for batch operations.
- Prefer focused query suites over running all queries.

## Workflow (short)

1. Set up workspace configuration (optional).
2. Create or select a database.
3. Write or select queries.
4. Run queries in VS Code or CLI.

## References (load when needed)

- `codeql/references/setup.md`: install and environment checks.
- `codeql/references/database.md`: database creation by language.
- `codeql/references/analyze.md`: running queries and output formats.
- `codeql/references/custom-queries.md`: writing, testing, and packaging queries.
- `codeql/references/ci.md`: GitHub Actions integration.
- `codeql/references/troubleshooting.md`: common failures and fixes.
- `codeql/references/scripts.md`: helper scripts for batch operations.
