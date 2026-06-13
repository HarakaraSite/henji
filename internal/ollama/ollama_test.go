package ollama

import (
	"testing"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"
)

func TestStreamNextCompletesAfterDoneResponse(t *testing.T) {
	stream := &Stream{
		respCh: make(chan api.ChatResponse, 1),
		request: api.ChatRequest{
			Messages: []api.Message{
				{Role: proto.RoleUser, Content: "hello"},
			},
		},
		messages: []proto.Message{
			{Role: proto.RoleUser, Content: "hello"},
		},
		factory: func() {
			t.Fatal("factory should not restart when there are no tool calls")
		},
	}
	stream.respCh <- api.ChatResponse{
		Message: api.Message{
			Role:    proto.RoleAssistant,
			Content: "hi",
		},
		Done: true,
	}

	require.True(t, stream.Next())
	chunk, err := stream.Current()
	require.NoError(t, err)
	require.Equal(t, "hi", chunk.Content)

	require.False(t, stream.Next())
	require.Equal(t, []proto.Message{
		{Role: proto.RoleUser, Content: "hello"},
		{Role: proto.RoleAssistant, Content: "hi"},
	}, stream.Messages())
}

func TestStreamNextRestartsAfterFinalizedToolCall(t *testing.T) {
	restarted := false
	stream := &Stream{
		done:      true,
		finalized: true,
		factory: func() {
			restarted = true
		},
	}

	require.True(t, stream.Next())
	require.True(t, restarted)
	require.False(t, stream.done)
	require.False(t, stream.finalized)
}
