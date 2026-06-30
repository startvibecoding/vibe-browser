---
name: core
description: Core vibe-browser usage guide. Read this before running any vibe-browser commands. Covers the snapshot-and-ref workflow, navigating pages, interacting with elements (click, fill, type, select), extracting text and data, taking screenshots, managing tabs, handling forms and auth, waiting for content, and troubleshooting common failures. Use when the user asks to interact with a website, fill a form, click something, extract data, take a screenshot, log into a site, test a web app, or automate any browser task.
allowed-tools: Bash(vibe-browser:*), Bash(npx vibe-browser:*)
---

# vibe-browser core

Fast browser automation CLI for AI agents. Chrome/Chromium via CDP, no Playwright or Puppeteer dependency. Accessibility-tree snapshots with compact refs let agents interact with pages efficiently.

## The core loop

```bash
vibe-browser open <url>        # 1. Open a page
vibe-browser snapshot -i       # 2. See what's on it (interactive elements only)
vibe-browser click "button"    # 3. Act on elements from the snapshot
vibe-browser snapshot -i       # 4. Re-snapshot after any page change
```

Refs are assigned fresh on every snapshot. They become **stale the moment the page changes** — after clicks that navigate, form submits, dynamic re-renders, dialog opens. Always re-snapshot before your next interaction.

## Quickstart

```bash
# Install once
npm i -g @startvibecoding/vibe-browser

# Take a screenshot of a page
vibe-browser open https://example.com
vibe-browser screenshot -o home.png
vibe-browser close

# Search, click a result, and capture it
vibe-browser open https://duckduckgo.com
vibe-browser snapshot -i                      # find the search box
vibe-browser fill "input[name=q]" "vibe-browser cli"
vibe-browser press Enter
vibe-browser wait text "Results"
vibe-browser snapshot -i                      # refs now reflect results
vibe-browser click "a.result"                 # click a result
vibe-browser screenshot -o result.png
```

The browser stays running across commands so these feel like a single session. Use `vibe-browser close` when you're done.

## MCP integration

For tools that support Model Context Protocol servers, start the stdio server:

```bash
vibe-browser mcp
```

Configure the MCP client to launch `vibe-browser` with `["mcp"]`. The server exposes all browser automation tools to AI agents.

## Reading a page

```bash
vibe-browser snapshot                    # full tree (verbose)
vibe-browser snapshot -i                 # interactive elements only (preferred)
vibe-browser snapshot -i -c              # compact (no empty structural nodes)
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
vibe-browser get text "h1"              # visible text of an element
vibe-browser get html "div.content"     # innerHTML
vibe-browser get attr "a" href          # any attribute
vibe-browser get value "input"          # input value
vibe-browser get url                    # current URL
vibe-browser get title                  # page title
```

## Interacting

```bash
vibe-browser click "button"             # click by CSS selector
vibe-browser dblclick "div.item"        # double-click
vibe-browser hover "a.link"             # hover
vibe-browser focus "input"              # focus (useful before keyboard input)
vibe-browser fill "input" --value "hello"  # clear then type
vibe-browser type "input" " world"      # type without clearing
vibe-browser press Enter                # press a key at current focus
vibe-browser press Control+a            # key combination
vibe-browser check "input[type=checkbox]"   # check checkbox
vibe-browser uncheck "input[type=checkbox]" # uncheck
vibe-browser select "select" --value "option1"  # select dropdown option
vibe-browser scroll --y 500             # scroll page
```

Rule of thumb: snapshot + CSS selectors are fastest and most reliable for AI agents.

## Waiting

Agents fail more often from bad waits than from bad selectors. Pick the right wait for the situation:

```bash
vibe-browser wait selector "div.loaded"  # until an element appears
vibe-browser wait ms 2000                # dumb wait, milliseconds (last resort)
vibe-browser wait text "Success"         # until the text appears on the page
vibe-browser wait url "/dashboard"       # until URL contains pattern
```

After any page-changing action, pick one:

- Wait for a specific element you expect to appear: `wait selector "..."`.
- Wait for URL change: `wait url "/new-page"`.

Avoid bare `wait ms 2000` except when debugging — it makes scripts slow and flaky.

## Common workflows

### Log in

```bash
vibe-browser open https://app.example.com/login
vibe-browser snapshot -i

# Pick the selectors out of the snapshot, then:
vibe-browser fill "input[name=email]" "user@example.com"
vibe-browser fill "input[name=password]" "hunter2"
vibe-browser click "button[type=submit]"
vibe-browser wait url "/dashboard"
vibe-browser snapshot -i
```

### Extract data

```bash
# Get page title
vibe-browser get title

# Get text from specific elements
vibe-browser get text "h1"
vibe-browser get text ".price"

# Get attribute values
vibe-browser get attr "a.main-link" href

# Evaluate JavaScript for complex extraction
vibe-browser eval "document.querySelectorAll('table tr').length"
```

### Screenshot

```bash
vibe-browser screenshot                        # output to stdout
vibe-browser screenshot -o page.png            # save to file
vibe-browser screenshot --full-page -o full.png  # full scroll height
```

### Handle multiple pages via tabs

```bash
vibe-browser tab new https://docs...  # open a new tab
vibe-browser tab list                  # list open tabs
vibe-browser tab close <tabId>         # close a tab
```

## Environment Variables

- `VIBE_BROWSER_CDP_URL`: Chrome DevTools Protocol WebSocket URL
- `VIBE_BROWSER_SESSION`: Session name (default "default")
- `VIBE_BROWSER_SOCKET_DIR`: Override socket directory
- `VIBE_BROWSER_DEBUG`: Enable debug logging
- `CHROME_PATH`: Path to Chrome executable

## SDK Usage

For programmatic use in Go:

```go
import "github.com/startvibecoding/vibe-browser/pkg/client"

// Connect to existing browser
c, _ := client.Open(ctx, &client.Options{
    CDPURL: "ws://127.0.0.1:9222/devtools/browser",
})
defer c.Close()

// Navigate and interact
c.Navigate(ctx, "https://example.com")
title, _ := c.Title(ctx)
c.Click(ctx, "button")
screenshot, _ := c.Screenshot(ctx)
```

## Troubleshooting

### Browser not found

If you get "Chrome not found" errors:

1. Install Chrome/Chromium
2. Or set `CHROME_PATH` environment variable
3. Or use `--executable-path` flag

### Connection refused

If you get "connection refused" errors:

1. Make sure Chrome is running with `--remote-debugging-port=9222`
2. Or let vibe-browser launch Chrome automatically

### Element not found

If selectors don't work:

1. Take a fresh snapshot: `vibe-browser snapshot -i`
2. Use the correct CSS selector from the snapshot
3. Wait for the element: `vibe-browser wait selector "..."`

### Stale refs

If interactions fail after page changes:

1. Re-snapshot: `vibe-browser snapshot -i`
2. Use the new refs/selectors
