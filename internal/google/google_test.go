package google

import (
	"bufio"
	"strings"
	"testing"

	"github.com/charmbracelet/mods/internal/proto"
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
