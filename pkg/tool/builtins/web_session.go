package builtins

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────
// BrowserSession  -  thin wrapper that shells out to gobrowser CLI
// ──────────────────────────────────────────────────────────────────

// BrowserSession manages the iTaK Browser (gobrowser) via CLI calls.
// The gobrowser daemon auto-starts on the first command and persists
// Chrome between calls, eliminating cold-start latency.
type BrowserSession struct {
	mu            sync.Mutex
	sessionID     string
	active        bool
	headed        bool
	dataDir       string
	daemonReady   bool
	gobrowserPath string // resolved path to gobrowser binary
	keepaliveOnce sync.Once
}

// Global singleton session.
var globalSession = &BrowserSession{}

// InitBrowserDataDir sets the data directory for browser profile persistence.
func InitBrowserDataDir(dataDir string) {
	globalSession.SetDataDir(dataDir)
}

// CleanupBrowser closes the browser session on shutdown.
func CleanupBrowser() {
	if globalSession.IsActive() {
		globalSession.Close()
	}
}

func (s *BrowserSession) SetDataDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataDir = dir
}

func (s *BrowserSession) SetHeaded(headed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active && s.headed != headed {
		// Close current session, will re-open with new mode on next call.
		s.mu.Unlock()
		s.Close()
		s.mu.Lock()
	}
	s.headed = headed
}

func (s *BrowserSession) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *BrowserSession) IsHeaded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.headed
}

// resolveGoBrowserPath finds the gobrowser binary by checking multiple locations.
// Priority: 1) already resolved, 2) sibling Browser directory, 3) dist/, 4) PATH
func (s *BrowserSession) resolveGoBrowserPath() (string, error) {
	if s.gobrowserPath != "" {
		return s.gobrowserPath, nil
	}

	binaryName := "gobrowser"
	if runtime.GOOS == "windows" {
		binaryName = "gobrowser.exe"
	}

	// Get the executable's directory to find sibling Browser folder.
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	// Search paths relative to the agent binary location.
	searchPaths := []string{
		// Sibling Browser directory (e.g. iTaK Eco/Browser/gobrowser.exe)
		filepath.Join(exeDir, "..", "Browser", binaryName),
		filepath.Join(exeDir, "..", "Browser", "dist", binaryName),
		// Same directory as agent binary.
		filepath.Join(exeDir, binaryName),
		// Common iTaK paths.
		filepath.Join("e:", ".agent", "iTaK Eco", "Browser", binaryName),
		filepath.Join("e:", ".agent", "iTaK Eco", "Browser", "dist", binaryName),
	}

	for _, p := range searchPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			log.Printf("[browser] Found gobrowser at: %s", abs)
			s.gobrowserPath = abs
			return abs, nil
		}
	}

	// Fallback: check PATH.
	if p, err := exec.LookPath(binaryName); err == nil {
		log.Printf("[browser] Found gobrowser in PATH: %s", p)
		s.gobrowserPath = p
		return p, nil
	}

	return "", fmt.Errorf("gobrowser binary not found. Searched: %s and PATH", strings.Join(searchPaths, ", "))
}

// ensureDaemon starts the gobrowser daemon as a background process if it
// isn't already running. It retries with backoff and starts a keepalive goroutine.
func (s *BrowserSession) ensureDaemon() error {
	if s.daemonReady {
		// Quick health check.
		if isDaemonHealthy() {
			return nil
		}
		log.Println("[browser] Daemon health check failed, restarting...")
		s.daemonReady = false
	}

	// Already healthy?
	if isDaemonHealthy() {
		s.daemonReady = true
		s.startKeepalive()
		return nil
	}

	binPath, err := s.resolveGoBrowserPath()
	if err != nil {
		return err
	}

	// Retry with backoff: 3 attempts (1s, 2s, 4s wait between).
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("[browser] Retrying daemon start (attempt %d/3, waiting %v)...", attempt+1, wait)
			time.Sleep(wait)
		}

		log.Printf("[browser] Starting gobrowser daemon (attempt %d/3)...", attempt+1)
		cmd := exec.Command(binPath, "daemon", "start")
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			lastErr = fmt.Errorf("start gobrowser daemon: %w", err)
			continue
		}

		// Detach so it doesn't die with this process.
		go func() {
			_ = cmd.Wait()
		}()

		// Wait for the daemon to become healthy (up to 20 seconds).
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			if isDaemonHealthy() {
				log.Println("[browser] gobrowser daemon is ready")
				s.daemonReady = true
				s.startKeepalive()
				return nil
			}
		}

		lastErr = fmt.Errorf("gobrowser daemon did not respond within 20 seconds (attempt %d)", attempt+1)
	}

	return fmt.Errorf("gobrowser daemon failed after 3 attempts: %w", lastErr)
}

// startKeepalive launches a background goroutine that checks daemon health
// every 30 seconds and auto-restarts it if it dies.
func (s *BrowserSession) startKeepalive() {
	s.keepaliveOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				if !isDaemonHealthy() {
					log.Println("[browser] Keepalive: daemon is down, restarting...")
					s.mu.Lock()
					s.daemonReady = false
					s.active = false
					s.sessionID = ""
					s.mu.Unlock()

					// Force restart.
					binPath, err := s.resolveGoBrowserPath()
					if err != nil {
						log.Printf("[browser] Keepalive: cannot find gobrowser: %v", err)
						continue
					}
					cmd := exec.Command(binPath, "daemon", "start")
					if err := cmd.Start(); err != nil {
						log.Printf("[browser] Keepalive: failed to restart daemon: %v", err)
						continue
					}
					go func() { _ = cmd.Wait() }()

					// Wait for it.
					for i := 0; i < 20; i++ {
						time.Sleep(time.Second)
						if isDaemonHealthy() {
							log.Println("[browser] Keepalive: daemon recovered")
							s.mu.Lock()
							s.daemonReady = true
							s.mu.Unlock()
							break
						}
					}
				}
			}
		}()
	})
}

// isDaemonHealthy checks if the gobrowser daemon is responding.
func isDaemonHealthy() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:43721/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// EnsureSession starts the daemon and creates a browser session if needed.
func (s *BrowserSession) EnsureSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return nil
	}

	// Step 1: make sure the daemon is running.
	if err := s.ensureDaemon(); err != nil {
		return err
	}

	// Step 2: create a new browser session.
	args := []string{"session", "new", "--json"}
	if s.headed {
		args = append(args, "--headed")
	}

	out, err := s.runGoBrowserInternal(args...)
	if err != nil {
		return fmt.Errorf("start browser session: %w\nOutput: %s", err, out)
	}

	// Parse session ID from response.
	// gobrowser returns: {"data": {"session": "ses_..."}, "ok": true}
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(out), &resp); err == nil {
		if data, ok := resp["data"].(map[string]interface{}); ok {
			if sid, ok := data["session"].(string); ok {
				s.sessionID = sid
			}
		}
		// Fallback: check top-level session field.
		if s.sessionID == "" {
			if sid, ok := resp["session"].(string); ok {
				s.sessionID = sid
			}
		}
	}

	if s.sessionID == "" {
		log.Printf("[browser] WARNING: could not parse session ID from: %s", out)
	} else {
		log.Printf("[browser] Session started: %s", s.sessionID)
	}

	s.active = true
	return nil
}

// Close terminates the gobrowser session.
func (s *BrowserSession) Close() {
	s.mu.Lock()
	sid := s.sessionID
	s.active = false
	s.sessionID = ""
	s.mu.Unlock()

	args := []string{"session", "close", "--json"}
	if sid != "" {
		args = append(args, "-s", sid)
	}
	s.runGoBrowserInternal(args...) // best-effort
}

// Run executes a gobrowser command with the current session and returns the raw JSON output.
func (s *BrowserSession) Run(args ...string) (string, error) {
	if err := s.EnsureSession(); err != nil {
		return "", err
	}

	// Inject session flag if we have one.
	s.mu.Lock()
	sid := s.sessionID
	s.mu.Unlock()

	fullArgs := make([]string, 0, len(args)+3)
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs, "--json")
	if sid != "" {
		fullArgs = append(fullArgs, "-s", sid)
	}

	return s.runGoBrowserInternal(fullArgs...)
}

// RunParsed executes a gobrowser command and returns the parsed JSON response.
func (s *BrowserSession) RunParsed(args ...string) (map[string]interface{}, error) {
	out, err := s.Run(args...)
	if err != nil {
		return nil, err
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		// Return non-JSON output as a data field.
		return map[string]interface{}{"data": out}, nil
	}

	// Check for error in response.
	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		return resp, fmt.Errorf("gobrowser: %s", errMsg)
	}

	return resp, nil
}

// ── Internal helpers ─────────────────────────────────────────────

// runGoBrowserInternal executes the gobrowser binary and returns stdout.
func (s *BrowserSession) runGoBrowserInternal(args ...string) (string, error) {
	binPath, err := s.resolveGoBrowserPath()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(binPath, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		errOut := stderr.String()
		if errOut != "" {
			return stdout.String(), fmt.Errorf("%w: %s", err, errOut)
		}
		return stdout.String(), err
	}

	return stdout.String(), nil
}

