// Package chrome handles launching and managing Chrome/Chromium processes.
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

// LaunchOptions configures how Chrome is launched.
type LaunchOptions struct {
	Headless       bool
	ExecutablePath string
	Args           []string
	Proxy          string
	UserDataDir    string
	ViewportWidth  int
	ViewportHeight int
	Extensions     []string
	Profile        string
	RemotePort     int
}

// Process represents a running Chrome process.
type Process struct {
	cmd        *exec.Cmd
	UserDataDir string
	CDPURL      string
	RemotePort  int
}

// Launch starts a Chrome process and returns its CDP WebSocket URL.
func Launch(ctx context.Context, opts LaunchOptions, logger *slog.Logger) (*Process, error) {
	if logger == nil {
		logger = slog.Default()
	}

	execPath := opts.ExecutablePath
	if execPath == "" {
		var err error
		execPath, err = findChrome()
		if err != nil {
			return nil, fmt.Errorf("chrome: find executable: %w", err)
		}
	}

	if opts.RemotePort == 0 {
		opts.RemotePort = 9222
	}

	// Prepare user data directory
	userDataDir := opts.UserDataDir
	if userDataDir == "" {
		tmpDir, err := os.MkdirTemp("", "vibe-browser-")
		if err != nil {
			return nil, fmt.Errorf("chrome: create temp dir: %w", err)
		}
		userDataDir = tmpDir
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", opts.RemotePort),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-extensions-except=",
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
	}

	if opts.Profile != "" {
		args = append(args, fmt.Sprintf("--profile-directory=%s", opts.Profile))
	}

	args = append(args, opts.Args...)

	logger.Info("chrome: launching", "exec", execPath, "port", opts.RemotePort)

	cmd := exec.CommandContext(ctx, execPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("chrome: start: %w", err)
	}

	p := &Process{
		cmd:         cmd,
		UserDataDir: userDataDir,
		RemotePort:  opts.RemotePort,
	}

	// Wait for CDP to become available
	cdpURL, err := waitForCDP(ctx, opts.RemotePort, logger)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("chrome: wait for CDP: %w", err)
	}

	p.CDPURL = cdpURL
	logger.Info("chrome: ready", "cdp_url", cdpURL)

	return p, nil
}

// Kill terminates the Chrome process.
func (p *Process) Kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	// Clean up temp user data dir if we created one
	if p.UserDataDir != "" && strings.Contains(p.UserDataDir, "vibe-browser-") {
		os.RemoveAll(p.UserDataDir)
	}
}

// waitForCDP polls the Chrome DevTools JSON endpoint until it's ready.
func waitForCDP(ctx context.Context, port int, logger *slog.Logger) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := &http.Client{Timeout: 1 * time.Second}

	deadline := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			return "", fmt.Errorf("chrome: CDP not available after 30s")
		default:
		}

		resp, err := client.Get(url)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var info struct {
			WebBrowserDebuggerURL string `json:"webSocketDebuggerUrl"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return "", fmt.Errorf("chrome: decode version response: %w", err)
		}

		if info.WebBrowserDebuggerURL != "" {
			return info.WebBrowserDebuggerURL, nil
		}

		// Fallback: construct URL from port
		return fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser", port), nil
	}
}

// DiscoverCDPURL discovers the CDP URL for a running Chrome instance.
func DiscoverCDPURL(port int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("chrome: cannot reach CDP on port %d: %w", port, err)
	}
	defer resp.Body.Close()

	var info struct {
		WebBrowserDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("chrome: decode version: %w", err)
	}

	if info.WebBrowserDebuggerURL != "" {
		return info.WebBrowserDebuggerURL, nil
	}

	return fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser", port), nil
}

// ListTargets returns the list of available DevTools targets.
func ListTargets(port int) ([]map[string]any, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("chrome: list targets: %w", err)
	}
	defer resp.Body.Close()

	var targets []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("chrome: decode targets: %w", err)
	}

	return targets, nil
}

// findChrome locates the Chrome/Chromium executable on this system.
func findChrome() (string, error) {
	candidates := chromeCandidates()
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("chrome not found; install Chrome or set --executable-path")
}

func chromeCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
	case "linux":
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			filepath.Join(os.Getenv("HOME"), ".local/bin/chromium"),
		}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
		}
	default:
		return []string{"google-chrome", "chromium"}
	}
}
