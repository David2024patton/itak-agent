# Custom Queries

## VS Code Workflow

1. Create a `.ql` file in your workspace
2. Write your query (see template below)
3. Right-click the file → "CodeQL: Run Query Analysis"
4. Select a database when prompted

## Query Template

```ql
/**
 * @name Find SQL injection vulnerabilities
 * @description Identifies potential SQL injection from user input
 * @kind path-problem
 * @problem.severity error
 * @security-severity 9.0
 * @precision high
 * @id py/sql-injection
 * @tags security
 *       external/cwe/cwe-089
 */

import python
import semmle.python.dataflow.new.DataFlow
import semmle.python.dataflow.new.TaintTracking

module SqlInjectionConfig implements DataFlow::ConfigSig {
  predicate isSource(DataFlow::Node source) { exists(source) }
  predicate isSink(DataFlow::Node sink) { exists(sink) }
}

module SqlInjectionFlow = TaintTracking::Global<SqlInjectionConfig>;

from SqlInjectionFlow::PathNode source, SqlInjectionFlow::PathNode sink
where SqlInjectionFlow::flowPath(source, sink)
select sink.getNode(), source, sink, "SQL injection from $@.", source.getNode(), "user input"
```

## Query Metadata

| Field | Description | Values |
| --- | --- | --- |
| `@kind` | Query type | `problem`, `path-problem` |
| `@problem.severity` | Issue severity | `error`, `warning`, `recommendation` |
| `@security-severity` | CVSS score | `0.0` - `10.0` |
| `@precision` | Confidence | `very-high`, `high`, `medium`, `low` |

## Query Packs

```
codeql pack init myorg/security-queries
```

Structure:

```
myorg-security-queries/
├── qlpack.yml
├── src/
│   └── SqlInjection.ql
└── test/
    └── SqlInjectionTest.expected
```

`qlpack.yml`:

```yaml
name: myorg/security-queries
version: 1.0.0
dependencies:
  codeql/python-all: "*"
```

## Test Queries

```
codeql test run test/
```
