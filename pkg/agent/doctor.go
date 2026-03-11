package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
)

// Doctor is the self-healing agent that monitors for errors, runs linters,
// and fixes problems automatically. It maintains a "fix memory" - a log of
// what fixed each error - so the same problem gets an instant fix next time.
// The Doctor has its own tiny LLM for independent analysis and a golden
// snapshot of known-good code for comparison when things break.
type Doctor struct {
	Bus            *eventbus.EventBus
	LLMClient      *llm.OpenAIClient // Doctor's own tiny LLM (independent of main agent)
	HealthInterval time.Duration     // default 30 minutes
	FixMemoryPath  string            // path to fix_memory.json
	GoldenDir      string            // path to .doctor/golden/ snapshot
	ProjectDir     string            // root of the project being monitored
	MaxFixAttempts int               // max auto-fix attempts per error (default: 3)
	FixCooldown    time.Duration     // minimum time between diagnose runs (default: 10s)

	mu           sync.RWMutex
	fixMemory    map[string]FixRecord // keyed by error signature
	running      bool
	healing      bool                 // true when actively fixing (orchestrator pauses)
	stopCh       chan struct{}
	lastDiagnose time.Time // rate limiting: when we last ran diagnose
	fixAttempts  int       // how many fix attempts this session
}

// FixRecord stores a past diagnosis and its successful fix.
type FixRecord struct {
	ErrorSignature string    `json:"error_signature"`
	ErrorText      string    `json:"error_text"`
	FixApplied     string    `json:"fix_applied"`
	FixCount       int       `json:"fix_count"`
	LastSeen       time.Time `json:"last_seen"`
}

// LintResult holds the output from running a linter.
type LintResult struct {
	Language string   `json:"language"`
	Tool     string   `json:"tool"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Success  bool     `json:"success"`
}

// NewDoctor creates a Doctor with default settings.
func NewDoctor(bus *eventbus.EventBus, dataDir string) *Doctor {
	goldenDir := filepath.Join(dataDir, ".doctor", "golden")
	_ = os.MkdirAll(goldenDir, 0o755)

	projectDir, _ := os.Getwd()

	d := &Doctor{
		Bus:            bus,
		HealthInterval: 30 * time.Minute,
		FixMemoryPath:  filepath.Join(dataDir, "fix_memory.json"),
		GoldenDir:      goldenDir,
		ProjectDir:     projectDir,
		MaxFixAttempts: 3,
		FixCooldown:    10 * time.Second,
		fixMemory:      make(map[string]FixRecord),
		stopCh:         make(chan struct{}),
	}
	d.loadFixMemory()
	return d
}

// SetLLM gives the Doctor its own LLM client for independent analysis.
// This should be a tiny model (qwen2.5:0.5b, qwen3:0.6b) so it runs
// even when the main agent's model is down or broken.
func (d *Doctor) SetLLM(client *llm.OpenAIClient) {
	d.LLMClient = client
	debug.Info("doctor", "LLM attached for independent analysis")
}

// Start begins the health check loop and subscribes to error events.
func (d *Doctor) Start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	debug.Info("doctor", "Starting health monitor (interval: %s)", d.HealthInterval)

	// Subscribe to agent errors for reactive healing.
	if d.Bus != nil {
		_, ch := d.Bus.Subscribe(16, eventbus.TopicAgentError)
		go func() {
			for e := range ch {
				debug.Info("doctor", "Error event received, running diagnosis")
				d.diagnose(e)
			}
		}()
	}

	// Background health loop.
	go func() {
		ticker := time.NewTicker(d.HealthInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				d.runHealthCheck()
			case <-d.stopCh:
				debug.Info("doctor", "Health monitor stopped")
				return
			}
		}
	}()
}

// Stop halts the health check loop.
func (d *Doctor) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		close(d.stopCh)
		d.running = false
	}
}

// DetectLanguage scans a directory and returns the primary programming language.
func DetectLanguage(dir string) string {
	counts := map[string]int{}

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			// Skip hidden dirs and common non-source dirs.
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			counts["go"]++
		case ".js", ".jsx", ".ts", ".tsx":
			counts["javascript"]++
		case ".py":
			counts["python"]++
		case ".rs":
			counts["rust"]++
		case ".java":
			counts["java"]++
		case ".rb":
			counts["ruby"]++
		case ".cs":
			counts["csharp"]++
		case ".cpp", ".cc", ".c", ".h":
			counts["cpp"]++
		}
		return nil
	})

	best := ""
	bestCount := 0
	for lang, count := range counts {
		if count > bestCount {
			best = lang
			bestCount = count
		}
	}
	return best
}

// LintProject runs the appropriate linter for the detected language.
func LintProject(dir string) LintResult {
	lang := DetectLanguage(dir)
	if lang == "" {
		return LintResult{Success: true, Language: "unknown"}
	}

	debug.Info("doctor", "Detected language: %s, running linter", lang)

	switch lang {
	case "go":
		return runGoLint(dir)
	case "javascript":
		return runJSLint(dir)
	case "python":
		return runPythonLint(dir)
	case "rust":
		return runRustLint(dir)
	default:
		debug.Info("doctor", "No linter configured for %s", lang)
		return LintResult{Language: lang, Success: true}
	}
}

// runGoLint runs `go vet` (always available) and `golangci-lint` if installed.
func runGoLint(dir string) LintResult {
	result := LintResult{Language: "go", Tool: "go vet"}

	// go vet is always available.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()

	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				result.Errors = append(result.Errors, line)
			}
		}
		result.Success = false
		debug.Warn("doctor", "go vet found %d issue(s)", len(result.Errors))
	} else {
		result.Success = true
		debug.Info("doctor", "go vet passed")
	}

	// Try golangci-lint if available.
	if _, lookErr := exec.LookPath("golangci-lint"); lookErr == nil {
		result.Tool = "golangci-lint"
		ctx2, cancel2 := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel2()

		cmd2 := exec.CommandContext(ctx2, "golangci-lint", "run", "--timeout", "90s", "--out-format", "line-number")
		cmd2.Dir = dir
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			lines := strings.Split(strings.TrimSpace(string(output2)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					result.Warnings = append(result.Warnings, line)
				}
			}
		}
	}

	return result
}

// runJSLint runs eslint if available.
func runJSLint(dir string) LintResult {
	result := LintResult{Language: "javascript", Tool: "eslint"}

	if _, err := exec.LookPath("npx"); err != nil {
		result.Success = true
		debug.Info("doctor", "npx not found, skipping JS lint")
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npx", "eslint", ".", "--format", "compact", "--max-warnings", "50")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()

	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				result.Errors = append(result.Errors, line)
			}
		}
		result.Success = false
	} else {
		result.Success = true
	}

	return result
}

// runPythonLint runs pylint or ruff if available.
func runPythonLint(dir string) LintResult {
	result := LintResult{Language: "python"}

	// Prefer ruff (faster).
	if _, err := exec.LookPath("ruff"); err == nil {
		result.Tool = "ruff"
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ruff", "check", ".", "--output-format", "text")
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					result.Warnings = append(result.Warnings, line)
				}
			}
			result.Success = false
		} else {
			result.Success = true
		}
		return result
	}

	// Fall back to pylint.
	if _, err := exec.LookPath("pylint"); err == nil {
		result.Tool = "pylint"
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "pylint", ".", "--output-format", "text", "--score", "no")
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					result.Warnings = append(result.Warnings, line)
				}
			}
			result.Success = false
		} else {
			result.Success = true
		}
		return result
	}

	result.Tool = "none"
	result.Success = true
	debug.Info("doctor", "No Python linter found (tried ruff, pylint)")
	return result
}

// runRustLint runs clippy if available.
func runRustLint(dir string) LintResult {
	result := LintResult{Language: "rust", Tool: "clippy"}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cargo", "clippy", "--", "-D", "warnings")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()

	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && strings.Contains(line, "warning") || strings.Contains(line, "error") {
				result.Warnings = append(result.Warnings, line)
			}
		}
		result.Success = false
	} else {
		result.Success = true
	}

	return result
}

// runHealthCheck performs a periodic system health check.
// On success: auto-snapshots golden copy. On failure: diffs golden, notifies user.
func (d *Doctor) runHealthCheck() {
	debug.Info("doctor", "Running periodic health check")

	// Signal that the Doctor is actively healing.
	d.setHealing(true)
	defer d.setHealing(false)

	cwd, err := os.Getwd()
	if err != nil {
		debug.Warn("doctor", "Cannot determine working directory: %v", err)
		return
	}

	result := LintProject(cwd)
	if !result.Success {
		debug.Warn("doctor", "Health check FAILED: %d error(s), %d warning(s) (%s/%s)",
			len(result.Errors), len(result.Warnings), result.Language, result.Tool)

		// Publish lint failure event.
		if d.Bus != nil {
			d.Bus.Publish(eventbus.Event{
				Topic:     "doctor.lint_result",
				Agent:     "doctor",
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"success":  result.Success,
					"language": result.Language,
					"tool":     result.Tool,
					"errors":   len(result.Errors),
					"warnings": len(result.Warnings),
				},
			})

			// Notify the user -- don't let them sit in a dead loop.
			summary := FormatLintResult(result)
			d.Bus.Publish(eventbus.Event{
				Topic:   "doctor.alert",
				Agent:   "doctor",
				Message: fmt.Sprintf("Build health check failed (%d errors). The Doctor is investigating.", len(result.Errors)),
				Data: map[string]interface{}{
					"severity": "error",
					"details":  truncate(summary, 500),
				},
			})
		}

		// Use the Doctor's own LLM to analyze (if attached).
		if len(result.Errors) > 0 {
			combined := strings.Join(result.Errors, "\n")
			analysis := d.AnalyzeWithLLM(combined, "lint_failure", "doctor")
			if analysis != "" {
				debug.Info("doctor", "Doctor's recommendation: %s", truncate(analysis, 300))
			}
		}
	} else {
		debug.Info("doctor", "Health check PASSED (%s)", result.Language)

		// Auto-snapshot golden copy after verified clean build.
		if snapErr := d.SnapshotGolden(cwd); snapErr != nil {
			debug.Debug("doctor", "Golden snapshot skipped: %v", snapErr)
		}
	}
}

// diagnose reacts to an error event: analyzes, attempts auto-fix, verifies, notifies.
func (d *Doctor) diagnose(e eventbus.Event) {
	// ── Rate limiting: don't spam diagnose ──
	d.mu.RLock()
	elapsed := time.Since(d.lastDiagnose)
	attempts := d.fixAttempts
	d.mu.RUnlock()

	if elapsed < d.FixCooldown {
		debug.Debug("doctor", "Skipping diagnosis (cooldown: %s remaining)", d.FixCooldown-elapsed)
		return
	}
	if attempts >= d.MaxFixAttempts {
		debug.Warn("doctor", "Max fix attempts (%d) reached this session, skipping", d.MaxFixAttempts)
		return
	}

	d.mu.Lock()
	d.lastDiagnose = time.Now()
	d.mu.Unlock()

	// ── Extract error info ──
	errText := e.Message
	if errText == "" {
		if msg, ok := e.Data["message"].(string); ok {
			errText = msg
		} else if msg, ok := e.Data["error"].(string); ok {
			errText = msg
		}
	}
	if errText == "" {
		return
	}

	category := "unknown"
	if cat, ok := e.Data["category"].(string); ok {
		category = cat
	}

	debug.Separator("doctor")
	debug.Info("doctor", "DIAGNOSIS: agent=%s category=%s attempt=%d/%d",
		e.Agent, category, attempts+1, d.MaxFixAttempts)
	debug.Info("doctor", "Error: %s", truncate(errText, 200))

	// Signal that the Doctor is actively healing.
	d.setHealing(true)
	defer d.setHealing(false)

	sig := errorSignature(errText)

	// ── Step 1: Check fix memory for known fix ──
	d.mu.RLock()
	fix, known := d.fixMemory[sig]
	d.mu.RUnlock()

	if known {
		debug.Info("doctor", "KNOWN ISSUE (seen %d times). Replaying fix: %s",
			fix.FixCount, truncate(fix.FixApplied, 100))

		d.mu.Lock()
		fix.FixCount++
		fix.LastSeen = time.Now()
		d.fixMemory[sig] = fix
		d.mu.Unlock()
		d.saveFixMemory()

		// Attempt to replay the known fix.
		fixed := d.replayKnownFix(fix)
		d.emitAlert(fixed, "known_fix_replay", e.Agent, category, errText,
			fmt.Sprintf("Replayed known fix: %s", fix.FixApplied))
		return
	}

	debug.Info("doctor", "NEW error -- starting auto-fix pipeline")

	// ── Step 2: Run linter to gather more context ──
	var lintResult LintResult
	if d.ProjectDir != "" {
		lintResult = LintProject(d.ProjectDir)
		if !lintResult.Success {
			debug.Warn("doctor", "Lint found %d error(s)", len(lintResult.Errors))
		}
	}

	// ── Step 3: Try golden revert for lint errors ──
	if !lintResult.Success && len(lintResult.Errors) > 0 {
		revertResult := d.tryGoldenRevert(lintResult)
		if revertResult != "" {
			debug.Info("doctor", "Golden revert applied: %s", truncate(revertResult, 100))

			// Verify the fix.
			if d.verifyBuild() {
				d.RecordFix(errText, "golden_revert: "+revertResult)
				d.emitAlert(true, "golden_revert", e.Agent, category, errText, revertResult)
				d.mu.Lock()
				d.fixAttempts++
				d.mu.Unlock()
				return
			}
			debug.Warn("doctor", "Golden revert didn't fix the build, continuing...")
		}
	}

	// ── Step 4: Use LLM to analyze and suggest fix ──
	analysis := d.AnalyzeWithLLM(errText, category, e.Agent)

	// ── Step 5: Notify user with diagnosis ──
	d.mu.Lock()
	d.fixAttempts++
	d.mu.Unlock()

	d.emitAlert(false, "diagnosis_complete", e.Agent, category, errText, analysis)
}

// replayKnownFix attempts to replay a previously successful fix.
// Returns true if the fix was replayed and build verified.
func (d *Doctor) replayKnownFix(fix FixRecord) bool {
	// Known fixes are stored as descriptions. For now, if it's a golden revert
	// we can re-apply it. Other fix types need the LLM to interpret.
	if strings.HasPrefix(fix.FixApplied, "golden_revert:") {
		// The fix was a golden revert -- just re-run it.
		debug.Info("doctor", "Re-running golden revert")
		if d.ProjectDir != "" {
			lintResult := LintProject(d.ProjectDir)
			if !lintResult.Success {
				revertResult := d.tryGoldenRevert(lintResult)
				if revertResult != "" && d.verifyBuild() {
					return true
				}
			}
		}
	}

	// For other fix types, log that manual intervention may be needed.
	debug.Info("doctor", "Known fix logged but cannot auto-replay: %s", truncate(fix.FixApplied, 100))
	return false
}

// tryGoldenRevert looks at lint errors, finds the affected files, and
// reverts them to their golden snapshot copies. Returns description of
// what was reverted, or empty string if nothing could be reverted.
func (d *Doctor) tryGoldenRevert(lint LintResult) string {
	if d.ProjectDir == "" || d.GoldenDir == "" {
		return ""
	}

	// Extract file paths from lint errors.
	// Go vet output formats:
	//   Unix:    main.go:4:2: undefined: foo
	//   Windows: vet.exe: .\main.go:4:2: undefined: foo
	revertedFiles := map[string]bool{}
	for _, errLine := range lint.Errors {
		line := errLine

		// Strip "vet.exe: " or similar tool prefix (Windows).
		if idx := strings.Index(line, ".go:"); idx >= 0 {
			// Find the start of the filename by scanning backward from .go:
			start := idx
			for start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
				start--
			}
			line = line[start:]
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		filePath := strings.TrimSpace(parts[0])

		// Clean up Windows-style paths.
		filePath = strings.TrimPrefix(filePath, ".\\")
		filePath = strings.TrimPrefix(filePath, "./")
		filePath = filepath.ToSlash(filePath) // normalize separators

		if filePath == "" || !strings.Contains(filePath, ".") {
			continue
		}

		// Skip already processed files.
		if revertedFiles[filePath] {
			continue
		}

		// Check if we have a golden copy.
		goldenPath := filepath.Join(d.GoldenDir, filePath)
		if _, err := os.Stat(goldenPath); err != nil {
			continue // No golden copy for this file.
		}

		// Read golden copy.
		goldenData, err := os.ReadFile(goldenPath)
		if err != nil {
			continue
		}

		// Back up current file first (for rollback).
		currentPath := filepath.Join(d.ProjectDir, filePath)
		backupPath := currentPath + ".doctor_backup"
		currentData, err := os.ReadFile(currentPath)
		if err != nil {
			continue
		}

		if err := os.WriteFile(backupPath, currentData, 0o644); err != nil {
			continue
		}

		// Revert to golden.
		if err := os.WriteFile(currentPath, goldenData, 0o644); err != nil {
			debug.Warn("doctor", "Failed to revert %s: %v", filePath, err)
			continue
		}

		debug.Info("doctor", "Reverted %s to golden snapshot (backup: %s)", filePath, backupPath)
		revertedFiles[filePath] = true
	}

	if len(revertedFiles) == 0 {
		return ""
	}

	files := make([]string, 0, len(revertedFiles))
	for f := range revertedFiles {
		files = append(files, f)
	}
	return fmt.Sprintf("reverted %d file(s): %s", len(files), strings.Join(files, ", "))
}

// verifyBuild runs `go vet ./...` and optionally `go build` to confirm the project compiles.
func (d *Doctor) verifyBuild() bool {
	if d.ProjectDir == "" {
		return false
	}

	debug.Info("doctor", "Verifying build after fix...")

	// go vet
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = d.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		debug.Warn("doctor", "Build verification FAILED: %s", truncate(string(output), 200))
		return false
	}

	debug.Info("doctor", "Build verification PASSED")
	return true
}

// rollbackFile restores a file from its .doctor_backup if the fix made things worse.
func (d *Doctor) rollbackFile(filePath string) bool {
	fullPath := filepath.Join(d.ProjectDir, filePath)
	backupPath := fullPath + ".doctor_backup"

	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		return false
	}

	if err := os.WriteFile(fullPath, backupData, 0o644); err != nil {
		return false
	}

	_ = os.Remove(backupPath) // Clean up backup.
	debug.Info("doctor", "Rolled back %s from backup", filePath)
	return true
}

// emitAlert publishes a doctor.alert event with fix status for the UI.
func (d *Doctor) emitAlert(fixed bool, action, sourceAgent, category, errText, details string) {
	if d.Bus == nil {
		return
	}

	severity := "warning"
	msg := fmt.Sprintf("Agent %q hit a %s error. ", sourceAgent, category)

	if fixed {
		severity = "info"
		msg += "Doctor FIXED it: " + truncate(details, 200)
	} else if details != "" {
		msg += "Doctor's analysis: " + truncate(details, 200)
	} else {
		msg += "Error: " + truncate(errText, 200)
	}

	d.Bus.Publish(eventbus.Event{
		Topic:   "doctor.alert",
		Agent:   "doctor",
		Message: msg,
		Data: map[string]interface{}{
			"severity":   severity,
			"category":   category,
			"source":     sourceAgent,
			"action":     action,
			"fixed":      fixed,
			"error_text": truncate(errText, 500),
			"details":    details,
		},
	})
}

// RecordFix stores a successful fix in memory for future recall.
func (d *Doctor) RecordFix(errorText, fixApplied string) {
	sig := errorSignature(errorText)

	d.mu.Lock()
	d.fixMemory[sig] = FixRecord{
		ErrorSignature: sig,
		ErrorText:      truncate(errorText, 500),
		FixApplied:     fixApplied,
		FixCount:       1,
		LastSeen:       time.Now(),
	}
	d.mu.Unlock()

	d.saveFixMemory()
	debug.Info("doctor", "Recorded fix for signature %q: %s", sig, truncate(fixApplied, 100))
}

// LookupFix checks if we have a known fix for an error.
func (d *Doctor) LookupFix(errorText string) (FixRecord, bool) {
	sig := errorSignature(errorText)
	d.mu.RLock()
	defer d.mu.RUnlock()
	fix, ok := d.fixMemory[sig]
	return fix, ok
}

// FixCount returns how many fixes are in memory.
func (d *Doctor) FixCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.fixMemory)
}

// errorSignature creates a stable hash-like key from an error message.
// Strips line numbers and paths to match similar errors.
func errorSignature(errText string) string {
	// Normalize: lowercase, strip file paths and line numbers.
	s := strings.ToLower(errText)
	// Remove file:line patterns like "main.go:42:"
	parts := strings.Fields(s)
	var sig []string
	for _, p := range parts {
		// Skip tokens that look like file:line references.
		if strings.Contains(p, ".go:") || strings.Contains(p, ".py:") ||
			strings.Contains(p, ".js:") || strings.Contains(p, ".rs:") {
			continue
		}
		// Skip pure numbers (line numbers).
		isNum := true
		for _, c := range p {
			if c < '0' || c > '9' {
				isNum = false
				break
			}
		}
		if isNum && len(p) < 6 {
			continue
		}
		sig = append(sig, p)
	}
	result := strings.Join(sig, " ")
	if len(result) > 200 {
		result = result[:200]
	}
	return result
}

// loadFixMemory reads the fix memory from disk.
func (d *Doctor) loadFixMemory() {
	data, err := os.ReadFile(d.FixMemoryPath)
	if err != nil {
		return // fresh start
	}

	var records []FixRecord
	if err := json.Unmarshal(data, &records); err != nil {
		debug.Warn("doctor", "Failed to parse fix memory: %v", err)
		return
	}

	d.mu.Lock()
	for _, r := range records {
		d.fixMemory[r.ErrorSignature] = r
	}
	d.mu.Unlock()

	debug.Info("doctor", "Loaded %d fix records from memory", len(records))
}

// saveFixMemory persists the fix memory to disk.
func (d *Doctor) saveFixMemory() {
	d.mu.RLock()
	records := make([]FixRecord, 0, len(d.fixMemory))
	for _, r := range d.fixMemory {
		records = append(records, r)
	}
	d.mu.RUnlock()

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		debug.Warn("doctor", "Failed to marshal fix memory: %v", err)
		return
	}

	// Ensure directory exists.
	dir := filepath.Dir(d.FixMemoryPath)
	_ = os.MkdirAll(dir, 0o755)

	if err := os.WriteFile(d.FixMemoryPath, data, 0o644); err != nil {
		debug.Warn("doctor", "Failed to save fix memory: %v", err)
	}
}

// FormatLintResult creates a human-readable summary of lint results.
func FormatLintResult(r LintResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Language: %s | Tool: %s | ", r.Language, r.Tool))

	if r.Success {
		sb.WriteString("PASS")
	} else {
		sb.WriteString(fmt.Sprintf("FAIL (%d errors, %d warnings)", len(r.Errors), len(r.Warnings)))
	}

	if len(r.Errors) > 0 {
		sb.WriteString("\n\nErrors:\n")
		for i, e := range r.Errors {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.Errors)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("  %s\n", e))
		}
	}

	if len(r.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for i, w := range r.Warnings {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.Warnings)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("  %s\n", w))
		}
	}

	return sb.String()
}

// ── Golden Snapshot System ────────────────────────────────────────
//
// After a successful build+test, call SnapshotGolden() to save a copy
// of all source files. When something breaks, the Doctor compares
// current code against the golden copy to find what changed.

// SnapshotGolden saves all source files from the project directory
// into the golden snapshot folder. Call after a verified build passes.
func (d *Doctor) SnapshotGolden(projectDir string) error {
	lang := DetectLanguage(projectDir)
	if lang == "" {
		return fmt.Errorf("no language detected in %s", projectDir)
	}

	// Map language to file extensions to snapshot.
	extMap := map[string][]string{
		"go":         {".go", ".mod", ".sum"},
		"javascript": {".js", ".jsx", ".ts", ".tsx", ".json"},
		"python":     {".py", ".toml", ".cfg"},
		"rust":       {".rs", ".toml"},
	}
	exts, ok := extMap[lang]
	if !ok {
		return fmt.Errorf("no snapshot extensions for language %s", lang)
	}

	count := 0
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		matched := false
		for _, e := range exts {
			if ext == e {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}

		// Compute the relative path and create the golden copy.
		rel, relErr := filepath.Rel(projectDir, path)
		if relErr != nil {
			return nil
		}

		dest := filepath.Join(d.GoldenDir, rel)
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		if writeErr := os.WriteFile(dest, data, 0o644); writeErr != nil {
			return nil
		}
		count++
		return nil
	})

	if err != nil {
		return fmt.Errorf("snapshot walk: %w", err)
	}

	debug.Info("doctor", "Golden snapshot saved: %d files from %s", count, projectDir)
	return nil
}

// DiffGolden compares a file's current content against its golden copy.
// Returns the diff output or empty string if no golden copy exists.
func (d *Doctor) DiffGolden(projectDir, relPath string) string {
	goldenPath := filepath.Join(d.GoldenDir, relPath)
	currentPath := filepath.Join(projectDir, relPath)

	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		return "" // No golden copy - can't diff.
	}

	currentData, err := os.ReadFile(currentPath)
	if err != nil {
		return fmt.Sprintf("File %s deleted (existed in golden snapshot)", relPath)
	}

	if string(goldenData) == string(currentData) {
		return "" // No changes.
	}

	// Count changed lines.
	goldenLines := strings.Split(string(goldenData), "\n")
	currentLines := strings.Split(string(currentData), "\n")

	var changes []string
	changes = append(changes, fmt.Sprintf("--- golden/%s (%d lines)", relPath, len(goldenLines)))
	changes = append(changes, fmt.Sprintf("+++ current/%s (%d lines)", relPath, len(currentLines)))

	// Simple line-by-line diff (not unified, but enough for the Doctor).
	maxLines := len(goldenLines)
	if len(currentLines) > maxLines {
		maxLines = len(currentLines)
	}
	diffCount := 0
	for i := 0; i < maxLines && diffCount < 20; i++ {
		golden := ""
		current := ""
		if i < len(goldenLines) {
			golden = goldenLines[i]
		}
		if i < len(currentLines) {
			current = currentLines[i]
		}
		if golden != current {
			if golden != "" {
				changes = append(changes, fmt.Sprintf("-%d: %s", i+1, golden))
			}
			if current != "" {
				changes = append(changes, fmt.Sprintf("+%d: %s", i+1, current))
			}
			diffCount++
		}
	}
	if diffCount >= 20 {
		changes = append(changes, "... (truncated, >20 line differences)")
	}

	return strings.Join(changes, "\n")
}

// AnalyzeWithLLM uses the Doctor's own tiny LLM to analyze an error.
// It sends the error text, any golden diff, and lint results to the model
// and asks for a diagnosis + suggested fix.
func (d *Doctor) AnalyzeWithLLM(errorText, category, agentName string) string {
	if d.LLMClient == nil {
		debug.Debug("doctor", "No LLM attached, skipping AI analysis")
		return ""
	}

	// Build context for the diagnosis.
	var contextParts []string
	contextParts = append(contextParts, fmt.Sprintf("ERROR from agent %q (category: %s):", agentName, category))
	contextParts = append(contextParts, errorText)

	// Check if there's a git diff we can include.
	cwd, _ := os.Getwd()
	if cwd != "" {
		cmd := exec.Command("git", "diff", "--stat", "HEAD")
		cmd.Dir = cwd
		if out, err := cmd.Output(); err == nil && len(out) > 0 {
			contextParts = append(contextParts, "\nRECENT GIT CHANGES:")
			contextParts = append(contextParts, string(out))
		}
	}

	prompt := strings.Join(contextParts, "\n")

	sysPrompt := `You are a code doctor. Analyze the error and suggest a fix.
Be concise. Output format:
DIAGNOSIS: (one line)
FIX: (one line, actionable)`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := d.LLMClient.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: prompt},
	}, nil)

	if err != nil {
		debug.Warn("doctor", "LLM analysis failed: %v", err)
		return ""
	}

	debug.Info("doctor", "AI diagnosis: %s", truncate(resp.Content, 200))
	return resp.Content
}

// setHealing toggles the healing flag and emits the corresponding event.
// When active=true, the orchestrator pauses delegation until clear.
func (d *Doctor) setHealing(active bool) {
	d.mu.Lock()
	d.healing = active
	d.mu.Unlock()

	if d.Bus == nil {
		return
	}

	if active {
		debug.Info("doctor", "HEALING ACTIVE -- orchestrator will pause delegation")
		d.Bus.Publish(eventbus.Event{
			Topic:     TopicDoctorActivated,
			Agent:     "doctor",
			Message:   "Doctor is actively diagnosing and fixing",
			Timestamp: time.Now(),
		})
	} else {
		debug.Info("doctor", "HEALING CLEAR -- orchestrator may resume")
		d.Bus.Publish(eventbus.Event{
			Topic:     TopicDoctorClear,
			Agent:     "doctor",
			Message:   "Doctor finished healing cycle",
			Timestamp: time.Now(),
		})
	}
}

// IsHealing returns whether the Doctor is currently in a healing cycle.
// Used by the API server's /v1/doctor endpoint and the orchestrator's
// isDoctorActive() check.
func (d *Doctor) IsHealing() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.healing
}
