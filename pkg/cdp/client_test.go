package cdp

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorString(t *testing.T) {
	assert.Equal(t, "CDP error -32000: no target", (&Error{Code: -32000, Message: "no target"}).Error())
}

func TestClientSendEventAndClose(t *testing.T) {
	c, conn := newTestClient(t)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		var msg map[string]any
		require.NoError(t, conn.readJSON(&msg))
		assert.Equal(t, "Runtime.evaluate", msg["method"])
		require.NoError(t, conn.writeJSON(map[string]any{
			"method": "Runtime.consoleAPICalled",
			"params": map[string]any{"type": "log"},
		}))
		require.NoError(t, conn.writeJSON(map[string]any{
			"id":     msg["id"],
			"result": map[string]any{"ok": true},
		}))
		_, _, _ = conn.readMessage()
	}()
	require.True(t, c.IsConnected())

	resp, err := c.Send(context.Background(), "Runtime.evaluate", map[string]any{"expression": "1"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(resp.Result))

	select {
	case evt := <-c.Events():
		assert.Equal(t, "Runtime.consoleAPICalled", evt.Method)
		assert.JSONEq(t, `{"type":"log"}`, string(evt.Params))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	require.NoError(t, c.Close())
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for close")
	}
	assert.False(t, c.IsConnected())
	<-serverDone
}

func TestClientSendToSession(t *testing.T) {
	c, conn := newTestClient(t)
	go func() {
		var msg map[string]any
		require.NoError(t, conn.readJSON(&msg))
		assert.Equal(t, "Page.navigate", msg["method"])
		assert.Equal(t, "session-1", msg["sessionId"])
		params := msg["params"].(map[string]any)
		assert.Equal(t, "https://example.com", params["url"])
		require.NoError(t, conn.writeJSON(map[string]any{
			"id":     msg["id"],
			"result": map[string]any{"frameId": "frame-1"},
		}))
	}()
	defer c.Close()

	resp, err := c.SendToSession(context.Background(), "Page.navigate", map[string]any{"url": "https://example.com"}, "session-1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"frameId":"frame-1"}`, string(resp.Result))
}

func TestClientProtocolError(t *testing.T) {
	c, conn := newTestClient(t)
	go func() {
		var msg map[string]any
		require.NoError(t, conn.readJSON(&msg))
		require.NoError(t, conn.writeJSON(map[string]any{
			"id": msg["id"],
			"error": map[string]any{
				"code":    -32601,
				"message": "method missing",
			},
		}))
	}()
	defer c.Close()

	_, err := c.Send(context.Background(), "Missing.method", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CDP error -32601")
}

func TestClientSendContextCanceled(t *testing.T) {
	c, conn := newTestClient(t)
	go func() {
		var msg map[string]any
		require.NoError(t, conn.readJSON(&msg))
		time.Sleep(100 * time.Millisecond)
	}()
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Send(ctx, "Runtime.evaluate", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestClientMarshalErrors(t *testing.T) {
	c, _ := newTestClient(t)
	defer c.Close()

	_, err := c.Send(context.Background(), "Runtime.evaluate", make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal params")

	_, err = c.SendToSession(context.Background(), "Runtime.evaluate", map[string]any{"bad": make(chan int)}, "session-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal command")
}

func TestClientConnectionClosedWhileWaiting(t *testing.T) {
	c, conn := newTestClient(t)
	go func() {
		var msg map[string]any
		require.NoError(t, conn.readJSON(&msg))
		require.NoError(t, conn.close())
	}()

	_, err := c.Send(context.Background(), "Runtime.evaluate", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

func TestClientKeepaliveSendsPingAndStops(t *testing.T) {
	oldInterval := keepaliveInterval
	keepaliveInterval = time.Millisecond
	defer func() { keepaliveInterval = oldInterval }()

	c, conn := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.keepalive(ctx)
	}()

	opcode, _, err := conn.readMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.PingMessage, opcode)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for keepalive to stop")
	}
}

func TestConnectError(t *testing.T) {
	_, err := Connect(context.Background(), "://bad-url", nil)
	require.Error(t, err)
}

func TestReadLoopIgnoresInvalidJSON(t *testing.T) {
	c, conn := newTestClient(t)
	go func() {
		require.NoError(t, conn.writeMessage(websocket.TextMessage, []byte("{")))
		var msg map[string]any
		require.NoError(t, conn.readJSON(&msg))
		require.NoError(t, conn.writeJSON(map[string]any{
			"id":     msg["id"],
			"result": map[string]any{"ok": true},
		}))
	}()
	defer c.Close()

	resp, err := c.Send(context.Background(), "Runtime.evaluate", nil)
	require.NoError(t, err)
	var got map[string]bool
	require.NoError(t, json.Unmarshal(resp.Result, &got))
	assert.True(t, got["ok"])
}

type testWSServer struct {
	conn net.Conn
	br   *bufio.Reader
}

func newTestClient(t *testing.T) (*Client, *testWSServer) {
	t.Helper()
	clientNet, serverNet := net.Pipe()
	server := &testWSServer{conn: serverNet, br: bufio.NewReader(serverNet)}
	handshakeDone := make(chan struct{})
	go func() {
		defer close(handshakeDone)
		require.NoError(t, server.handshake())
	}()

	u, err := url.Parse("ws://example.com/devtools/page/1")
	require.NoError(t, err)
	clientConn, _, err := websocket.NewClient(clientNet, u, nil, 1024, 1024)
	require.NoError(t, err)
	<-handshakeDone

	c := &Client{
		conn:    clientConn,
		pending: make(map[int64]chan *Message),
		eventCh: make(chan Event, 4096),
		done:    make(chan struct{}),
		logger:  slog.Default(),
	}
	go c.readLoop()
	t.Cleanup(func() {
		_ = c.Close()
		_ = server.close()
	})
	return c, server
}

func (s *testWSServer) handshake() error {
	req, err := http.ReadRequest(s.br)
	if err != nil {
		return err
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return fmt.Errorf("missing websocket key")
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, err = fmt.Fprintf(s.conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	return err
}

func (s *testWSServer) readJSON(v any) error {
	for {
		opcode, payload, err := s.readMessage()
		if err != nil {
			return err
		}
		switch opcode {
		case websocket.TextMessage:
			return json.Unmarshal(payload, v)
		case websocket.PingMessage:
			if err := s.writeMessage(websocket.PongMessage, payload); err != nil {
				return err
			}
		case websocket.CloseMessage:
			return io.EOF
		}
	}
}

func (s *testWSServer) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.writeMessage(websocket.TextMessage, data)
}

func (s *testWSServer) readMessage() (int, []byte, error) {
	b1, err := s.br.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	b2, err := s.br.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	length := uint64(b2 & 0x7f)
	switch length {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(s.br, buf[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(s.br, buf[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(buf[:])
	}
	var mask [4]byte
	if b2&0x80 != 0 {
		if _, err := io.ReadFull(s.br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.br, payload); err != nil {
		return 0, nil, err
	}
	if b2&0x80 != 0 {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return int(b1 & 0x0f), payload, nil
}

func (s *testWSServer) writeMessage(opcode int, payload []byte) error {
	header := []byte{byte(0x80 | opcode)}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 0xffff:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 127)
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(len(payload)))
		header = append(header, buf[:]...)
	}
	if _, err := s.conn.Write(header); err != nil {
		return err
	}
	_, err := s.conn.Write(payload)
	return err
}

func (s *testWSServer) close() error {
	return s.conn.Close()
}
