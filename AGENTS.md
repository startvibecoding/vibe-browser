# AGENTS.md

Instructions for AI coding agents working with this codebase.

## Project Overview

vibe-browser is a browser automation tool written in Go. It provides both a CLI binary and a Go SDK for programmatic use. The project is inspired by [agent-browser](https://github.com/vercel-labs/agent-browser) (Rust) but implemented in Go with minimal dependencies.

## Build System

This project uses **Make** for build automation. Always use `make` commands instead of raw `go build`.

```bash
make build              # Build the binary
make build-all          # Build for all platforms
make test               # Run tests
make clean              # Clean build artifacts
make install            # Install binary to $GOPATH/bin

# npm packaging
make npm-packages       # Build platform-specific npm packages
make npm-publish-all    # Publish all npm packages
```

## Code Style

- Use standard Go formatting (`gofmt`)
- Keep imports organized: stdlib, then external packages
- Use `golang.org/x/` packages for extended stdlib functionality
- Prefer `github.com/gorilla/websocket` for WebSocket connections
- CLI flags use kebab-case (e.g., `--cdp-url`, `--full-page`)
- Use `flag` package for CLI parsing (not cobra/pflag)

## Architecture

```
vibe-browser/
├── cmd/vibe-browser/     # CLI binary entry point
├── pkg/
│   ├── browser/          # High-level browser operations
│   ├── cdp/              # Chrome DevTools Protocol client
│   ├── client/           # Go SDK client
│   ├── daemon/           # Daemon server (Unix socket)
│   ├── mcp/              # MCP server for AI agents
│   └── protocol/         # Protocol types and constants
├── internal/
│   └── chrome/           # Browser launcher and discovery
└── npm/                  # npm packages
```

### Package Responsibilities

- **cmd/vibe-browser**: CLI entry point, flag parsing, command dispatch
- **pkg/browser**: High-level browser automation (navigate, click, fill, snapshot, screenshot)
- **pkg/cdp**: WebSocket client for Chrome DevTools Protocol (uses gorilla/websocket)
- **pkg/client**: Go SDK for programmatic use (direct and daemon modes)
- **pkg/daemon**: Unix socket server for long-running browser sessions
- **pkg/mcp**: Model Context Protocol server for AI agents
- **pkg/protocol**: Shared types (Request, Response, LaunchOptions, etc.)
- **internal/chrome**: Browser launcher, discovery, and multi-browser support

## Dependencies

- `github.com/gorilla/websocket` - WebSocket client for CDP
- `github.com/stretchr/testify` - Test assertions (test-only)
- Standard library for everything else

When adding new features, prefer standard library solutions. Only add external packages if necessary.

## Browser Discovery

The browser discovery mechanism (in `internal/chrome/launcher.go`) supports:

1. **Auto-connect**: Find running browser via DevToolsActivePort or common ports
2. **Multi-browser**: Chrome, Chromium, Brave, Edge, Chrome Canary
3. **Platform-specific**: macOS, Linux, Windows paths for each browser

### Discovery Order

1. Check DevToolsActivePort files in user data directories
2. Probe common CDP ports (9222, 9229)
3. HTTP /json/version endpoint
4. HTTP /json/list endpoint

### Supported Browsers

| Browser | Type Constant |
|---------|--------------|
| Chrome | `chrome.BrowserChrome` |
| Chromium | `chrome.BrowserChromium` |
| Brave | `chrome.BrowserBrave` |
| Edge | `chrome.BrowserEdge` |
| Chrome Canary | `chrome.BrowserChromeCanary` |

## CDP Protocol

The CDP client (`pkg/cdp/client.go`) uses gorilla/websocket for reliable WebSocket connections. Key points:

- Connect to browser-level URL: `ws://host:port/devtools/browser/{id}`
- Connect to page-level URL: `ws://host:port/devtools/page/{id}`
- Page commands (Page.navigate, Page.enable) must be sent to page targets
- Browser commands (Target.createTarget) can be sent to browser targets

## Testing

```bash
make test               # Run all tests
go test ./pkg/protocol  # Run specific package tests
go test -v ./...        # Verbose output
```

Test files use `_test.go` suffix. Use `github.com/stretchr/testify` for assertions.

## Common Patterns

### Adding a New CLI Command

1. Add command handler function in `cmd/vibe-browser/main.go`
2. Add case in the main switch statement
3. Update `printUsage()` with command description
4. Implement corresponding SDK method in `pkg/client/client.go` if needed

### Adding a New Browser Operation

1. Add method to `pkg/browser/browser.go`
2. Add corresponding action in `pkg/daemon/daemon.go` executeCommand
3. Add MCP tool in `pkg/mcp/mcp.go` if exposing to AI agents
4. Update protocol types in `pkg/protocol/types.go` if needed

### Adding Browser Support

1. Add browser type constant in `internal/chrome/launcher.go`
2. Add executable candidates for each platform (macOS, Linux, Windows)
3. Add user data directory paths for each platform
4. Update `FindBrowser()` and `getUserDataDirs()` functions

## CLI Command Structure

Commands follow this pattern:

```
vibe-browser <command> [args...] [--flags]
```

Subcommands use space separation:

```
vibe-browser get text "selector"
vibe-browser is visible "selector"
vibe-browser wait selector "selector"
```

## Binary Size Optimization

The project prioritizes small binary size:

1. Use standard library where possible
2. Use `github.com/gorilla/websocket` for reliable WebSocket (small overhead)
3. Build with `-ldflags "-s -w"` to strip debug info
4. Optional UPX compression for production builds

## npm Packaging

npm packages are scoped under `@startvibecoding`:

- Main package: `@startvibecoding/vibe-browser`
- Platform packages: `@startvibecoding/vibe-browser-linux-x64`, etc.

The wrapper script (`scripts/npm-installer-wrapper.js`) detects the platform and resolves the correct binary.

## Releasing

1. Update version in `npm/package.json`
2. Run `make npm-packages` to build platform packages
3. Run `make npm-publish-all` to publish to npm
4. Create git tag with version
5. Push tag to trigger CI release

## Debugging

Enable debug logging:

```bash
export VIBE_BROWSER_DEBUG=1
vibe-browser open https://example.com
```

Or use the logger in code:

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```
