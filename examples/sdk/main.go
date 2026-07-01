// Example: Using vibe-browser as a Go SDK
//
// This example demonstrates how to use the vibe-browser SDK
// to automate browser interactions programmatically.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/startvibecoding/vibe-browser/pkg/client"
)

type sdkClient interface {
	Navigate(context.Context, string) error
	Title(context.Context) (string, error)
	URL(context.Context) (string, error)
	Snapshot(context.Context) (string, error)
	Screenshot(context.Context) ([]byte, error)
	GetText(context.Context, string) (string, error)
	IsVisible(context.Context, string) (bool, error)
	Close() error
}

var (
	openClient    = func(ctx context.Context, opts *client.Options) (sdkClient, error) { return client.Open(ctx, opts) }
	connectClient = func(ctx context.Context, opts *client.Options) (sdkClient, error) { return client.Connect(ctx, opts) }
	writeFile     = os.WriteFile
)

func main() {
	ctx := context.Background()

	// Example 1: Connect to an existing browser via CDP
	fmt.Println("=== Example 1: Connect via CDP ===")
	exampleCDP(ctx)

	// Example 2: Use daemon mode
	fmt.Println("\n=== Example 2: Daemon Mode ===")
	exampleDaemon(ctx)
}

func exampleCDP(ctx context.Context) {
	// Connect to a running Chrome instance
	// Start Chrome with: google-chrome --remote-debugging-port=9222
	c, err := openClient(ctx, &client.Options{
		CDPURL: "ws://127.0.0.1:9222/devtools/browser",
	})
	if err != nil {
		log.Printf("Could not connect to Chrome: %v", err)
		log.Println("Start Chrome with: google-chrome --remote-debugging-port=9222")
		return
	}
	defer c.Close()

	// Navigate to a page
	if err := c.Navigate(ctx, "https://example.com"); err != nil {
		log.Fatal(err)
	}

	// Get the page title
	title, err := c.Title(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Page title:", title)

	// Get the URL
	url, err := c.URL(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Current URL:", url)

	// Take a snapshot
	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Snapshot:")
	fmt.Println(snapshot)

	// Take a screenshot
	screenshot, err := c.Screenshot(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := writeFile("screenshot.png", screenshot, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Screenshot saved to screenshot.png")

	// Get page text
	text, err := c.GetText(ctx, "h1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("H1 text:", text)

	// Check if element is visible
	visible, err := c.IsVisible(ctx, "h1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("H1 visible:", visible)
}

func exampleDaemon(ctx context.Context) {
	// Connect to a running daemon
	// Start daemon with: vibe-browser daemon --session my-session
	c, err := connectClient(ctx, &client.Options{
		Session: "my-session",
	})
	if err != nil {
		log.Printf("Could not connect to daemon: %v", err)
		log.Println("Start daemon with: vibe-browser daemon --session my-session")
		return
	}
	defer c.Close()

	// Navigate to a page
	if err := c.Navigate(ctx, "https://example.com"); err != nil {
		log.Fatal(err)
	}

	// Get the page title
	title, err := c.Title(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Page title:", title)
}
