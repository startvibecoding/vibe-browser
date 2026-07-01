package main

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/startvibecoding/vibe-browser/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSDKClient struct {
	closed    bool
	navigated string
}

func (f *fakeSDKClient) Navigate(ctx context.Context, url string) error {
	f.navigated = url
	return nil
}
func (f *fakeSDKClient) Title(context.Context) (string, error) { return "Example", nil }
func (f *fakeSDKClient) URL(context.Context) (string, error) {
	return "https://example.com", nil
}
func (f *fakeSDKClient) Snapshot(context.Context) (string, error) {
	return "button Example", nil
}
func (f *fakeSDKClient) Screenshot(context.Context) ([]byte, error) {
	return []byte("png"), nil
}
func (f *fakeSDKClient) GetText(context.Context, string) (string, error) { return "Heading", nil }
func (f *fakeSDKClient) IsVisible(context.Context, string) (bool, error) { return true, nil }
func (f *fakeSDKClient) Close() error {
	f.closed = true
	return nil
}

func TestExampleCDPSuccess(t *testing.T) {
	restoreExampleStubs(t)
	fake := &fakeSDKClient{}
	var wroteName string
	var wroteData []byte
	openClient = func(ctx context.Context, opts *client.Options) (sdkClient, error) {
		require.Equal(t, "ws://127.0.0.1:9222/devtools/browser", opts.CDPURL)
		return fake, nil
	}
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		wroteName = name
		wroteData = append([]byte(nil), data...)
		return nil
	}

	output := captureExampleOutput(t, func() {
		exampleCDP(context.Background())
	})

	assert.True(t, fake.closed)
	assert.Equal(t, "https://example.com", fake.navigated)
	assert.Equal(t, "screenshot.png", wroteName)
	assert.Equal(t, []byte("png"), wroteData)
	assert.Contains(t, output, "Page title: Example")
	assert.Contains(t, output, "H1 visible: true")
}

func TestExampleDaemonSuccess(t *testing.T) {
	restoreExampleStubs(t)
	fake := &fakeSDKClient{}
	connectClient = func(ctx context.Context, opts *client.Options) (sdkClient, error) {
		require.Equal(t, "my-session", opts.Session)
		return fake, nil
	}

	output := captureExampleOutput(t, func() {
		exampleDaemon(context.Background())
	})

	assert.True(t, fake.closed)
	assert.Equal(t, "https://example.com", fake.navigated)
	assert.Contains(t, output, "Page title: Example")
}

func TestExamplesReturnOnConnectionErrors(t *testing.T) {
	restoreExampleStubs(t)
	openClient = func(context.Context, *client.Options) (sdkClient, error) {
		return nil, errors.New("no chrome")
	}
	connectClient = func(context.Context, *client.Options) (sdkClient, error) {
		return nil, errors.New("no daemon")
	}

	output := captureExampleOutput(t, func() {
		exampleCDP(context.Background())
		exampleDaemon(context.Background())
	})

	assert.Contains(t, output, "Could not connect to Chrome")
	assert.Contains(t, output, "Could not connect to daemon")
}

func TestMainRunsBothExamples(t *testing.T) {
	restoreExampleStubs(t)
	openClient = func(context.Context, *client.Options) (sdkClient, error) {
		return &fakeSDKClient{}, nil
	}
	connectClient = func(context.Context, *client.Options) (sdkClient, error) {
		return &fakeSDKClient{}, nil
	}
	writeFile = func(string, []byte, os.FileMode) error { return nil }

	output := captureExampleOutput(t, main)

	assert.Contains(t, output, "=== Example 1: Connect via CDP ===")
	assert.Contains(t, output, "=== Example 2: Daemon Mode ===")
	assert.Contains(t, output, "Page title: Example")
}

func restoreExampleStubs(t *testing.T) {
	t.Helper()
	oldOpen := openClient
	oldConnect := connectClient
	oldWriteFile := writeFile
	t.Cleanup(func() {
		openClient = oldOpen
		connectClient = oldConnect
		writeFile = oldWriteFile
	})
}

func captureExampleOutput(t *testing.T, fn func()) string {
	t.Helper()
	temp := t.TempDir()
	out, err := os.Create(temp + "/out.log")
	require.NoError(t, err)
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = out
	os.Stderr = out
	oldLogOutput := log.Writer()
	log.SetOutput(out)

	fn()

	log.SetOutput(oldLogOutput)
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	require.NoError(t, out.Close())
	data, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	return string(data)
}
