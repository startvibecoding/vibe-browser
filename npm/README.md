# vibe-browser

Fast browser automation CLI and Go SDK for AI agents.

## Installation

```bash
npm install -g @startvibecoding/vibe-browser
```

## Usage

```bash
# Open a page
vibe-browser open https://example.com

# Take a snapshot (see interactive elements)
vibe-browser snapshot -i

# Click an element
vibe-browser click "button"

# Take a screenshot
vibe-browser screenshot -o page.png
```

## Features

- CLI mode for direct terminal use
- SDK mode for Go programmatic access
- MCP server for AI agents (Claude, Cursor, etc.)
- Daemon mode for long-running sessions
- Chrome DevTools Protocol support
- Accessibility snapshots with element refs

## Links

- [GitHub](https://github.com/startvibecoding/vibe-browser)
- [Documentation](https://github.com/startvibecoding/vibe-browser#readme)

## License

Apache-2.0
