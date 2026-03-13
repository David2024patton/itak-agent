---
name: agent-browser
description: Browser automation CLI for AI agents using vercel-labs/agent-browser. Use when you need to automate web browsers headlessly - navigating pages, clicking elements, filling forms, taking screenshots, extracting text, running JS, managing sessions, diffing snapshots, or testing web apps. Triggers on any browser automation task that benefits from CLI-driven Playwright with AI-optimized snapshot refs. Do NOT use for simple URL fetching (use read_url_content instead) or when the built-in browser_subagent tool is sufficient.
---

# Agent Browser Skill

CLI-based headless browser automation built on Playwright, optimized for AI agents with a "Snapshot + Refs" workflow that minimizes token usage.

**Repo:** [vercel-labs/agent-browser](https://github.com/vercel-labs/agent-browser)

## Installation

```bash
# Global install (recommended, uses native Rust binary)
npm install -g agent-browser
agent-browser install   # Download Chromium

# Quick start (no install)
npx agent-browser install
npx agent-browser open example.com

# Homebrew (macOS)
brew install agent-browser
agent-browser install
```

## Headed Mode (Visible Browser)

By default, agent-browser runs headless. Use `--headed` to launch a visible browser window:

```bash
# Launch visible browser for debugging/verification
agent-browser --headed open example.com

# Headed mode with all commands
agent-browser --headed open example.com
agent-browser --headed snapshot -i --json
agent-browser --headed click @e2
agent-browser --headed screenshot ./debug.png
```

**When to use `--headed`:**
- Debugging automation flows visually
- User-visible browser sessions (demos, presentations)
- Verifying visual rendering and layout
- Troubleshooting element interactions that fail headless
- Running on displays where the user wants to watch the automation

**Important:** `--headed` must come before the command (e.g., `--headed open`, not `open --headed`).

## Core Workflow (Snapshot + Refs)

The optimal AI workflow uses snapshots to discover refs, then refs to interact.
Add `--headed` to any command below to see the browser window.

```bash
# 1. Navigate and get snapshot (add --headed for visible browser)
agent-browser open example.com
agent-browser snapshot -i --json    # Interactive elements only, JSON output

# Output includes refs:
# - heading "Example Domain" [ref=e1] [level=1]
# - button "Submit" [ref=e2]
# - textbox "Email" [ref=e3]
# - link "Learn more" [ref=e4]

# 2. Use refs to interact
agent-browser click @e2                       # Click button
agent-browser fill @e3 "test@example.com"     # Fill input
agent-browser get text @e1                    # Get text
agent-browser hover @e4                       # Hover link

# 3. Re-snapshot after page changes
agent-browser snapshot -i --json
```

**Why refs?**
- Deterministic: Ref points to exact element from snapshot
- Fast: No DOM re-query needed
- AI-friendly: Snapshot + ref is the optimal LLM workflow

## Commands Reference

### Core Commands
```bash
agent-browser open <url>              # Navigate (aliases: goto, navigate)
agent-browser click <sel>             # Click element
agent-browser dblclick <sel>          # Double-click
agent-browser fill <sel> <text>       # Clear and fill
agent-browser type <sel> <text>       # Type into element
agent-browser press <key>             # Press key (Enter, Tab, Control+a)
agent-browser keyboard type <text>    # Type with real keystrokes (current focus)
agent-browser hover <sel>             # Hover element
agent-browser select <sel> <val>      # Select dropdown option
agent-browser check <sel>             # Check checkbox
agent-browser uncheck <sel>           # Uncheck checkbox
agent-browser scroll <dir> [px]       # Scroll (up/down/left/right)
agent-browser drag <src> <tgt>        # Drag and drop
agent-browser upload <sel> <files>    # Upload files
agent-browser screenshot [path]       # Screenshot (--full for full page)
agent-browser screenshot --annotate   # Annotated screenshot with numbered labels
agent-browser pdf <path>              # Save as PDF
agent-browser snapshot                # Accessibility tree with refs
agent-browser eval <js>               # Run JavaScript
agent-browser close                   # Close browser
```

### Get Info
```bash
agent-browser get text <sel>          # Get text content
agent-browser get html <sel>          # Get innerHTML
agent-browser get value <sel>         # Get input value
agent-browser get attr <sel> <attr>   # Get attribute
agent-browser get title               # Get page title
agent-browser get url                 # Get current URL
agent-browser get count <sel>         # Count matching elements
agent-browser get box <sel>           # Get bounding box
agent-browser get styles <sel>        # Get computed styles
```

### Check State
```bash
agent-browser is visible <sel>        # Check if visible
agent-browser is enabled <sel>        # Check if enabled
agent-browser is checked <sel>        # Check if checked
```

### Find Elements (Semantic Locators)
```bash
agent-browser find role button click --name "Submit"
agent-browser find text "Sign In" click
agent-browser find label "Email" fill "test@test.com"
agent-browser find first ".item" click
agent-browser find nth 2 "a" text
```

Actions: click, fill, type, hover, focus, check, uncheck, text

### Wait
```bash
agent-browser wait <selector>                 # Wait for element
agent-browser wait <ms>                       # Wait for time
agent-browser wait --text "Welcome"           # Wait for text
agent-browser wait --url "**/dash"            # Wait for URL pattern
agent-browser wait --load networkidle         # Wait for load state
agent-browser wait --fn "window.ready === true" # Wait for JS condition
```

### Navigation
```bash
agent-browser back                    # Go back
agent-browser forward                 # Go forward
agent-browser reload                  # Reload page
```

### Tabs and Windows
```bash
agent-browser tab                     # List tabs
agent-browser tab new [url]           # New tab
agent-browser tab <n>                 # Switch to tab n
agent-browser tab close [n]           # Close tab
agent-browser window new              # New window
```

### Frames
```bash
agent-browser frame <sel>             # Switch to iframe
agent-browser frame main              # Back to main frame
```

### Browser Settings
```bash
agent-browser set viewport <w> <h>    # Set viewport size
agent-browser set device <name>       # Emulate device ("iPhone 14")
agent-browser set geo <lat> <lng>     # Set geolocation
agent-browser set offline [on|off]    # Toggle offline mode
agent-browser set media [dark|light]  # Emulate color scheme
```

### Cookies and Storage
```bash
agent-browser cookies                 # Get all cookies
agent-browser cookies set <name> <val># Set cookie
agent-browser cookies clear           # Clear cookies
agent-browser storage local           # Get localStorage
agent-browser storage local set <k> <v> # Set value
agent-browser storage local clear     # Clear all
```

### Network
```bash
agent-browser network route <url>            # Intercept requests
agent-browser network route <url> --abort    # Block requests
agent-browser network route <url> --body <j> # Mock response
agent-browser network requests               # View tracked requests
agent-browser network requests --filter api  # Filter requests
```

### Diff (Snapshot and Visual Comparison)
```bash
agent-browser diff snapshot                           # Compare current vs last snapshot
agent-browser diff snapshot --baseline before.txt     # Compare vs saved file
agent-browser diff screenshot --baseline before.png   # Visual pixel diff
agent-browser diff url https://v1.com https://v2.com  # Compare two URLs
```

### Debug
```bash
agent-browser trace start [path]      # Start recording trace
agent-browser trace stop [path]       # Stop and save trace
agent-browser console                 # View console messages
agent-browser errors                  # View page errors
agent-browser highlight <sel>         # Highlight element
agent-browser state save <path>       # Save auth state
agent-browser state load <path>       # Load auth state
```

## Snapshot Options

```bash
agent-browser snapshot                # Full accessibility tree
agent-browser snapshot -i             # Interactive elements only
agent-browser snapshot -i -C          # Include cursor-interactive (onclick divs)
agent-browser snapshot -c             # Compact (remove empty structural elements)
agent-browser snapshot -d 3           # Limit depth to 3 levels
agent-browser snapshot -s "#main"     # Scope to CSS selector
agent-browser snapshot -i -c -d 5    # Combine options
```

## Annotated Screenshots

Overlays numbered labels on interactive elements. Each label [N] maps to ref @eN:

```bash
agent-browser screenshot --annotate
# -> Screenshot saved to /tmp/screenshot-xxx.png
# [1] @e1 button "Submit"
# [2] @e2 link "Home"
# [3] @e3 textbox "Email"

# Refs are cached, interact immediately:
agent-browser click @e2
```

Useful for multimodal AI models that reason about visual layout, icon buttons, or canvas elements.

## Sessions

```bash
# Isolated browser instances
agent-browser --session agent1 open site-a.com
agent-browser --session agent2 open site-b.com

# Persistent profiles (survive browser restarts)
agent-browser --profile ~/.myapp-profile open myapp.com

# Named session persistence (auto-save/load cookies + localStorage)
agent-browser --session-name twitter open twitter.com

# List active sessions
agent-browser session list
```

## Security Features

All opt-in:

- **Auth Vault** - Store credentials encrypted locally, LLM never sees passwords:
  ```bash
  echo "pass" | agent-browser auth save github --url https://github.com/login --username user --password-stdin
  agent-browser auth login github
  ```
- **Domain Allowlist** - Restrict navigation: `--allowed-domains "example.com,*.example.com"`
- **Action Policy** - Gate destructive actions: `--action-policy ./policy.json`
- **Action Confirmation** - Require approval: `--confirm-actions eval,download`
- **Output Limits** - Prevent context flooding: `--max-output 50000`
- **Content Boundaries** - Wrap output in delimiters: `--content-boundaries`

## Command Chaining

Commands can be chained with `&&`. The browser persists via a background daemon:

```bash
# Open, wait, and snapshot in one call
agent-browser open example.com && agent-browser wait --load networkidle && agent-browser snapshot -i

# Chain interactions
agent-browser fill @e1 "user@example.com" && agent-browser fill @e2 "pass" && agent-browser click @e3
```

Use `&&` when you don't need intermediate output. Run separately when you need to parse output first (e.g., snapshot to discover refs).

## Selectors Priority

1. **Refs** (`@e1`, `@e2`) - Best for AI, deterministic from snapshot
2. **CSS** (`"#id"`, `".class"`, `"div > button"`) - When you know the DOM structure
3. **Text/XPath** (`"text=Submit"`, `"xpath=//button"`) - Fallback
4. **Semantic** (`find role button click --name "Submit"`) - Accessibility-first

## Agent Mode (JSON Output)

```bash
agent-browser snapshot --json
# Returns: {"success":true,"data":{"snapshot":"...","refs":{"e1":{"role":"heading","name":"Title"},...}}}

agent-browser get text @e1 --json
agent-browser is visible @e2 --json
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `AGENT_BROWSER_SESSION` | Default session name |
| `AGENT_BROWSER_PROFILE` | Persistent profile directory |
| `AGENT_BROWSER_SESSION_NAME` | Auto-persist session name |
| `AGENT_BROWSER_ALLOWED_DOMAINS` | Domain allowlist |
| `AGENT_BROWSER_MAX_OUTPUT` | Output character limit |
| `AGENT_BROWSER_CONTENT_BOUNDARIES` | Enable content boundary markers |
| `AGENT_BROWSER_ENCRYPTION_KEY` | Custom encryption key for auth vault |
