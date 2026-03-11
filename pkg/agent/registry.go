package agent

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// Capability represents a discrete capability that an agent can have.
// Used by the registry to match tasks to agents based on what they can do.
type Capability string

const (
	CapResearch   Capability = "research"    // web research, URL fetching, information gathering
	CapCode       Capability = "code"        // writing, debugging, running code
	CapBrowse     Capability = "browse"      // browser automation, web interaction
	CapFileOps    Capability = "file_ops"    // reading, writing, listing files and directories
	CapShell      Capability = "shell"       // running shell commands
	CapMemory     Capability = "memory"      // saving and recalling facts, entity tracking
	CapSearch     Capability = "search"      // code search, grep, pattern matching
	CapSkills     Capability = "skills"      // loading and executing skill packs
)

// AgentEntry holds metadata about a registered agent for the registry.
type AgentEntry struct {
	Name         string       `json:"name"`
	Role         string       `json:"role"`
	Personality  string       `json:"personality"`
	Tools        []string     `json:"tools"`
	Capabilities []Capability `json:"capabilities"`
	Status       string       `json:"status"` // "ready", "busy", "error"
}

// AgentRegistry is a thread-safe lookup table of all agent types and
// capabilities. The orchestrator uses it to build system prompts and
// route tasks to the right agent based on capability matching.
//
// Why: Without a registry, the orchestrator has to hard-code agent names
// in its system prompt. The registry makes routing dynamic -- add a new
// agent to the config and it auto-appears in the routing table.
//
// How: Agents register at startup via Register(). The orchestrator calls
// Describe() to get a formatted string for its system prompt, and
// FindByCapability() when it needs to dynamically route a task.
type AgentRegistry struct {
	mu      sync.RWMutex
	entries map[string]AgentEntry
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *AgentRegistry {
	return &AgentRegistry{
		entries: make(map[string]AgentEntry),
	}
}

// Register adds or updates an agent in the registry.
func (r *AgentRegistry) Register(entry AgentEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry.Status == "" {
		entry.Status = "ready"
	}

	// Auto-detect capabilities from tool names if not explicitly set.
	if len(entry.Capabilities) == 0 {
		entry.Capabilities = inferCapabilities(entry.Tools)
	}

	r.entries[entry.Name] = entry
	debug.Debug("registry", "Registered agent %q (role: %s, tools: %d, caps: %v)",
		entry.Name, entry.Role, len(entry.Tools), entry.Capabilities)
}

// Lookup returns a single agent entry by name.
func (r *AgentRegistry) Lookup(name string) (AgentEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e, ok
}

// FindByCapability returns all agents that have the given capability.
func (r *AgentRegistry) FindByCapability(cap Capability) []AgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []AgentEntry
	for _, e := range r.entries {
		for _, c := range e.Capabilities {
			if c == cap {
				matches = append(matches, e)
				break
			}
		}
	}
	return matches
}

// ListAll returns all registered agents, sorted by name.
func (r *AgentRegistry) ListAll() []AgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]AgentEntry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// Count returns the number of registered agents.
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// SetStatus updates the status of a registered agent.
func (r *AgentRegistry) SetStatus(name, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[name]; ok {
		e.Status = status
		r.entries[name] = e
	}
}

// Describe generates a formatted string listing all agents for use in
// the orchestrator's system prompt. This replaces the old hardcoded
// buildAgentDescriptions() method.
func (r *AgentRegistry) Describe() string {
	entries := r.ListAll()
	if len(entries) == 0 {
		return "(no agents registered)"
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- **%s** (role: %s): %s\n", e.Name, e.Role, e.Personality))
		if len(e.Capabilities) > 0 {
			caps := make([]string, len(e.Capabilities))
			for i, c := range e.Capabilities {
				caps[i] = string(c)
			}
			sb.WriteString(fmt.Sprintf("  Capabilities: %s\n", strings.Join(caps, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  Tools: %s\n", strings.Join(e.Tools, ", ")))
	}
	return sb.String()
}

// inferCapabilities auto-detects capabilities from tool names.
// This means agents don't need to manually declare capabilities --
// the registry figures it out from which tools they have.
func inferCapabilities(toolNames []string) []Capability {
	capSet := make(map[Capability]bool)

	for _, t := range toolNames {
		switch {
		case t == "shell":
			capSet[CapShell] = true
		case t == "file_read" || t == "file_write" || t == "dir_list":
			capSet[CapFileOps] = true
		case t == "http_fetch":
			capSet[CapResearch] = true
		case strings.HasPrefix(t, "web_"):
			capSet[CapBrowse] = true
		case t == "memory_save" || t == "memory_recall" || t == "conversation_search" || t == "conversation_read":
			capSet[CapMemory] = true
		case t == "grep_search":
			capSet[CapSearch] = true
		case t == "skill_list" || t == "skill_load":
			capSet[CapSkills] = true
		}
	}

	caps := make([]Capability, 0, len(capSet))
	for c := range capSet {
		caps = append(caps, c)
	}
	sort.Slice(caps, func(i, j int) bool {
		return string(caps[i]) < string(caps[j])
	})
	return caps
}
