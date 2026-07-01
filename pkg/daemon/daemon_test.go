package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/startvibecoding/vibe-browser/internal/chrome"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBrowser struct {
	calls      []daemonCall
	err        error
	url        string
	title      string
	text       string
	html       string
	value      string
	attr       string
	visible    bool
	enabled    bool
	checked    bool
	eval       any
	snapshot   string
	screenshot []byte
	cookies    []protocol.Cookie
	targetID   string
	closed     bool
}

type daemonCall struct {
	name string
	args []any
}

func (f *fakeBrowser) record(name string, args ...any) error {
	f.calls = append(f.calls, daemonCall{name: name, args: args})
	return f.err
}

func (f *fakeBrowser) Navigate(ctx context.Context, url string, opts *protocol.NavigationOptions) error {
	return f.record("Navigate", url, opts)
}
func (f *fakeBrowser) Reload(ctx context.Context) error { return f.record("Reload") }
func (f *fakeBrowser) GoBack(ctx context.Context) error { return f.record("GoBack") }
func (f *fakeBrowser) GoForward(ctx context.Context) error {
	return f.record("GoForward")
}
func (f *fakeBrowser) Click(ctx context.Context, selector string, opts *protocol.ClickOptions) error {
	return f.record("Click", selector, opts)
}
func (f *fakeBrowser) DoubleClick(ctx context.Context, selector string) error {
	return f.record("DoubleClick", selector)
}
func (f *fakeBrowser) Fill(ctx context.Context, selector, value string) error {
	return f.record("Fill", selector, value)
}
func (f *fakeBrowser) Type(ctx context.Context, selector, text string, delay int) error {
	return f.record("Type", selector, text, delay)
}
func (f *fakeBrowser) Press(ctx context.Context, key string) error { return f.record("Press", key) }
func (f *fakeBrowser) Hover(ctx context.Context, selector string) error {
	return f.record("Hover", selector)
}
func (f *fakeBrowser) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	return f.record("Scroll", deltaX, deltaY)
}
func (f *fakeBrowser) Focus(ctx context.Context, selector string) error {
	return f.record("Focus", selector)
}
func (f *fakeBrowser) Check(ctx context.Context, selector string) error {
	return f.record("Check", selector)
}
func (f *fakeBrowser) Uncheck(ctx context.Context, selector string) error {
	return f.record("Uncheck", selector)
}
func (f *fakeBrowser) Select(ctx context.Context, selector, value string) error {
	return f.record("Select", selector, value)
}
func (f *fakeBrowser) Eval(ctx context.Context, expression string) (any, error) {
	return f.eval, f.record("Eval", expression)
}
func (f *fakeBrowser) GetText(ctx context.Context, selector string) (string, error) {
	return f.text, f.record("GetText", selector)
}
func (f *fakeBrowser) GetHTML(ctx context.Context, selector string) (string, error) {
	return f.html, f.record("GetHTML", selector)
}
func (f *fakeBrowser) GetValue(ctx context.Context, selector string) (string, error) {
	return f.value, f.record("GetValue", selector)
}
func (f *fakeBrowser) GetAttr(ctx context.Context, selector, attr string) (string, error) {
	return f.attr, f.record("GetAttr", selector, attr)
}
func (f *fakeBrowser) IsVisible(ctx context.Context, selector string) (bool, error) {
	return f.visible, f.record("IsVisible", selector)
}
func (f *fakeBrowser) IsEnabled(ctx context.Context, selector string) (bool, error) {
	return f.enabled, f.record("IsEnabled", selector)
}
func (f *fakeBrowser) IsChecked(ctx context.Context, selector string) (bool, error) {
	return f.checked, f.record("IsChecked", selector)
}
func (f *fakeBrowser) GetURL(ctx context.Context) (string, error) {
	return f.url, f.record("GetURL")
}
func (f *fakeBrowser) GetTitle(ctx context.Context) (string, error) {
	return f.title, f.record("GetTitle")
}
func (f *fakeBrowser) Snapshot(ctx context.Context, opts *protocol.SnapshotOptions) (string, error) {
	return f.snapshot, f.record("Snapshot", opts)
}
func (f *fakeBrowser) Screenshot(ctx context.Context, opts *protocol.ScreenshotOptions) ([]byte, error) {
	return f.screenshot, f.record("Screenshot", opts)
}
func (f *fakeBrowser) SetViewport(ctx context.Context, width, height int, scale float64) error {
	return f.record("SetViewport", width, height, scale)
}
func (f *fakeBrowser) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return f.record("SetGeolocation", lat, lng, accuracy)
}
func (f *fakeBrowser) SetOffline(ctx context.Context, offline bool) error {
	return f.record("SetOffline", offline)
}
func (f *fakeBrowser) SetHeaders(ctx context.Context, headers map[string]string) error {
	return f.record("SetHeaders", headers)
}
func (f *fakeBrowser) GetCookies(ctx context.Context) ([]protocol.Cookie, error) {
	return f.cookies, f.record("GetCookies")
}
func (f *fakeBrowser) SetCookie(ctx context.Context, cookie protocol.Cookie) error {
	return f.record("SetCookie", cookie)
}
func (f *fakeBrowser) ClearCookies(ctx context.Context) error { return f.record("ClearCookies") }
func (f *fakeBrowser) WaitMS(ctx context.Context, ms int) error {
	return f.record("WaitMS", ms)
}
func (f *fakeBrowser) WaitForSelector(ctx context.Context, selector string, timeout int) error {
	return f.record("WaitForSelector", selector, timeout)
}
func (f *fakeBrowser) WaitForText(ctx context.Context, text string, timeout int) error {
	return f.record("WaitForText", text, timeout)
}
func (f *fakeBrowser) WaitForURL(ctx context.Context, url string, timeout int) error {
	return f.record("WaitForURL", url, timeout)
}
func (f *fakeBrowser) NewTab(ctx context.Context, url string) (string, error) {
	return f.targetID, f.record("NewTab", url)
}
func (f *fakeBrowser) CloseTab(ctx context.Context, targetID string) error {
	return f.record("CloseTab", targetID)
}
func (f *fakeBrowser) Close() error {
	f.closed = true
	return f.record("Close")
}

func TestExecuteCommandDispatch(t *testing.T) {
	f := &fakeBrowser{
		url:        "https://example.com",
		title:      "Example",
		text:       "Text",
		html:       "<html></html>",
		value:      "Value",
		attr:       "Attr",
		visible:    true,
		enabled:    true,
		checked:    true,
		eval:       map[string]any{"ok": true},
		snapshot:   "tree",
		screenshot: []byte("png"),
		cookies:    []protocol.Cookie{{Name: "sid", Value: "1"}},
		targetID:   "target-1",
	}
	s := testServer(f)

	tests := []struct {
		name   string
		action string
		req    map[string]any
		call   string
	}{
		{"navigate", "navigate", map[string]any{"url": "https://example.com", "waitUntil": "load", "timeout": float64(100)}, "Navigate"},
		{"reload", "reload", nil, "Reload"},
		{"back", "back", nil, "GoBack"},
		{"forward", "forward", nil, "GoForward"},
		{"click", "click", map[string]any{"selector": "#a"}, "Click"},
		{"dblclick", "dblclick", map[string]any{"selector": "#a"}, "DoubleClick"},
		{"fill", "fill", map[string]any{"selector": "#a", "value": "x"}, "Fill"},
		{"type", "type", map[string]any{"selector": "#a", "text": "x", "delay": float64(1)}, "Type"},
		{"press", "press", map[string]any{"key": "Enter"}, "Press"},
		{"hover", "hover", map[string]any{"selector": "#a"}, "Hover"},
		{"scroll", "scroll", map[string]any{"deltaX": float64(1), "deltaY": float64(2)}, "Scroll"},
		{"focus", "focus", map[string]any{"selector": "#a"}, "Focus"},
		{"check", "check", map[string]any{"selector": "#a"}, "Check"},
		{"uncheck", "uncheck", map[string]any{"selector": "#a"}, "Uncheck"},
		{"select", "select", map[string]any{"selector": "#a", "value": "x"}, "Select"},
		{"eval", "eval", map[string]any{"expression": "1"}, "Eval"},
		{"get_text", "get_text", map[string]any{"selector": "#a"}, "GetText"},
		{"get_html", "get_html", map[string]any{"selector": "#a"}, "GetHTML"},
		{"get_value", "get_value", map[string]any{"selector": "#a"}, "GetValue"},
		{"get_attr", "get_attr", map[string]any{"selector": "#a", "attr": "href"}, "GetAttr"},
		{"is_visible", "is_visible", map[string]any{"selector": "#a"}, "IsVisible"},
		{"is_enabled", "is_enabled", map[string]any{"selector": "#a"}, "IsEnabled"},
		{"is_checked", "is_checked", map[string]any{"selector": "#a"}, "IsChecked"},
		{"get_url", "get_url", nil, "GetURL"},
		{"get_title", "get_title", nil, "GetTitle"},
		{"snapshot", "snapshot", map[string]any{"interactive": true, "compact": true, "selector": "main", "depth": float64(2), "urls": true}, "Snapshot"},
		{"screenshot", "screenshot", map[string]any{"format": "jpeg", "quality": float64(80), "fullPage": true, "selector": "main", "clipX": float64(1), "clipY": float64(2), "clipWidth": float64(3), "clipHeight": float64(4)}, "Screenshot"},
		{"set_viewport", "set_viewport", map[string]any{"width": float64(800), "height": float64(600), "deviceScaleFactor": float64(2)}, "SetViewport"},
		{"set_geolocation", "set_geolocation", map[string]any{"latitude": float64(1), "longitude": float64(2), "accuracy": float64(3)}, "SetGeolocation"},
		{"set_offline", "set_offline", map[string]any{"offline": true}, "SetOffline"},
		{"set_headers", "set_headers", map[string]any{"headers": map[string]any{"X-Test": "1"}}, "SetHeaders"},
		{"cookies_get", "cookies_get", nil, "GetCookies"},
		{"cookies_set", "cookies_set", map[string]any{"cookie": map[string]any{"name": "sid", "value": "1"}}, "SetCookie"},
		{"cookies_clear", "cookies_clear", nil, "ClearCookies"},
		{"wait_ms", "wait_ms", map[string]any{"ms": float64(1)}, "WaitMS"},
		{"wait_for_selector", "wait_for_selector", map[string]any{"selector": "#a", "timeout": float64(1)}, "WaitForSelector"},
		{"wait_for_text", "wait_for_text", map[string]any{"text": "ok", "timeout": float64(1)}, "WaitForText"},
		{"wait_for_url", "wait_for_url", map[string]any{"url": "/ok", "timeout": float64(1)}, "WaitForURL"},
		{"tab_new", "tab_new", map[string]any{"url": "about:blank"}, "NewTab"},
		{"tab_close", "tab_close", map[string]any{"targetId": "target-1"}, "CloseTab"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start := len(f.calls)
			resp := s.executeCommand(context.Background(), tc.action, tc.req)
			require.True(t, resp.Success, resp.Error)
			require.Greater(t, len(f.calls), start)
			assert.Equal(t, tc.call, f.calls[len(f.calls)-1].name)
		})
	}
}

func TestExecuteCommandDataResponses(t *testing.T) {
	f := &fakeBrowser{
		url:        "https://example.com",
		title:      "Example",
		text:       "Text",
		html:       "<html></html>",
		value:      "Value",
		attr:       "Attr",
		visible:    true,
		enabled:    false,
		checked:    true,
		eval:       float64(42),
		snapshot:   "tree",
		screenshot: []byte("png"),
		cookies:    []protocol.Cookie{{Name: "sid", Value: "1"}},
		targetID:   "target-1",
	}
	s := testServer(f)

	assertJSONData(t, s.executeCommand(context.Background(), "get_url", nil), "https://example.com")
	assertJSONData(t, s.executeCommand(context.Background(), "get_title", nil), "Example")
	assertJSONData(t, s.executeCommand(context.Background(), "get_text", map[string]any{"selector": "h1"}), "Text")
	assertJSONData(t, s.executeCommand(context.Background(), "get_html", nil), "<html></html>")
	assertJSONData(t, s.executeCommand(context.Background(), "get_value", map[string]any{"selector": "input"}), "Value")
	assertJSONData(t, s.executeCommand(context.Background(), "get_attr", map[string]any{"selector": "a", "attr": "href"}), "Attr")
	assertJSONData(t, s.executeCommand(context.Background(), "is_visible", map[string]any{"selector": "main"}), true)
	assertJSONData(t, s.executeCommand(context.Background(), "is_enabled", map[string]any{"selector": "button"}), false)
	assertJSONData(t, s.executeCommand(context.Background(), "is_checked", map[string]any{"selector": "input"}), true)
	assertJSONData(t, s.executeCommand(context.Background(), "eval", map[string]any{"expression": "21*2"}), float64(42))
	assertJSONData(t, s.executeCommand(context.Background(), "snapshot", nil), "tree")

	resp := s.executeCommand(context.Background(), "screenshot", nil)
	require.True(t, resp.Success)
	var screenshot struct {
		Data   []byte `json:"data"`
		Format string `json:"format"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &screenshot))
	assert.Equal(t, []byte("png"), screenshot.Data)
	assert.Equal(t, "png", screenshot.Format)

	var tab map[string]string
	resp = s.executeCommand(context.Background(), "tab_new", nil)
	require.True(t, resp.Success)
	require.NoError(t, json.Unmarshal(resp.Data, &tab))
	assert.Equal(t, "target-1", tab["targetId"])
}

func TestExecuteCommandValidationAndErrors(t *testing.T) {
	s := testServer(&fakeBrowser{})

	for _, tc := range []struct {
		action string
		req    map[string]any
		want   string
	}{
		{"navigate", nil, "missing url"},
		{"click", nil, "missing selector"},
		{"fill", nil, "missing selector"},
		{"type", nil, "missing selector"},
		{"press", nil, "missing key"},
		{"hover", nil, "missing selector"},
		{"focus", nil, "missing selector"},
		{"check", nil, "missing selector"},
		{"uncheck", nil, "missing selector"},
		{"select", nil, "missing selector"},
		{"eval", nil, "missing expression"},
		{"get_text", nil, "missing selector"},
		{"get_value", nil, "missing selector"},
		{"get_attr", nil, "missing selector or attr"},
		{"is_visible", nil, "missing selector"},
		{"is_enabled", nil, "missing selector"},
		{"is_checked", nil, "missing selector"},
		{"set_viewport", nil, "missing width or height"},
		{"cookies_set", nil, "missing cookie name"},
		{"wait_ms", nil, "missing ms"},
		{"wait_for_selector", nil, "missing selector"},
		{"wait_for_text", nil, "missing text"},
		{"wait_for_url", nil, "missing url"},
		{"tab_close", nil, "missing targetId"},
		{"unknown", nil, "unknown action"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			resp := s.executeCommand(context.Background(), tc.action, tc.req)
			require.False(t, resp.Success)
			assert.Contains(t, resp.Error, tc.want)
		})
	}

	noBrowser := testServer(nil)
	resp := noBrowser.executeCommand(context.Background(), "ping", nil)
	require.False(t, resp.Success)
	assert.Contains(t, resp.Error, "browser not connected")
}

func TestWriteResponseAndHandleConnection(t *testing.T) {
	s := testServer(&fakeBrowser{url: "https://example.com"})
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go s.writeResponse(server, protocol.Response{Success: true, Data: json.RawMessage(`"ok"`)})
	var resp protocol.Response
	require.NoError(t, json.NewDecoder(client).Decode(&resp))
	assert.True(t, resp.Success)

	client2, server2 := net.Pipe()
	defer client2.Close()
	go s.handleConnection(context.Background(), server2)
	_, err := client2.Write([]byte("not-json\n"))
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(client2).Decode(&resp))
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "invalid JSON")
}

func TestNewServerAndShutdown(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	s, err := NewServer(&Options{Session: "test", SocketDir: dir, Version: "v1", Logger: logger})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "test.sock"), s.SocketPath())
	assert.NotNil(t, s.Done())

	f := &fakeBrowser{}
	s.browser = f
	require.NoError(t, os.WriteFile(s.pidPath, []byte("123"), 0644))
	require.NoError(t, os.WriteFile(s.socketPath, []byte("sock"), 0644))
	s.Shutdown()
	assert.True(t, f.closed)
	_, err = os.Stat(s.pidPath)
	assert.True(t, os.IsNotExist(err))
}

func TestNewServerDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VIBE_BROWSER_SOCKET_DIR", dir)

	s, err := NewServer(nil)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "default.sock"), s.SocketPath())
	assert.Equal(t, filepath.Join(dir, "default.pid"), s.pidPath)
	assert.NotNil(t, s.logger)
}

func TestServerStartAcceptsCommandsAndShutdown(t *testing.T) {
	dir := t.TempDir()
	listener := newFakeListener()
	proc := &fakeChromeProc{url: "ws://launched"}
	fake := &fakeBrowser{url: "https://example.com"}
	stubDaemonStartDeps(t,
		func(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (daemonChromeProcess, error) {
			assert.Equal(t, chrome.BrowserBrave, opts.Browser)
			assert.Equal(t, "/bin/browser", opts.ExecutablePath)
			assert.Equal(t, []string{"--custom"}, opts.Args)
			assert.Equal(t, "http://proxy", opts.Proxy)
			assert.Equal(t, "/tmp/profile", opts.UserDataDir)
			assert.Equal(t, 800, opts.ViewportWidth)
			assert.Equal(t, 600, opts.ViewportHeight)
			assert.Equal(t, []string{"/ext"}, opts.Extensions)
			assert.Equal(t, "Profile 1", opts.Profile)
			return proc, nil
		},
		func(ctx context.Context, wsURL string, logger *slog.Logger) (browserSession, error) {
			assert.Equal(t, "ws://launched", wsURL)
			return fake, nil
		},
		func(network, address string) (net.Listener, error) {
			assert.Equal(t, "unix", network)
			assert.Equal(t, filepath.Join(dir, "sdk.sock"), address)
			return listener, nil
		},
	)

	s, err := NewServer(&Options{Session: "sdk", SocketDir: dir})
	require.NoError(t, err)
	require.NoError(t, s.Start(context.Background(), &protocol.LaunchOptions{
		Browser:        protocol.BrowserBrave,
		ExecutablePath: "/bin/browser",
		Args:           []string{"--custom"},
		Proxy:          "http://proxy",
		UserDataDir:    "/tmp/profile",
		ViewportWidth:  800,
		ViewportHeight: 600,
		Extensions:     []string{"/ext"},
		Profile:        "Profile 1",
	}))
	_, err = os.Stat(filepath.Join(dir, "sdk.pid"))
	require.NoError(t, err)

	client, server := net.Pipe()
	listener.conns <- server
	_, err = client.Write([]byte(`{"action":"ping"}` + "\n"))
	require.NoError(t, err)
	var resp protocol.Response
	require.NoError(t, json.NewDecoder(client).Decode(&resp))
	assert.True(t, resp.Success)
	assert.JSONEq(t, `"pong"`, string(resp.Data))
	require.NoError(t, client.Close())

	s.Shutdown()
	assert.True(t, fake.closed)
	assert.True(t, proc.killed)
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServerStartErrors(t *testing.T) {
	t.Run("launch", func(t *testing.T) {
		stubDaemonStartDeps(t,
			func(context.Context, chrome.LaunchOptions, *slog.Logger) (daemonChromeProcess, error) {
				return nil, errors.New("launch failed")
			},
			nil,
			nil,
		)
		s, err := NewServer(&Options{Session: "launch", SocketDir: t.TempDir()})
		require.NoError(t, err)
		err = s.Start(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "launch failed")
	})

	t.Run("connect kills process", func(t *testing.T) {
		proc := &fakeChromeProc{url: "ws://launched"}
		stubDaemonStartDeps(t,
			func(context.Context, chrome.LaunchOptions, *slog.Logger) (daemonChromeProcess, error) {
				return proc, nil
			},
			func(context.Context, string, *slog.Logger) (browserSession, error) {
				return nil, errors.New("connect failed")
			},
			nil,
		)
		s, err := NewServer(&Options{Session: "connect", SocketDir: t.TempDir()})
		require.NoError(t, err)
		err = s.Start(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connect failed")
		assert.True(t, proc.killed)
	})

	t.Run("listen closes browser", func(t *testing.T) {
		fake := &fakeBrowser{}
		stubDaemonStartDeps(t,
			func(context.Context, chrome.LaunchOptions, *slog.Logger) (daemonChromeProcess, error) {
				return &fakeChromeProc{url: "ws://launched"}, nil
			},
			func(context.Context, string, *slog.Logger) (browserSession, error) {
				return fake, nil
			},
			func(string, string) (net.Listener, error) {
				return nil, errors.New("listen failed")
			},
		)
		s, err := NewServer(&Options{Session: "listen", SocketDir: t.TempDir()})
		require.NoError(t, err)
		err = s.Start(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "listen failed")
		assert.True(t, fake.closed)
	})
}

func TestGetSocketDirEnv(t *testing.T) {
	t.Setenv("VIBE_BROWSER_SOCKET_DIR", "/tmp/vibe-daemon-test")
	assert.Equal(t, "/tmp/vibe-daemon-test", getSocketDir())

	dir := t.TempDir()
	t.Setenv("VIBE_BROWSER_SOCKET_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", dir)
	assert.Equal(t, filepath.Join(dir, "vibe-browser"), getSocketDir())

	home := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", home)
	assert.Equal(t, filepath.Join(home, ".vibe-browser"), getSocketDir())
}

func testServer(b browserSession) *Server {
	return &Server{
		session:    "test",
		socketPath: "test.sock",
		pidPath:    "test.pid",
		browser:    b,
		logger:     slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)),
		done:       make(chan struct{}),
	}
}

func assertJSONData(t *testing.T, resp protocol.Response, want any) {
	t.Helper()
	require.True(t, resp.Success, resp.Error)
	data, err := json.Marshal(want)
	require.NoError(t, err)
	assert.JSONEq(t, string(data), string(resp.Data))
}

type fakeChromeProc struct {
	url    string
	killed bool
}

func (p *fakeChromeProc) Kill()                   { p.killed = true }
func (p *fakeChromeProc) CDPWebSocketURL() string { return p.url }

type fakeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newFakeListener() *fakeListener {
	return &fakeListener{
		conns: make(chan net.Conn, 1),
		done:  make(chan struct{}),
	}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *fakeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *fakeListener) Addr() net.Addr { return fakeAddr("unix") }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }

func stubDaemonStartDeps(
	t *testing.T,
	launch func(context.Context, chrome.LaunchOptions, *slog.Logger) (daemonChromeProcess, error),
	connect func(context.Context, string, *slog.Logger) (browserSession, error),
	listen func(string, string) (net.Listener, error),
) {
	t.Helper()
	oldLaunch := launchBrowser
	oldConnect := connectBrowserCDP
	oldListen := listenUnix
	if launch != nil {
		launchBrowser = launch
	}
	if connect != nil {
		connectBrowserCDP = connect
	}
	if listen != nil {
		listenUnix = listen
	}
	t.Cleanup(func() {
		launchBrowser = oldLaunch
		connectBrowserCDP = oldConnect
		listenUnix = oldListen
	})
}
