# iTaK Agent

**Your personal AI-powered automation engine.** Chat with intelligent agents, orchestrate tasks, explore your knowledge base, and connect to any LLM provider, all from a single self-hosted dashboard.

iTaK Agent runs entirely on your hardware. Your data never leaves your machine.

---

## Install

### One-Line Install (recommended)

**Linux / macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/David2024patton/itak-agent/main/install.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/David2024patton/itak-agent/main/install.ps1 | iex
```

This downloads the latest binary, installs it to your system, and adds it to PATH so you can run `itak-agent` from any terminal.

### Direct Download

Download pre-compiled binaries from [GitHub Releases](https://github.com/David2024patton/itak-agent/releases):

| Platform | Architecture | Download |
|----------|-------------|----------|
| **Windows** | x64 | `itak-agent-windows-amd64.exe` |
| **Windows** | ARM64 | `itak-agent-windows-arm64.exe` |
| **macOS** | Intel | `itak-agent-darwin-amd64` |
| **macOS** | Apple Silicon | `itak-agent-darwin-arm64` |
| **Linux** | x64 | `itak-agent-linux-amd64` |
| **Linux** | ARM64 | `itak-agent-linux-arm64` |

### Run

```bash
# Start the agent (opens dashboard on port 42800)
itak-agent --port 42800

# Open in your browser
http://localhost:42800
```

### Docker

```bash
git clone https://github.com/David2024patton/itak-agent.git
cd itak-agent
docker compose up -d
```

### Build from Source

```bash
git clone https://github.com/David2024patton/itak-agent.git
cd itak-agent
go build -o itak-agent ./cmd/itakagent
./itak-agent --port 42800
```

---

## Supported LLM Backends

iTaK Agent works with any OpenAI-compatible API. Connect your preferred local or cloud model:

| Backend | Type | Config |
|---------|------|--------|
| **Ollama** | Local | `http://localhost:11434/v1` |
| **LM Studio** | Local | `http://localhost:1234/v1` |
| **iTaK Torch** | Local | `http://localhost:8080/v1` |
| **vLLM** | Local/Server | `http://localhost:8000/v1` |
| **LocalAI** | Local | `http://localhost:8080/v1` |
| **text-generation-webui** | Local | `http://localhost:5000/v1` |
| **llama.cpp server** | Local | `http://localhost:8080/v1` |
| **TabbyAPI** | Local | `http://localhost:5000/v1` |
| **Jan** | Local | `http://localhost:1337/v1` |
| **OpenAI** | Cloud API | `https://api.openai.com/v1` |
| **Anthropic** | Cloud API | `https://api.anthropic.com/v1` |
| **Google Gemini** | Cloud API | `https://generativelanguage.googleapis.com/v1beta` |
| **Mistral** | Cloud API | `https://api.mistral.ai/v1` |
| **Groq** | Cloud API | `https://api.groq.com/openai/v1` |
| **Together AI** | Cloud API | `https://api.together.xyz/v1` |
| **DeepSeek** | Cloud API | `https://api.deepseek.com/v1` |

Configure in `config.yaml`:

```yaml
orchestrator:
  llm:
    provider: ollama          # or: openai, anthropic, google, etc.
    api_base: http://localhost:11434/v1
    model: llama3.2
    api_key: ""               # only needed for cloud APIs
```

---

## Features

### Chat
Multi-persona AI chat with file attachments, vision support, and knowledge base integration.

### Agents
Deploy specialized autonomous agents (researcher, coder, browser, voice) that work independently on complex tasks with tool calling and memory.

### Task Board
Kanban-style task management with priority scheduling, auto-execution, and agent assignment.

### Knowledge Base
Upload documents, ingest repos, and build a searchable knowledge graph. Agents automatically reference your knowledge during conversations.

### Database Explorer
Visual graph explorer with SQL queries. See how your knowledge nodes connect.

### Automations
Cron-based job scheduler. Set agents to run on recurring schedules.

### Connectors
Extensible connector framework for integrating external APIs. Includes a Stripe reference connector.

---

## Architecture

```
iTaK Agent
├── cmd/itakagent/       # CLI entry point
├── pkg/
│   ├── agent/           # Orchestrator, tool registry, agent lifecycle
│   ├── api/             # HTTP server, REST endpoints, WebSocket
│   ├── config/          # YAML config loader
│   ├── llm/             # LLM client (OpenAI-compatible), vision
│   ├── memory/          # Knowledge graph (BoltDB backend)
│   ├── tasks/           # Task manager, execution engine
│   └── tools/           # Built-in tools (web search, file ops, code exec)
├── web/                 # Dashboard SPA (HTML/CSS/JS, embedded in binary)
└── Dockerfile           # Multi-stage build
```

Single binary. No external database required. Everything stored in BoltDB.

---

## Enterprise: iTaK Agent Cloud

Need more? **iTaK Agent Cloud** adds the full business automation suite on top of everything above:

### Agency CRM
Full contact management, deal pipelines, conversation inbox, and sub-account management. Everything you need to run a client-facing agency.

### 10+ Payment Gateways
Stripe, PayPal, Square, Zelle (J.P. Morgan), GoCardless, Authorize.net, and more. Invoicing, subscription management, and balance tracking built in.

### Phone System
Twilio, Vonage, Plivo. SMS messaging, call logs, number management, and AI-powered voice agents.

### Integrated Business Tools
- **Sites & Funnels** - Landing page builder
- **Social Planner** - Multi-platform scheduling
- **Reputation Management** - Review monitoring
- **Brand Boards** - Visual brand guidelines
- **Memberships** - Course & community management
- **Workflow Automation** - Multi-step business process flows
- **Media Library** - Centralized asset management

### Multi-User & Team
User registration, tiered access (Starter/Pro/Agency), team management, and per-user data isolation with security sandboxing.

**[Contact us for Enterprise access](mailto:david@itak.live)**

---

## Configuration

| Env Variable | Default | Description |
|---|---|---|
| `ITAK_API_PORT` | `42800` | Dashboard + API port |
| `ITAK_DATA_DIR` | `./data` | Data storage directory |
| `ITAK_PASSWORD` | *(none)* | Owner password (locks dashboard access) |

---

## License

MIT
