// Package cdp implements the Chrome DevTools Protocol client.
//
// It manages a WebSocket connection to a Chrome/Chromium browser instance,
// dispatches CDP commands, and receives events.
package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

// Message represents a CDP JSON message (command or response/event).
type Message struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error is a CDP protocol error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("CDP error %d: %s", e.Code, e.Message)
}

// Event is a parsed CDP event with its method name and params.
type Event struct {
	Method    string
	Params    json.RawMessage
	SessionID string
}

// Client manages a WebSocket connection to a CDP endpoint.
type Client struct {
	conn      *websocket.Conn
	nextID    atomic.Int64
	mu        sync.Mutex
	pending   map[int64]chan *Message
	eventCh   chan Event
	done      chan struct{}
	closeOnce sync.Once
	logger    *slog.Logger
}

// Connect establishes a WebSocket connection to the given CDP URL.
func Connect(ctx context.Context, wsURL string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Create WebSocket config
	config, err := websocket.NewConfig(wsURL, wsURL)
	if err != nil {
		return nil, fmt.Errorf("cdp: config: %w", err)
	}

	// Dial with context
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", config.Location.Host)
	if err != nil {
		return nil, fmt.Errorf("cdp: dial %s: %w", wsURL, err)
	}

	// Perform WebSocket handshake
	ws, err := websocket.NewClient(config, conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("cdp: handshake: %w", err)
	}

	c := &Client{
		conn:    ws,
		pending: make(map[int64]chan *Message),
		eventCh: make(chan Event, 4096),
		done:    make(chan struct{}),
		logger:  logger,
	}

	go c.readLoop()
	go c.keepalive(ctx)

	return c, nil
}

// readLoop reads messages from the WebSocket and dispatches them.
func (c *Client) readLoop() {
	defer close(c.done)

	for {
		var data string
		err := websocket.Message.Receive(c.conn, &data)
		if err != nil {
			if err != io.EOF {
				c.logger.Debug("cdp: read error", "err", err)
			}
			c.mu.Lock()
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}

		var msg Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			c.logger.Warn("cdp: unmarshal error", "err", err, "data", data)
			continue
		}

		if msg.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()

			if ok {
				ch <- &msg
			}
		} else if msg.Method != "" {
			c.eventCh <- Event{
				Method: msg.Method,
				Params: msg.Params,
			}
		}
	}
}

// keepalive sends periodic ping frames.
func (c *Client) keepalive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.conn.PayloadType = websocket.PingFrame
			if _, err := c.conn.Write(nil); err != nil {
				return
			}
		}
	}
}

// Send sends a CDP command and waits for the response.
func (c *Client) Send(ctx context.Context, method string, params any) (*Message, error) {
	id := c.nextID.Add(1)

	payload := Message{
		ID:     id,
		Method: method,
	}

	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("cdp: marshal params: %w", err)
		}
		payload.Params = data
	}

	ch := make(chan *Message, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp: marshal command: %w", err)
	}

	c.mu.Lock()
	if err := websocket.Message.Send(c.conn, string(data)); err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp: write: %w", err)
	}
	c.mu.Unlock()

	select {
	case msg := <-ch:
		if msg == nil {
			return nil, fmt.Errorf("cdp: connection closed waiting for response to %s", method)
		}
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("cdp: connection closed")
	}
}

// SendToSession sends a CDP command targeting a specific session.
func (c *Client) SendToSession(ctx context.Context, method string, params any, sessionID string) (*Message, error) {
	id := c.nextID.Add(1)

	payload := map[string]any{
		"id":        id,
		"method":    method,
		"sessionId": sessionID,
	}
	if params != nil {
		payload["params"] = params
	}

	ch := make(chan *Message, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp: marshal command: %w", err)
	}

	c.mu.Lock()
	if err := websocket.Message.Send(c.conn, string(data)); err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp: write: %w", err)
	}
	c.mu.Unlock()

	select {
	case msg := <-ch:
		if msg == nil {
			return nil, fmt.Errorf("cdp: connection closed waiting for response to %s", method)
		}
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("cdp: connection closed")
	}
}

// Events returns the channel of incoming CDP events.
func (c *Client) Events() <-chan Event {
	return c.eventCh
}

// Close closes the WebSocket connection.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		// Send close frame
		closeData := []byte{0x03, 0xe8} // StatusNormalClosure = 1000
		c.conn.PayloadType = websocket.BinaryFrame
		c.conn.Write(closeData)
		err = c.conn.Close()
	})
	return err
}

// Done returns a channel that is closed when the connection is lost.
func (c *Client) Done() <-chan struct{} {
	return c.done
}

// IsConnected reports whether the WebSocket connection is still alive.
func (c *Client) IsConnected() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}
