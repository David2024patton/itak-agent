<div align="center">
  <img src="docs/images/logo.png" alt="GOAgent Logo" width="280"/>
  <h1>GOAgent</h1>
  <p><strong>An AI agent framework written in Go. One boss delegates to focused agents who get work done.</strong></p>
  <p>Runs on small local models. No expensive API bills required.</p>

  <br/>

  ![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)
  ![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)
  ![Version](https://img.shields.io/badge/Version-0.1.0-blue?style=for-the-badge)
  ![Platform](https://img.shields.io/badge/Platform-Windows%20|%20Linux%20|%20macOS-lightgrey?style=for-the-badge)

</div>

---

## What Is GOAgent?

GOAgent is an AI agent framework built in **Go**. It's a different approach from Python-based frameworks like CrewAI, LangGraph, and AutoGen. Instead of needing expensive, massive models, GOAgent is designed to work with **smaller, efficient models** you can run locally.

The core idea: **keep the boss simple and the agents focused.**

- The **Boss (Orchestrator)** never touches tools. It just figures out what needs to happen and hands off work.
- **Focused Agents** (like researcher, coder, browser) each have a small set of tools, clear goals, and a specific job.
- Each agent only sees what it needs. This keeps things fast and cheap.

### Why Go?

| Feature | Why It Matters |
|---|---|
| **Single binary** | No virtual environments, no `pip install`, no dependency problems |
| **Fast startup** | Agents start in milliseconds, not seconds |
| **Cross-platform** | Same binary runs on Windows, Linux, macOS, Docker |
| **Low memory** | Uses way less memory than Python frameworks |
| **Easy deployment** | Copy one file to your server and run it |

---

## How It Works

```mermaid
flowchart TD
    User["You type a message"] --> Boss

    subgraph Boss["Boss (Orchestrator)"]
        direction TB
        B1["Reads your request"]
        B2["Creates a task list"]
        B3["Assigns each task to the right agent"]
        B1 --> B2 --> B3
    end

    Boss -- "Task 1" --> Researcher
    Boss -- "Task 2" --> Coder

    subgraph Researcher["Researcher Manager"]
        direction TB
        RM1["Gets task from Boss"]
        RM2["Breaks it into sub-tasks"]
        RM3["Spins up Workers in parallel"]
        RM1 --> RM2 --> RM3
    end

    subgraph Coder["Coder Manager"]
        direction TB
        CM1["Gets task from Boss"]
        CM2["Breaks it into sub-tasks"]
        CM3["Spins up Workers in parallel"]
        CM1 --> CM2 --> CM3
    end

    Researcher --> RW1["Worker 1: http_fetch"]
    Researcher --> RW2["Worker 2: file_write"]

    Coder --> CW1["Worker 1: shell"]
    Coder --> CW2["Worker 2: file_read"]
    Coder --> CW3["Worker 3: file_write"]

    CW1 --> Doctor
    CW2 --> Doctor
    CW3 --> Doctor

    subgraph Doctor["Doctor Agent (GOBeat)"]
        direction TB
        D1["Runs linter for the project language"]
        D2["Checks for errors"]
        D3["Fixes problems or reports back"]
        D1 --> D2 --> D3
    end

    RW1 --> Result["Results flow back up"]
    RW2 --> Result
    Doctor --> Result
    Result --> Answer["Boss combines everything into a final answer"]
```

### Step by Step

1. **You type a message** - `"Fetch google.com and save the HTML to output.html"`
2. **Boss thinks** - "This needs HTTP fetching AND file writing. I'll send it to `researcher`."
3. **Boss sends a task** - `{ agent: "researcher", task: "Fetch google.com and save to output.html" }`
4. **Researcher does the work** - calls `http_fetch` to get the HTML, then calls `file_write` to save it
5. **Result comes back** - Boss gives you a clean answer: *"Done! Saved Google's HTML to output.html (52KB)"*

---

## Quick Start

### What You Need

- **Go 1.22+** installed ([download here](https://go.dev/dl/))
- **One of these** (pick whichever works for you):
  - A **cloud API key** (NVIDIA NIM, OpenAI, OpenRouter, etc.)
  - A **local model** running on [Ollama](https://ollama.com/) (free, no API key needed)
  - Both! Use cloud for the boss, local for workers

### 1. Clone and Build

```bash
git clone https://github.com/David2024patton/GOAgent.git
cd GOAgent
go build -o goagent ./cmd/goagent/
```

This creates a single `goagent` binary (or `goagent.exe` on Windows).

### 2. Set Up Your Config

Copy the example config and add your API key:

```bash
cp configs/example.yaml goagent.yaml
```

Open `goagent.yaml` and change the API key:

```yaml
orchestrator:
  llm:
    provider: nvidia_nim
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: YOUR_API_KEY_HERE    # put your key here
```

> **Tip:** You can use `${NVIDIA_API_KEY}` and set the environment variable instead of putting the key directly in the file.

### 3. Run It

```bash
./goagent run
```

You'll see:
```
GOAgent v0.1.0 - 2 agents ready. Type a message (Ctrl+C to quit).

>
```

Type anything and watch it work!

### 4. Try These Examples

```
> What is the capital of Japan?
Tokyo

> List all files in the current directory
[Boss sends task to coder -> runs 'dir' -> returns file list]

> Fetch https://httpbin.org/get and show me the response
[Boss sends task to researcher -> calls http_fetch -> returns JSON]

> Write "hello world" to test.txt
[Boss sends task to coder -> calls file_write -> confirms success]
```

---

## Configuration Guide

The config file (`goagent.yaml`) controls everything. Here's a full example:

```yaml
# ORCHESTRATOR
# The boss that delegates work. It NEVER uses tools.
orchestrator:
  llm:
    provider: nvidia_nim
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}                        # supports env vars!
  system_prompt: ""                                   # optional custom instructions
  max_delegations: 5                                  # max agents per request

# FOCUSED AGENTS
# Each agent has a specific job, personality, and tools.
agents:
  # Agent 1: The Researcher
  - name: researcher                    # unique name
    role: Senior Research Analyst       # job title (shown to boss)
    personality: >-                     # how the agent "thinks"
      Thorough and methodical researcher who finds accurate,
      well-sourced information and summarizes clearly
    goals:                              # max 3 narrow goals
      - accuracy
      - comprehensiveness
      - source_verification
    tools:                              # which tools this agent can use
      - http_fetch
      - file_read
      - file_write
    max_loops: 8                        # max tries before giving up

  # Agent 2: The Coder
  - name: coder
    role: Software Developer
    personality: >-
      Pragmatic developer who writes clean, working code
      with clear comments and handles errors gracefully
    goals:
      - correctness
      - readability
      - efficiency
    tools:
      - shell
      - file_read
      - file_write
    max_loops: 10

  # Agent 3: Add your own! (example)
  # - name: writer
  #   role: Content Writer
  #   personality: "Creative writer with a focus on clear, engaging prose"
  #   goals: [engagement, clarity, originality]
  #   tools: [file_read, file_write]
  #   max_loops: 6

# INTEGRATIONS (optional)
integrations:
  serp_api:
    api_key: ${SERP_API_KEY}
```

### Using Different LLM Providers

Each agent can use a **different model**. Use a cheap model for simple tasks and a bigger one for hard ones:

```yaml
# Boss uses a small, fast model
orchestrator:
  llm:
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}

agents:
  # Researcher uses a local Ollama model (free)
  - name: researcher
    llm:
      model: llama3.1:8b
      api_base: http://localhost:11434/v1
      api_key: ""

  # Coder uses a bigger cloud model for complex code
  - name: coder
    llm:
      model: deepseek-coder-v2
      api_base: https://openrouter.ai/api/v1
      api_key: ${OPENROUTER_API_KEY}
```

> If an agent doesn't have its own `llm` section, it uses the same model as the boss.

---

## Built-in Tools

Every focused agent gets tools from this list. The boss has **no tools**.

### `shell` - Run Commands

Runs any shell command on the host system. Auto-detects the OS:

| Platform | Shell Used |
|---|---|
| Windows | `cmd /C` |
| macOS | `zsh -c` |
| Linux / Docker | `sh -c` |
| Android (Termux) | `sh -c` |

**Examples the agent might run:**

```
dir                           # list files (Windows)
ls -la                        # list files (Linux/Mac)
python script.py              # run a script
git status                    # check git state
curl https://example.com      # fetch a URL
pip install requests          # install a package
```

**Config:** `timeout_seconds` (default: 30) stops runaway commands.

---

### `file_read` - Read Files

Reads the full text of any file the agent has access to.

```
Agent calls: file_read({ path: "README.md" })
Returns: the full text of README.md
```

---

### `file_write` - Write Files

Writes content to a file. Creates folders automatically if they don't exist.

```
Agent calls: file_write({ path: "output/report.txt", content: "Hello World" })
Returns: "Wrote 11 bytes to /full/path/output/report.txt"
```

---

### `http_fetch` - HTTP Requests

Makes HTTP GET or POST requests and returns the response. Good for APIs, web scraping, and data retrieval.

```
Agent calls: http_fetch({ url: "https://api.example.com/data", method: "GET" })
Returns: "HTTP 200\n\n{...response body...}"
```

**Features:**
- GET and POST methods
- Custom headers
- Request body (for POST)
- 30-second timeout
- Response cut at 50KB to prevent overflow

---

## Debug Mode

GOAgent has three logging levels:

### Quiet Mode (default)

```bash
./goagent run
```

Only shows the final response. Warnings and errors show up if something goes wrong.

### Verbose Mode

```bash
./goagent run --verbose
```

Shows key decisions: which agent was picked, what tools were called, success or failure.

```
14:33:24.100 INFO [orchestrator] Processing: List all files in the current directory
14:33:24.101 INFO [orchestrator] Calling LLM for delegation decision...
14:33:25.543 INFO [orchestrator] -> Delegating [1/1] to "coder": Run dir command
14:33:25.544 INFO [coder] Starting task: Run dir command
14:33:25.544 INFO [coder] Loop 1/10
14:33:26.891 INFO [coder] Calling tool "shell" (id: call_abc123)
14:33:26.950 INFO [coder] Done (loop 2)
14:33:26.950 INFO [orchestrator] <- "coder" succeeded
14:33:26.951 INFO [orchestrator] Combining 1 result(s)...
```

### Debug Mode

```bash
./goagent run --debug
```

Shows **everything**: JSON payloads, token counts, HTTP timing, tool arguments, full results. Use this when something isn't working:

```
14:33:24.100 DEBUG [llm] POST https://integrate.api.nvidia.com/v1/chat/completions
                         (model: nemotron-3-nano-30b, messages: 2, tools: 0)
14:33:25.543 DEBUG [llm] Response: HTTP 200, 347 bytes, 1.443s elapsed
14:33:25.543 DEBUG [llm] Finish reason: stop, tool_calls: 0, content length: 203
14:33:25.543 DEBUG [orchestrator] Tokens: prompt: 412, completion: 87, total: 499
14:33:25.544 DEBUG [orchestrator] Raw LLM response:
{"reasoning":"...","delegations":[{"agent":"coder","task":"..."}]}
```

---

## Project Structure

```
GOAgent/
├── cmd/
│   └── goagent/
│       └── main.go              # CLI entrypoint (run, version, help)
│
├── pkg/
│   ├── agent/
│   │   ├── types.go             # Core types: AgentConfig, TaskPayload, Result
│   │   ├── orchestrator.go      # Boss: reasons and delegates (NO tools)
│   │   └── focused.go           # Focused Agent: task loop with tool calling
│   │
│   ├── llm/
│   │   ├── client.go            # OpenAI-compatible HTTP client
│   │   └── message.go           # Message, ToolCall, Response types
│   │
│   ├── tool/
│   │   ├── interface.go         # Tool interface (Name, Description, Schema, Execute)
│   │   ├── registry.go          # Tool registry + LLM schema generation
│   │   └── builtins/
│   │       ├── shell.go         # Shell command execution (cross-platform)
│   │       ├── file.go          # File read/write operations
│   │       └── http.go          # HTTP GET/POST requests
│   │
│   ├── config/
│   │   └── config.go            # YAML config loader with env var support
│   │
│   └── debug/
│       └── logger.go            # Structured logger (ERROR/WARN/INFO/DEBUG)
│
├── configs/
│   └── example.yaml             # Example configuration file
│
├── docs/
│   ├── images/
│   │   └── logo.png             # GOAgent logo
│   ├── CONFIGURATION.md         # Configuration deep-dive
│   ├── TOOLS.md                 # Tool reference
│   └── ARCHITECTURE.md          # Architecture explanation
│
├── goagent.yaml                 # Your active config (not committed)
├── go.mod
├── go.sum
├── .gitignore
├── LICENSE
└── README.md                    # This file
```

### What Each File Does

| File | What to edit it for |
|---|---|
| `goagent.yaml` | Change API keys, add/remove agents, change models |
| `configs/example.yaml` | Reference config. Don't edit directly, copy it |
| `pkg/agent/types.go` | Add new fields to agent config |
| `pkg/agent/orchestrator.go` | Change how the boss reasons or delegates |
| `pkg/agent/focused.go` | Change how focused agents run their task loop |
| `pkg/tool/builtins/*.go` | Add new built-in tools or change existing ones |
| `pkg/tool/interface.go` | Change the Tool interface (rarely needed) |
| `pkg/llm/client.go` | Add support for non-OpenAI API formats |
| `pkg/config/config.go` | Add new config options |
| `pkg/debug/logger.go` | Change log formatting or add new log levels |
| `cmd/goagent/main.go` | Add new CLI commands or flags |

---

## Adding a New Agent

Want to add a "Writer" agent? Just add it to your `goagent.yaml`:

```yaml
agents:
  # ... existing agents ...

  - name: writer
    role: Content Writer
    personality: "Creative writer who produces clear, engaging content"
    goals:
      - engagement
      - clarity
      - originality
    tools:
      - file_read
      - file_write
    max_loops: 6
```

That's it. Restart GOAgent and the boss will automatically know about the writer agent and can hand off writing tasks to it.

---

## Adding a New Tool

Create a new file in `pkg/tool/builtins/` that follows the `Tool` interface:

```go
// pkg/tool/builtins/mytool.go
package builtins

import "context"

type MyTool struct{}

func (m *MyTool) Name() string        { return "my_tool" }
func (m *MyTool) Description() string { return "What this tool does" }
func (m *MyTool) Schema() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "input": map[string]interface{}{
                "type":        "string",
                "description": "The input to process",
            },
        },
        "required": []string{"input"},
    }
}
func (m *MyTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
    input := args["input"].(string)
    // ... your logic here ...
    return "result: " + input, nil
}
```

Then register it in `cmd/goagent/main.go`:

```go
func buildToolCatalog() map[string]tool.Tool {
    return map[string]tool.Tool{
        "shell":      &builtins.ShellTool{},
        "file_read":  &builtins.FileReadTool{},
        "file_write": &builtins.FileWriteTool{},
        "http_fetch": &builtins.HTTPFetchTool{},
        "my_tool":    &builtins.MyTool{},          // add this line
    }
}
```

Rebuild, and now any agent with `my_tool` in its `tools` list can use it.

---

## Supported LLM Providers

GOAgent works with any API that uses the OpenAI `/v1/chat/completions` format:

| Provider | `api_base` | Notes |
|---|---|---|
| **NVIDIA NIM** | `https://integrate.api.nvidia.com/v1` | Free tier available |
| **Ollama** (local) | `http://localhost:11434/v1` | Free, runs on your machine |
| **OpenRouter** | `https://openrouter.ai/api/v1` | Access to 100+ models |
| **OpenAI** | `https://api.openai.com/v1` | GPT-4o, etc. |
| **Together AI** | `https://api.together.xyz/v1` | Open-source models |
| **Groq** | `https://api.groq.com/openai/v1` | Ultra-fast inference |
| **Any compatible** | Your URL | Anything with `/chat/completions` |

---

## The GOAgent Ecosystem

GOAgent is the core framework, but it's part of a bigger system:

| Project | What It Does |
|---|---|
| **GOAgent** | Core agent framework. Boss + focused agents + tools. |
| **GOGateway** | Standalone LLM gateway. Routes requests across 42+ providers with failover, rate limiting, and cost tracking. |
| **GOForge** | Live preview server + container runtime. Builds and hosts projects in real time as agents write code. GitHub integration. |
| **GODashboard** | Web-based dashboard. Chat, agent monitoring, task board, cost tracking. |
| **GOBeat** | Self-healing monitor. Auto-detects errors, runs diagnostics, fixes problems, remembers what worked. |
| **GOHub** | Extension marketplace. Browse, install, and share agent skills and tools. |
| **GOBrowser** | Custom browser engine for agents. DOM extraction and web automation. |
| **GOVision** | Screen automation. Takes screenshots, understands UI, clicks/types on your behalf. |

---

## Roadmap

- [x] Core orchestrator + focused agent pipeline
- [x] Built-in tools (shell, file, HTTP)
- [x] Structured debug logging
- [x] Cross-platform shell support
- [x] Per-agent LLM configuration
- [x] Mandatory task system (every request gets a checklist)
- [x] Single-call routing (one LLM call to pick the right agent)
- [x] GOGateway with 42-provider catalog, failover, cost tracking
- [ ] GOForge live preview server (Tier 1: process isolation)
- [ ] Local model marketplace with hardware auto-detect
- [ ] Manager-worker hierarchy (agents delegate to sub-agents)
- [ ] Memory system (session + persistent + knowledge graph)
- [ ] GOHub extension marketplace
- [ ] GOBeat self-healing
- [ ] Communication plugins (Discord, Telegram, WhatsApp, Slack)
- [ ] MCP client/server support
- [ ] GOVision screen automation

---

## License

MIT. Use it however you want.

## Contributing

PRs welcome! Check the [project structure](#project-structure) section to see where things live.

---

<div align="center">
  <sub>Built with Go</sub>
</div>
