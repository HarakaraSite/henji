package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

// newMockOpenAIServer starts an httptest server that captures the request body
// and responds with a minimal SSE "[DONE]" event to terminate the stream.
func newMockOpenAIServer(t *testing.T) (*Client, func() map[string]any) {
	t.Helper()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	c := New(cfg)

	return c, func() map[string]any {
		var body map[string]any
		if err := json.Unmarshal(captured, &body); err != nil {
			t.Fatalf("failed to parse captured request body: %v", err)
		}
		return body
	}
}

// TestRequestMaxCompletionTokens verifies PR#14 R-1:
// proto.Request.MaxCompletionTokens must reach the OpenAI API payload.
func TestRequestMaxCompletionTokens(t *testing.T) {
	c, body := newMockOpenAIServer(t)

	maxComp := int64(1000)
	s := c.Request(context.Background(), proto.Request{
		Model:               "o1-mini",
		MaxCompletionTokens: &maxComp,
	})
	s.Next()
	s.Close() //nolint:errcheck

	got := body()
	require.EqualValues(t, 1000, got["max_completion_tokens"], "max_completion_tokens must be wired through")
	require.Nil(t, got["max_tokens"], "max_tokens must not be set when only max_completion_tokens is provided")
}

// TestRequestMaxTokens verifies that the ordinary max_tokens path still works.
func TestRequestMaxTokens(t *testing.T) {
	c, body := newMockOpenAIServer(t)

	maxTok := int64(512)
	s := c.Request(context.Background(), proto.Request{
		Model:     "gpt-4o",
		MaxTokens: &maxTok,
	})
	s.Next()
	s.Close() //nolint:errcheck

	got := body()
	require.EqualValues(t, 512, got["max_tokens"])
	require.Nil(t, got["max_completion_tokens"])
}

// TestRequestJSONSchemaOpenAIStrict verifies that proto.Request.JSONSchema is
// sent as response_format.json_schema, with strict:true only for the real
// "openai" dialect.
func TestRequestJSONSchemaOpenAIStrict(t *testing.T) {
	c, body := newMockOpenAIServer(t)

	schema := map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}
	s := c.Request(context.Background(), proto.Request{
		Model:      "gpt-4o",
		API:        "openai",
		JSONSchema: schema,
	})
	s.Next()
	s.Close() //nolint:errcheck

	got := body()
	rf, ok := got["response_format"].(map[string]any)
	require.True(t, ok, "response_format must be set")
	require.Equal(t, "json_schema", rf["type"])
	js, ok := rf["json_schema"].(map[string]any)
	require.True(t, ok, "json_schema must be set")
	require.Equal(t, "response", js["name"])
	require.Equal(t, true, js["strict"], "strict must default to true for the openai dialect")
	require.EqualValues(t, schema, js["schema"])
}

// TestRequestJSONSchemaNonOpenAIOmitsStrict verifies that other
// OpenAI-compatible dialects (local gateways, Groq, ...) are not forced into
// strict mode, since they may reject or ignore it.
func TestRequestJSONSchemaNonOpenAIOmitsStrict(t *testing.T) {
	c, body := newMockOpenAIServer(t)

	schema := map[string]any{"type": "object"}
	s := c.Request(context.Background(), proto.Request{
		Model:      "llama3.2",
		API:        "local",
		JSONSchema: schema,
	})
	s.Next()
	s.Close() //nolint:errcheck

	got := body()
	rf, ok := got["response_format"].(map[string]any)
	require.True(t, ok, "response_format must be set")
	js, ok := rf["json_schema"].(map[string]any)
	require.True(t, ok, "json_schema must be set")
	require.Nil(t, js["strict"], "strict must be left unset for non-openai dialects")
}

// TestRequestJSONSchemaSentForPerplexityOnline is a regression test: the
// perplexity "online" model guard (that skips temperature/top_p/stop/etc.
// because the API rejects them) must not also silently drop JSONSchema.
// Without this, --json-schema would be a silent no-op for perplexity's
// online models, leaving only client-side validation to retry forever
// against a model that was never actually asked to follow the schema.
func TestRequestJSONSchemaSentForPerplexityOnline(t *testing.T) {
	c, body := newMockOpenAIServer(t)

	schema := map[string]any{"type": "object"}
	s := c.Request(context.Background(), proto.Request{
		Model:      "sonar-pro-online",
		API:        "perplexity",
		JSONSchema: schema,
	})
	s.Next()
	s.Close() //nolint:errcheck

	got := body()
	rf, ok := got["response_format"].(map[string]any)
	require.True(t, ok, "response_format must be set even for perplexity online models")
	require.Equal(t, "json_schema", rf["type"])
}

// TestRequestJSONSchemaOverridesResponseFormat verifies that --json-schema
// takes precedence over the loose --format json ResponseFormat when both
// are set.
func TestRequestJSONSchemaOverridesResponseFormat(t *testing.T) {
	c, body := newMockOpenAIServer(t)

	loose := "json"
	schema := map[string]any{"type": "object"}
	s := c.Request(context.Background(), proto.Request{
		Model:          "gpt-4o",
		API:            "openai",
		ResponseFormat: &loose,
		JSONSchema:     schema,
	})
	s.Next()
	s.Close() //nolint:errcheck

	got := body()
	rf, ok := got["response_format"].(map[string]any)
	require.True(t, ok, "response_format must be set")
	require.Equal(t, "json_schema", rf["type"], "JSONSchema must win over the loose json ResponseFormat")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	c := New(cfg)

	s := c.Request(context.Background(), proto.Request{Model: "gpt-4o"})
	require.False(t, s.Next(), "first Next() call should reach the end of the (empty) stream")
	require.Equal(t, 1, requests)

	require.False(t, s.Next(), "Next() must keep returning false once the stream is done")
	require.False(t, s.Next(), "Next() must keep returning false on repeated calls")
	require.Equal(t, 1, requests, "Next() must not issue another request after the stream is done")
}

func TestStreamIgnoresCommentOnlySSEEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, ": keepalive 1/2\r\n\r\n")
		fmt.Fprint(w, ": keepalive 2/2\r\n\r\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"2\"}}]}\r\n\r\n")
		fmt.Fprint(w, "data: [DONE]\r\n\r\n")
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	s := New(cfg).Request(context.Background(), proto.Request{Model: "test"})
	t.Cleanup(func() { _ = s.Close() })

	require.True(t, s.Next())
	chunk, err := s.Current()
	require.NoError(t, err)
	require.Equal(t, "2", chunk.Content)
	require.False(t, s.Next())
	require.NoError(t, s.Err())
}

func TestStreamKeepsAPIErrorAfterComment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keepalive\n\n")
		fmt.Fprint(w, "data: {\"error\":{\"message\":\"upstream failed\"}}\n\n")
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	s := New(cfg).Request(context.Background(), proto.Request{Model: "test"})
	t.Cleanup(func() { _ = s.Close() })

	require.False(t, s.Next())
	require.ErrorContains(t, s.Err(), "upstream failed")
}
