package google

import (
	"bufio"
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
