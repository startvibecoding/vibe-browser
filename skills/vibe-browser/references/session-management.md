# Session Management

vibe-browser supports multiple isolated browser sessions for parallel automation.

## Daemon Mode

The daemon runs as a background process and manages browser sessions:

```bash
# Start a daemon session
vibe-browser daemon --session my-session

# The daemon listens on a Unix socket
# ~/.vibe-browser/my-session.sock
```

## Using Sessions

Connect to a specific session:

```bash
# Use --session flag
vibe-browser open https://example.com --session my-session
vibe-browser snapshot -i --session my-session

# Or set environment variable
export VIBE_BROWSER_SESSION=my-session
vibe-browser open https://example.com
```

## Multiple Sessions

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

## Session Persistence

Sessions persist across commands. The browser stays running until explicitly closed:

```bash
vibe-browser open https://example.com --session my-session
vibe-browser snapshot -i --session my-session  # Same browser
vibe-browser click "button" --session my-session  # Still same browser
vibe-browser close --session my-session  # Now close it
```

## Socket Directory

Default socket location: `~/.vibe-browser/`

Override with:
```bash
export VIBE_BROWSER_SOCKET_DIR=/tmp/my-sessions
```

## SDK Session Usage

```go
import "github.com/startvibecoding/vibe-browser/pkg/client"

// Connect to daemon session
c, _ := client.Connect(ctx, &client.Options{
    Session: "my-session",
})
defer c.Close()
```
