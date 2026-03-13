# Databases

A CodeQL database captures the structure of your code. Databases are required for analysis but can be skipped for quick query testing.

## Create Database

```
codeql database create codeql.db --language=<LANG> [--command='<BUILD>'] --source-root=.
```

Language guide:

| Language | `--language` | Build Required |
| --- | --- | --- |
| Python | `python` | No |
| JavaScript/TypeScript | `javascript` | No |
| Go | `go` | No |
| Ruby | `ruby` | No |
| Rust | `rust` | Yes (`cargo build`) |
| Java/Kotlin | `java` | Yes (`./gradlew build`) |
| C/C++ | `cpp` | Yes (`make -j8`) |
| C# | `csharp` | Yes (`dotnet build`) |
| Swift | `swift` | Yes (macOS only) |

## VS Code Extension

The extension can create databases automatically:
1. Open the Command Palette (Cmd+Shift+P)
2. Run "CodeQL: Create Database from Command"
3. Select source folder and language

## Batch Database Creation

Use `codeql/scripts/codeql-build-all.sh` to build databases for multiple languages:

```
./scripts/codeql-build-all.sh
```

This creates databases in `codeql/<project>-<lang>/` and downloads query packs to `codeql/packs/`.

