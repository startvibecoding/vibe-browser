package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerRegistersTools(t *testing.T) {
	s := NewServer(nil, "test")
	assert.Equal(t, "test", s.session)
	assert.NotNil(t, s.logger)
	assert.NotNil(t, s.in)
	assert.NotNil(t, s.out)
	assert.Contains(t, s.tools, "vibe_browser_navigate")
	assert.Contains(t, s.tools, "vibe_browser_close")
}

func TestRunHandlesJSONRPCMethods(t *testing.T) {
	input := strings.Join([]string{
		`not-json`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":4,"method":"missing"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		``,
	}, "\n")
	var out bytes.Buffer
	s := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), "test")
	s.in = strings.NewReader(input)
	s.out = &out

	require.NoError(t, s.Run(context.Background()))
	responses := decodeJSONLines(t, out.String())
	require.Len(t, responses, 4)
	assert.Equal(t, float64(1), responses[0]["id"])
	assert.Equal(t, float64(2), responses[1]["id"])
	assert.Equal(t, float64(3), responses[2]["id"])
	assert.Equal(t, float64(4), responses[3]["id"])
	assert.Contains(t, responses[3]["error"].(map[string]any)["message"], "Method not found")
}

func TestHandleToolCall(t *testing.T) {
	var out bytes.Buffer
	s := NewServer(nil, "test")
	s.out = &out

	s.handleToolCall(context.Background(), "missing-params", map[string]any{})
	s.handleToolCall(context.Background(), "missing-name", map[string]any{"params": map[string]any{}})
	s.handleToolCall(context.Background(), "unknown", map[string]any{"params": map[string]any{"name": "nope"}})

	s.handleToolCall(context.Background(), "ok-string", map[string]any{
		"params": map[string]any{
			"name":      "vibe_browser_navigate",
			"arguments": map[string]any{"url": "https://example.com"},
		},
	})

	s.tools["bytes"] = func(context.Context, json.RawMessage) (any, error) { return []byte{0xde, 0xad}, nil }
	s.handleToolCall(context.Background(), "ok-bytes", map[string]any{"params": map[string]any{"name": "bytes"}})

	s.tools["object"] = func(context.Context, json.RawMessage) (any, error) { return map[string]any{"ok": true}, nil }
	s.handleToolCall(context.Background(), "ok-object", map[string]any{"params": map[string]any{"name": "object"}})

	s.tools["err"] = func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("boom") }
	s.handleToolCall(context.Background(), "handler-error", map[string]any{"params": map[string]any{"name": "err"}})

	responses := decodeJSONLines(t, out.String())
	require.Len(t, responses, 7)
	assert.Contains(t, responses[0]["error"].(map[string]any)["message"], "Missing params")
	assert.Contains(t, responses[1]["error"].(map[string]any)["message"], "Missing tool name")
	assert.Contains(t, responses[2]["error"].(map[string]any)["message"], "Tool not found")

	assertContentText(t, responses[3], "Navigated to https://example.com")
	content := responses[4]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "image", content["type"])
	assert.Equal(t, "dead", content["data"])
	assertContentText(t, responses[5], "{\n  \"ok\": true\n}")
	assert.True(t, responses[6]["result"].(map[string]any)["isError"].(bool))
	assertContentText(t, responses[6], "Error: boom")
}

func TestToolArguments(t *testing.T) {
	raw, err := toolArguments(nil)
	require.NoError(t, err)
	assert.Nil(t, raw)

	raw, err = toolArguments(json.RawMessage(`{"a":1}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(raw))

	raw, err = toolArguments(map[string]any{"a": 1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(raw))

	_, err = toolArguments(make(chan int))
	require.Error(t, err)
}

func TestHandlers(t *testing.T) {
	s := NewServer(nil, "session-1")
	ctx := context.Background()

	cases := []struct {
		name string
		fn   func(context.Context, json.RawMessage) (any, error)
		args string
		want any
	}{
		{"open", s.handleOpen, `{}`, "Browser opened. Use vibe_browser_navigate to go to a URL."},
		{"navigate", s.handleNavigate, `{"url":"https://example.com"}`, "Navigated to https://example.com"},
		{"back", s.handleBack, `{}`, "Navigated back"},
		{"forward", s.handleForward, `{}`, "Navigated forward"},
		{"reload", s.handleReload, `{}`, "Page reloaded"},
		{"snapshot", s.handleSnapshot, `{}`, "Snapshot captured"},
		{"click", s.handleClick, `{"selector":"#a"}`, "Clicked #a"},
		{"dblclick", s.handleDoubleClick, `{"selector":"#a"}`, "Double-clicked #a"},
		{"fill", s.handleFill, `{"selector":"#a","value":"x"}`, `Filled #a with "x"`},
		{"type", s.handleType, `{"selector":"#a","text":"x"}`, `Typed "x" into #a`},
		{"press", s.handlePress, `{"key":"Enter"}`, "Pressed Enter"},
		{"hover", s.handleHover, `{"selector":"#a"}`, "Hovered over #a"},
		{"scroll", s.handleScroll, `{}`, "Scrolled"},
		{"focus", s.handleFocus, `{"selector":"#a"}`, "Focused #a"},
		{"check", s.handleCheck, `{"selector":"#a"}`, "Checked #a"},
		{"uncheck", s.handleUncheck, `{"selector":"#a"}`, "Unchecked #a"},
		{"select", s.handleSelect, `{"selector":"#a","value":"x"}`, `Selected "x" in #a`},
		{"screenshot", s.handleScreenshot, `{}`, "Screenshot captured"},
		{"get_text", s.handleGetText, `{"selector":"#a"}`, "Text from #a"},
		{"get_html", s.handleGetHTML, `{}`, "HTML content"},
		{"get_value", s.handleGetValue, `{"selector":"#a"}`, "Value from #a"},
		{"get_attr", s.handleGetAttr, `{"selector":"#a","attr":"href"}`, "Attribute href from #a"},
		{"get_url", s.handleGetURL, `{}`, "about:blank"},
		{"get_title", s.handleGetTitle, `{}`, ""},
		{"is_visible", s.handleIsVisible, `{}`, true},
		{"is_enabled", s.handleIsEnabled, `{}`, true},
		{"is_checked", s.handleIsChecked, `{}`, false},
		{"eval", s.handleEval, `{"expression":"1"}`, "Evaluated: 1"},
		{"wait_ms", s.handleWaitMS, `{"ms":5}`, "Waited 5 ms"},
		{"wait_for_selector", s.handleWaitForSelector, `{"selector":"#a"}`, "Found #a"},
		{"wait_for_text", s.handleWaitForText, `{"text":"ready"}`, `Found text "ready"`},
		{"wait_for_url", s.handleWaitForURL, `{"url":"/ready"}`, "URL matched /ready"},
		{"set_viewport", s.handleSetViewport, `{"width":800,"height":600}`, "Viewport set to 800x600"},
		{"set_geolocation", s.handleSetGeolocation, `{}`, "Geolocation set"},
		{"set_offline_true", s.handleSetOffline, `{"offline":true}`, "Offline mode enabled"},
		{"set_offline_false", s.handleSetOffline, `{"offline":false}`, "Offline mode disabled"},
		{"set_headers", s.handleSetHeaders, `{}`, "Headers set"},
		{"cookies_clear", s.handleCookiesClear, `{}`, "Cookies cleared"},
		{"cookies_set", s.handleCookiesSet, `{"name":"sid","value":"1"}`, "Cookie sid set"},
		{"tab_new", s.handleTabNew, `{"url":"about:blank"}`, "New tab opened with URL about:blank"},
		{"tab_close", s.handleTabClose, `{"targetId":"target-1"}`, "Tab target-1 closed"},
		{"close", s.handleClose, `{}`, "Browser closed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(ctx, json.RawMessage(tc.args))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	cookies, err := s.handleCookiesGet(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, []any{}, cookies)

	session, err := s.handleSession(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"session": "session-1", "status": "active"}, session)
}

func TestHandlersInvalidJSON(t *testing.T) {
	s := NewServer(nil, "test")
	handlers := []func(context.Context, json.RawMessage) (any, error){
		s.handleNavigate,
		s.handleClick,
		s.handleDoubleClick,
		s.handleFill,
		s.handleType,
		s.handlePress,
		s.handleHover,
		s.handleFocus,
		s.handleCheck,
		s.handleUncheck,
		s.handleSelect,
		s.handleGetText,
		s.handleGetValue,
		s.handleGetAttr,
		s.handleEval,
		s.handleWaitMS,
		s.handleWaitForSelector,
		s.handleWaitForText,
		s.handleWaitForURL,
		s.handleSetViewport,
		s.handleSetOffline,
		s.handleCookiesSet,
		s.handleTabNew,
		s.handleTabClose,
	}
	for _, handler := range handlers {
		_, err := handler(context.Background(), json.RawMessage(`{`))
		require.Error(t, err)
	}
}

func TestWriteResultAndError(t *testing.T) {
	var out bytes.Buffer
	s := NewServer(nil, "test")
	s.out = &out
	s.writeResult("id1", map[string]any{"ok": true})
	s.writeError("id2", -1, "bad")
	responses := decodeJSONLines(t, out.String())
	require.Len(t, responses, 2)
	assert.Equal(t, "id1", responses[0]["id"])
	assert.Equal(t, "id2", responses[1]["id"])
}

func decodeJSONLines(t *testing.T, text string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(text), "\n")
	out := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var msg map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &msg))
		out = append(out, msg)
	}
	return out
}

func assertContentText(t *testing.T, resp map[string]any, want string) {
	t.Helper()
	content := resp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "text", content["type"])
	assert.Equal(t, want, content["text"])
}
