# Configuration Reference

Everything in GOAgent is controlled by a single YAML file. By default, GOAgent looks for `goagent.yaml` in the current directory, but you can pass any path:

```bash
goagent run                      # uses ./goagent.yaml
goagent run my-config.yaml       # uses my-config.yaml
goagent run configs/prod.yaml    # uses configs/prod.yaml
```

## Environment Variables

Any value in the config can reference an environment variable with `${VAR_NAME}`:

```yaml
api_key: ${NVIDIA_API_KEY}        # expanded at load time
api_base: ${CUSTOM_API_URL}       # works for any field
```

Set the variable before running:

```bash
# Linux/macOS
export NVIDIA_API_KEY="nvapi-xxxxx"
./goagent run

# Windows (PowerShell)
$env:NVIDIA_API_KEY = "nvapi-xxxxx"
.\goagent.exe run

# Windows (CMD)
set NVIDIA_API_KEY=nvapi-xxxxx
goagent.exe run
```

## Complete Config Reference

```yaml
# ── ORCHESTRATOR ─────────────────────────────────────────────────────
orchestrator:
  llm:
    provider: string        # Label for your reference (e.g., "nvidia_nim", "ollama")
    model: string           # REQUIRED. Model name (e.g., "nvidia/nemotron-3-nano-30b-a3b")
    api_base: string        # REQUIRED. API endpoint URL
    api_key: string         # API key (empty for local models like Ollama)
  system_prompt: string     # Optional. Extra instructions prepended to the orchestrator prompt
  max_delegations: int      # Optional. Max agents per request (default: 5)

# ── AGENTS ───────────────────────────────────────────────────────────
agents:                     # REQUIRED. At least one agent must be defined
  - name: string            # REQUIRED. Unique identifier (lowercase, no spaces)
    role: string            # REQUIRED. Job title shown to the orchestrator
    personality: string     # REQUIRED. How this agent "thinks" and communicates
    goals: [string]         # Optional. Max 3 narrow KPIs (e.g., accuracy, speed)
    tools: [string]         # Optional. List of tool names from the catalog
    max_loops: int          # Optional. Max ReAct iterations (default: 10)
    max_skills: int         # Optional. Max skills (default: 7, hard cap: 7)
    heartbeat: string       # Optional. Cron expression (future feature)
    skills_dir: string      # Optional. Path to skills directory (future feature)
    data: [string]          # Optional. Paths to reference data files (future feature)

    llm:                    # Optional. Per-agent LLM override
      provider: string      # If omitted, inherits from orchestrator
      model: string
      api_base: string
      api_key: string

# ── INTEGRATIONS ─────────────────────────────────────────────────────
integrations:               # Optional. External API connections
  service_name:
    api_key: string
    endpoint: string
```

## Config Validation Rules

When GOAgent loads your config, it checks for these errors:

| Rule | Error Message |
|---|---|
| No orchestrator model | `orchestrator.llm.model is required` |
| No agents defined | `at least one agent must be defined` |
| Agent without name | `every agent must have a name` |
| Duplicate agent names | `duplicate agent name: X` |
| Skills > 7 | `agent "X": max_skills cannot exceed 7` |

## Common Configurations

### Minimal (one agent, NVIDIA NIM)

```yaml
orchestrator:
  llm:
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}

agents:
  - name: worker
    role: General Assistant
    personality: "Helpful assistant that completes tasks efficiently"
    tools: [shell, file_read, file_write, http_fetch]
```

### Local Only (Ollama, no API keys needed)

```yaml
orchestrator:
  llm:
    model: llama3.1:8b
    api_base: http://localhost:11434/v1
    api_key: ""

agents:
  - name: assistant
    role: General Assistant
    personality: "Helpful and thorough"
    tools: [shell, file_read, file_write]
```

### Multi-Model (different LLM per agent)

```yaml
orchestrator:
  llm:
    model: nvidia/nemotron-3-nano-30b-a3b
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}

agents:
  - name: researcher
    role: Research Analyst
    personality: "Thorough researcher"
    tools: [http_fetch, file_read, file_write]
    llm:
      model: gpt-4o-mini
      api_base: https://api.openai.com/v1
      api_key: ${OPENAI_API_KEY}

  - name: coder
    role: Software Developer
    personality: "Pragmatic developer"
    tools: [shell, file_read, file_write]
    llm:
      model: codellama:13b
      api_base: http://localhost:11434/v1
      api_key: ""
```

## How Config Inheritance Works

If an agent doesn't specify its own `llm` section, it **inherits** the orchestrator's LLM config:

```yaml
orchestrator:
  llm:
    model: nvidia/nemotron-3-nano-30b-a3b    # ← this model
    api_base: https://integrate.api.nvidia.com/v1
    api_key: ${NVIDIA_API_KEY}

agents:
  - name: researcher
    # No llm section → uses nvidia/nemotron-3-nano-30b-a3b automatically

  - name: coder
    llm:
      model: codellama:13b                    # ← overrides to local Ollama
      api_base: http://localhost:11434/v1
      api_key: ""
```

## Security Notes

- **Never commit API keys** to git. Use `${ENV_VAR}` references
- `goagent.yaml` is in `.gitignore` by default  -  your live config stays local
- `configs/example.yaml` uses `${ENV_VAR}` placeholders and is safe to commit
- The `shell` tool can execute **any command**  -  only deploy to trusted environments
