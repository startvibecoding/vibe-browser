// Package daemon implements the vibe-browser daemon process.
//
// The daemon listens on a Unix domain socket and accepts JSON commands.
// It manages a Chrome browser instance and executes commands against it.
package daemon

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
	"strings"
	"sync"

	"github.com/startvibecoding/vibe-browser/internal/chrome"
	"github.com/startvibecoding/vibe-browser/pkg/browser"
	"github.com/startvibecoding/vibe-browser/pkg/cdp"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
)

// Server is the daemon server that manages browser sessions.
type Server struct {
	session    string
	socketPath string
	pidPath    string
	version    string
	listener   net.Listener
	browser    *browser.Browser
	chrome     *chrome.Process
	logger     *slog.Logger
	mu         sync.RWMutex
	done       chan struct{}
}

// Options configures the daemon server.
type Options struct {
	Session        string
	SocketDir      string
	Version        string
	Headless       bool
	ExecutablePath string
	Args           []string
	Logger         *slog.Logger
}

// NewServer creates a new daemon server.
func NewServer(opts *Options) (*Server, error) {
	if opts == nil {
		opts = &Options{}
	}

	session := opts.Session
	if session == "" {
		session = "default"
	}

	socketDir := opts.SocketDir
	if socketDir == "" {
		socketDir = getSocketDir()
	}

	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return nil, fmt.Errorf("daemon: create socket dir: %w", err)
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		session:    session,
		socketPath: filepath.Join(socketDir, session+".sock"),
		pidPath:    filepath.Join(socketDir, session+".pid"),
		version:    opts.Version,
		logger:     logger,
		done:       make(chan struct{}),
	}, nil
}

// Start starts the daemon server.
func (s *Server) Start(ctx context.Context, launchOpts *protocol.LaunchOptions) error {
	// Write PID file
	if err := os.WriteFile(s.pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return fmt.Errorf("daemon: write pid file: %w", err)
	}

	// Clean up stale socket
	os.Remove(s.socketPath)

	// Launch browser
	chromeOpts := chrome.LaunchOptions{
		Headless:       true,
		ExecutablePath: "",
	}

	if launchOpts != nil {
		chromeOpts.Headless = launchOpts.Headless
		chromeOpts.ExecutablePath = launchOpts.ExecutablePath
		chromeOpts.Args = launchOpts.Args
		chromeOpts.Proxy = launchOpts.Proxy
		chromeOpts.UserDataDir = launchOpts.UserDataDir
		chromeOpts.ViewportWidth = launchOpts.ViewportWidth
		chromeOpts.ViewportHeight = launchOpts.ViewportHeight
		chromeOpts.Extensions = launchOpts.Extensions
		chromeOpts.Profile = launchOpts.Profile
	}

	s.logger.Info("daemon: launching browser", "session", s.session)
	proc, err := chrome.Launch(ctx, chromeOpts, s.logger)
	if err != nil {
		return fmt.Errorf("daemon: launch browser: %w", err)
	}
	s.chrome = proc

	// Connect to browser
	cdpClient, err := cdp.Connect(ctx, proc.CDPURL, s.logger)
	if err != nil {
		proc.Kill()
		return fmt.Errorf("daemon: connect to browser: %w", err)
	}
	s.browser = browser.New(cdpClient, s.logger)

	// Start listening
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		cdpClient.Close()
		proc.Kill()
		return fmt.Errorf("daemon: listen: %w", err)
	}
	s.listener = listener

	// Set socket permissions
	os.Chmod(s.socketPath, 0600)

	s.logger.Info("daemon: listening", "socket", s.socketPath, "session", s.session)

	// Accept connections
	go s.acceptLoop(ctx)

	return nil
}

// acceptLoop accepts and handles incoming connections.
func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.logger.Error("daemon: accept error", "err", err)
				continue
			}
		}

		go s.handleConnection(ctx, conn)
	}
}

// handleConnection handles a single client connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeResponse(conn, protocol.Response{
				Success: false,
				Error:   fmt.Sprintf("invalid JSON: %v", err),
			})
			continue
		}

		action, _ := req["action"].(string)
		resp := s.executeCommand(ctx, action, req)
		s.writeResponse(conn, resp)
	}
}

// executeCommand executes a command and returns the response.
func (s *Server) executeCommand(ctx context.Context, action string, req map[string]any) protocol.Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.browser == nil {
		return protocol.Response{
			Success: false,
			Error:   "browser not connected",
		}
	}

	switch action {
	case "navigate", "goto":
		url, _ := req["url"].(string)
		if url == "" {
			return protocol.Response{Success: false, Error: "missing url"}
		}
		if err := s.browser.Navigate(ctx, url, nil); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "reload":
		if err := s.browser.Reload(ctx); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "back":
		if err := s.browser.GoBack(ctx); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "forward":
		if err := s.browser.GoForward(ctx); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "click":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Click(ctx, selector, nil); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "dblclick":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.DoubleClick(ctx, selector); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "fill":
		selector, _ := req["selector"].(string)
		value, _ := req["value"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Fill(ctx, selector, value); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "type":
		selector, _ := req["selector"].(string)
		text, _ := req["text"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		delay := 50
		if d, ok := req["delay"].(float64); ok {
			delay = int(d)
		}
		if err := s.browser.Type(ctx, selector, text, delay); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "press":
		key, _ := req["key"].(string)
		if key == "" {
			return protocol.Response{Success: false, Error: "missing key"}
		}
		if err := s.browser.Press(ctx, key); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "hover":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Hover(ctx, selector); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "scroll":
		deltaX := 0.0
		deltaY := 0.0
		if dx, ok := req["deltaX"].(float64); ok {
			deltaX = dx
		}
		if dy, ok := req["deltaY"].(float64); ok {
			deltaY = dy
		}
		if err := s.browser.Scroll(ctx, deltaX, deltaY); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "focus":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Focus(ctx, selector); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "check":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Check(ctx, selector); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "uncheck":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Uncheck(ctx, selector); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "select":
		selector, _ := req["selector"].(string)
		value, _ := req["value"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		if err := s.browser.Select(ctx, selector, value); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "eval":
		expression, _ := req["expression"].(string)
		if expression == "" {
			return protocol.Response{Success: false, Error: "missing expression"}
		}
		result, err := s.browser.Eval(ctx, expression)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(result)
		return protocol.Response{Success: true, Data: data}

	case "get_text":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		text, err := s.browser.GetText(ctx, selector)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(text)
		return protocol.Response{Success: true, Data: data}

	case "get_html":
		selector, _ := req["selector"].(string)
		html, err := s.browser.GetHTML(ctx, selector)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(html)
		return protocol.Response{Success: true, Data: data}

	case "get_value":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		value, err := s.browser.GetValue(ctx, selector)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(value)
		return protocol.Response{Success: true, Data: data}

	case "get_attr":
		selector, _ := req["selector"].(string)
		attr, _ := req["attr"].(string)
		if selector == "" || attr == "" {
			return protocol.Response{Success: false, Error: "missing selector or attr"}
		}
		value, err := s.browser.GetAttr(ctx, selector, attr)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(value)
		return protocol.Response{Success: true, Data: data}

	case "is_visible":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		visible, err := s.browser.IsVisible(ctx, selector)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(visible)
		return protocol.Response{Success: true, Data: data}

	case "is_enabled":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		enabled, err := s.browser.IsEnabled(ctx, selector)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(enabled)
		return protocol.Response{Success: true, Data: data}

	case "is_checked":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		checked, err := s.browser.IsChecked(ctx, selector)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(checked)
		return protocol.Response{Success: true, Data: data}

	case "get_url":
		url, err := s.browser.GetURL(ctx)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(url)
		return protocol.Response{Success: true, Data: data}

	case "get_title":
		title, err := s.browser.GetTitle(ctx)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(title)
		return protocol.Response{Success: true, Data: data}

	case "snapshot":
		snapshot, err := s.browser.Snapshot(ctx, nil)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(snapshot)
		return protocol.Response{Success: true, Data: data}

	case "screenshot":
		format := "png"
		if f, ok := req["format"].(string); ok {
			format = f
		}
		fullPage := false
		if fp, ok := req["fullPage"].(bool); ok {
			fullPage = fp
		}
		imgData, err := s.browser.Screenshot(ctx, &protocol.ScreenshotOptions{
			Format:   format,
			FullPage: fullPage,
		})
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		// Return base64 encoded image
		data, _ := json.Marshal(map[string]any{
			"data": imgData,
			"format": format,
		})
		return protocol.Response{Success: true, Data: data}

	case "set_viewport":
		width, _ := req["width"].(float64)
		height, _ := req["height"].(float64)
		if width == 0 || height == 0 {
			return protocol.Response{Success: false, Error: "missing width or height"}
		}
		scale := 1.0
		if s, ok := req["deviceScaleFactor"].(float64); ok {
			scale = s
		}
		if err := s.browser.SetViewport(ctx, int(width), int(height), scale); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "set_geolocation":
		lat, _ := req["latitude"].(float64)
		lng, _ := req["longitude"].(float64)
		accuracy := 100.0
		if a, ok := req["accuracy"].(float64); ok {
			accuracy = a
		}
		if err := s.browser.SetGeolocation(ctx, lat, lng, accuracy); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "set_offline":
		offline, _ := req["offline"].(bool)
		if err := s.browser.SetOffline(ctx, offline); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "set_headers":
		headers := make(map[string]string)
		if h, ok := req["headers"].(map[string]any); ok {
			for k, v := range h {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
		}
		if err := s.browser.SetHeaders(ctx, headers); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "cookies_get":
		cookies, err := s.browser.GetCookies(ctx)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(cookies)
		return protocol.Response{Success: true, Data: data}

	case "cookies_clear":
		if err := s.browser.ClearCookies(ctx); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "wait_ms":
		ms, _ := req["ms"].(float64)
		if ms == 0 {
			return protocol.Response{Success: false, Error: "missing ms"}
		}
		if err := s.browser.WaitMS(ctx, int(ms)); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "wait_for_selector":
		selector, _ := req["selector"].(string)
		if selector == "" {
			return protocol.Response{Success: false, Error: "missing selector"}
		}
		timeout := 30000
		if t, ok := req["timeout"].(float64); ok {
			timeout = int(t)
		}
		if err := s.browser.WaitForSelector(ctx, selector, timeout); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "wait_for_text":
		text, _ := req["text"].(string)
		if text == "" {
			return protocol.Response{Success: false, Error: "missing text"}
		}
		timeout := 30000
		if t, ok := req["timeout"].(float64); ok {
			timeout = int(t)
		}
		if err := s.browser.WaitForText(ctx, text, timeout); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "wait_for_url":
		urlPattern, _ := req["url"].(string)
		if urlPattern == "" {
			return protocol.Response{Success: false, Error: "missing url"}
		}
		timeout := 30000
		if t, ok := req["timeout"].(float64); ok {
			timeout = int(t)
		}
		if err := s.browser.WaitForURL(ctx, urlPattern, timeout); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "tab_new":
		url, _ := req["url"].(string)
		if url == "" {
			url = "about:blank"
		}
		targetID, err := s.browser.NewTab(ctx, url)
		if err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		data, _ := json.Marshal(map[string]string{"targetId": targetID})
		return protocol.Response{Success: true, Data: data}

	case "tab_close":
		targetID, _ := req["targetId"].(string)
		if targetID == "" {
			return protocol.Response{Success: false, Error: "missing targetId"}
		}
		if err := s.browser.CloseTab(ctx, targetID); err != nil {
			return protocol.Response{Success: false, Error: err.Error()}
		}
		return protocol.Response{Success: true}

	case "close":
		s.Shutdown()
		return protocol.Response{Success: true}

	case "ping":
		return protocol.Response{
			Success: true,
			Data:    json.RawMessage(`"pong"`),
		}

	default:
		return protocol.Response{
			Success: false,
			Error:   fmt.Sprintf("unknown action: %s", action),
		}
	}
}

// writeResponse writes a response to the connection.
func (s *Server) writeResponse(conn net.Conn, resp protocol.Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("daemon: marshal response error", "err", err)
		return
	}
	data = append(data, '\n')
	conn.Write(data)
}

// Shutdown gracefully shuts down the daemon.
func (s *Server) Shutdown() {
	select {
	case <-s.done:
		return // already shutting down
	default:
	}

	s.logger.Info("daemon: shutting down", "session", s.session)

	if s.browser != nil {
		s.browser.Close()
	}
	if s.chrome != nil {
		s.chrome.Kill()
	}
	if s.listener != nil {
		s.listener.Close()
	}

	// Clean up files
	os.Remove(s.socketPath)
	os.Remove(s.pidPath)
	os.Remove(strings.TrimSuffix(s.socketPath, ".sock") + ".version")

	close(s.done)
}

// Done returns a channel that is closed when the server shuts down.
func (s *Server) Done() <-chan struct{} {
	return s.done
}

// SocketPath returns the Unix socket path.
func (s *Server) SocketPath() string {
	return s.socketPath
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
