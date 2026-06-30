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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

	// Headless mode for launched browsers (default true).
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
	browser  *browser.Browser
	daemon   bool
	session  string
	logger   *slog.Logger
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
		b, err := browser.ConnectToCDP(ctx, opts.CDPURL, logger)
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
		cdpURL, err := chrome.DiscoverCDPURL(host, opts.CDPPort)
		if err != nil {
			return nil, fmt.Errorf("client: discover CDP: %w", err)
		}
		b, err := browser.ConnectToCDP(ctx, cdpURL, logger)
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
		Headless:       opts.Headless,
		ExecutablePath: opts.ExecutablePath,
	}

	if opts.Launch != nil {
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

	if !launchOpts.Headless && !opts.Headless {
		launchOpts.Headless = true // default to headless
	}

	proc, err := chrome.Launch(ctx, launchOpts, logger)
	if err != nil {
		return nil, fmt.Errorf("client: launch chrome: %w", err)
	}

	b, err := browser.ConnectToCDP(ctx, proc.CDPURL, logger)
	if err != nil {
		proc.Kill()
		return nil, fmt.Errorf("client: connect to launched browser: %w", err)
	}

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
	if !isDaemonRunning(socketPath, session, socketDir) {
		return nil, fmt.Errorf("client: daemon not running for session %q; start it with: vibe-browser daemon --session %s", session, session)
	}

	// Connect via Unix socket to verify daemon is reachable
	_, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("client: connect to daemon: %w", err)
	}

	return &Client{
		daemon:  true,
		session: session,
		logger:  logger,
	}, nil
}

// Browser returns the underlying browser.Browser for direct CDP operations.
func (c *Client) Browser() *browser.Browser {
	return c.browser
}

// Navigate navigates to a URL.
func (c *Client) Navigate(ctx context.Context, url string) error {
	return c.browser.Navigate(ctx, url, nil)
}

// NavigateWith waits for a specific load state after navigation.
func (c *Client) NavigateWith(ctx context.Context, url string, waitUntil string) error {
	return c.browser.Navigate(ctx, url, &protocol.NavigationOptions{
		WaitUntil: waitUntil,
	})
}

// URL returns the current page URL.
func (c *Client) URL(ctx context.Context) (string, error) {
	return c.browser.GetURL(ctx)
}

// Title returns the current page title.
func (c *Client) Title(ctx context.Context) (string, error) {
	return c.browser.GetTitle(ctx)
}

// Click clicks an element.
func (c *Client) Click(ctx context.Context, selector string) error {
	return c.browser.Click(ctx, selector, nil)
}

// DoubleClick double-clicks an element.
func (c *Client) DoubleClick(ctx context.Context, selector string) error {
	return c.browser.DoubleClick(ctx, selector)
}

// Fill fills an input element.
func (c *Client) Fill(ctx context.Context, selector string, value string) error {
	return c.browser.Fill(ctx, selector, value)
}

// Type types text into an element.
func (c *Client) Type(ctx context.Context, selector string, text string) error {
	return c.browser.Type(ctx, selector, text, 50)
}

// Press presses a keyboard key.
func (c *Client) Press(ctx context.Context, key string) error {
	return c.browser.Press(ctx, key)
}

// Hover moves the mouse over an element.
func (c *Client) Hover(ctx context.Context, selector string) error {
	return c.browser.Hover(ctx, selector)
}

// Scroll scrolls the page.
func (c *Client) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	return c.browser.Scroll(ctx, deltaX, deltaY)
}

// Focus focuses an element.
func (c *Client) Focus(ctx context.Context, selector string) error {
	return c.browser.Focus(ctx, selector)
}

// Check checks a checkbox.
func (c *Client) Check(ctx context.Context, selector string) error {
	return c.browser.Check(ctx, selector)
}

// Uncheck unchecks a checkbox.
func (c *Client) Uncheck(ctx context.Context, selector string) error {
	return c.browser.Uncheck(ctx, selector)
}

// Select selects a value in a select element.
func (c *Client) Select(ctx context.Context, selector string, value string) error {
	return c.browser.Select(ctx, selector, value)
}

// Eval evaluates JavaScript.
func (c *Client) Eval(ctx context.Context, expression string) (any, error) {
	return c.browser.Eval(ctx, expression)
}

// GetText returns the text content of an element.
func (c *Client) GetText(ctx context.Context, selector string) (string, error) {
	return c.browser.GetText(ctx, selector)
}

// GetHTML returns the HTML content.
func (c *Client) GetHTML(ctx context.Context, selector string) (string, error) {
	return c.browser.GetHTML(ctx, selector)
}

// GetValue returns the value of an input element.
func (c *Client) GetValue(ctx context.Context, selector string) (string, error) {
	return c.browser.GetValue(ctx, selector)
}

// GetAttr returns an attribute value.
func (c *Client) GetAttr(ctx context.Context, selector, attr string) (string, error) {
	return c.browser.GetAttr(ctx, selector, attr)
}

// IsVisible checks if an element is visible.
func (c *Client) IsVisible(ctx context.Context, selector string) (bool, error) {
	return c.browser.IsVisible(ctx, selector)
}

// IsEnabled checks if an element is enabled.
func (c *Client) IsEnabled(ctx context.Context, selector string) (bool, error) {
	return c.browser.IsEnabled(ctx, selector)
}

// IsChecked checks if a checkbox is checked.
func (c *Client) IsChecked(ctx context.Context, selector string) (bool, error) {
	return c.browser.IsChecked(ctx, selector)
}

// Snapshot captures an accessibility tree snapshot.
func (c *Client) Snapshot(ctx context.Context) (string, error) {
	return c.browser.Snapshot(ctx, nil)
}

// SnapshotWithOptions captures a snapshot with options.
func (c *Client) SnapshotWithOptions(ctx context.Context, opts *protocol.SnapshotOptions) (string, error) {
	return c.browser.Snapshot(ctx, opts)
}

// Screenshot captures a screenshot.
func (c *Client) Screenshot(ctx context.Context) ([]byte, error) {
	return c.browser.Screenshot(ctx, nil)
}

// ScreenshotWithOptions captures a screenshot with options.
func (c *Client) ScreenshotWithOptions(ctx context.Context, opts *protocol.ScreenshotOptions) ([]byte, error) {
	return c.browser.Screenshot(ctx, opts)
}

// Reload reloads the page.
func (c *Client) Reload(ctx context.Context) error {
	return c.browser.Reload(ctx)
}

// Back navigates back.
func (c *Client) Back(ctx context.Context) error {
	return c.browser.GoBack(ctx)
}

// Forward navigates forward.
func (c *Client) Forward(ctx context.Context) error {
	return c.browser.GoForward(ctx)
}

// WaitMS waits for a duration.
func (c *Client) WaitMS(ctx context.Context, ms int) error {
	return c.browser.WaitMS(ctx, ms)
}

// WaitForSelector waits for an element to appear.
func (c *Client) WaitForSelector(ctx context.Context, selector string) error {
	return c.browser.WaitForSelector(ctx, selector, 0)
}

// WaitForText waits for text to appear.
func (c *Client) WaitForText(ctx context.Context, text string) error {
	return c.browser.WaitForText(ctx, text, 0)
}

// WaitForURL waits for the URL to match.
func (c *Client) WaitForURL(ctx context.Context, urlPattern string) error {
	return c.browser.WaitForURL(ctx, urlPattern, 0)
}

// SetViewport sets the viewport size.
func (c *Client) SetViewport(ctx context.Context, width, height int) error {
	return c.browser.SetViewport(ctx, width, height, 1.0)
}

// SetGeolocation sets the geolocation.
func (c *Client) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return c.browser.SetGeolocation(ctx, lat, lng, accuracy)
}

// SetOffline enables/disables offline mode.
func (c *Client) SetOffline(ctx context.Context, offline bool) error {
	return c.browser.SetOffline(ctx, offline)
}

// SetHeaders sets extra HTTP headers.
func (c *Client) SetHeaders(ctx context.Context, headers map[string]string) error {
	return c.browser.SetHeaders(ctx, headers)
}

// GetCookies returns all cookies.
func (c *Client) GetCookies(ctx context.Context) ([]protocol.Cookie, error) {
	return c.browser.GetCookies(ctx)
}

// SetCookie sets a cookie.
func (c *Client) SetCookie(ctx context.Context, cookie protocol.Cookie) error {
	return c.browser.SetCookie(ctx, cookie)
}

// ClearCookies clears all cookies.
func (c *Client) ClearCookies(ctx context.Context) error {
	return c.browser.ClearCookies(ctx)
}

// NewTab opens a new tab.
func (c *Client) NewTab(ctx context.Context, url string) (string, error) {
	return c.browser.NewTab(ctx, url)
}

// CloseTab closes a tab.
func (c *Client) CloseTab(ctx context.Context, targetID string) error {
	return c.browser.CloseTab(ctx, targetID)
}

// Close closes the client connection.
func (c *Client) Close() error {
	if c.browser != nil {
		return c.browser.Close()
	}
	return nil
}

// IsConnected reports whether the connection is alive.
func (c *Client) IsConnected() bool {
	if c.browser != nil {
		return c.browser.IsConnected()
	}
	return false
}

// daemonSend sends a command to the daemon and returns the response.
func (c *Client) daemonSend(conn net.Conn, action string, extra map[string]any) (*protocol.Response, error) {
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

	// Read response
	buf := make([]byte, 1024*1024) // 1MB buffer
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp protocol.Response
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
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
	// Check if socket file exists
	if _, err := os.Stat(socketPath); err != nil {
		return false
	}

	// Check if PID file exists and process is alive
	pidPath := filepath.Join(socketDir, session+".pid")
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return false
	}

	// Check if process is alive (platform-specific)
	return isProcessAlive(pid)
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; we need to send signal 0
	err = proc.Signal(os.Signal(nil))
	return err == nil
}
