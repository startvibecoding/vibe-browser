package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/startvibecoding/vibe-browser/internal/chrome"
	"github.com/startvibecoding/vibe-browser/pkg/browser"
	"github.com/startvibecoding/vibe-browser/pkg/cdp"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCDPURL(t *testing.T) {
	stub := stubOpenDeps(t)
	defer stub.restore()
	stub.connect = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
		stub.connectedURLs = append(stub.connectedURLs, wsURL)
		return browser.New(newClientFakeCDP(), logger), nil
	}

	c, err := Open(context.Background(), &Options{CDPURL: "ws://cdp"})
	require.NoError(t, err)
	require.NotNil(t, c.Browser())
	assert.Equal(t, []string{"ws://cdp"}, stub.connectedURLs)
}

func TestOpenDiscoversCDPURL(t *testing.T) {
	stub := stubOpenDeps(t)
	defer stub.restore()
	stub.discover = func(host string, port int) (string, error) {
		assert.Equal(t, "127.0.0.1", host)
		assert.Equal(t, 9222, port)
		return "ws://discovered", nil
	}
	stub.connect = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
		stub.connectedURLs = append(stub.connectedURLs, wsURL)
		return browser.New(newClientFakeCDP(), logger), nil
	}

	c, err := Open(context.Background(), &Options{CDPPort: 9222})
	require.NoError(t, err)
	require.NotNil(t, c.Browser())
	assert.Equal(t, []string{"ws://discovered"}, stub.connectedURLs)
}

func TestOpenLaunchMapsOptionsAndClosesProcessOnConnectError(t *testing.T) {
	stub := stubOpenDeps(t)
	defer stub.restore()
	killed := false
	stub.launch = func(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (chromeProcess, error) {
		stub.launchOpts = append(stub.launchOpts, opts)
		return &fakeChromeProcess{url: "ws://launched", kill: func() { killed = true }}, nil
	}
	stub.connect = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
		return nil, errors.New("connect failed")
	}

	_, err := Open(context.Background(), &Options{
		Browser:        protocol.BrowserBrave,
		ExecutablePath: "/bin/browser",
		Launch: &protocol.LaunchOptions{
			Browser:        protocol.BrowserEdge,
			Headless:       true,
			ExecutablePath: "/bin/edge",
			Args:           []string{"--a"},
			Proxy:          "http://proxy",
			UserDataDir:    "/tmp/profile",
			ViewportWidth:  800,
			ViewportHeight: 600,
			Extensions:     []string{"/ext"},
			Profile:        "Profile 1",
		},
	})
	require.Error(t, err)
	assert.True(t, killed)
	require.Len(t, stub.launchOpts, 1)
	opts := stub.launchOpts[0]
	assert.Equal(t, chrome.BrowserEdge, opts.Browser)
	assert.True(t, opts.Headless)
	assert.Equal(t, "/bin/edge", opts.ExecutablePath)
	assert.Equal(t, []string{"--a"}, opts.Args)
	assert.Equal(t, "http://proxy", opts.Proxy)
	assert.Equal(t, "/tmp/profile", opts.UserDataDir)
	assert.Equal(t, 800, opts.ViewportWidth)
	assert.Equal(t, 600, opts.ViewportHeight)
	assert.Equal(t, []string{"/ext"}, opts.Extensions)
	assert.Equal(t, "Profile 1", opts.Profile)
}

func TestOpenLaunchDefaultsHeadless(t *testing.T) {
	stub := stubOpenDeps(t)
	defer stub.restore()
	stub.launch = func(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (chromeProcess, error) {
		stub.launchOpts = append(stub.launchOpts, opts)
		return &fakeChromeProcess{url: "ws://launched"}, nil
	}
	stub.connect = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
		return browser.New(newClientFakeCDP(), logger), nil
	}

	c, err := Open(context.Background(), &Options{})
	require.NoError(t, err)
	require.NotNil(t, c.Browser())
	require.Len(t, stub.launchOpts, 1)
	assert.True(t, stub.launchOpts[0].Headless)
}

func TestOpenErrors(t *testing.T) {
	t.Run("connect cdp url", func(t *testing.T) {
		stub := stubOpenDeps(t)
		defer stub.restore()
		stub.connect = func(context.Context, string, *slog.Logger) (*browser.Browser, error) {
			return nil, errors.New("bad cdp")
		}
		_, err := Open(context.Background(), &Options{CDPURL: "ws://bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connect to CDP")
	})

	t.Run("discover", func(t *testing.T) {
		stub := stubOpenDeps(t)
		defer stub.restore()
		stub.discover = func(string, int) (string, error) { return "", errors.New("no cdp") }
		_, err := Open(context.Background(), &Options{CDPPort: 1})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "discover CDP")
	})

	t.Run("launch", func(t *testing.T) {
		stub := stubOpenDeps(t)
		defer stub.restore()
		stub.launch = func(context.Context, chrome.LaunchOptions, *slog.Logger) (chromeProcess, error) {
			return nil, errors.New("no browser")
		}
		_, err := Open(context.Background(), &Options{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "launch chrome")
	})
}

func TestLaunchChromeDefaultReturnsLaunchError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := launchChromeDefault(ctx, chrome.LaunchOptions{Browser: chrome.BrowserType("missing-browser")}, slog.Default())
	require.Error(t, err)
}

func TestConnectAndDaemonMethods(t *testing.T) {
	stub := stubDaemonDialer(t, func(req map[string]any) protocol.Response {
		switch req["action"] {
		case "get_url":
			return responseData(t, "https://example.com")
		case "get_title":
			return responseData(t, "Example")
		case "eval":
			return responseData(t, map[string]any{"ok": true})
		case "get_text":
			return responseData(t, "Text")
		case "get_html":
			return responseData(t, "<html></html>")
		case "get_value":
			return responseData(t, "Value")
		case "get_attr":
			return responseData(t, "Attr")
		case "is_visible", "is_checked":
			return responseData(t, true)
		case "is_enabled":
			return responseData(t, false)
		case "snapshot":
			return responseData(t, "tree")
		case "screenshot":
			return responseData(t, map[string]any{"data": []byte("png")})
		case "cookies_get":
			return responseData(t, []protocol.Cookie{{Name: "sid", Value: "1"}})
		case "tab_new":
			return responseData(t, map[string]string{"targetId": "target-1"})
		default:
			return protocol.Response{Success: true}
		}
	})
	defer stub.Restore()

	c, err := Connect(context.Background(), &Options{Session: "sdk", DaemonSocketDir: t.TempDir()})
	require.NoError(t, err)
	assert.True(t, c.IsConnected())
	assert.Nil(t, c.Browser())
	require.NoError(t, c.Close())

	ctx := context.Background()
	require.NoError(t, c.Navigate(ctx, "https://example.com"))
	require.NoError(t, c.NavigateWith(ctx, "https://example.com/ready", "load"))
	require.NoError(t, c.Click(ctx, "#button"))
	require.NoError(t, c.DoubleClick(ctx, "#button"))
	require.NoError(t, c.Fill(ctx, "input", "abc"))
	require.NoError(t, c.Type(ctx, "input", "abc"))
	require.NoError(t, c.Press(ctx, "Enter"))
	require.NoError(t, c.Hover(ctx, "#button"))
	require.NoError(t, c.Scroll(ctx, 1, 2))
	require.NoError(t, c.Focus(ctx, "input"))
	require.NoError(t, c.Check(ctx, "input"))
	require.NoError(t, c.Uncheck(ctx, "input"))
	require.NoError(t, c.Select(ctx, "select", "a"))
	require.NoError(t, c.Reload(ctx))
	require.NoError(t, c.Back(ctx))
	require.NoError(t, c.Forward(ctx))
	require.NoError(t, c.WaitMS(ctx, 5))
	require.NoError(t, c.WaitForSelector(ctx, "#ready"))
	require.NoError(t, c.WaitForText(ctx, "Ready"))
	require.NoError(t, c.WaitForURL(ctx, "/ready"))
	require.NoError(t, c.SetViewport(ctx, 800, 600))
	require.NoError(t, c.SetGeolocation(ctx, 1, 2, 3))
	require.NoError(t, c.SetOffline(ctx, true))
	require.NoError(t, c.SetHeaders(ctx, map[string]string{"X-Test": "1"}))
	require.NoError(t, c.SetCookie(ctx, protocol.Cookie{Name: "sid", Value: "1"}))
	require.NoError(t, c.ClearCookies(ctx))
	require.NoError(t, c.CloseTab(ctx, "target-1"))

	url, err := c.URL(ctx)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", url)
	title, err := c.Title(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Example", title)
	value, err := c.Eval(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"ok": true}, value)
	text, err := c.GetText(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, "Text", text)
	html, err := c.GetHTML(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "<html></html>", html)
	inputValue, err := c.GetValue(ctx, "input")
	require.NoError(t, err)
	assert.Equal(t, "Value", inputValue)
	attr, err := c.GetAttr(ctx, "a", "href")
	require.NoError(t, err)
	assert.Equal(t, "Attr", attr)
	visible, err := c.IsVisible(ctx, "main")
	require.NoError(t, err)
	assert.True(t, visible)
	enabled, err := c.IsEnabled(ctx, "button")
	require.NoError(t, err)
	assert.False(t, enabled)
	checked, err := c.IsChecked(ctx, "input")
	require.NoError(t, err)
	assert.True(t, checked)
	snapshot, err := c.Snapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, "tree", snapshot)
	snapshot, err = c.SnapshotWithOptions(ctx, &protocol.SnapshotOptions{Interactive: true, Compact: true, Selector: "main", Depth: 2, URLs: true})
	require.NoError(t, err)
	assert.Equal(t, "tree", snapshot)
	img, err := c.Screenshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte("png"), img)
	img, err = c.ScreenshotWithOptions(ctx, &protocol.ScreenshotOptions{Format: "jpeg", Quality: 80, FullPage: true, Selector: "main", ClipX: 1, ClipY: 2, ClipWidth: 3, ClipHeight: 4})
	require.NoError(t, err)
	assert.Equal(t, []byte("png"), img)
	cookies, err := c.GetCookies(ctx)
	require.NoError(t, err)
	assert.Equal(t, []protocol.Cookie{{Name: "sid", Value: "1"}}, cookies)
	targetID, err := c.NewTab(ctx, "about:blank")
	require.NoError(t, err)
	assert.Equal(t, "target-1", targetID)

	seen := stub.Actions()
	assert.Contains(t, seen, "navigate")
	assert.Contains(t, seen, "screenshot")
	assert.Contains(t, seen, "cookies_set")
}

func TestConnectRequiresRunningDaemon(t *testing.T) {
	oldCheck := checkDaemonRunning
	checkDaemonRunning = func(string, string, string) bool { return false }
	defer func() { checkDaemonRunning = oldCheck }()

	_, err := Connect(context.Background(), &Options{Session: "missing", DaemonSocketDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon not running")
}

func TestConnectReachabilityError(t *testing.T) {
	oldCheck := checkDaemonRunning
	oldDialTimeout := dialDaemonTimeout
	checkDaemonRunning = func(string, string, string) bool { return true }
	dialDaemonTimeout = func(string, string, time.Duration) (net.Conn, error) {
		return nil, errors.New("dial failed")
	}
	defer func() {
		checkDaemonRunning = oldCheck
		dialDaemonTimeout = oldDialTimeout
	}()

	_, err := Connect(context.Background(), &Options{Session: "bad", DaemonSocketDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dial failed")
}

func TestDaemonCallErrors(t *testing.T) {
	stub := stubDaemonDialer(t, func(req map[string]any) protocol.Response {
		return protocol.Response{Success: false, Error: "boom"}
	})
	defer stub.Restore()

	c := &Client{daemon: true, socketPath: "fake.sock"}
	err := c.Navigate(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestDaemonDecodeErrors(t *testing.T) {
	stub := stubDaemonDialer(t, func(req map[string]any) protocol.Response {
		return responseData(t, map[string]any{"unexpected": true})
	})
	defer stub.Restore()

	c := &Client{daemon: true, socketPath: "fake.sock"}
	_, err := c.Title(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode daemon")
}

func TestDaemonSendEmptyResponse(t *testing.T) {
	c := &Client{}
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		_, _ = bufio.NewReader(serverConn).ReadBytes('\n')
		_, _ = serverConn.Write([]byte("\n"))
	}()
	_, err := c.daemonSend(context.Background(), clientConn, "empty", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestDaemonSendMalformedAndTimeout(t *testing.T) {
	c := &Client{}
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		_, _ = bufio.NewReader(serverConn).ReadBytes('\n')
		_, _ = serverConn.Write([]byte("{\n"))
	}()
	_, err := c.daemonSend(context.Background(), clientConn, "bad", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal response")

	clientConn2, serverConn2 := net.Pipe()
	defer clientConn2.Close()
	defer serverConn2.Close()
	go func() {
		_, _ = bufio.NewReader(serverConn2).ReadBytes('\n')
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err = c.daemonSend(ctx, clientConn2, "slow", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDaemonCallRequiresDaemonClient(t *testing.T) {
	_, err := (&Client{}).daemonCall(context.Background(), "ping", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected to daemon")
}

func TestDirectClientMethods(t *testing.T) {
	f := newClientFakeCDP()
	c := &Client{browser: browser.New(f, nil)}
	ctx := context.Background()

	f.queue("Page.navigate", `{}`)
	f.queue("Page.navigate", `{}`)
	f.events <- cdp.Event{Method: "Page.loadEventFired"}
	f.queue("Runtime.evaluate", clientRuntimeResult("https://example.com"))
	f.queue("Runtime.evaluate", clientRuntimeResult("Example"))
	f.queue("Runtime.evaluate", clientRuntimeResult(map[string]any{"x": 1, "y": 2, "width": 10, "height": 20}))
	f.queue("Runtime.evaluate", clientRuntimeResult(map[string]any{"x": 2, "y": 3, "width": 12, "height": 22}))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(map[string]any{"x": 3, "y": 4, "width": 14, "height": 24}))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(map[string]any{"ok": true}))
	f.queue("Runtime.evaluate", clientRuntimeResult("Text"))
	f.queue("Runtime.evaluate", clientRuntimeResult("<html></html>"))
	f.queue("Runtime.evaluate", clientRuntimeResult("Value"))
	f.queue("Runtime.evaluate", clientRuntimeResult("Attr"))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult(false))
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Accessibility.getFullAXTree", `{"nodes":[{"nodeId":"1","role":{"value":"button"},"name":{"value":"Save"}}]}`)
	f.queue("Accessibility.getFullAXTree", `{"nodes":[{"nodeId":"1","role":{"value":"button"},"name":{"value":"Save"}}]}`)
	f.queue("Page.captureScreenshot", `{"data":"cG5n"}`)
	f.queue("Page.captureScreenshot", `{"data":"anBlZw=="}`)
	f.queue("Runtime.evaluate", clientRuntimeResult(true))
	f.queue("Runtime.evaluate", clientRuntimeResult("ready text"))
	f.queue("Runtime.evaluate", clientRuntimeResult("https://example.com/ready"))
	f.queue("Network.getCookies", `{"cookies":[{"name":"sid","value":"1"}]}`)
	f.queue("Target.createTarget", `{"targetId":"target-1"}`)

	require.NoError(t, c.Navigate(ctx, "https://example.com"))
	require.NoError(t, c.NavigateWith(ctx, "https://example.com/load", "load"))

	url, err := c.URL(ctx)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", url)
	title, err := c.Title(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Example", title)

	require.NoError(t, c.Click(ctx, "#button"))
	require.NoError(t, c.DoubleClick(ctx, "#button"))
	require.NoError(t, c.Fill(ctx, "input", "abc"))
	require.NoError(t, c.Type(ctx, "input", "xy"))
	require.NoError(t, c.Press(ctx, "Enter"))
	require.NoError(t, c.Hover(ctx, "#button"))
	require.NoError(t, c.Scroll(ctx, 1, 2))
	require.NoError(t, c.Focus(ctx, "input"))
	require.NoError(t, c.Check(ctx, "input"))
	require.NoError(t, c.Uncheck(ctx, "input"))
	require.NoError(t, c.Select(ctx, "select", "a"))

	value, err := c.Eval(ctx, "({ok: true})")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"ok": true}, value)
	text, err := c.GetText(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, "Text", text)
	html, err := c.GetHTML(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "<html></html>", html)
	inputValue, err := c.GetValue(ctx, "input")
	require.NoError(t, err)
	assert.Equal(t, "Value", inputValue)
	attr, err := c.GetAttr(ctx, "a", "href")
	require.NoError(t, err)
	assert.Equal(t, "Attr", attr)
	visible, err := c.IsVisible(ctx, "main")
	require.NoError(t, err)
	assert.True(t, visible)
	enabled, err := c.IsEnabled(ctx, "button")
	require.NoError(t, err)
	assert.False(t, enabled)
	checked, err := c.IsChecked(ctx, "input")
	require.NoError(t, err)
	assert.True(t, checked)

	snapshot, err := c.Snapshot(ctx)
	require.NoError(t, err)
	assert.Contains(t, snapshot, "button Save")
	snapshot, err = c.SnapshotWithOptions(ctx, &protocol.SnapshotOptions{Interactive: true})
	require.NoError(t, err)
	assert.Contains(t, snapshot, "button Save")
	img, err := c.Screenshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte("png"), img)
	img, err = c.ScreenshotWithOptions(ctx, &protocol.ScreenshotOptions{Format: "jpeg", ClipWidth: 1, ClipHeight: 1})
	require.NoError(t, err)
	assert.Equal(t, []byte("jpeg"), img)

	require.NoError(t, c.Reload(ctx))
	require.NoError(t, c.Back(ctx))
	require.NoError(t, c.Forward(ctx))
	require.NoError(t, c.WaitMS(ctx, 1))
	require.NoError(t, c.WaitForSelector(ctx, "#ready"))
	require.NoError(t, c.WaitForText(ctx, "ready"))
	require.NoError(t, c.WaitForURL(ctx, "/ready"))
	require.NoError(t, c.SetViewport(ctx, 800, 600))
	require.NoError(t, c.SetGeolocation(ctx, 1, 2, 3))
	require.NoError(t, c.SetOffline(ctx, true))
	require.NoError(t, c.SetHeaders(ctx, map[string]string{"X-Test": "1"}))

	cookies, err := c.GetCookies(ctx)
	require.NoError(t, err)
	assert.Equal(t, []protocol.Cookie{{Name: "sid", Value: "1"}}, cookies)
	require.NoError(t, c.SetCookie(ctx, protocol.Cookie{Name: "sid", Value: "1"}))
	require.NoError(t, c.ClearCookies(ctx))
	targetID, err := c.NewTab(ctx, "about:blank")
	require.NoError(t, err)
	assert.Equal(t, "target-1", targetID)
	require.NoError(t, c.CloseTab(ctx, targetID))

	assert.True(t, c.IsConnected())
	require.NoError(t, c.Close())
	assert.False(t, c.IsConnected())
}

func TestClientNilBrowserConnectionState(t *testing.T) {
	c := &Client{}
	assert.False(t, c.IsConnected())
	require.NoError(t, c.Close())
}

func TestClientHelpers(t *testing.T) {
	pid, err := parsePID([]byte("123\n"))
	require.NoError(t, err)
	assert.Equal(t, 123, pid)
	_, err = parsePID([]byte("0"))
	require.Error(t, err)
	_, err = parsePID([]byte("abc"))
	require.Error(t, err)

	t.Setenv("VIBE_BROWSER_SOCKET_DIR", "/tmp/custom-vibe-browser")
	assert.Equal(t, "/tmp/custom-vibe-browser", getSocketDir())
	assert.False(t, isProcessAlive(-1))
	assert.True(t, isProcessAlive(os.Getpid()))

	dir := t.TempDir()
	assert.False(t, isDaemonRunning(filepath.Join(dir, "missing.sock"), "missing", dir))

	t.Setenv("VIBE_BROWSER_SOCKET_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", dir)
	assert.Equal(t, filepath.Join(dir, "vibe-browser"), getSocketDir())

	home := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", home)
	assert.Equal(t, filepath.Join(home, ".vibe-browser"), getSocketDir())

	socketPath := filepath.Join(dir, "live.sock")
	require.NoError(t, os.WriteFile(socketPath, nil, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "live.pid"), []byte(strconv.Itoa(os.Getpid())), 0644))
	assert.True(t, isDaemonRunning(socketPath, "live", dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.pid"), []byte("not-a-pid"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.sock"), nil, 0644))
	assert.False(t, isDaemonRunning(filepath.Join(dir, "bad.sock"), "bad", dir))
}

type restoreDaemonStubs struct {
	mu      sync.Mutex
	actions []string
	restore func()
}

func (r *restoreDaemonStubs) Actions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.actions...)
}

func (r *restoreDaemonStubs) Restore() { r.restore() }

func stubDaemonDialer(t *testing.T, handler func(map[string]any) protocol.Response) *restoreDaemonStubs {
	t.Helper()
	oldCheck := checkDaemonRunning
	oldDialTimeout := dialDaemonTimeout
	oldDialContext := dialDaemonContext
	stub := &restoreDaemonStubs{}

	makeConn := func() net.Conn {
		clientConn, serverConn := net.Pipe()
		go serveDaemonPipe(serverConn, handler, stub)
		return clientConn
	}
	checkDaemonRunning = func(string, string, string) bool { return true }
	dialDaemonTimeout = func(string, string, time.Duration) (net.Conn, error) {
		return makeConn(), nil
	}
	dialDaemonContext = func(context.Context, string, string) (net.Conn, error) {
		return makeConn(), nil
	}

	stub.restore = func() {
		checkDaemonRunning = oldCheck
		dialDaemonTimeout = oldDialTimeout
		dialDaemonContext = oldDialContext
	}
	return stub
}

func serveDaemonPipe(conn net.Conn, handler func(map[string]any) protocol.Response, stub *restoreDaemonStubs) {
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	var req map[string]any
	if err := json.Unmarshal(line, &req); err != nil {
		return
	}
	if action, _ := req["action"].(string); action != "" {
		stub.mu.Lock()
		stub.actions = append(stub.actions, action)
		stub.mu.Unlock()
	}
	resp := handler(req)
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = conn.Write(data)
}

func responseData(t *testing.T, v any) protocol.Response {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return protocol.Response{Success: true, Data: data}
}

type openStub struct {
	oldConnect    func(context.Context, string, *slog.Logger) (*browser.Browser, error)
	oldDiscover   func(string, int) (string, error)
	oldLaunch     func(context.Context, chrome.LaunchOptions, *slog.Logger) (chromeProcess, error)
	connect       func(context.Context, string, *slog.Logger) (*browser.Browser, error)
	discover      func(string, int) (string, error)
	launch        func(context.Context, chrome.LaunchOptions, *slog.Logger) (chromeProcess, error)
	connectedURLs []string
	launchOpts    []chrome.LaunchOptions
}

func stubOpenDeps(t *testing.T) *openStub {
	t.Helper()
	stub := &openStub{
		oldConnect:  connectToCDP,
		oldDiscover: discoverCDPURL,
		oldLaunch:   launchChrome,
	}
	stub.connect = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
		stub.connectedURLs = append(stub.connectedURLs, wsURL)
		return browser.New(newClientFakeCDP(), logger), nil
	}
	stub.discover = func(host string, port int) (string, error) {
		return "ws://discovered", nil
	}
	stub.launch = func(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (chromeProcess, error) {
		stub.launchOpts = append(stub.launchOpts, opts)
		return &fakeChromeProcess{url: "ws://launched"}, nil
	}
	connectToCDP = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
		return stub.connect(ctx, wsURL, logger)
	}
	discoverCDPURL = func(host string, port int) (string, error) {
		return stub.discover(host, port)
	}
	launchChrome = func(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (chromeProcess, error) {
		return stub.launch(ctx, opts, logger)
	}
	return stub
}

func (s *openStub) restore() {
	connectToCDP = s.oldConnect
	discoverCDPURL = s.oldDiscover
	launchChrome = s.oldLaunch
}

type fakeChromeProcess struct {
	url  string
	kill func()
}

func (p *fakeChromeProcess) CDPWebSocketURL() string { return p.url }
func (p *fakeChromeProcess) Kill() {
	if p.kill != nil {
		p.kill()
	}
}

type clientFakeCDP struct {
	responses map[string][]json.RawMessage
	calls     []string
	events    chan cdp.Event
	connected bool
}

func newClientFakeCDP() *clientFakeCDP {
	return &clientFakeCDP{
		responses: make(map[string][]json.RawMessage),
		events:    make(chan cdp.Event, 16),
		connected: true,
	}
}

func (f *clientFakeCDP) queue(method string, result string) {
	f.responses[method] = append(f.responses[method], json.RawMessage(result))
}

func (f *clientFakeCDP) Send(ctx context.Context, method string, params any) (*cdp.Message, error) {
	f.calls = append(f.calls, method)
	items := f.responses[method]
	if len(items) > 0 {
		result := items[0]
		f.responses[method] = items[1:]
		return &cdp.Message{Result: result}, nil
	}
	return &cdp.Message{Result: json.RawMessage(`{}`)}, nil
}

func (f *clientFakeCDP) Events() <-chan cdp.Event { return f.events }
func (f *clientFakeCDP) Close() error {
	f.connected = false
	return nil
}
func (f *clientFakeCDP) IsConnected() bool { return f.connected }

func clientRuntimeResult(value any) string {
	data, _ := json.Marshal(map[string]any{
		"result": map[string]any{"value": value},
	})
	return string(data)
}
