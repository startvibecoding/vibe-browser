# vibe-browser

Fast browser automation CLI and Go SDK for AI agents.

Inspired by [agent-browser](https://github.com/vercel-labs/agent-browser), vibe-browser provides a complete browser automation surface in Go with support for both CLI and SDK modes.

## Features

- **CLI Mode**: Run browser automation commands directly from the terminal
- **SDK Mode**: Import as a Go library for programmatic use
- **MCP Server**: Expose tools to AI agents (Claude, Cursor, etc.)
- **Daemon Mode**: Long-running browser session management
- **CDP Protocol**: Full Chrome DevTools Protocol support
- **Accessibility Snapshots**: Capture page structure for AI consumption
- **Screenshots**: Capture page or element screenshots
- **Cookies & Storage**: Manage browser state
- **Network Control**: Headers, offline mode, geolocation

## Installation

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

## CLI Usage

### Basic Commands

```bash
# Open a browser and navigate to a URL
vibe-browser open https://example.com

# Take a snapshot of the page
vibe-browser snapshot

# Click an element
vibe-browser click "button.submit"

# Fill an input field
vibe-browser fill "input[name=email]" --value "user@example.com"

# Take a screenshot
vibe-browser screenshot -o screenshot.png

# Evaluate JavaScript
vibe-browser eval "document.title"
```

### Navigation

```bash
vibe-browser navigate https://example.com
vibe-browser back
vibe-browser forward
vibe-browser reload
```

### Interaction

```bash
vibe-browser click "button"
vibe-browser dblclick "div.item"
vibe-browser hover "a.link"
vibe-browser fill "input" --value "text"
vibe-browser type "input" "Hello World" --delay 50
vibe-browser press Enter
vibe-browser select "select" --value "option1"
vibe-browser check "input[type=checkbox]"
vibe-browser uncheck "input[type=checkbox]"
vibe-browser focus "input"
vibe-browser scroll --y 100
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

    // Connect to existing Chrome instance
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

    // Take a snapshot
    snapshot, err := c.Snapshot(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(snapshot)

    // Click an element
    if err := c.Click(ctx, "a.more-info"); err != nil {
        log.Fatal(err)
    }

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
    "github.com/startvibecoding/vibe-browser/pkg/protocol"
)

func main() {
    ctx := context.Background()

    // Launch a new browser
    c, err := client.Open(ctx, &client.Options{
        Headless: true,
        Launch: &protocol.LaunchOptions{
            ViewportWidth:  1920,
            ViewportHeight: 1080,
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Use the browser...
}
```

### Daemon Mode

```go
package main

import (
    "context"
    "log"

    "github.com/startvibecoding/vibe-browser/pkg/client"
)

func main() {
    ctx := context.Background()

    // Connect to a running daemon
    c, err := client.Connect(ctx, &client.Options{
        Session: "my-session",
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
│   ├── chrome/           # Chrome launcher
│   └── server/           # Internal server utilities
├── docs/                 # Documentation
├── scripts/              # Build scripts
└── examples/             # Example usage
```

## Build

```bash
make build              # Build binary (4.4MB)
make build-compressed   # Build with UPX compression (1.9MB)
make build-all          # Build for all platforms
make test               # Run tests
make clean              # Clean build artifacts
```

## Environment Variables

- `VIBE_BROWSER_CDP_URL`: Same as --cdp-url flag
- `VIBE_BROWSER_SESSION`: Same as --session flag
- `VIBE_BROWSER_SOCKET_DIR`: Override default socket directory
- `VIBE_BROWSER_DEBUG`: Enable debug logging
- `CHROME_PATH`: Path to Chrome executable

## Dependencies

Minimal dependencies using Go standard library:

- `golang.org/x/net` - WebSocket client (official Go extension)
- `golang.org/x/text` - Indirect dependency
- Standard library `flag` - CLI parsing (no cobra/pflag)

## Comparison with agent-browser

| Feature | agent-browser | vibe-browser |
|---------|--------------|--------------|
| Language | Rust | Go |
| CLI Mode | ✓ | ✓ |
| SDK Mode | Node.js | Go |
| MCP Server | ✓ | ✓ |
| Daemon Mode | ✓ | ✓ |
| CDP Protocol | ✓ | ✓ |
| WebDriver | ✓ | Planned |
| Binary Size | ~10MB | 4.4MB (1.9MB compressed) |

## License

Apache-2.0
