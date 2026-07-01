package google

import (
	"bufio"
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
