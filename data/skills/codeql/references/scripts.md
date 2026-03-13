# Helper Scripts

Multi-language helpers live in `codeql/scripts/`:

- `codeql/scripts/codeql-build-all.sh`: builds databases for Go, Python, and JavaScript, and downloads official packs into `./codeql/packs`. Respects `GO_SRC_ROOT`, `PYTHON_SRC_ROOT`, `JS_SRC_ROOT`.
- `codeql/scripts/codeql-analyze-all.sh <suite>`: runs the suite for each language, writes SARIF under `./codeql/`, and summarizes with `jq`.

## VS Code Workspace Configuration

The scripts create a `codeql` directory structure that the VS Code extension automatically discovers:

```
codeql/
├── <project>-go/          # Go database
├── <project>-python/      # Python database
├── <project>-javascript/  # JavaScript database
└── packs/                 # Query packs (auto-discovered)
    └── codeql/
        ├── go-queries/
        ├── python-queries/
        └── javascript-queries/
```

The extension looks for query packs in:
- `.codeql/packs/`
- `codeql/packs/`
- `qlpacks.yml` in workspace root

## Usage

Run from any project:

```
bash /path/to/skills/codeql/scripts/codeql-build-all.sh
```

Then in VS Code:
1. Open the CodeQL extension view
2. Click "Choose Database from Folder" → select `codeql/<project>-<lang>`
3. Query packs in `codeql/packs/` appear automatically

