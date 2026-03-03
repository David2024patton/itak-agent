package builtins

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// ──────────────────────────────────────────────────────────────────
// BrowserSession — persistent browser that stays open across tools
// ──────────────────────────────────────────────────────────────────

// BrowserSession holds a persistent browser + page across tool calls.
// This enables multi-step flows like 2FA: navigate → type → wait → type code.
type BrowserSession struct {
	mu      sync.Mutex
	browser *rod.Browser
	page    *rod.Page
	url     string // last navigated URL
}

// Global singleton session — shared by all browser tools.
var globalSession = &BrowserSession{}

// GetSession returns the current browser session, creating one if needed.
func (s *BrowserSession) GetSession() (*rod.Browser, *rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browser != nil && s.page != nil {
		return s.browser, s.page, nil
	}

	// Launch a new browser.
	u, err := launcher.New().
		Headless(true).
		Leakless(false).
		Launch()
	if err != nil {
		return nil, nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect to browser: %w", err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		_ = browser.Close()
		return nil, nil, fmt.Errorf("create page: %w", err)
	}

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

// Close shuts down the browser session.
func (s *BrowserSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser != nil {
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
