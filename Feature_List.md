# GOAgent Master Feature List

Feature checklist for the entire GOAgent ecosystem. Each feature has its source, description, and status.

## Legend
- `[x]` Done
- `[/]` In Progress
- `[ ]` Planned

---

## 1. Core Architecture

- [x] **3-Tier Hierarchy** - Boss (Main Orchestrator) delegates to Managers (focused agents), Managers spin up Workers (tool users). Only workers use tools. *(Original)*
- [x] **Focused Agent System** - Named manager agents (scout, operator, browser, researcher, coder) with defined scopes. *(Agent Zero)*
- [x] **Per-Agent LLM Assignment** - Each agent can use a different LLM. Default to primary unless overridden. *(Original)*
- [x] **Shell Safety / Self-Preservation** - Protected paths, denied commands. Agents can't break their own code. *(Original, Agent Zero)*
- [x] **YAML Config** - All agents, tools, and providers defined in `goagent.yaml`. *(Agent Zero)*
- [x] **Mandatory Task System** - Every request creates a checklist. Boss breaks into tasks per manager, managers break into sub-tasks per worker. This is how small LLMs work: everything becomes tiny tasks. *(Original - notes.md)*
- [x] **Single-Call Router** - Script/template that routes prompts to correct managers in one LLM call instead of N calls. Reduces LLM usage. *(Original - notes.md)*
- [ ] **Managers are Mini-Orchestrators** - Focused agents don't use tools directly. They orchestrate workers who do the actual work. *(Original - notes.md)*
- [ ] **Direct Agent Chat** - Tap into any manager agent to chat with it directly, not just through the boss. *(Original - notes.md)*
- [ ] **Auto-Connect Template** - Every agent auto-gets a connection endpoint (SSE/WebTransport) so dashboard/external tools plug in instantly. *(Original - notes.md)*
- [ ] **Ignore List for Critical Agents** - Embed agent, builder agent, read/write managers on `.ignore` so framework can't edit them. *(Original)*
- [ ] **Multi-Project Support** - Only the boss can switch projects. Managers are sandboxed to their assigned project folder. Agent Zero-style project UI. *(Original, Agent Zero)*
- [ ] **Auto Agent Creation** - If no agent exists for a task type, boss creates one on the fly. Tells user what it did but doesn't need permission. *(Original - notes.md)*
- [ ] **7-10 Tool Cap per Agent** - Each agent gets max 7-10 tools. If more needed, split into another agent type. *(Original - notes.md)*
- [ ] **Master Agent Registry** - Lookup table of all agent types and capabilities so the boss can pick the right one. *(Original - notes.md)*
- [ ] **Kanban Task Board** - Drag-and-drop task board for visualizing agent work. ClickUp-style columns (To Do, In Progress, Done, Blocked). Dashboard and CLI views. *(ClickUp Super Agents, Original)*
- [ ] **Script Library (Worker Guides)** - Pre-built scripts that guide workers step-by-step through common tasks. "Holding a worker's hand" through complex operations. Triggerable by agents or events. *(Original - tasks.md)*
- [ ] **Structured Autonomy** - Framework should feel and look fully autonomous to the user, but underneath it's well-structured with guardrails, scripts, and workflows controlling the chaos. *(Original - tasks.md)*
- [ ] **Reflection Loops** - Advanced execution loops where agents constantly reflect on work they're doing. Self-correction before reporting back. *(ClickUp Super Agents)*
- [ ] **Ambient Awareness** - Agents run quietly in background, monitoring context and responding instantly when relevant. Always-on intelligence layer. *(ClickUp Super Agents)*

## 2. Extension & Marketplace System (GOHub)

- [ ] **GOHub Registry** - Public registry for sharing and discovering agent skills, tools, and extensions. Inspired by VS Code Marketplace + OpenClaw's ClawHub. *(ClickUp, OpenClaw ClawHub, VS Code Marketplace)*
- [ ] **Extension System** - Plugin architecture for agents. Install/uninstall skills, tools, and integrations like VS Code extensions. YAML manifest per extension. *(VS Code, OpenClaw ClawHub)*
- [ ] **Linting Extensions** - Plug linting tools (stylelint, eslint, golangci-lint, etc.) directly into agents as extensions. Doctor agent auto-discovers installed linters. *(VS Code stylelint, Original - tasks.md)*
- [ ] **Built-in Lint Framework** - Bake language-aware linting into the framework core. Auto-detect project language and apply correct linter. Results feed into Doctor agent. *(Original - tasks.md)*
- [ ] **ClawHub Integration** - Connect to OpenClaw's ClawHub registry to discover and install community skills. Cross-platform skill sharing. *(OpenClaw ClawHub - https://clawhub.ai/)*
- [ ] **MoltBook Integration** - Connect to MoltBook (Reddit-style platform where agents talk to each other via posts). Create our own agent social network when ready. *(OpenClaw MoltBook - https://www.moltbook.com/)*
- [ ] **Extension Versioning** - Semantic versioning for extensions. Auto-update, rollback, dependency resolution. *(VS Code, npm)*
- [ ] **Extension Templates** - Starter templates for creating new extensions: skill pack, tool adapter, integration plugin, agent profile. *(Original)*

## 3. Agent Types

### Core Agents (Built-in)
- [x] **Scout** - Read-only filesystem agent. *(Original)*
- [x] **Operator** - Write + shell agent. *(Original)*
- [x] **Browser** - Playwright web automation. *(Agent Zero)*
- [x] **Researcher** - HTTP + memory + skills. *(Original)*
- [x] **Coder** - Shell + file read/write. *(Original)*
- [ ] **Doctor (GOBeat)** - Self-healing. 30-min health loop, error-triggered, lint/security scripts per language, fix memory. *(OpenClaw "doctor", Original)*
- [ ] **Builder** - Creates new agents, skills, and tools. On the ignore list. *(Original)*
- [ ] **Embedder** - Dedicated embedding agent. Ships with tiny CPU-only model. *(Original)*
- [ ] **Git Agent** - Commit, push, pull, branch, merge, PRs. GitHub, GitLab, Bitbucket. *(Original)*
- [ ] **Database Agent** - DB operations, migrations, queries, schema. *(Original)*
- [ ] **Teacher Agent** - Uses DOM to show users how to do things: coding, Figma, browser, databases, social media. Interactive tutorials. *(Original - notes.md)*
- [ ] **Research Agent** - Deep research with SearXNG, web scraping, analysis, source handling, comparison tables, guardrails for thin evidence. Structured reports with executive summaries. *(Original, ClickUp Super Agents)*

### Personal Productivity Agents
- [ ] **Note Taking Agent** - Persistent notes with tagging, search, and categorization. Daily journal, idea catcher, memory recall, weekly digest. Accessible from chat, dashboard, and CLI. Links notes to projects and agents. *(Original, ClickUp Super Agents)*
- [ ] **Reminder Agent** - Scheduled reminders delivered via ALL communication plugins (Discord, Telegram, WhatsApp, Email, Slack). Context-aware, habit tracking, smart rescheduling. Recurring reminders, snooze, priority levels. Syncs with calendar if available. *(Original, ClickUp Super Agents)*
- [ ] **Daily Briefer Agent** - Summarizes today's priorities, flags overdue items, compiles mentions waiting on you. Morning Coffee mode for executives. *(ClickUp Super Agents)*
- [ ] **Meeting Agent** - Schedules meetings, drafts agendas, compiles relevant tasks, captures notes, extracts action items into tasks with due dates. End-of-day recap. *(ClickUp Super Agents)*
- [ ] **Personal Assistant Agent** - Reminders, emails, meetings, tasks. Does the work you don't want to do. Write-Like-Me voice matching. *(ClickUp Super Agents)*

### Business & Marketing Agents
- [ ] **Social Media Manager** - Facebook, Instagram, Twitter/X, TikTok, LinkedIn, YouTube. Content planning, draft composition, scheduling, publishing reminders, engagement tracking, keyword watching. *(Original - notes.md, ClickUp Super Agents)*
- [ ] **Marketing Agent** - Campaign management, SEO, content strategy, competitive intel research. *(Original - notes.md, ClickUp Super Agents)*
- [ ] **Sales Agent** - Lead qualification/scoring, outreach, pipeline management, contract renewal, deal desk, demo scheduling + prep + follow-up. *(Original - notes.md, ClickUp Super Agents)*
- [ ] **Customer Service Agent** - Support tickets, FAQ, chat responses, SLA monitoring, customer onboarding, IT service management. AI-drafted support responses. *(Original - notes.md, ClickUp Super Agents)*
- [ ] **Customer Engagement Agent** - Lead/inquiry detection, fast reply drafting, appointment/booking flow, keyword/intent watching across social platforms, conversation tracking, follow-up summaries. *(ClickUp Super Agents)*
- [ ] **Domain Agent** - Enterprise integrations via skill packs: Odoo, GoHighLevel, Zoho, HubSpot, Salesforce, QuickBooks, SAP, Oracle NetSuite, Microsoft Dynamics 365, Freshsales, Insightly, Pipedrive, Keap, ERPNext, Dolibarr, SuiteCRM, Bitrix24, Brevo, Hostinger, Order.co, Acumatica, Jestor, Sage Intacct, Infor SyteLine, Epicor Kinetic, IFS Cloud, Workday. *(Original)*

### Project Management Agents
- [ ] **Project Manager Agent** - Captures goals and scope, tracks project health, generates status reports with progress/blockers/next steps. *(ClickUp Super Agents)*
- [ ] **Sprint Planner Agent** - Organizes backlogs, drafts sprint goals based on team capacity. *(ClickUp Super Agents)*
- [ ] **Task Triage Agent** - Reviews incoming tasks to prioritize, schedule, and auto-fill details. Work breakdown into subtasks with assignees and due dates. *(ClickUp Super Agents)*
- [ ] **Priorities Manager Agent** - Continuously manages priorities, escalating and identifying urgent tasks. Deadline tracking with timely reminders. *(ClickUp Super Agents)*
- [ ] **Follow-Up Agent** - Tracks delegated requests, drafts follow-ups for overdue items, reports back. Ensures nothing slips through. *(ClickUp Super Agents)*

### Operations & Management Agents
- [ ] **Process Automator Agent** - Identifies repetitive behavior and automates it. Invoice routing, approval workflows, procurement. *(ClickUp Super Agents)*
- [ ] **Goal Manager Agent** - Sets OKRs, tracks progress, generates status updates across teams. *(ClickUp Super Agents)*
- [ ] **StandUp Runner Agent** - Collects async updates, summarizes blockers for the team. Retro facilitation. *(ClickUp Super Agents)*
- [ ] **Sentiment Scout Agent** - Aggregates feedback tone from interactions to track satisfaction. Pulse surveys. *(ClickUp Super Agents)*
- [ ] **Workspace Audit Agent** - Identifies unused resources, duplicate items, stale data for cleanup or archiving. *(ClickUp Super Agents)*

### Intelligence & Knowledge Agents
- [ ] **Wiki Upkeep Agent** - Scans and flags outdated docs to keep knowledge fresh. Auto-updates documentation. *(ClickUp Super Agents)*
- [ ] **Project Intelligence Agent** - Builds context on a specific project for generating updates and content. *(ClickUp Super Agents)*
- [ ] **Topic Intelligence Agent** - Builds comprehensive context on a chosen topic to generate summaries, content, and deep insights. *(ClickUp Super Agents)*
- [ ] **Fact Checker Agent** - Verifies claims against trusted web sources. Flags when evidence is thin or conflicting. *(ClickUp Super Agents)*
- [ ] **Competitive Intel Agent** - Tracks competitors and surfaces strategic insights without manual work. *(ClickUp Super Agents)*

### Writing & Content Agents
- [ ] **Copywriter Agent** - Creates tailored, on-brand content for every channel. Brand voice matching. *(ClickUp Super Agents)*
- [ ] **Proofreader Agent** - Cleans typos, tightens sentences, ensures everything reads perfectly. *(ClickUp Super Agents)*
- [ ] **Release Notes Agent** - Transforms tickets/PRs into brand-voiced, user-facing release notes. *(ClickUp Super Agents)*
- [ ] **PRD Writer Agent** - Generates complete product requirement docs from brief ideas and team input. *(ClickUp Super Agents)*
- [ ] **Translator Agent** - Auto-translates task descriptions, comments, and content. *(ClickUp Super Agents)*

### Creative & Dev Agents
- [ ] **Image Agent** - ComfyUI, Stable Diffusion, DALL-E, Adobe Photoshop, Blender, Krita. Concept art pipelines, style bibles, 3D-to-2D bridging, multi-channel asset adaptation, prompt iteration coaching. *(Original, ClickUp Super Agents)*
- [ ] **Diagram Agent** - Draw.io integration via MCP server. Generates flowcharts, architecture diagrams, ERDs, sequence diagrams. Replaces/supplements Mermaid with visual draw.io output. *(Original - tasks.md, draw.io MCP)*
- [ ] **Gamer Agent** - Unreal Engine, Unity, Godot, Roblox, Flax Engine, Defold. Android/iOS/web/browser games. *(Original - notes.md)*
- [ ] **Streaming Agent** - Stream to Twitch, YouTube, Facebook, TikTok, Instagram. *(Original - notes.md)*
- [ ] **Coding Agent** - Website/app builder. Triggers doctor for lint-check after each build. *(Original)*

### Security & Infrastructure
- [ ] **Pen Testing Agent** - White hat security testing, vulnerability scanning. *(Agent Zero profile, Original - notes.md)*
- [ ] **ECAM/Security Systems Agent** - Perimeter cameras, access control, MSU trailers, Morningstar, Cradlepoint, Ubiquiti, Zabbix, Axis, GeoVision, Avigilon. *(Original - notes.md, ECAM specific)*

### Media & Transcription Agents
- [ ] **Social Media Transcription Agent** - Transcribes videos from any social media platform. Uses GOMedia for subtitle extraction, falls back to Whisper for videos without subs. Outputs clean formatted transcripts with timestamps. *(Original)*

### Automation & Platform Agents
- [ ] **Automation Platform Agent (APA)** - Build workflows between apps/APIs. Skills for: Zapier, Make, n8n, Node-RED, IFTTT, UiPath, Power Automate, ServiceNow, Tray.ai, Workato, Gumloop, Lindy AI, Relevance AI, Relay.app, Integrately, Stack AI. *(Original - notes.md)*
- [ ] **Google Products Agent** - Gmail, Drive, Docs, Sheets, Calendar, Meet, Cloud Console, YouTube Studio, Search Console, Ads. *(Original)*
- [ ] **Microsoft Products Agent** - Office 365, Teams, Azure, Outlook, OneDrive, SharePoint, Power BI, Dynamics 365. *(Original)*
- [ ] **Apple Products Agent** - macOS system control, Shortcuts, iCloud, Notes, Reminders, Calendar, Xcode integration. *(Original)*
- [ ] **Android Products Agent** - ADB control, app management, notification relay, file transfer, screen mirroring. Bridges with GOVision. *(Original)*
- [ ] **Windows Products Agent** - PowerShell automation, Windows services, Registry, Task Scheduler, WSL management, Active Directory. *(Original)*
- [ ] **Chromebook Products Agent** - Chrome OS management, Android app sideloading, Linux container (Crostini) control. *(Original)*
- [ ] **Linux Agent** - System administration, package management, service control, cron, systemd, log analysis. *(Original)*

### Smart Home & IoT Agents
- [ ] **Home Automation Agent** - Control smart home devices: Philips Hue (lighting), 8Sleep (mattress), Home Assistant (hub), thermostats, locks, sensors. Voice-controllable. *(OpenClaw integrations)*
- [ ] **Music & Audio Agent** - Spotify playback control, Sonos multi-room audio, Shazam song recognition. *(OpenClaw integrations)*

### NEW: Recommended Agents (My Recommendations)
- [ ] **DevOps Agent** - Docker container management, CI/CD pipelines, Kubernetes, infrastructure as code (Terraform, Pulumi). Monitors deployments, rolls back on failure. *(Recommendation)*
- [ ] **Data Pipeline Agent** - ETL/ELT workflows, data transformation, scheduled data jobs, CSV/JSON/Parquet processing. Connects to databases, APIs, and file stores. *(Recommendation)*
- [ ] **Compliance Agent** - Audits codebase for license compliance (SPDX), accessibility (WCAG), GDPR/privacy requirements. Generates compliance reports. *(Recommendation)*
- [ ] **Incident Response Agent** - Monitors logs/metrics for anomalies, triggers alerts, creates incident timelines, coordinates response across agents. Post-mortem generation. *(Recommendation)*
- [ ] **Migration Agent** - Database migrations, API version upgrades, framework upgrades, dependency updates. Safely handles breaking changes with rollback support. *(Recommendation)*
- [ ] **Cost Optimizer Agent** - Monitors LLM token usage, cloud spending, and resource utilization. Recommends cheaper models, batching strategies, caching opportunities. Auto-switches providers based on cost/quality tradeoffs. *(Recommendation)*
- [ ] **Onboarding Agent** - Walks new users through GOAgent setup, explains agent capabilities, helps configure first project. Interactive tutorial mode. *(Recommendation, ClickUp Super Agents)*
- [ ] **Canvas Agent** - Visual workspace for agents. Drag-and-drop workflow building, A2UI (Agent-to-UI) rendering, collaborative whiteboarding. Research: `steipete/canvas` (Go-based, by OpenClaw creator). *(OpenClaw Canvas, Original - tasks.md)*
- [ ] **Voice Agent** - Voice Wake + Talk Mode. Speech-to-text input, text-to-speech output. Hands-free agent interaction. *(OpenClaw integrations)*
- [ ] **Scheduler Agent** - Cron-style scheduled tasks for agents. "Run this research every Monday at 9am." Time-triggered automation without external tools. *(Recommendation, OpenClaw Cron)*
- [ ] **Credential Agent** - Secure credential management via 1Password, Bitwarden, or built-in vault. Agents request secrets at runtime, never store them in memory. *(OpenClaw 1Password, Recommendation)*
- [ ] **Backup/Export Agent** - Backs up GOAgent's own data: knowledge graphs, memories, configs, project history. Export to portable format, import on new machine. Scheduled auto-backups. *(Recommendation)*

### Industry Domain Agents (Skill Packs via GOHub)
Specialized agents for specific industries. Each loads industry-specific knowledge, templates, and workflows on top of the core Marketing/Sales/Customer Service agents.

- [ ] **Pest Control Agent** - Social media marketing for pest control companies. Post scheduling, seasonal campaign templates (termite season, mosquito season), before/after photo workflows, Google Business review management, local SEO, lead generation landing pages. *(Original)*
- [ ] **Real Estate Agent** - Property listing management, virtual tour scheduling, open house marketing, MLS integration, neighborhood market reports, client follow-up sequences. *(Recommendation)*
- [ ] **Restaurant/Food Service Agent** - Menu management, food photography workflows, Yelp/Google review responses, reservation system integration, seasonal menu marketing, health inspection prep checklists. *(Recommendation)*
- [ ] **Construction/Trades Agent** - Project estimation templates, permit tracking, subcontractor coordination, safety compliance checklists, progress photo documentation, invoice generation. *(Recommendation)*
- [ ] **Healthcare/Medical Agent** - HIPAA-compliant communication, appointment scheduling, patient follow-up sequences, insurance verification workflows, medical record summaries. *(Recommendation)*
- [ ] **Legal Agent** - Contract review/drafting, case timeline management, legal research, client intake forms, billing/time tracking, compliance checklists. *(Recommendation)*
- [ ] **Education/Tutoring Agent** - Curriculum planning, lesson plan generation, student progress tracking, quiz/test creation, parent communication templates, grading assistance. *(Recommendation)*
- [ ] **Fitness/Wellness Agent** - Workout plan generation, nutrition tracking, client progress photos, class scheduling, social media fitness content, membership management. *(Recommendation)*

## 4. Agent Scaling & Parallelism

- [ ] **Worker Spawning** - Managers spin up N workers for parallel execution. E.g. web agent spins up 20 workers for 20 pages simultaneously. *(Original)*
- [ ] **Parallel Execution** - Workers run concurrently, report to their manager, manager reports to boss. *(Original)*
- [ ] **Dynamic Agent Creation from Chat** - User creates new agents via chat or dashboard. *(Original)*
- [ ] **No Permission Required** - Agents don't ask permission. They get the job done and report what they did. *(Original - notes.md)*
- [ ] **Agent Analytics** - Measure productivity across agents, monitor trends, spot top performers. Usage stats per agent. *(ClickUp Super Agents)*
- [ ] **Multi-User / Team Mode** - Multiple users on one GOAgent instance. Role-based access, agent ownership (my agents vs shared), shared knowledge graph with per-user privacy boundaries. *(Recommendation)*

## 5. Memory System

- [x] **Session Memory** - Sliding window of recent messages. *(Agent Zero)*
- [x] **Auto-Reflect** - Agent reflects on completed tasks. *(Agent Zero)*
- [x] **Auto-Entities** - Tracks mentioned entities. *(Agent Zero)*
- [x] **Session Workspace** - Per-session working directory. *(Original)*
- [ ] **Per-Agent Independent Memory** - Each manager has isolated memory. Only results go back to boss. *(Original)*
- [ ] **Knowledge Graph** - Persistent graph memory. Options: [Cayley](https://github.com/cayleygraph/cayley) (Go-native), [Dgraph](https://github.com/dgraph-io/dgraph) (Go-native). Viewable in dashboard. Ships with framework. *(OpenClaw, Original - notes.md)*
- [ ] **Embedded Recall** - Vector similarity search against past conversations/facts. *(OpenClaw)*
- [ ] **Episodic Memory** - Short-term, long-term, and episodic memory layers. Remember what happened, when, and in what context. *(ClickUp Super Agents "Human-level Memory")*
- [ ] **Live Intelligence** - Actively monitors all context to capture and update knowledge bases for people, teams, projects, decisions. Real-time 2-way syncing engine. *(ClickUp Super Agents)*
- [ ] **Infinite Knowledge** - Proprietary real-time syncing with retrieval from fine-tuned embeddings. Enterprise search from connected knowledge across 50+ apps. *(ClickUp Super Agents)*

## 6. LLM Provider System

- [x] **42-Provider Catalog** - All major providers with API endpoints pre-configured. *(Original)*
- [x] **Auto-Discovery** - Calls `/models` on each keyed provider at startup. *(Original)*
- [x] **FailoverClient** - Tries providers in sequence on failure. *(Original)*
- [x] **BudgetClient** - Token spending limits with auto-fallback to cheaper model. *(Original)*
- [/] **GOGateway** - Standalone/embeddable LLM gateway. *(Original, LiteLLM, BricksLLM, Bifrost, Instawork)*
  - [x] Provider adapters (4 API formats: OpenAI, Anthropic, Google, Cohere)
  - [x] Priority/RoundRobin/Latency routing
  - [x] Circuit breaker, rate limiter, content guardrails, cost tracker
  - [x] SSE streaming + standalone CLI binary (9.76 MB)
  - [x] WebTransport support *(quic-go/webtransport-go)*
  - [x] Admin API (provider management, usage reports, circuit breakers)
- [ ] **Optimized Orchestration** - Route to the best model based on intent. Simple queries go to fast/cheap models, complex reasoning goes to powerful models. Auto-classification. *(ClickUp Super Agents "BrainGPT")*
- [ ] **Self-Learning Routing** - Routes improve over time based on success/failure/cost data. Continuous optimization of model selection. *(ClickUp Super Agents)*

## 7. Self-Healing & Monitoring (GOBeat)

- [ ] **30-Minute Health Loop** - Doctor checks framework health on timer. *(OpenClaw "doctor")*
- [ ] **Error-Triggered Activation** - Log error auto-triggers doctor. *(Original)*
- [ ] **Boss Halt/Resume** - Boss pauses on doctor activation. Doctor sends thumbs-up when fixed. *(Original)*
- [ ] **Fix Memory** - Doctor logs what fixed each error. Same error = instant recall. *(Original)*
- [ ] **Pre-made Health Scripts** - Framework checks, lint per language, security scans. *(Original)*
- [ ] **Self-Heal Prompts** - Main agent can detect errors and route them to doctor automatically. *(Original - notes.md)*
- [ ] **Nudge Feature** - Poke agent if stuck. *(Agent Zero)*
- [ ] **Linter Integration** - Auto-detect project language and run appropriate linter (golangci-lint, eslint, stylelint, pylint, etc.) after every code change. Results feed into Doctor. *(Original - tasks.md, VS Code stylelint)*

## 8. Offline Mode & Local Model Marketplace

- [ ] **Offline Toggle** - Settings toggle for fully offline operation. *(Original)*
- [ ] **Ollama Integration** - Auto-detect local Ollama instance, auto-install if missing. Pull models via Ollama CLI. *(Agent Zero)*
- [ ] **Bundled Embed Model** - Ships with `qwen3-embedding` by default *(pending test vs `nomic-embed-text-v2-moe`)*. *(Original)*
- [ ] **Install-Time Model Picker** - During first run, user selects their hardware tier and picks models from the marketplace. GOAgent pulls them via Ollama automatically. *(Original)*
- [ ] **Model Marketplace UI** - Dashboard page showing all available local models organized by role. One-click install/remove. Size, RAM requirements, and quality ratings displayed. *(Original)*
- [ ] **Hardware Auto-Detection** - Detect CPU cores, RAM, GPU presence at startup. Auto-recommend appropriate model tier. *(Original)*
- [ ] **Go Offline Flow** - User downloads desired models while online, then toggles offline mode. GOAgent switches all routing to local models. *(Original)*

### Local Model Catalog (Curated)

#### Embedding Models (pick one, always loaded)
| Model | Size | Quality | Best For |
|-------|------|---------|----------|
| `qwen3-embedding` | ~600MB | Excellent | **Default choice.** Latest Qwen3 embeddings. |
| `nomic-embed-text-v2-moe` | ~275MB | Excellent | MoE architecture, very efficient on CPU. Smallest footprint. |
| `bge-large` | ~670MB | Great | Battle-tested BAAI embedding. Reliable fallback. |
| `mxbai-embed-large` | ~670MB | Great | mixedbread embedding. Strong retrieval performance. |
| `embeddinggemma` | ~800MB | Good | Google quality but larger than alternatives. |

#### Chat / Main Brain Models (pick one based on hardware)
| Model | Params | Size | Min RAM | Tier | Notes |
|-------|--------|------|---------|------|-------|
| `qwen3` 0.6B | 0.6B | ~400MB | 4GB | Nano | Fast router/classifier only. Too small for real tasks. |
| `LFM2` 1.2B | 1.2B | ~800MB | 4GB | Nano | **State-space model.** Fastest CPU inference. Made for edge. |
| `granite-4.0-nano` | 1-2B | ~1.2GB | 4GB | Nano | IBM's latest. Excellent tool-calling at tiny size. |
| `qwen3` 1.7B | 1.7B | ~1.2GB | 4GB | Lite | Solid balance of smart + small. Good tool calling. |
| `ministral-3` | 3B | ~2GB | 8GB | Lite | Mistral's edge model. Good instruction following. |
| `stablelm-zephyr` | 3B | ~2GB | 8GB | Lite | Decent chat. Older but proven. |
| `qwen3` 4B | 4B | ~2.5GB | 8GB | Standard | **Best brain under 3GB.** Smartest small model. |
| `qwen3` 8B | 8B | ~4.5GB | 16GB | Standard | Strong all-rounder. Needs 16GB RAM. |
| `deepseek-r1` 8B distill | 8B | ~4.5GB | 16GB | Standard | Reasoning-focused. Good for complex planning. |
| `qwen3.5` 7B | 7B | ~4GB | 16GB | Pro | Latest Qwen generation. Best quality/size if RAM allows. |
| `qwen3` 14B | 14B | ~8GB | 32GB | Pro | Excellent quality. Needs serious RAM. |

#### Coding Models (swap in when coding agent is active)
| Model | Params | Size | Notes |
|-------|--------|------|-------|
| `granite-4.0-nano` | 1-2B | ~1.2GB | IBM tool-calling + code. Best tiny coder. |
| `qwen2.5-coder` 3B | 3B | ~2GB | Purpose-built for code gen. |
| `qwen2.5-coder` 7B | 7B | ~4.5GB | Strong code gen. Needs 16GB. |
| `deepseek-r1` 8B distill | 8B | ~4.5GB | Reasons through code problems step by step. |

#### Vision Models (for GOVision / image understanding)
| Model | Params | Size | Notes |
|-------|--------|------|-------|
| `LFM2-VL` | 1-2B | ~1.5GB | Liquid vision. Most CPU-efficient. |
| `qwen3-vl` (smallest) | 2-4B | ~2-3GB | Latest Qwen vision. Good accuracy. |

#### Reasoning / Thinking Models (for complex planning)
| Model | Params | Size | Notes |
|-------|--------|------|-------|
| `deepseek-r1` 1.5B distill | 1.5B | ~1GB | Chain-of-thought but limited by size. |
| `deepseek-r1` 8B distill | 8B | ~4.5GB | Strong reasoning. 16GB required. |
| `qwen3` 4B (thinking mode) | 4B | ~2.5GB | Qwen3's built-in thinking mode. |

### Hardware Tier Presets
| Tier | RAM | Example Hardware | Recommended Stack |
|------|-----|-----------------|-------------------|
| **Nano** (4GB) | 4-8GB | Raspberry Pi 5, old laptops | LFM2 1.2B + nomic-embed-v2-moe |
| **Lite** (8GB) | 8-12GB | Budget desktops, Chromebooks | Qwen3 4B + qwen3-embedding |
| **Standard** (16GB) | 12-16GB | **Dell OptiPlex 7060 (Skynet)** | Qwen3 8B + Granite Code + qwen3-embedding |
| **Pro** (32GB+) | 32GB+ | Workstations, gaming PCs | Qwen3 14B + Qwen2.5-Coder 7B + qwen3-embedding |

## 9. GOBrowser (Custom Browser Engine)

- [ ] **GOBrowser Repo** - Custom Go browser engine for agents. Integrated into GOAgent. Research Vercel's browser SDK. *(Original - notes.md)*
- [ ] **DOM Extraction** - Navigate and extract structured data via DOM. *(Original - notes.md)*
- [ ] **Teaching Mode** - DOM-based interactive tutorials. Teacher agent uses this to guide users. *(Original - notes.md)*

## 10. GOVision (Screen Automation Agent)

Inspired by [Open-AutoGLM](https://github.com/THUDM/Open-AutoGLM). Rebuilt from scratch in Go.

- [ ] **Screen Capture Engine** - Take screenshots of desktop/mobile screens. *(Open-AutoGLM)*
- [ ] **Vision Model Integration** - Send screenshots to vision LLM (Gemini, GPT-4V, Qwen-VL) to understand UI state. *(Open-AutoGLM)*
- [ ] **Action Planner** - Plan next click/type/scroll action based on screen understanding + user goal. *(Open-AutoGLM)*
- [ ] **Desktop Executor** - Execute planned actions on Windows/Linux/Mac via OS-native APIs. *(Original)*
- [ ] **ADB Bridge** - Execute actions on Android devices via ADB (Android Debug Bridge). *(Open-AutoGLM)*
- [ ] **Teaching Mode Integration** - Record action sequences as interactive tutorials for the Teacher Agent. *(Original)*

## 11. GOForge (Live Preview + Container Runtime)

Go-native live preview server and lightweight container runtime for agent workloads. Every project built by GOAgent gets a live URL instantly. Separate repo (`GOForge`) but embeddable into GOAgent as a library.

### GitHub Integration Pipeline
- [ ] **Auto-Repo Creation** - Git Agent creates a private GitHub repo via API when a new project starts. *(Original)*
- [ ] **Live Push** - Code is committed and pushed to GitHub after each agent iteration. *(Original)*
- [ ] **Webhook Receiver** - GitHub push webhooks trigger automatic builds and deploys in GOForge. *(Original)*
- [ ] **Branch Previews** - Each branch gets its own preview URL. PRs show deploy previews. *(Vercel/Netlify pattern)*

### Live Preview Server
- [ ] **Project Auto-Detection** - Reads `package.json`, `go.mod`, `index.html`, `requirements.txt`, `Dockerfile` to identify project type. *(Original)*
- [ ] **Builder Engine** - Runs appropriate build command per project type (npm run build, go build, pip install, static serve). *(Original)*
- [ ] **Hot Reload** - New pushes trigger automatic rebuild and reload. User sees changes in seconds. *(Original)*
- [ ] **Reverse Proxy** - Routes `project-name.localhost:PORT` to the correct running process. Clean URLs per project. *(Original)*
- [ ] **Preview Dashboard** - Web UI showing all deployed projects, build status, logs, and URLs. *(Original)*
- [ ] **Deploy API** - Simple REST API for GOAgent to trigger deploys and check status. One call = one deploy. *(Original)*

### Tier 1: Process Isolation (MVP)
- [ ] **Process Groups** - Each project runs in its own process group. Isolated stdout/stderr. *(Original)*
- [ ] **Temp Workspaces** - Clean temporary directory per build. No cross-project contamination. *(Original)*
- [ ] **Process Manager** - Keeps apps alive, restarts on crash, graceful shutdown. *(Original)*
- [ ] **Port Manager** - Auto-assigns ports from a pool. No conflicts. *(Original)*

### Tier 2: Real Containers (Full Isolation)
- [ ] **Linux Namespaces** - PID, network, mount, UTS namespaces via Go `syscall` package for true container isolation. *(Docker pattern)*
- [ ] **Cgroups** - CPU and memory limits per container. Prevents runaway agents from crashing the host. *(Docker pattern)*
- [ ] **Rootfs Management** - Minimal root filesystems per project type (node-slim, go-slim, python-slim). *(Original)*
- [ ] **Agent Sandboxing** - Every agent worker runs inside a container. Bad code can only destroy its own sandbox. Hardware-enforced safety. *(Original)*
- [ ] **Extension Isolation** - Third-party GOHub extensions run in their own containers. No filesystem/API key access to host. *(Original)*
- [ ] **Snapshot & Restore** - Checkpoint a container's state at any point. Restore for debugging. Pairs with StepLogger for time-travel. *(Original)*
- [ ] **OCI Compatibility** - Can pull base images from Docker Hub/GHCR if needed. *(Standard)*
- [ ] **Windows Support** - Hyper-V isolation or WSL2 bridge for Windows hosts. *(Original)*

### Tier 3: Full Platform
- [ ] **Container Networking** - Private networks between containers for multi-service apps (frontend + backend + database). *(Docker Compose pattern)*
- [ ] **Built-in Image Registry** - Store and cache base images locally. No Docker Hub dependency for common images. *(Original)*
- [ ] **Container-to-Container Communication** - Service discovery so containers can talk to each other by name. *(Docker pattern)*
- [ ] **Parallel Worker Pools** - Spin up N containers for N tasks. 20 browser workers = 20 isolated Chromium instances. *(Original)*
- [ ] **Multi-Language Environments** - Go 1.22, Node 22, Python 3.12 all running simultaneously with zero conflicts. *(Original)*
- [ ] **Disposable Workers** - Fire-and-forget containers that auto-cleanup after task completion. *(Original)*
- [ ] **Resource Dashboard** - CPU/memory/network usage per container. Integrated into GODashboard. *(Original)*

### Supported Project Types
- [ ] **Static HTML/CSS/JS** - Direct file serving with live reload. *(Original)*
- [ ] **Vite/React/Vue/Next.js** - `npm install` + `npm run build` + serve dist/. *(Original)*
- [ ] **Go Applications** - `go build` + run binary. *(Original)*
- [ ] **Python/Flask/FastAPI** - `pip install` + run server. *(Original)*
- [ ] **Dockerfile Projects** - If Dockerfile exists, build and run as container. *(Original)*

## 12. Dashboard (GODashboard)

- [x] **Real-time Agent Monitoring** - Live feed of agent activity. *(Agent Zero dashboard)*
- [x] **Chat Interface** - Talk to orchestrator from browser. *(Agent Zero, OpenClaw)*
- [x] **Dark Mode** - Professional dark UI. *(OpenFang dashboard)*
- [ ] **Direct Agent Chat** - Tap into any specific manager agent from dashboard. *(Original - notes.md)*
- [ ] **Agent Creation UI** - Create/edit agents from dashboard. *(Original)*
- [ ] **Project Management UI** - Agent Zero-style project setup via dashboard and CLI. *(Agent Zero, Original - notes.md)*
- [ ] **Provider Management** - Add/remove API keys, see health status. *(Original)*
- [ ] **Cost Dashboard** - Token usage graphs, per-provider spending. *(BricksLLM)*
- [ ] **Knowledge Graph Viewer** - Visual graph exploration in dashboard. *(Original - notes.md)*
- [ ] **Log Viewer** - Real-time structured logs with filtering. *(OpenFang)*
- [ ] **GOBeat Status** - Health check results, doctor activity. *(Original)*
- [ ] **Private Info Manager** - UI for .env variables: API keys, tokens, provider configs. *(Original - notes.md)*
- [ ] **Kanban Board View** - Drag-and-drop task board for agent work items. Columns: Pending, Running, Done, Failed. Per-agent and per-project views. *(ClickUp Super Agents)*
- [ ] **Agent Analytics Dashboard** - Productivity metrics, usage percentile, top performer ranking, milestone tracking. *(ClickUp Super Agents)*
- [ ] **GOHub Browser** - Browse, search, install, and manage extensions from the dashboard. Rating system and reviews. *(VS Code Marketplace, OpenClaw ClawHub)*
- [ ] **Canvas/Whiteboard** - Visual workspace for building agent workflows, diagramming architectures, and collaborative planning. *(ClickUp Whiteboards, OpenClaw Canvas)*
- [ ] **Draw.io Integration** - Embed draw.io diagrams in dashboard for architecture docs, flowcharts, and ERDs. Via draw.io MCP server. *(Original - tasks.md)*
- [ ] **GOForge Deploy Panel** - View all live preview deployments, build logs, and project URLs directly from dashboard. *(Original)*

## 13. Transport & Connectivity

- [x] **SSE Streaming** - Server-Sent Events for universal compatibility. *(Standard)*
- [x] **WebTransport (HTTP/3)** - Primary transport for Go-to-Go communication. Uses `quic-go/webtransport-go`. Reliable streams + unreliable datagrams. *(Original - notes.md)*
- [ ] **Webhook Support** - For n8n, automation platforms. Inbound/outbound webhooks. *(Original - notes.md)*

## 14. Sequential Thinking Engine

- [ ] **Built-in Sequential Thinking** - Go-native implementation of the Sequential Thinking MCP pattern. Improves reasoning and planning for small models. Chain-of-thought with revision loops. *(Anthropic MCP, Original - notes.md)*
- [ ] **Research**: How does the Sequential Thinking MCP work? Build our own Go version. *(Original - notes.md)*

## 15. MCP System

- [ ] **MCP Client** - Connect to external MCP servers. *(Standard)*
- [ ] **MCP Server** - Expose GOAgent tools as MCP server. Written in Go. *(Standard)*
- [ ] **MCP Discovery** - Auto-discover and register from config. *(Standard)*
- [ ] **Bundled MCP Servers** (user activates what they want):
  - [ ] GitHub - repo tasks, code reviews
  - [ ] BrightData - web scraping + data feeds
  - [ ] GibsonAI - serverless SQL management
  - [ ] Notion - workspace + DB automation
  - [ ] Docker Hub - container + DevOps
  - [ ] Browserbase - browser control
  - [ ] Context7 - live code examples + docs
  - [ ] Figma - design-to-code
  - [ ] Reddit - fetch/analyze data
  - [ ] Sequential Thinking - reasoning loops
  - [ ] Draw.io - architecture diagrams, flowcharts, ERDs *(https://github.com/lgazo/drawio-mcp-server)*
  - [ ] 1Password - secure credential access

## 16. Communication Plugins

Expanded to match OpenClaw's 15+ chat provider coverage:

- [ ] **Discord** - Bot integration, chat, commands. *(OpenClaw)*
- [ ] **Telegram** - Bot API via grammY. *(OpenClaw)*
- [ ] **Email** - Send/receive/parse emails. Gmail Pub/Sub triggers. *(OpenClaw)*
- [ ] **WhatsApp** - QR pairing via Baileys. Go CLI via `steipete/wacli`. *(OpenClaw, Original - tasks.md)*
- [ ] **Slack** - Workspace apps via Bolt. *(Standard, OpenClaw)*
- [ ] **Signal** - Privacy-focused via signal-cli. *(OpenClaw)*
- [ ] **iMessage** - Via AppleScript bridge (imsg) or BlueBubbles server. *(OpenClaw)*
- [ ] **Microsoft Teams** - Enterprise support. *(OpenClaw)*
- [ ] **Matrix** - Matrix protocol for decentralized chat. *(OpenClaw)*
- [ ] **Nostr** - Decentralized DMs via NIP-04. *(OpenClaw)*
- [ ] **Nextcloud Talk** - Self-hosted Nextcloud chat. *(OpenClaw)*
- [ ] **Zalo** - Zalo Bot API + personal account via QR login. *(OpenClaw)*
- [ ] **Tlon Messenger** - P2P ownership-first chat. *(OpenClaw)*
- [ ] **WebChat** - Browser-based UI for direct agent interaction. *(OpenClaw)*

## 17. Search Engine

- [ ] **SearXNG Integration** - Self-hosted meta-search using top 10 providers. Ships with framework. *(Original - notes.md)*
- [ ] **Go-native Search** - Research: Can we build a Go version of SearXNG? *(Original - notes.md)*

## 18. Security

- [ ] **Outbound PII Scrubbing** - Scramble ALL private info before it leaves the agent: API keys, tokens, addresses, SSNs, phone numbers, credit cards. Nothing leaks to providers. *(Original - notes.md)*
- [x] **Shell Safety** - Blocked commands, protected paths. *(Original)*
- [x] **PII Detection** - Guardrails middleware scans for SSN, credit cards. *(GOGateway)*
- [ ] **Dashboard Login** - Username/password authentication for GODashboard and API access. Bcrypt password hashing. *(Original)*
- [ ] **2FA / Two-Factor Auth** - TOTP-based second factor using authenticator apps (Google Authenticator, Authy, Microsoft Authenticator). Go-native via `pquerna/otp`. *(Original)*
- [ ] **QR Code Pairing** - On first 2FA setup, generate a QR code the user scans with their authenticator app. Go-native via `skip2/go-qrcode`. *(Original)*
- [ ] **Session Management** - JWT or cookie-based sessions with configurable expiry. Auto-logout on inactivity. *(Original)*
- [ ] **Device Trust** - Remember trusted devices so 2FA isn't needed every login. Revoke trusted devices from dashboard. *(Original)*
- [ ] **API Key Encryption** - Store keys encrypted, not plaintext. *(Best practice)*
- [ ] **Database Encryption** - Encrypt all data at rest. *(Original - notes.md)*
- [ ] **Private Info Manager** - Custom section for secrets stored in .env. Dashboard + CLI can set/view. *(Original - notes.md)*
- [ ] **Security Audit** - Review framework. Study OpenClaw security patches. *(OpenClaw)*
- [ ] **Zero Data Retention** - Never store user data beyond session. Never train on user data. More secure than using OpenAI/Gemini directly. *(ClickUp Super Agents)*
- [ ] **Agentic User Security** - Permission model: implicit access, explicit access, custom permissions. Agents inherit user permissions. Full audit trail of every action. *(ClickUp Super Agents)*

## 19. Integrations & Skills

- [ ] **Figma** - Design-to-code pipeline. *(Original)*
- [ ] **Unreal Engine** - Game dev via MCP. *(Original)*
- [ ] **n8n Workflow Calls** - Agents can trigger n8n workflows via webhooks/WebSocket. *(Original - notes.md)*
- [ ] **No-Code Platform Integration** - Zapier, Make, IFTTT, Power Automate. *(Original - notes.md)*
- [ ] **Draw.io / Diagrams.net** - Architecture diagrams, flowcharts, ERDs. Self-hostable via Docker. *(Original - tasks.md)*
  - Repos: https://github.com/jgraph/drawio, https://github.com/jgraph/docker-drawio, https://github.com/jgraph/drawio-diagrams
- [ ] **Obsidian** - Knowledge graph notes integration. *(OpenClaw)*
- [ ] **Notion** - Workspace and database automation. *(OpenClaw)*
- [ ] **Trello** - Kanban board integration. *(OpenClaw)*
- [ ] **Home Assistant** - Smart home automation hub. *(OpenClaw)*
- [ ] **Spotify** - Music playback control for ambient agent environments. *(OpenClaw)*
- [ ] **Sonos** - Multi-room audio control. *(OpenClaw)*
- [ ] **Shazam** - Song recognition. *(OpenClaw)*
- [ ] **Weather** - Forecasts and conditions. Location-aware alerts. *(OpenClaw)*
- [ ] **Camera** - Photo and video capture from connected cameras/webcams. *(OpenClaw)*
- [ ] **GIF Search** - Find and send the perfect GIF. Integrates into chat and social media agents. *(OpenClaw)*
- [ ] **Peekaboo** - Quick screen capture and share. Lightweight alternative to full GOVision for simple screenshots. *(OpenClaw)*

## 20. Platform Support

- [x] **Windows** *(Primary)*
- [x] **Linux** *(Cross-compile)*
- [ ] **macOS** - Menu bar app + Voice Wake. *(Go cross-compile, OpenClaw)*
- [ ] **iOS** - Canvas, camera, Voice Wake companion app. *(OpenClaw)*
- [ ] **Android** - Canvas, camera, screen companion app. *(OpenClaw)*
- [ ] **Chromebook** *(Linux layer)*
- [ ] **Low-spec Hardware** - Target: old i7 mini PC, 16GB RAM (Dell OptiPlex 7060). *(Original)*

## 21. GOTorch (Go-Native Inference Engine)

Custom Go-native LLM inference runtime. No dependency on Ollama or any external tool. GOAgent loads and runs models by itself.

- [ ] **GGUF Model Loader** - Load quantized GGUF models directly via CGo bindings to `llama.cpp`. No Ollama required. *(Original)*
- [ ] **HuggingFace Model Pull** - Download models directly from HuggingFace Hub. Browse, search, and pull GGUF files by repo name. *(Original)*
- [ ] **Ollama Registry Pull** - Also pull models from the Ollama registry if users prefer that catalog. Best of both worlds. *(Original)*
- [ ] **Local File Support** - Point GOTorch at any local `.gguf` file and it just works. *(Original)*
- [ ] **Model Cache** - Downloaded models stored in `~/.gotorch/models/`. No re-download. Shared across all GOAgent instances on the machine. *(Original)*
- [ ] **Runtime Model Swapping** - Hot-swap models at runtime. Boss says "switch coder to qwen2.5-coder" and it loads in seconds. *(Original)*
- [ ] **Multi-Model Concurrent** - Run multiple models simultaneously (embedding + chat + coding). Memory-aware: only loads what fits in RAM. *(Original)*
- [ ] **CPU Optimized** - AVX2/AVX-512 auto-detection for maximum CPU inference speed. No GPU required. *(Original)*
- [ ] **GPU Acceleration (Optional)** - CUDA, ROCm, Metal support for users who have a GPU. Auto-detect and use if available. *(Original)*
- [ ] **OpenAI-Compatible API** - GOTorch exposes a local `/v1/chat/completions` endpoint so GOGateway and all agents can talk to it like any cloud provider. *(Original)*
- [ ] **Quantization on Download** - Auto-quantize models to Q4_K_M during download if the user's hardware needs it. *(Original)*
- [ ] **Inference Metrics** - Tokens/second, memory usage, model load time. Feed into GODashboard. *(Original)*
- [ ] **Research**: Study `go-skynet/LocalAI`, `mudler/go-llama.cpp`, `ggml-org/llama.cpp` CGo patterns, and `ollama/ollama` internals for architecture inspiration. *(Research)*

### Pure Go Backend (from GoMLX)
- [ ] **GoMLX SimpleGo Fork** - Fork GoMLX's `backends/simplego/` pure Go tensor engine into `pkg/torch/native/`. AVX-512 optimized matmul, convolutions, packed GEMM. Zero CGo deps. *(GoMLX)*
- [ ] **Native Inference Engine** - Run tiny models (<1B params) entirely in pure Go. No shared libraries needed at all. *(GoMLX, Original)*

### LLM Provider System (from LangChainGo)
- [ ] **Unified Model Interface** - Fork LangChainGo's `Model` interface with `GenerateContent()` and `ReasoningModel` support. *(LangChainGo)*
- [ ] **Cloud Provider Clients** - Fork provider implementations: OpenAI, Anthropic, Ollama, Gemini, Mistral, HuggingFace, Cloudflare, Cohere. *(LangChainGo)*
- [ ] **Response Cache Layer** - Wrap any provider with disk/memory cache. Don't re-call identical prompts. *(LangChainGo)*
- [ ] **Error Mapping** - Maps provider-specific errors to common error types. *(LangChainGo)*
- [ ] **Token Counting** - Count tokens without loading a model. *(LangChainGo)*

### Agent Tools (from LangChainGo)
- [ ] **DuckDuckGo Search** - Free web search, no API key. *(LangChainGo)*
- [ ] **Wikipedia Lookup** - Article search and retrieval. *(LangChainGo)*
- [ ] **Web Scraper** - Extract content from web pages. *(LangChainGo)*
- [ ] **SQL Database Tool** - Execute SQL queries against any database. *(LangChainGo)*
- [ ] **Calculator** - Math expression evaluation. *(LangChainGo)*
- [ ] **Perplexity Search** - AI-powered search. *(LangChainGo)*

### Agent Patterns (from Eino)
- [ ] **Stream Processing** - Auto-manage token streams between agent components. Concatenating, boxing, merging streams. *(Eino/ByteDance)*
- [ ] **Interrupt/Resume** - Pause agent execution for human approval, resume from checkpoint. State persistence. *(Eino/ByteDance)*
- [ ] **Callback Aspects** - Inject logging/tracing/metrics at OnStart/OnEnd/OnError hooks across all components. *(Eino/ByteDance)*

### ONNX Support (from Hugot)
- [ ] **ONNX Model Runner** - Run HuggingFace ONNX models for embeddings, classification, text generation. *(Hugot)*
- [ ] **Local Embedding Pipeline** - Generate embeddings locally without API calls using ONNX models. *(Hugot)*

## 22. GOMedia (Go-Native Media Downloader & Transcriber)

Go-native alternative to yt-dlp targeting the top 15 social media platforms. Single binary, no Python dependency. Fork base from `kkdai/youtube` (Go YouTube library) and `horiagug/youtube-transcript-api-go`.

### Core Engine
- [ ] **Platform Abstraction** - Unified `Extractor` interface per platform. Each implements: resolve URL, get metadata, get streams, get subtitles. *(Original)*
- [ ] **Stream Downloader** - HTTP, HLS (m3u8), DASH stream downloading with resume support. Goroutine-based parallel chunk downloads. *(Original)*
- [ ] **Subtitle Extractor** - Pull auto-generated and manual subtitles/captions. Multi-language. VTT/SRT/JSON output. *(Original)*
- [ ] **Audio Extractor** - Download audio-only streams for Whisper fallback transcription. *(Original)*
- [ ] **Whisper Integration** - For videos without subtitles: download audio, transcribe via local Whisper model or Whisper API. *(Original)*
- [ ] **FFmpeg Bridge** - Merge audio+video streams, convert formats. Optional dep for advanced use. *(Original)*
- [ ] **Clean Transcript Output** - Strip timestamps, dedupe repeated lines, format as readable text or structured JSON. *(Original)*

### Platform Extractors (Top 15)
| # | Platform | Auth | Notes |
|---|----------|------|-------|
| 1 | **YouTube** | Cookie/OAuth | Fork `kkdai/youtube`. Subtitles via transcript API |
| 2 | **TikTok** | Cookie | Video + captions extraction |
| 3 | **Instagram** | Cookie/Session | Reels, Stories, IGTV. Login required for private |
| 4 | **X/Twitter** | Cookie/Bearer | Spaces audio, video tweets |
| 5 | **Reddit** | API/Cookie | v.redd.it video extraction |
| 6 | **Facebook** | Cookie | **Private video support via session cookies** |
| 7 | **Twitch** | API | VODs, clips, live streams |
| 8 | **Vimeo** | API/Cookie | Public + private (password-protected) |
| 9 | **LinkedIn** | Cookie | Learning videos, feed videos |
| 10 | **Threads** | Cookie | Meta's Threads platform |
| 11 | **Bluesky** | API | AT Protocol video |
| 12 | **SoundCloud** | API | Audio tracks, podcasts |
| 13 | **Spotify** | Cookie | Podcast episodes (audio) |
| 14 | **Rumble** | Public | Video extraction |
| 15 | **Kick** | API/Cookie | VODs and clips |

### Authentication / Private Content
- [ ] **Cookie Import** - Import browser cookies for authenticated access. Read from Chrome/Firefox/Edge cookie stores. *(yt-dlp pattern)*
- [ ] **Session Token Auth** - Store and reuse session tokens per platform. Encrypted at rest. *(Original)*
- [ ] **Browser Profile Bridge** - Use GOBrowser's dedicated profile for authenticated downloads. Handles 2FA, CAPTCHA. *(Original)*
- [ ] **OAuth Flows** - Where platforms support it (YouTube, Twitch), use proper OAuth for stable access. *(Original)*
- [ ] **Private Video Support** - Facebook private videos, Instagram private accounts, unlisted YouTube via auth cookies. *(Original)*

### CLI & Library
- [ ] **Standalone CLI** - `gomedia download URL`, `gomedia transcript URL`, `gomedia info URL`. *(Original)*
- [ ] **Go Library** - Import as `pkg/media/` in GOAgent. Agents call it directly, no shell-out. *(Original)*
- [ ] **yt-dlp Fallback** - If GOMedia doesn't support a platform, shell out to yt-dlp if installed. Graceful degradation. *(Original)*

## 23. API Server

- [ ] **REST API** - Standalone HTTP API for external apps to interact with GOAgent. Send tasks, query agents, get results. *(Recommendation)*
- [ ] **gRPC API** - High-performance binary protocol for Go-to-Go and mobile app integration. *(Recommendation)*
- [ ] **API Key Auth** - Secure API access with generated API keys. Rate limiting per key. *(Recommendation)*
- [ ] **Webhook Callbacks** - Register webhook URLs. GOAgent calls back when tasks complete. *(Recommendation)*

## 24. Observability & Export

- [ ] **Log Export** - Export structured logs to Grafana/Loki, ELK stack, or plain JSON files. *(Recommendation)*
- [ ] **Trace Export** - OpenTelemetry-compatible traces. Full request lifecycle from user input to agent response. *(Recommendation)*
- [ ] **Metrics Export** - Prometheus-compatible metrics endpoint. Token usage, response times, agent utilization. *(Recommendation)*

---

## Research Queue

Items to investigate before implementation:

- [ ] **steipete/canvas** - Go-based visual workspace by OpenClaw creator. Assess relevance for GOAgent's Canvas Agent / dashboard whiteboard feature. *(https://github.com/steipete/canvas)*
- [ ] **steipete/wacli** - Go-based WhatsApp CLI. Evaluate for WhatsApp communication plugin. *(https://github.com/steipete/wacli)*
- [ ] **openclaw/clawhub** - ClawHub registry source code. Study for GOHub marketplace architecture. *(https://github.com/openclaw/clawhub)*
- [ ] **openclaw/skills** - OpenClaw skills repository. Mine for skill pack patterns and templates. *(https://github.com/openclaw/skills)*
- [ ] **openclaw/lobster** - OpenClaw Lobster (Molty core). Study architecture patterns. *(https://github.com/openclaw/lobster)*
- [ ] **openclaw/openclaw-ansible** - Ansible deployment playbooks. Reference for GOAgent deployment automation. *(https://github.com/openclaw/openclaw-ansible)*
- [ ] **draw.io MCP Server** - MCP server for draw.io diagram generation. *(https://github.com/lgazo/drawio-mcp-server)*
- [ ] **Go container runtimes** - Study containerd, runc, Podman internals for Tier 2 container implementation. All written in Go. *(https://github.com/containerd/containerd, https://github.com/opencontainers/runc)*
- [ ] **gogs/gogs** - Self-hosted Git service in Go. Reference for Git hosting patterns (we'll use GitHub API instead). *(https://github.com/gogs/gogs)*
- [ ] **go-gitea/gitea** - Gogs fork, more active. Reference for Go-based Git server patterns. *(https://github.com/go-gitea/gitea)*
- [ ] **gomlx/gomlx** - Accelerated ML framework for Go. Fork `backends/simplego/` for pure Go tensor engine. *(https://github.com/gomlx/gomlx)*
- [ ] **tmc/langchaingo** - LangChain for Go. Fork LLM provider interface, tools, and cache layer. *(https://github.com/tmc/langchaingo)*
- [ ] **cloudwego/eino** - ByteDance LLM agent framework. Study streaming, interrupt/resume, callback patterns. *(https://github.com/cloudwego/eino)*
- [ ] **knights-analytics/hugot** - ONNX transformer pipelines in Go. Fork for ONNX model support + local embeddings. *(https://github.com/knights-analytics/hugot)*
- [ ] **kkdai/youtube** - Go YouTube downloader. Fork as base for GOMedia YouTube extractor. *(https://github.com/kkdai/youtube)*
- [ ] **horiagug/youtube-transcript-api-go** - Go YouTube transcript extractor. Fork for GOMedia subtitle pipeline. *(https://github.com/horiagug/youtube-transcript-api-go)*
- [ ] **gonum/gonum** - Go numeric libraries (matrices, stats, optimization). Potential dep for tensor math. *(https://github.com/gonum/gonum)*

---

## Sources

| Project | URL | Inspiration |
|---------|-----|-------------|
| Agent Zero | https://github.com/frdel/agent-zero | Orchestrator, agents, browser, memory, nudge, projects |
| OpenClaw | https://github.com/PeterJCLaw/openclaw | Doctor, security, plugins, 50+ integrations |
| OpenClaw ClawHub | https://clawhub.ai/ | Extension marketplace, skill registry |
| OpenClaw MoltBook | https://www.moltbook.com/ | Agent social network (Reddit-style) |
| OpenFang | (dashboard ref) | Dashboard UI |
| ClickUp Super Agents | https://clickup.com/ai/agents | Agent builder, Kanban tasks, agent analytics, ambient awareness |
| VS Code Marketplace | https://marketplace.visualstudio.com/ | Extension system architecture |
| LiteLLM | https://github.com/BerriAI/litellm | Provider translation |
| BricksLLM | https://github.com/bricks-cloud/BricksLLM | Go gateway, cost tracking |
| Instawork | https://github.com/Instawork/llm-proxy | Go provider adapters |
| Plano | https://github.com/katanemo/plano | Orchestration, observability |
| Cayley | https://github.com/cayleygraph/cayley | Go knowledge graph |
| Dgraph | https://github.com/dgraph-io/dgraph | Go graph database |
| quic-go | https://github.com/quic-go/webtransport-go | WebTransport in Go |
| Docker/containerd | https://github.com/containerd/containerd | Go container runtime reference |
| runc | https://github.com/opencontainers/runc | OCI container runtime (Go) |
| Gogs | https://github.com/gogs/gogs | Self-hosted Git in Go |
| Gitea | https://github.com/go-gitea/gitea | Self-hosted Git in Go (Gogs fork) |
| Draw.io MCP | https://github.com/lgazo/drawio-mcp-server | Diagram generation via MCP |
| Canvas (Go) | https://github.com/steipete/canvas | Visual workspace in Go |
| wacli (Go) | https://github.com/steipete/wacli | WhatsApp CLI in Go |
| Vercel Browser | (research needed) | Agent browser engine |
| GoMLX | https://github.com/gomlx/gomlx | Pure Go tensor engine, XLA GPU backend |
| LangChainGo | https://github.com/tmc/langchaingo | LLM providers, tools, cache |
| Eino | https://github.com/cloudwego/eino | ByteDance agent framework, streaming, interrupt/resume |
| Hugot | https://github.com/knights-analytics/hugot | ONNX transformer pipelines in Go |
| Gonum | https://github.com/gonum/gonum | Go numeric/matrix libraries |
| kkdai/youtube | https://github.com/kkdai/youtube | Go YouTube downloader (GOMedia base) |
| yt-transcript-api-go | https://github.com/horiagug/youtube-transcript-api-go | Go YouTube transcript extraction |
