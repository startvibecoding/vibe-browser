# AGENTS.md

Instructions for AI coding agents working with this codebase.

## Project Overview

vibe-browser is a browser automation tool written in Go. It provides both a CLI binary and a Go SDK for programmatic use. The project is inspired by [agent-browser](https://github.com/vercel-labs/agent-browser) (Rust) but implemented in Go with minimal dependencies.

## Build System

This project uses **Make** for build automation. Always use `make` commands instead of raw `go build`.

```bash
make build              # Build the binary
make build-compressed   # Build with UPX compression
make build-all          # Build for all platforms (linux, darwin, windows)
make test               # Run tests
make clean              # Clean build artifacts
make install            # Install binary to $GOPATH/bin
```

## Code Style

- Use standard Go formatting (`gofmt`)
- Keep imports organized: stdlib, then external packages
- Use `golang.org/x/` packages for extended stdlib functionality
- Avoid unnecessary external dependencies
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
│   ├── chrome/           # Chrome launcher
│   └── server/           # Internal server utilities
└── examples/             # Example usage
```

### Package Responsibilities

- **cmd/vibe-browser**: CLI entry point, flag parsing, command dispatch
- **pkg/browser**: High-level browser automation (navigate, click, fill, snapshot, screenshot)
- **pkg/cdp**: WebSocket client for Chrome DevTools Protocol
- **pkg/client**: Go SDK for programmatic use (direct and daemon modes)
- **pkg/daemon**: Unix socket server for long-running browser sessions
- **pkg/mcp**: Model Context Protocol server for AI agents
- **pkg/protocol**: Shared types (Request, Response, LaunchOptions, etc.)
- **internal/chrome**: Chrome process launcher and discovery

## Dependencies

Minimal external dependencies:

- `golang.org/x/net` - WebSocket client (official Go extension)
- `golang.org/x/text` - Indirect dependency
- Standard library only for CLI (`flag`), JSON, HTTP, etc.

When adding new features, prefer standard library solutions. Only add `golang.org/x/` packages if necessary.

## Testing

```bash
make test               # Run all tests
go test ./pkg/protocol  # Run specific package tests
go test -v ./...        # Verbose output
```

Test files use `_test.go` suffix. Use `github.com/stretchr/testify` for assertions (test-only dependency).

## Environment Variables

- `VIBE_BROWSER_CDP_URL`: Chrome DevTools Protocol WebSocket URL
- `VIBE_BROWSER_SESSION`: Session name (default "default")
- `VIBE_BROWSER_SOCKET_DIR`: Override socket directory
- `VIBE_BROWSER_DEBUG`: Enable debug logging
- `CHROME_PATH`: Path to Chrome executable

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

### Adding a New CDP Feature

1. Add CDP command in `pkg/cdp/client.go` (Send/SendToSession methods)
2. Use it in `pkg/browser/browser.go`
3. Follow existing patterns for error handling and response parsing

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

## SDK Usage Pattern

```go
import "github.com/startvibecoding/vibe-browser/pkg/client"

// Direct connection
c, _ := client.Open(ctx, &client.Options{
    CDPURL: "ws://...",
})
defer c.Close()

// Daemon connection
c, _ := client.Connect(ctx, &client.Options{
    Session: "my-session",
})
defer c.Close()
```

## Binary Size Optimization

The project prioritizes small binary size:

1. Use standard library where possible
2. Use `golang.org/x/` instead of third-party packages
3. Build with `-ldflags "-s -w"` to strip debug info
4. Optional UPX compression for production builds

## MCP Server

The MCP server exposes browser automation tools to AI agents. Tools are registered in `pkg/mcp/mcp.go` and follow the naming pattern:

```
vibe_browser_<action>
```

Each tool has:
- Name
- Description
- Input schema (JSON Schema format)
- Handler function

## Releasing

1. Update version in `cmd/vibe-browser/main.go`
2. Update `CHANGELOG.md` (if exists)
3. Run `make build-all` to build for all platforms
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

## Windows Support

- Unix sockets are used on Linux/macOS
- TCP sockets are used on Windows (with .port file)
- Cross-compilation: `GOOS=windows GOARCH=amd64 make build`
