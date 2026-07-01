package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/startvibecoding/vibe-browser/pkg/cdp"
	"github.com/startvibecoding/vibe-browser/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCDP struct {
	responses map[string][]json.RawMessage
	calls     []fakeCall
	events    chan cdp.Event
	connected bool
	closeErr  error
	sendErr   error
}

type fakeCall struct {
	method string
	params any
}

func newFakeCDP() *fakeCDP {
	return &fakeCDP{
		responses: make(map[string][]json.RawMessage),
		events:    make(chan cdp.Event, 8),
		connected: true,
	}
}

func (f *fakeCDP) queue(method string, result string) {
	f.responses[method] = append(f.responses[method], json.RawMessage(result))
}

func (f *fakeCDP) Send(ctx context.Context, method string, params any) (*cdp.Message, error) {
	f.calls = append(f.calls, fakeCall{method: method, params: params})
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

func (f *fakeCDP) Events() <-chan cdp.Event { return f.events }
func (f *fakeCDP) Close() error {
	f.connected = false
	return f.closeErr
}
func (f *fakeCDP) IsConnected() bool { return f.connected }

func runtimeResult(value any) string {
	data, _ := json.Marshal(map[string]any{
		"result": map[string]any{"value": value},
	})
	return string(data)
}

func TestBrowserEvalAndAccessors(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", runtimeResult("https://example.com/a"))
	f.queue("Runtime.evaluate", runtimeResult("Example"))
	f.queue("Runtime.evaluate", runtimeResult("Hello"))
	f.queue("Runtime.evaluate", runtimeResult("<h1>Hello</h1>"))
	f.queue("Runtime.evaluate", runtimeResult("typed"))
	f.queue("Runtime.evaluate", runtimeResult("main"))
	f.queue("Runtime.evaluate", runtimeResult(true))
	f.queue("Runtime.evaluate", runtimeResult(false))
	f.queue("Runtime.evaluate", runtimeResult(true))
	f.queue("Runtime.evaluate", runtimeResult(float64(42)))

	ctx := context.Background()
	gotURL, err := b.GetURL(ctx)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/a", gotURL)

	title, err := b.GetTitle(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Example", title)

	text, err := b.GetText(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, "Hello", text)

	html, err := b.GetHTML(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, "<h1>Hello</h1>", html)

	value, err := b.GetValue(ctx, "input")
	require.NoError(t, err)
	assert.Equal(t, "typed", value)

	attr, err := b.GetAttr(ctx, "main", "id")
	require.NoError(t, err)
	assert.Equal(t, "main", attr)

	visible, err := b.IsVisible(ctx, "main")
	require.NoError(t, err)
	assert.True(t, visible)

	enabled, err := b.IsEnabled(ctx, "button")
	require.NoError(t, err)
	assert.False(t, enabled)

	checked, err := b.IsChecked(ctx, "input")
	require.NoError(t, err)
	assert.True(t, checked)

	eval, err := b.Eval(ctx, "21 * 2")
	require.NoError(t, err)
	assert.Equal(t, float64(42), eval)
}

func TestBrowserEvalReportsRuntimeException(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", `{"exceptionDetails":{"text":"Uncaught","exception":{"description":"Error: boom"}}}`)

	_, err := b.Eval(context.Background(), "throw new Error('boom')")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime exception: Error: boom")
}

func TestBrowserEvalHelpers(t *testing.T) {
	assert.Equal(t, "", resultString(nil))
	assert.Equal(t, "", resultString(&runtimeEvalResult{}))
	assert.Equal(t, "42", resultString(&runtimeEvalResult{Result: struct {
		Value any    `json:"value"`
		Type  string `json:"type"`
	}{Value: float64(42)}}))
	assert.False(t, resultBool(nil))
	assert.False(t, resultBool(&runtimeEvalResult{}))
}

func TestBrowserEvalPropagatesInvalidJSON(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", `{`)

	_, err := b.Eval(context.Background(), "1")
	require.Error(t, err)
}

func TestBrowserNavigationAndWaitForLoad(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Page.enable", `{}`)
	f.queue("Page.navigate", `{}`)
	f.queue("Page.enable", `{}`)
	f.events <- cdp.Event{Method: "Page.loadEventFired"}

	err := b.Navigate(context.Background(), "https://example.com", &protocol.NavigationOptions{
		WaitUntil: "load",
		Timeout:   1000,
	})
	require.NoError(t, err)
	require.Len(t, f.calls, 3)
	assert.Equal(t, "Page.enable", f.calls[0].method)
	assert.Equal(t, "Page.navigate", f.calls[1].method)
	assert.Equal(t, "Page.enable", f.calls[2].method)
}

func TestBrowserNavigationErrorText(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Page.enable", `{}`)
	f.queue("Page.navigate", `{"errorText":"bad url"}`)

	err := b.Navigate(context.Background(), "bad", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad url")
}

func TestConnectToCDPExistingPageTarget(t *testing.T) {
	restoreHTTP := stubBrowserHTTP(t, func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/json/list", r.URL.Path)
		return browserJSONResponse(200, `[{"id":"page-1","type":"page","webSocketDebuggerUrl":"ws://page"}]`), nil
	})
	defer restoreHTTP()
	restoreConnect := stubBrowserConnect(t, func(ctx context.Context, wsURL string, logger *slog.Logger) (cdpClient, error) {
		assert.Equal(t, "ws://page", wsURL)
		return newFakeCDP(), nil
	})
	defer restoreConnect()

	b, err := ConnectToCDP(context.Background(), "ws://127.0.0.1:9222/devtools/browser/abc", nil)
	require.NoError(t, err)
	require.NotNil(t, b.pageTarget)
	assert.Equal(t, "page-1", b.pageTarget.ID)
	assert.True(t, b.IsConnected())
}

func TestConnectToCDPCreatesPageWhenMissing(t *testing.T) {
	restoreHTTP := stubBrowserHTTP(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/json/list":
			return browserJSONResponse(200, `[{"id":"worker-1","type":"service_worker"}]`), nil
		case "/json/new":
			return browserJSONResponse(200, `{"id":"page-2","type":"page","webSocketDebuggerUrl":"ws://created"}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	defer restoreHTTP()
	restoreConnect := stubBrowserConnect(t, func(ctx context.Context, wsURL string, logger *slog.Logger) (cdpClient, error) {
		assert.Equal(t, "ws://created", wsURL)
		return newFakeCDP(), nil
	})
	defer restoreConnect()

	b, err := ConnectToCDP(context.Background(), "ws://127.0.0.1:9222/devtools/browser/abc", nil)
	require.NoError(t, err)
	require.NotNil(t, b.pageTarget)
	assert.Equal(t, "page-2", b.pageTarget.ID)
}

func TestConnectToCDPErrors(t *testing.T) {
	t.Run("list targets", func(t *testing.T) {
		restoreHTTP := stubBrowserHTTP(t, func(r *http.Request) (*http.Response, error) {
			return browserJSONResponse(200, `not-json`), nil
		})
		defer restoreHTTP()
		_, err := ConnectToCDP(context.Background(), "ws://127.0.0.1:9222/devtools/browser/abc", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list targets")
	})

	t.Run("create target", func(t *testing.T) {
		restoreHTTP := stubBrowserHTTP(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/json/list" {
				return browserJSONResponse(200, `[]`), nil
			}
			return browserJSONResponse(200, `not-json`), nil
		})
		defer restoreHTTP()
		_, err := ConnectToCDP(context.Background(), "ws://127.0.0.1:9222/devtools/browser/abc", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create target")
	})

	t.Run("connect page target", func(t *testing.T) {
		restoreHTTP := stubBrowserHTTP(t, func(r *http.Request) (*http.Response, error) {
			return browserJSONResponse(200, `[{"id":"page-1","type":"page","webSocketDebuggerUrl":"ws://page"}]`), nil
		})
		defer restoreHTTP()
		restoreConnect := stubBrowserConnect(t, func(ctx context.Context, wsURL string, logger *slog.Logger) (cdpClient, error) {
			return nil, errors.New("dial failed")
		})
		defer restoreConnect()
		_, err := ConnectToCDP(context.Background(), "ws://127.0.0.1:9222/devtools/browser/abc", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connect to page target")
	})
}

func TestBrowserInputMethods(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", runtimeResult(map[string]any{"x": 10, "y": 20, "width": 30, "height": 40}))
	f.queue("Runtime.evaluate", runtimeResult(map[string]any{"x": 1, "y": 2, "width": 4, "height": 6}))
	f.queue("Runtime.evaluate", runtimeResult(true))
	f.queue("Runtime.evaluate", runtimeResult(true))
	f.queue("Runtime.evaluate", runtimeResult(true))
	f.queue("Runtime.evaluate", runtimeResult(true))
	f.queue("Runtime.evaluate", runtimeResult(true))

	ctx := context.Background()
	require.NoError(t, b.Click(ctx, "#ok", &protocol.ClickOptions{Button: "left", ClickCount: 2, Delay: 1}))
	require.NoError(t, b.Hover(ctx, "#ok"))
	require.NoError(t, b.Fill(ctx, "input", "abc"))
	require.NoError(t, b.Type(ctx, "input", "xy", 0))
	require.NoError(t, b.Press(ctx, "Enter"))
	require.NoError(t, b.Focus(ctx, "input"))
	require.NoError(t, b.Check(ctx, "input"))
	require.NoError(t, b.Uncheck(ctx, "input"))
	require.NoError(t, b.Select(ctx, "select", "a"))
	require.NoError(t, b.Scroll(ctx, 1, 2))

	methods := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		methods = append(methods, call.method)
	}
	assert.Contains(t, methods, "Input.dispatchMouseEvent")
	assert.Contains(t, methods, "Input.insertText")
	assert.Contains(t, methods, "Input.dispatchKeyEvent")
}

func TestBrowserDoubleClick(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", runtimeResult(map[string]any{"x": 10, "y": 20, "width": 30, "height": 40}))

	require.NoError(t, b.DoubleClick(context.Background(), "#ok"))
	require.Len(t, f.calls, 3)
	params := f.calls[1].params.(map[string]any)
	assert.Equal(t, 2, params["clickCount"])
}

func TestBrowserScreenshotOptions(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	png := []byte("png data")
	f.queue("Runtime.evaluate", runtimeResult(map[string]any{"x": 3, "y": 4, "width": 50, "height": 60}))
	f.queue("Page.captureScreenshot", `{"data":"`+base64.StdEncoding.EncodeToString(png)+`"}`)

	got, err := b.Screenshot(context.Background(), &protocol.ScreenshotOptions{
		Format:   "png",
		FullPage: true,
		Selector: "#card",
	})
	require.NoError(t, err)
	assert.Equal(t, png, got)

	require.Len(t, f.calls, 2)
	params, ok := f.calls[1].params.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "png", params["format"])
	assert.Equal(t, true, params["captureBeyondViewport"])
	clip := params["clip"].(map[string]any)
	assert.Equal(t, float64(3), clip["x"])
	assert.Equal(t, float64(50), clip["width"])
}

func TestBrowserScreenshotExplicitClipOverridesSelector(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", runtimeResult(map[string]any{"x": 3, "y": 4, "width": 50, "height": 60}))
	f.queue("Page.captureScreenshot", `{"data":"`+base64.StdEncoding.EncodeToString([]byte("clip"))+`"}`)

	_, err := b.Screenshot(context.Background(), &protocol.ScreenshotOptions{
		Selector:   "#card",
		ClipX:      8,
		ClipY:      9,
		ClipWidth:  10,
		ClipHeight: 11,
	})
	require.NoError(t, err)
	params := f.calls[1].params.(map[string]any)
	clip := params["clip"].(map[string]any)
	assert.Equal(t, float64(8), clip["x"])
	assert.Equal(t, float64(11), clip["height"])
}

func TestBrowserSnapshotFormatting(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("DOM.enable", `{}`)
	f.queue("Accessibility.enable", `{}`)
	f.queue("Accessibility.getFullAXTree", `{"nodes":[
		{"nodeId":"1","role":{"value":"RootWebArea"},"name":{"value":"Page"},"childIds":["2","3"]},
		{"nodeId":"2","role":{"value":"button"},"name":{"value":"Save"},"properties":[{"name":"disabled","value":{"value":true}}]},
		{"nodeId":"3","role":{"value":"StaticText"},"name":{"value":"Ignored by interactive"}}
	]}`)

	got, err := b.Snapshot(context.Background(), &protocol.SnapshotOptions{Interactive: true})
	require.NoError(t, err)
	assert.Contains(t, got, "[1] button Save [disabled]")
	assert.NotContains(t, got, "StaticText")
}

func TestBrowserPageSettingsCookiesAndTabs(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Network.getCookies", `{"cookies":[{"name":"sid","value":"1"}]}`)
	f.queue("Target.createTarget", `{"targetId":"target-1"}`)

	ctx := context.Background()
	require.NoError(t, b.SetViewport(ctx, 800, 600, 2))
	require.NoError(t, b.SetGeolocation(ctx, 1.2, 3.4, 5))
	require.NoError(t, b.SetOffline(ctx, true))
	require.NoError(t, b.SetHeaders(ctx, map[string]string{"X-Test": "1"}))
	cookies, err := b.GetCookies(ctx)
	require.NoError(t, err)
	assert.Equal(t, []protocol.Cookie{{Name: "sid", Value: "1"}}, cookies)
	require.NoError(t, b.SetCookie(ctx, protocol.Cookie{Name: "a", Value: "b", Domain: "example.com", Path: "/", HTTPOnly: true, Secure: true, SameSite: "Lax", Expires: 123}))
	require.NoError(t, b.ClearCookies(ctx))
	target, err := b.NewTab(ctx, "about:blank")
	require.NoError(t, err)
	assert.Equal(t, "target-1", target)
	require.NoError(t, b.CloseTab(ctx, target))
	require.NoError(t, b.Reload(ctx))
	require.NoError(t, b.GoBack(ctx))
	require.NoError(t, b.GoForward(ctx))
}

func TestBrowserWaitHelpers(t *testing.T) {
	t.Run("selector succeeds", func(t *testing.T) {
		f := newFakeCDP()
		b := New(f, nil)
		f.queue("Runtime.evaluate", runtimeResult(true))
		require.NoError(t, b.WaitForSelector(context.Background(), "#ready", 200))
	})

	t.Run("text succeeds", func(t *testing.T) {
		f := newFakeCDP()
		b := New(f, nil)
		f.queue("Runtime.evaluate", runtimeResult("hello world"))
		require.NoError(t, b.WaitForText(context.Background(), "world", 200))
	})

	t.Run("url succeeds", func(t *testing.T) {
		f := newFakeCDP()
		b := New(f, nil)
		f.queue("Runtime.evaluate", runtimeResult("https://example.com/done"))
		require.NoError(t, b.WaitForURL(context.Background(), "/done", 200))
	})

	t.Run("ms observes context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, bWithFake().WaitMS(ctx, 1000), context.Canceled)
	})
}

func TestBrowserConnectionAndClose(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	killed := false
	b.SetProcessKiller(func() { killed = true })

	assert.True(t, b.IsConnected())
	require.NoError(t, b.Close())
	assert.True(t, killed)
	assert.False(t, b.IsConnected())
}

func TestBrowserUsesPageClientWhenPresent(t *testing.T) {
	browserCDP := newFakeCDP()
	pageCDP := newFakeCDP()
	b := New(browserCDP, nil)
	b.pageCDP = pageCDP
	pageCDP.queue("Runtime.evaluate", runtimeResult("https://page.example"))

	assert.True(t, b.IsConnected())
	url, err := b.GetURL(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "https://page.example", url)
	assert.Empty(t, browserCDP.calls)
	require.NoError(t, b.Close())
	assert.False(t, pageCDP.IsConnected())
	assert.False(t, browserCDP.IsConnected())
}

func TestBrowserCloseReturnsClientError(t *testing.T) {
	f := newFakeCDP()
	f.closeErr = errors.New("close failed")
	b := New(f, nil)

	err := b.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close failed")
}

func TestBrowserCDPClientForFakeReturnsNil(t *testing.T) {
	assert.Nil(t, bWithFake().CDPClient())
}

func TestFormatAXTreeEmpty(t *testing.T) {
	assert.Equal(t, "(empty page)", formatAXTree(nil, nil))
}

func TestExtractHostPort(t *testing.T) {
	host, port := extractHostPort("wss://localhost:9333/devtools/browser/abc")
	assert.Equal(t, "localhost", host)
	assert.Equal(t, 9333, port)

	host, port = extractHostPort("ws://127.0.0.1/devtools/browser/abc")
	assert.Equal(t, "127.0.0.1", host)
	assert.Equal(t, 9222, port)
}

func bWithFake() *Browser {
	return New(newFakeCDP(), nil)
}

func TestWaitForLoadVariants(t *testing.T) {
	t.Run("domcontentloaded", func(t *testing.T) {
		f := newFakeCDP()
		b := New(f, nil)
		f.queue("Page.enable", `{}`)
		f.events <- cdp.Event{Method: "Page.domContentEventFired"}
		require.NoError(t, b.waitForLoad(context.Background(), "domcontentloaded", 100))
	})

	t.Run("networkidle", func(t *testing.T) {
		f := newFakeCDP()
		b := New(f, nil)
		f.queue("Network.enable", `{}`)
		require.NoError(t, b.waitForLoad(context.Background(), "networkidle", 1000))
	})

	t.Run("unknown state", func(t *testing.T) {
		require.NoError(t, bWithFake().waitForLoad(context.Background(), "commit", 100))
	})
}

func TestResolveNodeID(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("DOM.getDocument", `{"root":{"nodeId":7}}`)
	f.queue("DOM.querySelector", `{"nodeId":9}`)

	nodeID, err := b.resolveNodeID(context.Background(), "#x")
	require.NoError(t, err)
	assert.Equal(t, 9, nodeID)
}

func TestResolveNodeIDMissing(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("DOM.getDocument", `{"root":{"nodeId":7}}`)
	f.queue("DOM.querySelector", `{"nodeId":0}`)

	_, err := b.resolveNodeID(context.Background(), "#missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "element not found")
}

func TestResolveElementBoxEmpty(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", runtimeResult(map[string]any{"x": 1, "y": 2, "width": 0, "height": 3}))

	_, _, _, _, err := b.resolveElementBox(context.Background(), "#empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty bounding box")
}

func TestWaitForSelectorTimeout(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Runtime.evaluate", runtimeResult(false))

	err := b.WaitForSelector(context.Background(), "#never", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestWaitForNetworkIdleCountsRequests(t *testing.T) {
	f := newFakeCDP()
	b := New(f, nil)
	f.queue("Network.enable", `{}`)
	go func() {
		f.events <- cdp.Event{Method: "Network.requestWillBeSent"}
		f.events <- cdp.Event{Method: "Network.loadingFinished"}
	}()
	require.NoError(t, b.waitForNetworkIdle(context.Background(), 1000))
}

func TestWaitMSCompletes(t *testing.T) {
	start := time.Now()
	require.NoError(t, bWithFake().WaitMS(context.Background(), 1))
	assert.GreaterOrEqual(t, time.Since(start), time.Millisecond)
}

type browserRoundTripFunc func(*http.Request) (*http.Response, error)

func (f browserRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func stubBrowserHTTP(t *testing.T, fn browserRoundTripFunc) func() {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = fn
	return func() {
		http.DefaultTransport = old
	}
}

func browserJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

func stubBrowserConnect(t *testing.T, fn func(context.Context, string, *slog.Logger) (cdpClient, error)) func() {
	t.Helper()
	old := connectCDP
	connectCDP = fn
	return func() {
		connectCDP = old
	}
}
