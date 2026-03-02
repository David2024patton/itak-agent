# GOAgent

**A lightweight Go-based AI agent framework where a small orchestrator delegates to focused sub-agents.**

Built to run efficiently on 30B models like NVIDIA Nemotron — no $200/mo required.

---

## Architecture

```
                    ┌─────────────────────────┐
                    │   ORCHESTRATOR AGENT    │
                    │  (reasons & delegates)  │
                    │  ⛔ NO tools            │
                    └───┬──────┬──────┬───────┘
                        │      │      │
                   ┌────▼──┐ ┌─▼───┐ ┌▼─────┐
                   │Agent A│ │  B  │ │  C   │
                   │(Coder)│ │(Res)│ │(YT)  │
                   │tools: │ │     │ │      │
                   │shell  │ │http │ │http  │
                   │file   │ │file │ │file  │
                   └───────┘ └─────┘ └──────┘
```

**Core principles:**
- **Orchestrator never touches tools** — it only reasons and delegates
- **Focused agents** each have max 7 skills, clear goals, and a personality
- **Per-agent LLM** — each agent can use a different model/provider
- **Heartbeat** — agents can run proactively on a cron schedule
- **Self-copy** — focused agents can spawn sub-agents and swarms
- **Progressive disclosure** — skills loaded on-demand to keep context small

## Quick Start

```bash
# Build
go build -o goagent ./cmd/goagent/

# Create your config
cp configs/example.yaml goagent.yaml
# Edit goagent.yaml with your API keys

# Run
./goagent run
```

## Configuration

```yaml
orchestrator:
  llm:
    provider: nvidia_nim
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}

agents:
  - name: researcher
    role: Senior Research Analyst
    personality: "Thorough researcher who verifies sources"
    goals: [accuracy, comprehensiveness]
    tools: [http_fetch, file_read, file_write]

  - name: coder
    role: Software Developer
    personality: "Pragmatic developer who writes clean code"
    goals: [correctness, readability]
    tools: [shell, file_read, file_write]
```

## Built-in Tools

| Tool | Description |
|---|---|
| `shell` | Execute shell commands |
| `file_read` | Read file contents |
| `file_write` | Write content to files |
| `http_fetch` | Make HTTP GET/POST requests |

## License

MIT

## Contributing

PRs welcome. See [GitHub](https://github.com/David2024patton/GOAgent).
