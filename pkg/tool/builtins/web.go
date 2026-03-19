package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════
// Helper: format gobrowser JSON response for agent consumption
// ══════════════════════════════════════════════════════════════════

// gobrowserResult extracts a human-readable string from gobrowser JSON output.
// gobrowser responses have the structure:
//
//	{"data": {"snapshot": {"tree": "...", "refs": {...}, "url": "..."}}, "ok": true}
//	{"data": {"scan_result": {...}}, "ok": true}
//	{"data": "plain text result", "ok": true}
func gobrowserResult(out string) string {
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return strings.TrimSpace(out)
	}

	// If there's an error field, return it.
	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		return "Error: " + errMsg
	}

	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		// data might be a plain string.
		if s, ok := resp["data"].(string); ok {
			return s
		}
		return strings.TrimSpace(out)
	}

	var sb strings.Builder

	// Handle snapshot nested under data.snapshot (most common for snapshot/open responses).
	if snap, ok := data["snapshot"].(map[string]interface{}); ok {
		if url, ok := snap["url"].(string); ok {
			sb.WriteString(fmt.Sprintf("URL: %s\n", url))
		}
		if title, ok := snap["title"].(string); ok {
			sb.WriteString(fmt.Sprintf("Title: %s\n", title))
		}
		if tree, ok := snap["tree"].(string); ok && tree != "" {
			sb.WriteString("\n")
			sb.WriteString(tree)
		}
	}

	// Handle direct fields (some commands put url/title/tree at the data level).
	if sb.Len() == 0 {
		if url, ok := data["url"].(string); ok {
			sb.WriteString(fmt.Sprintf("URL: %s\n", url))
		}
		if title, ok := data["title"].(string); ok {
			sb.WriteString(fmt.Sprintf("Title: %s\n", title))
		}
		if tree, ok := data["tree"].(string); ok && tree != "" {
			sb.WriteString("\n")
			sb.WriteString(tree)
		}
	}

	// Handle text/result fields (from eval, extract, etc).
	if text, ok := data["text"].(string); ok && text != "" {
		sb.WriteString(text)
	}
	if result, ok := data["result"]; ok {
		switch r := result.(type) {
		case string:
			sb.WriteString(r)
		default:
			raw, _ := json.MarshalIndent(r, "", "  ")
			sb.WriteString(string(raw))
		}
	}

	// Handle file paths.
	if path, ok := data["path"].(string); ok && path != "" {
		sb.WriteString(fmt.Sprintf("Saved: %s\n", path))
	}
	if screenshot, ok := data["screenshot_path"].(string); ok && screenshot != "" {
		sb.WriteString(fmt.Sprintf("Screenshot: %s\n", screenshot))
	}

	result := sb.String()
	if result == "" {
		// Fallback: marshal the whole data map.
		raw, _ := json.MarshalIndent(data, "", "  ")
		return string(raw)
	}
	return result
}

// ══════════════════════════════════════════════════════════════════
// web_navigate  -  go to a URL via gobrowser
// ══════════════════════════════════════════════════════════════════

type WebNavigateTool struct{}

func (w *WebNavigateTool) Name() string { return "web_navigate" }
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

	_, err := globalSession.Run("open", url)
	if err != nil {
		return "", fmt.Errorf("navigate to %s: %w", url, err)
	}

	// Wait for JavaScript rendering before taking a snapshot.
	time.Sleep(2 * time.Second)

	snapOut, snapErr := globalSession.Run("snapshot")
	if snapErr != nil {
		return fmt.Sprintf("Navigated to %s (snapshot unavailable: %v)", url, snapErr), nil
	}
	return gobrowserResult(snapOut), nil
}

// ══════════════════════════════════════════════════════════════════
// web_click  -  click an element by @ref
// ══════════════════════════════════════════════════════════════════

type WebClickTool struct{}

func (w *WebClickTool) Name() string { return "web_click" }
func (w *WebClickTool) Description() string {
	return "Click an element on the current page using an @e ref (e.g. @e3 or just e3). Returns the page state after clicking."
}
func (w *WebClickTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or e3"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebClickTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := normalizeRef(argStr(args, "ref"))
	if ref == "" {
		return "", fmt.Errorf("missing required argument: ref")
	}

	out, err := globalSession.Run("click", ref)
	if err != nil {
		return "", fmt.Errorf("click %s: %w", ref, err)
	}

	snapOut, _ := globalSession.Run("snapshot")
	if snapOut != "" {
		return gobrowserResult(snapOut), nil
	}
	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_type  -  type text into an input
// ══════════════════════════════════════════════════════════════════

type WebTypeTool struct{}

func (w *WebTypeTool) Name() string { return "web_type" }
func (w *WebTypeTool) Description() string {
	return "Type text into an input field using an @e ref (e.g. @e2 or e2). Optionally press Enter after typing."
}
func (w *WebTypeTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref":         prop("string", "Element ref like @e2 or e2"),
			"text":        prop("string", "Text to type into the element"),
			"press_enter": propBool("Press Enter after typing (default: false)"),
		},
		"required": []string{"ref", "text"},
	}
}

func (w *WebTypeTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := normalizeRef(argStr(args, "ref"))
	text := argStr(args, "text")
	if ref == "" || text == "" {
		return "", fmt.Errorf("missing required arguments: ref and text")
	}

	out, err := globalSession.Run("fill", ref, text)
	if err != nil {
		return "", fmt.Errorf("type into %s: %w", ref, err)
	}

	pressEnter, _ := args["press_enter"].(bool)
	if pressEnter {
		globalSession.Run("press", "Enter")
	}

	return fmt.Sprintf("Typed %q into %s\n%s", text, ref, gobrowserResult(out)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_scroll  -  scroll the page
// ══════════════════════════════════════════════════════════════════

type WebScrollTool struct{}

func (w *WebScrollTool) Name() string { return "web_scroll" }
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
	dir := strings.ToLower(argStr(args, "direction"))

	// Map directions to scroll deltas.
	dy := "300"
	switch dir {
	case "up":
		dy = "-300"
	case "bottom":
		dy = "99999"
	case "top":
		dy = "-99999"
	}

	out, err := globalSession.Run("scroll", "--y", dy)
	if err != nil {
		return "", fmt.Errorf("scroll %s: %w", dir, err)
	}

	snapOut, _ := globalSession.Run("snapshot")
	if snapOut != "" {
		return gobrowserResult(snapOut), nil
	}
	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_back  -  go back in browser history
// ══════════════════════════════════════════════════════════════════

type WebBackTool struct{}

func (w *WebBackTool) Name() string        { return "web_back" }
func (w *WebBackTool) Description() string { return "Go back to the previous page in browser history." }
func (w *WebBackTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebBackTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	_, err := globalSession.Run("nav", "back")
	if err != nil {
		return "", fmt.Errorf("nav back: %w", err)
	}

	snapOut, _ := globalSession.Run("snapshot")
	return gobrowserResult(snapOut), nil
}

// ══════════════════════════════════════════════════════════════════
// web_eval  -  run JavaScript on the current page
// ══════════════════════════════════════════════════════════════════

type WebEvalTool struct{}

func (w *WebEvalTool) Name() string { return "web_eval" }
func (w *WebEvalTool) Description() string {
	return "Execute JavaScript code on the current page and return the result."
}
func (w *WebEvalTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": prop("string", "JavaScript code to execute"),
		},
		"required": []string{"code"},
	}
}

func (w *WebEvalTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	code := argStr(args, "code")
	if code == "" {
		return "", fmt.Errorf("missing required argument: code")
	}

	out, err := globalSession.Run("eval", code)
	if err != nil {
		return "", fmt.Errorf("eval error: %w", err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_wait  -  wait for an element or duration
// ══════════════════════════════════════════════════════════════════

type WebWaitTool struct{}

func (w *WebWaitTool) Name() string { return "web_wait" }
func (w *WebWaitTool) Description() string {
	return "Wait for a condition: an element to appear (by CSS selector), or a number of seconds."
}
func (w *WebWaitTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref":     prop("string", "Optional: CSS selector to wait for"),
			"seconds": propNum("Optional: wait this many seconds (default: 5, max: 60)"),
		},
	}
}

func (w *WebWaitTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := argStr(args, "ref")
	seconds := argFloat(args, "seconds")

	if ref != "" {
		out, err := globalSession.Run("wait", "visible", ref)
		if err != nil {
			return fmt.Sprintf("Element %q did not appear within timeout", ref), nil
		}
		return fmt.Sprintf("Element %q is now visible\n%s", ref, gobrowserResult(out)), nil
	}

	if seconds <= 0 {
		seconds = 5
	}
	if seconds > 60 {
		seconds = 60
	}
	time.Sleep(time.Duration(seconds) * time.Second)

	snapOut, _ := globalSession.Run("snapshot")
	return fmt.Sprintf("Waited %.0f seconds.\n\n%s", seconds, gobrowserResult(snapOut)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_screenshot  -  capture current page
// ══════════════════════════════════════════════════════════════════

type WebScreenshotTool struct {
	DataDir string
}

func (w *WebScreenshotTool) Name() string { return "web_screenshot" }
func (w *WebScreenshotTool) Description() string {
	return "Take a screenshot of the current page. If no session, navigates to the given URL first."
}
func (w *WebScreenshotTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": prop("string", "Optional URL to navigate to first"),
		},
	}
}

func (w *WebScreenshotTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url := argStr(args, "url")
	if url != "" {
		url = ensureScheme(url)
		if _, err := globalSession.Run("open", url); err != nil {
			return "", fmt.Errorf("navigate for screenshot: %w", err)
		}
	}

	out, err := globalSession.Run("screenshot")
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_extract  -  extract text from elements
// ══════════════════════════════════════════════════════════════════

type WebExtractTool struct{}

func (w *WebExtractTool) Name() string { return "web_extract" }
func (w *WebExtractTool) Description() string {
	return "Extract text from an element by @e ref. Use for reading specific content."
}
func (w *WebExtractTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or e3"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebExtractTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := normalizeRef(argStr(args, "ref"))
	if ref == "" {
		return "", fmt.Errorf("missing required argument: ref")
	}

	out, err := globalSession.Run("get", "text", ref)
	if err != nil {
		return "", fmt.Errorf("extract text from %s: %w", ref, err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_pdf  -  save page as PDF
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
			"filename": prop("string", "Output filename"),
		},
	}
}

func (w *WebPDFTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	url := argStr(args, "url")
	if url != "" {
		url = ensureScheme(url)
		if _, err := globalSession.Run("open", url); err != nil {
			return "", fmt.Errorf("navigate for pdf: %w", err)
		}
	}

	filename := argStr(args, "filename")
	if filename == "" {
		filename = fmt.Sprintf("page_%d.pdf", time.Now().Unix())
	}

	out, err := globalSession.Run("pdf", filename)
	if err != nil {
		return "", fmt.Errorf("generate PDF: %w", err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_search  -  search via SearXNG
// ══════════════════════════════════════════════════════════════════

type WebSearchTool struct{}

func (w *WebSearchTool) Name() string { return "web_search" }
func (w *WebSearchTool) Description() string {
	return "Search the web via SearXNG (self-hosted, aggregates 10+ engines). Returns results with titles, URLs, and snippets."
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

	out, err := globalSession.Run("search", query)
	if err != nil {
		return "", fmt.Errorf("search %q: %w", query, err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_close  -  close the browser session
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
// web_snapshot  -  re-read current page state
// ══════════════════════════════════════════════════════════════════

type WebSnapshotTool struct{}

func (w *WebSnapshotTool) Name() string { return "web_snapshot" }
func (w *WebSnapshotTool) Description() string {
	return "Get a fresh snapshot of the current page (accessibility tree with element refs). Use after waiting or to re-read."
}
func (w *WebSnapshotTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebSnapshotTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	out, err := globalSession.Run("snapshot")
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_cookies  -  manage cookies
// ══════════════════════════════════════════════════════════════════

type WebCookiesTool struct{}

func (w *WebCookiesTool) Name() string { return "web_cookies" }
func (w *WebCookiesTool) Description() string {
	return "Manage browser cookies. Actions: 'save' persists to disk, 'load' restores, 'list' shows current, 'clear' deletes all."
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
		out, err := globalSession.Run("cookies", "save")
		if err != nil {
			return "", fmt.Errorf("save cookies: %w", err)
		}
		return gobrowserResult(out), nil
	case "load":
		out, err := globalSession.Run("cookies", "load")
		if err != nil {
			return "", fmt.Errorf("load cookies: %w", err)
		}
		return gobrowserResult(out), nil
	case "list":
		out, err := globalSession.Run("cookies", "list")
		if err != nil {
			return "", fmt.Errorf("list cookies: %w", err)
		}
		return gobrowserResult(out), nil
	case "clear":
		out, err := globalSession.Run("cookies", "clear")
		if err != nil {
			return "", fmt.Errorf("clear cookies: %w", err)
		}
		return gobrowserResult(out), nil
	default:
		return "", fmt.Errorf("unknown action %q, use save, load, list, or clear", action)
	}
}

// ══════════════════════════════════════════════════════════════════
// web_headed  -  toggle visible browser
// ══════════════════════════════════════════════════════════════════

type WebHeadedTool struct{}

func (w *WebHeadedTool) Name() string { return "web_headed" }
func (w *WebHeadedTool) Description() string {
	return "Toggle headed mode (visible browser). Use 'on' for 2FA/CAPTCHA flows, 'off' to go headless."
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
		return "Browser switched to HEADED mode. A visible window will appear on next navigation.", nil
	case "off", "false", "no", "headless":
		globalSession.SetHeaded(false)
		return "Browser switched to HEADLESS mode.", nil
	default:
		return "", fmt.Errorf("unknown mode %q, use 'on' or 'off'", mode)
	}
}

// ══════════════════════════════════════════════════════════════════
// web_hover  -  hover over an element
// ══════════════════════════════════════════════════════════════════

type WebHoverTool struct{}

func (w *WebHoverTool) Name() string { return "web_hover" }
func (w *WebHoverTool) Description() string {
	return "Hover over an element by @e ref. Triggers hover menus, tooltips, and dropdowns."
}
func (w *WebHoverTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or e3"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebHoverTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := normalizeRef(argStr(args, "ref"))
	out, err := globalSession.Run("hover", ref)
	if err != nil {
		return "", fmt.Errorf("hover %s: %w", ref, err)
	}

	snapOut, _ := globalSession.Run("snapshot")
	if snapOut != "" {
		return fmt.Sprintf("Hovering over %s\n\n%s", ref, gobrowserResult(snapOut)), nil
	}
	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_double_click  -  double-click an element
// ══════════════════════════════════════════════════════════════════

type WebDoubleClickTool struct{}

func (w *WebDoubleClickTool) Name() string { return "web_double_click" }
func (w *WebDoubleClickTool) Description() string {
	return "Double-click an element by @e ref."
}
func (w *WebDoubleClickTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e3 or e3"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebDoubleClickTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := normalizeRef(argStr(args, "ref"))
	out, err := globalSession.Run("dblclick", ref)
	if err != nil {
		return "", fmt.Errorf("double-click %s: %w", ref, err)
	}

	snapOut, _ := globalSession.Run("snapshot")
	if snapOut != "" {
		return gobrowserResult(snapOut), nil
	}
	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_focus  -  focus an element without clicking
// ══════════════════════════════════════════════════════════════════

type WebFocusTool struct{}

func (w *WebFocusTool) Name() string { return "web_focus" }
func (w *WebFocusTool) Description() string {
	return "Focus an element without clicking. Use for form fields or triggering focus events."
}
func (w *WebFocusTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": prop("string", "Element ref like @e2 or e2"),
		},
		"required": []string{"ref"},
	}
}

func (w *WebFocusTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	ref := normalizeRef(argStr(args, "ref"))
	// gobrowser doesn't have a focus command, use click as closest equivalent.
	out, err := globalSession.Run("click", ref)
	if err != nil {
		return "", fmt.Errorf("focus %s: %w", ref, err)
	}
	return fmt.Sprintf("Focused %s\n%s", ref, gobrowserResult(out)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_keys  -  send keyboard keys
// ══════════════════════════════════════════════════════════════════

type WebKeysTool struct{}

func (w *WebKeysTool) Name() string { return "web_keys" }
func (w *WebKeysTool) Description() string {
	return "Send keyboard keys: Enter, Tab, Escape, Backspace, ArrowUp/Down, Ctrl+A, etc."
}
func (w *WebKeysTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"keys": prop("string", "Key(s) to send, e.g. 'Enter', 'Tab', 'Ctrl+A'"),
		},
		"required": []string{"keys"},
	}
}

func (w *WebKeysTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	keys := argStr(args, "keys")
	if keys == "" {
		return "", fmt.Errorf("missing required argument: keys")
	}

	out, err := globalSession.Run("press", keys)
	if err != nil {
		return "", fmt.Errorf("press %s: %w", keys, err)
	}

	return fmt.Sprintf("Sent key: %s\n%s", keys, gobrowserResult(out)), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_new  -  open a new tab
// ══════════════════════════════════════════════════════════════════

type WebTabNewTool struct{}

func (w *WebTabNewTool) Name() string { return "web_tab_new" }
func (w *WebTabNewTool) Description() string {
	return "Open a new browser tab, optionally navigating to a URL."
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
	cmdArgs := []string{"tab", "new"}
	if url != "" {
		url = ensureScheme(url)
		cmdArgs = append(cmdArgs, url)
	}

	out, err := globalSession.Run(cmdArgs...)
	if err != nil {
		return "", fmt.Errorf("new tab: %w", err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_switch  -  switch to a tab by index
// ══════════════════════════════════════════════════════════════════

type WebTabSwitchTool struct{}

func (w *WebTabSwitchTool) Name() string { return "web_tab_switch" }
func (w *WebTabSwitchTool) Description() string {
	return "Switch to a different browser tab by index."
}
func (w *WebTabSwitchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"index": propNum("Tab index (0-based)"),
		},
		"required": []string{"index"},
	}
}

func (w *WebTabSwitchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	idx := fmt.Sprintf("%d", int(argFloat(args, "index")))

	out, err := globalSession.Run("tab", "switch", idx)
	if err != nil {
		return "", fmt.Errorf("switch tab: %w", err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_close  -  close a tab by index
// ══════════════════════════════════════════════════════════════════

type WebTabCloseTool struct{}

func (w *WebTabCloseTool) Name() string        { return "web_tab_close" }
func (w *WebTabCloseTool) Description() string { return "Close a browser tab by index." }
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
	idx := fmt.Sprintf("%d", int(argFloat(args, "index")))

	out, err := globalSession.Run("tab", "close", idx)
	if err != nil {
		return "", fmt.Errorf("close tab: %w", err)
	}

	return gobrowserResult(out), nil
}

// ══════════════════════════════════════════════════════════════════
// web_tab_list  -  list all open tabs
// ══════════════════════════════════════════════════════════════════

type WebTabListTool struct{}

func (w *WebTabListTool) Name() string        { return "web_tab_list" }
func (w *WebTabListTool) Description() string { return "List all open browser tabs." }
func (w *WebTabListTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}

func (w *WebTabListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	out, err := globalSession.Run("tab", "list")
	if err != nil {
		return "", fmt.Errorf("list tabs: %w", err)
	}

	return gobrowserResult(out), nil
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

// normalizeRef strips the "@" prefix from refs like "@e3" to get "e3".
func normalizeRef(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "@")
	return ref
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
