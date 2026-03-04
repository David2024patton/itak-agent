# Architecture

## Overview

GOAgent follows an **orchestrator-delegate** pattern. The core idea: split the AI work into two distinct roles so each one stays simple enough for a 30B model to handle well.

```
┌─────────────────────────────────────────────────────────┐
│                     USER REQUEST                        │
│              "Fetch example.com and save it"             │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│                   ORCHESTRATOR                          │
│                                                         │
│  System prompt with:                                    │
│  • List of available agents + their roles/tools         │
│  • Strict JSON output format                            │
│  • Rule: NEVER use tools directly                       │
│                                                         │
│  Output:                                                │
│  {                                                      │
│    "reasoning": "This needs HTTP + file write...",      │
│    "delegations": [{                                    │
│      "agent": "researcher",                             │
│      "task": "Fetch example.com and save to file",      │
│      "context": "Save as output.html"                   │
│    }]                                                   │
│  }                                                      │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│                 FOCUSED AGENT                           │
│                 (researcher)                             │
│                                                         │
│  ReAct Loop:                                            │
│  ┌──────────────────────────────────────┐               │
│  │ 1. LLM decides what to do           │               │
│  │ 2. Calls a tool (http_fetch)        │ ◄── repeats   │
│  │ 3. Gets tool result                 │     until done │
│  │ 4. Decides: done? or call another?  │               │
│  └──────────────────────────────────────┘               │
│                                                         │
│  Tools available: http_fetch, file_read, file_write     │
│  Max loops: 8 (configurable)                            │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│                   SYNTHESIS                             │
│                                                         │
│  Orchestrator receives the agent's result and creates   │
│  a clean, user-facing response  -  hiding all the         │
│  internal mechanics.                                    │
│                                                         │
│  User sees: "Done! Saved example.com HTML to            │
│  output.html (1.2KB)."                                  │
└─────────────────────────────────────────────────────────┘
```

## Why This Design?

### Problem with Other Frameworks

Most AI agent frameworks (LangGraph, CrewAI, AutoGen) give a single agent access to everything  -  dozens of tools, massive system prompts, and long conversation histories. This works with GPT-4 (200K context) but **falls apart** with smaller models because:

- The system prompt alone can eat 30-50% of the context window
- Tool definitions add thousands of tokens
- The model gets confused choosing between 15+ tools
- Conversation history fills up fast

### GOAgent's Solution

| Design Rule | Why |
|---|---|
| Orchestrator has **zero tools** | Its prompt stays tiny: just agent descriptions + user message |
| Focused agents have **max 7 tools** | Small tool list = accurate tool selection |
| Each agent has **narrow goals** (max 3) | Keeps the agent focused on one type of work |
| **Per-agent LLM** config | Use cheap models for simple tasks, bigger for hard ones |
| Context is **filtered** per agent | Each agent only gets the info it needs |

### The ReAct Loop

Each focused agent runs a **ReAct loop** (Reason → Act → Observe → Decide):

```
┌─────────────────────────────────────────┐
│             FOCUSED AGENT               │
├─────────────────────────────────────────┤
│                                         │
│  Iteration 1:                           │
│    LLM: "I need to fetch the URL"       │
│    Action: http_fetch(url="...")         │
│    Observation: "HTTP 200 <html>..."     │
│                                         │
│  Iteration 2:                           │
│    LLM: "Got the HTML, now save it"     │
│    Action: file_write(path="output.html",│
│            content="<html>...")          │
│    Observation: "Wrote 1234 bytes"       │
│                                         │
│  Iteration 3:                           │
│    LLM: "Task complete!"                │
│    Action: (none  -  returns result)      │
│                                         │
└─────────────────────────────────────────┘
```

The loop continues until either:
- The agent gives a **final answer** (no tool calls)
- **Max loops** is reached (safety limit)
- An **error** occurs

## Data Flow

```
User Message
    │
    ▼
Orchestrator LLM Call
    │  (no tools, just delegation JSON)
    │
    ├──► Agent A LLM Call
    │       │  (with tools)
    │       ├──► Tool Execution
    │       ├──► Tool Result
    │       └──► Agent A Result
    │
    ├──► Agent B LLM Call
    │       │  (with tools)
    │       └──► Agent B Result
    │
    ▼
Synthesis LLM Call
    │  (combines all results)
    │
    ▼
Final Response to User
```

**LLM calls per request:** Minimum 2 (orchestrator + synthesis), typically 4-6 (orchestrator + 1-2 agents with 1-2 tool loops each + synthesis).

## Key Components

| Component | File | Responsibility |
|---|---|---|
| **Orchestrator** | `pkg/agent/orchestrator.go` | Receives user input → calls LLM → parses delegation JSON → routes to agents → synthesizes results |
| **Focused Agent** | `pkg/agent/focused.go` | Receives task → runs ReAct loop → calls tools → returns result |
| **Types** | `pkg/agent/types.go` | AgentConfig, TaskPayload, Delegation, Result structs |
| **LLM Client** | `pkg/llm/client.go` | HTTP client for OpenAI-compatible APIs |
| **Tool Interface** | `pkg/tool/interface.go` | `Name()`, `Description()`, `Schema()`, `Execute()` |
| **Tool Registry** | `pkg/tool/registry.go` | Stores tools, generates LLM function-calling schemas |
| **Config** | `pkg/config/config.go` | YAML parsing, env var expansion, validation |
| **Debug Logger** | `pkg/debug/logger.go` | Structured logging with levels and colors |
| **CLI** | `cmd/goagent/main.go` | Parses flags, initializes everything, runs REPL |
