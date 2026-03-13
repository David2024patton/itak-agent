# CodeQL Query Suites

## Available Suites by Language

### JavaScript/TypeScript
- `javascript-all.qls` - All JavaScript queries (security + quality)
- `javascript-experimental.qls` - All security queries including experimental
- `javascript-security-and-quality.qls` - Security + quality rules
- `javascript-security-extended.qls` - Extended security rules
- `javascript-security-quick.qls` - Fast security queries (quickstart)

### Python
- `python-all.qls` - All Python queries
- `python-experimental.qls` - All security queries including experimental
- `python-security-and-quality.qls` - Security + quality rules
- `python-security-extended.qls` - Extended security rules
- `python-security-quick.qls` - Fast security queries

### Go
- `go-all.qls` - All Go queries
- `go-experimental.qls` - All security queries including experimental
- `go-security-and-quality.qls` - Security + quality rules
- `go-security-extended.qls` - Extended security rules
- `go-security-quick.qls` - Fast security queries

## Usage

```bash
codeql database analyze <database> \
  --codeql/javascript-queries:codeql-suites/javascript-experimental.qls
```

## Notes

- `experimental` suites include all security rules plus experimental queries under evaluation
- `security-and-quality` includes security rules plus code quality checks
- `security-extended` includes extended security rules beyond the base suite
- `security-quick` is optimized for speed (useful for CI/CD)
