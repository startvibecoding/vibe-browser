// Package client provides the high-level Go SDK for vibe-browser.
//
// It supports two modes:
//   - Direct mode: connects directly to a browser via CDP
//   - Daemon mode: communicates with a vibe-browser daemon process
//
// Usage:
//
//	import "github.com/startvibecoding/vibe-browser/pkg/client"
//
//	// Direct mode
//	b, err := client.Open(ctx, &client.Options{
//	    CDPURL: "ws://127.0.0.1:9222/devtools/browser/...",
//	})
//
//	// Daemon mode
//	b, err := client.Connect(ctx, &client.Options{
//	    Session: "my-session",
//	})
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/startvibecoding/vibe-browser/internal/chrome"
	"github.com/startvibecoding/vibe-browser/pkg/browser"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
)

// Options configures the client behavior.
type Options struct {
	// Session name for daemon mode. If empty, uses "default".
	Session string

	// CDPURL is the Chrome DevTools Protocol WebSocket URL.
	// If set, connects directly to this browser.
	CDPURL string

	// CDPHost and CDPPort discover CDP from a running Chrome instance.
	CDPHost string
	CDPPort int

	// Launch options (used when starting a new browser).
	Launch *protocol.LaunchOptions

	// Browser type to launch (chrome, chromium, brave, edge, chrome-canary).
	Browser protocol.BrowserType

	// Headless mode for launched browsers (default true).
	// Use Launch.Headless when you need to explicitly request a headed browser.
	Headless bool

	// ExecutablePath is the path to the Chrome executable.
	ExecutablePath string

	// Logger for SDK operations. Defaults to slog.Default().
	Logger *slog.Logger

	// DaemonSocketDir overrides the default daemon socket directory.
	DaemonSocketDir string
}

// Client is the main SDK entry point.
type Client struct {
	browser    *browser.Browser
	daemon     bool
	session    string
	socketPath string
	logger     *slog.Logger
	mu         sync.Mutex
}

type chromeProcess interface {
	Kill()
	CDPWebSocketURL() string
}

var (
	dialDaemonTimeout = net.DialTimeout
	dialDaemonContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, address)
	}
	checkDaemonRunning = isDaemonRunning
	connectToCDP       = browser.ConnectToCDP
	discoverCDPURL     = chrome.DiscoverCDPURL
	launchChrome       = launchChromeDefault
)

func launchChromeDefault(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (chromeProcess, error) {
	return chrome.Launch(ctx, opts, logger)
}

// Open connects directly to a browser via CDP or launches one.
func Open(ctx context.Context, opts *Options) (*Client, error) {
	if opts == nil {
		opts = &Options{}
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// If CDPURL is provided, connect directly
	if opts.CDPURL != "" {
		b, err := connectToCDP(ctx, opts.CDPURL, logger)
		if err != nil {
			return nil, fmt.Errorf("client: connect to CDP: %w", err)
		}
		return &Client{
			browser: b,
			logger:  logger,
		}, nil
	}

	// If CDPHost/Port is provided, discover CDP URL
	if opts.CDPPort > 0 {
		host := opts.CDPHost
		if host == "" {
			host = "127.0.0.1"
		}
		cdpURL, err := discoverCDPURL(host, opts.CDPPort)
		if err != nil {
			return nil, fmt.Errorf("client: discover CDP: %w", err)
		}
		b, err := connectToCDP(ctx, cdpURL, logger)
		if err != nil {
			return nil, fmt.Errorf("client: connect to CDP: %w", err)
		}
		return &Client{
			browser: b,
			logger:  logger,
		}, nil
	}

	// Launch a new browser
	launchOpts := chrome.LaunchOptions{
		Browser:        chrome.BrowserType(opts.Browser),
		Headless:       opts.Headless,
		ExecutablePath: opts.ExecutablePath,
	}

	if opts.Launch != nil {
		launchOpts.Browser = chrome.BrowserType(opts.Launch.Browser)
		launchOpts.Headless = opts.Launch.Headless
		launchOpts.ExecutablePath = opts.Launch.ExecutablePath
		launchOpts.Args = opts.Launch.Args
		launchOpts.Proxy = opts.Launch.Proxy
		launchOpts.UserDataDir = opts.Launch.UserDataDir
		launchOpts.ViewportWidth = opts.Launch.ViewportWidth
		launchOpts.ViewportHeight = opts.Launch.ViewportHeight
		launchOpts.Extensions = opts.Launch.Extensions
		launchOpts.Profile = opts.Launch.Profile
	}

	if launchOpts.ExecutablePath == "" && opts.ExecutablePath != "" {
		launchOpts.ExecutablePath = opts.ExecutablePath
	}
	if launchOpts.Browser == "" && opts.Browser != "" {
		launchOpts.Browser = chrome.BrowserType(opts.Browser)
	}

	if opts.Launch == nil && !launchOpts.Headless && !opts.Headless {
		launchOpts.Headless = true // default to headless
	}

	proc, err := launchChrome(ctx, launchOpts, logger)
	if err != nil {
		return nil, fmt.Errorf("client: launch chrome: %w", err)
	}

	b, err := connectToCDP(ctx, proc.CDPWebSocketURL(), logger)
	if err != nil {
		proc.Kill()
		return nil, fmt.Errorf("client: connect to launched browser: %w", err)
	}
	b.SetProcessKiller(proc.Kill)

	return &Client{
		browser: b,
		logger:  logger,
	}, nil
}

// Connect connects to a daemon session.
func Connect(ctx context.Context, opts *Options) (*Client, error) {
	if opts == nil {
		opts = &Options{}
	}

	session := opts.Session
	if session == "" {
		session = "default"
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	socketDir := opts.DaemonSocketDir
	if socketDir == "" {
		socketDir = getSocketDir()
	}

	socketPath := filepath.Join(socketDir, session+".sock")

	// Check if daemon is running
	if !checkDaemonRunning(socketPath, session, socketDir) {
		return nil, fmt.Errorf("client: daemon not running for session %q; start it with: vibe-browser daemon --session %s", session, session)
	}

	// Verify the daemon is reachable.
	conn, err := dialDaemonTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("client: connect to daemon: %w", err)
	}
	conn.Close()

	return &Client{
		daemon:     true,
		session:    session,
		socketPath: socketPath,
		logger:     logger,
	}, nil
}

// Browser returns the underlying browser.Browser for direct CDP operations.
func (c *Client) Browser() *browser.Browser {
	return c.browser
}

// Navigate navigates to a URL.
func (c *Client) Navigate(ctx context.Context, url string) error {
	if c.daemon {
		return c.daemonExec(ctx, "navigate", map[string]any{"url": url})
	}
	return c.browser.Navigate(ctx, url, nil)
}

// NavigateWith waits for a specific load state after navigation.
func (c *Client) NavigateWith(ctx context.Context, url string, waitUntil string) error {
	if c.daemon {
		return c.daemonExec(ctx, "navigate", map[string]any{"url": url, "waitUntil": waitUntil})
	}
	return c.browser.Navigate(ctx, url, &protocol.NavigationOptions{
		WaitUntil: waitUntil,
	})
}

// URL returns the current page URL.
func (c *Client) URL(ctx context.Context) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "get_url", nil)
	}
	return c.browser.GetURL(ctx)
}

// Title returns the current page title.
func (c *Client) Title(ctx context.Context) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "get_title", nil)
	}
	return c.browser.GetTitle(ctx)
}

// Click clicks an element.
func (c *Client) Click(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "click", map[string]any{"selector": selector})
	}
	return c.browser.Click(ctx, selector, nil)
}

// DoubleClick double-clicks an element.
func (c *Client) DoubleClick(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "dblclick", map[string]any{"selector": selector})
	}
	return c.browser.DoubleClick(ctx, selector)
}

// Fill fills an input element.
func (c *Client) Fill(ctx context.Context, selector string, value string) error {
	if c.daemon {
		return c.daemonExec(ctx, "fill", map[string]any{"selector": selector, "value": value})
	}
	return c.browser.Fill(ctx, selector, value)
}

// Type types text into an element.
func (c *Client) Type(ctx context.Context, selector string, text string) error {
	if c.daemon {
		return c.daemonExec(ctx, "type", map[string]any{"selector": selector, "text": text, "delay": 50})
	}
	return c.browser.Type(ctx, selector, text, 50)
}

// Press presses a keyboard key.
func (c *Client) Press(ctx context.Context, key string) error {
	if c.daemon {
		return c.daemonExec(ctx, "press", map[string]any{"key": key})
	}
	return c.browser.Press(ctx, key)
}

// Hover moves the mouse over an element.
func (c *Client) Hover(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "hover", map[string]any{"selector": selector})
	}
	return c.browser.Hover(ctx, selector)
}

// Scroll scrolls the page.
func (c *Client) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	if c.daemon {
		return c.daemonExec(ctx, "scroll", map[string]any{"deltaX": deltaX, "deltaY": deltaY})
	}
	return c.browser.Scroll(ctx, deltaX, deltaY)
}

// Focus focuses an element.
func (c *Client) Focus(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "focus", map[string]any{"selector": selector})
	}
	return c.browser.Focus(ctx, selector)
}

// Check checks a checkbox.
func (c *Client) Check(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "check", map[string]any{"selector": selector})
	}
	return c.browser.Check(ctx, selector)
}

// Uncheck unchecks a checkbox.
func (c *Client) Uncheck(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "uncheck", map[string]any{"selector": selector})
	}
	return c.browser.Uncheck(ctx, selector)
}

// Select selects a value in a select element.
func (c *Client) Select(ctx context.Context, selector string, value string) error {
	if c.daemon {
		return c.daemonExec(ctx, "select", map[string]any{"selector": selector, "value": value})
	}
	return c.browser.Select(ctx, selector, value)
}

// Eval evaluates JavaScript.
func (c *Client) Eval(ctx context.Context, expression string) (any, error) {
	if c.daemon {
		return daemonValue[any](ctx, c, "eval", map[string]any{"expression": expression})
	}
	return c.browser.Eval(ctx, expression)
}

// GetText returns the text content of an element.
func (c *Client) GetText(ctx context.Context, selector string) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "get_text", map[string]any{"selector": selector})
	}
	return c.browser.GetText(ctx, selector)
}

// GetHTML returns the HTML content.
func (c *Client) GetHTML(ctx context.Context, selector string) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "get_html", map[string]any{"selector": selector})
	}
	return c.browser.GetHTML(ctx, selector)
}

// GetValue returns the value of an input element.
func (c *Client) GetValue(ctx context.Context, selector string) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "get_value", map[string]any{"selector": selector})
	}
	return c.browser.GetValue(ctx, selector)
}

// GetAttr returns an attribute value.
func (c *Client) GetAttr(ctx context.Context, selector, attr string) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "get_attr", map[string]any{"selector": selector, "attr": attr})
	}
	return c.browser.GetAttr(ctx, selector, attr)
}

// IsVisible checks if an element is visible.
func (c *Client) IsVisible(ctx context.Context, selector string) (bool, error) {
	if c.daemon {
		return daemonValue[bool](ctx, c, "is_visible", map[string]any{"selector": selector})
	}
	return c.browser.IsVisible(ctx, selector)
}

// IsEnabled checks if an element is enabled.
func (c *Client) IsEnabled(ctx context.Context, selector string) (bool, error) {
	if c.daemon {
		return daemonValue[bool](ctx, c, "is_enabled", map[string]any{"selector": selector})
	}
	return c.browser.IsEnabled(ctx, selector)
}

// IsChecked checks if a checkbox is checked.
func (c *Client) IsChecked(ctx context.Context, selector string) (bool, error) {
	if c.daemon {
		return daemonValue[bool](ctx, c, "is_checked", map[string]any{"selector": selector})
	}
	return c.browser.IsChecked(ctx, selector)
}

// Snapshot captures an accessibility tree snapshot.
func (c *Client) Snapshot(ctx context.Context) (string, error) {
	if c.daemon {
		return daemonValue[string](ctx, c, "snapshot", nil)
	}
	return c.browser.Snapshot(ctx, nil)
}

// SnapshotWithOptions captures a snapshot with options.
func (c *Client) SnapshotWithOptions(ctx context.Context, opts *protocol.SnapshotOptions) (string, error) {
	if c.daemon {
		extra := map[string]any{}
		if opts != nil {
			extra["interactive"] = opts.Interactive
			extra["compact"] = opts.Compact
			extra["selector"] = opts.Selector
			extra["depth"] = opts.Depth
			extra["urls"] = opts.URLs
		}
		return daemonValue[string](ctx, c, "snapshot", extra)
	}
	return c.browser.Snapshot(ctx, opts)
}

// Screenshot captures a screenshot.
func (c *Client) Screenshot(ctx context.Context) ([]byte, error) {
	if c.daemon {
		return daemonScreenshot(ctx, c, nil)
	}
	return c.browser.Screenshot(ctx, nil)
}

// ScreenshotWithOptions captures a screenshot with options.
func (c *Client) ScreenshotWithOptions(ctx context.Context, opts *protocol.ScreenshotOptions) ([]byte, error) {
	if c.daemon {
		return daemonScreenshot(ctx, c, opts)
	}
	return c.browser.Screenshot(ctx, opts)
}

// Reload reloads the page.
func (c *Client) Reload(ctx context.Context) error {
	if c.daemon {
		return c.daemonExec(ctx, "reload", nil)
	}
	return c.browser.Reload(ctx)
}

// Back navigates back.
func (c *Client) Back(ctx context.Context) error {
	if c.daemon {
		return c.daemonExec(ctx, "back", nil)
	}
	return c.browser.GoBack(ctx)
}

// Forward navigates forward.
func (c *Client) Forward(ctx context.Context) error {
	if c.daemon {
		return c.daemonExec(ctx, "forward", nil)
	}
	return c.browser.GoForward(ctx)
}

// WaitMS waits for a duration.
func (c *Client) WaitMS(ctx context.Context, ms int) error {
	if c.daemon {
		return c.daemonExec(ctx, "wait_ms", map[string]any{"ms": ms})
	}
	return c.browser.WaitMS(ctx, ms)
}

// WaitForSelector waits for an element to appear.
func (c *Client) WaitForSelector(ctx context.Context, selector string) error {
	if c.daemon {
		return c.daemonExec(ctx, "wait_for_selector", map[string]any{"selector": selector})
	}
	return c.browser.WaitForSelector(ctx, selector, 0)
}

// WaitForText waits for text to appear.
func (c *Client) WaitForText(ctx context.Context, text string) error {
	if c.daemon {
		return c.daemonExec(ctx, "wait_for_text", map[string]any{"text": text})
	}
	return c.browser.WaitForText(ctx, text, 0)
}

// WaitForURL waits for the URL to match.
func (c *Client) WaitForURL(ctx context.Context, urlPattern string) error {
	if c.daemon {
		return c.daemonExec(ctx, "wait_for_url", map[string]any{"url": urlPattern})
	}
	return c.browser.WaitForURL(ctx, urlPattern, 0)
}

// SetViewport sets the viewport size.
func (c *Client) SetViewport(ctx context.Context, width, height int) error {
	if c.daemon {
		return c.daemonExec(ctx, "set_viewport", map[string]any{"width": width, "height": height})
	}
	return c.browser.SetViewport(ctx, width, height, 1.0)
}

// SetGeolocation sets the geolocation.
func (c *Client) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	if c.daemon {
		return c.daemonExec(ctx, "set_geolocation", map[string]any{"latitude": lat, "longitude": lng, "accuracy": accuracy})
	}
	return c.browser.SetGeolocation(ctx, lat, lng, accuracy)
}

// SetOffline enables/disables offline mode.
func (c *Client) SetOffline(ctx context.Context, offline bool) error {
	if c.daemon {
		return c.daemonExec(ctx, "set_offline", map[string]any{"offline": offline})
	}
	return c.browser.SetOffline(ctx, offline)
}

// SetHeaders sets extra HTTP headers.
func (c *Client) SetHeaders(ctx context.Context, headers map[string]string) error {
	if c.daemon {
		return c.daemonExec(ctx, "set_headers", map[string]any{"headers": headers})
	}
	return c.browser.SetHeaders(ctx, headers)
}

// GetCookies returns all cookies.
func (c *Client) GetCookies(ctx context.Context) ([]protocol.Cookie, error) {
	if c.daemon {
		return daemonValue[[]protocol.Cookie](ctx, c, "cookies_get", nil)
	}
	return c.browser.GetCookies(ctx)
}

// SetCookie sets a cookie.
func (c *Client) SetCookie(ctx context.Context, cookie protocol.Cookie) error {
	if c.daemon {
		return c.daemonExec(ctx, "cookies_set", map[string]any{"cookie": cookie})
	}
	return c.browser.SetCookie(ctx, cookie)
}

// ClearCookies clears all cookies.
func (c *Client) ClearCookies(ctx context.Context) error {
	if c.daemon {
		return c.daemonExec(ctx, "cookies_clear", nil)
	}
	return c.browser.ClearCookies(ctx)
}

// NewTab opens a new tab.
func (c *Client) NewTab(ctx context.Context, url string) (string, error) {
	if c.daemon {
		var result struct {
			TargetID string `json:"targetId"`
		}
		if err := c.daemonCallInto(ctx, "tab_new", map[string]any{"url": url}, &result); err != nil {
			return "", err
		}
		return result.TargetID, nil
	}
	return c.browser.NewTab(ctx, url)
}

// CloseTab closes a tab.
func (c *Client) CloseTab(ctx context.Context, targetID string) error {
	if c.daemon {
		return c.daemonExec(ctx, "tab_close", map[string]any{"targetId": targetID})
	}
	return c.browser.CloseTab(ctx, targetID)
}

// Close closes the client connection.
func (c *Client) Close() error {
	if c.daemon {
		return nil
	}
	if c.browser != nil {
		return c.browser.Close()
	}
	return nil
}

// IsConnected reports whether the connection is alive.
func (c *Client) IsConnected() bool {
	if c.daemon {
		conn, err := dialDaemonTimeout("unix", c.socketPath, 500*time.Millisecond)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
	if c.browser != nil {
		return c.browser.IsConnected()
	}
	return false
}

func (c *Client) daemonExec(ctx context.Context, action string, extra map[string]any) error {
	_, err := c.daemonCall(ctx, action, extra)
	return err
}

func daemonValue[T any](ctx context.Context, c *Client, action string, extra map[string]any) (T, error) {
	var out T
	err := c.daemonCallInto(ctx, action, extra, &out)
	return out, err
}

func daemonScreenshot(ctx context.Context, c *Client, opts *protocol.ScreenshotOptions) ([]byte, error) {
	extra := map[string]any{}
	if opts != nil {
		extra["format"] = opts.Format
		extra["quality"] = opts.Quality
		extra["fullPage"] = opts.FullPage
		extra["selector"] = opts.Selector
		extra["clipX"] = opts.ClipX
		extra["clipY"] = opts.ClipY
		extra["clipWidth"] = opts.ClipWidth
		extra["clipHeight"] = opts.ClipHeight
	}

	var result struct {
		Data []byte `json:"data"`
	}
	if err := c.daemonCallInto(ctx, "screenshot", extra, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *Client) daemonCallInto(ctx context.Context, action string, extra map[string]any, out any) error {
	resp, err := c.daemonCall(ctx, action, extra)
	if err != nil {
		return err
	}
	if out == nil || len(resp.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("client: decode daemon %s response: %w", action, err)
	}
	return nil
}

// daemonCall sends a command to the daemon and returns the response.
func (c *Client) daemonCall(ctx context.Context, action string, extra map[string]any) (*protocol.Response, error) {
	if !c.daemon || c.socketPath == "" {
		return nil, fmt.Errorf("client: not connected to daemon")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := dialDaemonContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("client: connect to daemon: %w", err)
	}
	defer conn.Close()

	resp, err := c.daemonSend(ctx, conn, action, extra)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		if resp.Error == "" {
			resp.Error = "daemon command failed"
		}
		return nil, fmt.Errorf("client: daemon %s: %s", action, resp.Error)
	}
	return resp, nil
}

// daemonSend sends a command to the daemon and returns the response.
func (c *Client) daemonSend(ctx context.Context, conn net.Conn, action string, extra map[string]any) (*protocol.Response, error) {
	req := map[string]any{
		"id":     fmt.Sprintf("r%d", time.Now().UnixMicro()%1000000),
		"action": action,
	}
	for k, v := range extra {
		req[k] = v
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
				return nil, context.DeadlineExceeded
			}
		}
		return nil, fmt.Errorf("read response: %w", err)
	}

	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return nil, fmt.Errorf("read response: empty response")
	}

	var resp protocol.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

func parsePID(data []byte) (int, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		if err == nil {
			err = fmt.Errorf("pid must be positive")
		}
		return 0, err
	}
	return pid, nil
}

// getSocketDir returns the default socket directory.
func getSocketDir() string {
	if dir := os.Getenv("VIBE_BROWSER_SOCKET_DIR"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "vibe-browser")
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "vibe-browser")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".vibe-browser")
	}
	return filepath.Join(os.TempDir(), "vibe-browser")
}

// isDaemonRunning checks if a daemon is running for the given session.
func isDaemonRunning(socketPath, session, socketDir string) bool {
	if _, err := os.Stat(socketPath); err != nil {
		return false
	}

	pidPath := filepath.Join(socketDir, session+".pid")
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}

	pid, err := parsePID(pidData)
	if err != nil {
		return false
	}

	return isProcessAlive(pid)
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
