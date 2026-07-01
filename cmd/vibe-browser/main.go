// vibe-browser is a fast browser automation CLI and Go SDK for AI agents.
//
// Usage:
//
//	vibe-browser [command] [flags]
//
// Commands:
//
//	open, navigate, back, forward, reload
//	click, dblclick, fill, type, press, hover, scroll, focus
//	check, uncheck, select
//	snapshot, screenshot
//	get (text, html, value, attr, url, title)
//	is (visible, enabled, checked)
//	eval, wait, set, cookies
//	daemon, mcp, close
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/startvibecoding/vibe-browser/internal/chrome"
	"github.com/startvibecoding/vibe-browser/pkg/browser"
	"github.com/startvibecoding/vibe-browser/pkg/cdp"
	"github.com/startvibecoding/vibe-browser/pkg/daemon"
	"github.com/startvibecoding/vibe-browser/pkg/mcp"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
)

var (
	version = "0.1.0"
	logger  *slog.Logger
)

type daemonServer interface {
	Start(context.Context, *protocol.LaunchOptions) error
	Shutdown()
	Done() <-chan struct{}
	SocketPath() string
}

type mcpServer interface {
	Run(context.Context) error
}

type closeCDPClient interface {
	Send(context.Context, string, any) (*cdp.Message, error)
	Close() error
}

var (
	connectBrowser        = connectBrowserDefault
	autoConnectCDP        = chrome.AutoConnectCDP
	newDaemonServer       = func(opts *daemon.Options) (daemonServer, error) { return daemon.NewServer(opts) }
	newMCPServer          = func(logger *slog.Logger, session string) mcpServer { return mcp.NewServer(logger, session) }
	closeBrowser          = closeBrowserByCDP
	findBrowser           = chrome.FindBrowser
	findChromeUserDataDir = chrome.FindChromeUserDataDir
	listChromeProfiles    = chrome.ListChromeProfiles
	launchChrome          = chrome.Launch
	connectToCDP          = browser.ConnectToCDP
	connectCDPClient      = func(ctx context.Context, wsURL string, logger *slog.Logger) (closeCDPClient, error) {
		return cdp.Connect(ctx, wsURL, logger)
	}
	currentBrowserType string
	osExit             = os.Exit
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	setupLogger()
	osExit(run(ctx, os.Args[1:]))
}

func setupLogger() {
	logLevel := slog.LevelInfo
	if os.Getenv("VIBE_BROWSER_DEBUG") != "" {
		logLevel = slog.LevelDebug
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
}

func run(ctx context.Context, argv []string) int {
	if logger == nil {
		setupLogger()
	}

	if len(argv) < 1 {
		printUsage()
		return 1
	}

	command := argv[0]
	args := argv[1:]

	// Global flags
	var cdpURL string
	var session string
	var headless bool
	var execPath string
	var browserType string

	// Parse global flags from environment
	if v := os.Getenv("VIBE_BROWSER_CDP_URL"); v != "" {
		cdpURL = v
	}
	if v := os.Getenv("VIBE_BROWSER_SESSION"); v != "" {
		session = v
	} else {
		session = "default"
	}
	if v := os.Getenv("VIBE_BROWSER_BROWSER"); v != "" {
		browserType = v
	}
	headless = true

	// Parse command-specific flags
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cdpURL, "cdp-url", cdpURL, "Chrome DevTools Protocol WebSocket URL")
	fs.StringVar(&session, "session", session, "Session name")
	fs.BoolVar(&headless, "headless", headless, "Run browser in headless mode")
	fs.StringVar(&execPath, "executable-path", execPath, "Path to Chrome executable")
	fs.StringVar(&browserType, "browser", browserType, "Browser type (chrome, chromium, brave, edge)")

	// Command-specific flags
	var value string
	var delay int
	var format string
	var fullPage bool
	var output string
	var interactive bool
	var compact bool
	var width, height int
	var scrollX, scrollY float64

	switch command {
	case "fill", "select":
		fs.StringVar(&value, "value", "", "Value to fill/select")
	case "type":
		fs.IntVar(&delay, "delay", 50, "Delay between keystrokes in ms")
	case "screenshot":
		fs.StringVar(&format, "format", "png", "Image format (png, jpeg, webp)")
		fs.BoolVar(&fullPage, "full-page", false, "Capture full scrollable page")
		fs.StringVar(&output, "output", "", "Output file path")
		fs.StringVar(&output, "o", "", "Output file path")
	case "snapshot":
		fs.BoolVar(&interactive, "interactive", false, "Only show interactive elements")
		fs.BoolVar(&interactive, "i", false, "Only show interactive elements")
		fs.BoolVar(&compact, "compact", false, "Compact output")
	case "set":
		if hasArg(args, "viewport") {
			fs.IntVar(&width, "width", 1280, "Viewport width")
			fs.IntVar(&height, "height", 720, "Viewport height")
		}
	case "scroll":
		fs.Float64Var(&scrollX, "x", 0, "Horizontal scroll amount")
		fs.Float64Var(&scrollY, "y", 100, "Vertical scroll amount")
	}

	parseArgs := normalizeFlagArgs(fs, args)
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	args = fs.Args()

	previousBrowserType := currentBrowserType
	currentBrowserType = browserType
	defer func() {
		currentBrowserType = previousBrowserType
	}()

	var err error
	switch command {
	case "open":
		err = cmdOpen(ctx, cdpURL, session, headless, execPath, args)
	case "navigate", "goto":
		err = cmdNavigate(ctx, cdpURL, session, headless, execPath, args)
	case "back":
		err = cmdBack(ctx, cdpURL, session, headless, execPath)
	case "forward":
		err = cmdForward(ctx, cdpURL, session, headless, execPath)
	case "reload":
		err = cmdReload(ctx, cdpURL, session, headless, execPath)
	case "click":
		err = cmdClick(ctx, cdpURL, session, headless, execPath, args)
	case "dblclick":
		err = cmdDoubleClick(ctx, cdpURL, session, headless, execPath, args)
	case "fill":
		err = cmdFill(ctx, cdpURL, session, headless, execPath, args, value)
	case "type":
		err = cmdType(ctx, cdpURL, session, headless, execPath, args, delay)
	case "press":
		err = cmdPress(ctx, cdpURL, session, headless, execPath, args)
	case "hover":
		err = cmdHover(ctx, cdpURL, session, headless, execPath, args)
	case "scroll":
		err = cmdScroll(ctx, cdpURL, session, headless, execPath, scrollX, scrollY)
	case "focus":
		err = cmdFocus(ctx, cdpURL, session, headless, execPath, args)
	case "check":
		err = cmdCheck(ctx, cdpURL, session, headless, execPath, args)
	case "uncheck":
		err = cmdUncheck(ctx, cdpURL, session, headless, execPath, args)
	case "select":
		err = cmdSelect(ctx, cdpURL, session, headless, execPath, args, value)
	case "snapshot":
		err = cmdSnapshot(ctx, cdpURL, session, headless, execPath, interactive, compact)
	case "screenshot":
		err = cmdScreenshot(ctx, cdpURL, session, headless, execPath, format, fullPage, output)
	case "get":
		err = cmdGet(ctx, cdpURL, session, headless, execPath, args)
	case "is":
		err = cmdIs(ctx, cdpURL, session, headless, execPath, args)
	case "eval":
		err = cmdEval(ctx, cdpURL, session, headless, execPath, args)
	case "wait":
		err = cmdWait(ctx, cdpURL, session, headless, execPath, args)
	case "set":
		err = cmdSet(ctx, cdpURL, session, headless, execPath, args, width, height)
	case "cookies":
		err = cmdCookies(ctx, cdpURL, session, headless, execPath, args)
	case "daemon":
		err = cmdDaemon(ctx, session, headless, execPath)
	case "mcp":
		err = cmdMCP(ctx, session)
	case "close":
		err = cmdClose(ctx, cdpURL, session, headless, execPath)
	case "discover":
		err = cmdDiscover()
	case "browsers":
		err = cmdListBrowsers()
	case "profiles":
		err = cmdListProfiles()
	case "version":
		fmt.Printf("vibe-browser version %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		return 1
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Print(`vibe-browser - Fast browser automation CLI and Go SDK for AI agents

Usage:
  vibe-browser <command> [flags] [args...]

Commands:
  open [url]              Open a browser and optionally navigate
  navigate <url>          Navigate to a URL
  back                    Go back in history
  forward                 Go forward in history
  reload                  Reload the page
  click <selector>        Click an element
  dblclick <selector>     Double-click an element
  fill <selector>         Fill an input (--value required)
  type <selector> <text>  Type text into an element
  press <key>             Press a keyboard key
  hover <selector>        Hover over an element
  scroll                  Scroll the page (--x, --y)
  focus <selector>        Focus an element
  check <selector>        Check a checkbox
  uncheck <selector>      Uncheck a checkbox
  select <selector>       Select option (--value required)
  snapshot                Capture accessibility tree
  screenshot              Capture screenshot
  get <subcommand>        Get element properties
  is <subcommand>         Check element state
  eval <expression>       Evaluate JavaScript
  wait <subcommand>       Wait for conditions
  set <subcommand>        Set browser properties
  cookies <subcommand>    Manage cookies
  discover                Discover running browser CDP URL
  browsers                List available browsers
  profiles                List Chrome profiles
  daemon                  Start daemon server
  mcp                     Start MCP server
  close                   Close browser session
  version                 Show version
  help                    Show this help

Flags:
  --cdp-url string        Chrome DevTools Protocol WebSocket URL
  --session string        Session name (default "default")
  --headless              Run in headless mode (default true)
  --executable-path       Path to Chrome executable
  --browser string        Browser type (chrome, chromium, brave, edge)

Environment Variables:
  VIBE_BROWSER_CDP_URL    Same as --cdp-url
  VIBE_BROWSER_SESSION    Same as --session
  VIBE_BROWSER_BROWSER    Same as --browser
  VIBE_BROWSER_DEBUG      Enable debug logging
  VIBE_BROWSER_SOCKET_DIR Override socket directory
  CHROME_PATH             Path to Chrome executable

Browser Discovery:
  vibe-browser discover                   Auto-detect running browser
  vibe-browser browsers                   List installed browsers
  vibe-browser profiles                   List Chrome profiles
  vibe-browser open --cdp-url ws://...    Connect to specific CDP endpoint

Supported Browsers:
  Chrome, Chromium, Brave, Edge, Chrome Canary

Examples:
  # Open a page in Chrome (default)
  vibe-browser open https://example.com

  # Open in Brave
  vibe-browser open --browser brave https://example.com

  # Connect to running browser
  vibe-browser open --cdp-url ws://127.0.0.1:9222/devtools/browser

  # Take a snapshot
  vibe-browser snapshot -i

  # Click an element
  vibe-browser click "button.submit"

  # Take a screenshot
  vibe-browser screenshot -o page.png
`)
}

// connectBrowserDefault connects to a browser via CDP URL or launches one.
func connectBrowserDefault(ctx context.Context, cdpURL, session string, headless bool, execPath string, browserType ...string) (*browser.Browser, error) {
	// If CDP URL is provided, connect directly
	if cdpURL != "" {
		return connectToCDP(ctx, cdpURL, logger)
	}

	// Try to auto-connect to a running browser
	wsURL, err := autoConnectCDP()
	if err == nil {
		logger.Info("auto-connected to browser", "url", wsURL)
		b, err := connectToCDP(ctx, wsURL, logger)
		if err != nil {
			return nil, err
		}
		// For auto-connected browsers, we don't have a process to kill
		// The user started it themselves
		return b, nil
	}

	// Launch a new browser
	bt := chrome.BrowserChrome
	requestedBrowser := currentBrowserType
	if len(browserType) > 0 && browserType[0] != "" {
		requestedBrowser = browserType[0]
	}
	if requestedBrowser != "" {
		bt = chrome.BrowserType(requestedBrowser)
	}

	proc, err := launchChrome(ctx, chrome.LaunchOptions{
		Browser:        bt,
		ExecutablePath: execPath,
		Headless:       headless,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	b, err := connectToCDP(ctx, proc.CDPURL, logger)
	if err != nil {
		proc.Kill() // Kill the process if we can't connect
		return nil, err
	}

	// Set the process killer so the browser is killed when Close() is called
	b.SetProcessKiller(func() {
		proc.Kill()
	})

	return b, nil
}

// Command implementations

func cmdOpen(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	if len(args) > 0 {
		if err := b.Navigate(ctx, args[0], nil); err != nil {
			return err
		}
	}
	url, _ := b.GetURL(ctx)
	fmt.Println(url)
	return nil
}

func cmdNavigate(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser navigate <url>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	if err := b.Navigate(ctx, args[0], nil); err != nil {
		return err
	}
	url, _ := b.GetURL(ctx)
	fmt.Println(url)
	return nil
}

func cmdBack(ctx context.Context, cdpURL, session string, headless bool, execPath string) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	if err := b.GoBack(ctx); err != nil {
		return err
	}
	url, _ := b.GetURL(ctx)
	fmt.Println(url)
	return nil
}

func cmdForward(ctx context.Context, cdpURL, session string, headless bool, execPath string) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	if err := b.GoForward(ctx); err != nil {
		return err
	}
	url, _ := b.GetURL(ctx)
	fmt.Println(url)
	return nil
}

func cmdReload(ctx context.Context, cdpURL, session string, headless bool, execPath string) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Reload(ctx)
}

func cmdClick(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser click <selector>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Click(ctx, args[0], nil)
}

func cmdDoubleClick(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser dblclick <selector>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.DoubleClick(ctx, args[0])
}

func cmdFill(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string, value string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser fill <selector> --value <value>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Fill(ctx, args[0], value)
}

func cmdType(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string, delay int) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: vibe-browser type <selector> <text>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Type(ctx, args[0], args[1], delay)
}

func cmdPress(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser press <key>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Press(ctx, args[0])
}

func cmdHover(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser hover <selector>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Hover(ctx, args[0])
}

func cmdScroll(ctx context.Context, cdpURL, session string, headless bool, execPath string, x, y float64) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Scroll(ctx, x, y)
}

func cmdFocus(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser focus <selector>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Focus(ctx, args[0])
}

func cmdCheck(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser check <selector>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Check(ctx, args[0])
}

func cmdUncheck(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser uncheck <selector>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Uncheck(ctx, args[0])
}

func cmdSelect(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string, value string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser select <selector> --value <value>")
	}
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()
	return b.Select(ctx, args[0], value)
}

func cmdSnapshot(ctx context.Context, cdpURL, session string, headless bool, execPath string, interactive, compact bool) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	opts := &protocol.SnapshotOptions{
		Interactive: interactive,
		Compact:     compact,
	}
	tree, err := b.Snapshot(ctx, opts)
	if err != nil {
		return err
	}
	fmt.Print(tree)
	return nil
}

func cmdScreenshot(ctx context.Context, cdpURL, session string, headless bool, execPath string, format string, fullPage bool, output string) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	opts := &protocol.ScreenshotOptions{
		Format:   format,
		FullPage: fullPage,
	}
	data, err := b.Screenshot(ctx, opts)
	if err != nil {
		return err
	}

	if output != "" {
		return os.WriteFile(output, data, 0644)
	}
	os.Stdout.Write(data)
	return nil
}

func cmdGet(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser get <text|html|value|attr|url|title> [selector]")
	}

	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	subcmd := args[0]
	switch subcmd {
	case "text":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibe-browser get text <selector>")
		}
		text, err := b.GetText(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Println(text)

	case "html":
		selector := ""
		if len(args) > 1 {
			selector = args[1]
		}
		html, err := b.GetHTML(ctx, selector)
		if err != nil {
			return err
		}
		fmt.Println(html)

	case "value":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibe-browser get value <selector>")
		}
		val, err := b.GetValue(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Println(val)

	case "attr":
		if len(args) < 3 {
			return fmt.Errorf("usage: vibe-browser get attr <selector> <attribute>")
		}
		val, err := b.GetAttr(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Println(val)

	case "url":
		url, err := b.GetURL(ctx)
		if err != nil {
			return err
		}
		fmt.Println(url)

	case "title":
		title, err := b.GetTitle(ctx)
		if err != nil {
			return err
		}
		fmt.Println(title)

	default:
		return fmt.Errorf("unknown get subcommand: %s", subcmd)
	}

	return nil
}

func cmdIs(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: vibe-browser is <visible|enabled|checked> <selector>")
	}

	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	subcmd := args[0]
	selector := args[1]

	switch subcmd {
	case "visible":
		visible, err := b.IsVisible(ctx, selector)
		if err != nil {
			return err
		}
		fmt.Println(visible)

	case "enabled":
		enabled, err := b.IsEnabled(ctx, selector)
		if err != nil {
			return err
		}
		fmt.Println(enabled)

	case "checked":
		checked, err := b.IsChecked(ctx, selector)
		if err != nil {
			return err
		}
		fmt.Println(checked)

	default:
		return fmt.Errorf("unknown is subcommand: %s", subcmd)
	}

	return nil
}

func cmdEval(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser eval <expression>")
	}

	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	result, err := b.Eval(ctx, args[0])
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return nil
}

func cmdWait(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vibe-browser wait <ms|selector|text|url> [value]")
	}

	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	subcmd := args[0]
	switch subcmd {
	case "ms":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibe-browser wait ms <milliseconds>")
		}
		ms, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		return b.WaitMS(ctx, ms)

	case "selector":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibe-browser wait selector <selector>")
		}
		return b.WaitForSelector(ctx, args[1], 0)

	case "text":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibe-browser wait text <text>")
		}
		return b.WaitForText(ctx, args[1], 0)

	case "url":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibe-browser wait url <pattern>")
		}
		return b.WaitForURL(ctx, args[1], 0)

	default:
		return fmt.Errorf("unknown wait subcommand: %s", subcmd)
	}
}

func cmdSet(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string, width, height int) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser set <viewport|geolocation|offline|headers>")
	}

	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	subcmd := args[0]
	switch subcmd {
	case "viewport":
		return b.SetViewport(ctx, width, height, 1.0)

	case "geolocation":
		if len(args) < 3 {
			return fmt.Errorf("usage: vibe-browser set geolocation <lat> <lng>")
		}
		lat, _ := strconv.ParseFloat(args[1], 64)
		lng, _ := strconv.ParseFloat(args[2], 64)
		return b.SetGeolocation(ctx, lat, lng, 100)

	case "offline":
		return b.SetOffline(ctx, true)

	default:
		return fmt.Errorf("unknown set subcommand: %s", subcmd)
	}
}

func cmdCookies(ctx context.Context, cdpURL, session string, headless bool, execPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vibe-browser cookies <get|set|clear>")
	}

	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	defer b.Close()

	subcmd := args[0]
	switch subcmd {
	case "get":
		cookies, err := b.GetCookies(ctx)
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(cookies, "", "  ")
		fmt.Println(string(data))

	case "clear":
		return b.ClearCookies(ctx)

	default:
		return fmt.Errorf("unknown cookies subcommand: %s", subcmd)
	}

	return nil
}

func cmdDaemon(ctx context.Context, session string, headless bool, execPath string) error {
	server, err := newDaemonServer(&daemon.Options{
		Session:        session,
		Version:        version,
		Headless:       headless,
		ExecutablePath: execPath,
		Logger:         logger,
	})
	if err != nil {
		return err
	}

	if err := server.Start(ctx, &protocol.LaunchOptions{
		Headless:       headless,
		ExecutablePath: execPath,
	}); err != nil {
		return err
	}
	fmt.Printf("Daemon started for session %q at %s\n", session, server.SocketPath())

	select {
	case <-ctx.Done():
		server.Shutdown()
		return ctx.Err()
	case <-server.Done():
		return nil
	}
}

func cmdMCP(ctx context.Context, session string) error {
	return newMCPServer(logger, session).Run(ctx)
}

func cmdClose(ctx context.Context, cdpURL, session string, headless bool, execPath string) error {
	// If CDP URL is provided, close that specific browser
	if cdpURL != "" {
		return closeBrowser(ctx, cdpURL)
	}

	// Try to find and close a running browser
	wsURL, err := autoConnectCDP()
	if err != nil {
		return fmt.Errorf("no running browser found")
	}

	return closeBrowser(ctx, wsURL)
}

// closeBrowserByCDP closes a browser via CDP by sending Browser.close command.
func closeBrowserByCDP(ctx context.Context, wsURL string) error {
	client, err := connectCDPClient(ctx, wsURL, logger)
	if err != nil {
		return fmt.Errorf("connect to browser: %w", err)
	}
	defer client.Close()

	// Send Browser.close command
	_, err = client.Send(ctx, "Browser.close", nil)
	if err != nil {
		// If Browser.close fails, try to close the connection
		logger.Debug("Browser.close failed", "error", err)
	}

	fmt.Println("Browser closed")
	return nil
}

func cmdDiscover() error {
	// Try to auto-connect to a running browser
	wsURL, err := autoConnectCDP()
	if err != nil {
		return err
	}
	fmt.Println(wsURL)
	return nil
}

func cmdListBrowsers() error {
	browsers := []struct {
		Name string
		Type chrome.BrowserType
	}{
		{"Chrome", chrome.BrowserChrome},
		{"Chromium", chrome.BrowserChromium},
		{"Brave", chrome.BrowserBrave},
		{"Edge", chrome.BrowserEdge},
		{"Chrome Canary", chrome.BrowserChromeCanary},
	}

	fmt.Println("Available browsers:")
	for _, b := range browsers {
		path, err := findBrowser(b.Type)
		if err != nil {
			fmt.Printf("  %-15s not found\n", b.Name)
		} else {
			fmt.Printf("  %-15s %s\n", b.Name, path)
		}
	}
	return nil
}

func cmdListProfiles() error {
	userDir := findChromeUserDataDir()
	if userDir == "" {
		return fmt.Errorf("no Chrome user data directory found")
	}

	profiles := listChromeProfiles(userDir)
	if len(profiles) == 0 {
		return fmt.Errorf("no profiles found in %s", userDir)
	}

	fmt.Printf("Chrome profiles in %s:\n", userDir)
	for _, p := range profiles {
		fmt.Printf("  %-20s %s\n", p["directory"], p["name"])
	}
	return nil
}

// Helper: parse float from string
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Helper: check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

type boolFlag interface {
	IsBoolFlag() bool
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func normalizeFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}

		name, ok := flagName(arg)
		if !ok {
			positionals = append(positionals, arg)
			continue
		}

		f := fs.Lookup(name)
		if f == nil {
			flags = append(flags, arg)
			continue
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		if bf, ok := f.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positionals...)
}

func flagName(arg string) (string, bool) {
	if arg == "-" || !strings.HasPrefix(arg, "-") {
		return "", false
	}
	name := strings.TrimLeft(arg, "-")
	if name == "" {
		return "", false
	}
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		name = name[:idx]
	}
	return name, name != ""
}
