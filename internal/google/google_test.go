package google

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge.harakara.site/littleisland/henji/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestStreamMessagesIncludesRequestAndAssistantResponse(t *testing.T) {
	s := &Stream{
		reader: bufio.NewReader(strings.NewReader(
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n" +
				"\n" +
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" world\"}]}}]}\n",
		)),
		unmarshaler: &JSONUnmarshaler{},
		messages: []proto.Message{
			{Role: proto.RoleUser, Content: "prompt"},
		},
	}

	chunk, err := s.Current()
	require.NoError(t, err)
	require.Equal(t, "hello", chunk.Content)

	chunk, err = s.Current()
	require.NoError(t, err)
	require.Equal(t, " world", chunk.Content)

	require.Equal(t, []proto.Message{
		{Role: proto.RoleUser, Content: "prompt"},
		{Role: proto.RoleAssistant, Content: "hello world"},
	}, s.Messages())
}

// TestCloseNilResponseNoPanic is the PR#4 regression test.
// Close must be safe to call when the HTTP response is nil (request failed before
// the stream was established).
func TestCloseNilResponseNoPanic(t *testing.T) {
	s := &Stream{} // response is nil by default
	require.NoError(t, s.Close())
}

// TestSendRequestStreamAPIErrorNoPanic is a regression test: when the Google
// API returns a non-2xx response, the returned Stream must have isFinished
// set so callers stop after Next() instead of calling Current() on a Stream
// with a nil reader (which panicked with a nil pointer dereference).
func TestSendRequestStreamAPIErrorNoPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"invalid API key","status":"UNAUTHENTICATED"}}`))
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	req, err := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	s, sendErr := googleSendRequestStream(client, req)
	require.Error(t, sendErr)
	require.False(t, s.Next(), "Next() must return false so Current() (nil reader) is never called")

	// The wrapped error must be safe to render: (*openai.Error).Error() panics
	// if its Request/Response fields are nil, which handleErrorResp used to leave unset.
	require.NotPanics(t, func() { _ = sendErr.Error() })
}

// TestRequestJSONSchema verifies that proto.Request.JSONSchema is sent as
// generationConfig.responseSchema, alongside responseMimeType:"application/json".
func TestRequestJSONSchema(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	schema := map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}
	s := client.Request(context.Background(), proto.Request{
		Messages:   []proto.Message{{Role: proto.RoleUser, Content: "hi"}},
		JSONSchema: schema,
	})
	defer s.Close() //nolint:errcheck

	var body map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &body))
	genConfig, ok := body["generationConfig"].(map[string]any)
	require.True(t, ok, "generationConfig must be set")
	require.Equal(t, "application/json", genConfig["responseMimeType"])
	require.EqualValues(t, schema, genConfig["responseSchema"])
}

// TestRequestSendsExplicitZeroSamplingValues is the F-2 regression test.
// GenerationConfig used value types with omitempty, so an explicit temp:0
// (deterministic output) was indistinguishable from "unset" and dropped
// from the request body, silently falling back to the server default.
func TestRequestSendsExplicitZeroSamplingValues(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	temp, topP, topK := 0.0, 0.0, int64(0)
	s := client.Request(context.Background(), proto.Request{
		Messages:    []proto.Message{{Role: proto.RoleUser, Content: "hi"}},
		Temperature: &temp,
		TopP:        &topP,
		TopK:        &topK,
	})
	defer s.Close() //nolint:errcheck

	var body map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &body))
	genConfig, ok := body["generationConfig"].(map[string]any)
	require.True(t, ok, "generationConfig must be set")
	require.Contains(t, genConfig, "temperature")
	require.Equal(t, 0.0, genConfig["temperature"])
	require.Contains(t, genConfig, "topP")
	require.Equal(t, 0.0, genConfig["topP"])
	require.Contains(t, genConfig, "topK")
	require.Equal(t, 0.0, genConfig["topK"])
}

// TestRequestOmitsUnsetSamplingValues verifies unset (nil) sampling fields
// are still omitted from the request body, so the server default applies.
func TestRequestOmitsUnsetSamplingValues(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	s := client.Request(context.Background(), proto.Request{
		Messages: []proto.Message{{Role: proto.RoleUser, Content: "hi"}},
	})
	defer s.Close() //nolint:errcheck

	var body map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &body))
	genConfig, ok := body["generationConfig"].(map[string]any)
	require.True(t, ok, "generationConfig must be set")
	require.NotContains(t, genConfig, "temperature")
	require.NotContains(t, genConfig, "topP")
	require.NotContains(t, genConfig, "topK")
}

func TestStreamMessagesReturnsCopy(t *testing.T) {
	s := &Stream{
		messages: []proto.Message{
			{Role: proto.RoleUser, Content: "prompt"},
		},
		content: "response",
	}

	messages := s.Messages()
	messages[0].Content = "changed"

	require.Equal(t, []proto.Message{
		{Role: proto.RoleUser, Content: "prompt"},
		{Role: proto.RoleAssistant, Content: "response"},
	}, s.Messages())
}
