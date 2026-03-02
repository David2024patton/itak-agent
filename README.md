<div align="center">
  <img src="docs/images/logo.png" alt="GOAgent Logo" width="280"/>
  <h1>GOAgent</h1>
  <p><strong>A lightweight Go-based AI agent framework where a small orchestrator delegates to focused sub-agents.</strong></p>
  <p>Built to run efficiently on 30B models — no $200/mo API bills required.</p>

  <br/>

  ![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)
  ![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)
  ![Version](https://img.shields.io/badge/Version-0.1.0-blue?style=for-the-badge)
  ![Platform](https://img.shields.io/badge/Platform-Windows%20|%20Linux%20|%20macOS-lightgrey?style=for-the-badge)

</div>

---

## What Is GOAgent?

GOAgent is an AI agent framework written in **Go** that takes a different approach from Python-based frameworks like CrewAI, LangGraph, and AutoGen. Instead of requiring expensive frontier models with massive context windows, GOAgent is designed to run on **smaller, efficient models** like NVIDIA's Nemotron 30B.

The key insight: **keep the orchestrator dumb and the agents focused.**

- The **Orchestrator** never touches tools — it only reasons about what needs to happen and delegates work
- **Focused Agents** each have a small set of tools (max 7), clear goals, and a specific personality
- **Progressive disclosure** keeps each agent's context window small and efficient

### Why Go?

| Feature | Benefit |
|---|---|
| **Single binary** | No virtual environments, no `pip install`, no dependency hell |
| **Fast startup** | Agents start in milliseconds, not seconds |
| **Cross-platform** | Same binary runs on Windows, Linux, macOS, Docker |
| **Low memory** | Fraction of what Python frameworks consume |
| **Easy deployment** | Copy one file to your server and run it |

---

## Architecture

```
                    User sends a message
                           │
                           ▼
              ┌─────────────────────────┐
              │   ORCHESTRATOR AGENT    │
              │                         │
              │  • Receives the request │
              │  • Thinks about it      │
              │  • Decides WHO to ask   │
              │  • Sends focused tasks  │
              │                         │
              │  ⛔ NO tools           │
              │  ⛔ NO file access     │
              │  ⛔ NO shell access    │
              └───┬──────────┬──────────┘
                  │          │
         ┌────────▼───┐  ┌───▼────────┐
         │ RESEARCHER │  │   CODER    │
         │            │  │            │
         │ Tools:     │  │ Tools:     │
         │ • http     │  │ • shell    │
         │ • file_read│  │ • file_read│
         │ • file_wrt │  │ • file_wrt │
         │            │  │            │
         │ "Find info"│  │ "Write code│
         └────────────┘  └────────────┘
                  │          │
                  ▼          ▼
              Results flow back to
              the Orchestrator, which
              combines them into a
              final response for the user
```

### How It Works Step-by-Step

1. **You type a message** → `"Fetch google.com and save the HTML to output.html"`
2. **Orchestrator thinks** → "This needs HTTP fetching AND file writing. I'll send it to `researcher`."
3. **Orchestrator creates a TaskPayload** → `{ agent: "researcher", task: "Fetch google.com and save to output.html" }`
4. **Researcher agent receives the task** → Calls `http_fetch` tool → Gets the HTML → Calls `file_write` tool → Saves it
5. **Result flows back** → Orchestrator synthesizes a clean response for you: *"Done! Saved Google's HTML to output.html (52KB)"*

---

## Quick Start

### Prerequisites

- **Go 1.22+** installed ([download](https://go.dev/dl/))
- An **API key** for an LLM provider (NVIDIA NIM, OpenAI, Ollama, etc.)

### 1. Clone and Build

```bash
git clone https://github.com/David2024patton/GOAgent.git
cd GOAgent
go build -o goagent ./cmd/goagent/
```

This creates a single `goagent` binary (or `goagent.exe` on Windows).

### 2. Create Your Config

Copy the example config and add your API key:

```bash
cp configs/example.yaml goagent.yaml
```

Edit `goagent.yaml` — the only thing you **must** change is the API key:

```yaml
orchestrator:
  llm:
    provider: nvidia_nim
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: YOUR_API_KEY_HERE    # ← put your key here
```

> 💡 **Tip:** You can also use `${NVIDIA_API_KEY}` and set the environment variable instead of hardcoding the key.

### 3. Run It

```bash
./goagent run
```

You'll see:
```
GOAgent v0.1.0 — 2 agents ready. Type a message (Ctrl+C to quit).

>
```

Type anything and watch it work!

### 4. Try These Examples

```
> What is the capital of Japan?
Tokyo

> List all files in the current directory
[Orchestrator delegates to coder → runs 'dir' → returns file list]

> Fetch https://httpbin.org/get and show me the response
[Orchestrator delegates to researcher → calls http_fetch → returns JSON]

> Write "hello world" to test.txt
[Orchestrator delegates to coder → calls file_write → confirms success]
```

---

## Configuration Guide

The config file (`goagent.yaml`) controls everything. Here's a complete annotated example:

```yaml
# ── ORCHESTRATOR ────────────────────────────────────────
# The orchestrator is the "brain" that delegates work.
# It NEVER uses tools — it only decides which agent to ask.
orchestrator:
  llm:
    provider: nvidia_nim                              # provider name (for your reference)
    model: nvidia/nemotron-3-nano-30b-a3b             # which model to use
    api_base: https://integrate.api.nvidia.com/v1     # API endpoint
    api_key: ${NVIDIA_API_KEY}                        # supports env vars!
  system_prompt: ""                                   # optional: add custom instructions
  max_delegations: 5                                  # max agents per request

# ── FOCUSED AGENTS ──────────────────────────────────────
# Each agent has a specific role, personality, and tools.
agents:
  # Agent 1: The Researcher
  - name: researcher                    # unique name (used in delegation)
    role: Senior Research Analyst       # job title (shown to orchestrator)
    personality: >-                     # how the agent "thinks"
      Thorough and methodical researcher who finds accurate,
      well-sourced information and summarizes clearly
    goals:                              # max 3 narrow KPIs
      - accuracy
      - comprehensiveness
      - source_verification
    tools:                              # which built-in tools this agent can use
      - http_fetch
      - file_read
      - file_write
    max_loops: 8                        # max reasoning iterations before giving up

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

# ── INTEGRATIONS (optional) ─────────────────────────────
# External API connections that skills can reference.
integrations:
  serp_api:
    api_key: ${SERP_API_KEY}
```

### Using Different LLM Providers

Each agent can use a **different model** — use a cheap model for simple tasks, a bigger one for hard ones:

```yaml
# Orchestrator uses a small, fast model
orchestrator:
  llm:
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}

agents:
  # Researcher uses Ollama local model (free)
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

> If an agent doesn't specify its own `llm` section, it automatically inherits the orchestrator's model.

---

## Built-in Tools

Every focused agent is assigned tools from this catalog. The orchestrator has **no tools**.

### `shell` — Execute Commands

Runs any shell command on the host system. Auto-detects the OS:

| Platform | Shell Used |
|---|---|
| Windows | `cmd /C` |
| macOS | `zsh -c` |
| Linux / Docker | `sh -c` |
| Android (Termux) | `sh -c` |
| FreeBSD / OpenBSD | `sh -c` |

**What it can do:** Run programs, check system state, install packages, chain commands, execute scripts.

```
Examples the agent might run:
  dir                           # list files (Windows)
  ls -la                        # list files (Linux/Mac)
  python script.py              # run a script
  git status                    # check git state
  curl https://example.com      # fetch a URL
  pip install requests          # install a package
```

**Config:** `timeout_seconds` (default: 30) prevents runaway commands.

---

### `file_read` — Read Files

Reads the full text content of any file the agent has access to.

```
Agent calls: file_read({ path: "README.md" })
Returns: the full text content of README.md
```

---

### `file_write` — Write Files

Writes content to a file. Creates parent directories automatically if they don't exist.

```
Agent calls: file_write({ path: "output/report.txt", content: "Hello World" })
Returns: "Wrote 11 bytes to /full/path/output/report.txt"
```

---

### `http_fetch` — HTTP Requests

Makes HTTP GET or POST requests and returns the response body. Useful for APIs, web scraping, and data retrieval.

```
Agent calls: http_fetch({ url: "https://api.example.com/data", method: "GET" })
Returns: "HTTP 200\n\n{...response body...}"
```

**Features:**
- GET and POST methods
- Custom headers
- Request body (for POST)
- 30-second timeout
- Response truncated at 50KB (prevents context overflow)

---

## Debug Mode

GOAgent has three logging levels to help you understand what's happening:

### Quiet Mode (default)

```bash
./goagent run
```

Only shows the final response. Warnings and errors appear if something goes wrong.

### Verbose Mode

```bash
./goagent run --verbose
```

Shows key decisions: which agent was chosen, what tools were called, success/failure.

```
14:33:24.100 INFO [orchestrator] Processing: List all files in the current directory
14:33:24.101 INFO [orchestrator] Calling LLM for delegation decision...
14:33:25.543 INFO [orchestrator] → Delegating [1/1] to "coder": Run dir command
14:33:25.544 INFO [coder] Starting task: Run dir command
14:33:25.544 INFO [coder] Loop 1/10
14:33:26.891 INFO [coder] Calling tool "shell" (id: call_abc123)
14:33:26.950 INFO [coder] ✓ Task complete (loop 2)
14:33:26.950 INFO [orchestrator] ← "coder" succeeded
14:33:26.951 INFO [orchestrator] Synthesizing 1 result(s)...
```

### Debug Mode

```bash
./goagent run --debug
```

Shows **everything** — JSON payloads, token counts, HTTP timing, tool arguments, full results. Use this when something isn't working:

```
14:33:24.100 DEBUG [llm] POST https://integrate.api.nvidia.com/v1/chat/completions
                         (model: nemotron-3-nano-30b, messages: 2, tools: 0)
14:33:25.543 DEBUG [llm] Response: HTTP 200, 347 bytes, 1.443s elapsed
14:33:25.543 DEBUG [llm] Finish reason: stop, tool_calls: 0, content length: 203
14:33:25.543 DEBUG [orchestrator] Tokens — prompt: 412, completion: 87, total: 499
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
│   │   ├── orchestrator.go      # Orchestrator: reasons and delegates (NO tools)
│   │   └── focused.go           # Focused Agent: ReAct loop with tool calling
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
│   │   └── config.go            # YAML config loader with env var expansion
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
├── goagent.yaml                 # Your active config (not committed — in .gitignore)
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksums
├── .gitignore
├── LICENSE
└── README.md                    # This file
```

### What Each File Does

| File | What to edit it for |
|---|---|
| `goagent.yaml` | Change your API keys, add/remove agents, change models |
| `configs/example.yaml` | Reference config — don't edit directly, copy it |
| `pkg/agent/types.go` | Add new fields to agent config (e.g., new properties) |
| `pkg/agent/orchestrator.go` | Change how the orchestrator reasons or delegates |
| `pkg/agent/focused.go` | Change how focused agents run their task loop |
| `pkg/tool/builtins/*.go` | Add new built-in tools or modify existing ones |
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

That's it. Restart GOAgent and the orchestrator will automatically know about the writer agent and can delegate writing tasks to it.

---

## Adding a New Tool

To create a custom tool, create a new file in `pkg/tool/builtins/` that implements the `Tool` interface:

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
        "my_tool":    &builtins.MyTool{},          // ← add this line
    }
}
```

Rebuild, and now any agent with `my_tool` in its `tools` list can use it.

---

## Supported LLM Providers

GOAgent works with any API that speaks the OpenAI `/v1/chat/completions` format:

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

## Roadmap

- [x] Core orchestrator + focused agent pipeline
- [x] Built-in tools (shell, file, HTTP)
- [x] Structured debug logging
- [x] Cross-platform shell support
- [x] Per-agent LLM configuration
- [ ] Skills system (progressive disclosure, max 7 per agent)
- [ ] Memory system (session + persistent)
- [ ] HTTP API server (for mobile/web clients)
- [ ] Agent self-copy (spawn sub-agents)
- [ ] Swarm execution (parallel agents)
- [ ] Heartbeat scheduler (cron-style proactive agents)

---

## License

MIT — use it however you want.

## Contributing

PRs welcome! See the [project structure](#project-structure) section to understand where things live.

---

<div align="center">
  <sub>Built with ❤️ in Go</sub>
</div>
