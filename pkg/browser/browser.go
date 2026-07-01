// Package browser provides high-level browser automation operations.
//
// It wraps the CDP client with methods for navigation, interaction,
// snapshot capture, screenshots, and more.
package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/startvibecoding/vibe-browser/pkg/cdp"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
)

// Target represents a browser target (page, background_page, etc.)
type Target struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	URL                   string `json:"url"`
	Type                  string `json:"type"`
	WebBrowserDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// ProcessKiller is a function that kills a browser process.
// This is used to avoid importing the chrome package directly.
type ProcessKiller func()

type cdpClient interface {
	Send(ctx context.Context, method string, params any) (*cdp.Message, error)
	Events() <-chan cdp.Event
	Close() error
	IsConnected() bool
}

var connectCDP = func(ctx context.Context, wsURL string, logger *slog.Logger) (cdpClient, error) {
	return cdp.Connect(ctx, wsURL, logger)
}

// Browser represents a connected browser instance.
type Browser struct {
	cdp           cdpClient
	logger        *slog.Logger
	browserURL    string    // Browser-level WebSocket URL
	pageCDP       cdpClient // Page-level CDP client
	pageTarget    *Target
	processKiller ProcessKiller // Function to kill the browser process
}

type runtimeEvalResult struct {
	Result struct {
		Value any    `json:"value"`
		Type  string `json:"type"`
	} `json:"result"`
	ExceptionDetails *struct {
		Text      string `json:"text"`
		Exception *struct {
			Description string `json:"description"`
		} `json:"exception"`
	} `json:"exceptionDetails"`
}

// New creates a new Browser from an existing CDP client.
func New(client cdpClient, logger *slog.Logger) *Browser {
	if logger == nil {
		logger = slog.Default()
	}
	return &Browser{
		cdp:    client,
		logger: logger,
	}
}

// ConnectToCDP connects to a browser's CDP WebSocket URL and returns a Browser.
func ConnectToCDP(ctx context.Context, wsURL string, logger *slog.Logger) (*Browser, error) {
	if logger == nil {
		logger = slog.Default()
	}

	b := &Browser{
		browserURL: wsURL,
		logger:     logger,
	}

	// Try to find and connect to a page target
	if err := b.connectToPage(ctx); err != nil {
		return nil, fmt.Errorf("connect to page: %w", err)
	}

	return b, nil
}

// connectToPage finds a page target and connects to it.
func (b *Browser) connectToPage(ctx context.Context) error {
	// Get the host:port from the browser URL
	host, port := extractHostPort(b.browserURL)

	// List targets
	targets, err := listTargets(host, port)
	if err != nil {
		return fmt.Errorf("list targets: %w", err)
	}

	// Find a page target
	var pageTarget *Target
	for _, t := range targets {
		if t.Type == "page" {
			pageTarget = &t
			break
		}
	}

	// If no page target, create one
	if pageTarget == nil {
		pageTarget, err = b.createTarget(ctx, host, port)
		if err != nil {
			return fmt.Errorf("create target: %w", err)
		}
	}

	// Connect to the page target
	b.pageTarget = pageTarget
	pageCDP, err := connectCDP(ctx, pageTarget.WebBrowserDebuggerURL, b.logger)
	if err != nil {
		return fmt.Errorf("connect to page target: %w", err)
	}
	b.pageCDP = pageCDP

	return nil
}

// createTarget creates a new page target.
func (b *Browser) createTarget(ctx context.Context, host string, port int) (*Target, error) {
	url := fmt.Sprintf("http://%s:%d/json/new?about:blank", host, port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("create target: %w", err)
	}
	defer resp.Body.Close()

	var target Target
	if err := json.NewDecoder(resp.Body).Decode(&target); err != nil {
		return nil, fmt.Errorf("decode target: %w", err)
	}

	return &target, nil
}

// listTargets returns the list of browser targets.
func listTargets(host string, port int) ([]Target, error) {
	url := fmt.Sprintf("http://%s:%d/json/list", host, port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var targets []Target
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}

	return targets, nil
}

// extractHostPort extracts host and port from a WebSocket URL.
func extractHostPort(wsURL string) (string, int) {
	// ws://127.0.0.1:9222/devtools/browser/xxx
	url := strings.TrimPrefix(wsURL, "ws://")
	url = strings.TrimPrefix(url, "wss://")

	parts := strings.SplitN(url, "/", 2)
	hostPort := parts[0]

	hostParts := strings.SplitN(hostPort, ":", 2)
	host := hostParts[0]
	port := 9222
	if len(hostParts) > 1 {
		fmt.Sscanf(hostParts[1], "%d", &port)
	}

	return host, port
}

// SetProcessKiller sets a function that will be called to kill the browser process.
func (b *Browser) SetProcessKiller(killer ProcessKiller) {
	b.processKiller = killer
}

// Close closes the CDP connection and kills the browser process.
func (b *Browser) Close() error {
	var err error
	if b.pageCDP != nil {
		err = b.pageCDP.Close()
	}
	if b.cdp != nil && b.cdp != b.pageCDP {
		if closeErr := b.cdp.Close(); err == nil {
			err = closeErr
		}
	}
	if b.processKiller != nil {
		b.processKiller()
	}
	return err
}

// IsConnected reports whether the CDP connection is alive.
func (b *Browser) IsConnected() bool {
	if b.pageCDP != nil {
		return b.pageCDP.IsConnected()
	}
	return b.cdp != nil && b.cdp.IsConnected()
}

// getClient returns the page-level CDP client.
func (b *Browser) getClient() cdpClient {
	if b.pageCDP != nil {
		return b.pageCDP
	}
	return b.cdp
}

func (b *Browser) eval(ctx context.Context, expression string) (*runtimeEvalResult, error) {
	client := b.getClient()
	msg, err := client.Send(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return nil, err
	}

	var result runtimeEvalResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, err
	}
	if result.ExceptionDetails != nil {
		text := result.ExceptionDetails.Text
		if result.ExceptionDetails.Exception != nil && result.ExceptionDetails.Exception.Description != "" {
			text = result.ExceptionDetails.Exception.Description
		}
		return nil, fmt.Errorf("runtime exception: %s", text)
	}
	return &result, nil
}

func resultString(result *runtimeEvalResult) string {
	if result == nil || result.Result.Value == nil {
		return ""
	}
	if s, ok := result.Result.Value.(string); ok {
		return s
	}
	return fmt.Sprint(result.Result.Value)
}

func resultBool(result *runtimeEvalResult) bool {
	if result == nil {
		return false
	}
	v, _ := result.Result.Value.(bool)
	return v
}

// Navigate navigates the browser to a URL.
func (b *Browser) Navigate(ctx context.Context, url string, opts *protocol.NavigationOptions) error {
	client := b.getClient()

	// Enable Page domain first
	_, err := client.Send(ctx, "Page.enable", nil)
	if err != nil {
		return fmt.Errorf("navigate: enable Page: %w", err)
	}

	params := map[string]any{"url": url}

	if opts != nil {
		if opts.WaitUntil != "" {
			params["waitUntil"] = opts.WaitUntil
		}
		if opts.Timeout > 0 {
			params["timeout"] = opts.Timeout
		}
	}

	msg, err := client.Send(ctx, "Page.navigate", params)
	if err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	// Check for error response in the result
	var result struct {
		ErrorText string `json:"errorText"`
	}
	if err := json.Unmarshal(msg.Result, &result); err == nil && result.ErrorText != "" {
		return fmt.Errorf("navigate: %s", result.ErrorText)
	}

	// Wait for load if requested
	if opts != nil && opts.WaitUntil != "" {
		return b.waitForLoad(ctx, opts.WaitUntil, opts.Timeout)
	}

	return nil
}

// waitForLoad waits for a specific load state.
func (b *Browser) waitForLoad(ctx context.Context, state string, timeout int) error {
	client := b.getClient()

	if timeout == 0 {
		timeout = 30000
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	switch state {
	case "load":
		_, err := client.Send(waitCtx, "Page.enable", nil)
		if err != nil {
			return err
		}
		// Wait for Page.loadEventFired
		for {
			select {
			case <-waitCtx.Done():
				return waitCtx.Err()
			case evt := <-client.Events():
				if evt.Method == "Page.loadEventFired" {
					return nil
				}
			}
		}
	case "domcontentloaded":
		_, err := client.Send(waitCtx, "Page.enable", nil)
		if err != nil {
			return err
		}
		for {
			select {
			case <-waitCtx.Done():
				return waitCtx.Err()
			case evt := <-client.Events():
				if evt.Method == "Page.domContentEventFired" {
					return nil
				}
			}
		}
	case "networkidle":
		return b.waitForNetworkIdle(waitCtx, timeout)
	default:
		return nil
	}
}

// waitForNetworkIdle waits until there are no more than 0 network connections for 500ms.
func (b *Browser) waitForNetworkIdle(ctx context.Context, timeout int) error {
	client := b.getClient()

	_, err := client.Send(ctx, "Network.enable", nil)
	if err != nil {
		return err
	}

	pending := 0
	idleTimer := time.NewTimer(500 * time.Millisecond)
	defer idleTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt := <-client.Events():
			switch evt.Method {
			case "Network.requestWillBeSent":
				pending++
				idleTimer.Reset(500 * time.Millisecond)
			case "Network.loadingFinished", "Network.loadingFailed":
				pending--
				if pending <= 0 {
					pending = 0
					idleTimer.Reset(500 * time.Millisecond)
				}
			}
		case <-idleTimer.C:
			if pending == 0 {
				return nil
			}
		}
	}
}

// Reload reloads the current page.
func (b *Browser) Reload(ctx context.Context) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Page.reload", nil)
	return err
}

// GoBack navigates back in history.
func (b *Browser) GoBack(ctx context.Context) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Page.goBack", nil)
	return err
}

// GoForward navigates forward in history.
func (b *Browser) GoForward(ctx context.Context) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Page.goForward", nil)
	return err
}

// GetURL returns the current page URL.
func (b *Browser) GetURL(ctx context.Context) (string, error) {
	result, err := b.eval(ctx, "window.location.href")
	if err != nil {
		return "", err
	}
	return resultString(result), nil
}

// GetTitle returns the current page title.
func (b *Browser) GetTitle(ctx context.Context) (string, error) {
	result, err := b.eval(ctx, "document.title")
	if err != nil {
		return "", err
	}
	return resultString(result), nil
}

// Click clicks an element matching the selector.
func (b *Browser) Click(ctx context.Context, selector string, opts *protocol.ClickOptions) error {
	// First, find the element center coordinates
	x, y, err := b.resolveElementCenter(ctx, selector)
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}

	button := "left"
	clickCount := 1
	if opts != nil {
		if opts.Button != "" {
			button = opts.Button
		}
		if opts.ClickCount > 0 {
			clickCount = opts.ClickCount
		}
	}

	client := b.getClient()

	// Dispatch mousePressed
	_, err = client.Send(ctx, "Input.dispatchMouseEvent", map[string]any{
		"type":       "mousePressed",
		"x":          x,
		"y":          y,
		"button":     button,
		"clickCount": clickCount,
	})
	if err != nil {
		return fmt.Errorf("click press: %w", err)
	}

	if opts != nil && opts.Delay > 0 {
		time.Sleep(time.Duration(opts.Delay) * time.Millisecond)
	}

	// Dispatch mouseReleased
	_, err = client.Send(ctx, "Input.dispatchMouseEvent", map[string]any{
		"type":       "mouseReleased",
		"x":          x,
		"y":          y,
		"button":     button,
		"clickCount": clickCount,
	})
	if err != nil {
		return fmt.Errorf("click release: %w", err)
	}

	return nil
}

// DoubleClick double-clicks an element.
func (b *Browser) DoubleClick(ctx context.Context, selector string) error {
	return b.Click(ctx, selector, &protocol.ClickOptions{ClickCount: 2})
}

// Fill fills an input element with a value.
func (b *Browser) Fill(ctx context.Context, selector string, value string) error {
	// Focus the element
	_, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				el.focus();
				return true;
			})()
		`, selector, selector))
	if err != nil {
		return fmt.Errorf("fill focus: %w", err)
	}

	client := b.getClient()
	// Select all existing text
	_, err = client.Send(ctx, "Input.dispatchKeyEvent", map[string]any{
		"type":     "keyDown",
		"key":      "a",
		"code":     "KeyA",
		"commands": []string{"selectAll"},
	})
	if err != nil {
		return fmt.Errorf("fill select all: %w", err)
	}

	// Type the new value
	_, err = client.Send(ctx, "Input.insertText", map[string]any{
		"text": value,
	})
	if err != nil {
		return fmt.Errorf("fill insert: %w", err)
	}

	return nil
}

// Type types text character by character.
func (b *Browser) Type(ctx context.Context, selector string, text string, delay int) error {
	// Focus the element
	_, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				el.focus();
				return true;
			})()
		`, selector, selector))
	if err != nil {
		return fmt.Errorf("type focus: %w", err)
	}

	client := b.getClient()
	if delay == 0 {
		delay = 50
	}

	for _, ch := range text {
		_, err = client.Send(ctx, "Input.dispatchKeyEvent", map[string]any{
			"type": "keyDown",
			"text": string(ch),
		})
		if err != nil {
			return fmt.Errorf("type keydown: %w", err)
		}

		_, err = client.Send(ctx, "Input.dispatchKeyEvent", map[string]any{
			"type": "keyUp",
			"text": string(ch),
		})
		if err != nil {
			return fmt.Errorf("type keyup: %w", err)
		}

		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}

	return nil
}

// Press presses a keyboard key.
func (b *Browser) Press(ctx context.Context, key string) error {
	client := b.getClient()

	_, err := client.Send(ctx, "Input.dispatchKeyEvent", map[string]any{
		"type": "keyDown",
		"key":  key,
	})
	if err != nil {
		return err
	}

	_, err = client.Send(ctx, "Input.dispatchKeyEvent", map[string]any{
		"type": "keyUp",
		"key":  key,
	})
	return err
}

// Hover moves the mouse over an element.
func (b *Browser) Hover(ctx context.Context, selector string) error {
	x, y, err := b.resolveElementCenter(ctx, selector)
	if err != nil {
		return fmt.Errorf("hover: %w", err)
	}

	client := b.getClient()
	_, err = client.Send(ctx, "Input.dispatchMouseEvent", map[string]any{
		"type": "mouseMoved",
		"x":    x,
		"y":    y,
	})
	return err
}

// Scroll scrolls the page.
func (b *Browser) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Input.dispatchMouseEvent", map[string]any{
		"type":   "mouseWheel",
		"x":      0,
		"y":      0,
		"deltaX": deltaX,
		"deltaY": deltaY,
	})
	return err
}

// Focus focuses an element.
func (b *Browser) Focus(ctx context.Context, selector string) error {
	_, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				el.focus();
				return true;
			})()
		`, selector, selector))
	return err
}

// Check checks a checkbox element.
func (b *Browser) Check(ctx context.Context, selector string) error {
	_, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				if (!el.checked) el.click();
				return true;
			})()
		`, selector, selector))
	return err
}

// Uncheck unchecks a checkbox element.
func (b *Browser) Uncheck(ctx context.Context, selector string) error {
	_, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				if (el.checked) el.click();
				return true;
			})()
		`, selector, selector))
	return err
}

// Select selects an option in a <select> element.
func (b *Browser) Select(ctx context.Context, selector string, value string) error {
	_, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				el.value = %q;
				el.dispatchEvent(new Event('change', { bubbles: true }));
				return true;
			})()
		`, selector, selector, value))
	return err
}

// Eval evaluates a JavaScript expression and returns the result.
func (b *Browser) Eval(ctx context.Context, expression string) (any, error) {
	result, err := b.eval(ctx, expression)
	if err != nil {
		return nil, fmt.Errorf("eval: %w", err)
	}
	return result.Result.Value, nil
}

// GetText returns the text content of an element.
func (b *Browser) GetText(ctx context.Context, selector string) (string, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				return el.innerText || el.textContent || '';
			})()
		`, selector, selector))
	if err != nil {
		return "", err
	}
	return resultString(result), nil
}

// GetHTML returns the HTML content of an element or the whole page.
func (b *Browser) GetHTML(ctx context.Context, selector string) (string, error) {
	expr := "document.documentElement.outerHTML"
	if selector != "" {
		expr = fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				return el.outerHTML;
			})()
		`, selector, selector)
	}

	result, err := b.eval(ctx, expr)
	if err != nil {
		return "", err
	}
	return resultString(result), nil
}

// GetValue returns the value of an input element.
func (b *Browser) GetValue(ctx context.Context, selector string) (string, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				return el.value || '';
			})()
		`, selector, selector))
	if err != nil {
		return "", err
	}
	return resultString(result), nil
}

// GetAttr returns an attribute value of an element.
func (b *Browser) GetAttr(ctx context.Context, selector, attr string) (string, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				return el.getAttribute(%q) || '';
			})()
		`, selector, selector, attr))
	if err != nil {
		return "", err
	}
	return resultString(result), nil
}

// IsVisible checks if an element is visible.
func (b *Browser) IsVisible(ctx context.Context, selector string) (bool, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) return false;
				var rect = el.getBoundingClientRect();
				var style = window.getComputedStyle(el);
				return rect.width > 0 && rect.height > 0 && 
					   style.visibility !== 'hidden' && style.display !== 'none';
			})()
		`, selector))
	if err != nil {
		return false, err
	}
	return resultBool(result), nil
}

// IsEnabled checks if an element is enabled.
func (b *Browser) IsEnabled(ctx context.Context, selector string) (bool, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) return false;
				return !el.disabled;
			})()
		`, selector))
	if err != nil {
		return false, err
	}
	return resultBool(result), nil
}

// IsChecked checks if a checkbox is checked.
func (b *Browser) IsChecked(ctx context.Context, selector string) (bool, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) return false;
				return !!el.checked;
			})()
		`, selector))
	if err != nil {
		return false, err
	}
	return resultBool(result), nil
}

// Screenshot captures a screenshot of the page.
func (b *Browser) Screenshot(ctx context.Context, opts *protocol.ScreenshotOptions) ([]byte, error) {
	client := b.getClient()
	params := map[string]any{}

	if opts != nil {
		if opts.Format != "" {
			params["format"] = opts.Format
		} else {
			params["format"] = "png"
		}
		if opts.Quality > 0 {
			params["quality"] = opts.Quality
		}
		if opts.FullPage {
			params["captureBeyondViewport"] = true
		}
		if opts.Selector != "" {
			x, y, width, height, err := b.resolveElementBox(ctx, opts.Selector)
			if err != nil {
				return nil, err
			}
			params["clip"] = map[string]any{
				"x":      x,
				"y":      y,
				"width":  width,
				"height": height,
				"scale":  1,
			}
		}
		if opts.ClipWidth > 0 && opts.ClipHeight > 0 {
			params["clip"] = map[string]any{
				"x":      opts.ClipX,
				"y":      opts.ClipY,
				"width":  opts.ClipWidth,
				"height": opts.ClipHeight,
				"scale":  1,
			}
		}
	} else {
		params["format"] = "png"
	}

	msg, err := client.Send(ctx, "Page.captureScreenshot", params)
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, err
	}

	data, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return nil, fmt.Errorf("screenshot decode: %w", err)
	}

	return data, nil
}

// Snapshot captures an accessibility tree snapshot of the page.
func (b *Browser) Snapshot(ctx context.Context, opts *protocol.SnapshotOptions) (string, error) {
	client := b.getClient()

	// Enable DOM and Accessibility
	_, err := client.Send(ctx, "DOM.enable", nil)
	if err != nil {
		return "", fmt.Errorf("snapshot: enable DOM: %w", err)
	}

	_, err = client.Send(ctx, "Accessibility.enable", nil)
	if err != nil {
		return "", fmt.Errorf("snapshot: enable Accessibility: %w", err)
	}

	// Get the full accessibility tree
	msg, err := client.Send(ctx, "Accessibility.getFullAXTree", nil)
	if err != nil {
		return "", fmt.Errorf("snapshot: get AX tree: %w", err)
	}

	var result struct {
		Nodes []AXNode `json:"nodes"`
	}
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return "", err
	}

	// Build and format the tree
	return formatAXTree(result.Nodes, opts), nil
}

// AXNode represents an accessibility tree node.
type AXNode struct {
	NodeID           string       `json:"nodeId"`
	Ignored          bool         `json:"ignored"`
	IgnoredReasons   []any        `json:"ignoredReasons"`
	Role             *AXValue     `json:"role"`
	Name             *AXValue     `json:"name"`
	Description      *AXValue     `json:"description"`
	Value            *AXValue     `json:"value"`
	Properties       []AXProperty `json:"properties"`
	ChildIDs         []string     `json:"childIds"`
	BackendDOMNodeID int          `json:"backendDOMNodeId"`
}

type AXValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type AXProperty struct {
	Name  string  `json:"name"`
	Value AXValue `json:"value"`
}

// formatAXTree formats the accessibility tree into a readable string.
func formatAXTree(nodes []AXNode, opts *protocol.SnapshotOptions) string {
	if len(nodes) == 0 {
		return "(empty page)"
	}

	nodeMap := make(map[string]*AXNode)
	for i := range nodes {
		nodeMap[nodes[i].NodeID] = &nodes[i]
	}

	var sb strings.Builder
	refCounter := 0

	var walk func(nodeID string, depth int)
	walk = func(nodeID string, depth int) {
		node, ok := nodeMap[nodeID]
		if !ok || node.Ignored {
			return
		}

		role := ""
		if node.Role != nil {
			role = fmt.Sprint(node.Role.Value)
		}

		name := ""
		if node.Name != nil {
			name = fmt.Sprint(node.Name.Value)
		}

		// Skip if interactive-only mode and not interactive
		if opts != nil && opts.Interactive {
			interactiveRoles := map[string]bool{
				"button": true, "link": true, "textbox": true, "checkbox": true,
				"radio": true, "combobox": true, "listbox": true, "menuitem": true,
				"switch": true, "tab": true, "slider": true, "spinbutton": true,
				"searchbox": true,
			}
			if !interactiveRoles[role] {
				// Still walk children
				for _, childID := range node.ChildIDs {
					walk(childID, depth)
				}
				return
			}
		}

		indent := strings.Repeat("  ", depth)

		// Generate ref ID for interactive elements
		refID := ""
		interactiveRoles := map[string]bool{
			"button": true, "link": true, "textbox": true, "checkbox": true,
			"radio": true, "combobox": true, "listbox": true, "menuitem": true,
			"switch": true, "tab": true,
		}
		if interactiveRoles[role] {
			refCounter++
			refID = fmt.Sprintf("[%d]", refCounter)
		}

		// Format the line
		line := indent
		if refID != "" {
			line += refID + " "
		}
		line += role
		if name != "" {
			line += " " + name
		}

		// Add state info
		for _, prop := range node.Properties {
			switch prop.Name {
			case "checked":
				if v := fmt.Sprint(prop.Value.Value); v != "" {
					line += " [" + v + "]"
				}
			case "expanded":
				if v := fmt.Sprint(prop.Value.Value); v != "" {
					line += " [expanded:" + v + "]"
				}
			case "disabled":
				if fmt.Sprint(prop.Value.Value) == "true" {
					line += " [disabled]"
				}
			}
		}

		sb.WriteString(line + "\n")

		// Walk children
		for _, childID := range node.ChildIDs {
			walk(childID, depth+1)
		}
	}

	// Find root nodes (those not referenced as children)
	childSet := make(map[string]bool)
	for _, node := range nodes {
		for _, childID := range node.ChildIDs {
			childSet[childID] = true
		}
	}

	for _, node := range nodes {
		if !childSet[node.NodeID] {
			walk(node.NodeID, 0)
		}
	}

	return sb.String()
}

// resolveElementCenter returns the center coordinates of an element.
func (b *Browser) resolveElementCenter(ctx context.Context, selector string) (float64, float64, error) {
	x, y, width, height, err := b.resolveElementBox(ctx, selector)
	if err != nil {
		return 0, 0, err
	}
	return x + width/2, y + height/2, nil
}

// resolveElementBox returns the page-space bounding box of an element.
func (b *Browser) resolveElementBox(ctx context.Context, selector string) (float64, float64, float64, float64, error) {
	result, err := b.eval(ctx, fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) throw new Error('Element not found: %s');
				var rect = el.getBoundingClientRect();
				return {
					x: rect.x + window.scrollX,
					y: rect.y + window.scrollY,
					width: rect.width,
					height: rect.height
				};
			})()
		`, selector, selector))
	if err != nil {
		return 0, 0, 0, 0, err
	}

	var box struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	data, err := json.Marshal(result.Result.Value)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if err := json.Unmarshal(data, &box); err != nil {
		return 0, 0, 0, 0, err
	}
	if box.Width <= 0 || box.Height <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("element has empty bounding box: %s", selector)
	}
	return box.X, box.Y, box.Width, box.Height, nil
}

// resolveNodeID returns the DOM node ID for a selector.
func (b *Browser) resolveNodeID(ctx context.Context, selector string) (int, error) {
	client := b.getClient()

	// First get the document
	docMsg, err := client.Send(ctx, "DOM.getDocument", nil)
	if err != nil {
		return 0, err
	}

	var docResult struct {
		Root struct {
			NodeID int `json:"nodeId"`
		} `json:"root"`
	}
	if err := json.Unmarshal(docMsg.Result, &docResult); err != nil {
		return 0, err
	}

	// Query selector
	nodeMsg, err := client.Send(ctx, "DOM.querySelector", map[string]any{
		"nodeId":   docResult.Root.NodeID,
		"selector": selector,
	})
	if err != nil {
		return 0, err
	}

	var nodeResult struct {
		NodeID int `json:"nodeId"`
	}
	if err := json.Unmarshal(nodeMsg.Result, &nodeResult); err != nil {
		return 0, err
	}

	if nodeResult.NodeID == 0 {
		return 0, fmt.Errorf("element not found: %s", selector)
	}
	return nodeResult.NodeID, nil
}

// SetViewport sets the browser viewport size.
func (b *Browser) SetViewport(ctx context.Context, width, height int, deviceScaleFactor float64) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width":             width,
		"height":            height,
		"deviceScaleFactor": deviceScaleFactor,
		"mobile":            false,
	})
	return err
}

// SetGeolocation sets the geolocation.
func (b *Browser) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Emulation.setGeolocationOverride", map[string]any{
		"latitude":  lat,
		"longitude": lng,
		"accuracy":  accuracy,
	})
	return err
}

// SetOffline enables or disables offline mode.
func (b *Browser) SetOffline(ctx context.Context, offline bool) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Network.emulateNetworkConditions", map[string]any{
		"offline":            offline,
		"latency":            0,
		"downloadThroughput": -1,
		"uploadThroughput":   -1,
	})
	return err
}

// SetHeaders sets extra HTTP headers.
func (b *Browser) SetHeaders(ctx context.Context, headers map[string]string) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Network.setExtraHTTPHeaders", map[string]any{
		"headers": headers,
	})
	return err
}

// GetCookies returns all cookies.
func (b *Browser) GetCookies(ctx context.Context) ([]protocol.Cookie, error) {
	client := b.getClient()
	msg, err := client.Send(ctx, "Network.getCookies", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Cookies []protocol.Cookie `json:"cookies"`
	}
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, err
	}

	return result.Cookies, nil
}

// SetCookie sets a cookie.
func (b *Browser) SetCookie(ctx context.Context, cookie protocol.Cookie) error {
	client := b.getClient()
	params := map[string]any{
		"name":  cookie.Name,
		"value": cookie.Value,
	}
	if cookie.Domain != "" {
		params["domain"] = cookie.Domain
	}
	if cookie.Path != "" {
		params["path"] = cookie.Path
	}
	if cookie.Expires > 0 {
		params["expires"] = cookie.Expires
	}
	if cookie.HTTPOnly {
		params["httpOnly"] = true
	}
	if cookie.Secure {
		params["secure"] = true
	}
	if cookie.SameSite != "" {
		params["sameSite"] = cookie.SameSite
	}

	_, err := client.Send(ctx, "Network.setCookie", params)
	return err
}

// ClearCookies clears all cookies.
func (b *Browser) ClearCookies(ctx context.Context) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Network.clearBrowserCookies", nil)
	return err
}

// WaitMS waits for a specified number of milliseconds.
func (b *Browser) WaitMS(ctx context.Context, ms int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return nil
	}
}

// WaitForSelector waits for an element matching the selector to appear.
func (b *Browser) WaitForSelector(ctx context.Context, selector string, timeout int) error {
	if timeout == 0 {
		timeout = 30000
	}

	deadline := time.After(time.Duration(timeout) * time.Millisecond)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("waitForSelector: timeout waiting for %s", selector)
		case <-ticker.C:
			client := b.getClient()
			msg, err := client.Send(ctx, "Runtime.evaluate", map[string]any{
				"expression": fmt.Sprintf(`
					(function() {
						var el = document.querySelector(%q);
						return el !== null;
					})()
				`, selector),
				"returnByValue": true,
			})
			if err != nil {
				continue
			}

			var result struct {
				Result struct {
					Value bool `json:"value"`
				} `json:"result"`
			}
			if err := json.Unmarshal(msg.Result, &result); err != nil {
				continue
			}

			if result.Result.Value {
				return nil
			}
		}
	}
}

// WaitForText waits for text to appear on the page.
func (b *Browser) WaitForText(ctx context.Context, text string, timeout int) error {
	if timeout == 0 {
		timeout = 30000
	}

	deadline := time.After(time.Duration(timeout) * time.Millisecond)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("waitForText: timeout waiting for %q", text)
		case <-ticker.C:
			client := b.getClient()
			msg, err := client.Send(ctx, "Runtime.evaluate", map[string]any{
				"expression":    "document.body.innerText",
				"returnByValue": true,
			})
			if err != nil {
				continue
			}

			var result struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			}
			if err := json.Unmarshal(msg.Result, &result); err != nil {
				continue
			}

			if strings.Contains(result.Result.Value, text) {
				return nil
			}
		}
	}
}

// WaitForURL waits for the page URL to match.
func (b *Browser) WaitForURL(ctx context.Context, urlPattern string, timeout int) error {
	if timeout == 0 {
		timeout = 30000
	}

	deadline := time.After(time.Duration(timeout) * time.Millisecond)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("waitForURL: timeout waiting for %s", urlPattern)
		case <-ticker.C:
			currentURL, err := b.GetURL(ctx)
			if err != nil {
				continue
			}
			if strings.Contains(currentURL, urlPattern) {
				return nil
			}
		}
	}
}

// NewTab opens a new browser tab.
func (b *Browser) NewTab(ctx context.Context, url string) (string, error) {
	client := b.getClient()
	msg, err := client.Send(ctx, "Target.createTarget", map[string]any{
		"url": url,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return "", err
	}

	return result.TargetID, nil
}

// CloseTab closes a browser tab.
func (b *Browser) CloseTab(ctx context.Context, targetID string) error {
	client := b.getClient()
	_, err := client.Send(ctx, "Target.closeTarget", map[string]any{
		"targetId": targetID,
	})
	return err
}

// CDPClient returns the underlying CDP client for advanced usage.
func (b *Browser) CDPClient() *cdp.Client {
	client, _ := b.getClient().(*cdp.Client)
	return client
}
