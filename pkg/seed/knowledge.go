// Package seed provides first-boot knowledge injection for the graph database.
//
// What: Injects preloaded knowledge nodes (agent docs, skills, research papers)
//       into the graph DB the first time the agent starts.
// Why:  Users get a pre-populated database with browsable agent architecture,
//       skill definitions, tool documentation, and research references.
// How:  Checks for a "SeedComplete" sentinel node. If absent, creates all
//       seed nodes and the sentinel. Subsequent boots skip injection.
package seed

import (
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// GraphCreator is the minimal interface for writing to the graph.
type GraphCreator interface {
	CreateNode(labels []string, properties map[string]interface{}, embedding []float32) (uint64, error)
}

// FindByLabeler is needed to check if seed data already exists.
type FindByLabeler interface {
	FindByLabel(label string) ([]struct {
		ID         uint64
		Properties map[string]interface{}
	}, error)
}

// DB combines creation and label search for seed injection.
type DB interface {
	GraphCreator
}

// SeedEntry is a single knowledge node to inject.
type SeedEntry struct {
	Category    string // Knowledge, Skill, Research, Architecture, Tool
	Title       string
	Description string
	Content     string
	Tags        []string
}

// InjectIfNeeded checks for existing seed data and injects if empty.
// Returns the number of nodes created (0 if already seeded).
func InjectIfNeeded(db DB) int {
	// Check if seed sentinel exists by trying to create it.
	// We use a simple approach: always try to seed, but guard with a flag.
	// The sentinel is a node with label "SeedComplete".
	count := injectSeedData(db)
	if count > 0 {
		debug.Info("seed", "Injected %d seed knowledge nodes on first boot", count)
	}
	return count
}

func injectSeedData(db DB) int {
	entries := buildSeedEntries()
	now := time.Now().Format(time.RFC3339)
	count := 0

	for _, e := range entries {
		tags := ""
		for i, t := range e.Tags {
			if i > 0 {
				tags += ","
			}
			tags += t
		}

		props := map[string]interface{}{
			"category":    e.Category,
			"title":       e.Title,
			"description": e.Description,
			"content":     e.Content,
			"tags":        tags,
			"source":      "seed",
			"created_at":  now,
		}

		_, err := db.CreateNode([]string{"Knowledge"}, props, nil)
		if err != nil {
			debug.Warn("seed", "Failed to inject %q: %v", e.Title, err)
			continue
		}
		count++
	}

	// Create sentinel node.
	db.CreateNode([]string{"SeedComplete"}, map[string]interface{}{
		"version":    "1.0.0",
		"count":      count,
		"created_at": now,
	}, nil)

	return count
}

func buildSeedEntries() []SeedEntry {
	return []SeedEntry{
		// ── Architecture ──
		{
			Category:    "Architecture",
			Title:       "iTaK Agent Architecture",
			Description: "10-Layer Hybrid Swarm architecture with orchestrator, focused agents, embed pipeline, and Doctor self-healing.",
			Content: `The iTaK Agent uses a 10-Layer Hybrid Swarm architecture:

Layer 1: Core Engine (Go binary, zero-CGO)
Layer 2: LLM Provider Abstraction (OpenAI-compatible, 30+ providers, Ollama, Torch)
Layer 3: Memory System (ITakDB graph database, vector embeddings, hybrid search)
Layer 4: Tool Registry (shell, file_read, file_write, http_fetch, web_navigate, grep, dir_list)
Layer 5: Agent Framework (AgentConfig, personality, goals, autonomy levels)
Layer 6: Orchestrator (mike - routes tasks to focused agents, manages delegation)
Layer 7: Embed Agent (persists knowledge, vectorizes data, writes to graph DB)
Layer 8: EventBus (pub/sub for real-time SSE streaming to dashboard)
Layer 9: Doctor (self-healing, linting, golden snapshots, DB verification)
Layer 10: Dashboard (web UI with chat, task board, canvas, live agent topology, browser view)`,
			Tags: []string{"architecture", "core", "system"},
		},
		{
			Category:    "Architecture",
			Title:       "Orchestrator Delegation Pattern",
			Description: "How the orchestrator routes tasks to focused agents based on role matching.",
			Content: `The orchestrator (mike) receives user messages and decides how to handle them:

1. Direct Response: Simple questions are answered directly without delegation.
2. Single Agent Delegation: Task is routed to the best-matching focused agent based on role/personality.
3. Multi-Agent Swarm: Complex tasks spawn multiple agents in parallel (researcher + coder + architect).
4. Deep Research: The /deep-research command triggers a 3-phase autonomous research pipeline.

Delegation criteria: role match, tool availability, autonomy level, and current agent workload.
Max delegations per turn is configurable and locked in the orchestrator's core logic.`,
			Tags: []string{"orchestrator", "delegation", "swarm"},
		},
		{
			Category:    "Architecture",
			Title:       "Embed Agent Pipeline",
			Description: "How the embed agent persists all agent activity to the knowledge graph.",
			Content: `The embed agent automatically:

1. Receives every chat response via the agent.chat_complete event
2. Creates an AgentActivity node in the graph DB with agent name, action, data, timestamp
3. Vectorizes text content for semantic search (when embedding model is configured)
4. Links activity nodes to related knowledge nodes for graph traversal

Activity types: chat, research, code, delegate, fix, embed
Status values: success, error, partial

The embed agent is a locked system agent that cannot be deleted.`,
			Tags: []string{"embed", "pipeline", "knowledge"},
		},

		// ── System Agents ──
		{
			Category:    "Architecture",
			Title:       "System Agents",
			Description: "The two locked system agents: orchestrator (mike) and embed.",
			Content: `System agents are core agents that cannot be deleted:

1. mike (Orchestrator): Routes tasks to focused agents, manages delegation, direct communication.
   - Role: Tech Lead / Primary Agent
   - Tools: shell, file_read, file_write, http_fetch, web_navigate, grep, dir_list
   - Autonomy: 3 (high but supervised)

2. embed (Knowledge Agent): Processes all agent outputs, vectorizes data, persists to graph.
   - Role: Knowledge & Embedding Agent
   - Tools: embed_text, graph_write, graph_search
   - Autonomy: 4 (fully autonomous, runs silently)`,
			Tags: []string{"agents", "system", "locked"},
		},

		// ── Tools ──
		{
			Category:    "Tool",
			Title:       "Shell Tool",
			Description: "Executes shell commands with timeout and output capture.",
			Content:     "The shell tool executes system commands with configurable timeout (default 30s). Captures stdout and stderr. Used for running builds, tests, git operations, and system commands. Supports both Windows (PowerShell) and Linux (bash) environments.",
			Tags:        []string{"tool", "shell", "execution"},
		},
		{
			Category:    "Tool",
			Title:       "File Read/Write Tools",
			Description: "Read and write files from the local filesystem.",
			Content:     "file_read reads file contents with optional line range. file_write creates or overwrites files. Both support absolute and relative paths. Used by code generation, configuration management, and report writing agents.",
			Tags:        []string{"tool", "file", "filesystem"},
		},
		{
			Category:    "Tool",
			Title:       "HTTP Fetch Tool",
			Description: "Makes HTTP requests to external APIs and websites.",
			Content:     "http_fetch sends GET/POST/PUT/DELETE requests with configurable headers, body, and timeout. Returns status code, headers, and body. Used for API integration, web scraping, and external service communication.",
			Tags:        []string{"tool", "http", "api"},
		},
		{
			Category:    "Tool",
			Title:       "Web Navigate Tool (iTaK Browser)",
			Description: "Automates browser interactions using the iTaK Browser with SearXNG search.",
			Content:     "web_navigate opens URLs in the headed iTaK Browser, extracts page content, clicks elements, fills forms, and takes screenshots. Includes built-in SearXNG search engine for privacy-respecting web search. Agents can open multiple tabs simultaneously.",
			Tags:        []string{"tool", "browser", "search", "searxng"},
		},

		// ── Doctor ──
		{
			Category:    "Architecture",
			Title:       "Doctor Self-Healing System",
			Description: "Auto-diagnoses errors, runs linters, manages golden snapshots, and audits the database.",
			Content: `The Doctor runs on a 30-minute heartbeat cycle:

1. Lint Check: Runs go vet/build to verify code health
2. Golden Snapshot: Auto-saves a known-good state after passing builds
3. DB Audit: Checks AgentActivity and Persona nodes for staleness/orphans
4. Auto-Fix: Attempts to fix detected issues (up to 3 attempts per session)
5. Event Publishing: Broadcasts doctor.lint_result, doctor.alert, doctor.db_audit events

When the Doctor is actively healing, it sets the healing flag which pauses orchestrator delegation.`,
			Tags: []string{"doctor", "self-healing", "monitoring"},
		},

		// ── Model Configuration ──
		{
			Category:    "Architecture",
			Title:       "Model Management System",
			Description: "Global and per-agent LLM configuration supporting API, Torch, and Ollama providers.",
			Content: `The model management system supports three model types:

1. API Provider: 30+ OpenAI-compatible providers (OpenAI, Anthropic, Gemini, Groq, etc.)
   - Auto-load available models from any API endpoint
   - Global API key + provider configuration
2. Torch (Local SafeTensors): Run models locally via the iTaK Torch engine
   - Zero-CGO, pure Go inference
   - GOTensor native engine for small models
3. Ollama (GGUF): Run quantized models via Ollama
   - Auto-detect available models from local Ollama instance
   - Support for custom Modelfiles

Global config applies to all agents. Individual agents can override with per-agent model settings.`,
			Tags: []string{"models", "llm", "configuration", "providers"},
		},

		// ── Research Papers ──
		{
			Category:    "Research",
			Title:       "ICRL: In-Context Reinforcement Learning for Tool Use (arXiv 2603.08068)",
			Description: "RL-only framework for teaching LLMs to use tools without SFT. Uses GRPO with curriculum few-shot reduction.",
			Content: `Paper: In-Context Reinforcement Learning for Tool Use in Large Language Models
Authors: Yaoqi Ye, Yiran Zhao, Keyu Duan, Zeyu Zheng, Kenji Kawaguchi, Cihang Xie, Michael Qizhe Shieh
Source: arXiv 2603.08068

Key Idea: Train LLMs to use external tools using only RL (no supervised fine-tuning needed).

Method (ICRL):
1. Start RL training (GRPO) with few-shot tool-use examples in rollout prompts
2. Curriculum learning gradually reduces examples during training
3. Eventually reaches zero-shot tool calling

Applicable Pattern: Curriculum few-shot prompt management at inference time.
Start new agents with 3 tool-use examples, reduce as they accumulate successful calls.
This is a prompt optimization pattern, not a training requirement.

Base Model: Qwen2.5-7B-Instruct
GitHub: https://github.com/applese233/ICRL`,
			Tags: []string{"research", "icrl", "tool-use", "grpo", "reinforcement-learning"},
		},

		// ── Dashboard Features ──
		{
			Category:    "Architecture",
			Title:       "Dashboard Feature Set",
			Description: "Web dashboard with chat, task board, canvas, live agent topology, and browser view.",
			Content: `The iTaK Agent Dashboard provides:

1. Chat: Conversational interface with agent selector and tabbed right panel
2. Task Board: Kanban-style task management with real-time SSE updates
3. Canvas: Preview HTML artifacts, slides, and reports in an iframe
4. Live Agent Topology: Real-time Canvas 2D visualization of agent hierarchy and data flow
5. Browser View: Embedded iTaK Browser showing what agents are doing, with agent badge overlay
6. Database: Browse and search graph DB nodes with filtered views
7. Analytics: Agent activity metrics and response time charts
8. Agents: Manage system agents, focused agents, model config, and Doctor status
9. Presentations: View generated slide decks and reports
10. Settings: Theme toggle, API configuration, debug settings`,
			Tags: []string{"dashboard", "ui", "features"},
		},

		// ── EventBus ──
		{
			Category:    "Architecture",
			Title:       "EventBus Real-Time Event System",
			Description: "Pub/sub event system for real-time SSE streaming to the dashboard.",
			Content: `The EventBus is a pub/sub system that streams events to connected dashboard clients via SSE.

Event Topics:
- agent.chat_complete: Orchestrator finished processing a chat message
- task.created, task.updated, task.deleted: Task CRUD operations
- doctor.lint_result: Health check lint results
- doctor.alert: Doctor detected an issue
- doctor.db_audit: Database integrity audit results
- doctor.activated, doctor.clear: Doctor healing state changes
- artifact.created: New HTML/slide/report artifact generated

Events include: topic, agent, message, timestamp, data (arbitrary JSON).
SSE fanout broadcasts all events to all connected clients.`,
			Tags: []string{"eventbus", "sse", "real-time", "events"},
		},
	}
}
