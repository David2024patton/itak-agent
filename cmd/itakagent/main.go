package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/agent"
	"github.com/David2024patton/iTaKAgent/pkg/config"
	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKAgent/pkg/skill"
	"github.com/David2024patton/iTaKAgent/pkg/tool"
	"github.com/David2024patton/iTaKAgent/pkg/tool/builtins"
	"github.com/David2024patton/iTaKAgent/pkg/ui"
	"github.com/David2024patton/iTaKAgent/pkg/ws"
)

const version = "0.2.0"

func main() {
	// Suppress default Go logger (we use our own).
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// Parse flags.
	var debugMode, verboseMode bool
	var configPath string
	var command string

	filtered := make([]string, 0)
	for _, arg := range args {
		switch arg {
		case "--debug":
			debugMode = true
		case "--verbose", "-v":
			verboseMode = true
		case "--version":
			fmt.Printf("iTaKAgent v%s\n", version)
			os.Exit(0)
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(arg, "--config=") {
				configPath = strings.TrimPrefix(arg, "--config=")
			} else {
				filtered = append(filtered, arg)
			}
		}
	}

	if len(filtered) > 0 {
		command = filtered[0]
	}

	// Set debug level.
	if debugMode {
		debug.SetLevel(debug.LevelDebug)
	} else if verboseMode {
		debug.SetLevel(debug.LevelInfo)
	} else {
		debug.SetLevel(debug.LevelWarn)
	}

	// Handle subcommands.
	switch command {
	case "run", "chat", "":
		// Default: interactive REPL.
	case "version":
		fmt.Printf("iTaKAgent v%s\n", version)
		os.Exit(0)
	case "help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}

	// Find config file.
	if configPath == "" {
		for _, candidate := range []string{"itakagent.yaml", "itakagent.yml", "configs/example.yaml"} {
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}
	}
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "Error: No config file found. Create itakagent.yaml or use --config=path")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		debug.Error("main", "Failed to load config: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		debug.Error("main", "Invalid config: %v", err)
		fmt.Fprintf(os.Stderr, "Error: Invalid config: %v\n", err)
		os.Exit(1)
	}

	debug.Info("main", "iTaKAgent v%s starting", version)
	debug.Info("main", "Config: %s", configPath)
	debug.Info("main", "Orchestrator model: %s @ %s", cfg.Orchestrator.LLM.Model, cfg.Orchestrator.LLM.APIBase)

	// -- Initialize memory system --
	mem, err := memory.NewManager(cfg.DataDir, cfg.Memory.WindowSize)
	if err != nil {
		debug.Error("main", "Failed to initialize memory: %v", err)
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize memory: %v\n", err)
		os.Exit(1)
	}
	mem.Archive.StartConversation()
	restoredMsgs := mem.RestoreLastSession()
	debug.Info("main", "Memory system initialized (data: %s)", cfg.DataDir)

	// -- Initialize step logger (time-travel debugging) --
	trace, err := debug.NewStepLogger(cfg.DataDir)
	if err != nil {
		debug.Warn("main", "Failed to initialize step logger: %v (traces disabled)", err)
	}

	// -- Initialize token tracker --
	tokens := llm.NewTokenTracker()

	// -- Initialize event bus (central pub/sub for all subsystems) --
	bus := eventbus.New()

	// -- Start WebSocket server (for dashboard connections) --
	wsPort := 47200
	wsServer := ws.NewServer(bus, wsPort)
	if err := wsServer.Start(); err != nil {
		debug.Warn("main", "WebSocket server failed to start: %v (dashboard disabled)", err)
		wsServer = nil
	}

	// -- Create session workspace --
	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	var workspace *memory.Workspace
	if cfg.Memory.SessionWorkspace {
		workspace, err = memory.NewWorkspace(cfg.DataDir, sessionID)
		if err != nil {
			debug.Warn("main", "Failed to create session workspace: %v", err)
		}
		// Clean old session workspaces (keep last 10).
		memory.CleanOldSessions(cfg.DataDir, 10)
	}

	// Build the available tools catalog (with memory + safety config).
	workDir := ""
	if workspace != nil {
		workDir = workspace.WorkDir()
	}

	// Load skill repository.
	var skillRepo *skill.Repository
	skillsDir := filepath.Join(cfg.DataDir, "skills")
	if _, err := os.Stat(skillsDir); err == nil {
		skillRepo, err = skill.NewRepository(skillsDir)
		if err != nil {
			debug.Warn("main", "Failed to load skills: %v", err)
		}
	}

	allTools := buildToolCatalog(mem, cfg, workDir, skillRepo)

	// Initialize browser data directory.
	builtins.InitBrowserDataDir(cfg.DataDir)

	// -- Discover active providers and list their models --
	startCtx, startCancel := context.WithTimeout(context.Background(), 15*time.Second)
	activeProviderCount := 0
	if len(cfg.Orchestrator.Providers) > 0 {
		debug.Info("main", "Scanning %d configured providers for available models...", len(cfg.Orchestrator.Providers))
		for _, pc := range cfg.Orchestrator.Providers {
			if pc.APIBase == "" || pc.APIKey == "" {
				continue
			}
			client := llm.NewOpenAIClient(pc)
			models, err := client.ListModels(startCtx)
			if err != nil {
				debug.Warn("main", "Provider %s: model discovery failed: %v", pc.Provider, err)
				continue
			}
			activeProviderCount++
			// Show up to 10 model IDs.
			names := make([]string, 0, 10)
			for i, m := range models {
				if i >= 10 {
					names = append(names, fmt.Sprintf("...and %d more", len(models)-10))
					break
				}
				names = append(names, m.ID)
			}
			debug.Info("main", "Provider %s: %d models available [%s]",
				pc.Provider, len(models), strings.Join(names, ", "))
		}
	}
	startCancel()

	// -- Build the orchestrator's LLM client (with failover + budget) --
	var orchClient llm.Client
	orchClient = llm.NewOpenAIClient(cfg.Orchestrator.LLM) // primary

	// Wrap with FailoverClient if additional providers are active.
	if len(cfg.Orchestrator.Providers) > 0 {
		allProviders := []llm.ProviderConfig{cfg.Orchestrator.LLM}
		for _, pc := range cfg.Orchestrator.Providers {
			if pc.APIBase != "" && pc.APIKey != "" {
				// If provider has no explicit model, use the orchestrator's model.
				if pc.Model == "" {
					pc.Model = cfg.Orchestrator.LLM.Model
				}
				allProviders = append(allProviders, pc)
			}
		}
		if len(allProviders) > 1 {
			orchClient = llm.NewFailoverClient(allProviders)
			debug.Info("main", "Failover enabled: %d providers in chain", len(allProviders))
		}
	}

	// Wrap with BudgetClient if token budget is set.
	if cfg.Orchestrator.TokenBudget > 0 {
		var fallback llm.Client
		if cfg.Orchestrator.FallbackModel.Model != "" {
			fallback = llm.NewOpenAIClient(cfg.Orchestrator.FallbackModel)
			debug.Info("main", "Token budget: %d (fallback: %s)", cfg.Orchestrator.TokenBudget, cfg.Orchestrator.FallbackModel.Model)
		} else {
			debug.Info("main", "Token budget: %d (no fallback model)", cfg.Orchestrator.TokenBudget)
		}
		orchClient = llm.NewBudgetClient(orchClient, fallback, cfg.Orchestrator.TokenBudget)
	}

	// Create focused agents.
	agents := make(map[string]*agent.FocusedAgent)
	for _, agentYAML := range cfg.Agents {
		ac := agentYAML.ToAgentConfig()

		// Create per-agent LLM client.
		client := llm.NewOpenAIClient(ac.LLM)

		// Build per-agent tool registry (only listed tools).
		registry := tool.NewRegistry()
		for _, toolName := range ac.ToolNames {
			if t, ok := allTools[toolName]; ok {
				if err := registry.Register(t); err != nil {
					debug.Warn("main", "Tool registration: %v", err)
				}
			} else {
				debug.Warn("main", "Unknown tool %q for agent %q - skipping", toolName, ac.Name)
			}
		}

		fa := agent.NewFocusedAgent(ac, client, registry, mem, trace, tokens, bus, sessionID)
		agents[ac.Name] = fa
		ui.AgentReady(ac.Name, ac.Role, len(ac.ToolNames))
	}

	// Create orchestrator with the failover/budget-wrapped client.
	orchCfg := agent.OrchestratorConfig{
		LLM:            cfg.Orchestrator.LLM,
		SystemPrompt:   cfg.Orchestrator.SystemPrompt,
		MaxDelegations: cfg.Orchestrator.MaxDelegations,
		Memory: agent.MemoryConfig{
			AutoReflect:  cfg.Memory.AutoReflect,
			AutoEntities: cfg.Memory.AutoEntities,
		},
	}
	orch := agent.NewOrchestrator(orchCfg, agents, mem, trace, tokens, bus)
	orch.LLMClient = orchClient // Override with failover/budget client.

	// Print styled startup banner.
	skillCount := 0
	if skillRepo != nil {
		skillCount = skillRepo.Count()
	}
	ui.Banner(version, cfg.Orchestrator.LLM.Model, len(agents), skillCount, restoredMsgs)

	if activeProviderCount > 0 {
		debug.Info("main", "Active failover providers: %d", activeProviderCount)
	}

	if len(cfg.ShellSafety.DeniedCommands) > 0 {
		debug.Info("main", "Shell safety: %d custom denied commands loaded", len(cfg.ShellSafety.DeniedCommands))
	}

	// Wire status callback to animated spinner.
	spinner := ui.NewSpinner()
	orch.StatusFunc = func(status string) {
		spinner.Start(status)
	}

	// Publish system ready event.
	bus.Publish(eventbus.Event{
		Topic:   eventbus.TopicSystemReady,
		Message: fmt.Sprintf("iTaKAgent v%s ready with %d agents", version, len(agents)),
		Data: map[string]interface{}{
			"version":    version,
			"agents":     len(agents),
			"skills":     skillCount,
			"ws_port":    wsPort,
		},
	})

	// Interactive REPL.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown: close browser, archive conversation, close trace, stop WS, print tokens.
	shutdown := func() {
		spinner.Stop()
		bus.Publish(eventbus.NewEvent(eventbus.TopicSystemShutdown, "iTaKAgent shutting down"))
		builtins.CleanupBrowser() // Kill Chrome, save cookies.
		archiveConversation(mem)
		if wsServer != nil {
			wsServer.Stop()
		}
		bus.Close()
		if trace != nil {
			trace.Close()
		}
		if tokens != nil {
			debug.Info("main", tokens.Summary())
		}
		ui.Goodbye()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		shutdown()
		cancel()
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		ui.Prompt()
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			shutdown()
			break
		}

		response, err := orch.Run(ctx, input)
		spinner.Stop()
		if err != nil {
			debug.Error("main", "Orchestrator error: %v", err)
			ui.Error(fmt.Sprintf("Error: %v", err))
			continue
		}

		ui.Response(response)
	}
}

// buildToolCatalog creates the complete set of available tools.
func buildToolCatalog(mem *memory.Manager, cfg *config.Config, workDir string, skillRepo *skill.Repository) map[string]tool.Tool {
	tools := make(map[string]tool.Tool)

	// Shell tool.
	shellTool := &builtins.ShellTool{
		WorkDir:        workDir,
		DeniedCommands: cfg.ShellSafety.DeniedCommands,
	}
	tools["shell"] = shellTool

	// File tools.
	tools["file_read"] = &builtins.FileReadTool{}
	tools["file_write"] = &builtins.FileWriteTool{ProtectedPaths: cfg.ShellSafety.ProtectedPaths}

	// HTTP tool.
	tools["http_fetch"] = &builtins.HTTPFetchTool{}

	// Directory listing tool.
	tools["dir_list"] = &builtins.DirListTool{}

	// Memory tools.
	tools["memory_save"] = &builtins.MemorySaveTool{Manager: mem}
	tools["memory_recall"] = &builtins.MemoryRecallTool{Manager: mem}
	tools["conversation_search"] = &builtins.ConversationSearchTool{Manager: mem}
	tools["conversation_read"] = &builtins.ConversationReadTool{Manager: mem}

	// Web (browser) tools.
	tools["web_navigate"] = &builtins.WebNavigateTool{}
	tools["web_click"] = &builtins.WebClickTool{}
	tools["web_type"] = &builtins.WebTypeTool{}
	tools["web_scroll"] = &builtins.WebScrollTool{}
	tools["web_back"] = &builtins.WebBackTool{}
	tools["web_eval"] = &builtins.WebEvalTool{}
	tools["web_wait"] = &builtins.WebWaitTool{}
	tools["web_screenshot"] = &builtins.WebScreenshotTool{DataDir: cfg.DataDir}
	tools["web_extract"] = &builtins.WebExtractTool{}
	tools["web_pdf"] = &builtins.WebPDFTool{DataDir: cfg.DataDir}
	tools["web_search"] = &builtins.WebSearchTool{}
	tools["web_close"] = &builtins.WebCloseTool{}
	tools["web_snapshot"] = &builtins.WebSnapshotTool{}
	tools["web_cookies"] = &builtins.WebCookiesTool{}
	tools["web_headed"] = &builtins.WebHeadedTool{}
	tools["web_hover"] = &builtins.WebHoverTool{}
	tools["web_double_click"] = &builtins.WebDoubleClickTool{}
	tools["web_tab_new"] = &builtins.WebTabNewTool{}
	tools["web_tab_switch"] = &builtins.WebTabSwitchTool{}
	tools["web_tab_close"] = &builtins.WebTabCloseTool{}
	tools["web_tab_list"] = &builtins.WebTabListTool{}

	// Skill tools.
	tools["skill_list"] = &builtins.SkillListTool{Repo: skillRepo}
	tools["skill_load"] = &builtins.SkillLoadTool{Repo: skillRepo}

	debug.Info("main", "Tool catalog built: %d tools available", len(tools))
	return tools
}

// archiveConversation saves the current conversation to persistent storage.
func archiveConversation(mem *memory.Manager) {
	if mem == nil {
		return
	}

	// Gather metadata from the session.
	agentsUsed := []string{}
	toolsUsed := []string{}
	entities := []string{}
	tags := []string{}

	allEntities := mem.Entities.All()
	for _, e := range allEntities {
		entities = append(entities, e.Name)
	}

	err := mem.Archive.EndConversation(
		"Session archived on exit",
		fmt.Sprintf("Session %s", time.Now().Format("2006-01-02 15:04")),
		tags,
		agentsUsed,
		toolsUsed,
		entities,
	)
	if err != nil {
		debug.Warn("main", "Failed to archive conversation: %v", err)
	}
}

func printUsage() {
	fmt.Printf(`iTaKAgent v%s - Lightweight AI Agent Framework

Usage: itakagent [command] [flags]

Commands:
  run, chat    Start interactive REPL (default)
  version      Print version
  help         Show this help

Flags:
  --config=PATH  Config file path (default: itakagent.yaml)
  --debug        Enable debug logging (very verbose)
  --verbose, -v  Enable info logging
  --version      Print version and exit
  --help, -h     Show this help

Examples:
  itakagent run
  itakagent run --debug
  itakagent chat --config=configs/example.yaml
  itakagent run --verbose
`, version)
}
