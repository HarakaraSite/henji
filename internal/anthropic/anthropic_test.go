package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge.harakara.site/littleisland/henji/internal/proto"
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
