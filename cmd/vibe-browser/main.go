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

	"github.com/startvibecoding/vibe-browser/pkg/browser"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
)

var (
	version = "0.1.0"
	logger  *slog.Logger
)

func main() {
	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Setup logger
	logLevel := slog.LevelInfo
	if os.Getenv("VIBE_BROWSER_DEBUG") != "" {
		logLevel = slog.LevelDebug
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Global flags
	var cdpURL string
	var session string
	var headless bool
	var execPath string

	// Parse global flags from environment
	if v := os.Getenv("VIBE_BROWSER_CDP_URL"); v != "" {
		cdpURL = v
	}
	if v := os.Getenv("VIBE_BROWSER_SESSION"); v != "" {
		session = v
	} else {
		session = "default"
	}
	headless = true

	// Parse command-specific flags
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	fs.StringVar(&cdpURL, "cdp-url", cdpURL, "Chrome DevTools Protocol WebSocket URL")
	fs.StringVar(&session, "session", session, "Session name")
	fs.BoolVar(&headless, "headless", headless, "Run browser in headless mode")
	fs.StringVar(&execPath, "executable-path", execPath, "Path to Chrome executable")

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
	case "snapshot":
		fs.BoolVar(&interactive, "interactive", false, "Only show interactive elements")
		fs.BoolVar(&compact, "compact", false, "Compact output")
	case "set":
		if len(args) > 0 && args[0] == "viewport" {
			fs.IntVar(&width, "width", 1280, "Viewport width")
			fs.IntVar(&height, "height", 720, "Viewport height")
		}
	case "scroll":
		fs.Float64Var(&scrollX, "x", 0, "Horizontal scroll amount")
		fs.Float64Var(&scrollY, "y", 100, "Vertical scroll amount")
	}

	fs.Parse(args)
	args = fs.Args()

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
	case "version":
		fmt.Printf("vibe-browser version %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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

Environment Variables:
  VIBE_BROWSER_CDP_URL    Same as --cdp-url
  VIBE_BROWSER_SESSION    Same as --session
  VIBE_BROWSER_DEBUG      Enable debug logging
  VIBE_BROWSER_SOCKET_DIR Override socket directory
  CHROME_PATH             Path to Chrome executable
`)
}

// connectBrowser connects to a browser via CDP URL or launches one.
func connectBrowser(ctx context.Context, cdpURL, session string, headless bool, execPath string) (*browser.Browser, error) {
	if cdpURL != "" {
		return browser.ConnectToCDP(ctx, cdpURL, logger)
	}

	// TODO: Connect to daemon or launch browser
	return nil, fmt.Errorf("no CDP URL provided; use --cdp-url flag")
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
	// Import daemon package
	fmt.Println("Starting daemon...")
	// TODO: Implement daemon
	return fmt.Errorf("daemon not implemented yet")
}

func cmdMCP(ctx context.Context, session string) error {
	// Import mcp package
	fmt.Println("Starting MCP server...")
	// TODO: Implement MCP
	return fmt.Errorf("MCP not implemented yet")
}

func cmdClose(ctx context.Context, cdpURL, session string, headless bool, execPath string) error {
	b, err := connectBrowser(ctx, cdpURL, session, headless, execPath)
	if err != nil {
		return err
	}
	return b.Close()
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
