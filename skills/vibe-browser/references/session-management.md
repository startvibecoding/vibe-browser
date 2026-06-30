# Session Management

vibe-browser supports multiple isolated browser sessions for parallel automation.

## Daemon Mode (macOS/Linux)

The daemon runs as a background process and manages browser sessions:

```bash
# Start a daemon session
vibe-browser daemon --session my-session

# The daemon listens on a Unix socket
# ~/.vibe-browser/my-session.sock
```

## Direct Mode (All Platforms)

For one-off commands or platforms without Unix socket support (Windows), use direct CDP connections:

```bash
# Launch browser and connect via CDP
vibe-browser open https://example.com --cdp-url ws://127.0.0.1:9222/devtools/browser

# Or set via environment variable
export VIBE_BROWSER_CDP_URL="ws://127.0.0.1:9222/devtools/browser"  # macOS/Linux
$env:VIBE_BROWSER_CDP_URL = "ws://127.0.0.1:9222/devtools/browser"  # PowerShell
vibe-browser open https://example.com
```

## Using Sessions

Connect to a specific session:

```bash
# Use --session flag
vibe-browser open https://example.com --session my-session
vibe-browser snapshot -i --session my-session

# Or set environment variable
export VIBE_BROWSER_SESSION=my-session           # macOS/Linux (bash/zsh)
$env:VIBE_BROWSER_SESSION = "my-session"         # Windows (PowerShell)
set VIBE_BROWSER_SESSION=my-session              # Windows (cmd)
vibe-browser open https://example.com
```

## Multiple Sessions (macOS/Linux)

Run multiple isolated browsers:

```bash
# Terminal 1
vibe-browser daemon --session user-a

# Terminal 2
vibe-browser daemon --session user-b

# Use them
vibe-browser open https://app.example.com --session user-a
vibe-browser open https://app.example.com --session user-b
```

## Multiple Instances (Windows)

On Windows, use separate CDP ports for parallel sessions:

```powershell
# Instance 1 - port 9222
vibe-browser open https://app.example.com --cdp-url ws://127.0.0.1:9222/devtools/browser

# Instance 2 - port 9223
vibe-browser open https://app.example.com --cdp-url ws://127.0.0.1:9223/devtools/browser
```

## Session Persistence

Sessions persist across commands. The browser stays running until explicitly closed:

```bash
vibe-browser open https://example.com --session my-session
vibe-browser snapshot -i --session my-session  # Same browser
vibe-browser click "button" --session my-session  # Still same browser
vibe-browser close --session my-session  # Now close it
```

## Socket Directory

Default socket location varies by platform:

| Platform | Default Path |
|----------|-------------|
| macOS/Linux | `~/.vibe-browser/` |
| Linux (with XDG) | `$XDG_RUNTIME_DIR/vibe-browser/` |
| Windows | `%TEMP%\vibe-browser\` |

Override with:
```bash
export VIBE_BROWSER_SOCKET_DIR=/tmp/my-sessions        # macOS/Linux
$env:VIBE_BROWSER_SOCKET_DIR = "C:\temp\sessions"      # PowerShell (Windows)
```

## SDK Session Usage

```go
import "github.com/startvibecoding/vibe-browser/pkg/client"

// Connect to daemon session (macOS/Linux)
c, _ := client.Connect(ctx, &client.Options{
    Session: "my-session",
})
defer c.Close()

// Direct CDP connection (all platforms)
c, _ := client.Open(ctx, &client.Options{
    CDPURL: "ws://127.0.0.1:9222/devtools/browser",
})
defer c.Close()
```
