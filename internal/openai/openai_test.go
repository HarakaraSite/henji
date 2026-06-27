package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge.harakara.site/littleisland/henji/internal/proto"
	"github.com/stretchr/testify/require"
)

// TestCallToolsEmptyChoicesNoPanic is the PR#1 regression test.
// CallTools must return nil when the accumulator has no Choices.
func TestCallToolsEmptyChoicesNoPanic(t *testing.T) {
	s := &Stream{}
	require.Nil(t, s.CallTools())
}

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
