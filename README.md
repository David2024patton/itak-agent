<div align="center">
  <img src="docs/images/logo.png" alt="iTaK Agent Logo" width="280"/>
  <h1>iTaK Agent</h1>
  <p><strong>An AI agent framework written in Go. One boss delegates to focused agents who get work done.</strong></p>
  <p>Runs on small local models. No expensive API bills required.</p>

  <br/>

  ![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)
  ![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)
  ![Version](https://img.shields.io/badge/Version-0.3.0-blue?style=for-the-badge)
  ![Platform](https://img.shields.io/badge/Platform-Windows%20|%20Linux%20|%20macOS-lightgrey?style=for-the-badge)

</div>

> [!IMPORTANT]
> The **iTaK Agent** is a fully bundled AI ecosystem. While it features an orchestrator (**iTaK Core**), advanced graph memory (**iTaK Memory**), robust security guardrails (**iTaK Shield**), and a full task management system (**iTaK Tasks**) neatly embedded into a single binary with a native HTML dashboard, **each of these sub-systems is built as an individual, reusable component**. This means you can use the iTaK Agent out-of-the-box, or extract its components for your own standalone projects!

---

## Table of Contents

- [Dashboard UI](#-dashboard-ui)
- [Task Management System](#-task-management-system--ai-orchestrated-kanban)
- [Knowledge Pipeline](#-knowledge-pipeline)
- [Memory System](#-memory-system)
- [Plugin Architecture](#-plugin-architecture)
- [MCP Support](#-model-context-protocol-mcp)
- [What Is iTaK Agent?](#-what-is-itak-agent)
- [How It Works](#-how-it-works)
- [Quick Start](#-quick-start)
- [Configuration Guide](#-configuration-guide)
- [Built-in Tools](#-built-in-tools)
- [iTaK Shield (Security Guardrails)](#-itak-shield-security-guardrails)
- [GPU Compute Engine](#-gpu-compute-engine)
- [Model File Parsers](#-model-file-parsers)
- [Cron Scheduler](#-cron-scheduler)
- [Code Indexer](#-code-indexer)
- [Agency Catalog](#-agency-catalog)
- [Debug Mode](#-debug-mode)
- [Project Structure](#-project-structure)
- [Extending the Agent](#-extending-the-agent)
- [Supported LLM Providers](#-supported-llm-providers)
- [Ecosystem](#-the-itak-agent-ecosystem)
- [Roadmap](#-roadmap)

---

## Dashboard UI

iTaK Agent features a real-time dashboard served directly from the embedded Go binary. The sidebar organizes 21 pages across 8 groups. Every page is documented below.

<div align="center">
  <img src="assets/screenshots/overview_page_1773254697715.png" alt="Overview Dashboard" width="800" />
</div>

### Chat

<details>
<summary><strong>Chat</strong> - Embedded conversational interface</summary>

- Multi-agent persona selector (chat with any registered agent individually)
- Full markdown rendering in responses (code blocks, tables, lists)
- Streaming response display with typing indicator
- Session-aware context (conversations persist across page navigations)
- Message input with Enter-to-send and Shift+Enter for newlines
- Auto-scroll to latest message
- Copy-to-clipboard on code blocks
- Agent role badge displayed next to each response
- WebSocket-powered real-time streaming
- Project auto-creation on new chat session (links to task board)

</details>

### Monitor

<details>
<summary><strong>Overview</strong> - Real-time system health dashboard</summary>

- Live system memory usage (allocated, system, GC stats)
- Goroutine count with trend indicators
- Active agent count and availability status
- Auto-healing Doctor status (up/degraded/down)
- Uptime display
- Quick-access metric cards with sparkline trends
- Connection status indicator (green dot = connected, red = disconnected)
- WebSocket heartbeat monitoring
- File system activity stream with live event feed
- System resource gauges

</details>

<details>
<summary><strong>Analytics</strong> - Token usage and performance metrics</summary>

- Per-model token usage breakdown (prompt tokens, completion tokens, total)
- Real-time generation speed tracking (tokens/second)
- Cost estimation per model and per agent
- Usage over time charts
- Agent-level performance comparison
- Session duration analytics
- Tool call frequency breakdown
- Error rate tracking per agent
- Response latency histograms

</details>

<details>
<summary><strong>Logs</strong> - Live structured log viewer</summary>

- Real-time log streaming from the debug ring buffer
- Level-based filtering (DEBUG, INFO, WARN, ERROR)
- Color-coded severity levels
- Timestamp display with millisecond precision
- Component/module source labels
- Auto-scroll with pause-on-hover
- Search/filter within visible logs
- Log export capability
- JSONL trace file integration

</details>

### Agents

<details>
<summary><strong>Sessions</strong> - Persistent conversation workspaces</summary>

- List all active and archived sessions
- Session metadata (created_at, last_active, message_count)
- Agent assignment per session
- Session context preview (last few messages)
- Archive/restore session controls
- Session-to-project linking
- Memory usage per session
- Workspace isolation indicators
- Click-to-resume any previous session

</details>

<details>
<summary><strong>Agents</strong> - Agent roster and configuration viewer</summary>

- All registered agents with name, role, and personality
- Tool assignment list per agent (which tools each agent can use)
- Max loops configuration display
- LLM provider and model per agent
- Goal list display
- Agent status (idle, working, error)
- Agent card layout with role badges
- Quick-chat button to jump into conversation with that agent
- Custom LLM override indicators

</details>

### Tasks

<details>
<summary><strong>Task Board</strong> - AI-orchestrated Kanban board</summary>

- Drag-and-drop Kanban columns (Todo, In Progress, Blocked, Review, Done, Failed, Escalated)
- Priority color-coded card borders (green=low, blue=medium, orange=high, red=urgent)
- Priority icons and labels on each card
- Overdue detection with red border warnings
- Sub-item checklist progress bars on cards
- Label badges with distinct colors
- Due date badges with calendar icons
- Assigned agent display on cards
- Comment count badges
- Attachment count badges (paperclip icon)
- Review pass/fail indicators on cards
- Dependency chain indicators (blocker count)
- **Run button** on Todo cards for one-click AI execution
- Project tabs grouped by agency with Auto/Manual toggle
- Approval queue button with pending count badge
- **New Task button** opening enhanced creation modal
- Task detail slide-out panel (click any card)
- Drag-to-reorder within and between columns

**Task Creation Modal:**
- Title and description fields
- "Let AI Handle This" checkbox (green highlight, auto-dispatches on save)
- Priority selector (Low/Medium/High/Urgent)
- Labels input (comma-separated, color-coded)
- Due date picker
- Project dropdown (populated from active projects)
- Agent dropdown (populated from registered agents, "Auto" default)
- Drag-and-drop file attachment zone (images, docs, code - 32MB limit)
- File staging with preview and remove
- Collapsible **AI Execution Settings** section:
  - System prompt textarea (per-task agent instructions)
  - Expected output format field (JSON, markdown, file path)
  - Autonomy level dropdown (Draft Only / Supervised / Autonomous)
  - Max retries spinner (0-10)
  - Max tokens limit
  - Timeout (TTL) in seconds
  - Allowed tools whitelist (comma-separated)
  - Context references (graph node IDs, document paths)
- "Save Task" and "Save & Run" buttons
- Checklist sub-items with add/remove/toggle

**Task Detail Panel:**
- Full status/priority/agent header with edit controls
- Description block
- Checklist with completion tracking and progress bar
- Dependency tree (blocked by / blocks with clickable links)
- Auto-review results (pass/fail with detailed issue list)
- Progress bar (0-100% with animated green fill)
- AI Configuration summary (autonomy, retries, tokens, timeout, prompt preview, tools)
- Reasoning Scratchpad (collapsible chain-of-thought log with timestamps, steps, tool calls, results)
- "Run with AI" full-width execute button (for Todo tasks)
- Activity history timeline with dot indicators
- Comments section with inline posting

</details>

<details>
<summary><strong>Presentations</strong> - Slide deck viewer and manager</summary>

- Presentation listing and management
- Slide preview cards
- Full-screen presentation mode
- Agent-generated slide decks from task output
- Import/export presentation data

</details>

<details>
<summary><strong>Projects</strong> - Project workspace manager</summary>

- Project listing with session binding
- Auto-create project on new chat session
- Auto Mode toggle (AI-managed vs manual board)
- Project-to-agency grouping
- Task count per project
- Active session indicator
- Project metadata (created, last activity)
- Click to filter task board by project
- Project CRUD via API

</details>

### Agency

<details>
<summary><strong>Agencies</strong> - Pre-built agent team catalog</summary>

- Browse 30KB catalog of pre-configured agent teams
- Agency cards with name, description, and agent roster
- One-click deploy an entire agency (spawns all agents)
- Agent count and tool summary per agency
- Category filtering (research, development, content, data)
- Custom agency creation form
- Agency-to-project assignment

</details>

<details>
<summary><strong>Credentials</strong> - API key and secret management</summary>

- Secure credential storage for LLM providers
- API key listing with masked values
- Add/edit/delete credentials
- Provider-specific fields (api_base, api_key, organization)
- Environment variable reference support (${VAR_NAME})
- Per-agent credential assignment
- Test connection button

</details>

<details>
<summary><strong>Contacts</strong> - Contact database</summary>

- Contact listing with search and filter
- Contact cards with name, email, role
- Add/edit/delete contacts
- Contact-to-project association
- Agent assignment per contact
- Communication history per contact

</details>

<details>
<summary><strong>Pipelines</strong> - Data pipeline monitoring</summary>

- Pipeline status dashboard
- Stage-by-stage progress tracking
- Error and retry monitoring
- Pipeline execution history
- Trigger configuration
- Input/output data flow visualization
- Pipeline templates

</details>

<details>
<summary><strong>Workflows</strong> - Visual workflow builder (n8n-style)</summary>

- Drag-and-drop canvas editor
- **18 node types**: Prompt, Agent, Webhook, API Call, WebSocket, Condition, Transform, Delay, Loop, Merge, Code, Error Handler, Schedule, DB Query, Email, Notification, Approval, Note
- Live wire dragging with real-time bezier curve preview
- Conditional ports: True (green) and False (red) output ports
- Multi-select: Ctrl+click or Shift+drag rubber band selection
- Snap-to-grid toggle for precise alignment
- Auto-layout: one-click DAG arrangement (left-to-right)
- Copy/paste: Ctrl+C/V with edge preservation
- Import/export workflow definitions as JSON
- Execution animation: nodes glow green sequentially during runs
- Workflow variables panel: define global {{variables}}
- 4 built-in templates: Social Media Monitor, Research Pipeline, Data ETL, Approval Flow
- Right-click context menu (Test, Duplicate, Copy, Delete)
- Minimap navigation for large workflows
- Scroll zoom and +/- buttons
- Node property panel with configuration forms
- Edge labels and annotations

</details>

### Marketplace

<details>
<summary><strong>Marketplace</strong> - Extension and skill marketplace</summary>

- Browse available skills, tools, and agent templates
- Category-based filtering
- Skill cards with description, author, and rating
- One-click install/enable
- Installed skills management
- Skill configuration panels
- Version tracking and update notifications
- Community-contributed extensions

</details>

### Automations

<details>
<summary><strong>Scheduler</strong> - Cron-based task automation</summary>

- Create recurring tasks with cron expressions
- Visual cron expression builder
- Next-run preview
- Execution history per schedule
- Enable/disable toggle
- Task template association
- Agent assignment for automated execution
- Error notification on failed runs

</details>

<details>
<summary><strong>Calendar</strong> - Task timeline and scheduling view</summary>

- Month/week/day calendar views
- Task cards placed on due dates
- Drag-to-reschedule tasks
- Color-coded by priority
- Overdue task highlighting
- Recurring task indicators
- Click-to-create task on date
- Filter by project or agent

</details>

<details>
<summary><strong>Reports</strong> - Automated report generation</summary>

- Pre-built report templates (task completion, agent performance, token usage)
- Date range selection
- Export to PDF/JSON
- Agent productivity metrics
- Task throughput analysis
- Cost breakdown reports
- Trend visualizations
- Scheduled report delivery

</details>

### Database

<details>
<summary><strong>Explorer</strong> - Database table browser</summary>

- Multi-engine data explorer (Graph, Vector, Table, Full-Text)
- Node/record listing with pagination
- Property viewer with edit capability
- Relationship visualization
- Query builder interface
- Import/export data
- Schema inspection
- CRUD operations on records

</details>

<details>
<summary><strong>Knowledge</strong> - Knowledge base management</summary>

- List all ingested knowledge bases
- Repository ingestion form (GitHub/GitLab/Bitbucket/Codeberg URL)
- Ingestion progress tracking
- Unified search across all 4 engines (Graph + Vector + Table + FTS)
- Auto-documentation viewer (per-node generated docs)
- Dependency audit tool
- File type distribution statistics
- Hierarchical Graph Explorer (3-level drill-down):
  - Level 1: Template hubs and top-level nodes
  - Level 2: Type clusters (Script x141, Config x7, etc.)
  - Level 3: Individual files with metadata panels
  - Breadcrumb navigation with Escape-to-go-back
  - Glow effects on hub nodes, dashed import edges
  - Right-click context menu (edit, link, expand, delete)
  - Side panel with full node metadata and relationships

</details>

### System

<details>
<summary><strong>Settings</strong> - Application configuration</summary>

- Theme toggle (dark/light mode)
- API endpoint configuration
- LLM provider settings
- Debug mode toggle
- System information display
- WebSocket reconnection settings
- Sidebar behavior preferences
- Keyboard shortcut reference
- Version and build information
- Configuration export/import

</details>

### Global UI Features

- **Collapsible sidebar** with icon-only collapsed mode
- **Command palette** (Ctrl+K) for fuzzy search across all pages and actions
- **Keyboard shortcuts** (Ctrl+N for new, Ctrl+K for palette)
- **Dark/light theme** with CSS custom properties and localStorage persistence
- **Real-time WebSocket** updates for chat, events, and task status
- **Connection status indicator** in sidebar header
- **Version label** auto-updated from API
- **Responsive layout** with sidebar toggle button

---

## Task Management System / AI-Orchestrated Kanban

The task system is not a simple sticky-note board. It is a hybrid human/AI execution engine where tasks can be created manually or auto-generated by the orchestrator, then dispatched to AI agents for autonomous execution with full chain-of-thought traceability.

### Core Task Properties

Every task has a rich data model designed for both human readability and programmatic AI execution:

| Field | Description |
|-------|-------------|
| **Title & Description** | Natural language outline of the task |
| **Status** | `Todo`, `In Progress`, `Blocked`, `Review`, `Done`, `Failed`, `Escalated` |
| **Priority** | Low (0), Medium (1), High (2), Urgent (3) with color-coded icons |
| **Labels** | Multi-label tagging with color-coded badges |
| **Due Date** | Deadline with overdue detection and visual warnings |
| **Assigned Agent** | Human or AI agent (with role-based routing) |
| **Sub-Items** | Checklist entries with completion tracking and progress bar |
| **Dependencies** | `blocked_by` and `blocks` arrays for prerequisite chains |
| **Recurring Pattern** | Cron-like scheduling for repeating tasks |
| **Project ID** | Group tasks by project, each project maps to a chat session |
| **Attachments** | File/image uploads via drag-and-drop (32MB limit per file) |
| **Comments** | Threaded discussion entries with author and timestamp |
| **History** | Full audit trail of every status change and action |
| **Progress** | 0-100% completion percentage with animated progress bar |
| **Auto-Execute** | Checkbox to automatically dispatch task to orchestrator on creation |
| **Task Type** | `atomic`, `compound`, `recurring`, or `meta` |
| **Retry Count** | Current attempt number for failed retries |
| **Escalation Note** | Reason when a task is escalated from AI to human |

### AI Execution Parameters (`AgentConfig`)

This is what separates the system from a standard Kanban board. Each task can carry a full set of agent instructions:

| Parameter | Description |
|-----------|-------------|
| **System Prompt** | Per-task behavioral instructions for the AI (e.g., "Extract key metrics as JSON") |
| **Output Schema** | What "done" looks like (JSON schema, file format, boolean) |
| **Allowed Tools** | Whitelist of tools/APIs the agent can use for this specific task |
| **Max Retries** | How many times to retry on failure before escalating to human |
| **Max Tokens** | Cap compute spend for this task |
| **TTL (Timeout)** | Maximum wall-clock seconds allowed before auto-fail |
| **Autonomy Level** | `draft_only` (human must approve), `supervised` (AI runs, human reviews), `autonomous` (AI executes and closes) |
| **Skills Required** | Capabilities needed for agent routing |
| **Context Refs** | Pointers to external memory (graph node IDs, vector store entries, document paths) |

### Chain-of-Thought Reasoning Scratchpad

Every task has a collapsible reasoning log where the AI records its step-by-step thinking:

- **Step name** with timestamp
- **Tool calls** with monospace formatting
- **Results** for each step (truncated to 200 chars in UI)
- Scrollable, max-height container for long reasoning chains

### Automated Review Pipeline

When a task enters `Review` status, the system automatically runs `RunReview()`:

- Syntax validation on code output
- Error pattern detection
- File-level issue tracking
- Pass/fail result stored on task with detailed issue list
- Visual indicator on kanban cards and detail panel

### Human-in-the-Loop Approval Queue

For destructive or high-risk operations, the approval system provides a safety net:

- Pending approval requests with descriptions
- Approve/Reject actions via API or UI
- Approval badge count in toolbar
- Blocking execution until human decision

### Task API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/tasks` | GET | List all tasks (filterable by project) |
| `/v1/tasks` | POST | Create a new task with full config |
| `/v1/tasks/{id}` | GET | Get task details including reasoning |
| `/v1/tasks/{id}` | PUT | Update task fields |
| `/v1/tasks/{id}` | DELETE | Delete a task |
| `/v1/tasks/{id}/comments` | POST | Add a comment to a task |
| `/v1/tasks/{id}/execute` | POST | Dispatch task to orchestrator for AI execution |
| `/v1/tasks/{id}/attachments` | POST | Upload files to a task (multipart, 32MB) |
| `/v1/approvals` | GET | List pending approval requests |
| `/v1/approvals/{id}` | POST | Approve or reject a request |

### Kanban Board UI

- Drag-and-drop cards between columns
- Priority color-coded borders (green/blue/orange/red)
- Overdue detection with visual warnings
- Sub-item progress bars on cards
- Attachment count badges
- Review pass/fail indicators
- Comment count badges
- Dependency chain indicators
- **Run button** on Todo cards for one-click AI execution
- Project tabs grouped by agency with Auto/Manual toggle

### Task Detail Slide-Out Panel

Click any card to see the full detail panel with:

- Status/priority/agent header
- Description block
- Checklist with completion tracking
- Dependency tree (blocked by / blocks)
- Auto-review results (pass/fail with issue list)
- **Progress bar** (0-100% animated)
- **AI Configuration summary** (autonomy, retries, tokens, timeout, system prompt, tools)
- **Reasoning Scratchpad** (collapsible chain-of-thought log)
- **Run with AI button** (full-width execute for Todo tasks)
- Activity history timeline
- Comments section with inline posting

---

## Knowledge Pipeline

iTaK Agent ships with a multi-database knowledge ingestion system that turns entire repositories, ZIP archives, and structured datasets into searchable, graph-connected knowledge. Every piece of ingested content is stored across **four engines simultaneously** for maximum recall and redundancy.

### Multi-Database Architecture

| Engine | Purpose | What It Stores |
|--------|---------|----------------|
| **Graph** | Relationships and structure | Nodes (Template, Script, Config, Page, etc.) with typed edges (CONTAINS, IMPORTS, REFERENCES) |
| **Vector** | Semantic similarity | Content fingerprints for dedup and similarity search |
| **Table** | Structured metadata | File paths, sizes, extensions, MIME types, timestamps |
| **Full-Text Search** | Keyword matching | Indexed text content for instant grep-like queries |

### Repository Ingestion

Ingest an entire GitHub repository (or GitLab, Bitbucket, Codeberg) with a single API call:

```bash
curl -X POST http://localhost:42100/v1/graph/ingest/repo \
  -H 'Content-Type: application/json' \
  -d '{"url": "expressjs/express"}'
```

The pipeline automatically:
1. Downloads the repository as a ZIP archive
2. Classifies every file by type (Script, Config, Page, Stylesheet, Image, Document, etc.)
3. Creates a Template hub node with CONTAINS edges to every file
4. Detects cross-file references (imports, includes, requires)
5. Indexes text content for full-text search
6. Generates content fingerprints for deduplication

### Knowledge Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/graph/ingest/repo` | POST | Ingest a GitHub/GitLab/Bitbucket repo |
| `/v1/knowledge/search?q=router` | GET | Unified search across all 4 engines |
| `/v1/knowledge/describe/{id}` | GET | Auto-generated documentation for a node |
| `/v1/knowledge/deps/{id}` | GET | Dependency audit for a template |
| `/v1/knowledge/list` | GET | List all ingested knowledge bases |

### Hierarchical Graph Explorer

The Graph Explorer provides a 3-level drill-down view instead of showing hundreds of flat nodes:

**Level 1: Overview** - Template hubs and top-level session nodes

**Level 2: Type Clusters** - Click a template to see files grouped by type (Script x141, Config x7, etc.)

**Level 3: Individual Files** - Click a cluster to browse individual files with metadata panels

Features:
- Breadcrumb navigation (Overview > Template > Cluster)
- Escape key to go back one level
- Glow effects on hub nodes, dashed lines for import edges
- Right-click context menu for edit, link, expand, and delete
- Side panel with full node metadata, properties, and relationships

---

## Memory System & Embedded Database (iTaKDB)

iTaK Agent ships with a **custom embedded graph+vector+SQL database** called **iTaKDB** -- built in pure Go with zero external dependencies. No Docker, no network, no VPS. The entire knowledge graph, vector index, and relational tables live in a single BoltDB file on disk.

### Storage Architecture

```
data/itakdb/
  graph.db      ← BoltDB-backed property graph (nodes, edges, labels)
  hnsw.idx      ← HNSW vector index for semantic similarity search
```

| Engine | Implementation | Purpose |
|--------|---------------|---------|
| **Graph Database** | Property graph on bbolt (~1,180 lines) | Node/edge CRUD, BFS traversal, shortest path, batch ops, export/import, backup |
| **Vector Index** | HNSW with persistence (~700 lines) | K-NN search, 3 distance metrics, metadata filtering, range search, batch ops |
| **SQL Engine** | Recursive-descent parser (~2,040 lines) | Full SQL with GROUP BY, JOINs, transactions, views, OFFSET, NOT ops, UNION |
| **Full-Text Search** | BM25 inverted index (~820 lines) | Keyword, phrase, fuzzy (Levenshtein), highlighting, field-scoped search |

### Node Types (Graph Schema)

The graph stores 12 distinct node labels, each with their own property schema:

<details>
<summary><strong>Core Memory Nodes</strong></summary>

| Label | Key Properties | Created By |
|-------|----------------|------------|
| **Session** | `session_id`, `start_time`, `title`, `type` | Memory Manager on startup |
| **Message** | `role`, `content`, `agent`, `timestamp` | Every chat turn |
| **Action** | `agent`, `tool`, `args`, `result_summary`, `timestamp` | Every tool call |
| **Fact** | `key`, `value`, `category`, `importance` (1-10), `created_at` | `memory_save` tool |
| **Entity** | `name`, `type` (server/person/url/project), `first_seen`, `last_seen` | Auto entity extraction |
| **Conversation** | `id`, `slug`, `title`, `summary`, `timestamp` | Archive system |

</details>

<details>
<summary><strong>Browser & Research Nodes</strong></summary>

| Label | Key Properties | Created By |
|-------|----------------|------------|
| **Page** | `url`, `title`, `last_visited`, `visit_count` | Browser agent page visits |
| **Search** | `query`, `result_count`, `source`, `timestamp` | Search tool executions |
| **BrowserSession** | `browser_session_id`, `headed`, `start_time` | Browser launch events |
| **Research** | `url`, `domain`, `title`, `content`, `findings`, `topic`, `last_visited` | Web research tool |
| **Domain** | `name`, `last_seen` | Auto-grouped from research URLs |

</details>

<details>
<summary><strong>Knowledge & Error Nodes</strong></summary>

| Label | Key Properties | Created By |
|-------|----------------|------------|
| **AgencyKnowledge** | `agency_id`, `project_id`, `title`, `content`, `source`, `type`, `tags` | Knowledge API |
| **Error** | `message`, `error_type`, `source`, `resolved` (bool), `timestamp` | Debug memory on failures |
| **Fix** | `description`, `code`, `agent`, `timestamp` | Auto-linked when errors are resolved |

</details>

### Edge Relationships

All relationships are directed and typed. The graph tracks 10+ relationship types:

| Edge Type | From | To | Meaning |
|-----------|------|-----|---------|
| `PERFORMED` | Session | Action | Session executed this tool call |
| `VISITED` | Session | Page | Session visited this URL |
| `SEARCHED` | Session | Search | Session ran this search query |
| `INCLUDES` | Session | Message | Session contains this message |
| `USED_BROWSER` | Session | BrowserSession | Session launched a browser |
| `RESEARCHED` | Session | Research | Session performed web research |
| `MENTIONED_IN` | Entity | Conversation | Entity appeared in this conversation |
| `FROM_DOMAIN` | Research | Domain | Research page belongs to this domain |
| `RESOLVED_BY` | Error | Fix | Error was fixed by this patch |
| `CONTAINS` | Template | File | Ingested repo contains this file |
| `IMPORTS` | File | File | Code file imports/requires another |

### GraphBackend Interface

The `GraphBackend` interface (`pkg/memory/graph_backend.go`) provides 16 methods that all memory subsystems use:

```go
type GraphBackend interface {
    // Core memory tracking
    SyncFact(f Fact)
    SyncEntity(e Entity)
    LinkEntityToConversation(entityName string, convID int)
    TrackSession(sessionID, title string)
    TrackAction(sessionID, agent, tool, args, result string, embedding []float32)
    TrackPage(sessionID, url, title string, embedding []float32)
    TrackSearch(sessionID, query string, resultCount int, source string, embedding []float32)
    TrackMessage(sessionID, role, content, agent string, embedding []float32)
    TrackBrowserSession(sessionID, browserSessionID string, headed bool)
    
    // Semantic search (HNSW vector)
    SemanticSearch(queryEmbed []float32, limit int) ([]map[string]interface{}, error)
    
    // Debug memory (error/fix tracking)
    StoreError(sessionID, errorMsg, errorType, source string, embedding []float32) uint64
    StoreFix(errorNodeID uint64, fixDescription, fixCode, fixAgent string, embedding []float32)
    SearchErrors(queryEmbed []float32, limit int) ([]map[string]interface{}, error)
    
    // Web research memory
    StoreResearch(sessionID, url, domain, title, content, findings, topic string, embedding []float32) uint64
    SearchResearch(queryEmbed []float32, limit int) ([]map[string]interface{}, error)
    
    EnsureIndexes()
    Close() error
}
```

### Graph REST API

The Graph API (`pkg/api/graph_api.go`) exposes 6 endpoints for exploration, visualization, and editing:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/graph/nodes?label=Action&limit=50` | GET | List nodes by label with pagination |
| `/v1/graph/nodes` | POST | Create a new node with labels and properties |
| `/v1/graph/node/{id}` | GET/PUT/DELETE | Read, update, or delete a single node |
| `/v1/graph/neighbors/{id}?depth=2` | GET | Traverse graph from a node up to 5 levels deep |
| `/v1/graph/stats` | GET | Database statistics (node count, edge count, label distribution) |
| `/v1/graph/edges` | POST | Create a typed edge between two nodes |
| `/v1/graph/search?q=query&limit=10` | GET | Semantic search across all node types |

**Example: Create a custom node**
```bash
curl -X POST http://localhost:42100/v1/graph/nodes \
  -H 'Content-Type: application/json' \
  -d '{"labels": ["Entity"], "props": {"name": "production-server", "type": "server", "ip": "10.0.0.5"}}'
```

**Example: Link two nodes**
```bash
curl -X POST http://localhost:42100/v1/graph/edges \
  -H 'Content-Type: application/json' \
  -d '{"type": "CONNECTS_TO", "source": 42, "target": 87, "props": {"port": "5432"}}'
### SQL Engine (Pure Go)

The SQL engine (`Database/pkg/sql/`) is a custom recursive-descent parser built in Go with zero dependencies. It translates SQL strings into table engine operations, giving agents and API consumers familiar SQL syntax for structured data.

**Supported statements:**

| Statement | Syntax |
|-----------|--------|
| `CREATE TABLE` | `CREATE TABLE tasks (title STRING, status STRING NOT NULL, priority INT)` |
| `INSERT` | `INSERT INTO tasks (title, status) VALUES ('Build API', 'Todo')` |
| `SELECT` | `SELECT title, status FROM tasks WHERE priority > 2 ORDER BY title DESC LIMIT 10` |
| `SELECT DISTINCT` | `SELECT DISTINCT category FROM products` |
| `UPDATE` | `UPDATE tasks SET status = 'Done' WHERE title = 'Build API'` |
| `DELETE` | `DELETE FROM tasks WHERE status = 'Done'` |
| `DROP TABLE` | `DROP TABLE tasks` |
| `ALTER TABLE` | `ALTER TABLE tasks ADD COLUMN deadline TIME, DROP COLUMN old_col` |
| `COUNT` | `SELECT COUNT(*) FROM tasks WHERE status = 'Todo'` |
| `SUM / AVG` | `SELECT SUM(amount) FROM orders WHERE region = 'east'` |
| `MIN / MAX` | `SELECT MAX(score) FROM leaderboard` |
| `GROUP BY` | `SELECT COUNT(*) FROM orders GROUP BY product` |
| `HAVING` | `SELECT COUNT(*) FROM orders GROUP BY product HAVING COUNT(*) > 5` |
| `INNER JOIN` | `SELECT * FROM users JOIN departments ON users.dept_id = departments.dept_id` |
| `LEFT JOIN` | `SELECT * FROM users LEFT JOIN departments ON users.dept_id = departments.dept_id` |
| `UPSERT` | `INSERT OR UPDATE INTO settings (key, val) VALUES ('theme', 'dark') ON CONFLICT key` |
| `CREATE VIEW` | `CREATE VIEW active_tasks AS SELECT * FROM tasks WHERE status = 'active'` |
| `DROP VIEW` | `DROP VIEW active_tasks` |
| `SHOW TABLES` | `SHOW TABLES` |
| `DESCRIBE` | `DESCRIBE tasks` (returns actual stored column schema) |
| `BEGIN` | `BEGIN` (start transaction) |
| `COMMIT` | `COMMIT` (execute buffered statements) |
| `ROLLBACK` | `ROLLBACK` (discard buffered statements) |

**WHERE operators:** `=`, `!=`, `>`, `<`, `>=`, `<=`, `LIKE`, `IN`, `BETWEEN`, `IS NULL`, `IS NOT NULL`, `AND`, `OR`

**Subqueries:** `WHERE col IN (SELECT col FROM other_table WHERE ...)`

**Column types:** `STRING`, `INT`, `FLOAT`, `BOOL`, `TIME`, `JSON`

**Advanced features:**
- **Foreign Keys** - `AddForeignKey()` / `CheckForeignKey()` for referential integrity
- **Export/Import** - `ExportJSON()` / `ImportJSON()` for data portability
- **Prepared Statements** - `ExecutePrepared("SELECT * FROM users WHERE name = ?", "Alice")`
- **Backup/Snapshot** - `engine.Backup(destPath)` for consistent file copy

**REST API:**

```bash
# SELECT with GROUP BY
curl -X POST http://localhost:42100/v1/sql \
  -d '{"query": "SELECT COUNT(*) FROM orders GROUP BY product HAVING COUNT(*) > 5"}'

# LEFT JOIN
curl -X POST http://localhost:42100/v1/sql \
  -d '{"query": "SELECT * FROM users LEFT JOIN roles ON users.role_id = roles.id"}'

# UPSERT
curl -X POST http://localhost:42100/v1/sql \
  -d '{"query": "INSERT OR UPDATE INTO config (key, val) VALUES ('\''theme'\'', '\''dark'\'') ON CONFLICT key"}'

# BETWEEN
curl -X POST http://localhost:42100/v1/sql \
  -d '{"query": "SELECT * FROM events WHERE date BETWEEN '\''2026-01-01'\'' AND '\''2026-06-01'\''"}'

# Transaction
curl -X POST http://localhost:42100/v1/sql -d '{"query": "BEGIN"}'
curl -X POST http://localhost:42100/v1/sql -d '{"query": "INSERT INTO log (msg) VALUES ('\''step1'\'')"}'
curl -X POST http://localhost:42100/v1/sql -d '{"query": "COMMIT"}'
```

**Response format:**
```json
{
  "ok": true,
  "query": "SELECT COUNT(*) FROM orders GROUP BY product",
  "result": {
    "groups": [
      {"key": {"product": "Widget"}, "count": 15, "sum": 150},
      {"key": {"product": "Gizmo"}, "count": 8, "sum": 80}
    ],
    "count": 2,
    "message": "2 group(s)"
  }
}
```

### Graph Engine (Pure Go)

The graph engine (`Database/pkg/graph/store.go`) is a property graph database built on bbolt. ~1,180 lines of pure Go.

**Core CRUD:**

| Method | Description |
|--------|-------------|
| `CreateNode(labels, props)` | Create a node with labels and properties |
| `GetNode(id)` | Retrieve a node by ID |
| `UpdateNode(id, props)` | Merge properties into a node |
| `DeleteNode(id)` | Remove a node and its connected edges |
| `CreateEdge(type, source, target, props)` | Create a directed edge |
| `GetEdge(id)` | Retrieve a single edge |
| `UpdateEdge(id, props)` | Merge properties into an edge |
| `DeleteEdge(id)` | Remove an edge and clean up index |

**Traversal and Pathfinding:**

| Method | Description |
|--------|-------------|
| `Traverse(start, depth, edgeType)` | BFS from a node following outgoing edges |
| `TraverseBidirectional(start, depth, edgeType)` | BFS following both incoming and outgoing edges |
| `ShortestPath(from, to, edgeType)` | BFS pathfinding returning node IDs, edge IDs, and distance |
| `GetEdgesFrom(nodeID)` | All outgoing edges from a node |
| `GetEdgesTo(nodeID)` | All incoming edges to a node |
| `FindEdgesBetween(nodeA, nodeB)` | All edges between two nodes (either direction) |

**Batch Operations:**

| Method | Description |
|--------|-------------|
| `BatchCreateNodes([]NodeInput)` | Bulk insert nodes in a single transaction |
| `BatchCreateEdges([]EdgeInput)` | Bulk insert edges in a single transaction |

**Data Management:**

| Method | Description |
|--------|-------------|
| `ExportJSON(writer)` | Full graph dump (nodes + edges) as JSON |
| `ImportJSON(reader)` | Load graph from JSON |
| `Backup(destPath)` | Consistent bbolt snapshot to file |
| `AllNodes()` | Return every node in the graph |
| `AllEdges()` | Return every edge in the graph |
| `NodeCountByLabel(label)` | Fast count by label without loading nodes |
| `Stats()` | Node count, edge count, label distribution |

### Vector Engine (Pure Go, HNSW)

The vector engine (`Database/pkg/vector/index.go`) implements Hierarchical Navigable Small World (HNSW) for approximate nearest neighbor search. ~700 lines of pure Go with O(log N) search and >95% recall.

**Distance Metrics:**

| Metric | Constructor | Use Case |
|--------|------------|----------|
| Cosine | `NewIndex(dims)` (default) | Text embeddings, normalized vectors |
| Euclidean | `NewIndexWithMetric(dims, Euclidean)` | Spatial data, image features |
| Dot Product | `NewIndexWithMetric(dims, DotProduct)` | Recommendation systems |

**Core Operations:**

| Method | Description |
|--------|-------------|
| `Insert(id, vector)` | Add a vector to the index |
| `InsertWithMeta(id, vector, metadata)` | Add with key-value metadata |
| `Update(id, vector, metadata)` | Replace a vector's data and metadata |
| `Delete(id)` | Remove a vector from the index |
| `Search(query, k)` | Find K nearest neighbors |
| `SearchFiltered(query, k, filterFn)` | K-NN with metadata filter function |
| `RangeSearch(query, threshold)` | All vectors within a similarity threshold |

**Batch and Persistence:**

| Method | Description |
|--------|-------------|
| `BatchInsert(ids, vectors)` | Bulk insert multiple vectors |
| `BatchInsertWithMeta(ids, vectors, metas)` | Bulk insert with metadata |
| `Save(db)` | Persist index to bbolt (survives restart) |
| `Load(db)` | Restore index from bbolt |
| `ExportJSON(writer)` | Export all vectors as JSON |
| `ImportJSON(reader)` | Import vectors from JSON |

**Info:**

| Method | Description |
|--------|-------------|
| `Stats()` | Size, dimensions, max level, metric, memory estimate |
| `GetVector(id)` | Retrieve raw vector data |
| `GetMetadata(id)` | Retrieve metadata for a vector |

### Full-Text Search Engine (Pure Go, BM25)

The FTS engine (`Database/pkg/search/search.go`) uses BM25 ranking with an inverted index on bbolt. ~820 lines of pure Go with phrase matching, fuzzy search, and field-scoped indexing.

**Core Indexing:**

| Method | Description |
|--------|-------------|
| `IndexDocument(docID, text)` | Tokenize and index a document |
| `IndexDocumentWithContent(docID, text)` | Index + store full text (needed for phrase/highlight) |
| `IndexField(docID, field, text)` | Index under a specific field name (e.g., "title", "body") |
| `RemoveDocument(docID)` | Remove a document from the index |

**Search Methods:**

| Method | Description |
|--------|-------------|
| `Search(query, limit)` | BM25-ranked keyword search |
| `PhraseSearch(phrase, limit)` | Exact consecutive phrase matching |
| `FuzzySearch(query, maxDist, limit)` | Typo-tolerant search via Levenshtein distance |
| `SearchField(field, query, limit)` | Search within a specific field |
| `SearchWithHighlight(query, contextWords, limit)` | Returns context snippets around matches |

**Example: Fuzzy search with typo tolerance**
```bash
# Finds "kubernetes" even when searching "kuberntes" (edit distance 1)
curl http://localhost:42100/v1/search -d '{"query": "kuberntes", "fuzzy": true, "max_distance": 2}'
```

**Example: Phrase search**
```bash
# Only matches documents containing the exact phrase "machine learning pipeline"
curl http://localhost:42100/v1/search -d '{"query": "machine learning pipeline", "phrase": true}'
```

### Knowledge API (Agency-Scoped)

The Knowledge API (`pkg/api/knowledge_api.go`) manages business-scoped knowledge entries stored as `AgencyKnowledge` graph nodes:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/knowledge?agency_id=xyz` | GET | List knowledge entries filtered by agency |
| `/v1/knowledge` | POST | Create a knowledge entry (title, content, source, type, tags) |
| `/v1/knowledge/{id}` | DELETE | Remove a knowledge entry |

Knowledge types: `fact`, `document`, `faq`, `note`
Knowledge sources: `url`, `file`, `scrape`, `manual`

### Memory Subsystems

All 9 subsystems are coordinated through the Memory Manager (`pkg/memory/manager.go`):

| Component | File | Purpose |
|-----------|------|---------|
| **Session Memory** | `session.go` | Short-term context window for the current conversation |
| **Persistent Memory** | `persistent.go` | Long-term fact storage that survives restarts (JSON on disk + graph sync) |
| **Entity Tracking** | `entities.go` | Named entity extraction (servers, people, URLs, credentials, projects) |
| **Reflection** | `reflection.go` | Agent self-assessment and learning from past task outcomes |
| **Activity Stream** | `activity.go` | File system activity monitoring and event logging |
| **Activity Bridge** | `activity_bridge.go` | Connects file system events to the memory graph |
| **Archive** | `archive.go` | Conversation archival with summary, tags, agents used, tools used |
| **Workspace** | `workspace.go` | Project-level context isolation |
| **Manager** | `manager.go` | Unified interface that initializes the graph backend and passes it to all subsystems |

### Memory Tools

Agents interact with memory through built-in tools:

| Tool | Description |
|------|-------------|
| `memory_save` | Store a fact with key, value, category, and importance (1-10) |
| `memory_recall` | Query facts by keyword (fuzzy matching) |

All tool calls, messages, page visits, searches, errors, and research are **automatically** tracked in the graph without any explicit tool calls -- the Activity Tracker handles this transparently.



---

## Plugin Architecture

iTaK Agent uses a formal plugin system (`pkg/plugin/`) for I/O channels. Each plugin implements the `Plugin` interface with lifecycle management:

```go
type Plugin interface {
    Name() string
    Start(ctx context.Context) error
    Stop() error
    SendMessage(msg Message) error
    ReceiveMessages() <-chan Message
}
```

### Available Plugins

| Plugin | Directory | Description |
|--------|-----------|-------------|
| **Web API** | `plugin/web/` | REST API + embedded dashboard (primary interface) |
| **Dashboard WebSocket** | `plugin/dashboard/` | Real-time WebSocket feed for the UI |
| **CLI REPL** | `plugin/cli/` | Interactive terminal interface |
| **Discord Bot** | `plugin/discord/` | Discord channel integration |
| **VisionClaw** | `plugin/visionclaw/` | Smart glasses integration (OpenClaw gateway) |
| **Voice** | `plugin/voice/` | Voice input/output channel |

### Plugin Manager

The `PluginManager` (`plugin/manager.go`) handles:
- Plugin registration and discovery
- Lifecycle management (start/stop ordering)
- Message routing between plugins and the orchestrator
- Health monitoring

---

## Model Context Protocol (MCP)

Full MCP client and server implementation in `pkg/mcp/`:

| Component | File | Purpose |
|-----------|------|---------|
| **Client** | `client.go` | Connect to external MCP servers and consume their tools |
| **Server** | `server.go` | Expose iTaK tools as an MCP server for other agents |
| **Discovery** | `discovery.go` | Auto-discover MCP servers on the network |
| **Tool Adapter** | `tool_adapter.go` | Bridge between MCP tool format and iTaK's native Tool interface |

This allows iTaK to both consume tools from external MCP servers and expose its own tools to other MCP-compatible agents.

---

## What Is iTaK Agent?

iTaK Agent is a dual-purpose AI framework built in **Go**.

Its primary form is a **fully bundled, batteries-included agent** that features an embedded Core orchestrator, integrated Memory, native Shield guardrails, and a complete task management system. All of these are optimized to work seamlessly with **smaller, efficient models** you can run locally.

However, its secret weapon is its **modular architecture**. Every major piece of the iTaK Agent is built as an individual component first. This allows the open-source community to use iTaK components (like the security guardrails or the memory system) independently in other projects, while the main iTaK Agent bundles them all together for the ultimate out-of-the-box experience.

The core idea: **keep the boss simple and the agents focused.**

- The **Boss (Orchestrator)** never touches tools. It just figures out what needs to happen and hands off work.
- **Focused Agents** (like researcher, coder, browser) each have a small set of tools, clear goals, and a specific job.
- Each agent only sees what it needs. This keeps things fast and cheap.
- The **Dashboard Task Manager** bridges orchestrator decisions to a visual Kanban board with full AI execution.

### Why Go?

| Feature | Why It Matters |
|---------|----------------|
| **Single binary** | No virtual environments, no `pip install`, no dependency problems |
| **Fast startup** | Agents start in milliseconds, not seconds |
| **Cross-platform** | Same binary runs on Windows, Linux, macOS, Docker |
| **Low memory** | Uses way less memory than Python frameworks |
| **Easy deployment** | Copy one file to your server and run it |

---

## How It Works

<div align="center">
  <img src="assets/screenshots/itak_architecture_diagram.png" alt="iTaK Agent Architecture" width="800" />
</div>

### Step by Step

1. **You type a message** - `"Fetch google.com and save the HTML to output.html"`
2. **Boss thinks** - "This needs HTTP fetching AND file writing. I'll send it to `researcher`."
3. **Boss sends a task** - `{ agent: "researcher", task: "Fetch google.com and save to output.html" }`
4. **Researcher does the work** - calls `http_fetch` to get the HTML, then calls `file_write` to save it
5. **Dashboard task is created** - The task appears on the Kanban board with status "In Progress"
6. **Result comes back** - Boss gives you a clean answer: *"Done! Saved Google's HTML to output.html (52KB)"*
7. **Task moves to Review** - Automated review pipeline checks the output quality
8. **Task completes** - Status moves to Done with full reasoning trace

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
git clone https://github.com/David2024patton/iTaKAgent.git
cd iTaKAgent
go build -o itakagent ./cmd/itakagent/
```

This creates a single `itakagent` binary (or `itakagent.exe` on Windows).

### Docker

```bash
docker compose up -d --build
```

The container exposes the dashboard on the configured port (default `43211`).

### 2. Set Up Your Config

Copy the example config and add your API key:

```bash
cp configs/example.yaml itakagent.yaml
```

Open `itakagent.yaml` and change the API key:

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
./itakagent run
```

You'll see:
```
iTaK Agent v0.3.0 - 6 agents ready. Type a message (Ctrl+C to quit).
  REST API: http://localhost:42100
  WebSocket: ws://localhost:48900/ws

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

The config file (`itakagent.yaml`) controls everything. Here's a full example:

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
|----------|------------|
| Windows | `cmd /C` |
| macOS | `zsh -c` |
| Linux / Docker | `sh -c` |
| Android (Termux) | `sh -c` |

### `file_read` - Read Files

Reads the full text of any file the agent has access to.

### `file_write` - Write Files

Writes content to a file. Creates folders automatically if they don't exist.

### `http_fetch` - HTTP Requests

Makes HTTP GET or POST requests and returns the response. 30-second timeout, response cut at 50KB.

### `dir_list` - List Directory Contents

Lists files and directories with sizes and modification times.

### `grep_search` - Search Files

Searches for text patterns in files recursively. Supports regex, extension filtering, and case-insensitive search.

### `memory_save` / `memory_recall` - Persistent Memory

Saves and recalls facts across sessions. Facts persist as JSON on disk.

### `skill_list` / `skill_load` - Skill System

Discovers and loads skill definitions from the skills directory. Skills are YAML frontmatter + markdown files.

### Browser Tools (23 tools)

Full browser automation suite:

| Tool | Purpose |
|------|---------|
| `web_navigate` | Open a URL |
| `web_click` | Click an element by selector |
| `web_type` | Type text into an input |
| `web_scroll` | Scroll the page |
| `web_back` | Go back in history |
| `web_eval` | Execute JavaScript |
| `web_wait` | Wait for an element |
| `web_screenshot` | Capture a screenshot |
| `web_extract` | Extract text/data from page |
| `web_pdf` | Save page as PDF |
| `web_search` | Search the web |
| `web_close` | Close the browser |
| `web_snapshot` | DOM snapshot |
| `web_cookies` | Manage cookies |
| `web_headed` | Toggle headed/headless |
| `web_hover` | Hover over element |
| `web_double_click` | Double-click element |
| `web_focus` | Focus an element |
| `web_keys` | Send keyboard shortcuts |
| `web_tab_new` | Open new tab |
| `web_tab_switch` | Switch between tabs |
| `web_tab_close` | Close a tab |
| `web_tab_list` | List all tabs |

---

## iTaK Shield (Security Guardrails)

All tool calls pass through a zero-latency, fully embedded guardrail chain implemented in `pkg/tool/guardrails.go` and `pkg/guard/guard.go`:

| Guardrail | What It Does |
|-----------|-------------|
| **Rate Limit** | Prevents runaway agents (30 calls/min per tool) |
| **Content Filter** | Blocks dangerous patterns (`curl \| bash`, `rm -rf /`, `format C:`, etc.) |
| **SSRF Protection** | Blocks requests to private IPs and internal networks (`pkg/tool/ssrf.go`) |
| **Script Snapshot** | Captures script content before execution for audit trail |
| **Path Traversal** | Prevents agents from accessing files outside allowed directories |

The guard package (`pkg/guard/`) provides additional security:
- Pattern-based threat detection
- Configurable block/allow lists
- Full test coverage (`guard_test.go`)

---

## GPU Compute Engine

iTaK Agent includes a Vulkan-based GPU compute engine (`pkg/gpu/`) for accelerated tensor operations:

### Hardware-Accelerated Operations

| Operation | Description |
|-----------|-------------|
| **MatMul** | Matrix multiplication |
| **Softmax** | Softmax normalization |
| **RoPE** | Rotary Position Embeddings |
| **SiLU** | SiLU activation function |
| **GELU** | GELU activation function |
| **RMSNorm** | Root Mean Square Layer Normalization |

### Two Backends

| Backend | Location | Purpose |
|---------|----------|---------|
| **Compute (Vulkan)** | `pkg/gpu/compute/` | Hardware GPU acceleration via Vulkan SPIR-V shaders |
| **SoftGPU** | `pkg/gpu/softgpu/` | CPU fallback when no GPU is available |

---

## Model File Parsers

Native Go parsers for loading model weights without Python dependencies (`pkg/model/`):

| Format | File | Capabilities |
|--------|------|-------------|
| **GGUF** | `gguf.go` | Parse llama.cpp quantized model files (metadata, tensor info, quantization types) |
| **SafeTensors** | `safetensors.go` | Parse HuggingFace SafeTensors format (header parsing, tensor metadata) |
| **Model Manager** | `model.go` | Unified interface for model discovery and loading |

Both parsers have comprehensive test suites (`gguf_test.go`, `safetensors_test.go`).

---

## Cron Scheduler

Built-in cron scheduler (`pkg/cron/scheduler.go`) for recurring task automation:

- Standard cron expression support
- Task auto-creation on schedule
- Integration with the orchestrator for automated execution
- Persistent schedule storage

---

## Code Indexer

The code indexer (`pkg/codex/code_indexer.go`) provides:

- Automatic source code analysis
- Language detection and classification
- Function/class/method extraction
- Cross-file reference detection
- Integration with the knowledge graph for code-aware queries

---

## Agency Catalog

Pre-built agent configurations (`pkg/seed/`) for common use cases:

| File | Purpose |
|------|---------|
| `agency_catalog.json` | 30KB catalog of pre-configured agent teams (agencies) |
| `focus_agents.go` | Go code for bootstrapping focused agents from the catalog |
| `knowledge.go` | Knowledge seeding utilities for new installations |

The catalog includes ready-to-use agent configurations for research, development, content creation, data analysis, and more.

---

## Debug Mode

iTaK Agent has three logging levels:

### Quiet Mode (default)

```bash
./itakagent run
```

### Verbose Mode

```bash
./itakagent run --verbose
```

Shows key decisions: which agent was picked, what tools were called, success or failure.

### Debug Mode

```bash
./itakagent run --debug
```

Shows everything: JSON payloads, token counts, HTTP timing, tool arguments, full results.

### Debug HTTP Endpoints

When `ITAK_DEBUG=1` is set, Agent exposes HTTP debug endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/debug/snapshot` | GET | Full state dump: health + recent logs + events + config |
| `/debug/logs?level=debug&last=100` | GET | Recent log ring buffer, filterable by level |
| `/debug/events` | GET | Recent event bus history (task.created, tool.called, etc.) |
| `/debug/health` | GET | Full health report (agent count, active tasks, uptime) |
| `/debug/config` | GET | Runtime config (API keys redacted) |
| `/debug/level` | GET/POST | View or change log level at runtime |

**Environment Variables:**

| Variable | Values | Effect |
|----------|--------|--------|
| `ITAK_DEBUG` | `1`, `true` | Enables debug HTTP endpoints and DEBUG-level logging |
| `ITAK_DEBUG_COLOR` | `false`, `0` | Disables ANSI colors (for log files) |

---

## Project Structure

```
iTaK Agent/
├── cmd/
│   └── itakagent/
│       └── main.go                  # CLI entrypoint (run, serve, version)
│
├── pkg/
│   ├── agent/
│   │   ├── types.go                 # Core types: Orchestrator, AgentConfig, TaskPayload
│   │   ├── orchestrator.go          # Boss: reasons, delegates, bridges to dashboard tasks
│   │   └── focused.go              # Focused Agent: task loop with tool calling
│   │
│   ├── api/
│   │   ├── server.go               # REST API server (chat, tasks, projects, knowledge)
│   │   └── project_api.go          # Project CRUD with session binding
│   │
│   ├── codex/
│   │   └── code_indexer.go          # Source code analysis and indexing
│   │
│   ├── config/
│   │   └── config.go               # YAML config loader with env var support
│   │
│   ├── cron/
│   │   └── scheduler.go            # Cron-based task scheduling
│   │
│   ├── debug/
│   │   └── logger.go               # Structured logger (ERROR/WARN/INFO/DEBUG)
│   │
│   ├── embed/                      # Embedded filesystem for web assets
│   │
│   ├── eventbus/                   # Pub/sub event bus for inter-component events
│   │
│   ├── gpu/
│   │   ├── compute/                # Vulkan GPU compute shaders (SPIR-V)
│   │   └── softgpu/                # CPU fallback compute engine
│   │
│   ├── guard/
│   │   ├── guard.go                # Security guardrail engine
│   │   └── guard_test.go           # Guard test suite
│   │
│   ├── llm/
│   │   ├── client.go               # OpenAI-compatible HTTP client
│   │   └── message.go              # Message, ToolCall, Response types
│   │
│   ├── mcp/
│   │   ├── client.go               # MCP client (consume external tools)
│   │   ├── server.go               # MCP server (expose iTaK tools)
│   │   ├── discovery.go            # Auto-discover MCP servers
│   │   └── tool_adapter.go         # Bridge MCP <-> iTaK tool format
│   │
│   ├── memory/
│   │   ├── manager.go              # Unified memory interface
│   │   ├── session.go              # Short-term conversation memory
│   │   ├── persistent.go           # Long-term fact storage
│   │   ├── entities.go             # Named entity tracking
│   │   ├── reflection.go           # Agent self-assessment
│   │   ├── activity.go             # File system activity monitoring
│   │   ├── activity_bridge.go      # Events -> memory graph bridge
│   │   ├── archive.go              # Session archival
│   │   ├── workspace.go            # Project-level context isolation
│   │   ├── graph_backend.go        # Neo4j storage backend
│   │   ├── itakdb_backend.go       # Embedded BoltDB backend
│   │   └── interface.go            # Memory interface definitions
│   │
│   ├── model/
│   │   ├── gguf.go                 # GGUF model file parser
│   │   ├── safetensors.go          # SafeTensors model file parser
│   │   └── model.go                # Model discovery and management
│   │
│   ├── plugin/
│   │   ├── plugin.go               # Plugin interface definition
│   │   ├── manager.go              # Plugin lifecycle manager
│   │   ├── web/                    # Web API plugin
│   │   ├── dashboard/              # WebSocket dashboard plugin
│   │   ├── cli/                    # CLI REPL plugin
│   │   ├── discord/                # Discord bot plugin
│   │   ├── visionclaw/             # Smart glasses integration
│   │   └── voice/                  # Voice I/O plugin
│   │
│   ├── seed/
│   │   ├── agency_catalog.json     # Pre-built agent team configurations
│   │   ├── focus_agents.go         # Agent bootstrapping from catalog
│   │   └── knowledge.go            # Knowledge seeding utilities
│   │
│   ├── skill/                      # Skill system (YAML frontmatter parsing)
│   │
│   ├── task/                       # Legacy task system
│   │
│   ├── tasks/
│   │   ├── models.go               # Task, AgentConfig, ReasoningEntry, Attachment structs
│   │   ├── manager.go              # Task CRUD, AddReasoning, UpdateProgress, AddAttachment
│   │   ├── reviewer.go             # Automated code review pipeline
│   │   └── approval.go             # Human-in-the-loop approval queue
│   │
│   ├── tool/
│   │   ├── interface.go            # Tool interface (Name, Description, Schema, Execute)
│   │   ├── registry.go             # Tool registry + LLM schema generation
│   │   ├── guardrails.go           # Guardrail chain (rate limit, content filter)
│   │   ├── ssrf.go                 # SSRF protection (blocks private IPs)
│   │   └── builtins/               # Built-in tool implementations
│   │
│   ├── ui/                         # UI utilities
│   │
│   └── ws/                         # WebSocket server
│
├── web/
│   ├── index.html                  # Dashboard HTML (task modal, kanban board)
│   ├── app.js                      # Dashboard JavaScript (7400+ lines)
│   ├── styles.css                  # Full CSS design system (50KB)
│   ├── graph.html                  # Graph Explorer (standalone, 46KB)
│   └── embed.go                    # Go embed directives
│
├── configs/
│   └── example.yaml                # Example configuration
│
├── data/
│   ├── skills/                     # Built-in skill definitions
│   ├── medgeclaw/                  # Medical agent skills
│   ├── netclaw/                    # Network agent skills
│   └── network/                    # Network lab configs
│
├── scripts/
│   └── benchmark/                  # Performance benchmarking tools
│
├── deploy/                         # Deployment scripts
├── benchmarks/                     # Benchmark results
├── Dockerfile                      # Multi-stage Docker build
├── docker-compose.yml              # Docker Compose orchestration
├── itakagent.yaml                  # Active configuration
├── setup.bat                       # Windows setup script
├── setup.sh                        # Linux/macOS setup script
├── BENCHMARKS.md                   # Performance benchmark results
├── Feature_List.md                 # Detailed feature documentation
└── README.md                       # This file
```

---

## Extending the Agent

### Adding a New Agent

Just add it to your `itakagent.yaml`:

```yaml
agents:
  - name: writer
    role: Content Writer
    personality: "Creative writer who produces clear, engaging content"
    goals: [engagement, clarity, originality]
    tools: [file_read, file_write]
    max_loops: 6
```

Restart iTaK Agent and the boss will automatically know about the writer agent.

### Adding a New Tool

Create a new file in `pkg/tool/builtins/` that follows the `Tool` interface:

```go
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
    return "result: " + input, nil
}
```

Register it in `cmd/itakagent/main.go`, rebuild, and any agent with `my_tool` in its tools list can use it.

### Adding a New Plugin

Implement the `Plugin` interface in a new directory under `pkg/plugin/`:

```go
type Plugin interface {
    Name() string
    Start(ctx context.Context) error
    Stop() error
    SendMessage(msg Message) error
    ReceiveMessages() <-chan Message
}
```

Register it with the `PluginManager` and it will be managed alongside existing plugins.

---

## Supported LLM Providers

iTaK Agent works with any API that uses the OpenAI `/v1/chat/completions` format:

| Provider | `api_base` | Notes |
|----------|------------|-------|
| **NVIDIA NIM** | `https://integrate.api.nvidia.com/v1` | Free tier available |
| **Ollama** (local) | `http://localhost:11434/v1` | Free, runs on your machine |
| **OpenRouter** | `https://openrouter.ai/api/v1` | Access to 100+ models |
| **OpenAI** | `https://api.openai.com/v1` | GPT-4o, etc. |
| **Together AI** | `https://api.together.xyz/v1` | Open-source models |
| **Groq** | `https://api.groq.com/openai/v1` | Ultra-fast inference |
| **Any compatible** | Your URL | Anything with `/chat/completions` |

---

## The iTaK Agent Ecosystem

| Project | What It Does |
|---------|-------------|
| **iTaK Agent** | Core agent framework. Boss + focused agents + tools + task management. |
| **iTaK Gateway** | Standalone LLM gateway. Routes requests across 42+ providers with failover, rate limiting, and cost tracking. |
| **iTaK Forge** | Live preview server + container runtime. Builds and hosts projects as agents write code. |
| **iTaK Dashboard** | Web-based dashboard. Chat, agent monitoring, task board, cost tracking. |
| **iTaK Beat** | Self-healing monitor. Auto-detects errors, runs diagnostics, fixes problems. |
| **iTaK Hub** | Extension marketplace. Browse, install, and share agent skills and tools. |
| **iTaK Browser** | Custom browser engine for agents. DOM extraction and web automation. |
| **iTaK Vision** | Screen automation. Takes screenshots, understands UI, clicks/types on your behalf. |
| **iTaK Shield** | Security guardrail engine. Rate limiting, content filtering, SSRF protection, PII redaction. |

---

## Roadmap

- [x] Core orchestrator + focused agent pipeline
- [x] Built-in tools (shell, file, HTTP, dir_list, grep_search)
- [x] Structured debug logging with JSONL trace files
- [x] Cross-platform shell support
- [x] Per-agent LLM configuration
- [x] Mandatory task system (every request gets a checklist)
- [x] Single-call routing (one LLM call to pick the right agent)
- [x] iTaK Gateway with 42-provider catalog, failover, cost tracking
- [x] Memory system (session + persistent facts + entity tracking + reflections)
- [x] Browser automation (23 web tools)
- [x] WebSocket server for dashboard communication
- [x] Event bus (pub/sub for inter-component events)
- [x] Guardrail chain (rate limit, content filter, SSRF, script snapshot)
- [x] REST API server (/health, /v1/chat, /v1/agents, /v1/status, /debug/snapshot)
- [x] ITAK_DEBUG=1 observability standard
- [x] Skill system with YAML frontmatter parsing
- [x] Vulkan GPU compute (MatMul, Softmax, RoPE, SiLU, GELU, RMSNorm)
- [x] GGUF and SafeTensors model file parsers
- [x] `serve` subcommand for API-only mode
- [x] Multi-DB knowledge pipeline (Graph + Vector + Table + FTS)
- [x] GitHub/GitLab/Bitbucket/Codeberg repo ingestion
- [x] Unified search across all 4 database engines
- [x] Auto-documentation generator for ingested repos
- [x] Dependency auditing tool
- [x] Hierarchical Graph Explorer with 3-level drill-down
- [x] Visual Workflow Builder (n8n-style node editor, 18 node types)
- [x] Workflow templates, import/export JSON, variables panel
- [x] Multi-select, copy/paste, auto-layout, snap-to-grid, minimap
- [x] Plugin architecture (Web, Dashboard, CLI, Discord, VisionClaw, Voice)
- [x] MCP client and server (consume and expose tools)
- [x] AI-orchestrated task management (Kanban + execute + review)
- [x] AgentConfig per task (system prompt, output schema, autonomy level)
- [x] Chain-of-thought reasoning scratchpad
- [x] File attachments on tasks (drag-and-drop, 32MB)
- [x] Automated code review pipeline
- [x] Human-in-the-loop approval queue
- [x] Project-to-session binding with auto-create
- [x] Cron scheduler for recurring tasks
- [x] Code indexer for source analysis
- [x] Agency catalog with pre-built agent teams
- [x] Docker deployment with compose
- [ ] Real embedding model integration (semantic search)
- [ ] iTaK Forge live preview server
- [ ] Local model marketplace with hardware auto-detect
- [ ] Manager-worker hierarchy (agents delegate to sub-agents)
- [ ] iTaK Hub extension marketplace
- [ ] Communication plugins (Telegram, WhatsApp, Slack)
- [ ] iTaK Vision screen automation

---

## API Reference

### REST Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check with agent count and uptime |
| `/v1/chat` | POST | Send a message to the orchestrator |
| `/v1/agents` | GET | List all registered agents |
| `/v1/status` | GET | System status and metrics |
| `/v1/tasks` | GET/POST | List or create tasks |
| `/v1/tasks/{id}` | GET/PUT/DELETE | Read, update, or delete a task |
| `/v1/tasks/{id}/execute` | POST | Dispatch task to orchestrator |
| `/v1/tasks/{id}/comments` | POST | Add a comment |
| `/v1/tasks/{id}/attachments` | POST | Upload files |
| `/v1/approvals` | GET | List pending approvals |
| `/v1/approvals/{id}` | POST | Approve or reject |
| `/v1/projects` | GET/POST | List or create projects |
| `/v1/projects/{id}` | GET/PUT/DELETE | Project CRUD |
| `/v1/graph/ingest/repo` | POST | Ingest a repository |
| `/v1/knowledge/search` | GET | Unified knowledge search |
| `/v1/knowledge/describe/{id}` | GET | Auto-documentation |
| `/v1/knowledge/deps/{id}` | GET | Dependency audit |
| `/v1/knowledge/list` | GET | List knowledge bases |
| `/debug/snapshot` | GET | Full state dump |
| `/debug/logs` | GET | Log ring buffer |
| `/debug/events` | GET | Event history |
| `/debug/health` | GET | Health report |
| `/debug/config` | GET | Runtime config |
| `/debug/level` | GET/POST | Log level control |

---

## Core Integration

> **Imports:** [`iTaK Core`](../Core/) | **Implements:** none (consumer only) | **Requires at runtime:** InferenceEngine, PrivacyProxy (via SafeClient)

| Core Package | How Agent Uses It |
|---|---|
| `pkg/types` | ChatMessage, InferenceRequest/Response, TokenUsage for all LLM calls |
| `pkg/contract` | Calls `InferenceEngine` (satisfied by Torch or Gateway). Uses **`SafeClient`** to force Shield on cloud calls |
| `pkg/contract` | Calls `BrowserEngine` (satisfied by Browser) for web automation tasks |
| `pkg/registry` | Registers itself at startup. Discovers Torch, Gateway, Browser, Shield endpoints |
| `pkg/auth` | Validates incoming API keys. Signs outbound requests with HMAC-SHA256 |
| `pkg/health` | Exposes `GET /health` with agent count, active tasks, uptime |
| `pkg/event` | Emits `task.created`, `task.completed`, `task.failed`, `tool.called`, `tool.result` |

---

## License

MIT. Use it however you want.

## Contributing

PRs welcome! Check the [project structure](#-project-structure) section to see where things live.

---

<div align="center">
  <sub>Built with Go</sub>
</div>
