// Package chrome handles launching and managing Chrome/Chromium-based browsers.
//
// It supports multiple browsers: Chrome, Chromium, Brave, Edge, and their variants.
// The discovery mechanism tries multiple methods to find a running browser or launch a new one.
package chrome

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BrowserType represents a supported browser type.
type BrowserType string

const (
	BrowserChrome       BrowserType = "chrome"
	BrowserChromium     BrowserType = "chromium"
	BrowserBrave        BrowserType = "brave"
	BrowserEdge         BrowserType = "edge"
	BrowserChromeCanary BrowserType = "chrome-canary"
)

// DefaultCDPPort is the default Chrome DevTools Protocol port.
const DefaultCDPPort = 9222

// AlternateCDPPort is the fallback port for CDP.
const AlternateCDPPort = 9229

// LaunchOptions configures how a browser is launched.
type LaunchOptions struct {
	// Browser type to launch (default: auto-detect)
	Browser BrowserType

	// ExecutablePath overrides the auto-detected browser path
	ExecutablePath string

	// Headless mode (default: true)
	Headless bool

	// Args are additional browser arguments
	Args []string

	// Proxy server URL
	Proxy string

	// UserDataDir is the browser profile directory
	UserDataDir string

	// Profile is the Chrome profile name (e.g., "Default", "Profile 1")
	Profile string

	// ViewportWidth and ViewportHeight set the initial window size
	ViewportWidth  int
	ViewportHeight int

	// Extensions to load
	Extensions []string

	// RemotePort for CDP (default: 0 = auto)
	RemotePort int

	// RemoteAddress to bind CDP (default: 127.0.0.1)
	RemoteAddress string
}

// Process represents a running browser process.
type Process struct {
	cmd         *exec.Cmd
	Browser     BrowserType
	Executable  string
	UserDataDir string
	CDPURL      string
	RemotePort  int
	PID         int
}

// CDPWebSocketURL returns the browser-level DevTools WebSocket URL.
func (p *Process) CDPWebSocketURL() string {
	return p.CDPURL
}

// DiscoverCDPURL discovers the CDP WebSocket URL from a running browser.
// It tries multiple discovery methods in order:
// 1. DevToolsActivePort file
// 2. HTTP /json/version endpoint
// 3. HTTP /json/list endpoint
// Returns error if no browser is found.
func DiscoverCDPURL(host string, port int) (string, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = DefaultCDPPort
	}

	// Method 1: Try /json/version
	wsURL, err := discoverFromVersionEndpoint(host, port)
	if err == nil {
		return wsURL, nil
	}

	// Method 2: Try /json/list
	wsURL, err = discoverFromListEndpoint(host, port)
	if err == nil {
		return wsURL, nil
	}

	return "", fmt.Errorf("no browser found on %s:%d (is Chrome running with --remote-debugging-port=%d?)", host, port, port)
}

// discoverFromVersionEndpoint tries to get CDP URL from /json/version.
func discoverFromVersionEndpoint(host string, port int) (string, error) {
	url := fmt.Sprintf("http://%s:%d/json/version", host, port)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("cannot reach /json/version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var info struct {
		WebBrowserDebuggerURL string `json:"webSocketDebuggerUrl"`
		Browser               string `json:"Browser"`
		ProtocolVersion       string `json:"Protocol-Version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}

	if info.WebBrowserDebuggerURL != "" {
		// Rewrite host/port to match requested target
		return rewriteWSHost(info.WebBrowserDebuggerURL, host, port), nil
	}

	return "", fmt.Errorf("no webSocketDebuggerUrl in response")
}

// discoverFromListEndpoint tries to get CDP URL from /json/list.
func discoverFromListEndpoint(host string, port int) (string, error) {
	url := fmt.Sprintf("http://%s:%d/json/list", host, port)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("cannot reach /json/list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var targets []struct {
		ID                    string `json:"id"`
		Type                  string `json:"type"`
		WebBrowserDebuggerURL string `json:"webSocketDebuggerUrl"`
		URL                   string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}

	// Find the browser target
	for _, t := range targets {
		if t.Type == "browser" && t.WebBrowserDebuggerURL != "" {
			return rewriteWSHost(t.WebBrowserDebuggerURL, host, port), nil
		}
	}

	// Fallback: return first target with a WebSocket URL
	for _, t := range targets {
		if t.WebBrowserDebuggerURL != "" {
			return rewriteWSHost(t.WebBrowserDebuggerURL, host, port), nil
		}
	}

	return "", fmt.Errorf("no suitable target found")
}

// rewriteWSHost rewrites the host and port in a WebSocket URL.
func rewriteWSHost(wsURL, host string, port int) string {
	// Simple rewrite: replace ws://HOST:PORT
	// This handles the common case where Chrome returns ws://127.0.0.1:XXXXX
	if strings.HasPrefix(wsURL, "ws://") {
		parts := strings.SplitN(wsURL[5:], "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("ws://%s:%d/%s", host, port, parts[1])
		}
	}
	if strings.HasPrefix(wsURL, "wss://") {
		parts := strings.SplitN(wsURL[6:], "/", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("wss://%s:%d/%s", host, port, parts[1])
		}
	}
	return wsURL
}

// AutoConnectCDP tries to find a running browser and return its CDP URL.
// It checks:
// 1. DevToolsActivePort files in user data directories
// 2. Common CDP ports (9222, 9229)
func AutoConnectCDP() (string, error) {
	// Method 1: Check DevToolsActivePort files
	for _, dir := range getUserDataDirs() {
		port, _ := readDevToolsActivePort(dir)
		if port > 0 {
			wsURL, err := DiscoverCDPURL("127.0.0.1", port)
			if err == nil {
				return wsURL, nil
			}
		}
	}

	// Method 2: Probe common ports
	for _, port := range []int{DefaultCDPPort, AlternateCDPPort} {
		wsURL, err := DiscoverCDPURL("127.0.0.1", port)
		if err == nil {
			return wsURL, nil
		}
	}

	return "", fmt.Errorf("no running browser found; launch Chrome with --remote-debugging-port=%d or use --cdp-url", DefaultCDPPort)
}

// getUserDataDirs returns potential browser user data directories.
func getUserDataDirs() []string {
	var dirs []string

	home, err := os.UserHomeDir()
	if err != nil {
		return dirs
	}

	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		dirs = append(dirs,
			filepath.Join(base, "Google", "Chrome"),
			filepath.Join(base, "Google", "Chrome Canary"),
			filepath.Join(base, "Chromium"),
			filepath.Join(base, "BraveSoftware", "Brave-Browser"),
			filepath.Join(base, "Microsoft Edge"),
		)
	case "linux":
		config := filepath.Join(home, ".config")
		dirs = append(dirs,
			filepath.Join(config, "google-chrome"),
			filepath.Join(config, "google-chrome-unstable"),
			filepath.Join(config, "chromium"),
			filepath.Join(config, "BraveSoftware", "Brave-Browser"),
			filepath.Join(config, "microsoft-edge"),
			filepath.Join(config, "microsoft-edge-dev"),
		)
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			base := filepath.Join(local)
			dirs = append(dirs,
				filepath.Join(base, "Google", "Chrome", "User Data"),
				filepath.Join(base, "Google", "Chrome SxS", "User Data"),
				filepath.Join(base, "Chromium", "User Data"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "User Data"),
				filepath.Join(base, "Microsoft", "Edge", "User Data"),
			)
		}
	}

	return dirs
}

// readDevToolsActivePort reads the DevToolsActivePort file from a user data directory.
func readDevToolsActivePort(userDir string) (int, string) {
	portFile := filepath.Join(userDir, "DevToolsActivePort")
	data, err := os.ReadFile(portFile)
	if err != nil {
		return 0, ""
	}

	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return 0, ""
	}

	var port int
	fmt.Sscanf(strings.TrimSpace(lines[0]), "%d", &port)

	wsPath := ""
	if len(lines) > 1 {
		wsPath = strings.TrimSpace(lines[1])
	}

	return port, wsPath
}

// FindBrowser finds a browser executable on the system.
func FindBrowser(browserType BrowserType) (string, error) {
	if browserType == "" {
		browserType = BrowserChrome
	}

	candidates := getBrowserCandidates(browserType)
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
		// Also check absolute paths
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("%s not found; install it or use --executable-path", browserType)
}

// getBrowserCandidates returns potential executable names/paths for a browser type.
func getBrowserCandidates(browserType BrowserType) []string {
	switch runtime.GOOS {
	case "darwin":
		return getDarwinCandidates(browserType)
	case "linux":
		return getLinuxCandidates(browserType)
	case "windows":
		return getWindowsCandidates(browserType)
	default:
		return []string{string(browserType)}
	}
}

func getDarwinCandidates(browserType BrowserType) []string {
	switch browserType {
	case BrowserChrome:
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"google-chrome",
		}
	case BrowserChromeCanary:
		return []string{
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
	case BrowserChromium:
		return []string{
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"chromium",
		}
	case BrowserBrave:
		return []string{
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"brave-browser",
		}
	case BrowserEdge:
		return []string{
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	default:
		return nil
	}
}

func getLinuxCandidates(browserType BrowserType) []string {
	switch browserType {
	case BrowserChrome:
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/opt/google/chrome/chrome",
		}
	case BrowserChromium:
		return []string{
			"chromium-browser",
			"chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
			"/snap/bin/chromium",
		}
	case BrowserBrave:
		return []string{
			"brave-browser",
			"brave-browser-stable",
			"/usr/bin/brave-browser",
		}
	case BrowserEdge:
		return []string{
			"microsoft-edge",
			"microsoft-edge-stable",
			"/usr/bin/microsoft-edge",
		}
	default:
		return nil
	}
}

func getWindowsCandidates(browserType BrowserType) []string {
	local := os.Getenv("LOCALAPPDATA")
	progFiles := os.Getenv("PROGRAMFILES")
	progFilesX86 := os.Getenv("PROGRAMFILES(X86)")

	switch browserType {
	case BrowserChrome:
		candidates := []string{
			filepath.Join(progFiles, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(progFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
		}
		if local != "" {
			candidates = append(candidates, filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"))
		}
		return candidates
	case BrowserChromium:
		candidates := []string{
			filepath.Join(progFiles, "Chromium", "Application", "chrome.exe"),
		}
		if local != "" {
			candidates = append(candidates, filepath.Join(local, "Chromium", "Application", "chrome.exe"))
		}
		return candidates
	case BrowserBrave:
		candidates := []string{}
		if local != "" {
			candidates = append(candidates, filepath.Join(local, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"))
		}
		return candidates
	case BrowserEdge:
		return []string{
			filepath.Join(progFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(progFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
		}
	default:
		return nil
	}
}

// Launch starts a browser process and returns its CDP URL.
func Launch(ctx context.Context, opts LaunchOptions, logger *slog.Logger) (*Process, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Find browser executable
	execPath := opts.ExecutablePath
	if execPath == "" {
		var err error
		execPath, err = FindBrowser(opts.Browser)
		if err != nil {
			return nil, err
		}
	}

	// Determine remote port
	remotePort := opts.RemotePort
	if remotePort == 0 {
		remotePort = DefaultCDPPort
	}

	remoteAddr := opts.RemoteAddress
	if remoteAddr == "" {
		remoteAddr = "127.0.0.1"
	}

	// Prepare user data directory
	userDataDir := opts.UserDataDir
	if userDataDir == "" {
		tmpDir, err := os.MkdirTemp("", "vibe-browser-")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		userDataDir = tmpDir
	}

	// Build arguments
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", remotePort),
		fmt.Sprintf("--remote-debugging-address=%s", remoteAddr),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-hang-monitor",
		"--disable-popup-blocking",
		"--disable-prompt-on-repost",
		"--disable-sync",
		"--disable-translate",
		"--metrics-recording-only",
		"--safebrowsing-disable-auto-update",
	}

	if opts.Headless {
		args = append(args, "--headless=new")
	}

	if opts.Proxy != "" {
		args = append(args, fmt.Sprintf("--proxy-server=%s", opts.Proxy))
	}

	if opts.ViewportWidth > 0 && opts.ViewportHeight > 0 {
		args = append(args, fmt.Sprintf("--window-size=%d,%d", opts.ViewportWidth, opts.ViewportHeight))
	}

	if len(opts.Extensions) > 0 {
		args = append(args, "--disable-extensions-except="+strings.Join(opts.Extensions, ","))
		args = append(args, "--load-extension="+strings.Join(opts.Extensions, ","))
	} else {
		args = append(args, "--disable-extensions")
	}

	if opts.Profile != "" {
		args = append(args, fmt.Sprintf("--profile-directory=%s", opts.Profile))
	}

	args = append(args, opts.Args...)

	logger.Info("launching browser",
		"exec", execPath,
		"port", remotePort,
		"headless", opts.Headless,
	)

	// Start process - use exec.Command instead of CommandContext so the process
	// continues running after the CLI exits
	cmd := exec.Command(execPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	// Set process group so it doesn't die when the parent exits
	cmd.SysProcAttr = getSysProcAttr()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start browser: %w", err)
	}

	p := &Process{
		cmd:         cmd,
		Browser:     opts.Browser,
		Executable:  execPath,
		UserDataDir: userDataDir,
		RemotePort:  remotePort,
		PID:         cmd.Process.Pid,
	}

	// Wait for CDP to become available
	cdpURL, err := waitForCDP(ctx, remoteAddr, remotePort, logger)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("wait for CDP: %w", err)
	}

	p.CDPURL = cdpURL
	logger.Info("browser ready", "cdp_url", cdpURL, "pid", p.PID)

	return p, nil
}

// waitForCDP polls the Chrome DevTools JSON endpoint until it's ready.
func waitForCDP(ctx context.Context, host string, port int, logger *slog.Logger) (string, error) {
	if host == "" {
		host = "127.0.0.1"
	}

	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			return "", fmt.Errorf("CDP not available after 30s on %s:%d", host, port)
		case <-ticker.C:
			wsURL, err := DiscoverCDPURL(host, port)
			if err == nil {
				return wsURL, nil
			}
		}
	}
}

// Kill terminates the browser process.
func (p *Process) Kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	// Clean up temp user data dir
	if p.UserDataDir != "" && strings.Contains(p.UserDataDir, "vibe-browser-") {
		os.RemoveAll(p.UserDataDir)
	}
}

// ListTargets returns the list of available DevTools targets.
func ListTargets(host string, port int) ([]map[string]any, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = DefaultCDPPort
	}

	url := fmt.Sprintf("http://%s:%d/json/list", host, port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	defer resp.Body.Close()

	var targets []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("decode targets: %w", err)
	}

	return targets, nil
}

// GetBrowserVersion returns the browser version info.
func GetBrowserVersion(host string, port int) (map[string]any, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = DefaultCDPPort
	}

	url := fmt.Sprintf("http://%s:%d/json/version", host, port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	defer resp.Body.Close()

	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode version: %w", err)
	}

	return info, nil
}

// FindChromeUserDataDir finds the first existing Chrome user data directory.
func FindChromeUserDataDir() string {
	for _, dir := range getUserDataDirs() {
		localState := filepath.Join(dir, "Local State")
		if _, err := os.Stat(localState); err == nil {
			return dir
		}
	}
	return ""
}

// ListChromeProfiles lists Chrome profiles in a user data directory.
func ListChromeProfiles(userDir string) []map[string]string {
	localState := filepath.Join(userDir, "Local State")
	data, err := os.ReadFile(localState)
	if err != nil {
		return nil
	}

	var state struct {
		Profile struct {
			InfoCache map[string]struct {
				Name string `json:"name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}

	var profiles []map[string]string
	for dir, info := range state.Profile.InfoCache {
		name := info.Name
		if name == "" {
			name = dir
		}
		profiles = append(profiles, map[string]string{
			"directory": dir,
			"name":      name,
		})
	}

	return profiles
}
