# Setup

## VS Code Extension

Install the [CodeQL for Visual Studio Code](https://marketplace.visualstudio.com/items?itemName=GitHub.vscode-codeql) extension.

### Workspace Configuration

The extension automatically discovers query packs in these locations:
- `.codeql/packs/`
- `codeql/packs/`
- `qlpacks.yml` in workspace root

Run `codeql/scripts/codeql-build-all.sh` to set up the recommended structure.

## Excluding Folders from Database

When building databases, exclude folders like `examples`, `tests`, or `vendor` using the `--exclude` flag:

```
codeql database create <db-name> \
  --language=<language> \
  --source-root=<source-root> \
  --exclude="examples,tests,vendor" \
  -- <command>
```

The `codeql-build-all.sh` script supports this via environment variables:

```bash
# For a single build
CODEQL_EXCLUDE="examples,tests" ./scripts/codeql-build-all.sh

# Or set in your shell profile for persistent configuration
export CODEQL_EXCLUDE="examples,tests,vendor"
```

## Check Installation

```
command -v codeql >/dev/null 2>&1 && echo "CodeQL: installed" || echo "CodeQL: NOT installed"
```

## Install (macOS/Linux)

```
brew install --cask codeql
brew upgrade codeql
```

Manual bundles: https://github.com/github/codeql-action/releases

## Optional Packs

Trail of Bits packs:

```
codeql pack download trailofbits/cpp-queries trailofbits/go-queries
codeql resolve qlpacks | grep trailofbits
```

