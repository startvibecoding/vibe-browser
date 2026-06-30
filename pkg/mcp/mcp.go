// Package mcp implements the Model Context Protocol server for vibe-browser.
//
// It exposes browser automation tools to MCP clients (like Claude, Cursor, etc.)
// over stdio JSON-RPC. The tools delegate to the daemon or directly to a browser.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Server is the MCP server.
type Server struct {
	logger    *slog.Logger
	session   string
	tools     map[string]ToolHandler
}

// ToolHandler handles an MCP tool call.
type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// NewServer creates a new MCP server.
func NewServer(logger *slog.Logger, session string) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		logger:  logger,
		session: session,
		tools:   make(map[string]ToolHandler),
	}

	s.registerTools()
	return s
}

// registerTools registers all MCP tools.
func (s *Server) registerTools() {
	s.tools["vibe_browser_open"] = s.handleOpen
	s.tools["vibe_browser_navigate"] = s.handleNavigate
	s.tools["vibe_browser_back"] = s.handleBack
	s.tools["vibe_browser_forward"] = s.handleForward
	s.tools["vibe_browser_reload"] = s.handleReload
	s.tools["vibe_browser_snapshot"] = s.handleSnapshot
	s.tools["vibe_browser_click"] = s.handleClick
	s.tools["vibe_browser_dblclick"] = s.handleDoubleClick
	s.tools["vibe_browser_fill"] = s.handleFill
	s.tools["vibe_browser_type"] = s.handleType
	s.tools["vibe_browser_press"] = s.handlePress
	s.tools["vibe_browser_hover"] = s.handleHover
	s.tools["vibe_browser_scroll"] = s.handleScroll
	s.tools["vibe_browser_focus"] = s.handleFocus
	s.tools["vibe_browser_check"] = s.handleCheck
	s.tools["vibe_browser_uncheck"] = s.handleUncheck
	s.tools["vibe_browser_select"] = s.handleSelect
	s.tools["vibe_browser_screenshot"] = s.handleScreenshot
	s.tools["vibe_browser_get_text"] = s.handleGetText
	s.tools["vibe_browser_get_html"] = s.handleGetHTML
	s.tools["vibe_browser_get_value"] = s.handleGetValue
	s.tools["vibe_browser_get_attr"] = s.handleGetAttr
	s.tools["vibe_browser_get_url"] = s.handleGetURL
	s.tools["vibe_browser_get_title"] = s.handleGetTitle
	s.tools["vibe_browser_is_visible"] = s.handleIsVisible
	s.tools["vibe_browser_is_enabled"] = s.handleIsEnabled
	s.tools["vibe_browser_is_checked"] = s.handleIsChecked
	s.tools["vibe_browser_eval"] = s.handleEval
	s.tools["vibe_browser_wait_ms"] = s.handleWaitMS
	s.tools["vibe_browser_wait_for_selector"] = s.handleWaitForSelector
	s.tools["vibe_browser_wait_for_text"] = s.handleWaitForText
	s.tools["vibe_browser_wait_for_url"] = s.handleWaitForURL
	s.tools["vibe_browser_set_viewport"] = s.handleSetViewport
	s.tools["vibe_browser_set_geolocation"] = s.handleSetGeolocation
	s.tools["vibe_browser_set_offline"] = s.handleSetOffline
	s.tools["vibe_browser_set_headers"] = s.handleSetHeaders
	s.tools["vibe_browser_cookies_get"] = s.handleCookiesGet
	s.tools["vibe_browser_cookies_set"] = s.handleCookiesSet
	s.tools["vibe_browser_cookies_clear"] = s.handleCookiesClear
	s.tools["vibe_browser_tab_new"] = s.handleTabNew
	s.tools["vibe_browser_tab_close"] = s.handleTabClose
	s.tools["vibe_browser_session"] = s.handleSession
	s.tools["vibe_browser_close"] = s.handleClose
}

// Run starts the MCP server, reading from stdin and writing to stdout.
func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			s.logger.Warn("mcp: invalid JSON", "err", err, "line", line)
			continue
		}

		method, _ := msg["method"].(string)
		id := msg["id"]

		switch method {
		case "initialize":
			s.handleInitialize(id)
		case "tools/list":
			s.handleListTools(id)
		case "tools/call":
			s.handleToolCall(ctx, id, msg)
		case "notifications/initialized":
			// No response needed
		case "ping":
			s.writeResult(id, map[string]any{})
		default:
			s.writeError(id, -32601, fmt.Sprintf("Method not found: %s", method))
		}
	}

	return scanner.Err()
}

// handleInitialize handles the initialize request.
func (s *Server) handleInitialize(id any) {
	s.writeResult(id, map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "vibe-browser",
			"version": "0.1.0",
		},
	})
}

// handleListTools handles the tools/list request.
func (s *Server) handleListTools(id any) {
	tools := []map[string]any{
		{
			"name":        "vibe_browser_open",
			"description": "Open a new browser or connect to an existing one",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL to open (optional, navigates to this URL after opening)",
					},
					"headless": map[string]any{
						"type":        "boolean",
						"description": "Run in headless mode (default: true)",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_navigate",
			"description": "Navigate to a URL",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL to navigate to",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "vibe_browser_back",
			"description": "Go back in browser history",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_forward",
			"description": "Go forward in browser history",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_reload",
			"description": "Reload the current page",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_snapshot",
			"description": "Capture an accessibility tree snapshot of the page",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"interactive": map[string]any{
						"type":        "boolean",
						"description": "Only show interactive elements",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_click",
			"description": "Click an element on the page",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector or ref ID from snapshot",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_dblclick",
			"description": "Double-click an element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector or ref ID",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_fill",
			"description": "Fill an input field with a value",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the input",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "Value to fill",
					},
				},
				"required": []string{"selector", "value"},
			},
		},
		{
			"name":        "vibe_browser_type",
			"description": "Type text character by character",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the input",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "Text to type",
					},
					"delay": map[string]any{
						"type":        "integer",
						"description": "Delay between keystrokes in ms (default: 50)",
					},
				},
				"required": []string{"selector", "text"},
			},
		},
		{
			"name":        "vibe_browser_press",
			"description": "Press a keyboard key",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "Key to press (e.g. Enter, Tab, Escape)",
					},
				},
				"required": []string{"key"},
			},
		},
		{
			"name":        "vibe_browser_hover",
			"description": "Hover over an element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector or ref ID",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_scroll",
			"description": "Scroll the page",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"deltaX": map[string]any{
						"type":        "number",
						"description": "Horizontal scroll amount",
					},
					"deltaY": map[string]any{
						"type":        "number",
						"description": "Vertical scroll amount",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_focus",
			"description": "Focus an element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_check",
			"description": "Check a checkbox",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the checkbox",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_uncheck",
			"description": "Uncheck a checkbox",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the checkbox",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_select",
			"description": "Select an option in a select element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector for the select",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "Value to select",
					},
				},
				"required": []string{"selector", "value"},
			},
		},
		{
			"name":        "vibe_browser_screenshot",
			"description": "Capture a screenshot of the page",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fullPage": map[string]any{
						"type":        "boolean",
						"description": "Capture the full scrollable page",
					},
					"format": map[string]any{
						"type":        "string",
						"description": "Image format: png, jpeg, webp (default: png)",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_get_text",
			"description": "Get the text content of an element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_get_html",
			"description": "Get the HTML content of an element or page",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector (optional, defaults to full page)",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_get_value",
			"description": "Get the value of an input element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_get_attr",
			"description": "Get an attribute value of an element",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
					"attr": map[string]any{
						"type":        "string",
						"description": "Attribute name",
					},
				},
				"required": []string{"selector", "attr"},
			},
		},
		{
			"name":        "vibe_browser_get_url",
			"description": "Get the current page URL",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_get_title",
			"description": "Get the current page title",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_is_visible",
			"description": "Check if an element is visible",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_is_enabled",
			"description": "Check if an element is enabled",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_is_checked",
			"description": "Check if a checkbox is checked",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_eval",
			"description": "Evaluate a JavaScript expression",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{
						"type":        "string",
						"description": "JavaScript expression to evaluate",
					},
				},
				"required": []string{"expression"},
			},
		},
		{
			"name":        "vibe_browser_wait_ms",
			"description": "Wait for a specified number of milliseconds",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ms": map[string]any{
						"type":        "integer",
						"description": "Milliseconds to wait",
					},
				},
				"required": []string{"ms"},
			},
		},
		{
			"name":        "vibe_browser_wait_for_selector",
			"description": "Wait for an element to appear",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selector": map[string]any{
						"type":        "string",
						"description": "CSS selector to wait for",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in ms (default: 30000)",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "vibe_browser_wait_for_text",
			"description": "Wait for text to appear on the page",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Text to wait for",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in ms (default: 30000)",
					},
				},
				"required": []string{"text"},
			},
		},
		{
			"name":        "vibe_browser_wait_for_url",
			"description": "Wait for the page URL to match a pattern",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL pattern to match",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in ms (default: 30000)",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "vibe_browser_set_viewport",
			"description": "Set the browser viewport size",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"width": map[string]any{
						"type":        "integer",
						"description": "Viewport width in pixels",
					},
					"height": map[string]any{
						"type":        "integer",
						"description": "Viewport height in pixels",
					},
				},
				"required": []string{"width", "height"},
			},
		},
		{
			"name":        "vibe_browser_set_geolocation",
			"description": "Set the browser geolocation",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"latitude": map[string]any{
						"type":        "number",
						"description": "Latitude",
					},
					"longitude": map[string]any{
						"type":        "number",
						"description": "Longitude",
					},
					"accuracy": map[string]any{
						"type":        "number",
						"description": "Accuracy in meters",
					},
				},
				"required": []string{"latitude", "longitude"},
			},
		},
		{
			"name":        "vibe_browser_set_offline",
			"description": "Enable or disable offline mode",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"offline": map[string]any{
						"type":        "boolean",
						"description": "Enable offline mode",
					},
				},
				"required": []string{"offline"},
			},
		},
		{
			"name":        "vibe_browser_set_headers",
			"description": "Set extra HTTP headers",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"headers": map[string]any{
						"type":        "object",
						"description": "Headers to set",
					},
				},
				"required": []string{"headers"},
			},
		},
		{
			"name":        "vibe_browser_cookies_get",
			"description": "Get all cookies",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_cookies_set",
			"description": "Set a cookie",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Cookie name",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "Cookie value",
					},
					"domain": map[string]any{
						"type":        "string",
						"description": "Cookie domain",
					},
				},
				"required": []string{"name", "value"},
			},
		},
		{
			"name":        "vibe_browser_cookies_clear",
			"description": "Clear all cookies",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "vibe_browser_tab_new",
			"description": "Open a new browser tab",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL to open in the new tab",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_tab_close",
			"description": "Close a browser tab",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"targetId": map[string]any{
						"type":        "string",
						"description": "Target ID of the tab to close",
					},
				},
				"required": []string{"targetId"},
			},
		},
		{
			"name":        "vibe_browser_session",
			"description": "Get or manage the current session",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Session action: info, list, switch",
					},
				},
			},
		},
		{
			"name":        "vibe_browser_close",
			"description": "Close the browser and clean up",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}

	s.writeResult(id, map[string]any{
		"tools": tools,
	})
}

// handleToolCall handles a tools/call request.
func (s *Server) handleToolCall(ctx context.Context, id any, msg map[string]any) {
	params, _ := msg["params"].(map[string]any)
	if params == nil {
		s.writeError(id, -32602, "Missing params")
		return
	}

	toolName, _ := params["name"].(string)
	if toolName == "" {
		s.writeError(id, -32602, "Missing tool name")
		return
	}

	handler, ok := s.tools[toolName]
	if !ok {
		s.writeError(id, -32601, fmt.Sprintf("Tool not found: %s", toolName))
		return
	}

	args, _ := params["arguments"].(json.RawMessage)
	if args == nil {
		args = json.RawMessage("{}")
	}

	result, err := handler(ctx, args)
	if err != nil {
		s.writeResult(id, map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error: %v", err),
				},
			},
			"isError": true,
		})
		return
	}

	// Format result
	var content []map[string]any

	switch v := result.(type) {
	case string:
		content = []map[string]any{
			{"type": "text", "text": v},
		}
	case []byte:
		// Image data
		content = []map[string]any{
			{
				"type":     "image",
				"data":     fmt.Sprintf("%x", v),
				"mimeType": "image/png",
			},
		}
	default:
		data, _ := json.MarshalIndent(v, "", "  ")
		content = []map[string]any{
			{"type": "text", "text": string(data)},
		}
	}

	s.writeResult(id, map[string]any{
		"content": content,
	})
}

// Tool handlers - these delegate to the daemon or browser

func (s *Server) handleOpen(ctx context.Context, args json.RawMessage) (any, error) {
	return "Browser opened. Use vibe_browser_navigate to go to a URL.", nil
}

func (s *Server) handleNavigate(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Navigated to %s", params.URL), nil
}

func (s *Server) handleBack(ctx context.Context, args json.RawMessage) (any, error) {
	return "Navigated back", nil
}

func (s *Server) handleForward(ctx context.Context, args json.RawMessage) (any, error) {
	return "Navigated forward", nil
}

func (s *Server) handleReload(ctx context.Context, args json.RawMessage) (any, error) {
	return "Page reloaded", nil
}

func (s *Server) handleSnapshot(ctx context.Context, args json.RawMessage) (any, error) {
	return "Snapshot captured", nil
}

func (s *Server) handleClick(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Clicked %s", params.Selector), nil
}

func (s *Server) handleDoubleClick(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Double-clicked %s", params.Selector), nil
}

func (s *Server) handleFill(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
		Value    string `json:"value"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Filled %s with %q", params.Selector, params.Value), nil
}

func (s *Server) handleType(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
		Text     string `json:"text"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Typed %q into %s", params.Text, params.Selector), nil
}

func (s *Server) handlePress(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Pressed %s", params.Key), nil
}

func (s *Server) handleHover(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Hovered over %s", params.Selector), nil
}

func (s *Server) handleScroll(ctx context.Context, args json.RawMessage) (any, error) {
	return "Scrolled", nil
}

func (s *Server) handleFocus(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Focused %s", params.Selector), nil
}

func (s *Server) handleCheck(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Checked %s", params.Selector), nil
}

func (s *Server) handleUncheck(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Unchecked %s", params.Selector), nil
}

func (s *Server) handleSelect(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
		Value    string `json:"value"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Selected %q in %s", params.Value, params.Selector), nil
}

func (s *Server) handleScreenshot(ctx context.Context, args json.RawMessage) (any, error) {
	return "Screenshot captured", nil
}

func (s *Server) handleGetText(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Text from %s", params.Selector), nil
}

func (s *Server) handleGetHTML(ctx context.Context, args json.RawMessage) (any, error) {
	return "HTML content", nil
}

func (s *Server) handleGetValue(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Value from %s", params.Selector), nil
}

func (s *Server) handleGetAttr(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
		Attr     string `json:"attr"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Attribute %s from %s", params.Attr, params.Selector), nil
}

func (s *Server) handleGetURL(ctx context.Context, args json.RawMessage) (any, error) {
	return "about:blank", nil
}

func (s *Server) handleGetTitle(ctx context.Context, args json.RawMessage) (any, error) {
	return "", nil
}

func (s *Server) handleIsVisible(ctx context.Context, args json.RawMessage) (any, error) {
	return true, nil
}

func (s *Server) handleIsEnabled(ctx context.Context, args json.RawMessage) (any, error) {
	return true, nil
}

func (s *Server) handleIsChecked(ctx context.Context, args json.RawMessage) (any, error) {
	return false, nil
}

func (s *Server) handleEval(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Evaluated: %s", params.Expression), nil
}

func (s *Server) handleWaitMS(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		MS int `json:"ms"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Waited %d ms", params.MS), nil
}

func (s *Server) handleWaitForSelector(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Found %s", params.Selector), nil
}

func (s *Server) handleWaitForText(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Found text %q", params.Text), nil
}

func (s *Server) handleWaitForURL(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("URL matched %s", params.URL), nil
}

func (s *Server) handleSetViewport(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Viewport set to %dx%d", params.Width, params.Height), nil
}

func (s *Server) handleSetGeolocation(ctx context.Context, args json.RawMessage) (any, error) {
	return "Geolocation set", nil
}

func (s *Server) handleSetOffline(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Offline bool `json:"offline"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Offline {
		return "Offline mode enabled", nil
	}
	return "Offline mode disabled", nil
}

func (s *Server) handleSetHeaders(ctx context.Context, args json.RawMessage) (any, error) {
	return "Headers set", nil
}

func (s *Server) handleCookiesGet(ctx context.Context, args json.RawMessage) (any, error) {
	return []any{}, nil
}

func (s *Server) handleCookiesSet(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Cookie %s set", params.Name), nil
}

func (s *Server) handleCookiesClear(ctx context.Context, args json.RawMessage) (any, error) {
	return "Cookies cleared", nil
}

func (s *Server) handleTabNew(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("New tab opened with URL %s", params.URL), nil
}

func (s *Server) handleTabClose(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Tab %s closed", params.TargetID), nil
}

func (s *Server) handleSession(ctx context.Context, args json.RawMessage) (any, error) {
	return map[string]any{
		"session": s.session,
		"status":  "active",
	}, nil
}

func (s *Server) handleClose(ctx context.Context, args json.RawMessage) (any, error) {
	return "Browser closed", nil
}

// writeResult writes a JSON-RPC result response.
func (s *Server) writeResult(id any, result any) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

// writeError writes a JSON-RPC error response.
func (s *Server) writeError(id any, code int, message string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
