package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// annotatedSnapshot JS — enumerates interactive elements with @e refs.
// Shared between web_navigate and web_snapshot.
const annotatedSnapshotJS = `() => {
	const els = document.querySelectorAll('a, button, input, textarea, select, [role="button"], [onclick]');
	const items = [];
	let id = 1;
	for (const el of els) {
		if (id > 30) break;
		const tag = el.tagName.toLowerCase();
		const type = el.getAttribute('type') || '';
		const role = el.getAttribute('role') || '';
		const href = el.getAttribute('href') || '';
		const placeholder = el.getAttribute('placeholder') || '';
		const ariaLabel = el.getAttribute('aria-label') || '';
		let label = (el.innerText || '').trim().substring(0, 60);
		if (!label) label = ariaLabel || placeholder || el.getAttribute('name') || el.getAttribute('id') || '';
		let desc = '';
		if (tag === 'a') {
			desc = 'link "' + label + '"';
			if (href && href !== '#') desc += ' → ' + href.substring(0, 80);
		} else if (tag === 'button' || role === 'button') {
			desc = 'button "' + label + '"';
		} else if (tag === 'input') {
			desc = 'input[' + (type || 'text') + ']';
			if (placeholder) desc += ' placeholder="' + placeholder + '"';
			if (label) desc += ' "' + label + '"';
		} else if (tag === 'textarea') {
			desc = 'textarea';
			if (placeholder) desc += ' placeholder="' + placeholder + '"';
		} else if (tag === 'select') {
			desc = 'select "' + label + '"';
		} else {
			desc = tag + ' "' + label + '"';
		}
		if (desc && label !== '') {
			el.setAttribute('data-ref', 'e' + id);
			items.push('[@e' + id + '] ' + desc);
			id++;
		}
	}
	return items.join('\n');
}`

// resolveRef converts "@e5" → "[data-ref='e5']" CSS selector.
func resolveRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "@e") {
		return fmt.Sprintf("[data-ref='%s']", ref[1:])
	}
	// Already a CSS selector.
	return ref
}

// getPageSnapshot returns page title + text + annotated elements.
func getPageSnapshot(page *rod.Page) string {
	title, _ := page.Eval(`() => document.title`)
	body, _ := page.Eval(`() => document.body.innerText`)

	var result strings.Builder
	if title != nil {
		result.WriteString(fmt.Sprintf("Title: %s\n", title.Value.Str()))
	}

	currentURL, _ := page.Eval(`() => window.location.href`)
	if currentURL != nil {
		result.WriteString(fmt.Sprintf("URL: %s\n\n", currentURL.Value.Str()))
	}

	if body != nil {
		text := body.Value.Str()
		if len(text) > 3000 {
			text = text[:3000] + "\n... (truncated)"
		}
		result.WriteString(text)
	}

	elements, _ := page.Eval(annotatedSnapshotJS)
	if elements != nil {
		elemText := elements.Value.Str()
		if elemText != "" {
			result.WriteString("\n\n─── Interactive Elements ───\n")
			result.WriteString(elemText)
		}
	}

	return result.String()
}

// ══════════════════════════════════════════════════════════════════
// web_navigate — go to a URL, return page + elements
// ══════════════════════════════════════════════════════════════════

type WebNavigateTool struct{}

func (w *WebNavigateTool) Name() string        { return "web_navigate" }
func (w *WebNavigateTool) Description() string {
	return "Navigate to a URL. Returns page title, text content, and interactive elements with @e refs. The browser stays open for follow-up actions (click, type, etc)."
}
func (w *WebNavigateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": prop("string", "URL to navigate to (e.g. https://example.com)"),
		},
		"required": []string{"url"},
	}
}

func (w *WebNavigateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url := argStr(args, "url")
	if url == "" {
		return "", fmt.Errorf("missing required argument: url")
	}
	url = ensureScheme(url)

	page, err := globalSession.Navigate(url)
	if err != nil {
		return "", err
	}
	return getPageSnapshot(page), nil
}

// ══════════════════════════════════════════════════════════════════
// web_click — click an element by @ref or CSS selector
// ══════════════════════════════════════════════════════════════════

type WebClickTool struct{}

func (w *WebClickTool) Name() string        { return "web_click" }
func (w *WebClickTool) Description() string {
	return "Click an element on the current page using an @e ref (e.g. @e3) or CSS selector. Returns the page state after clicking."
}
func (w *WebClickTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or a CSS selector like #submit-btn"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebClickTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	ref := argStr(args, "ref")
	if ref == "" {
		return "", fmt.Errorf("missing required argument: ref")
	}

	selector := resolveRef(ref)
	el, err := page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", fmt.Errorf("element %q not found: %w", ref, err)
	}

	err = el.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return "", fmt.Errorf("click %q: %w", ref, err)
	}

	// Wait for navigation or DOM changes.
	_ = page.Timeout(5 * time.Second).WaitStable(300 * time.Millisecond)

	return getPageSnapshot(page), nil
}

// ══════════════════════════════════════════════════════════════════
// web_type — type text into an input by @ref or CSS selector
// ══════════════════════════════════════════════════════════════════

type WebTypeTool struct{}

func (w *WebTypeTool) Name() string        { return "web_type" }
func (w *WebTypeTool) Description() string {
	return "Type text into an input field using an @e ref (e.g. @e2) or CSS selector. Optionally press Enter after typing."
}
func (w *WebTypeTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref":        prop("string", "Element ref like @e2 or CSS selector like input[name='email']"),
			"text":       prop("string", "Text to type into the element"),
			"press_enter": propBool("Press Enter after typing (default: false)"),
			"clear":      propBool("Clear existing text before typing (default: true)"),
		},
		"required": []string{"ref", "text"},
	}
}

func (w *WebTypeTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	ref := argStr(args, "ref")
	text := argStr(args, "text")
	if ref == "" || text == "" {
		return "", fmt.Errorf("missing required arguments: ref and text")
	}

	pressEnter, _ := args["press_enter"].(bool)
	clearFirst := true
	if v, ok := args["clear"].(bool); ok {
		clearFirst = v
	}

	selector := resolveRef(ref)
	el, err := page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", fmt.Errorf("element %q not found: %w", ref, err)
	}

	// Focus and optionally clear.
	el.Focus()
	if clearFirst {
		el.SelectAllText()
		el.MustType(input.Backspace)
	}

	el.Input(text)

	if pressEnter {
		el.MustType(input.Enter)
		_ = page.Timeout(5 * time.Second).WaitStable(300 * time.Millisecond)
	}

	return fmt.Sprintf("Typed %q into %s", text, ref), nil
}

// ══════════════════════════════════════════════════════════════════
// web_scroll — scroll the page up or down
// ══════════════════════════════════════════════════════════════════

type WebScrollTool struct{}

func (w *WebScrollTool) Name() string        { return "web_scroll" }
func (w *WebScrollTool) Description() string {
	return "Scroll the current page. Direction: 'down', 'up', 'bottom', 'top'. Use to reveal lazy-loaded content."
}
func (w *WebScrollTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"direction": prop("string", "Scroll direction: down, up, bottom, top"),
		},
		"required": []string{"direction"},
	}
}

func (w *WebScrollTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	dir := strings.ToLower(argStr(args, "direction"))
	switch dir {
	case "down":
		page.Eval(`() => window.scrollBy(0, window.innerHeight)`)
	case "up":
		page.Eval(`() => window.scrollBy(0, -window.innerHeight)`)
	case "bottom":
		page.Eval(`() => window.scrollTo(0, document.body.scrollHeight)`)
	case "top":
		page.Eval(`() => window.scrollTo(0, 0)`)
	default:
		page.Eval(`() => window.scrollBy(0, window.innerHeight)`)
	}

	_ = page.Timeout(3 * time.Second).WaitStable(300 * time.Millisecond)
	return getPageSnapshot(page), nil
}

// ══════════════════════════════════════════════════════════════════
// web_back — go back in browser history
// ══════════════════════════════════════════════════════════════════

type WebBackTool struct{}

func (w *WebBackTool) Name() string        { return "web_back" }
func (w *WebBackTool) Description() string { return "Go back to the previous page in browser history." }
func (w *WebBackTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebBackTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	page.NavigateBack()
	_ = page.Timeout(5 * time.Second).WaitStable(300 * time.Millisecond)
	return getPageSnapshot(page), nil
}

// ══════════════════════════════════════════════════════════════════
// web_eval — run JavaScript on the current page
// ══════════════════════════════════════════════════════════════════

type WebEvalTool struct{}

func (w *WebEvalTool) Name() string        { return "web_eval" }
func (w *WebEvalTool) Description() string {
	return "Execute JavaScript code on the current page and return the result. Powerful for custom extraction or DOM manipulation."
}
func (w *WebEvalTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": prop("string", "JavaScript code to execute (wrapped in arrow function)"),
		},
		"required": []string{"code"},
	}
}

func (w *WebEvalTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	code := argStr(args, "code")
	if code == "" {
		return "", fmt.Errorf("missing required argument: code")
	}

	// Wrap in arrow function if not already.
	if !strings.HasPrefix(strings.TrimSpace(code), "()") {
		code = "() => { " + code + " }"
	}

	result, err := page.Timeout(10 * time.Second).Eval(code)
	if err != nil {
		return "", fmt.Errorf("eval error: %w", err)
	}

	if result == nil {
		return "undefined", nil
	}

	// Try to return as JSON, fall back to string.
	raw, err := json.MarshalIndent(result.Value, "", "  ")
	if err != nil {
		return result.Value.Str(), nil
	}
	return string(raw), nil
}

// ══════════════════════════════════════════════════════════════════
// web_wait — wait for an element or a duration
// ══════════════════════════════════════════════════════════════════

type WebWaitTool struct{}

func (w *WebWaitTool) Name() string        { return "web_wait" }
func (w *WebWaitTool) Description() string {
	return "Wait for a condition: an element to appear (by @ref or CSS), or a number of seconds. Use for 2FA flows or slow-loading pages."
}
func (w *WebWaitTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref":     prop("string", "Optional: wait for this element to appear (@e ref or CSS selector)"),
			"seconds": propNum("Optional: wait this many seconds (default: 5, max: 60)"),
		},
	}
}

func (w *WebWaitTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	ref := argStr(args, "ref")
	seconds := argFloat(args, "seconds")

	if ref != "" {
		selector := resolveRef(ref)
		timeout := 10 * time.Second
		if seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
		_, err := page.Timeout(timeout).Element(selector)
		if err != nil {
			return fmt.Sprintf("Element %q did not appear within timeout", ref), nil
		}
		return fmt.Sprintf("Element %q is now visible", ref), nil
	}

	// Just wait N seconds.
	if seconds <= 0 {
		seconds = 5
	}
	if seconds > 60 {
		seconds = 60
	}
	time.Sleep(time.Duration(seconds) * time.Second)
	return fmt.Sprintf("Waited %.0f seconds. Page state:\n\n%s", seconds, getPageSnapshot(page)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_screenshot — capture current page (session-based)
// ══════════════════════════════════════════════════════════════════

type WebScreenshotTool struct {
	DataDir string
}

func (w *WebScreenshotTool) Name() string        { return "web_screenshot" }
func (w *WebScreenshotTool) Description() string {
	return "Take a screenshot of the current page. If no session, navigates to the given URL first."
}
func (w *WebScreenshotTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url":      prop("string", "Optional URL to navigate to first (uses current page if omitted)"),
			"filename": prop("string", "Optional filename (default: auto-generated)"),
		},
	}
}

func (w *WebScreenshotTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url := argStr(args, "url")
	var page *rod.Page

	if url != "" {
		url = ensureScheme(url)
		var err error
		page, err = globalSession.Navigate(url)
		if err != nil {
			return "", err
		}
	} else {
		page = globalSession.Page()
		if page == nil {
			return "", fmt.Errorf("no browser session and no URL — use web_navigate first or provide a url")
		}
	}

	screenshotDir := filepath.Join(w.DataDir, "screenshots")
	os.MkdirAll(screenshotDir, 0o755)

	filename := argStr(args, "filename")
	if filename == "" {
		filename = fmt.Sprintf("screenshot_%d.png", time.Now().Unix())
	}
	if !strings.HasSuffix(filename, ".png") {
		filename += ".png"
	}
	outPath := filepath.Join(screenshotDir, filename)

	screenshot, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
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

// ══════════════════════════════════════════════════════════════════
// web_extract — CSS selector extraction (session-based)
// ══════════════════════════════════════════════════════════════════

type WebExtractTool struct{}

func (w *WebExtractTool) Name() string        { return "web_extract" }
func (w *WebExtractTool) Description() string {
	return "Extract text from elements on the current page using a CSS selector."
}
func (w *WebExtractTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"selector": prop("string", "CSS selector to match elements (e.g. 'h1', '.title', '#main p')"),
		},
		"required": []string{"selector"},
	}
}

func (w *WebExtractTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	selector := argStr(args, "selector")
	if selector == "" {
		return "", fmt.Errorf("missing required argument: selector")
	}

	elements, err := page.Elements(selector)
	if err != nil || len(elements) == 0 {
		return fmt.Sprintf("No elements found matching '%s'", selector), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d element(s) matching '%s':\n\n", len(elements), selector))

	for i, el := range elements {
		if i >= 20 {
			result.WriteString(fmt.Sprintf("\n... and %d more", len(elements)-20))
			break
		}
		text, _ := el.Text()
		text = strings.TrimSpace(text)
		if text != "" {
			result.WriteString(fmt.Sprintf("[%d] %s\n", i+1, text))
		}
	}

	return result.String(), nil
}

// ══════════════════════════════════════════════════════════════════
// web_pdf — save page as PDF
// ══════════════════════════════════════════════════════════════════

type WebPDFTool struct {
	DataDir string
}

func (w *WebPDFTool) Name() string        { return "web_pdf" }
func (w *WebPDFTool) Description() string { return "Save the current page as a PDF file." }
func (w *WebPDFTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url":      prop("string", "Optional URL to navigate to first"),
			"filename": prop("string", "Optional filename (default: auto-generated)"),
		},
	}
}

func (w *WebPDFTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url := argStr(args, "url")
	var page *rod.Page

	if url != "" {
		url = ensureScheme(url)
		var err error
		page, err = globalSession.Navigate(url)
		if err != nil {
			return "", err
		}
	} else {
		page = globalSession.Page()
		if page == nil {
			return "", fmt.Errorf("no browser session — use web_navigate first or provide a url")
		}
	}

	pdfDir := filepath.Join(w.DataDir, "pdfs")
	os.MkdirAll(pdfDir, 0o755)

	filename := argStr(args, "filename")
	if filename == "" {
		filename = fmt.Sprintf("page_%d.pdf", time.Now().Unix())
	}
	if !strings.HasSuffix(filename, ".pdf") {
		filename += ".pdf"
	}
	outPath := filepath.Join(pdfDir, filename)

	pdf, err := page.PDF(&proto.PagePrintToPDF{})
	if err != nil {
		return "", fmt.Errorf("generate PDF: %w", err)
	}

	reader := pdf
	data := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", fmt.Errorf("save PDF: %w", err)
	}

	absPath, _ := filepath.Abs(outPath)
	return fmt.Sprintf("PDF saved: %s", absPath), nil
}

// ══════════════════════════════════════════════════════════════════
// web_search — Google search, return results
// ══════════════════════════════════════════════════════════════════

type WebSearchTool struct{}

func (w *WebSearchTool) Name() string        { return "web_search" }
func (w *WebSearchTool) Description() string {
	return "Search Google and return top results with titles, URLs, and snippets. No API key needed."
}
func (w *WebSearchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": prop("string", "Search query"),
		},
		"required": []string{"query"},
	}
}

func (w *WebSearchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query := argStr(args, "query")
	if query == "" {
		return "", fmt.Errorf("missing required argument: query")
	}

	searchURL := fmt.Sprintf("https://www.google.com/search?q=%s&hl=en", strings.ReplaceAll(query, " ", "+"))

	page, err := globalSession.Navigate(searchURL)
	if err != nil {
		return "", err
	}

	// Extract search results via JS.
	resultsJS := `() => {
		const results = [];
		const items = document.querySelectorAll('div.g, div[data-sokoban-container]');
		let count = 0;
		for (const item of items) {
			if (count >= 10) break;
			const titleEl = item.querySelector('h3');
			const linkEl = item.querySelector('a');
			const snippetEl = item.querySelector('.VwiC3b, [data-sncf], .st');
			if (titleEl && linkEl) {
				const title = titleEl.innerText || '';
				const url = linkEl.href || '';
				const snippet = snippetEl ? snippetEl.innerText : '';
				if (title && url) {
					results.push('[' + (count+1) + '] ' + title + '\n    ' + url + '\n    ' + snippet);
					count++;
				}
			}
		}
		return results.length > 0 ? results.join('\n\n') : 'No results found';
	}`

	result, _ := page.Eval(resultsJS)
	if result != nil {
		return fmt.Sprintf("Google results for %q:\n\n%s", query, result.Value.Str()), nil
	}

	return "No results found", nil
}

// ══════════════════════════════════════════════════════════════════
// web_close — explicitly close the browser session
// ══════════════════════════════════════════════════════════════════

type WebCloseTool struct{}

func (w *WebCloseTool) Name() string        { return "web_close" }
func (w *WebCloseTool) Description() string { return "Close the browser session. Use when browsing is complete." }
func (w *WebCloseTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebCloseTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if !globalSession.IsActive() {
		return "No active browser session", nil
	}
	globalSession.Close()
	return "Browser session closed", nil
}

// ══════════════════════════════════════════════════════════════════
// web_snapshot — re-read current page state (no navigation)
// ══════════════════════════════════════════════════════════════════

type WebSnapshotTool struct{}

func (w *WebSnapshotTool) Name() string        { return "web_snapshot" }
func (w *WebSnapshotTool) Description() string {
	return "Get a fresh snapshot of the current page state (title, text, interactive elements). Use after waiting or when you need to re-read the page."
}
func (w *WebSnapshotTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebSnapshotTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}
	return getPageSnapshot(page), nil
}
// ══════════════════════════════════════════════════════════════════
// web_cookies — manage cookies for persistent auth
// ══════════════════════════════════════════════════════════════════

type WebCookiesTool struct{}

func (w *WebCookiesTool) Name() string        { return "web_cookies" }
func (w *WebCookiesTool) Description() string {
	return "Manage browser cookies. Actions: 'save' persists to disk, 'load' restores from disk, 'list' shows current cookies, 'clear' deletes all. Saved cookies survive browser restarts for persistent login sessions."
}
func (w *WebCookiesTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": prop("string", "Action: save, load, list, or clear"),
		},
		"required": []string{"action"},
	}
}

func (w *WebCookiesTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	action := strings.ToLower(argStr(args, "action"))

	switch action {
	case "save":
		if err := globalSession.SaveCookies(); err != nil {
			return "", fmt.Errorf("save cookies: %w", err)
		}
		return "Cookies saved to disk. They will persist across browser restarts.", nil

	case "load":
		// Close and relaunch to reload cookies from profile + saved file.
		if globalSession.IsActive() {
			globalSession.Close()
		}
		_, _, err := globalSession.GetSession()
		if err != nil {
			return "", err
		}
		return "Cookies reloaded from disk.", nil

	case "list":
		if !globalSession.IsActive() {
			return "No active session. Navigate first.", nil
		}
		page := globalSession.Page()
		result, _ := page.Eval(`() => document.cookie`)
		if result != nil && result.Value.Str() != "" {
			return fmt.Sprintf("Current cookies:\n%s", result.Value.Str()), nil
		}
		return "No cookies set on current page (or they are HttpOnly).", nil

	case "clear":
		if !globalSession.IsActive() {
			return "No active session.", nil
		}
		page := globalSession.Page()
		page.Eval(`() => {
			document.cookie.split(";").forEach(c => {
				document.cookie = c.replace(/^ +/, "").replace(/=.*/, "=;expires=" + new Date().toUTCString() + ";path=/");
			});
		}`)
		return "Cookies cleared.", nil

	default:
		return "", fmt.Errorf("unknown action %q — use save, load, list, or clear", action)
	}
}

// ══════════════════════════════════════════════════════════════════
// web_headed — toggle visible browser for 2FA
// ══════════════════════════════════════════════════════════════════

type WebHeadedTool struct{}

func (w *WebHeadedTool) Name() string        { return "web_headed" }
func (w *WebHeadedTool) Description() string {
	return "Toggle headed mode (visible browser window). Use 'on' when user needs to see the browser for 2FA, CAPTCHAs, or manual steps. Use 'off' to go back to headless. This restarts the browser session."
}
func (w *WebHeadedTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mode": prop("string", "'on' for visible browser, 'off' for headless"),
		},
		"required": []string{"mode"},
	}
}

func (w *WebHeadedTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	mode := strings.ToLower(argStr(args, "mode"))
	switch mode {
	case "on", "true", "yes", "headed":
		globalSession.SetHeaded(true)
		return "Browser switched to HEADED mode — a visible window will appear on next navigation. Use this for 2FA or CAPTCHA flows.", nil
	case "off", "false", "no", "headless":
		globalSession.SetHeaded(false)
		return "Browser switched to HEADLESS mode.", nil
	default:
		return "", fmt.Errorf("unknown mode %q — use 'on' or 'off'", mode)
	}
}

// ══════════════════════════════════════════════════════════════════
// web_hover — hover over an element
// ══════════════════════════════════════════════════════════════════

type WebHoverTool struct{}

func (w *WebHoverTool) Name() string        { return "web_hover" }
func (w *WebHoverTool) Description() string {
	return "Hover over an element by @e ref or CSS selector. Triggers hover menus, tooltips, and dropdown reveals."
}
func (w *WebHoverTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or CSS selector"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebHoverTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	ref := argStr(args, "ref")
	selector := resolveRef(ref)
	el, err := page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", fmt.Errorf("element %q not found: %w", ref, err)
	}

	el.Hover()
	time.Sleep(300 * time.Millisecond)

	return fmt.Sprintf("Hovering over %s\n\n%s", ref, getPageSnapshot(page)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_double_click — double-click an element
// ══════════════════════════════════════════════════════════════════

type WebDoubleClickTool struct{}

func (w *WebDoubleClickTool) Name() string        { return "web_double_click" }
func (w *WebDoubleClickTool) Description() string {
	return "Double-click an element by @e ref or CSS selector. Use for text selection, edit-in-place, or double-click actions."
}
func (w *WebDoubleClickTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or CSS selector"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebDoubleClickTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	ref := argStr(args, "ref")
	selector := resolveRef(ref)
	el, err := page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", fmt.Errorf("element %q not found: %w", ref, err)
	}

	el.Click(proto.InputMouseButtonLeft, 2)
	_ = page.Timeout(3 * time.Second).WaitStable(300 * time.Millisecond)

	return getPageSnapshot(page), nil
}

// ══════════════════════════════════════════════════════════════════
// web_focus — focus an element (without clicking)
// ══════════════════════════════════════════════════════════════════

type WebFocusTool struct{}

func (w *WebFocusTool) Name() string        { return "web_focus" }
func (w *WebFocusTool) Description() string {
	return "Focus an element without clicking. Use for form fields, contenteditable elements, or triggering focus events."
}
func (w *WebFocusTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e2 or CSS selector"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebFocusTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	ref := argStr(args, "ref")
	selector := resolveRef(ref)
	el, err := page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", fmt.Errorf("element %q not found: %w", ref, err)
	}

	el.Focus()
	return fmt.Sprintf("Focused %s", ref), nil
}

// ══════════════════════════════════════════════════════════════════
// web_keys — send keyboard keys (Enter, Tab, Escape, shortcuts)
// ══════════════════════════════════════════════════════════════════

type WebKeysTool struct{}

func (w *WebKeysTool) Name() string        { return "web_keys" }
func (w *WebKeysTool) Description() string {
	return "Send keyboard keys to the page. Supports: Enter, Tab, Escape, Backspace, Delete, ArrowUp/Down/Left/Right, Home, End, PageUp, PageDown, Space, Ctrl+A, Ctrl+C, Ctrl+V."
}
func (w *WebKeysTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"keys": prop("string", "Key(s) to send, e.g. 'Enter', 'Tab', 'Escape', 'Ctrl+A'"),
		},
		"required": []string{"keys"},
	}
}

func (w *WebKeysTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	page := globalSession.Page()
	if page == nil {
		return "", fmt.Errorf("no browser session — use web_navigate first")
	}

	keys := argStr(args, "keys")
	if keys == "" {
		return "", fmt.Errorf("missing required argument: keys")
	}

	keyMap := map[string]input.Key{
		"enter":      input.Enter,
		"tab":        input.Tab,
		"escape":     input.Escape,
		"backspace":  input.Backspace,
		"delete":     input.Delete,
		"arrowup":    input.ArrowUp,
		"arrowdown":  input.ArrowDown,
		"arrowleft":  input.ArrowLeft,
		"arrowright": input.ArrowRight,
		"home":       input.Home,
		"end":        input.End,
		"pageup":     input.PageUp,
		"pagedown":   input.PageDown,
		"space":      input.Space,
	}

	lower := strings.ToLower(strings.TrimSpace(keys))

	// Handle Ctrl+ combos.
	if strings.HasPrefix(lower, "ctrl+") {
		char := strings.TrimPrefix(lower, "ctrl+")
		page.KeyActions().Press(input.ControlLeft).Type(input.Key(strings.ToUpper(char)[0])).Release(input.ControlLeft).MustDo()
		return fmt.Sprintf("Sent Ctrl+%s", strings.ToUpper(char)), nil
	}

	if k, ok := keyMap[lower]; ok {
		page.Keyboard.MustType(k)
		_ = page.Timeout(2 * time.Second).WaitStable(200 * time.Millisecond)
		return fmt.Sprintf("Sent key: %s", keys), nil
	}

	// If not a special key, type as text.
	page.Keyboard.MustType([]input.Key(keys)...)
	return fmt.Sprintf("Typed: %s", keys), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_new — open a new tab
// ══════════════════════════════════════════════════════════════════

type WebTabNewTool struct{}

func (w *WebTabNewTool) Name() string        { return "web_tab_new" }
func (w *WebTabNewTool) Description() string {
	return "Open a new browser tab, optionally navigating to a URL. Returns the tab index."
}
func (w *WebTabNewTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": prop("string", "Optional URL to open in the new tab"),
		},
	}
}

func (w *WebTabNewTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url := argStr(args, "url")
	if url != "" {
		url = ensureScheme(url)
	}

	idx, page, err := globalSession.NewTab(url)
	if err != nil {
		return "", err
	}

	result := fmt.Sprintf("Opened new tab [%d]", idx)
	if url != "" {
		result += fmt.Sprintf(" → %s\n\n%s", url, getPageSnapshot(page))
	}
	return result, nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_switch — switch to a tab by index
// ══════════════════════════════════════════════════════════════════

type WebTabSwitchTool struct{}

func (w *WebTabSwitchTool) Name() string        { return "web_tab_switch" }
func (w *WebTabSwitchTool) Description() string {
	return "Switch to a different browser tab by index. Use web_tab_list to see available tabs."
}
func (w *WebTabSwitchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"index": propNum("Tab index to switch to (0-based)"),
		},
		"required": []string{"index"},
	}
}

func (w *WebTabSwitchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	idx := int(argFloat(args, "index"))

	page, err := globalSession.SwitchTab(idx)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Switched to tab [%d]\n\n%s", idx, getPageSnapshot(page)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_close — close a tab by index
// ══════════════════════════════════════════════════════════════════

type WebTabCloseTool struct{}

func (w *WebTabCloseTool) Name() string        { return "web_tab_close" }
func (w *WebTabCloseTool) Description() string {
	return "Close a browser tab by index. Cannot close the last tab."
}
func (w *WebTabCloseTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"index": propNum("Tab index to close (0-based)"),
		},
		"required": []string{"index"},
	}
}

func (w *WebTabCloseTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	idx := int(argFloat(args, "index"))

	if err := globalSession.CloseTab(idx); err != nil {
		return "", err
	}

	return fmt.Sprintf("Closed tab [%d]. %d tab(s) remaining. Active: [%d]",
		idx, globalSession.TabCount(), globalSession.ActiveTabIndex()), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_list — list all open tabs
// ══════════════════════════════════════════════════════════════════

type WebTabListTool struct{}

func (w *WebTabListTool) Name() string        { return "web_tab_list" }
func (w *WebTabListTool) Description() string { return "List all open browser tabs with their titles and URLs." }
func (w *WebTabListTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebTabListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	tabs := globalSession.ListTabs()
	if len(tabs) == 0 {
		return "No browser session. Use web_navigate first.", nil
	}

	return fmt.Sprintf("Open tabs (%d):\n%s", len(tabs), strings.Join(tabs, "\n")), nil
}

// ══════════════════════════════════════════════════════════════════
// Helpers
// ══════════════════════════════════════════════════════════════════

func ensureScheme(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "https://" + url
	}
	return url
}

func argStr(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

func argFloat(args map[string]interface{}, key string) float64 {
	v, _ := args[key].(float64)
	return v
}

func prop(t, desc string) map[string]interface{} {
	return map[string]interface{}{"type": t, "description": desc}
}

func propBool(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": desc}
}

func propNum(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "number", "description": desc}
}
