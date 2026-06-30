// Package protocol defines the core types and message format for vibe-browser.
//
// The protocol is JSON-RPC style over Unix domain sockets (daemon mode) or
// direct in-process calls (SDK mode). Every command carries an action name
// and arbitrary extra fields; every response carries success, optional data,
// and an optional error string.
package protocol

import (
	"encoding/json"
	"time"
)

// Request is the wire format for a command sent to the daemon.
type Request struct {
	ID     string          `json:"id"`
	Action string          `json:"action"`
	Extra  json.RawMessage `json:"extra,omitempty"`
}

// Response is the wire format for a result returned by the daemon.
type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Warning string          `json:"warning,omitempty"`
}

// NavigationOptions configure page navigation.
type NavigationOptions struct {
	WaitUntil string `json:"waitUntil,omitempty"` // load, domcontentloaded, networkidle
	Timeout   int    `json:"timeout,omitempty"`    // milliseconds
}

// ClickOptions configure a click action.
type ClickOptions struct {
	Button     string `json:"button,omitempty"` // left, right, middle
	ClickCount int    `json:"clickCount,omitempty"`
	Delay      int    `json:"delay,omitempty"` // ms between down and up
}

// FillOptions configure a fill action.
type FillOptions struct {
	Timeout int `json:"timeout,omitempty"`
}

// ScreenshotOptions configure screenshot capture.
type ScreenshotOptions struct {
	Format     string `json:"format,omitempty"` // png, jpeg, webp
	Quality    int    `json:"quality,omitempty"`
	FullPage   bool   `json:"fullPage,omitempty"`
	Selector   string `json:"selector,omitempty"`
	ClipX      float64 `json:"clipX,omitempty"`
	ClipY      float64 `json:"clipY,omitempty"`
	ClipWidth  float64 `json:"clipWidth,omitempty"`
	ClipHeight float64 `json:"clipHeight,omitempty"`
}

// SnapshotOptions configure accessibility tree snapshot.
type SnapshotOptions struct {
	Selector    string `json:"selector,omitempty"`
	Interactive bool   `json:"interactive,omitempty"`
	Compact     bool   `json:"compact,omitempty"`
	Depth       int    `json:"depth,omitempty"`
	URLs        bool   `json:"urls,omitempty"`
}

// WaitOptions configure wait actions.
type WaitOptions struct {
	Timeout int    `json:"timeout,omitempty"` // milliseconds
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	LoadState string `json:"loadState,omitempty"` // load, domcontentloaded, networkidle
	Function string `json:"function,omitempty"`
}

// LaunchOptions configure browser launch.
type LaunchOptions struct {
	Headless        bool     `json:"headless,omitempty"`
	ExecutablePath  string   `json:"executablePath,omitempty"`
	Args            []string `json:"args,omitempty"`
	Proxy           string   `json:"proxy,omitempty"`
	UserDataDir     string   `json:"userDataDir,omitempty"`
	ViewportWidth   int      `json:"viewportWidth,omitempty"`
	ViewportHeight  int      `json:"viewportHeight,omitempty"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor,omitempty"`
	IgnoreHTTPSErrors bool   `json:"ignoreHttpsErrors,omitempty"`
	ColorScheme     string   `json:"colorScheme,omitempty"` // light, dark, no-preference
	Locale          string   `json:"locale,omitempty"`
	Timezone        string   `json:"timezone,omitempty"`
	Geolocation     *Geolocation `json:"geolocation,omitempty"`
	Offline         bool     `json:"offline,omitempty"`
	Extensions      []string `json:"extensions,omitempty"`
	Profile         string   `json:"profile,omitempty"`
}

// Geolocation represents geographic coordinates.
type Geolocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy,omitempty"`
}

// SessionInfo holds metadata about a daemon session.
type SessionInfo struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	Version   string    `json:"version,omitempty"`
	Engine    string    `json:"engine,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	StartTime time.Time `json:"startTime,omitempty"`
}

// TabInfo describes a browser tab.
type TabInfo struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	URL             string `json:"url"`
	IsActive        bool   `json:"isActive"`
	IsAttached      bool   `json:"isAttached"`
}

// NetworkRequest describes a captured network request.
type NetworkRequest struct {
	RequestID   string            `json:"requestId"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers,omitempty"`
	PostData    string            `json:"postData,omitempty"`
	ResourceType string           `json:"resourceType"`
	Timestamp   uint64            `json:"timestamp"`
	Status      int               `json:"status,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	MimeType    string            `json:"mimeType,omitempty"`
}

// Cookie represents a browser cookie.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

// StorageEntry represents a localStorage/sessionStorage entry.
type StorageEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// NodeRef is a reference to a DOM node via accessibility tree ref ID.
type NodeRef struct {
	RefID string `json:"refId"`
	Role  string `json:"role"`
	Name  string `json:"name"`
}

// ActionResult is the generic result of an action command.
type ActionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}
