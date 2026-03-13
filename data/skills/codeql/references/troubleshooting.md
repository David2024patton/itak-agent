# Troubleshooting

| Issue | Fix |
| --- | --- |
| Database creation fails | Verify build command works outside CodeQL |
| Slow analysis | Use `--threads`, narrow query suite |
| Missing results | Check source root and exclusions |
| Out of memory | Set `CODEQL_RAM` (e.g., `48000`) |
| CMake path issues | Adjust `--source-root` |

## Rationalizations to Reject

- "No findings means secure." Queries are limited to known patterns.
- "Small change, low risk." Small diffs can introduce critical flaws.
- "Tests passed." Tests do not prove absence of vulnerabilities.
