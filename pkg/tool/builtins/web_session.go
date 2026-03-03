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
// BrowserSession — persistent, stealth, multi-tab browser for AI agents
// ──────────────────────────────────────────────────────────────────

type BrowserSession struct {
	mu       sync.Mutex
	browser  *rod.Browser
	page     *rod.Page   // active tab
	pages    []*rod.Page // all open tabs
	activeTab int        // index into pages
	url      string
	dataDir  string
	headed   bool
}

// Global singleton session.
var globalSession = &BrowserSession{}

// InitBrowserDataDir sets the data directory for browser profile and cookie persistence.
func InitBrowserDataDir(dataDir string) {
	globalSession.SetDataDir(dataDir)
}

// CleanupBrowser saves cookies and kills any browser processes.
// Called during graceful shutdown to prevent orphan Chrome instances.
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
	if s.browser != nil && s.headed != headed {
		s.saveCookiesLocked()
		_ = s.browser.Close()
		s.browser = nil
		s.page = nil
		s.pages = nil
		s.activeTab = 0
	}
	s.headed = headed
}

func (s *BrowserSession) profileDir() string {
	base := s.dataDir
	if base == "" {
		base = "."
	}
	dir := filepath.Join(base, "browser_profile")
	os.MkdirAll(dir, 0o755)
	return dir
}

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
func (s *BrowserSession) GetSession() (*rod.Browser, *rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser != nil && s.page != nil {
		return s.browser, s.page, nil
	}

	l := launcher.New().
		Headless(!s.headed).
		Leakless(false).
		UserDataDir(s.profileDir()).
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

	page, err := stealth.Page(browser)
	if err != nil {
		_ = browser.Close()
		return nil, nil, fmt.Errorf("create stealth page: %w", err)
	}

	page.MustEvalOnNewDocument(antiDetectionJS)
	page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  1920,
		Height: 1080,
	})

	s.loadCookies(browser)
	s.browser = browser
	s.page = page
	s.pages = []*rod.Page{page}
	s.activeTab = 0
	return browser, page, nil
}

// Navigate goes to a URL in the active tab.
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

func (s *BrowserSession) Page() *rod.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.page
}

func (s *BrowserSession) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *BrowserSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser != nil {
		s.saveCookiesLocked()
		_ = s.browser.Close()
	}
	s.browser = nil
	s.page = nil
	s.pages = nil
	s.activeTab = 0
	s.url = ""
}

func (s *BrowserSession) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.browser != nil
}

func (s *BrowserSession) IsHeaded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.headed
}

// ── Tab Management ───────────────────────────────────────────────

// NewTab opens a new browser tab. Returns the tab index.
func (s *BrowserSession) NewTab(url string) (int, *rod.Page, error) {
	_, _, err := s.GetSession()
	if err != nil {
		return -1, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	page, err := stealth.Page(s.browser)
	if err != nil {
		return -1, nil, fmt.Errorf("new tab: %w", err)
	}

	page.MustEvalOnNewDocument(antiDetectionJS)
	page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  1920,
		Height: 1080,
	})

	s.pages = append(s.pages, page)
	idx := len(s.pages) - 1
	s.activeTab = idx
	s.page = page

	if url != "" {
		s.mu.Unlock()
		err = page.Timeout(15 * time.Second).Navigate(url)
		if err == nil {
			_ = page.Timeout(5 * time.Second).WaitStable(300 * time.Millisecond)
		}
		s.mu.Lock()
		s.url = url
	}

	return idx, page, nil
}

// SwitchTab activates a tab by index.
func (s *BrowserSession) SwitchTab(idx int) (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser == nil {
		return nil, fmt.Errorf("no browser session")
	}
	if idx < 0 || idx >= len(s.pages) {
		return nil, fmt.Errorf("tab index %d out of range (0-%d)", idx, len(s.pages)-1)
	}

	s.activeTab = idx
	s.page = s.pages[idx]

	// Bring the tab to focus.
	s.page.Activate()

	return s.page, nil
}

// CloseTab closes a tab by index. Cannot close the last tab.
func (s *BrowserSession) CloseTab(idx int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser == nil {
		return fmt.Errorf("no browser session")
	}
	if len(s.pages) <= 1 {
		return fmt.Errorf("cannot close the last tab — use web_close to end the session")
	}
	if idx < 0 || idx >= len(s.pages) {
		return fmt.Errorf("tab index %d out of range (0-%d)", idx, len(s.pages)-1)
	}

	_ = s.pages[idx].Close()

	// Remove from slice.
	s.pages = append(s.pages[:idx], s.pages[idx+1:]...)

	// Adjust active tab.
	if s.activeTab >= len(s.pages) {
		s.activeTab = len(s.pages) - 1
	}
	if idx == s.activeTab || s.activeTab >= len(s.pages) {
		s.activeTab = len(s.pages) - 1
	}
	s.page = s.pages[s.activeTab]

	return nil
}

// ListTabs returns info about all open tabs.
func (s *BrowserSession) ListTabs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser == nil {
		return nil
	}

	tabs := make([]string, 0, len(s.pages))
	for i, p := range s.pages {
		info, _ := p.Info()
		title := "untitled"
		url := "about:blank"
		if info != nil {
			title = info.Title
			url = info.URL
		}
		marker := "  "
		if i == s.activeTab {
			marker = "→ "
		}
		tabs = append(tabs, fmt.Sprintf("%s[%d] %s (%s)", marker, i, title, url))
	}
	return tabs
}

func (s *BrowserSession) TabCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pages)
}

func (s *BrowserSession) ActiveTabIndex() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTab
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
		return
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

const antiDetectionJS = `() => {
	Object.defineProperty(navigator, 'webdriver', {
		get: () => undefined,
	});
	if (!window.chrome) {
		window.chrome = { runtime: {} };
	}
	if (window.Notification) {
		Notification.permission = 'default';
	}
	Object.defineProperty(navigator, 'plugins', {
		get: () => [1, 2, 3, 4, 5],
	});
	Object.defineProperty(navigator, 'languages', {
		get: () => ['en-US', 'en'],
	});
	const getParameter = WebGLRenderingContext.prototype.getParameter;
	WebGLRenderingContext.prototype.getParameter = function(parameter) {
		if (parameter === 37445) return 'Intel Inc.';
		if (parameter === 37446) return 'Intel Iris OpenGL Engine';
		return getParameter.apply(this, arguments);
	};
}`
