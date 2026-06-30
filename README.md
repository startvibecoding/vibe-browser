# vibe-browser

Fast browser automation CLI and Go SDK for AI agents.

Inspired by [agent-browser](https://github.com/vercel-labs/agent-browser), vibe-browser provides a complete browser automation surface in Go with support for both CLI and SDK modes.

## Features

- **CLI Mode**: Run browser automation commands directly from the terminal
- **SDK Mode**: Import as a Go library for programmatic use
- **MCP Server**: Expose tools to AI agents (Claude, Cursor, etc.)
- **Daemon Mode**: Long-running browser session management
- **CDP Protocol**: Full Chrome DevTools Protocol support
- **Multi-Browser**: Chrome, Chromium, Brave, Edge support
- **Auto-Discovery**: Automatically find running browsers
- **Accessibility Snapshots**: Capture page structure for AI consumption
- **Screenshots**: Capture page or element screenshots

## Installation

### npm (Recommended)

```bash
npm install -g @startvibecoding/vibe-browser
```

### From Source

```bash
git clone https://github.com/startvibecoding/vibe-browser.git
cd vibe-browser
make build
```

### Using Go

```bash
go install github.com/startvibecoding/vibe-browser/cmd/vibe-browser@latest
```

## Quick Start

```bash
# Open a page (auto-launches Chrome if not running)
vibe-browser open https://example.com

# Get page title
vibe-browser get title

# Take a screenshot
vibe-browser screenshot --output page.png

# Take a snapshot of interactive elements
vibe-browser snapshot --interactive
```

## CLI Usage

### Browser Discovery

```bash
# Auto-detect running browser
vibe-browser discover

# List installed browsers
vibe-browser browsers

# List Chrome profiles
vibe-browser profiles
```

### Navigation

```bash
vibe-browser open https://example.com
vibe-browser navigate https://example.com
vibe-browser back
vibe-browser forward
vibe-browser reload
```

### Interaction

```bash
vibe-browser click "button.submit"
vibe-browser dblclick "div.item"
vibe-browser hover "a.link"
vibe-browser fill "input[name=email]" --value "user@example.com"
vibe-browser type "input" "Hello World" --delay 50
vibe-browser press Enter
vibe-browser select "select" --value "option1"
vibe-browser check "input[type=checkbox]"
vibe-browser uncheck "input[type=checkbox]"
vibe-browser focus "input"
vibe-browser scroll --x 0 --y 500
```

### Reading

```bash
vibe-browser get text "p.content"
vibe-browser get html "div.container"
vibe-browser get value "input"
vibe-browser get attr "a" href
vibe-browser get url
vibe-browser get title
```

### State Checking

```bash
vibe-browser is visible "div.modal"
vibe-browser is enabled "button"
vibe-browser is checked "input[type=checkbox]"
```

### Waiting

```bash
vibe-browser wait ms 1000
vibe-browser wait selector "div.loaded"
vibe-browser wait text "Welcome"
vibe-browser wait url "/dashboard"
```

### Screenshots

```bash
vibe-browser screenshot --output page.png
vibe-browser screenshot --full-page --output full.png
vibe-browser screenshot --format jpeg --output page.jpg
```

### Browser Settings

```bash
vibe-browser set viewport --width 1920 --height 1080
vibe-browser cookies get
vibe-browser cookies clear
```

### Daemon Mode

```bash
# Start a daemon session
vibe-browser daemon --session my-session

# Connect to daemon from another terminal
vibe-browser open https://example.com --session my-session
```

### MCP Server

```bash
# Start MCP server for AI agents
vibe-browser mcp --session my-session
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
VIBE_BROWSER_SOCKET_DIR Override socket directory
CHROME_PATH             Path to Chrome executable
```

## Supported Browsers

| Browser | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Chrome | ✓ | ✓ | ✓ |
| Chromium | ✓ | ✓ | ✓ |
| Brave | ✓ | ✓ | ✓ |
| Edge | ✓ | ✓ | ✓ |
| Chrome Canary | ✓ | - | ✓ |

## SDK Usage

### Direct Mode

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/startvibecoding/vibe-browser/pkg/client"
)

func main() {
    ctx := context.Background()

    // Connect to existing browser or launch new one
    c, err := client.Open(ctx, &client.Options{
        CDPURL: "ws://127.0.0.1:9222/devtools/browser",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Navigate to a URL
    if err := c.Navigate(ctx, "https://example.com"); err != nil {
        log.Fatal(err)
    }

    // Get the page title
    title, err := c.Title(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Title:", title)

    // Take a screenshot
    screenshot, err := c.Screenshot(ctx)
    if err != nil {
        log.Fatal(err)
    }
    os.WriteFile("screenshot.png", screenshot, 0644)
}
```

### With Browser Launch

```go
package main

import (
    "context"
    "log"

    "github.com/startvibecoding/vibe-browser/pkg/client"
    "github.com/startvibecoding/vibe-browser/internal/chrome"
)

func main() {
    ctx := context.Background()

    // Launch a new browser
    c, err := client.Open(ctx, &client.Options{
        Browser: chrome.BrowserBrave,
        Headless: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Use the browser...
}
```

## Architecture

```
vibe-browser/
├── cmd/vibe-browser/     # CLI binary
├── pkg/
│   ├── browser/          # High-level browser operations
│   ├── cdp/              # Chrome DevTools Protocol client
│   ├── client/           # Go SDK client
│   ├── daemon/           # Daemon server
│   ├── mcp/              # MCP server for AI agents
│   └── protocol/         # Protocol types
├── internal/
│   └── chrome/           # Browser launcher and discovery
├── npm/                  # npm packages
├── scripts/              # Build scripts
├── skills/               # AI agent skills
└── skill-data/           # Skill documentation
```

## Build

```bash
make build              # Build binary
make build-all          # Build for all platforms
make test               # Run tests
make clean              # Clean build artifacts

make npm-packages       # Build npm packages
make npm-publish-all    # Publish to npm
```

## Comparison with agent-browser

| Feature | agent-browser | vibe-browser |
|---------|--------------|--------------|
| Language | Rust | Go |
| CLI Mode | ✓ | ✓ |
| SDK Mode | Node.js | Go |
| MCP Server | ✓ | ✓ |
| Daemon Mode | ✓ | ✓ |
| CDP Protocol | ✓ | ✓ |
| Multi-Browser | Chrome, Chromium, Brave | Chrome, Chromium, Brave, Edge |
| Binary Size | ~10MB | 4.4MB (1.9MB compressed) |

## License

Apache-2.0
