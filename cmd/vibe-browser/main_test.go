package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/startvibecoding/vibe-browser/internal/chrome"
	"github.com/startvibecoding/vibe-browser/pkg/browser"
	"github.com/startvibecoding/vibe-browser/pkg/cdp"
	"github.com/startvibecoding/vibe-browser/pkg/daemon"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cmdFakeCDP struct {
	responses map[string][]json.RawMessage
	calls     []cmdFakeCall
	events    chan cdp.Event
	connected bool
	sendErr   error
}

type cmdFakeCall struct {
	method string
	params any
}

func newCmdFakeCDP() *cmdFakeCDP {
	return &cmdFakeCDP{
		responses: make(map[string][]json.RawMessage),
		events:    make(chan cdp.Event, 8),
		connected: true,
	}
}

func (f *cmdFakeCDP) queue(method string, result string) {
	f.responses[method] = append(f.responses[method], json.RawMessage(result))
}

func (f *cmdFakeCDP) Send(ctx context.Context, method string, params any) (*cdp.Message, error) {
	f.calls = append(f.calls, cmdFakeCall{method: method, params: params})
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	items := f.responses[method]
	if len(items) == 0 {
		return &cdp.Message{Result: json.RawMessage(`{}`)}, nil
	}
	result := items[0]
	f.responses[method] = items[1:]
	return &cdp.Message{Result: result}, nil
}

func (f *cmdFakeCDP) Events() <-chan cdp.Event { return f.events }
func (f *cmdFakeCDP) Close() error {
	f.connected = false
	return nil
}
func (f *cmdFakeCDP) IsConnected() bool { return f.connected }

func cmdRuntimeResult(value any) string {
	data, _ := json.Marshal(map[string]any{
		"result": map[string]any{"value": value},
	})
	return string(data)
}

func withCommandBrowser(t *testing.T, fake *cmdFakeCDP) {
	t.Helper()
	oldConnect := connectBrowser
	connectBrowser = func(context.Context, string, string, bool, string, ...string) (*browser.Browser, error) {
		return browser.New(fake, slog.Default()), nil
	}
	t.Cleanup(func() {
		connectBrowser = oldConnect
	})
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	readFile, writeFile, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writeFile

	err = fn()
	require.NoError(t, writeFile.Close())
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, readFile)
	require.NoError(t, copyErr)
	require.NoError(t, readFile.Close())
	return buf.String(), err
}

func commandCallMethods(fake *cmdFakeCDP) []string {
	methods := make([]string, 0, len(fake.calls))
	for _, call := range fake.calls {
		methods = append(methods, call.method)
	}
	return methods
}

func TestCommandNavigationOutputsCurrentURL(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		run  func(context.Context) error
		call string
	}{
		{
			name: "open",
			run: func(ctx context.Context) error {
				return cmdOpen(ctx, "", "default", true, "", []string{"https://example.com"})
			},
			call: "Page.navigate",
		},
		{
			name: "navigate",
			run: func(ctx context.Context) error {
				return cmdNavigate(ctx, "", "default", true, "", []string{"https://example.com/next"})
			},
			call: "Page.navigate",
		},
		{
			name: "back",
			run: func(ctx context.Context) error {
				return cmdBack(ctx, "", "default", true, "")
			},
			call: "Page.goBack",
		},
		{
			name: "forward",
			run: func(ctx context.Context) error {
				return cmdForward(ctx, "", "default", true, "")
			},
			call: "Page.goForward",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newCmdFakeCDP()
			fake.queue("Page.navigate", `{}`)
			fake.queue("Runtime.evaluate", cmdRuntimeResult("https://example.com/current"))
			withCommandBrowser(t, fake)

			output, err := captureStdout(t, func() error { return tt.run(ctx) })
			require.NoError(t, err)
			assert.Equal(t, "https://example.com/current\n", output)
			assert.Contains(t, commandCallMethods(fake), tt.call)
		})
	}
}

func TestCommandActions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		run    func(context.Context) error
		queue  func(*cmdFakeCDP)
		method string
	}{
		{
			name: "reload",
			run: func(ctx context.Context) error {
				return cmdReload(ctx, "", "default", true, "")
			},
			method: "Page.reload",
		},
		{
			name: "click",
			run: func(ctx context.Context) error {
				return cmdClick(ctx, "", "default", true, "", []string{"#ok"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(map[string]any{"x": 1, "y": 2, "width": 10, "height": 20}))
			},
			method: "Input.dispatchMouseEvent",
		},
		{
			name: "double click",
			run: func(ctx context.Context) error {
				return cmdDoubleClick(ctx, "", "default", true, "", []string{"#ok"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(map[string]any{"x": 1, "y": 2, "width": 10, "height": 20}))
			},
			method: "Input.dispatchMouseEvent",
		},
		{
			name: "fill",
			run: func(ctx context.Context) error {
				return cmdFill(ctx, "", "default", true, "", []string{"input"}, "abc")
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			method: "Input.insertText",
		},
		{
			name: "type",
			run: func(ctx context.Context) error {
				return cmdType(ctx, "", "default", true, "", []string{"input", "xy"}, 0)
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			method: "Input.dispatchKeyEvent",
		},
		{
			name: "press",
			run: func(ctx context.Context) error {
				return cmdPress(ctx, "", "default", true, "", []string{"Enter"})
			},
			method: "Input.dispatchKeyEvent",
		},
		{
			name: "hover",
			run: func(ctx context.Context) error {
				return cmdHover(ctx, "", "default", true, "", []string{"#ok"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(map[string]any{"x": 1, "y": 2, "width": 10, "height": 20}))
			},
			method: "Input.dispatchMouseEvent",
		},
		{
			name: "scroll",
			run: func(ctx context.Context) error {
				return cmdScroll(ctx, "", "default", true, "", 1, 2)
			},
			method: "Input.dispatchMouseEvent",
		},
		{
			name: "focus",
			run: func(ctx context.Context) error {
				return cmdFocus(ctx, "", "default", true, "", []string{"input"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			method: "Runtime.evaluate",
		},
		{
			name: "check",
			run: func(ctx context.Context) error {
				return cmdCheck(ctx, "", "default", true, "", []string{"input"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			method: "Runtime.evaluate",
		},
		{
			name: "uncheck",
			run: func(ctx context.Context) error {
				return cmdUncheck(ctx, "", "default", true, "", []string{"input"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			method: "Runtime.evaluate",
		},
		{
			name: "select",
			run: func(ctx context.Context) error {
				return cmdSelect(ctx, "", "default", true, "", []string{"select"}, "a")
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			method: "Runtime.evaluate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newCmdFakeCDP()
			if tt.queue != nil {
				tt.queue(fake)
			}
			withCommandBrowser(t, fake)

			err := tt.run(ctx)
			require.NoError(t, err)
			assert.Contains(t, commandCallMethods(fake), tt.method)
		})
	}
}

func TestCommandOutputCommands(t *testing.T) {
	ctx := context.Background()

	t.Run("snapshot", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Accessibility.getFullAXTree", `{"nodes":[{"nodeId":"1","role":{"value":"button"},"name":{"value":"Save"}}]}`)
		withCommandBrowser(t, fake)

		output, err := captureStdout(t, func() error {
			return cmdSnapshot(ctx, "", "default", true, "", false, false)
		})
		require.NoError(t, err)
		assert.Contains(t, output, "button Save")
	})

	t.Run("screenshot stdout", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Page.captureScreenshot", `{"data":"`+base64.StdEncoding.EncodeToString([]byte("png"))+`"}`)
		withCommandBrowser(t, fake)

		output, err := captureStdout(t, func() error {
			return cmdScreenshot(ctx, "", "default", true, "", "png", true, "")
		})
		require.NoError(t, err)
		assert.Equal(t, "png", output)
	})

	t.Run("screenshot file", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Page.captureScreenshot", `{"data":"`+base64.StdEncoding.EncodeToString([]byte("filepng"))+`"}`)
		withCommandBrowser(t, fake)
		path := t.TempDir() + "/shot.png"

		require.NoError(t, cmdScreenshot(ctx, "", "default", true, "", "png", false, path))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("filepng"), data)
	})
}

func TestCommandGetIsEvalSetAndCookies(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		run    func(context.Context) error
		queue  func(*cmdFakeCDP)
		output string
		method string
	}{
		{
			name: "get text",
			run: func(ctx context.Context) error {
				return cmdGet(ctx, "", "default", true, "", []string{"text", "h1"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult("Hello"))
			},
			output: "Hello\n",
		},
		{
			name: "get html",
			run: func(ctx context.Context) error {
				return cmdGet(ctx, "", "default", true, "", []string{"html"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult("<html></html>"))
			},
			output: "<html></html>\n",
		},
		{
			name: "get value",
			run: func(ctx context.Context) error {
				return cmdGet(ctx, "", "default", true, "", []string{"value", "input"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult("abc"))
			},
			output: "abc\n",
		},
		{
			name: "get attr",
			run: func(ctx context.Context) error {
				return cmdGet(ctx, "", "default", true, "", []string{"attr", "a", "href"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult("/next"))
			},
			output: "/next\n",
		},
		{
			name: "get url",
			run: func(ctx context.Context) error {
				return cmdGet(ctx, "", "default", true, "", []string{"url"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult("https://example.com"))
			},
			output: "https://example.com\n",
		},
		{
			name: "get title",
			run: func(ctx context.Context) error {
				return cmdGet(ctx, "", "default", true, "", []string{"title"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult("Title"))
			},
			output: "Title\n",
		},
		{
			name: "is visible",
			run: func(ctx context.Context) error {
				return cmdIs(ctx, "", "default", true, "", []string{"visible", "main"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			output: "true\n",
		},
		{
			name: "is enabled",
			run: func(ctx context.Context) error {
				return cmdIs(ctx, "", "default", true, "", []string{"enabled", "button"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(false))
			},
			output: "false\n",
		},
		{
			name: "is checked",
			run: func(ctx context.Context) error {
				return cmdIs(ctx, "", "default", true, "", []string{"checked", "input"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
			},
			output: "true\n",
		},
		{
			name: "eval",
			run: func(ctx context.Context) error {
				return cmdEval(ctx, "", "default", true, "", []string{"1 + 1"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Runtime.evaluate", cmdRuntimeResult(float64(2)))
			},
			output: "2\n",
		},
		{
			name: "cookies get",
			run: func(ctx context.Context) error {
				return cmdCookies(ctx, "", "default", true, "", []string{"get"})
			},
			queue: func(fake *cmdFakeCDP) {
				fake.queue("Network.getCookies", `{"cookies":[{"name":"sid","value":"1"}]}`)
			},
			output: "[\n  {\n    \"name\": \"sid\",\n    \"value\": \"1\"\n  }\n]\n",
		},
		{
			name: "set viewport",
			run: func(ctx context.Context) error {
				return cmdSet(ctx, "", "default", true, "", []string{"viewport"}, 800, 600)
			},
			method: "Emulation.setDeviceMetricsOverride",
		},
		{
			name: "set geolocation",
			run: func(ctx context.Context) error {
				return cmdSet(ctx, "", "default", true, "", []string{"geolocation", "1.2", "3.4"}, 0, 0)
			},
			method: "Emulation.setGeolocationOverride",
		},
		{
			name: "set offline",
			run: func(ctx context.Context) error {
				return cmdSet(ctx, "", "default", true, "", []string{"offline"}, 0, 0)
			},
			method: "Network.emulateNetworkConditions",
		},
		{
			name: "cookies clear",
			run: func(ctx context.Context) error {
				return cmdCookies(ctx, "", "default", true, "", []string{"clear"})
			},
			method: "Network.clearBrowserCookies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newCmdFakeCDP()
			if tt.queue != nil {
				tt.queue(fake)
			}
			withCommandBrowser(t, fake)

			output, err := captureStdout(t, func() error { return tt.run(ctx) })
			require.NoError(t, err)
			assert.Equal(t, tt.output, output)
			if tt.method != "" {
				assert.Contains(t, commandCallMethods(fake), tt.method)
			}
		})
	}
}

func TestCommandWaitsAndUsageErrors(t *testing.T) {
	ctx := context.Background()

	fake := newCmdFakeCDP()
	withCommandBrowser(t, fake)
	require.NoError(t, cmdWait(ctx, "", "default", true, "", []string{"ms", "1"}))

	errorCases := []struct {
		name string
		err  error
	}{
		{"navigate", cmdNavigate(ctx, "", "default", true, "", nil)},
		{"click", cmdClick(ctx, "", "default", true, "", nil)},
		{"double click", cmdDoubleClick(ctx, "", "default", true, "", nil)},
		{"fill", cmdFill(ctx, "", "default", true, "", nil, "")},
		{"type", cmdType(ctx, "", "default", true, "", []string{"input"}, 0)},
		{"press", cmdPress(ctx, "", "default", true, "", nil)},
		{"hover", cmdHover(ctx, "", "default", true, "", nil)},
		{"focus", cmdFocus(ctx, "", "default", true, "", nil)},
		{"check", cmdCheck(ctx, "", "default", true, "", nil)},
		{"uncheck", cmdUncheck(ctx, "", "default", true, "", nil)},
		{"select", cmdSelect(ctx, "", "default", true, "", nil, "")},
		{"get", cmdGet(ctx, "", "default", true, "", nil)},
		{"get text", cmdGet(ctx, "", "default", true, "", []string{"text"})},
		{"get attr", cmdGet(ctx, "", "default", true, "", []string{"attr", "a"})},
		{"get unknown", cmdGet(ctx, "", "default", true, "", []string{"missing"})},
		{"is", cmdIs(ctx, "", "default", true, "", nil)},
		{"is unknown", cmdIs(ctx, "", "default", true, "", []string{"missing", "x"})},
		{"eval", cmdEval(ctx, "", "default", true, "", nil)},
		{"wait", cmdWait(ctx, "", "default", true, "", nil)},
		{"wait ms", cmdWait(ctx, "", "default", true, "", []string{"ms"})},
		{"wait selector", cmdWait(ctx, "", "default", true, "", []string{"selector"})},
		{"wait text", cmdWait(ctx, "", "default", true, "", []string{"text"})},
		{"wait url", cmdWait(ctx, "", "default", true, "", []string{"url"})},
		{"wait unknown", cmdWait(ctx, "", "default", true, "", []string{"missing"})},
		{"set", cmdSet(ctx, "", "default", true, "", nil, 0, 0)},
		{"set geolocation", cmdSet(ctx, "", "default", true, "", []string{"geolocation"}, 0, 0)},
		{"set unknown", cmdSet(ctx, "", "default", true, "", []string{"missing"}, 0, 0)},
		{"cookies", cmdCookies(ctx, "", "default", true, "", nil)},
		{"cookies unknown", cmdCookies(ctx, "", "default", true, "", []string{"set"})},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.err)
		})
	}
}

func TestCommandHelpers(t *testing.T) {
	assert.Equal(t, 3.5, parseFloat("3.5"))
	assert.Equal(t, 0.0, parseFloat("bad"))
	assert.True(t, contains("vibe-browser", "browser"))
	assert.False(t, contains("vibe-browser", "chrome"))

	output, err := captureStdout(t, func() error {
		printUsage()
		return nil
	})
	require.NoError(t, err)
	assert.Contains(t, output, "vibe-browser")
	assert.Contains(t, output, "--cdp-url")
}

func TestRunParsesFlagsEnvAndBrowserType(t *testing.T) {
	fake := newCmdFakeCDP()
	fake.queue("Page.navigate", `{}`)
	fake.queue("Runtime.evaluate", cmdRuntimeResult("https://example.com"))

	oldConnect := connectBrowser
	var seenCDPURL string
	var seenSession string
	var seenHeadless bool
	var seenExecPath string
	var seenBrowserType string
	connectBrowser = func(ctx context.Context, cdpURL, session string, headless bool, execPath string, browserType ...string) (*browser.Browser, error) {
		seenCDPURL = cdpURL
		seenSession = session
		seenHeadless = headless
		seenExecPath = execPath
		seenBrowserType = currentBrowserType
		return browser.New(fake, slog.Default()), nil
	}
	t.Cleanup(func() { connectBrowser = oldConnect })
	t.Setenv("VIBE_BROWSER_CDP_URL", "ws://env")
	t.Setenv("VIBE_BROWSER_SESSION", "env-session")
	t.Setenv("VIBE_BROWSER_BROWSER", "brave")

	var code int
	output, err := captureStdout(t, func() error {
		code = run(context.Background(), []string{
			"open",
			"--session", "flag-session",
			"--headless=false",
			"--executable-path", "/bin/browser",
			"https://example.com",
		})
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Equal(t, "https://example.com\n", output)
	assert.Equal(t, "ws://env", seenCDPURL)
	assert.Equal(t, "flag-session", seenSession)
	assert.False(t, seenHeadless)
	assert.Equal(t, "/bin/browser", seenExecPath)
	assert.Equal(t, "brave", seenBrowserType)
	assert.Empty(t, currentBrowserType)
}

func TestRunCommandsAndExitCodes(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		var code int
		output, err := captureStdout(t, func() error {
			code = run(context.Background(), []string{"version"})
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 0, code)
		assert.Contains(t, output, "vibe-browser version")
	})

	t.Run("help", func(t *testing.T) {
		var code int
		output, err := captureStdout(t, func() error {
			code = run(context.Background(), []string{"help"})
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 0, code)
		assert.Contains(t, output, "Usage:")
	})

	t.Run("missing command", func(t *testing.T) {
		var code int
		output, err := captureStdout(t, func() error {
			code = run(context.Background(), nil)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, code)
		assert.Contains(t, output, "Usage:")
	})

	t.Run("unknown command", func(t *testing.T) {
		var code int
		_, err := captureStdout(t, func() error {
			code = run(context.Background(), []string{"missing-command"})
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, code)
	})

	t.Run("flag parse error", func(t *testing.T) {
		code := run(context.Background(), []string{"open", "--missing-flag"})
		assert.Equal(t, 2, code)
	})

	t.Run("command error", func(t *testing.T) {
		code := run(context.Background(), []string{"navigate"})
		assert.Equal(t, 1, code)
	})

	t.Run("goto alias", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Page.navigate", `{}`)
		fake.queue("Runtime.evaluate", cmdRuntimeResult("https://example.com/alias"))
		withCommandBrowser(t, fake)
		var code int
		output, err := captureStdout(t, func() error {
			code = run(context.Background(), []string{"goto", "https://example.com/alias"})
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 0, code)
		assert.Equal(t, "https://example.com/alias\n", output)
	})
}

func TestRunParsesDocumentedShortFlagsAndSubcommandFlags(t *testing.T) {
	t.Run("snapshot short interactive flag", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Accessibility.getFullAXTree", `{"nodes":[{"nodeId":"1","role":{"value":"button"},"name":{"value":"Save"}}]}`)
		withCommandBrowser(t, fake)

		var code int
		output, err := captureStdout(t, func() error {
			code = run(context.Background(), []string{"snapshot", "-i"})
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 0, code)
		assert.Contains(t, output, "button Save")
	})

	t.Run("screenshot short output flag", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Page.captureScreenshot", `{"data":"`+base64.StdEncoding.EncodeToString([]byte("filepng"))+`"}`)
		withCommandBrowser(t, fake)
		path := t.TempDir() + "/shot.png"

		code := run(context.Background(), []string{"screenshot", "-o", path})
		assert.Equal(t, 0, code)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("filepng"), data)
	})

	t.Run("set viewport flags after subcommand", func(t *testing.T) {
		fake := newCmdFakeCDP()
		withCommandBrowser(t, fake)

		code := run(context.Background(), []string{"set", "viewport", "--width", "321", "--height", "222"})
		assert.Equal(t, 0, code)
		require.Len(t, fake.calls, 1)
		params, ok := fake.calls[0].params.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 321, params["width"])
		assert.Equal(t, 222, params["height"])
	})

	t.Run("fill flag after selector", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
		withCommandBrowser(t, fake)

		code := run(context.Background(), []string{"fill", "input[name=q]", "--value", "hello"})
		assert.Equal(t, 0, code)

		var text string
		for _, call := range fake.calls {
			if call.method != "Input.insertText" {
				continue
			}
			params, ok := call.params.(map[string]any)
			require.True(t, ok)
			text, _ = params["text"].(string)
		}
		assert.Equal(t, "hello", text)
	})

	t.Run("type delay flag after text", func(t *testing.T) {
		fake := newCmdFakeCDP()
		fake.queue("Runtime.evaluate", cmdRuntimeResult(true))
		withCommandBrowser(t, fake)

		code := run(context.Background(), []string{"type", "input[name=q]", "hi", "--delay", "-1"})
		assert.Equal(t, 0, code)
		assert.Contains(t, commandCallMethods(fake), "Input.dispatchKeyEvent")
	})
}

func TestMainUsesRunExitCode(t *testing.T) {
	restoreCommandServiceStubs(t)
	oldArgs := os.Args
	oldExit := osExit
	var gotCode int
	exited := false
	os.Args = []string{"vibe-browser", "version"}
	osExit = func(code int) {
		gotCode = code
		exited = true
	}
	t.Cleanup(func() {
		os.Args = oldArgs
		osExit = oldExit
	})

	output, err := captureStdout(t, func() error {
		main()
		return nil
	})
	require.NoError(t, err)
	assert.True(t, exited)
	assert.Equal(t, 0, gotCode)
	assert.Contains(t, output, "vibe-browser version")
}

func TestConnectBrowserDefaultPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("direct cdp", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		var connected string
		connectToCDP = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
			connected = wsURL
			return browser.New(newCmdFakeCDP(), logger), nil
		}
		b, err := connectBrowserDefault(ctx, "ws://direct", "default", true, "")
		require.NoError(t, err)
		require.NotNil(t, b)
		assert.Equal(t, "ws://direct", connected)
	})

	t.Run("auto connect", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "ws://auto", nil }
		var connected string
		connectToCDP = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
			connected = wsURL
			return browser.New(newCmdFakeCDP(), logger), nil
		}
		b, err := connectBrowserDefault(ctx, "", "default", true, "")
		require.NoError(t, err)
		require.NotNil(t, b)
		assert.Equal(t, "ws://auto", connected)
	})

	t.Run("launch success", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "", errors.New("no running browser") }
		proc := &chrome.Process{CDPURL: "ws://launched"}
		launchChrome = func(ctx context.Context, opts chrome.LaunchOptions, logger *slog.Logger) (*chrome.Process, error) {
			assert.Equal(t, chrome.BrowserEdge, opts.Browser)
			assert.Equal(t, "/bin/browser", opts.ExecutablePath)
			assert.False(t, opts.Headless)
			return proc, nil
		}
		connectToCDP = func(ctx context.Context, wsURL string, logger *slog.Logger) (*browser.Browser, error) {
			assert.Equal(t, "ws://launched", wsURL)
			return browser.New(newCmdFakeCDP(), logger), nil
		}
		b, err := connectBrowserDefault(ctx, "", "default", false, "/bin/browser", "edge")
		require.NoError(t, err)
		require.NotNil(t, b)
	})

	t.Run("launch error", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "", errors.New("no running browser") }
		launchChrome = func(context.Context, chrome.LaunchOptions, *slog.Logger) (*chrome.Process, error) {
			return nil, errors.New("launch failed")
		}
		_, err := connectBrowserDefault(ctx, "", "default", true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "launch failed")
	})

	t.Run("auto connect cdp error returns before launch", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "ws://auto", nil }
		connectToCDP = func(context.Context, string, *slog.Logger) (*browser.Browser, error) {
			return nil, errors.New("connect failed")
		}
		launchChrome = func(context.Context, chrome.LaunchOptions, *slog.Logger) (*chrome.Process, error) {
			t.Fatal("launch should not be called after successful auto discovery")
			return nil, nil
		}
		_, err := connectBrowserDefault(ctx, "", "default", true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connect failed")
	})
}

func TestCloseBrowserByCDP(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		fake := &fakeCloseCDPClient{}
		connectCDPClient = func(ctx context.Context, wsURL string, logger *slog.Logger) (closeCDPClient, error) {
			assert.Equal(t, "ws://browser", wsURL)
			return fake, nil
		}
		output, err := captureStdout(t, func() error {
			return closeBrowserByCDP(ctx, "ws://browser")
		})
		require.NoError(t, err)
		assert.True(t, fake.closed)
		assert.Equal(t, []string{"Browser.close"}, fake.methods)
		assert.Equal(t, "Browser closed\n", output)
	})

	t.Run("connect error", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		connectCDPClient = func(context.Context, string, *slog.Logger) (closeCDPClient, error) {
			return nil, errors.New("dial failed")
		}
		err := closeBrowserByCDP(ctx, "ws://bad")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dial failed")
	})

	t.Run("browser close command error is ignored", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		fake := &fakeCloseCDPClient{sendErr: errors.New("close rejected")}
		connectCDPClient = func(context.Context, string, *slog.Logger) (closeCDPClient, error) {
			return fake, nil
		}
		_, err := captureStdout(t, func() error {
			return closeBrowserByCDP(ctx, "ws://browser")
		})
		require.NoError(t, err)
		assert.True(t, fake.closed)
	})
}

func TestCommandInjectedServices(t *testing.T) {
	ctx := context.Background()

	t.Run("daemon", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		fake := &fakeDaemonServer{done: make(chan struct{})}
		newDaemonServer = func(opts *daemon.Options) (daemonServer, error) {
			assert.Equal(t, "sdk", opts.Session)
			return fake, nil
		}
		output, err := captureStdout(t, func() error {
			return cmdDaemon(ctx, "sdk", true, "/bin/browser")
		})
		require.NoError(t, err)
		assert.True(t, fake.started)
		assert.Equal(t, "/bin/browser", fake.launchOpts.ExecutablePath)
		assert.Contains(t, output, "Daemon started")
	})

	t.Run("daemon start error", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		newDaemonServer = func(opts *daemon.Options) (daemonServer, error) {
			return &fakeDaemonServer{startErr: errors.New("start failed"), done: make(chan struct{})}, nil
		}
		err := cmdDaemon(ctx, "sdk", true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start failed")
	})

	t.Run("mcp", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		fake := &fakeMCPServer{}
		newMCPServer = func(log *slog.Logger, session string) mcpServer {
			assert.Equal(t, "sdk", session)
			return fake
		}
		require.NoError(t, cmdMCP(ctx, "sdk"))
		assert.True(t, fake.ran)
	})

	t.Run("close direct", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		var closedURL string
		closeBrowser = func(ctx context.Context, wsURL string) error {
			closedURL = wsURL
			return nil
		}
		require.NoError(t, cmdClose(ctx, "ws://direct", "default", true, ""))
		assert.Equal(t, "ws://direct", closedURL)
	})

	t.Run("close discovered", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "ws://auto", nil }
		var closedURL string
		closeBrowser = func(ctx context.Context, wsURL string) error {
			closedURL = wsURL
			return nil
		}
		require.NoError(t, cmdClose(ctx, "", "default", true, ""))
		assert.Equal(t, "ws://auto", closedURL)
	})

	t.Run("close discovery error", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "", errors.New("missing") }
		err := cmdClose(ctx, "", "default", true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no running browser")
	})

	t.Run("discover", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		autoConnectCDP = func() (string, error) { return "ws://auto", nil }
		output, err := captureStdout(t, cmdDiscover)
		require.NoError(t, err)
		assert.Equal(t, "ws://auto\n", output)
	})

	t.Run("list browsers", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		findBrowser = func(bt chrome.BrowserType) (string, error) {
			if bt == chrome.BrowserChrome {
				return "/bin/chrome", nil
			}
			return "", errors.New("missing")
		}
		output, err := captureStdout(t, cmdListBrowsers)
		require.NoError(t, err)
		assert.Contains(t, output, "Chrome")
		assert.Contains(t, output, "/bin/chrome")
		assert.Contains(t, output, "not found")
	})

	t.Run("list profiles", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		findChromeUserDataDir = func() string { return "/profiles" }
		listChromeProfiles = func(string) []map[string]string {
			return []map[string]string{{"directory": "Default", "name": "Person 1"}}
		}
		output, err := captureStdout(t, cmdListProfiles)
		require.NoError(t, err)
		assert.Contains(t, output, "Default")
		assert.Contains(t, output, "Person 1")
	})

	t.Run("list profiles errors", func(t *testing.T) {
		restoreCommandServiceStubs(t)
		findChromeUserDataDir = func() string { return "" }
		require.Error(t, cmdListProfiles())

		findChromeUserDataDir = func() string { return "/profiles" }
		listChromeProfiles = func(string) []map[string]string { return nil }
		require.Error(t, cmdListProfiles())
	})
}

func TestCommandScreenshotPassesOptions(t *testing.T) {
	fake := newCmdFakeCDP()
	fake.queue("Page.captureScreenshot", `{"data":"`+base64.StdEncoding.EncodeToString([]byte("png"))+`"}`)
	withCommandBrowser(t, fake)

	_, err := captureStdout(t, func() error {
		return cmdScreenshot(context.Background(), "", "default", true, "", "jpeg", true, "")
	})
	require.NoError(t, err)

	require.Len(t, fake.calls, 1)
	params, ok := fake.calls[0].params.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "jpeg", params["format"])
	assert.Equal(t, true, params["captureBeyondViewport"])
}

func TestCommandSetCookieUnsupportedInCLI(t *testing.T) {
	fake := newCmdFakeCDP()
	withCommandBrowser(t, fake)

	err := cmdCookies(context.Background(), "", "default", true, "", []string{"set", "sid", "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown cookies subcommand")
}

var _ = protocol.Cookie{}

type fakeDaemonServer struct {
	done       chan struct{}
	startErr   error
	started    bool
	shutdown   bool
	launchOpts *protocol.LaunchOptions
}

func (f *fakeDaemonServer) Start(ctx context.Context, opts *protocol.LaunchOptions) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = true
	f.launchOpts = opts
	close(f.done)
	return nil
}
func (f *fakeDaemonServer) Shutdown() {
	f.shutdown = true
}
func (f *fakeDaemonServer) Done() <-chan struct{} { return f.done }
func (f *fakeDaemonServer) SocketPath() string    { return "/tmp/sdk.sock" }

type fakeMCPServer struct {
	ran bool
	err error
}

func (f *fakeMCPServer) Run(ctx context.Context) error {
	f.ran = true
	return f.err
}

func restoreCommandServiceStubs(t *testing.T) {
	t.Helper()
	oldAutoConnect := autoConnectCDP
	oldNewDaemon := newDaemonServer
	oldNewMCP := newMCPServer
	oldClose := closeBrowser
	oldFindBrowser := findBrowser
	oldFindUserDir := findChromeUserDataDir
	oldListProfiles := listChromeProfiles
	oldLaunchChrome := launchChrome
	oldConnectToCDP := connectToCDP
	oldConnectCDPClient := connectCDPClient
	t.Cleanup(func() {
		autoConnectCDP = oldAutoConnect
		newDaemonServer = oldNewDaemon
		newMCPServer = oldNewMCP
		closeBrowser = oldClose
		findBrowser = oldFindBrowser
		findChromeUserDataDir = oldFindUserDir
		listChromeProfiles = oldListProfiles
		launchChrome = oldLaunchChrome
		connectToCDP = oldConnectToCDP
		connectCDPClient = oldConnectCDPClient
	})
}

type fakeCloseCDPClient struct {
	methods []string
	sendErr error
	closed  bool
}

func (f *fakeCloseCDPClient) Send(ctx context.Context, method string, params any) (*cdp.Message, error) {
	f.methods = append(f.methods, method)
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &cdp.Message{Result: json.RawMessage(`{}`)}, nil
}

func (f *fakeCloseCDPClient) Close() error {
	f.closed = true
	return nil
}
