---
name: core
description: Core vibe-browser usage guide. Read this before running any vibe-browser commands. Covers the snapshot-and-ref workflow, navigating pages, interacting with elements (click, fill, type, select), extracting text and data, taking screenshots, managing tabs, handling forms and auth, waiting for content, and troubleshooting common failures. Supports Chrome, Chromium, Brave, and Edge browsers on macOS, Linux, and Windows. Use when the user asks to interact with a website, fill a form, click something, extract data, take a screenshot, log into a site, test a web app, or automate any browser task.
allowed-tools: Bash(vibe-browser:*), Bash(npx @startvibecoding/vibe-browser:*), Bash(powershell:*), Bash(pwsh:*)
---

# vibe-browser core

Fast browser automation CLI for AI agents. Chrome/Chromium via CDP, no Playwright or Puppeteer dependency. Supports Chrome, Chromium, Brave, and Edge browsers on macOS, Linux, and Windows.

## Installation

```bash
npm install -g @startvibecoding/vibe-browser
```

## The core loop

### macOS / Linux (bash/zsh)

```bash
vibe-browser open <url>               # 1. Open a page
vibe-browser snapshot --interactive    # 2. See interactive elements
vibe-browser click "button"            # 3. Act on elements
vibe-browser snapshot --interactive    # 4. Re-snapshot after changes
```

### Windows (PowerShell)

```powershell
vibe-browser open "<url>"               # 1. Open a page
vibe-browser snapshot --interactive    # 2. See interactive elements
vibe-browser click "button"            # 3. Act on elements
vibe-browser snapshot --interactive    # 4. Re-snapshot after changes
```

## Quickstart

### macOS / Linux

```bash
# Take a screenshot of a page
vibe-browser open https://example.com
vibe-browser screenshot --output home.png

# Search, click a result, and capture it
vibe-browser open https://duckduckgo.com
vibe-browser snapshot --interactive
vibe-browser fill "input[name=q]" "vibe-browser cli"
vibe-browser press Enter
vibe-browser wait text "Results"
vibe-browser snapshot --interactive
vibe-browser click "a.result"
vibe-browser screenshot --output result.png
```

### Windows (PowerShell)

```powershell
# Take a screenshot of a page
vibe-browser open "https://example.com"
vibe-browser screenshot --output home.png

# Search, click a result, and capture it
vibe-browser open "https://duckduckgo.com"
vibe-browser snapshot --interactive
vibe-browser fill "input[name=q]" "vibe-browser cli"
vibe-browser press Enter
vibe-browser wait text "Results"
vibe-browser snapshot --interactive
vibe-browser click "a.result"
vibe-browser screenshot --output result.png
```

## Browser Discovery

```bash
# Auto-detect running browser
vibe-browser discover

# List installed browsers
vibe-browser browsers

# Use specific browser
vibe-browser open --browser brave https://example.com
vibe-browser open --browser edge https://example.com

# Connect to specific CDP endpoint
vibe-browser open --cdp-url ws://127.0.0.1:9222/devtools/browser
```

### Supported Browsers by Platform

| Browser | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Chrome | `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` | `google-chrome`, `google-chrome-stable` | `%PROGRAMFILES%\Google\Chrome\Application\chrome.exe` |
| Chromium | `/Applications/Chromium.app/Contents/MacOS/Chromium` | `chromium`, `chromium-browser` | `%LOCALAPPDATA%\Chromium\Application\chrome.exe` |
| Brave | `/Applications/Brave Browser.app/Contents/MacOS/Brave Browser` | `brave-browser` | `%LOCALAPPDATA%\BraveSoftware\Brave-Browser\Application\brave.exe` |
| Edge | `/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge` | `microsoft-edge` | `%PROGRAMFILES%\Microsoft\Edge\Application\msedge.exe` |
| Chrome Canary | `/Applications/Google Chrome Canary.app/...` | — | — |

### User Data Directories (for connecting to existing profiles)

| Browser | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Chrome | `~/Library/Application Support/Google/Chrome` | `~/.config/google-chrome` | `%LOCALAPPDATA%\Google\Chrome\User Data` |
| Chromium | `~/Library/Application Support/Chromium` | `~/.config/chromium` | `%LOCALAPPDATA%\Chromium\User Data` |
| Brave | `~/Library/Application Support/BraveSoftware/Brave-Browser` | `~/.config/BraveSoftware/Brave-Browser` | `%LOCALAPPDATA%\BraveSoftware\Brave-Browser\User Data` |
| Edge | `~/Library/Application Support/Microsoft Edge` | `~/.config/microsoft-edge` | `%LOCALAPPDATA%\Microsoft\Edge\User Data` |

## Reading a page

```bash
vibe-browser snapshot                    # full tree
vibe-browser snapshot --interactive      # interactive elements only (preferred)
vibe-browser snapshot --interactive --compact  # compact mode
```

Snapshot output looks like:

```
Page: Example - Log in
URL: https://example.com/login

[1] heading "Log in"
[2] form
  [3] textbox Email
  [4] textbox Password
  [5] button "Continue"
  [6] link "Forgot password?"
```

For reading content:

```bash
vibe-browser get text "h1"
vibe-browser get html "div.content"
vibe-browser get attr "a" href
vibe-browser get value "input"
vibe-browser get url
vibe-browser get title
```

## Interacting

```bash
vibe-browser click "button"
vibe-browser dblclick "div.item"
vibe-browser hover "a.link"
vibe-browser focus "input"
vibe-browser fill "input" --value "hello"
vibe-browser type "input" " world"
vibe-browser press Enter
vibe-browser check "input[type=checkbox]"
vibe-browser uncheck "input[type=checkbox]"
vibe-browser select "select" --value "option1"
vibe-browser scroll --x 0 --y 500
```

## Waiting

```bash
vibe-browser wait selector "div.loaded"  # until element appears
vibe-browser wait ms 2000                # dumb wait (last resort)
vibe-browser wait text "Success"         # until text appears
vibe-browser wait url "/dashboard"       # until URL matches
```

After any page-changing action, wait for:
- A specific element: `wait selector "..."`
- URL change: `wait url "/new-page"`

## Common workflows

### Log in

```bash
vibe-browser open https://app.example.com/login
vibe-browser snapshot --interactive
vibe-browser fill "input[name=email]" "user@example.com"
vibe-browser fill "input[name=password]" "hunter2"
vibe-browser click "button[type=submit]"
vibe-browser wait url "/dashboard"
```

### Extract data

```bash
vibe-browser get title
vibe-browser get text "h1"
vibe-browser get text ".price"
vibe-browser get attr "a.main-link" href
vibe-browser eval "document.querySelectorAll('table tr').length"
```

### Screenshot

```bash
vibe-browser screenshot --output page.png
vibe-browser screenshot --full-page --output full.png
```

## Flags

```
--cdp-url string        Chrome DevTools Protocol WebSocket URL
--session string        Session name (default "default")
--headless              Run in headless mode (default true)
--executable-path       Path to Chrome executable
--browser string        Browser type (chrome, chromium, brave, edge)
```

## Environment Variables

```
VIBE_BROWSER_CDP_URL    Same as --cdp-url
VIBE_BROWSER_SESSION    Same as --session
VIBE_BROWSER_BROWSER    Same as --browser
VIBE_BROWSER_DEBUG      Enable debug logging
VIBE_BROWSER_SOCKET_DIR Override socket directory (see platform-specific defaults below)
CHROME_PATH             Path to Chrome executable
```

### Platform-Specific Defaults

| Setting | macOS / Linux | Windows |
|---------|---------------|---------|
| Socket directory | `~/.vibe-browser/` or `$XDG_RUNTIME_DIR/vibe-browser/` | `%TEMP%\vibe-browser\` |
| Default browser | Auto-detected from PATH | Auto-detected from PATH / Program Files |

> **Note**: The daemon mode (long-running sessions via Unix socket) is not available on Windows. On Windows, use direct CDP connections (`--cdp-url`) or launch browser per command.

## SDK Usage

```go
import "github.com/startvibecoding/vibe-browser/pkg/client"

c, _ := client.Open(ctx, &client.Options{
    CDPURL: "ws://127.0.0.1:9222/devtools/browser",
})
defer c.Close()

c.Navigate(ctx, "https://example.com")
title, _ := c.Title(ctx)
screenshot, _ := c.Screenshot(ctx)
```

## Troubleshooting

### Browser not found

```bash
# Check installed browsers
vibe-browser browsers

# Or set CHROME_PATH
export CHROME_PATH=/path/to/chrome          # macOS/Linux
$env:CHROME_PATH="C:\path\to\chrome.exe"    # PowerShell (Windows)
```

### Connection refused

```bash
# Discover running browser
vibe-browser discover

# Or let vibe-browser launch Chrome automatically
vibe-browser open https://example.com
```

### Element not found

```bash
# Take a fresh snapshot
vibe-browser snapshot --interactive

# Use correct selector
vibe-browser wait selector "button.submit"
vibe-browser click "button.submit"
```
