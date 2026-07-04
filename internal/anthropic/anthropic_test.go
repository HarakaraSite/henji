package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

// TestRequestOmitsTopPWhenTemperatureSet is a regression test: Anthropic's API
// rejects requests that set both temperature and top_p ("temperature and
// top_p cannot both be specified for this model"). The default henji config
// sets both temp and topp globally (for compatibility with other providers),
// so every Anthropic request failed with a 400 until this was fixed.
func TestRequestOmitsTopPWhenTemperatureSet(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Config{AuthToken: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	temp := 1.0
	topP := 1.0
	s := client.Request(context.Background(), proto.Request{
		Model:       "claude-haiku-4-5",
		Messages:    []proto.Message{{Role: proto.RoleUser, Content: "hi"}},
		Temperature: &temp,
		TopP:        &topP,
	})
	defer s.Close() //nolint:errcheck

	require.Contains(t, gotBody, `"temperature":1`)
	require.NotContains(t, gotBody, "top_p")
}

// TestRequestJSONSchema verifies that proto.Request.JSONSchema is sent as
// the stable output_config.format.schema field (not the Beta namespace).
func TestRequestJSONSchema(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Config{AuthToken: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	schema := map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}
	s := client.Request(context.Background(), proto.Request{
		Model:      "claude-haiku-4-5",
		Messages:   []proto.Message{{Role: proto.RoleUser, Content: "hi"}},
		JSONSchema: schema,
	})
	defer s.Close() //nolint:errcheck

	var body map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &body))
	outputConfig, ok := body["output_config"].(map[string]any)
	require.True(t, ok, "output_config must be set")
	format, ok := outputConfig["format"].(map[string]any)
	require.True(t, ok, "output_config.format must be set")
	require.Equal(t, "json_schema", format["type"])
	require.EqualValues(t, schema, format["schema"])
}

// TestNextStaysFalseAfterStreamEnds is a regression test: Stream.Next() used
// to restart the underlying request (via a since-removed factory/done combo
// built for the MCP tool-call loop) whenever it was called again after
// already returning false once. With MCP removed, no caller does this
// anymore, but the interface contract ("false means done") must still hold:
// calling Next() again after it returns false must keep returning false
// without issuing another HTTP request.
func TestNextStaysFalseAfterStreamEnds(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Config{AuthToken: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	s := client.Request(context.Background(), proto.Request{
		Model:    "claude-haiku-4-5",
		Messages: []proto.Message{{Role: proto.RoleUser, Content: "hi"}},
	})
	defer s.Close() //nolint:errcheck

	require.False(t, s.Next(), "first Next() call should reach the end of the (empty) stream")
	require.NoError(t, s.Err())
	require.Equal(t, 1, requests)

	require.False(t, s.Next(), "Next() must keep returning false once the stream is done")
	require.False(t, s.Next(), "Next() must keep returning false on repeated calls")
	require.Equal(t, 1, requests, "Next() must not issue another request after the stream is done")
}
