# Commands Reference

Complete reference for all vibe-browser CLI commands.

## Navigation

```bash
vibe-browser open [url]              # Open browser, optionally navigate
vibe-browser navigate <url>          # Navigate to URL
vibe-browser back                    # Go back in history
vibe-browser forward                 # Go forward in history
vibe-browser reload                  # Reload current page
```

## Interaction

```bash
vibe-browser click <selector>        # Click an element
vibe-browser dblclick <selector>     # Double-click an element
vibe-browser hover <selector>        # Hover over element
vibe-browser focus <selector>        # Focus an element
vibe-browser fill <selector> --value <text>  # Clear and fill input
vibe-browser type <selector> <text>  # Type without clearing
vibe-browser type <selector> <text> --delay <ms>  # Type with delay
vibe-browser press <key>             # Press keyboard key
vibe-browser check <selector>        # Check checkbox
vibe-browser uncheck <selector>      # Uncheck checkbox
vibe-browser select <selector> --value <value>  # Select dropdown option
vibe-browser scroll --x <px> --y <px>  # Scroll page
```

## Reading

```bash
vibe-browser snapshot                # Full accessibility tree
vibe-browser snapshot -i             # Interactive elements only
vibe-browser snapshot -i -c          # Compact mode
vibe-browser get text <selector>     # Get element text
vibe-browser get html <selector>     # Get element HTML
vibe-browser get value <selector>    # Get input value
vibe-browser get attr <selector> <attr>  # Get attribute
vibe-browser get url                 # Get current URL
vibe-browser get title               # Get page title
```

## State Checking

```bash
vibe-browser is visible <selector>   # Check if visible
vibe-browser is enabled <selector>   # Check if enabled
vibe-browser is checked <selector>   # Check if checked
```

## Waiting

```bash
vibe-browser wait ms <milliseconds>  # Wait for duration
vibe-browser wait selector <selector>  # Wait for element
vibe-browser wait text <text>        # Wait for text
vibe-browser wait url <pattern>      # Wait for URL
```

## Screenshots

```bash
vibe-browser screenshot              # Output to stdout
vibe-browser screenshot -o <file>    # Save to file
vibe-browser screenshot --format png # PNG format (default)
vibe-browser screenshot --format jpeg  # JPEG format
vibe-browser screenshot --full-page  # Capture full scroll height
```

## JavaScript

```bash
vibe-browser eval <expression>       # Evaluate JavaScript
```

## Browser Settings

```bash
vibe-browser set viewport --width <px> --height <px>  # Set viewport
vibe-browser set geolocation <lat> <lng>  # Set geolocation
vibe-browser set offline             # Enable offline mode
```

## Cookies

```bash
vibe-browser cookies get             # Get all cookies
vibe-browser cookies clear           # Clear all cookies
```

## Tabs

```bash
vibe-browser tab new [url]           # Open new tab
vibe-browser tab list                # List tabs
vibe-browser tab close <tabId>       # Close tab
```

## Session Management

```bash
vibe-browser daemon --session <name> # Start daemon session
vibe-browser mcp --session <name>    # Start MCP server
vibe-browser close                   # Close browser
```

## Global Flags

```bash
--cdp-url <url>        # Chrome DevTools Protocol WebSocket URL
--session <name>       # Session name (default "default")
--headless             # Run in headless mode (default true)
--executable-path <path>  # Path to Chrome executable
```

## Environment Variables

```bash
VIBE_BROWSER_CDP_URL   # Same as --cdp-url
VIBE_BROWSER_SESSION   # Same as --session
VIBE_BROWSER_SOCKET_DIR  # Override socket directory
VIBE_BROWSER_DEBUG     # Enable debug logging
CHROME_PATH            # Path to Chrome executable
```
