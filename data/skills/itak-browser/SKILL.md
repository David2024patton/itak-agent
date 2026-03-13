---
name: itak-browser
description: Native automation using the iTaK Browser CLI (gobrowser) for high-performance, low-token browser interaction via Snapshot+Refs. Use this skill when you need to automate a browser, extract page data, interact with web elements, or test local apps without Playwright.
---

# iTaK Browser (gobrowser)

The iTaK Browser is a Go-native CLI (`gobrowser.exe`) located at `e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe`. It uses a persistent background daemon for sub-second responses and features **Snapshot+Refs** which reduces DOM token usage by 93%.

## Core Workflow

Always use the `run_command` tool to execute `gobrowser.exe`. When using PowerShell, ensure you capture the output.

### 1. Ensure Daemon is Running
The daemon keeps Chrome alive between commands, eliminating cold starts.
```powershell
$proc = Get-Process -Name "gobrowser" -ErrorAction SilentlyContinue
if (!$proc) {
    Start-Process -FilePath "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" -ArgumentList "daemon","start" -WindowStyle Hidden
    Start-Sleep -Seconds 2
}
```

### 2. Create a Session
Create a new browsing session. Use `--headed` if you want to see the browser visibly (e.g., to use the annotation toolbar), and `--stealth` to avoid bot detection.
```powershell
$raw = & "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" session new --headed --stealth 2>&1
$sid = ($raw | Select-String -Pattern "ses_\d+").Matches[0].Value
Write-Output "Session ID: $sid"
```

### 3. Browse and Interact
Pass the `-s <session_id>` flag to every command.

```powershell
# Search the web using the built-in SearXNG aggregator (Bypasses Google Bot Detection)
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" search "what is golang" -s $sid

# Navigate directly to a specific URL
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" open "https://example.com" -s $sid

# Take an accessibility snapshot (returns lightweight Ref IDs like 'e1', 'a5')
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" snapshot -s $sid

# Interact with elements using Ref IDs from the snapshot
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" click e2 -s $sid
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" fill e3 "hello world" -s $sid

# Wait for navigation to complete after a click
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" wait-nav -s $sid
```

### 4. Page Intelligence and Data Extraction
```powershell
# Extract all links on the page (JSON output)
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" links -s $sid

# Extract all forms and fields
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" forms -s $sid

# Page metrics (DOM depth, load time, JS heap)
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" metrics -s $sid
```

### 5. Utilities
```powershell
# Take a screenshot (saved to disk, returns path)
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" screenshot -s $sid

# Run the comprehensive debug bundle (snapshot + screenshot + network + console + threat scan)
& "e:\.agent\iTaK Eco\Browser\dist\gobrowser.exe" --json debug -s $sid
```
