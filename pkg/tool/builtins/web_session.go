package builtins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// ──────────────────────────────────────────────────────────────────
// BrowserSession — persistent, stealth browser for AI agents
// ──────────────────────────────────────────────────────────────────

// BrowserSession holds a persistent browser + page across tool calls.
// Features:
//   - Stealth mode: bypasses navigator.webdriver, plugin checks, etc.
//   - Persistent profile: cookies/localStorage survive restarts.
//   - Headed mode: visible browser for 2FA flows.
//   - Cookie management: save/load cookies to disk.
type BrowserSession struct {
	mu       sync.Mutex
	browser  *rod.Browser
	page     *rod.Page
	url      string
	dataDir  string // GOAgent data directory
	headed   bool   // true = visible browser window
}

// Global singleton session — shared by all browser tools.
var globalSession = &BrowserSession{}

// InitBrowserDataDir sets the data directory for browser profile and cookie persistence.
// Called once at startup from main.go.
func InitBrowserDataDir(dataDir string) {
	globalSession.SetDataDir(dataDir)
}

// SetDataDir configures where browser data lives (profiles, cookies, etc).
func (s *BrowserSession) SetDataDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataDir = dir
}

// SetHeaded controls whether the browser is visible (for 2FA).
func (s *BrowserSession) SetHeaded(headed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// If changing mode, close existing session so it relaunches.
	if s.browser != nil && s.headed != headed {
		_ = s.browser.Close()
		s.browser = nil
		s.page = nil
	}
	s.headed = headed
}

// profileDir returns the persistent Chrome profile directory.
func (s *BrowserSession) profileDir() string {
	base := s.dataDir
	if base == "" {
		base = "."
	}
	dir := filepath.Join(base, "browser_profile")
	os.MkdirAll(dir, 0o755)
	return dir
}

// cookieFile returns the path to the cookie storage file.
func (s *BrowserSession) cookieFile() string {
	base := s.dataDir
	if base == "" {
		base = "."
	}
	dir := filepath.Join(base, "browser_data")
	os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "cookies.json")
}

// GetSession returns the current browser session, creating one if needed.
// Uses stealth mode and persistent profile by default.
func (s *BrowserSession) GetSession() (*rod.Browser, *rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser != nil && s.page != nil {
		return s.browser, s.page, nil
	}

	// Build launcher with anti-detection flags.
	l := launcher.New().
		Headless(!s.headed).
		Leakless(false).
		// Persistent user profile — keeps cookies, localStorage, sessions.
		UserDataDir(s.profileDir()).
		// Anti-detection flags.
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-infobars").
		Set("no-first-run").
		Set("no-default-browser-check")

	u, err := l.Launch()
	if err != nil {
		return nil, nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect to browser: %w", err)
	}

	// Use stealth to create the page — bypasses most bot detection.
	page, err := stealth.Page(browser)
	if err != nil {
		_ = browser.Close()
		return nil, nil, fmt.Errorf("create stealth page: %w", err)
	}

	// Inject additional anti-detection overrides.
	page.MustEvalOnNewDocument(antiDetectionJS)

	// Set a realistic viewport.
	page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  1920,
		Height: 1080,
	})

	// Load saved cookies if they exist.
	s.loadCookies(browser)

	s.browser = browser
	s.page = page
	return browser, page, nil
}

// Navigate goes to a URL in the current session.
func (s *BrowserSession) Navigate(url string) (*rod.Page, error) {
	_, page, err := s.GetSession()
	if err != nil {
		return nil, err
	}

	err = page.Timeout(15 * time.Second).Navigate(url)
	if err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", url, err)
	}

	_ = page.Timeout(5 * time.Second).WaitStable(300 * time.Millisecond)

	s.mu.Lock()
	s.url = url
	s.mu.Unlock()

	return page, nil
}

// Page returns the current page (nil if no session).
func (s *BrowserSession) Page() *rod.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.page
}

// URL returns the last navigated URL.
func (s *BrowserSession) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

// Close shuts down the browser session, saving cookies first.
func (s *BrowserSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser != nil {
		s.saveCookiesLocked()
		_ = s.browser.Close()
	}
	s.browser = nil
	s.page = nil
	s.url = ""
}

// IsActive returns whether a browser session is currently running.
func (s *BrowserSession) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.browser != nil
}

// IsHeaded returns whether the browser is in visible mode.
func (s *BrowserSession) IsHeaded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.headed
}

// ── Cookie Persistence ───────────────────────────────────────────

type savedCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

// SaveCookies saves all browser cookies to disk (thread-safe).
func (s *BrowserSession) SaveCookies() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCookiesLocked()
}

func (s *BrowserSession) saveCookiesLocked() error {
	if s.browser == nil {
		return nil
	}

	cookies, err := s.browser.GetCookies()
	if err != nil {
		return err
	}

	saved := make([]savedCookie, 0, len(cookies))
	for _, c := range cookies {
		sc := savedCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  float64(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
		if c.SameSite != "" {
			sc.SameSite = string(c.SameSite)
		}
		saved = append(saved, sc)
	}

	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.cookieFile(), data, 0o644)
}

func (s *BrowserSession) loadCookies(browser *rod.Browser) {
	data, err := os.ReadFile(s.cookieFile())
	if err != nil {
		return // no saved cookies yet
	}

	var cookies []savedCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return
	}

	for _, c := range cookies {
		sameSite := proto.NetworkCookieSameSiteNone
		switch c.SameSite {
		case "Strict":
			sameSite = proto.NetworkCookieSameSiteStrict
		case "Lax":
			sameSite = proto.NetworkCookieSameSiteLax
		}

		_, _ = proto.NetworkSetCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  proto.TimeSinceEpoch(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: sameSite,
		}.Call(browser)
	}
}

// ── Anti-Detection JavaScript ────────────────────────────────────

// This JS runs on every new document to patch detectable browser properties.
// Combined with rod/stealth, this covers the major bot-detection vectors.
const antiDetectionJS = `() => {
	// Override navigator.webdriver (primary detection method).
	Object.defineProperty(navigator, 'webdriver', {
		get: () => undefined,
	});

	// Fix Chrome runtime — headless Chrome is missing window.chrome.
	if (!window.chrome) {
		window.chrome = { runtime: {} };
	}

	// Override permissions query for notifications.
	const origQuery = window.Notification && Notification.permission;
	if (window.Notification) {
		Notification.permission = 'default';
	}

	// Fix navigator.plugins — headless has empty array.
	Object.defineProperty(navigator, 'plugins', {
		get: () => [1, 2, 3, 4, 5],
	});

	// Fix navigator.languages — headless sometimes returns empty.
	Object.defineProperty(navigator, 'languages', {
		get: () => ['en-US', 'en'],
	});

	// Spoof WebGL vendor and renderer.
	const getParameter = WebGLRenderingContext.prototype.getParameter;
	WebGLRenderingContext.prototype.getParameter = function(parameter) {
		if (parameter === 37445) return 'Intel Inc.';
		if (parameter === 37446) return 'Intel Iris OpenGL Engine';
		return getParameter.apply(this, arguments);
	};
}`
