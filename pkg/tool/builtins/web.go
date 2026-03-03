package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// ──────────────────────────────────────────────────────────────────
// WebNavigateTool — browse a URL and return page text
// ──────────────────────────────────────────────────────────────────

type WebNavigateTool struct{}

func (w *WebNavigateTool) Name() string { return "web_navigate" }

func (w *WebNavigateTool) Description() string {
	return "Navigate to a URL and return the page title and text content. Use this to read web pages."
}

func (w *WebNavigateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to navigate to (e.g. https://example.com)",
			},
		},
		"required": []string{"url"},
	}
}

func (w *WebNavigateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url, ok := args["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("missing required argument: url")
	}

	// Ensure URL has a scheme.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	browser, page, err := launchBrowser(ctx)
	if err != nil {
		return "", err
	}
	defer browser.MustClose()

	err = page.Timeout(15 * time.Second).Navigate(url)
	if err != nil {
		return "", fmt.Errorf("navigate to %s: %w", url, err)
	}

	// Wait for page to be ready.
	page.MustWaitStable()

	title, _ := page.Eval(`() => document.title`)
	body, _ := page.Eval(`() => document.body.innerText`)

	var result strings.Builder
	if title != nil {
		result.WriteString(fmt.Sprintf("Title: %s\n\n", title.Value.Str()))
	}
	if body != nil {
		text := body.Value.Str()
		// Truncate to ~4000 chars to fit LLM context.
		if len(text) > 4000 {
			text = text[:4000] + "\n\n... (truncated, page has more content)"
		}
		result.WriteString(text)
	}

	return result.String(), nil
}

// ──────────────────────────────────────────────────────────────────
// WebScreenshotTool — capture a page as PNG
// ──────────────────────────────────────────────────────────────────

type WebScreenshotTool struct {
	DataDir string
}

func (w *WebScreenshotTool) Name() string { return "web_screenshot" }

func (w *WebScreenshotTool) Description() string {
	return "Take a screenshot of a web page and save it as a PNG file. Returns the file path."
}

func (w *WebScreenshotTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to screenshot (e.g. https://example.com)",
			},
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "Optional filename for the screenshot (default: auto-generated)",
			},
		},
		"required": []string{"url"},
	}
}

func (w *WebScreenshotTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url, ok := args["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("missing required argument: url")
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	// Determine output path.
	screenshotDir := filepath.Join(w.DataDir, "screenshots")
	os.MkdirAll(screenshotDir, 0o755)

	filename, _ := args["filename"].(string)
	if filename == "" {
		filename = fmt.Sprintf("screenshot_%d.png", time.Now().Unix())
	}
	if !strings.HasSuffix(filename, ".png") {
		filename += ".png"
	}
	outPath := filepath.Join(screenshotDir, filename)

	browser, page, err := launchBrowser(ctx)
	if err != nil {
		return "", err
	}
	defer browser.MustClose()

	err = page.Timeout(15 * time.Second).Navigate(url)
	if err != nil {
		return "", fmt.Errorf("navigate to %s: %w", url, err)
	}

	page.MustWaitStable()

	// Full page screenshot.
	screenshot, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: nil,
	})
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}

	if err := os.WriteFile(outPath, screenshot, 0o644); err != nil {
		return "", fmt.Errorf("save screenshot: %w", err)
	}

	absPath, _ := filepath.Abs(outPath)
	return fmt.Sprintf("Screenshot saved: %s", absPath), nil
}

// ──────────────────────────────────────────────────────────────────
// WebExtractTool — extract elements by CSS selector
// ──────────────────────────────────────────────────────────────────

type WebExtractTool struct{}

func (w *WebExtractTool) Name() string { return "web_extract" }

func (w *WebExtractTool) Description() string {
	return "Extract text from specific elements on a web page using a CSS selector. Use this for targeted data extraction."
}

func (w *WebExtractTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to extract from",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector to match elements (e.g. 'h1', '.title', '#main p')",
			},
		},
		"required": []string{"url", "selector"},
	}
}

func (w *WebExtractTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url, ok := args["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("missing required argument: url")
	}
	selector, ok := args["selector"].(string)
	if !ok || selector == "" {
		return "", fmt.Errorf("missing required argument: selector")
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	browser, page, err := launchBrowser(ctx)
	if err != nil {
		return "", err
	}
	defer browser.MustClose()

	err = page.Timeout(15 * time.Second).Navigate(url)
	if err != nil {
		return "", fmt.Errorf("navigate to %s: %w", url, err)
	}

	page.MustWaitStable()

	elements, err := page.Elements(selector)
	if err != nil {
		return fmt.Sprintf("No elements found matching selector '%s'", selector), nil
	}

	if len(elements) == 0 {
		return fmt.Sprintf("No elements found matching selector '%s'", selector), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d element(s) matching '%s':\n\n", len(elements), selector))

	for i, el := range elements {
		text, err := el.Text()
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if i >= 20 {
			result.WriteString(fmt.Sprintf("\n... and %d more elements", len(elements)-20))
			break
		}
		result.WriteString(fmt.Sprintf("[%d] %s\n", i+1, text))
	}

	return result.String(), nil
}

// ──────────────────────────────────────────────────────────────────
// launchBrowser — shared helper to launch headless Chrome via rod
// ──────────────────────────────────────────────────────────────────

func launchBrowser(ctx context.Context) (*rod.Browser, *rod.Page, error) {
	// Auto-download and launch Chromium.
	u := launcher.New().
		Headless(true).
		MustLaunch()

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect to browser: %w", err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		browser.MustClose()
		return nil, nil, fmt.Errorf("create page: %w", err)
	}

	return browser, page, nil
}
