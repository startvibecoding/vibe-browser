package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestSerialization(t *testing.T) {
	req := Request{
		ID:     "r123",
		Action: "navigate",
		Extra:  json.RawMessage(`{"url":"https://example.com"}`),
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded Request
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "r123", decoded.ID)
	assert.Equal(t, "navigate", decoded.Action)
}

func TestResponseSerialization(t *testing.T) {
	resp := Response{
		Success: true,
		Data:    json.RawMessage(`{"title":"Example"}`),
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
	assert.NotNil(t, decoded.Data)
	assert.Empty(t, decoded.Error)
}

func TestResponseWithError(t *testing.T) {
	resp := Response{
		Success: false,
		Error:   "element not found",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.False(t, decoded.Success)
	assert.Equal(t, "element not found", decoded.Error)
}

func TestLaunchOptions(t *testing.T) {
	opts := LaunchOptions{
		Headless:       true,
		ExecutablePath: "/usr/bin/google-chrome",
		Args:           []string{"--no-sandbox"},
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	data, err := json.Marshal(opts)
	require.NoError(t, err)

	var decoded LaunchOptions
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Headless)
	assert.Equal(t, "/usr/bin/google-chrome", decoded.ExecutablePath)
	assert.Equal(t, 1920, decoded.ViewportWidth)
	assert.Equal(t, 1080, decoded.ViewportHeight)
}

func TestCookieSerialization(t *testing.T) {
	cookie := Cookie{
		Name:     "session",
		Value:    "abc123",
		Domain:   ".example.com",
		Path:     "/",
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	}

	data, err := json.Marshal(cookie)
	require.NoError(t, err)

	var decoded Cookie
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "session", decoded.Name)
	assert.Equal(t, "abc123", decoded.Value)
	assert.Equal(t, ".example.com", decoded.Domain)
	assert.True(t, decoded.HTTPOnly)
	assert.True(t, decoded.Secure)
	assert.Equal(t, "Lax", decoded.SameSite)
}
