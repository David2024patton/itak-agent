# Tools Reference

Tools are the actions that focused agents can perform. Each tool has a name, description, input schema, and an execute function.

> **Important:** The orchestrator has **NO tools**. Only focused agents call tools.

## Built-in Tools

### `shell`

**What it does:** Executes a shell command on the host machine and returns the combined stdout/stderr output.

**Platform detection:**

| OS | Shell |
|---|---|
| Windows | `cmd /C "command"` |
| macOS | `zsh -c "command"` |
| Linux, Android, FreeBSD, OpenBSD, NetBSD | `sh -c "command"` |

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `command` | string | Yes | The shell command to run |
| `timeout_seconds` | integer | No | Max execution time (default: 30) |

**Examples of what the agent might do:**

```
List files:          shell({ command: "dir" })              → Windows
                     shell({ command: "ls -la" })           → Linux/Mac

Run scripts:         shell({ command: "python analyze.py" })
Install packages:    shell({ command: "pip install requests" })
Check git:           shell({ command: "git status" })
Network:             shell({ command: "ping -n 1 google.com" })
Chain commands:      shell({ command: "mkdir output & echo done" })
```

**Error handling:** If the command fails, the tool returns the error + any partial output (instead of crashing). The agent can then decide to retry or report the failure.

---

### `file_read`

**What it does:** Reads the full text content of a file and returns it as a string.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `path` | string | Yes | Path to the file (absolute or relative) |

**Example:**

```
file_read({ path: "README.md" })
→ returns the full text content of README.md
```

**Notes:**
- Relative paths are resolved from the current working directory
- Binary files will return garbled text (this tool is for text files)
- If the file doesn't exist, returns an error the agent can handle

---

### `file_write`

**What it does:** Writes text content to a file. Creates parent directories if they don't exist.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `path` | string | Yes | Path to the file to write |
| `content` | string | Yes | Content to write |

**Example:**

```
file_write({ path: "output/report.txt", content: "Analysis complete." })
→ "Wrote 18 bytes to C:\projects\output\report.txt"
```

**Notes:**
- Overwrites the file if it already exists
- Creates directories automatically (`output/` in the example above)
- Returns the absolute path and byte count for confirmation

---

### `http_fetch`

**What it does:** Makes an HTTP GET or POST request and returns the response body.

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `url` | string | Yes | The URL to fetch |
| `method` | string | No | `GET` or `POST` (default: `GET`) |
| `body` | string | No | Request body for POST requests |
| `headers` | object | No | Custom HTTP headers (key-value pairs) |

**Examples:**

```
# Simple GET
http_fetch({ url: "https://httpbin.org/get" })
→ "HTTP 200\n\n{...response...}"

# POST with body
http_fetch({
  url: "https://api.example.com/data",
  method: "POST",
  body: "{\"key\": \"value\"}",
  headers: { "Content-Type": "application/json" }
})
→ "HTTP 201\n\n{...response...}"
```

**Notes:**
- 30-second timeout on all requests
- Responses over 50KB are automatically truncated (prevents context overflow)
- Returns the HTTP status code with the response body
- The agent can inspect the status code to detect errors

---

## Creating Custom Tools

### Step 1: Create the Tool File

Create a new `.go` file in `pkg/tool/builtins/`:

```go
// pkg/tool/builtins/search.go
package builtins

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "net/url"
)

// SearchTool performs web searches via SerpAPI.
type SearchTool struct {
    APIKey string
}

func (s *SearchTool) Name() string { return "web_search" }

func (s *SearchTool) Description() string {
    return "Search the web for information. Returns top results with titles, snippets, and URLs."
}

func (s *SearchTool) Schema() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "query": map[string]interface{}{
                "type":        "string",
                "description": "The search query",
            },
        },
        "required": []string{"query"},
    }
}

func (s *SearchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
    query, ok := args["query"].(string)
    if !ok || query == "" {
        return "", fmt.Errorf("missing required argument: query")
    }

    // Build the SerpAPI URL
    apiURL := fmt.Sprintf(
        "https://serpapi.com/search.json?q=%s&api_key=%s",
        url.QueryEscape(query), s.APIKey,
    )

    resp, err := http.Get(apiURL)
    if err != nil {
        return "", fmt.Errorf("search request failed: %w", err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    return string(body), nil
}
```

### Step 2: Register in the Catalog

Edit `cmd/goagent/main.go` and add your tool to the catalog:

```go
func buildToolCatalog() map[string]tool.Tool {
    return map[string]tool.Tool{
        "shell":      &builtins.ShellTool{},
        "file_read":  &builtins.FileReadTool{},
        "file_write": &builtins.FileWriteTool{},
        "http_fetch": &builtins.HTTPFetchTool{},
        "web_search": &builtins.SearchTool{            // ← NEW
            APIKey: os.Getenv("SERP_API_KEY"),
        },
    }
}
```

### Step 3: Assign to an Agent

Update your `goagent.yaml`:

```yaml
agents:
  - name: researcher
    tools:
      - web_search      # ← now available
      - http_fetch
      - file_read
```

### Step 4: Rebuild

```bash
go build -o goagent ./cmd/goagent/
```

The orchestrator will now see `web_search` in the researcher's tool list and can delegate search tasks to it.

## The Tool Interface

Every tool must implement this Go interface:

```go
type Tool interface {
    Name() string                                                  // unique identifier
    Description() string                                           // what the LLM sees
    Schema() map[string]interface{}                                // JSON Schema for parameters
    Execute(ctx context.Context, args map[string]interface{}) (string, error)
}
```

### Schema Format

The `Schema()` method returns a [JSON Schema](https://json-schema.org/) object that describes the tool's input parameters. The LLM uses this to know what arguments to pass:

```go
func (t *MyTool) Schema() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "param1": map[string]interface{}{
                "type":        "string",     // string, integer, number, boolean
                "description": "What this parameter does",
            },
            "param2": map[string]interface{}{
                "type":        "integer",
                "description": "Another parameter",
            },
        },
        "required": []string{"param1"},      // which params are mandatory
    }
}
```

### Error Handling Best Practices

When writing tool `Execute()` methods:

1. **Always validate inputs** — check that required args exist and have the right type
2. **Return errors, don't panic** — the agent loop handles errors gracefully
3. **Include context in errors** — `"missing required argument: query"` not just `"bad input"`
4. **Partial results are OK** — if a command fails but produces output, return both
5. **Truncate large outputs** — prevent context overflow (see `http_fetch` for an example)
