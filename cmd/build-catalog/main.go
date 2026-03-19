package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MarketplaceItem is the unified catalog entry.
type MarketplaceItem struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	DisplayName    string   `json:"display_name"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Division       string   `json:"division,omitempty"`
	Author         string   `json:"author"`
	Version        string   `json:"version"`
	Tags           []string `json:"tags"`
	Icon           string   `json:"icon"`
	IsCore         bool     `json:"is_core"`
	RequiresSkills []string `json:"requires_skills,omitempty"`
	RequiresTools  []string `json:"requires_tools,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	DownloadURL    string   `json:"download_url,omitempty"`
}

// AgencyCatalogEntry matches the seed agency_catalog.json format.
type AgencyCatalogEntry struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Division    string   `json:"division"`
	Source      string   `json:"source"`
	Tools       []string `json:"tools"`
}

// Core agents that ship pre-installed.
var coreAgents = map[string]bool{
	"scout":      true,
	"operator":   true,
	"browser":    true,
	"researcher": true,
	"coder":      true,
	"architect":  true,
}

// Agent-to-skill dependency map. Agents that need specific skills to function.
var agentSkillDeps = map[string][]string{
	"seo-specialist":        {"web-scraper", "report-writer"},
	"content-creator":       {"report-writer"},
	"frontend-developer":    {"code-reviewer"},
	"backend-architect":     {"code-reviewer", "db-admin"},
	"ai-engineer":           {"prompt-engineer"},
	"devops-automator":      {"ci-cd-builder", "docker-ops"},
	"security-engineer":     {"vulnerability-scanner"},
	"database-optimizer":    {"db-admin"},
	"data-engineer":         {"data-pipeline"},
	"data-remediation":      {"data-pipeline"},
	"growth-hacker":         {"web-scraper", "report-writer"},
	"analytics-reporter":    {"report-writer"},
	"api-tester":            {"api-tester"},
	"performance-benchmarker": {"report-writer"},
	"visual-tester":         {"web-scraper"},
	"smart-contract-engineer": {"vulnerability-scanner"},
	"blockchain-auditor":    {"vulnerability-scanner"},
	"incident-response":     {"report-writer"},
	"threat-detection":      {"network-monitor", "vulnerability-scanner"},
	"tracking-specialist":   {"web-scraper"},
	"ppc-strategist":        {"report-writer"},
	"paid-media-auditor":    {"report-writer"},
	"infra-maintainer":      {"network-monitor"},
	"sre":                   {"network-monitor", "report-writer"},
	"mcp-builder":           {"code-reviewer"},
	"document-generator":    {"report-writer"},
}

// Category icons for agent divisions.
var divisionIcons = map[string]string{
	"Engineering":        "⚙️",
	"Design":             "🎨",
	"Paid Media":         "📢",
	"Sales":              "💼",
	"Marketing":          "📣",
	"Product":            "📦",
	"Project Management": "📋",
	"Testing":            "🧪",
	"Support":            "🛠️",
	"Specialized":        "🔬",
	"Game Development":   "🎮",
}

// Tool definitions: each builtin tool grouped by family.
type ToolFamily struct {
	Name        string
	DisplayName string
	Description string
	Category    string
	Tags        []string
	Icon        string
	ToolNames   []string // individual tool function names in this family
}

var toolFamilies = []ToolFamily{
	{
		Name: "file-ops", DisplayName: "File Operations",
		Description: "Read, write, and manage files on the local filesystem. Includes file_read and file_write tools.",
		Category: "core", Tags: []string{"file", "read", "write", "filesystem"}, Icon: "📁",
		ToolNames: []string{"file_read", "file_write"},
	},
	{
		Name: "shell", DisplayName: "Shell Executor",
		Description: "Execute shell commands with safety guardrails. Runs PowerShell on Windows, bash on Linux/macOS.",
		Category: "core", Tags: []string{"shell", "command", "terminal", "exec"}, Icon: "💻",
		ToolNames: []string{"shell"},
	},
	{
		Name: "web-browser", DisplayName: "Web Browser Suite",
		Description: "Full browser automation: navigate, click, type, screenshot, extract data, manage tabs and cookies.",
		Category: "core", Tags: []string{"browser", "web", "automation", "scraping"}, Icon: "🌐",
		ToolNames: []string{"web_navigate", "web_click", "web_type", "web_scroll", "web_back", "web_eval", "web_wait", "web_screenshot", "web_extract", "web_pdf", "web_close", "web_snapshot", "web_cookies", "web_headed", "web_hover", "web_double_click", "web_focus", "web_keys", "web_tab_new", "web_tab_switch", "web_tab_close", "web_tab_list"},
	},
	{
		Name: "web-search", DisplayName: "Web Search",
		Description: "Search the web using multiple providers (SearXNG, DuckDuckGo). Returns structured search results.",
		Category: "core", Tags: []string{"search", "web", "query"}, Icon: "🔎",
		ToolNames: []string{"web_search"},
	},
	{
		Name: "http-client", DisplayName: "HTTP Client",
		Description: "Make HTTP requests (GET, POST, PUT, DELETE) to any URL. Supports custom headers and JSON bodies.",
		Category: "core", Tags: []string{"http", "api", "fetch", "rest"}, Icon: "📡",
		ToolNames: []string{"http_fetch"},
	},
	{
		Name: "memory", DisplayName: "Memory System",
		Description: "Save facts, recall knowledge, and maintain entity tracking across sessions. Powers long-term agent memory.",
		Category: "core", Tags: []string{"memory", "recall", "save", "knowledge"}, Icon: "🧠",
		ToolNames: []string{"memory_save", "memory_recall"},
	},
	{
		Name: "grep-search", DisplayName: "Code Search (Grep)",
		Description: "Search file contents using patterns. Find code, text, and data across directories with regex support.",
		Category: "core", Tags: []string{"grep", "search", "regex", "code"}, Icon: "🔍",
		ToolNames: []string{"grep_search"},
	},
	{
		Name: "dir-listing", DisplayName: "Directory Listing",
		Description: "List directory contents with file sizes, types, and modification dates.",
		Category: "core", Tags: []string{"directory", "list", "files"}, Icon: "📂",
		ToolNames: []string{"dir_list"},
	},
	{
		Name: "skill-manager", DisplayName: "Skill Manager",
		Description: "List and load skill instructions dynamically. Agents can discover and use skills at runtime.",
		Category: "core", Tags: []string{"skills", "list", "load"}, Icon: "📚",
		ToolNames: []string{"skill_list", "skill_load"},
	},
	{
		Name: "network-tools", DisplayName: "Network Diagnostics",
		Description: "DNS lookup, ping, traceroute, port scanning, and WHOIS queries for network troubleshooting.",
		Category: "advanced", Tags: []string{"network", "dns", "ping", "port"}, Icon: "🌐",
		ToolNames: []string{"net_dns", "net_ping", "net_traceroute", "net_portscan", "net_whois"},
	},
	{
		Name: "report-gen", DisplayName: "Report Generator",
		Description: "Generate formatted PDF and HTML reports with charts, tables, and executive summaries.",
		Category: "advanced", Tags: []string{"report", "pdf", "html", "chart"}, Icon: "📊",
		ToolNames: []string{"report_generate"},
	},
	{
		Name: "slide-gen", DisplayName: "Slide Generator",
		Description: "Generate presentation slides from structured content. Outputs HTML slide decks.",
		Category: "advanced", Tags: []string{"slides", "presentation", "deck"}, Icon: "🎞️",
		ToolNames: []string{"slide_generate"},
	},
	{
		Name: "code-index", DisplayName: "Code Indexer",
		Description: "Index and search codebases for symbols, functions, and cross-references.",
		Category: "advanced", Tags: []string{"code", "index", "symbols"}, Icon: "🗂️",
		ToolNames: []string{"code_index_search"},
	},
}

// Plugins (kept from original catalog).
var plugins = []MarketplaceItem{
	{Name: "web", Type: "plugin", DisplayName: "Web API", Description: "REST API server plugin. Exposes /v1/chat and the embedded dashboard for browser-based interaction.", Category: "channel", Author: "iTaK Core", Version: "0.2.0", Tags: []string{"api", "rest", "http"}, Icon: "🌐", IsCore: true},
	{Name: "dashboard", Type: "plugin", DisplayName: "Dashboard WebSocket", Description: "WebSocket event relay for the live dashboard. Streams agent activity, tool calls, and system events in real-time.", Category: "channel", Author: "iTaK Core", Version: "0.2.0", Tags: []string{"websocket", "dashboard", "realtime"}, Icon: "📺", IsCore: true},
	{Name: "discord", Type: "plugin", DisplayName: "Discord Bot", Description: "Discord bot integration. Chat with your agents directly from Discord channels with thread-based conversations.", Category: "channel", Author: "iTaK Core", Version: "0.2.0", Tags: []string{"discord", "bot", "chat"}, Icon: "💬"},
	{Name: "visionclaw", Type: "plugin", DisplayName: "VisionClaw Glasses", Description: "Ray-Ban Meta smart glasses gateway. Voice commands + camera frames for hands-free agent interaction via OpenClaw protocol.", Category: "channel", Author: "iTaK Core", Version: "0.2.0", Tags: []string{"glasses", "vision", "ar"}, Icon: "👓"},
	{Name: "cli", Type: "plugin", DisplayName: "CLI REPL", Description: "Interactive terminal REPL. Type commands directly in your terminal for quick agent interaction.", Category: "channel", Author: "iTaK Core", Version: "0.2.0", Tags: []string{"cli", "terminal", "repl"}, Icon: "⌨️", IsCore: true},
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: build-catalog <agent-data-dir> <output-file>\n")
		fmt.Fprintf(os.Stderr, "  agent-data-dir: path to Agent/ directory\n")
		fmt.Fprintf(os.Stderr, "  output-file: path to write catalog.json\n")
		os.Exit(1)
	}
	agentDir := os.Args[1]
	outputFile := os.Args[2]

	var catalog []MarketplaceItem

	// ── 1. Load agents from agency_catalog.json ──────────────────
	agencyCatalogPath := filepath.Join(agentDir, "pkg", "seed", "agency_catalog.json")
	agents, err := loadAgencyCatalog(agencyCatalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not load agency catalog: %v\n", err)
	}
	for _, a := range agents {
		icon := divisionIcons[a.Division]
		if icon == "" {
			icon = "🤖"
		}
		item := MarketplaceItem{
			Name:          a.Name,
			Type:          "agent",
			DisplayName:   a.Role,
			Description:   a.Description,
			Category:      a.Category,
			Division:      a.Division,
			Author:        "iTaK Labs",
			Version:       "1.0.0",
			Tags:          buildTags(a.Category, a.Division),
			Icon:          icon,
			IsCore:        coreAgents[a.Name],
			RequiresTools: a.Tools,
		}
		if deps, ok := agentSkillDeps[a.Name]; ok {
			item.RequiresSkills = deps
		}
		catalog = append(catalog, item)
	}
	fmt.Printf("  Loaded %d agents\n", len(agents))

	// ── 2. Scan skills from data/skills/ ─────────────────────────
	skillsDir := filepath.Join(agentDir, "data", "skills")
	skills := scanSkills(skillsDir)
	catalog = append(catalog, skills...)
	fmt.Printf("  Loaded %d skills\n", len(skills))

	// ── 3. Add tool family entries ───────────────────────────────
	for _, tf := range toolFamilies {
		item := MarketplaceItem{
			Name:        tf.Name,
			Type:        "tool",
			DisplayName: tf.DisplayName,
			Description: tf.Description,
			Category:    tf.Category,
			Author:      "iTaK Core",
			Version:     "0.2.0",
			Tags:        tf.Tags,
			Icon:        tf.Icon,
			IsCore:      tf.Category == "core",
			Tools:       tf.ToolNames,
		}
		catalog = append(catalog, item)
	}
	fmt.Printf("  Added %d tool families\n", len(toolFamilies))

	// ── 4. Add plugins ───────────────────────────────────────────
	catalog = append(catalog, plugins...)
	fmt.Printf("  Added %d plugins\n", len(plugins))

	// ── Write output ─────────────────────────────────────────────
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: marshal catalog: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: create output dir: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputFile, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: write catalog: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nWrote %d items to %s\n", len(catalog), outputFile)
}

func loadAgencyCatalog(path string) ([]AgencyCatalogEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []AgencyCatalogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func scanSkills(dir string) []MarketplaceItem {
	var items []MarketplaceItem

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not read skills dir: %v\n", err)
		return items
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMD := filepath.Join(dir, e.Name(), "SKILL.md")
		name, desc, tags := parseSkillMD(skillMD)
		if name == "" {
			name = e.Name()
		}
		if desc == "" {
			desc = "Skill: " + name
		}

		items = append(items, MarketplaceItem{
			Name:        e.Name(),
			Type:        "skill",
			DisplayName: strings.ReplaceAll(name, "-", " "),
			Description: desc,
			Category:    categorizeSkill(e.Name(), tags),
			Author:      "iTaK Labs",
			Version:     "1.0.0",
			Tags:        tags,
			Icon:        "📦",
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

// parseSkillMD reads YAML frontmatter from a SKILL.md file.
func parseSkillMD(path string) (name, description string, tags []string) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}

		if !inFrontmatter {
			continue
		}

		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		}
		if strings.HasPrefix(line, "description:") {
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			description = strings.Trim(description, "\"'")
		}
	}

	return name, description, tags
}

func categorizeSkill(name string, tags []string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "golang") || strings.Contains(lower, "python") || strings.Contains(lower, "code"):
		return "coding"
	case strings.Contains(lower, "docker") || strings.Contains(lower, "deploy") || strings.Contains(lower, "github"):
		return "automation"
	case strings.Contains(lower, "security") || strings.Contains(lower, "guard") || strings.Contains(lower, "vuln"):
		return "security"
	case strings.Contains(lower, "agent") || strings.Contains(lower, "llm") || strings.Contains(lower, "model"):
		return "ai"
	case strings.Contains(lower, "database") || strings.Contains(lower, "data") || strings.Contains(lower, "rag"):
		return "data"
	case strings.Contains(lower, "design") || strings.Contains(lower, "brand") || strings.Contains(lower, "art"):
		return "creative"
	case strings.Contains(lower, "web") || strings.Contains(lower, "frontend") || strings.Contains(lower, "browser"):
		return "web"
	case strings.Contains(lower, "doc") || strings.Contains(lower, "pdf") || strings.Contains(lower, "xlsx"):
		return "documents"
	case strings.Contains(lower, "prompt") || strings.Contains(lower, "humanizer"):
		return "ai"
	default:
		return "utility"
	}
}

func buildTags(category, division string) []string {
	tags := []string{category}
	if division != "" {
		tags = append(tags, strings.ToLower(division))
	}
	return tags
}
