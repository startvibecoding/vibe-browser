package chrome

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDiscoverCDPURLFromVersionEndpoint(t *testing.T) {
	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/json/version", r.URL.Path)
		return jsonResponse(200, `{"webSocketDebuggerUrl":"ws://127.0.0.1:1111/devtools/browser/abc"}`), nil
	})
	defer restore()

	got, err := DiscoverCDPURL("chrome.local", 9333)
	require.NoError(t, err)
	assert.Equal(t, "ws://chrome.local:9333/devtools/browser/abc", got)
}

func TestDiscoverCDPURLFallsBackToListEndpoint(t *testing.T) {
	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/json/version":
			return jsonResponse(404, `{}`), nil
		case "/json/list":
			return jsonResponse(200, `[
				{"type":"page","webSocketDebuggerUrl":"ws://127.0.0.1:1111/devtools/page/p"},
				{"type":"browser","webSocketDebuggerUrl":"ws://127.0.0.1:1111/devtools/browser/b"}
			]`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	defer restore()

	got, err := DiscoverCDPURL("", 0)
	require.NoError(t, err)
	assert.Equal(t, "ws://127.0.0.1:9222/devtools/browser/b", got)
}

func TestDiscoverCDPURLListFallbackFirstTarget(t *testing.T) {
	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/json/version" {
			return jsonResponse(200, `{}`), nil
		}
		return jsonResponse(200, `[{"type":"page","webSocketDebuggerUrl":"wss://example.test:443/devtools/page/p"}]`), nil
	})
	defer restore()

	got, err := DiscoverCDPURL("host", 1234)
	require.NoError(t, err)
	assert.Equal(t, "wss://host:1234/devtools/page/p", got)
}

func TestProcessCDPWebSocketURL(t *testing.T) {
	assert.Equal(t, "ws://cdp", (&Process{CDPURL: "ws://cdp"}).CDPWebSocketURL())
}

func TestAutoConnectCDPFromDevToolsActivePort(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test controls HOME for linux user-data paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "google-chrome")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte("9333\n/devtools/browser/abc\n"), 0644))

	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/json/version", r.URL.Path)
		return jsonResponse(200, `{"webSocketDebuggerUrl":"ws://127.0.0.1:9333/devtools/browser/abc"}`), nil
	})
	defer restore()

	got, err := AutoConnectCDP()
	require.NoError(t, err)
	assert.Equal(t, "ws://127.0.0.1:9333/devtools/browser/abc", got)
}

func TestDiscoverCDPURLErrors(t *testing.T) {
	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `not-json`), nil
	})
	defer restore()

	_, err := DiscoverCDPURL("host", 1234)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no browser found")
}

func TestRewriteWSHost(t *testing.T) {
	assert.Equal(t, "ws://host:1/path", rewriteWSHost("ws://127.0.0.1:2/path", "host", 1))
	assert.Equal(t, "wss://host:1/path", rewriteWSHost("wss://127.0.0.1:2/path", "host", 1))
	assert.Equal(t, "http://127.0.0.1/path", rewriteWSHost("http://127.0.0.1/path", "host", 1))
}

func TestReadDevToolsActivePort(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte("9229\n/devtools/browser/abc\n"), 0644))
	port, path := readDevToolsActivePort(dir)
	assert.Equal(t, 9229, port)
	assert.Equal(t, "/devtools/browser/abc", path)

	port, path = readDevToolsActivePort(filepath.Join(dir, "missing"))
	assert.Zero(t, port)
	assert.Empty(t, path)
}

func TestBrowserCandidates(t *testing.T) {
	assert.NotEmpty(t, getBrowserCandidates(BrowserChrome))
	assert.Nil(t, getDarwinCandidates("unknown"))
	assert.Nil(t, getLinuxCandidates("unknown"))
	assert.Nil(t, getWindowsCandidates("unknown"))
}

func TestFindBrowser(t *testing.T) {
	dir := t.TempDir()
	name := "google-chrome"
	if runtime.GOOS == "windows" {
		name = "google-chrome.exe"
	}
	exe := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\n"), 0755))
	t.Setenv("PATH", dir)

	path, err := FindBrowser(BrowserChrome)
	require.NoError(t, err)
	assert.Equal(t, exe, path)

	_, err = FindBrowser(BrowserType("definitely-missing-browser"))
	require.Error(t, err)
}

func TestListTargetsAndVersion(t *testing.T) {
	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/json/list":
			return jsonResponse(200, `[{"id":"1","type":"page"}]`), nil
		case "/json/version":
			return jsonResponse(200, `{"Browser":"Chrome/1"}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	defer restore()

	targets, err := ListTargets("", 0)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "1", targets[0]["id"])

	version, err := GetBrowserVersion("", 0)
	require.NoError(t, err)
	assert.Equal(t, "Chrome/1", version["Browser"])
}

func TestListTargetsAndVersionDecodeErrors(t *testing.T) {
	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `not-json`), nil
	})
	defer restore()

	_, err := ListTargets("host", 1)
	require.Error(t, err)
	_, err = GetBrowserVersion("host", 1)
	require.Error(t, err)
}

func TestListChromeProfiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Local State"), []byte(`{
		"profile":{"info_cache":{"Default":{"name":"Person 1"},"Profile 2":{}}}
	}`), 0644))
	profiles := ListChromeProfiles(dir)
	require.Len(t, profiles, 2)

	got := map[string]string{}
	for _, profile := range profiles {
		got[profile["directory"]] = profile["name"]
	}
	assert.Equal(t, "Person 1", got["Default"])
	assert.Equal(t, "Profile 2", got["Profile 2"])
	assert.Nil(t, ListChromeProfiles(filepath.Join(dir, "missing")))
}

func TestFindChromeUserDataDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test controls HOME for linux paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	userDir := filepath.Join(home, ".config", "google-chrome")
	require.NoError(t, os.MkdirAll(userDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "Local State"), []byte(`{}`), 0644))
	assert.Equal(t, userDir, FindChromeUserDataDir())
}

func TestWaitForCDPContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitForCDP(ctx, "127.0.0.1", 1, slog.Default())
	require.ErrorIs(t, err, context.Canceled)
}

func TestGetUserDataDirs(t *testing.T) {
	dirs := getUserDataDirs()
	if runtime.GOOS == "linux" {
		require.NotEmpty(t, dirs)
		assert.True(t, strings.Contains(strings.Join(dirs, "\n"), ".config/google-chrome"))
	}
}

func stubHTTPClient(t *testing.T, fn roundTripFunc) func() {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = fn
	return func() {
		http.DefaultTransport = old
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

func TestProcessKillRemovesTempDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vibe-browser-test")
	require.NoError(t, os.MkdirAll(dir, 0755))
	p := &Process{UserDataDir: dir}
	p.Kill()
	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err))
}

func TestLaunchFindBrowserError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := Launch(ctx, LaunchOptions{Browser: BrowserType("definitely-missing-browser")}, slog.Default())
	require.Error(t, err)
}

func TestLaunchSuccessMapsOptionsAndKillsTempProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake browser is unix-only")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	exe := filepath.Join(dir, "fake-browser")
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\nprintf '%s\n' \"$@\" > "+argsPath+"\nexec sleep 30\n"), 0755))

	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/json/version", r.URL.Path)
		return jsonResponse(200, `{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/launched"}`), nil
	})
	defer restore()

	proc, err := Launch(context.Background(), LaunchOptions{
		ExecutablePath: exe,
		Headless:       true,
		Args:           []string{"--custom"},
		Proxy:          "http://proxy",
		ViewportWidth:  800,
		ViewportHeight: 600,
		Extensions:     []string{"/ext/a", "/ext/b"},
		Profile:        "Profile 1",
	}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, proc)
	defer proc.Kill()

	assert.Contains(t, proc.CDPURL, "/devtools/browser/launched")
	assert.Contains(t, proc.UserDataDir, "vibe-browser-")
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(argsPath)
		if err != nil {
			return false
		}
		args := string(data)
		return strings.Contains(args, "--headless") &&
			strings.Contains(args, "--custom") &&
			strings.Contains(args, "--proxy-server=http://proxy") &&
			strings.Contains(args, "--window-size=800,600") &&
			strings.Contains(args, "--load-extension=/ext/a,/ext/b") &&
			strings.Contains(args, "--profile-directory=Profile 1")
	}, time.Second, 10*time.Millisecond)

	tempProfile := proc.UserDataDir
	proc.Kill()
	_, err = os.Stat(tempProfile)
	assert.True(t, os.IsNotExist(err))
}

func TestLaunchUsesDefaultViewportSize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake browser is unix-only")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	exe := filepath.Join(dir, "fake-browser")
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\nprintf '%s\n' \"$@\" > "+argsPath+"\nexec sleep 30\n"), 0755))

	restore := stubHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/json/version", r.URL.Path)
		return jsonResponse(200, `{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/launched"}`), nil
	})
	defer restore()

	proc, err := Launch(context.Background(), LaunchOptions{
		ExecutablePath: exe,
	}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, proc)
	defer proc.Kill()

	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(argsPath)
		if err != nil {
			return false
		}
		return strings.Contains(string(data), "--window-size=1920,1080")
	}, time.Second, 10*time.Millisecond)
}
